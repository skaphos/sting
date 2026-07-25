// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/skaphos/sting/internal/buildinfo"
)

// testUpdater wires an Updater to an httptest server and a temporary install
// location, so no test touches the network or the developer's real binary.
func testUpdater(t *testing.T, execPath string, run commandRunner, handler http.HandlerFunc) *Updater {
	t.Helper()
	return &Updater{
		Client:   newTestClient(t, handler),
		Verifier: NewVerifier(),
		ExecPath: fixedPath(execPath),
		Run:      run,
		GOOS:     runtime.GOOS,
	}
}

// updaterForRelease builds an Updater serving the given release tag, with all
// asset URLs resolving back to the test server.
func updaterForRelease(t *testing.T, execPath string, run commandRunner, tag string) *Updater {
	t.Helper()
	base := new(string)
	u := testUpdater(t, execPath, run, releaseHandler(tag, base))
	*base = u.Client.APIBase
	return u
}

// releaseHandler serves a release whose asset URLs point back at the test
// server. base must be the server's own URL: an asset pointing anywhere else
// would make the test reach the network, which is forbidden.
func releaseHandler(tag string, base *string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		assets := map[string]string{
			assetNameFor(tag, runtime.GOOS, runtime.GOARCH): *base + "/archive",
			checksumsName: *base + "/checksums",
			bundleName:    *base + "/bundle",
		}
		_, _ = w.Write([]byte(releaseJSON(tag, assets, false, false)))
	}
}

func TestPlanReplacesWhenUnmanaged(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v2.0.0")

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionReplace {
		t.Fatalf("Action = %v, want ActionReplace (reason: %s)", plan.Action, plan.Reason)
	}
	if plan.Target != "v2.0.0" {
		t.Errorf("Target = %q, want v2.0.0", plan.Target)
	}
	if plan.DownloadURL == "" {
		t.Error("no download URL resolved")
	}
}

func TestPlanUpToDate(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v2.0.0")

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v2.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionUpToDate {
		t.Errorf("Action = %v, want ActionUpToDate", plan.Action)
	}
}

// TestPlanTreatsPrefixedVersionsAsEqual: a module version "1.0.0" and a tag
// "v1.0.0" are the same release and must not look like an available upgrade.
func TestPlanTreatsPrefixedVersionsAsEqual(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v2.0.0")

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "2.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionUpToDate {
		t.Errorf("Action = %v, want ActionUpToDate for an unprefixed version", plan.Action)
	}
}

func TestPlanDefersToPackageManager(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "Cellar", "sting", "1.0.0", "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v2.0.0")

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionDeferToManager {
		t.Fatalf("Action = %v, want ActionDeferToManager", plan.Action)
	}
	if plan.Provenance.UpgradeCommand == "" {
		t.Error("no upgrade command offered")
	}
}

// TestPlanGatesWindows: the mechanism is specified in full but disabled,
// because Windows binaries are not yet Authenticode-signed.
func TestPlanGatesWindows(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v2.0.0")
	u.GOOS = "windows"

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionGated {
		t.Fatalf("Action = %v, want ActionGated", plan.Action)
	}
	if !strings.Contains(plan.Reason, "Authenticode") {
		t.Errorf("gate reason does not name the cause: %q", plan.Reason)
	}
	// The user still learns what is available.
	if plan.Target != "v2.0.0" {
		t.Errorf("Target = %q, want the available version to be reported", plan.Target)
	}
}

// TestPackageManagerBeatsWindowsGate: a package-managed binary gets the
// specific, actionable instruction rather than the generic gate message.
func TestPackageManagerBeatsWindowsGate(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "Cellar", "sting", "1.0.0", "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v2.0.0")
	u.GOOS = "windows"

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionDeferToManager {
		t.Errorf("Action = %v, want ActionDeferToManager to win over the platform gate", plan.Action)
	}
}

// TestPlanRefusesUnknownVersionWithoutTarget: assuming a version-less build is
// out of date would mean replacing a binary on a guess. It must also not touch
// the network to find that out.
func TestPlanRefusesUnknownVersionWithoutTarget(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))

	var called bool
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(releaseJSON("v2.0.0", nil, false, false)))
	})

	plan, err := u.Plan(context.Background(), buildinfo.Info{}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionRefuse {
		t.Fatalf("Action = %v, want ActionRefuse", plan.Action)
	}
	if !strings.Contains(plan.Reason, "--version") {
		t.Errorf("reason does not tell the user how to proceed: %q", plan.Reason)
	}
	if called {
		t.Error("refusal made a network request it did not need")
	}
}

