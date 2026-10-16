package xmind

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// lockRefs reports how many goroutines hold or wait on the write lock for key.
func lockRefs(key string) int {
	pathLocks.mu.Lock()
	defer pathLocks.mu.Unlock()
	if e, ok := pathLocks.m[key]; ok {
		return e.refs
	}
	return 0
}

// assertLockRefs fails unless lockRefs(key) is want; when describes the moment checked.
func assertLockRefs(t *testing.T, key string, want int, when string) {
	t.Helper()
	if got := lockRefs(key); got != want {
		t.Fatalf("lock refs %s = %d, want %d", when, got, want)
	}
}

// symlinkOrSkip creates newname -> oldname, skipping the test where symlinks are unavailable
// (e.g. Windows without the privilege).
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// evalSymlinks resolves path, failing the test on error. Expected keys are built from it
// rather than by hand so platform-level links (macOS /var -> /private/var) cannot break
// the comparison.
func evalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// copyKitchenSink copies the kitchen-sink fixture into a temp dir and returns its path.
func copyKitchenSink(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

// assertResolvesTo fails unless resolveMapPath(path) is want.
func assertResolvesTo(t *testing.T, path, want string) {
	t.Helper()
	if got := resolveMapPath(path); got != want {
		t.Fatalf("resolveMapPath(%q) = %q, want %q", path, got, want)
	}
}

func TestResolveMapPath(t *testing.T) {
	realDir := t.TempDir()
	realFile := filepath.Join(realDir, "m.xmind")
	if err := os.WriteFile(realFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantFile := evalSymlinks(t, realFile)
	wantNew := filepath.Join(evalSymlinks(t, realDir), "new.xmind")

	t.Run("existing file", func(t *testing.T) {
		assertResolvesTo(t, realFile, wantFile)
	})

	t.Run("relative path", func(t *testing.T) {
		t.Chdir(realDir)
		assertResolvesTo(t, "m.xmind", wantFile)
	})

	t.Run("nonexistent file keys like its later location", func(t *testing.T) {
		assertResolvesTo(t, filepath.Join(realDir, "new.xmind"), wantNew)
	})

	t.Run("symlinked directory", func(t *testing.T) {
		linkDir := filepath.Join(t.TempDir(), "maps")
		symlinkOrSkip(t, realDir, linkDir)
		assertResolvesTo(t, filepath.Join(linkDir, "m.xmind"), wantFile)
		assertResolvesTo(t, filepath.Join(linkDir, "new.xmind"), wantNew)
	})

	t.Run("file symlink keys as its target", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link.xmind")
		symlinkOrSkip(t, realFile, link)
		assertResolvesTo(t, link, wantFile)
	})
}

func TestLockReleasedByClose(t *testing.T) {
	_, mw, err := OpenMapForUpdate(copyKitchenSink(t))
	if err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, mw.target, 1, "while open")
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, mw.target, 0, "after Close")
}

func TestLockReleasedByCommit(t *testing.T) {
	sheets, mw, err := OpenMapForUpdate(copyKitchenSink(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := mw.Commit(sheets); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, mw.target, 0, "after Commit")
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, mw.target, 0, "after Commit and Close")
}

func TestLockReleasedOnFailedOpen(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.xmind")
	bad := filepath.Join(t.TempDir(), "bad.xmind")
	if err := os.WriteFile(bad, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{missing, bad} {
		if _, _, err := OpenMapForUpdate(path); err == nil {
			t.Fatalf("expected OpenMapForUpdate(%q) to fail", path)
		}
		assertLockRefs(t, resolveMapPath(path), 0, "after failed open of "+filepath.Base(path))
	}
}

func TestLockHeldAfterFailedCommitUntilClose(t *testing.T) {
	sheets, mw, err := OpenMapForUpdate(copyKitchenSink(t))
	if err != nil {
		t.Fatal(err)
	}
	orig := osRename
	t.Cleanup(func() { osRename = orig })
	osRename = func(string, string) error {
		return errors.New("injected swap failure")
	}
	if err := mw.Commit(sheets); err == nil {
		t.Fatal("expected Commit to fail when the swap rename fails")
	}
	assertLockRefs(t, mw.target, 1, "after failed Commit")
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, mw.target, 0, "after Close")
}

func TestLockReleasedByWriteMapAndCreateNewMap(t *testing.T) {
	dst := copyKitchenSink(t)
	sheets, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteMap(dst, sheets); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, resolveMapPath(dst), 0, "after WriteMap")

	created := filepath.Join(t.TempDir(), "new.xmind")
	if err := CreateNewMap(created, sheets[:1]); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, resolveMapPath(created), 0, "after CreateNewMap")
}

// openResult is one OpenMapForUpdate outcome, collected off the test goroutine.
type openResult struct {
	mw  *MapWriter
	err error
}

// waitForWaiter polls until a second goroutine is queued on key's lock (refs reaches 2).
func waitForWaiter(t *testing.T, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for lockRefs(key) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("second OpenMapForUpdate never queued on the lock")
		}
		runtime.Gosched()
	}
}

func TestOpenMapForUpdateWaitsForHolder(t *testing.T) {
	dst := copyKitchenSink(t)
	_, first, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()

	done := make(chan openResult, 1)
	go func() {
		_, mw, err := OpenMapForUpdate(dst)
		done <- openResult{mw, err}
	}()

	waitForWaiter(t, first.target)
	select {
	case <-done:
		t.Fatal("second OpenMapForUpdate returned while the first writer held the lock")
	default:
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	var second openResult
	select {
	case second = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("second OpenMapForUpdate did not proceed after the first writer closed")
	}
	if second.err != nil {
		t.Fatal(second.err)
	}
	if err := second.mw.Close(); err != nil {
		t.Fatal(err)
	}
	assertLockRefs(t, first.target, 0, "after both writers closed")
}
