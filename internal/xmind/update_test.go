package xmind

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// readContentJSON returns the raw content.json bytes from a .xmind zip.
func readContentJSON(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	for _, zf := range zr.File {
		if zf.Name != "content.json" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatal("content.json not found")
	return nil
}

func TestOpenMapForUpdatePreservesNonContentEntries(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}
	before := zipEntryNames(t, dst)

	sheets, mw, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mw.Close() }()
	if err := mw.Commit(sheets); err != nil {
		t.Fatal(err)
	}

	if after := zipEntryNames(t, dst); !slices.Equal(before, after) {
		t.Fatalf("zip entry names changed:\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestOpenMapForUpdateRawMessageFieldsPreserved(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}

	sheets, mw, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mw.Close() }()
	if len(sheets) == 0 {
		t.Fatal("no sheets")
	}
	themeBefore := append([]byte(nil), sheets[0].Theme...)
	if len(themeBefore) == 0 {
		t.Fatal("expected sheet[0].Theme to be non-empty in kitchen sink")
	}
	if err := mw.Commit(sheets); err != nil {
		t.Fatal(err)
	}

	s2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(themeBefore, s2[0].Theme) {
		t.Fatalf("Theme RawMessage changed (len before %d after %d)", len(themeBefore), len(s2[0].Theme))
	}
}

// TestCommitMatchesWriteMap proves the single-open Commit path produces a byte-identical
// content.json and the same entry set as the read-fresh WriteMap path for the same mutation.
func TestCommitMatchesWriteMap(t *testing.T) {
	dir := t.TempDir()
	viaWrite := filepath.Join(dir, "write.xmind")
	viaCommit := filepath.Join(dir, "commit.xmind")
	if err := copyFile(kitchenSinkPath(t), viaWrite); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(kitchenSinkPath(t), viaCommit); err != nil {
		t.Fatal(err)
	}

	mutate := func(sheets []Sheet) {
		sheets[0].RootTopic.Title = "Mutated Root"
	}

	// WriteMap path.
	ws, err := ReadMap(viaWrite)
	if err != nil {
		t.Fatal(err)
	}
	mutate(ws)
	if err := WriteMap(viaWrite, ws); err != nil {
		t.Fatal(err)
	}

	// Commit path.
	cs, mw, err := OpenMapForUpdate(viaCommit)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mw.Close() }()
	mutate(cs)
	if err := mw.Commit(cs); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(readContentJSON(t, viaWrite), readContentJSON(t, viaCommit)) {
		t.Fatal("content.json differs between WriteMap and Commit paths")
	}
	if !slices.Equal(zipEntryNames(t, viaWrite), zipEntryNames(t, viaCommit)) {
		t.Fatal("zip entry sets differ between WriteMap and Commit paths")
	}
}

func TestCommitAtomicSafetyOnFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}
	sheets, mw, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mw.Close() }()

	before := fileSHA256(t, dst)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := mw.Commit(sheets); err == nil {
		t.Fatal("expected Commit to fail when temp dir is not writable")
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".xmind-tmp-") {
			t.Fatalf("leaked temp file: %s", e.Name())
		}
	}
	if after := fileSHA256(t, dst); after != before {
		t.Fatalf("original file changed after a failed commit: before %s, after %s", before, after)
	}
}

func TestCommitPreservesOriginalOnSwapFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}
	sheets, mw, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mw.Close() }()
	before := fileSHA256(t, dst)

	orig := osRename
	t.Cleanup(func() { osRename = orig })
	osRename = func(string, string) error {
		return errors.New("injected swap failure")
	}

	if err := mw.Commit(sheets); err == nil {
		t.Fatal("expected Commit to fail when the swap rename fails")
	}

	if after := fileSHA256(t, dst); after != before {
		t.Fatalf("original file changed after a failed swap: before %s, after %s", before, after)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".xmind-tmp-") {
			t.Fatalf("leaked temp file: %s", e.Name())
		}
	}
}