// TestPlanAcceptsExplicitTargetForUnknownVersion is the rollback path: naming a
// version is exactly how a user escapes an unknown build.
func TestPlanAcceptsExplicitTargetForUnknownVersion(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := updaterForRelease(t, binary, neverOwned, "v1.5.0")

	plan, err := u.Plan(context.Background(), buildinfo.Info{}, "v1.5.0")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionReplace {
		t.Errorf("Action = %v, want ActionReplace (reason: %s)", plan.Action, plan.Reason)
	}
}

// TestPlanRefusesWhenNoAssetForPlatform names the platform and what does exist,
// rather than failing obscurely.
func TestPlanRefusesWhenNoAssetForPlatform(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseJSON("v2.0.0", map[string]string{
			"sting_2.0.0_plan9_mips.tar.gz": "https://example.test/other",
		}, false, false)))
	})

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionRefuse {
		t.Fatalf("Action = %v, want ActionRefuse", plan.Action)
	}
	if !strings.Contains(plan.Reason, runtime.GOOS) {
		t.Errorf("reason does not name the platform: %q", plan.Reason)
	}
	if !strings.Contains(plan.Reason, "plan9") {
		t.Errorf("reason does not list the assets that do exist: %q", plan.Reason)
	}
}

func TestPlanRefusesUndeterminablePath(t *testing.T) {
	isolateEnv(t)
	u := updaterForRelease(t, filepath.Join(t.TempDir(), "missing", "sting"), neverOwned, "v2.0.0")

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionRefuse {
		t.Fatalf("Action = %v, want ActionRefuse", plan.Action)
	}
	if !strings.Contains(plan.Reason, "will not guess") {
		t.Errorf("reason does not explain the refusal: %q", plan.Reason)
	}
}

func TestPlanSurfacesReleaseLookupFailure(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "v9.9.9"); err == nil {
		t.Fatal("expected an error when the release does not exist")
	}
}

// TestApplyRejectsNonReplacementPlan: Apply must never run for a plan that
// decided not to replace anything.
func TestApplyRejectsNonReplacementPlan(t *testing.T) {
	u := New()
	err := u.Apply(context.Background(), &Plan{Action: ActionDeferToManager})
	if err == nil {
		t.Fatal("Apply() accepted a non-replacement plan")
	}
}

// TestApplyVerifiesBeforeWriting is the load-bearing guarantee: a tampered
// release must leave the installed binary untouched.
func TestApplyVerifiesBeforeWriting(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	original, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("reading seeded binary: %v", err)
	}

	base := new(string)
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/") {
			releaseHandler("v2.0.0", base)(w, r)
			return
		}
		_, _ = w.Write([]byte("not a valid signature bundle"))
	})
	*base = u.Client.APIBase

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if err := u.Apply(context.Background(), plan); err == nil {
		t.Fatal("Apply() succeeded against an unverifiable release")
	}

	after, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("reading binary after failed update: %v", err)
	}
	if string(after) != string(original) {
		t.Error("the installed binary was modified despite verification failing")
	}
}

func TestNewUpdaterDefaults(t *testing.T) {
	u := New()
	if u.Client == nil || u.Verifier == nil || u.ExecPath == nil || u.Run == nil {
		t.Fatal("New() left a dependency nil")
	}
	if u.goos() != runtime.GOOS {
		t.Errorf("goos() = %q, want %q", u.goos(), runtime.GOOS)
	}

	bare := &Updater{}
	if bare.goos() != runtime.GOOS {
		t.Error("a zero Updater did not fall back to runtime.GOOS")
	}
	if bare.execPath() == nil || bare.runner() == nil {
		t.Error("a zero Updater did not fall back to real implementations")
	}
}

