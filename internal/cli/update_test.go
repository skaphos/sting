// SPDX-License-Identifier: MIT

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skaphos/sting/internal/buildinfo"
	"github.com/skaphos/sting/internal/selfupdate"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestExitCodeContract pins every row of the documented exit-code table. These
// statuses are a public contract: scripts branch on them, so a change here is a
// breaking change.
func TestExitCodeContract(t *testing.T) {
	for _, tc := range []struct {
		action selfupdate.Action
		want   int
		reason string
	}{
		{selfupdate.ActionReplace, 0, "a successful update succeeds"},
		{selfupdate.ActionUpToDate, 0, "already current is not a failure"},
		{selfupdate.ActionDeferToManager, 2, "package-managed binaries exit non-zero"},
		{selfupdate.ActionGated, 3, "the platform gate is distinguishable"},
		{selfupdate.ActionRefuse, 4, "refusals are distinguishable from failures"},
	} {
		if got := exitCodeFor(tc.action); got != tc.want {
			t.Errorf("exitCodeFor(%v) = %d, want %d (%s)", tc.action, got, tc.want, tc.reason)
		}
	}
}

// TestExitCodesAreDistinct: the codes only carry information if no two outcomes
// share one.
func TestExitCodesAreDistinct(t *testing.T) {
	codes := map[int]string{
		exitVerificationFailed: "verification failed",
		exitPackageManaged:     "package managed",
		exitPlatformGated:      "platform gated",
		exitRefused:            "refused",
		exitPermission:         "permission",
	}
	if len(codes) != 5 {
		t.Errorf("exit codes collide: %v", codes)
	}
	for _, code := range []int{exitVerificationFailed, exitPackageManaged,
		exitPlatformGated, exitRefused, exitPermission} {
		if code == 0 {
			t.Error("a failure outcome was assigned the success status 0")
		}
	}
}

// TestNoFlagCanSkipVerification: verification is unskippable by design, so the
// command must expose no way to turn it off.
func TestNoFlagCanSkipVerification(t *testing.T) {
	forbidden := []string{"insecure", "skip-verify", "no-verify", "force", "unsafe", "no-check-signature"}
	updateCmd.Flags().VisitAll(func(f *pflag.Flag) {
		for _, bad := range forbidden {
			if strings.Contains(f.Name, bad) {
				t.Errorf("update exposes a flag that could bypass verification: --%s", f.Name)
			}
		}
	})
}

// newTestUpdater wires the command's updater to a local server so no test
// touches the network.
func newTestUpdater(t *testing.T, execPath, tag string, assets map[string]string) *selfupdate.Updater {
	t.Helper()

	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		type asset struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		}
		payload := struct {
			TagName string  `json:"tag_name"`
			Assets  []asset `json:"assets"`
		}{TagName: tag}
		for name := range assets {
			payload.Assets = append(payload.Assets, asset{Name: name, URL: base + "/a"})
		}
		b, _ := json.Marshal(payload)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	base = srv.URL

	return &selfupdate.Updater{
		Client:   &selfupdate.Client{HTTP: srv.Client(), APIBase: srv.URL},
		Verifier: selfupdate.NewVerifier(),
		ExecPath: func() (string, error) { return execPath, nil },
		Run:      func(string, ...string) error { return errUnowned },
	}
}

var errUnowned = errors.New("not owned")

// withUpdater swaps the command's updater for the duration of a test.
func withUpdater(t *testing.T, u *selfupdate.Updater) {
	t.Helper()
	original := newUpdater
	newUpdater = func() *selfupdate.Updater { return u }
	t.Cleanup(func() { newUpdater = original })
}

func runUpdateCmd(t *testing.T) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetContext(context.Background())
	err := runUpdate(cmd, nil)
	return buf.String(), err
}

