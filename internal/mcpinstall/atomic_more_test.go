// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteAtomicNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")
	if err := WriteAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not model Unix permission bits, so only assert them elsewhere.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestWriteAtomicOverwritePreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exist.txt")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Pass a different mode; existing file's mode should be preserved.
	if err := WriteAtomic(path, []byte("new content"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new content" {
		t.Errorf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want preserved 0640", info.Mode().Perm())
	}
}

func TestWriteAtomicThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := WriteAtomic(link, []byte("updated"), 0o644); err != nil {
		t.Fatalf("WriteAtomic through symlink: %v", err)
	}
	// The real target must be updated, and the link must still be a symlink.
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "updated" {
		t.Errorf("target content = %q, want updated", data)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("link is no longer a symlink after write")
	}
}

func TestWriteAtomicParentMissing(t *testing.T) {
	// CreateTemp into a non-existent dir should fail.
	dir := t.TempDir()
	path := filepath.Join(dir, "no-such-dir", "f.txt")
	if err := WriteAtomic(path, []byte("x"), 0o644); err == nil {
		t.Error("WriteAtomic into missing parent expected error")
	}
}

func TestWriteAtomicRenameOntoDir(t *testing.T) {
	// resolved path is an existing directory; rename of the temp file onto a
	// directory must fail, exercising the rename-error branch.
	dir := t.TempDir()
	target := filepath.Join(dir, "asdir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(target, []byte("x"), 0o644); err == nil {
		t.Error("WriteAtomic onto a directory expected error")
	}
}

func TestResolveTargetDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "nope"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	// EvalSymlinks should fail on a dangling link.
	if _, _, _, err := resolveTarget(link); err == nil {
		t.Error("resolveTarget on dangling symlink expected error")
	}
}

func TestResolveTargetMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	resolved, _, existed, err := resolveTarget(path)
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if existed {
		t.Error("existed = true for missing path")
	}
	if resolved != path {
		t.Errorf("resolved = %q, want %q", resolved, path)
	}
}
