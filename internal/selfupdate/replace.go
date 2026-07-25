// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// ErrPermission reports that the binary could not be replaced because the
// filesystem said no. sting never attempts to escalate privileges; it names the
// path and the reason and lets the user decide.
var ErrPermission = errors.New("permission denied")

// displacedSuffix marks a binary renamed aside on platforms that forbid
// overwriting a running executable.
const displacedSuffix = ".old"

// defaultMode is used when the existing binary's mode cannot be read.
const defaultMode fs.FileMode = 0o755

// Replace swaps the binary at target for data.
//
// The write goes to a temporary file in the *same directory* as the target so
// the final rename stays within one filesystem and is therefore atomic. An
// interruption at any point leaves either the complete old binary or the
// complete new one -- never a truncated file, which for the binary you are
// currently running is the difference between an interrupted update and an
// unusable install.
func Replace(target string, data []byte) error {
	if runtime.GOOS == "windows" {
		return replaceWindows(target, data)
	}
	return replaceAtomic(target, data)
}

func replaceAtomic(target string, data []byte) error {
	dir := filepath.Dir(target)
	mode := existingMode(target)

	tmp, err := os.CreateTemp(dir, ".sting-update-*")
	if err != nil {
		return permissionAware(dir, fmt.Errorf("creating temporary file next to %s: %w", target, err))
	}
	tmpName := tmp.Name()

	// From here on, any failure must not leave the temporary file behind.
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing new binary: %w", err)
	}
	// Flush to disk before the rename, so a crash immediately afterwards
	// cannot leave a renamed-but-empty file in place.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("flushing new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("closing new binary: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return fmt.Errorf("setting permissions on new binary: %w", err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		cleanup()
		return permissionAware(target, fmt.Errorf("replacing %s: %w", target, err))
	}
	return nil
}

// replaceWindows implements the rename-aside dance for platforms that refuse to
// overwrite a running executable: move the running file out of the way, put the
// new one at the original path, and clean up the displaced file on a later run.
//
// This path is not reached today. Self-replacement is disabled on Windows until
// release binaries are Authenticode-signed, because asking a user to
// self-install an unsigned replacement is worse than not offering the feature.
// The mechanism is implemented so that lifting the gate is a change of policy
// rather than a new design.
func replaceWindows(target string, data []byte) error {
	displaced := target + displacedSuffix

	// A displaced file from a previous update may still be present; it is
	// only removable once the process holding it has exited.
	_ = os.Remove(displaced)

	if err := os.Rename(target, displaced); err != nil {
		return permissionAware(target, fmt.Errorf("moving the running binary aside: %w", err))
	}

	if err := os.WriteFile(target, data, existingModeOr(displaced, defaultMode)); err != nil {
		// Put the original back rather than leaving the user with no
		// binary at all.
		if restoreErr := os.Rename(displaced, target); restoreErr != nil {
			return fmt.Errorf("writing new binary failed (%w) and the original "+
				"could not be restored from %s: %w", err, displaced, restoreErr)
		}
		return permissionAware(target, fmt.Errorf("writing new binary: %w", err))
	}

	// Best effort: the file is typically still locked by the running
	// process and will be removed by CleanupDisplaced on a later run.
	_ = os.Remove(displaced)
	return nil
}

// CleanupDisplaced removes a binary left behind by a previous replacement. A
// leftover file must never prevent sting from running, so every failure here is
// ignored by design.
func CleanupDisplaced(target string) {
	_ = os.Remove(target + displacedSuffix)
}

// existingMode reads the target's permissions so the replacement keeps them.
func existingMode(target string) fs.FileMode {
	return existingModeOr(target, defaultMode)
}

func existingModeOr(path string, fallback fs.FileMode) fs.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}

// permissionAware maps a filesystem permission failure onto ErrPermission with
// a message that names the path and suggests the correct action.
func permissionAware(path string, err error) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	return fmt.Errorf("%w: cannot write to %s. Re-run with sufficient permissions "+
		"for that location, or reinstall through the channel that owns it. "+
		"sting will not escalate privileges on your behalf: %w", ErrPermission, path, err)
}
