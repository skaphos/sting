// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateEnv clears every environment variable that influences ownership
// detection, so a developer's real GOPATH or Homebrew install cannot make a
// test pass or fail by accident.
func isolateEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("HOMEBREW_PREFIX", "")
	return home
}

// binaryAt creates a file to stand in for an installed sting.
func binaryAt(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("creating binary: %v", err)
	}
	return path
}

func fixedPath(p string) func() (string, error) {
	return func() (string, error) { return p, nil }
}

// neverOwned stands in for rpm/dpkg reporting "not my file".
func neverOwned(string, ...string) error { return errors.New("not owned") }

// alwaysOwned stands in for a package manager claiming the file.
func alwaysOwned(string, ...string) error { return nil }

// ownedBySpecific claims the file only for the named manager.
func ownedBySpecific(manager string) commandRunner {
	return func(name string, _ ...string) error {
		if name == manager {
			return nil
		}
		return errors.New("not owned")
	}
}

func TestClassifyUnmanaged(t *testing.T) {
	home := isolateEnv(t)
	path := binaryAt(t, filepath.Join(home, "bin", "sting"))

	got := classify(fixedPath(path), neverOwned)
	if got.Owner != OwnerUnmanaged {
		t.Errorf("Owner = %v, want OwnerUnmanaged", got.Owner)
	}
	if got.Owner.Managed() {
		t.Error("unmanaged binary reported as managed")
	}
	if got.RealPath != path {
		t.Errorf("RealPath = %q, want %q", got.RealPath, path)
	}
}

// TestClassifyHomebrewViaSymlink is the case the ordering exists for: Homebrew
// puts a symlink on PATH pointing into a versioned cellar. Judging the link
// rather than its target would classify a package-managed binary as unmanaged
// and overwrite it.
func TestClassifyHomebrewViaSymlink(t *testing.T) {
	home := isolateEnv(t)

	real := binaryAt(t, filepath.Join(home, "Cellar", "sting", "1.0.0", "bin", "sting"))
	link := filepath.Join(home, "bin", "sting")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("creating link directory: %v", err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := classify(fixedPath(link), neverOwned)
	if got.Owner != OwnerHomebrew {
		t.Fatalf("Owner = %v, want OwnerHomebrew (symlink was not resolved before judging)", got.Owner)
	}
	if got.UpgradeCommand != "brew upgrade --cask sting" {
		t.Errorf("UpgradeCommand = %q", got.UpgradeCommand)
	}
	if got.RealPath == link {
		t.Error("RealPath is the symlink, not its target")
	}
}

func TestClassifyHomebrewPrefix(t *testing.T) {
	home := isolateEnv(t)
	prefix := filepath.Join(home, "brew")
	t.Setenv("HOMEBREW_PREFIX", prefix)
	path := binaryAt(t, filepath.Join(prefix, "bin", "sting"))

	if got := classify(fixedPath(path), neverOwned); got.Owner != OwnerHomebrew {
		t.Errorf("Owner = %v, want OwnerHomebrew", got.Owner)
	}
}

// TestHomebrewPrefixDoesNotMatchSiblingDirectory guards the whole-segment
// comparison: "/opt/homebrew-other" must not match the "/opt/homebrew" prefix.
func TestHomebrewPrefixDoesNotMatchSiblingDirectory(t *testing.T) {
	home := isolateEnv(t)
	t.Setenv("HOMEBREW_PREFIX", filepath.Join(home, "brew"))
	path := binaryAt(t, filepath.Join(home, "brew-other", "bin", "sting"))

	if got := classify(fixedPath(path), neverOwned); got.Owner == OwnerHomebrew {
		t.Error("a sibling directory matched the Homebrew prefix")
	}
}

func TestClassifyPackageManagers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("rpm and dpkg detection only applies on linux")
	}

	for _, tc := range []struct {
		manager   string
		wantOwner Owner
		wantCmd   string
	}{
		{"rpm", OwnerRPM, "sudo dnf upgrade sting"},
		{"dpkg", OwnerDPKG, ""},
	} {
		t.Run(tc.manager, func(t *testing.T) {
			home := isolateEnv(t)
			path := binaryAt(t, filepath.Join(home, "usr", "bin", "sting"))

			got := classify(fixedPath(path), ownedBySpecific(tc.manager))
			if got.Owner != tc.wantOwner {
				t.Fatalf("Owner = %v, want %v", got.Owner, tc.wantOwner)
			}
			if !got.Owner.Managed() {
				t.Error("package-managed binary reported as unmanaged")
			}
			if got.UpgradeCommand == "" {
				t.Error("no upgrade command provided for a managed binary")
			}
			if tc.wantCmd != "" && got.UpgradeCommand != tc.wantCmd {
				t.Errorf("UpgradeCommand = %q, want %q", got.UpgradeCommand, tc.wantCmd)
			}
		})
	}
}