// TestUpdateDefersToPackageManager covers the documented output and status for
// a Homebrew-installed binary.
func TestUpdateDefersToPackageManager(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("HOMEBREW_PREFIX", "")

	binary := filepath.Join(home, "Cellar", "sting", "1.0.0", "bin", "sting")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	if err := os.WriteFile(binary, []byte("old"), 0o755); err != nil {
		t.Fatalf("creating binary: %v", err)
	}

	withVersion(t, "v1.0.0")
	withUpdater(t, newTestUpdater(t, binary, "v9.0.0", map[string]string{"checksums.txt": "x"}))
	t.Cleanup(resetUpdateFlags)

	out, err := runUpdateCmd(t)
	if err == nil {
		t.Fatal("expected a non-zero outcome for a package-managed binary")
	}

	var coder interface{ ExitCode() int }
	if !errors.As(err, &coder) {
		t.Fatalf("error does not carry an exit code: %v", err)
	}
	if coder.ExitCode() != exitPackageManaged {
		t.Errorf("exit code = %d, want %d", coder.ExitCode(), exitPackageManaged)
	}
	if !strings.Contains(out, "brew upgrade --cask sting") {
		t.Errorf("output does not tell the user the correct command:\n%s", out)
	}
	if !strings.Contains(out, "Refusing to overwrite a package-managed file") {
		t.Errorf("output does not explain the refusal:\n%s", out)
	}

	// Nothing may have been written.
	if data, readErr := os.ReadFile(binary); readErr == nil && string(data) != "old" {
		t.Error("the binary was modified for a package-managed install")
	}
}

// TestUpdateRefusesUnknownVersion covers the refusal path and its status.
func TestUpdateRefusesUnknownVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	binary := filepath.Join(home, "sting")
	if err := os.WriteFile(binary, []byte("old"), 0o755); err != nil {
		t.Fatalf("creating binary: %v", err)
	}

	withUpdater(t, newTestUpdater(t, binary, "v9.0.0", nil))
	t.Cleanup(resetUpdateFlags)

	withVersion(t, "dev")

	out, err := runUpdateCmd(t)
	if err == nil {
		t.Fatal("expected a refusal when the running version is unknown")
	}
	if !strings.Contains(out, "--version") {
		t.Errorf("output does not tell the user how to proceed:\n%s", out)
	}
}

// TestUpdateCheckWritesNothing: --check reports and stops.
func TestUpdateCheckWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("HOMEBREW_PREFIX", "")

	binary := filepath.Join(home, "bin", "sting")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	if err := os.WriteFile(binary, []byte("old"), 0o755); err != nil {
		t.Fatalf("creating binary: %v", err)
	}

	withVersion(t, "v1.0.0")
	withUpdater(t, newTestUpdater(t, binary, "v9.0.0", map[string]string{
		selfupdate.AssetName("v9.0.0"): "x",
	}))
	updateCheckOnly = true
	t.Cleanup(resetUpdateFlags)

	out, err := runUpdateCmd(t)
	if err != nil {
		t.Fatalf("--check returned an error: %v", err)
	}
	if !strings.Contains(out, "current:") || !strings.Contains(out, "action:") {
		t.Errorf("--check output is incomplete:\n%s", out)
	}

	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("reading binary: %v", err)
	}
	if string(data) != "old" {
		t.Error("--check modified the binary")
	}
}

// withVersion stamps a known running version, so tests that exercise later
// decisions are not short-circuited by the "unknown version" refusal.
func withVersion(t *testing.T, v string) {
	t.Helper()
	original := Version
	Version = v
	t.Cleanup(func() { Version = original })
}

func resetUpdateFlags() {
	updateCheckOnly = false
	updateTarget = ""
}

func TestWriteGateNamesTheCause(t *testing.T) {
	var buf bytes.Buffer
	writeGate(&buf, &selfupdate.Plan{
		Target: "v2.0.0",
		Reason: "self-update is not enabled on Windows: release binaries are not yet Authenticode-signed",
	})

	out := buf.String()
	for _, want := range []string{"v2.0.0", "Authenticode", "Download:"} {
		if !strings.Contains(out, want) {
			t.Errorf("gate output missing %q:\n%s", want, out)
		}
	}
}

func TestDisplayVersion(t *testing.T) {
	if got := displayVersion(buildinfo.Info{Version: "v1.0.0"}); got != "v1.0.0" {
		t.Errorf("displayVersion() = %q, want v1.0.0", got)
	}
	if got := displayVersion(buildinfo.Info{}); got != "(version unavailable)" {
		t.Errorf("displayVersion() = %q, want the unavailable marker", got)
	}
}

func TestCheckActionDescribesEveryOutcome(t *testing.T) {
	for _, action := range []selfupdate.Action{
		selfupdate.ActionReplace, selfupdate.ActionUpToDate,
		selfupdate.ActionDeferToManager, selfupdate.ActionGated, selfupdate.ActionRefuse,
	} {
		plan := &selfupdate.Plan{Action: action, Reason: "because"}
		if got := checkAction(plan); got == "" {
			t.Errorf("checkAction(%v) returned an empty description", action)
		}
	}
}
