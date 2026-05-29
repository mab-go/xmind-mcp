package xmind

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests exercise replaceViaBackup directly. It is the replace strategy used
// on Windows (where os.Rename cannot overwrite an existing file), but it relies
// only on os.Stat/os.Rename/os.Remove, whose semantics for this pattern are the
// same on Linux, so the logic is fully covered on the (Linux) CI without build
// tags or test-only globals.

// mustWriteFile writes content to path, failing the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readFileString reads path and returns its content as a string.
func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestReplaceViaBackupOverwriteSuccess verifies that an existing dst is replaced
// with the temp file's contents, leaving neither the temp file nor a backup.
func TestReplaceViaBackupOverwriteSuccess(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target")
	tmpPath := filepath.Join(dir, ".xmind-tmp-001")
	mustWriteFile(t, dst, "ORIGINAL")
	mustWriteFile(t, tmpPath, "REPLACEMENT")

	if err := replaceViaBackup(tmpPath, dst); err != nil {
		t.Fatalf("replaceViaBackup: unexpected error: %v", err)
	}

	if got := readFileString(t, dst); got != "REPLACEMENT" {
		t.Fatalf("dst content = %q, want %q", got, "REPLACEMENT")
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temp file should be consumed, stat err = %v", err)
	}
	if _, err := os.Stat(tmpPath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not remain, stat err = %v", err)
	}
}

// TestReplaceViaBackupRestoreOnFailure is the regression test for issue #41: when
// the move-into-place step fails, the original dst must be left intact and
// recoverable, never destroyed. Failure is forced naturally by passing a tmpPath
// that does not exist, so the rename of tmp -> dst fails.
func TestReplaceViaBackupRestoreOnFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target")
	tmpPath := filepath.Join(dir, ".xmind-tmp-missing") // intentionally never created
	mustWriteFile(t, dst, "ORIGINAL")

	err := replaceViaBackup(tmpPath, dst)
	if err == nil {
		t.Fatal("expected replaceViaBackup to fail when the temp file is missing")
	}

	if got := readFileString(t, dst); got != "ORIGINAL" {
		t.Fatalf("original must survive a failed replace: dst content = %q, want %q", got, "ORIGINAL")
	}
	if _, err := os.Stat(tmpPath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not remain after a successful restore, stat err = %v", err)
	}
}

// TestReplaceViaBackupNewFile verifies the no-original case (e.g. CreateNewMap):
// when dst does not yet exist, the temp file is moved into place with no backup.
func TestReplaceViaBackupNewFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target") // intentionally absent
	tmpPath := filepath.Join(dir, ".xmind-tmp-002")
	mustWriteFile(t, tmpPath, "CONTENT")

	if err := replaceViaBackup(tmpPath, dst); err != nil {
		t.Fatalf("replaceViaBackup: unexpected error: %v", err)
	}

	if got := readFileString(t, dst); got != "CONTENT" {
		t.Fatalf("dst content = %q, want %q", got, "CONTENT")
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temp file should be consumed, stat err = %v", err)
	}
	if _, err := os.Stat(tmpPath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not exist for a new file, stat err = %v", err)
	}
}

// TestReplaceViaBackupReportsBackupWhenRestoreFails covers the double-fault path:
// the temp file cannot be moved into place AND the backup cannot be restored. The
// original must survive on disk under the backup name, and the returned error must
// name that backup so the user can recover manually. A failing rename is injected
// via the osRename seam (the only portable way to force the restore to fail).
func TestReplaceViaBackupReportsBackupWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target")
	tmpPath := filepath.Join(dir, ".xmind-tmp-003")
	mustWriteFile(t, dst, "ORIGINAL")
	mustWriteFile(t, tmpPath, "REPLACEMENT")

	// Fail every rename whose destination is dst: the move-into-place (tmp -> dst)
	// fails, and so does the restore (backup -> dst). The initial dst -> backup
	// move (destination != dst) still runs for real, so the backup holds the
	// original on disk.
	orig := osRename
	t.Cleanup(func() { osRename = orig })
	osRename = func(oldpath, newpath string) error {
		if newpath == dst {
			return errors.New("injected rename failure")
		}
		return orig(oldpath, newpath)
	}

	err := replaceViaBackup(tmpPath, dst)
	if err == nil {
		t.Fatal("expected replaceViaBackup to fail when the restore also fails")
	}

	backup := tmpPath + ".bak"
	if !strings.Contains(err.Error(), backup) {
		t.Fatalf("error must name the backup path %q for recovery, got: %v", backup, err)
	}
	if got := readFileString(t, backup); got != "ORIGINAL" {
		t.Fatalf("original must survive at the backup: content = %q, want %q", got, "ORIGINAL")
	}
}
