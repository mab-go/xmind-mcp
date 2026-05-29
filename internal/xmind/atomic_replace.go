package xmind

import (
	"fmt"
	"os"
	"runtime"
)

// osRename is a seam over os.Rename so tests can force the rename failures that
// drive replaceViaBackup's restore path. Production always uses os.Rename.
var osRename = os.Rename

// replaceTempFile moves tmpPath to dst as the final step of an atomic write.
// On Unix, os.Rename atomically replaces an existing dst. On Windows, Rename
// fails if dst exists, so the original must be moved aside first; replaceViaBackup
// does so via a backup so a mid-write failure still leaves the original recoverable.
func replaceTempFile(tmpPath, dst string) error {
	if runtime.GOOS == "windows" {
		return replaceViaBackup(tmpPath, dst)
	}
	if err := osRename(tmpPath, dst); err != nil {
		return fmt.Errorf("rename temp to target: %w", err)
	}
	return nil
}

// replaceViaBackup moves tmpPath to dst on platforms where os.Rename cannot
// overwrite an existing file (Windows). The original dst is moved aside to a
// backup before the temp file is renamed into place, so a failure at any step
// leaves the original intact and recoverable; on a failed move-into-place the
// backup is restored. It is OS-agnostic so it can be tested directly on any
// platform.
func replaceViaBackup(tmpPath, dst string) error {
	// The backup name is derived from tmpPath (which os.CreateTemp already made
	// unique), so a collision is effectively impossible; and if a ".bak" sibling
	// somehow existed, the Windows os.Rename below would fail safely, leaving the
	// original in place.
	backup := tmpPath + ".bak"

	haveBackup := false
	if _, err := os.Stat(dst); err == nil {
		if err := osRename(dst, backup); err != nil {
			return fmt.Errorf("back up existing file: %w", err)
		}
		haveBackup = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target: %w", err)
	}

	if err := osRename(tmpPath, dst); err != nil {
		if haveBackup {
			if rerr := osRename(backup, dst); rerr != nil {
				return fmt.Errorf("rename temp to target: %w; original preserved at %q (restore failed: %v)", err, backup, rerr)
			}
			return fmt.Errorf("rename temp to target: %w (original restored)", err)
		}
		return fmt.Errorf("rename temp to target: %w", err)
	}

	// The write succeeded; removing the backup is best effort and must not turn
	// a successful write into a reported failure.
	if haveBackup {
		_ = os.Remove(backup)
	}
	return nil
}
