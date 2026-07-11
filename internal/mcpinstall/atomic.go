// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// WriteAtomic writes data to path via a temp-file-plus-rename so readers never
// observe a truncated file. If path already exists as a regular file (or a
// symlink to one), its mode is preserved; otherwise mode is used. Symlinks are
// resolved and the write lands on the target, leaving the link in place. The
// parent directory must already exist.
func WriteAtomic(path string, data []byte, mode fs.FileMode) error {
	resolved, existingMode, existed, err := resolveTarget(path)
	if err != nil {
		return err
	}
	if existed {
		mode = existingMode
	}
	dir := filepath.Dir(resolved)
	tmp, err := os.CreateTemp(dir, ".mcpinstall.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	// Flush the temp file's contents to disk before the rename so a crash cannot
	// leave a renamed-but-empty (or truncated) config behind.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, resolved); err != nil {
		cleanup()
		return fmt.Errorf("rename: %w", err)
	}
	// Persist the rename itself by fsyncing the parent directory, so the file is
	// durably present under its final name after a crash.
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync dir: %w", err)
	}
	return nil
}

// syncDir fsyncs a directory so a prior rename into it is durable. It is a no-op
// on platforms where opening or syncing a directory is unsupported (Windows).
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// resolveTarget returns the concrete write path (after symlink resolution), the
// existing regular-file mode (if any), and whether the target existed.
func resolveTarget(path string) (string, fs.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, 0, false, nil
		}
		return "", 0, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", 0, false, fmt.Errorf("resolve symlink %q: %w", path, err)
		}
		targetInfo, err := os.Stat(target)
		if err != nil {
			return "", 0, false, fmt.Errorf("stat symlink target %q: %w", target, err)
		}
		return target, targetInfo.Mode().Perm(), true, nil
	}
	return path, info.Mode().Perm(), true, nil
}