func TestMapWriterLifecycleGuards(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}

	sheets, mw, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := mw.Commit(sheets); err != nil {
		t.Fatal(err)
	}
	// A second Commit must be rejected.
	if err := mw.Commit(sheets); err == nil {
		t.Fatal("expected second Commit to fail")
	}
	// Close after a successful Commit is a no-op and must not error.
	if err := mw.Close(); err != nil {
		t.Fatalf("Close after Commit: %v", err)
	}
	// Close is idempotent.
	if err := mw.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	// The committed content must still be readable.
	if _, err := ReadMap(dst); err != nil {
		t.Fatalf("read committed map: %v", err)
	}
}

func TestMapWriterCommitAfterClose(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}
	sheets, mw, err := OpenMapForUpdate(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mw.Commit(sheets); err == nil {
		t.Fatal("expected Commit after Close to fail")
	}
}

// assertOpenErrorParity checks that OpenMapForUpdate fails on path with the same error as
// ReadMap and returns no MapWriter.
func assertOpenErrorParity(t *testing.T, path string) {
	t.Helper()
	_, readErr := ReadMap(path)
	_, mw, openErr := OpenMapForUpdate(path)
	if mw != nil {
		t.Fatal("expected nil MapWriter on error")
	}
	if readErr == nil || openErr == nil || readErr.Error() != openErr.Error() {
		t.Fatalf("error mismatch: read=%v open=%v", readErr, openErr)
	}
}

// writeZipWithoutContentJSON writes a valid zip at path whose only entry is readme.txt.
func writeZipWithoutContentJSON(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenMapForUpdateErrorParity(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		assertOpenErrorParity(t, filepath.Join(t.TempDir(), "nope.xmind"))
	})

	t.Run("not a zip", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "bad.xmind")
		if err := os.WriteFile(bad, []byte("not a zip"), 0o644); err != nil {
			t.Fatal(err)
		}
		assertOpenErrorParity(t, bad)
	})

	t.Run("zip without content.json", func(t *testing.T) {
		noContent := filepath.Join(t.TempDir(), "no-content.xmind")
		writeZipWithoutContentJSON(t, noContent)
		assertOpenErrorParity(t, noContent)
	})
}

// assertOnlyEntry fails unless dir contains exactly one entry, named name (so no temp file
// was left behind there).
func assertOnlyEntry(t *testing.T, dir, name string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != name {
		names := make([]string, 0, len(ents))
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("%s: entries %v, want only %q", dir, names, name)
	}
}

// assertWriteThroughSymlink checks that writing via link updated target (its root title is
// now want) and left link a symlink, with no temp files in either directory.
func assertWriteThroughSymlink(t *testing.T, target, link, want string) {
	t.Helper()
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink (mode %v)", link, fi.Mode())
	}
	sheets, err := ReadMap(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := sheets[0].RootTopic.Title; got != want {
		t.Fatalf("real file root title = %q, want %q", got, want)
	}
	assertOnlyEntry(t, filepath.Dir(target), filepath.Base(target))
	assertOnlyEntry(t, filepath.Dir(link), filepath.Base(link))
}

// newSymlinkedMap copies the fixture into one temp dir and links to it from another,
// returning (target, link). It skips the test where symlinks are unavailable.
func newSymlinkedMap(t *testing.T) (target, link string) {
	t.Helper()
	target = copyKitchenSink(t)
	link = filepath.Join(t.TempDir(), "link.xmind")
	symlinkOrSkip(t, target, link)
	return target, link
}

func TestCommitWritesThroughSymlink(t *testing.T) {
	target, link := newSymlinkedMap(t)
	sheets, mw, err := OpenMapForUpdate(link)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mw.Close() }()
	sheets[0].RootTopic.Title = "Changed via link"
	if err := mw.Commit(sheets); err != nil {
		t.Fatal(err)
	}
	assertWriteThroughSymlink(t, target, link, "Changed via link")
}

func TestWriteMapWritesThroughSymlink(t *testing.T) {
	target, link := newSymlinkedMap(t)
	sheets, err := ReadMap(link)
	if err != nil {
		t.Fatal(err)
	}
	sheets[0].RootTopic.Title = "Changed via link"
	if err := WriteMap(link, sheets); err != nil {
		t.Fatal(err)
	}
	assertWriteThroughSymlink(t, target, link, "Changed via link")
}