// TestDpkgAdviceDoesNotPromiseAnAptRepo: there is no hosted apt repository, so
// telling a user to "apt upgrade sting" would be advice that cannot work.
func TestDpkgAdviceDoesNotPromiseAnAptRepo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("dpkg detection only applies on linux")
	}
	home := isolateEnv(t)
	path := binaryAt(t, filepath.Join(home, "usr", "bin", "sting"))

	got := classify(fixedPath(path), ownedBySpecific("dpkg"))
	if got.UpgradeCommand == "" {
		t.Fatal("no upgrade command for a dpkg-owned binary")
	}
	if want := "apt upgrade"; contains(got.UpgradeCommand, want) {
		t.Errorf("advice implies an apt repository that does not exist: %q", got.UpgradeCommand)
	}
	if !contains(got.UpgradeCommand, "dpkg -i") {
		t.Errorf("advice does not tell the user how to install the package: %q", got.UpgradeCommand)
	}
}

func TestClassifyGoToolchain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, home string) string
	}{
		{
			name: "GOBIN",
			setup: func(t *testing.T, home string) string {
				gobin := filepath.Join(home, "custom-bin")
				t.Setenv("GOBIN", gobin)
				return binaryAt(t, filepath.Join(gobin, "sting"))
			},
		},
		{
			name: "GOPATH/bin",
			setup: func(t *testing.T, home string) string {
				gopath := filepath.Join(home, "gopath")
				t.Setenv("GOPATH", gopath)
				return binaryAt(t, filepath.Join(gopath, "bin", "sting"))
			},
		},
		{
			name: "default ~/go/bin",
			setup: func(t *testing.T, home string) string {
				return binaryAt(t, filepath.Join(home, "go", "bin", "sting"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateEnv(t)
			path := tc.setup(t, home)

			got := classify(fixedPath(path), neverOwned)
			if got.Owner != OwnerGoToolchain {
				t.Fatalf("Owner = %v, want OwnerGoToolchain", got.Owner)
			}
			if got.UpgradeCommand != "go install github.com/skaphos/sting/cmd/sting@latest" {
				t.Errorf("UpgradeCommand = %q", got.UpgradeCommand)
			}
		})
	}
}

// TestClassifyUndeterminable: when the path cannot be resolved, sting refuses
// rather than guessing where to write.
func TestClassifyUndeterminable(t *testing.T) {
	isolateEnv(t)

	t.Run("executable path unavailable", func(t *testing.T) {
		got := classify(func() (string, error) { return "", errors.New("no") }, neverOwned)
		if got.Owner != OwnerUndeterminable {
			t.Errorf("Owner = %v, want OwnerUndeterminable", got.Owner)
		}
	})

	t.Run("path does not resolve", func(t *testing.T) {
		got := classify(fixedPath(filepath.Join(t.TempDir(), "missing", "sting")), neverOwned)
		if got.Owner != OwnerUndeterminable {
			t.Errorf("Owner = %v, want OwnerUndeterminable", got.Owner)
		}
	})
}

// TestHomebrewWinsOverPackageManagers pins the documented ordering: the checks
// overlap, and the most specific answer must win.
func TestHomebrewWinsOverPackageManagers(t *testing.T) {
	home := isolateEnv(t)
	path := binaryAt(t, filepath.Join(home, "Cellar", "sting", "1.0.0", "bin", "sting"))

	if got := classify(fixedPath(path), alwaysOwned); got.Owner != OwnerHomebrew {
		t.Errorf("Owner = %v, want OwnerHomebrew to take precedence", got.Owner)
	}
}

func TestOwnerString(t *testing.T) {
	for _, tt := range []struct {
		owner Owner
		want  string
	}{
		{OwnerUnmanaged, "unmanaged"},
		{OwnerHomebrew, "Homebrew"},
		{OwnerRPM, "an RPM package"},
		{OwnerDPKG, "a Debian package"},
		{OwnerGoToolchain, "the Go toolchain"},
		{OwnerUndeterminable, "an undeterminable location"},
	} {
		if got := tt.owner.String(); got != tt.want {
			t.Errorf("Owner(%d).String() = %q, want %q", tt.owner, got, tt.want)
		}
	}
}

func TestUnderDir(t *testing.T) {
	for _, tt := range []struct {
		path, dir string
		want      bool
	}{
		{"/opt/homebrew/bin/sting", "/opt/homebrew", true},
		{"/opt/homebrew-other/bin/sting", "/opt/homebrew", false},
		{"/usr/bin/sting", "/opt/homebrew", false},
		{"/opt/homebrew", "/opt/homebrew", true},
		{"/anything", "", false},
	} {
		if got := underDir(tt.path, tt.dir); got != tt.want {
			t.Errorf("underDir(%q, %q) = %v, want %v", tt.path, tt.dir, got, tt.want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
