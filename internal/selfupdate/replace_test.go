// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplaceSwapsBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	if err := Replace(target, []byte("new binary")); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if string(got) != "new binary" {
		t.Errorf("content = %q, want %q", got, "new binary")
	}
}

// TestReplacePreservesMode: the replacement must stay executable, or the update
// leaves the user with a file they cannot run.
func TestReplacePreservesMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	if err := Replace(target, []byte("new")); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("replaced binary is not executable: mode %v", info.Mode().Perm())
	}
}

// TestReplaceLeavesNoTemporaryFiles: a partially written update must not leave
// debris in the install directory.
func TestReplaceLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	if err := Replace(target, []byte("new")); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading directory: %v", err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only the binary to remain, found %v", names)
	}
}

// TestReplaceIsAtomic: at every observable moment the target is one complete
// binary. The temporary file is written alongside and renamed, so the target is
// never seen half-written -- for the binary you are currently running, that is
// the difference between an interrupted update and an unusable install.
func TestReplaceIsAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting")
	old := []byte("old binary content")
	replacement := []byte("new binary content, of a different length entirely")

	if err := os.WriteFile(target, old, 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	// Observe the target while the replacement runs.
	done := make(chan error, 1)
	go func() { done <- Replace(target, replacement) }()

	for range 200 {
		data, err := os.ReadFile(target)
		if err != nil {
			// The path must never be absent: a rename replaces it in
			// one step rather than unlinking first.
			t.Errorf("target disappeared during replacement: %v", err)
			continue
		}
		if s := string(data); s != string(old) && s != string(replacement) {
			t.Fatalf("observed a partially written binary: %q", s)
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
}

// TestReplaceUnwritableDirectory: the failure names the path and does not
// attempt to escalate privileges.
func TestReplaceUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permissions are not enforced")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "sting")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("making directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := Replace(target, []byte("new"))
	if err == nil {
		t.Fatal("expected a permission failure")
	}
	if !errors.Is(err, ErrPermission) {
		t.Fatalf("error = %v, want ErrPermission", err)
	}
	if !contains(err.Error(), dir) {
		t.Errorf("error does not name the path: %v", err)
	}
	if !contains(err.Error(), "will not escalate privileges") {
		t.Errorf("error does not state that sting will not escalate: %v", err)
	}

	// The original must be untouched.
	if data, readErr := os.ReadFile(target); readErr == nil && string(data) != "old" {
		t.Errorf("target was modified despite the failure: %q", data)
	}
}

// TestReplaceWindowsRenamesAside exercises the platform-gated path directly.
// It is not reached in production -- Windows self-replacement is disabled
// pending Authenticode signing -- but the mechanism is implemented so that
// lifting the gate is a policy change rather than a new design.
func TestReplaceWindowsRenamesAside(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	if err := replaceWindows(target, []byte("new")); err != nil {
		t.Fatalf("replaceWindows() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

// TestCleanupDisplacedIsAlwaysSafe: a leftover file must never stop sting from
// running, so cleanup failures are ignored by design.
func TestCleanupDisplacedIsAlwaysSafe(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting")

	// No displaced file at all.
	CleanupDisplaced(target)

	// A displaced file that does exist.
	if err := os.WriteFile(target+displacedSuffix, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seeding displaced file: %v", err)
	}
	CleanupDisplaced(target)
	if _, err := os.Stat(target + displacedSuffix); !os.IsNotExist(err) {
		t.Errorf("displaced file was not removed: %v", err)
	}

	// A path that cannot exist.
	CleanupDisplaced(filepath.Join(dir, "no", "such", "dir", "sting"))
}

func TestReplaceDispatchesByPlatform(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sting")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	if err := Replace(target, []byte("new")); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	// On Windows the rename-aside path runs; everywhere else the atomic
	// rename does. Both must end with exactly one working binary.
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(target + displacedSuffix); !os.IsNotExist(err) {
			t.Error("the atomic path left a displaced file behind")
		}
	}
}

func TestExistingModeFallsBack(t *testing.T) {
	if got := existingMode(filepath.Join(t.TempDir(), "missing")); got != defaultMode {
		t.Errorf("existingMode(missing) = %v, want %v", got, defaultMode)
	}
}