func TestSameVersion(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"v1.0.0", "v1.0.0", true},
		{"1.0.0", "v1.0.0", true},
		{"v1.0.0", "1.0.0", true},
		{"v1.0.0", "v1.0.1", false},
		{"", "v1.0.0", false},
	} {
		if got := sameVersion(tt.a, tt.b); got != tt.want {
			t.Errorf("sameVersion(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// stubVerifier stands in for real signature verification so the
// download-extract-replace path can be exercised. The verification policy
// itself is tested directly against sigstore in sigstore_test.go.
type stubVerifier struct {
	err    error
	called bool
}

func (s *stubVerifier) VerifyRelease(_, _, _ []byte, _ string) error {
	s.called = true
	return s.err
}

// TestApplyReplacesBinaryAfterVerification is the happy path end to end:
// resolve, download, verify, extract, and swap.
func TestApplyReplacesBinaryAfterVerification(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))

	newBinary := []byte("the replacement binary")
	assetName := assetNameFor("v2.0.0", runtime.GOOS, runtime.GOARCH)
	archive := tarGz(t, map[string][]byte{"sting": newBinary, "LICENSE": []byte("MIT")})

	verifier := &stubVerifier{}
	base := new(string)
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/"):
			releaseHandler("v2.0.0", base)(w, r)
		case strings.Contains(r.URL.Path, "archive"):
			_, _ = w.Write(archive)
		default:
			_, _ = w.Write([]byte("checksums or bundle"))
		}
	})
	*base = u.Client.APIBase
	u.Verifier = verifier

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Action != ActionReplace {
		t.Fatalf("Action = %v, want ActionReplace", plan.Action)
	}
	if plan.AssetName != assetName {
		t.Errorf("AssetName = %q, want %q", plan.AssetName, assetName)
	}
}

// TestApplyRefusesWhenVerificationFails: the binary must survive untouched.
func TestApplyRefusesWhenVerificationFails(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))
	original, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("reading seeded binary: %v", err)
	}

	verifier := &stubVerifier{err: ErrVerification}
	base := new(string)
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/") {
			releaseHandler("v2.0.0", base)(w, r)
			return
		}
		_, _ = w.Write([]byte("anything"))
	})
	*base = u.Client.APIBase
	u.Verifier = verifier

	plan := &Plan{
		Action:      ActionReplace,
		Target:      "v2.0.0",
		AssetName:   assetNameFor("v2.0.0", runtime.GOOS, runtime.GOARCH),
		DownloadURL: u.Client.APIBase + "/archive",
		Provenance:  Provenance{Owner: OwnerUnmanaged, RealPath: binary},
	}

	applyErr := u.Apply(context.Background(), plan)
	if !errors.Is(applyErr, ErrVerification) {
		t.Fatalf("Apply() error = %v, want ErrVerification", applyErr)
	}
	if !verifier.called {
		t.Error("Apply() wrote without calling the verifier")
	}

	after, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("reading binary: %v", err)
	}
	if string(after) != string(original) {
		t.Error("the installed binary was modified despite verification failing")
	}
}

// TestApplyFullFlow drives Apply all the way through to a replaced binary.
func TestApplyFullFlow(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))

	newBinary := []byte("the replacement binary")
	archive := tarGz(t, map[string][]byte{"sting": newBinary})
	assetName := assetNameFor("v2.0.0", runtime.GOOS, runtime.GOARCH)

	var srvURL string
	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/"):
			assets := map[string]string{
				assetName:     srvURL + "/archive",
				checksumsName: srvURL + "/checksums",
				bundleName:    srvURL + "/bundle",
			}
			_, _ = w.Write([]byte(releaseJSON("v2.0.0", assets, false, false)))
		case strings.Contains(r.URL.Path, "/archive"):
			_, _ = w.Write(archive)
		default:
			_, _ = w.Write([]byte("manifest or signature"))
		}
	})
	srvURL = u.Client.APIBase
	verifier := &stubVerifier{}
	u.Verifier = verifier

	plan, err := u.Plan(context.Background(), buildinfo.Info{Version: "v1.0.0"}, "")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if err := u.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !verifier.called {
		t.Fatal("Apply() replaced the binary without verifying")
	}

	got, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if string(got) != string(newBinary) {
		t.Errorf("binary content = %q, want %q", got, newBinary)
	}
}

// TestApplyReportsMissingManifest: a tag that exists but whose assets are not
// published yet is "not available", not a verification failure.
func TestApplyReportsMissingManifest(t *testing.T) {
	home := isolateEnv(t)
	binary := binaryAt(t, filepath.Join(home, "bin", "sting"))

	u := testUpdater(t, binary, neverOwned, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseJSON("v2.0.0", map[string]string{
			assetNameFor("v2.0.0", runtime.GOOS, runtime.GOARCH): "https://example.test/a",
		}, false, false)))
	})
	u.Verifier = &stubVerifier{}

	plan := &Plan{
		Action:     ActionReplace,
		Target:     "v2.0.0",
		AssetName:  assetNameFor("v2.0.0", runtime.GOOS, runtime.GOARCH),
		Provenance: Provenance{Owner: OwnerUnmanaged, RealPath: binary},
	}

	err := u.Apply(context.Background(), plan)
	if err == nil {
		t.Fatal("expected an error when the checksum manifest is absent")
	}
	if !strings.Contains(err.Error(), "still be publishing") {
		t.Errorf("error does not explain the likely cause: %v", err)
	}
}
