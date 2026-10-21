package xmind

import (
	"archive/zip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// marshalSheetsForContentJSON encodes sheets for content.json the same way XMind writes
// files: literal &, <, > inside JSON strings (not \u0026 / \u003c / \u003e). Top-level
// json.Marshal applies HTML-related escaping and can alter json.Marshaler output, so
// persistence uses marshalJSONNoHTMLEscape (json.Encoder with SetEscapeHTML(false)).
func marshalSheetsForContentJSON(sheets []Sheet) ([]byte, error) {
	return marshalJSONNoHTMLEscape(sheets)
}

//go:embed stub_content.xml
var stubContentXML []byte

const metadataJSON = `{"dataStructureVersion":"2","creator":{"name":"xmind-mcp","version":"0.1.0"},"layoutEngineVersion":"3"}`

const manifestJSON = `{"file-entries":{"content.json":{},"metadata.json":{},"content.xml":{}}}`

// ErrMapReadOnly is returned by WriteMap and MapWriter.Commit when this process can't write the
// map file: it has no write permission bits, or opening it for writing is refused. Replacing it
// would succeed (the swap needs only a writable directory), but a map made read-only is left as
// it is. It matches fs.ErrPermission.
var ErrMapReadOnly = fmt.Errorf("map file is read-only: %w", fs.ErrPermission)

// openZipReaderAtPath opens path as a zip and returns a reader; the caller must close the file.
func openZipReaderAtPath(path string) (*zip.Reader, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open xmind file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
		}
	}()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat xmind file: %w", err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return nil, nil, fmt.Errorf("read zip: %w", err)
	}
	ok = true
	return zr, f, nil
}

// copyZipEntryRaw copies one entry from an existing zip into zw using raw compressed bytes.
// The caller must skip entries (e.g. content.json) that should not be copied verbatim.
func copyZipEntryRaw(zw *zip.Writer, src *zip.File) error {
	fh := src.FileHeader
	if len(fh.Extra) > 0 {
		fh.Extra = append([]byte(nil), fh.Extra...)
	}
	dst, err := zw.CreateRaw(&fh)
	if err != nil {
		return fmt.Errorf("zip create raw %q: %w", src.Name, err)
	}
	raw, err := src.OpenRaw()
	if err != nil {
		return fmt.Errorf("zip open raw %q: %w", src.Name, err)
	}
	if _, err := io.Copy(dst, raw); err != nil {
		if closer, ok := raw.(io.Closer); ok {
			_ = closer.Close()
		}
		return fmt.Errorf("copy zip entry %q: %w", src.Name, err)
	}
	if closer, ok := raw.(io.Closer); ok {
		_ = closer.Close()
	}
	return nil
}

func copyZipEntriesExceptContentJSON(zw *zip.Writer, zr *zip.Reader) error {
	for _, src := range zr.File {
		if src.Name == "content.json" {
			continue
		}
		if err := copyZipEntryRaw(zw, src); err != nil {
			return err
		}
	}
	return nil
}

func writeZipBytes(zw *zip.Writer, name string, body []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("zip create %q: %w", name, err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("zip write %q: %w", name, err)
	}
	return nil
}

func writeNewMapStandardEntries(zw *zip.Writer, contentJSON []byte) error {
	entries := []struct {
		name string
		body []byte
	}{
		{"content.json", contentJSON},
		{"metadata.json", []byte(metadataJSON)},
		{"content.xml", stubContentXML},
		{"manifest.json", []byte(manifestJSON)},
	}
	for _, e := range entries {
		if err := writeZipBytes(zw, e.name, e.body); err != nil {
			return err
		}
	}
	return nil
}

func abortZipWriterAndFile(zw *zip.Writer, tmp *os.File) {
	_ = zw.Close()
	_ = tmp.Close()
}

// finishZipTempFile completes the temp file, gives it permission bits perm, and swaps it
// into dst. os.CreateTemp always creates files with mode 0600, so without the chmod every
// write would leave the map readable only by its owner.
func finishZipTempFile(zw *zip.Writer, tmp *os.File, tmpPath, dst string, perm fs.FileMode) error {
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("close zip writer: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set temp file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := replaceTempFile(tmpPath, dst); err != nil {
		return err
	}
	return nil
}

// WriteMap replaces content.json in an existing .xmind zip, preserving all other entries
// via raw compressed-byte copy (OpenRaw / CreateRaw). It writes through a symlink to the
// real file.
//
// WriteMap holds the file's in-process write lock (see lockPath) while it re-reads the
// entries and swaps the file, so it never interleaves with a MapWriter's Commit. The lock
// does not cover the caller's own earlier read of sheets: a read-modify-write cycle that
// must not lose concurrent updates belongs in OpenMapForUpdate and Commit.
func WriteMap(path string, sheets []Sheet) error {
	target, unlock := lockPath(path)
	defer unlock()

	oldZR, oldFile, err := openZipReaderAtPath(path)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = oldFile.Close()
		}
	}()

	return writeMapBody(target, sheets, oldZR, func() error {
		closed = true
		return oldFile.Close()
	})
}

// writeMapBody serializes sheets into a new content.json and writes a fresh .xmind zip
// at a temp path, raw-copying every non-content.json entry from zr, then atomically
// swaps it into dst. dst must already be symlink-resolved (the path lockPath returned):
// the temp file is created in dst's directory and renamed onto dst, so resolving first
// keeps the rename on one filesystem and updates the real file instead of replacing a
// symlink with a regular file. releaseSource is invoked once the entry copy is complete
// (and before the atomic swap) so the caller can close the source handle that backs zr;
// this lets the swap run with no open handle on the original. On any failure the temp
// file is removed and the original is left untouched. The new file keeps the original's
// permission bits. A dst with no write permission bits is refused with ErrMapReadOnly, and
// sheets holding a value that could not be decoded in a field the tools edit are refused with
// ErrUndecodableField, both before anything is written.
func writeMapBody(dst string, sheets []Sheet, zr *zip.Reader, releaseSource func() error) error {
	data, err := contentJSONToSave(sheets)
	if err != nil {
		return err
	}

	perm, err := writableTargetPerm(dst)
	if err != nil {
		return err
	}

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".xmind-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	swapped := false
	defer func() {
		if !swapped {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmp)

	if err := copyZipEntriesExceptContentJSON(zw, zr); err != nil {
		abortZipWriterAndFile(zw, tmp)
		return err
	}

	if err := writeZipBytes(zw, "content.json", data); err != nil {
		abortZipWriterAndFile(zw, tmp)
		return err
	}

	if releaseSource != nil {
		if err := releaseSource(); err != nil {
			abortZipWriterAndFile(zw, tmp)
			return fmt.Errorf("close source handle: %w", err)
		}
	}

	if err := finishZipTempFile(zw, tmp, tmpPath, dst, perm); err != nil {
		return err
	}
	swapped = true
	return nil
}

// contentJSONToSave returns sheets as content.json, or, unwrapped, checkSavable's error when they
// hold a value that could not be decoded in a field the tools edit.
func contentJSONToSave(sheets []Sheet) ([]byte, error) {
	if err := checkSavable(sheets); err != nil {
		return nil, err
	}
	data, err := marshalSheetsForContentJSON(sheets)
	if err != nil {
		return nil, fmt.Errorf("marshal content.json: %w", err)
	}
	return data, nil
}

// writableTargetPerm returns the permission bits of the map file a write will replace, which
// the new file keeps, or ErrMapReadOnly when this process can't write the file: it has no write
// bits (refused even for root, which could write it anyway), or opening it for writing is
// refused (an owner without the write bit on a group-writable file, an ACL, an immutable file).
func writableTargetPerm(dst string) (fs.FileMode, error) {
	orig, err := os.Stat(dst)
	if err != nil {
		return 0, fmt.Errorf("stat target: %w", err)
	}
	perm := orig.Mode().Perm()
	if perm&0o222 == 0 {
		return 0, ErrMapReadOnly
	}
	// Opening for writing without O_TRUNC changes nothing. Any failure other than a permission
	// error is left for the swap to report.
	f, err := os.OpenFile(dst, os.O_WRONLY, 0)
	if errors.Is(err, fs.ErrPermission) {
		return 0, ErrMapReadOnly
	}
	if err == nil {
		_ = f.Close()
	}
	return perm, nil
}

// CreateNewMap writes a new .xmind zip with content.json, metadata.json, manifest.json,
// and the standard XMind compatibility content.xml stub.
//
// It never overwrites. It first claims path with O_CREATE|O_EXCL, which is atomic, so a
// file that already exists (or appears concurrently) is left alone and the returned error
// wraps fs.ErrExist. The finished temp file is then renamed over that empty placeholder;
// on any failure the placeholder is removed. The placeholder is created with mode 0666
// filtered by the process umask (as os.Create does), and the finished map gets the same
// permission bits.
//
// It holds the file's in-process write lock (see lockPath) from before the claim until
// the rename or cleanup, so a concurrent OpenMapForUpdate never opens the empty
// placeholder; it waits and then sees the finished map. Readers take no lock and can
// still see the placeholder.
func CreateNewMap(path string, sheets []Sheet) error {
	_, unlock := lockPath(path)
	defer unlock()

	data, err := marshalSheetsForContentJSON(sheets)
	if err != nil {
		return fmt.Errorf("marshal content.json: %w", err)
	}

	placeholder, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return fmt.Errorf("create map file: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.Remove(path)
		}
	}()
	perm, err := closePlaceholder(placeholder)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".xmind-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmp)

	if err := writeNewMapStandardEntries(zw, data); err != nil {
		abortZipWriterAndFile(zw, tmp)
		return err
	}

	if err := finishZipTempFile(zw, tmp, tmpPath, path, perm); err != nil {
		return err
	}
	committed = true
	published = true
	return nil
}

// closePlaceholder closes the empty file CreateNewMap claimed and returns its permission
// bits, which the umask has already filtered, for the finished map to reuse.
func closePlaceholder(placeholder *os.File) (fs.FileMode, error) {
	st, err := placeholder.Stat()
	if err != nil {
		_ = placeholder.Close()
		return 0, fmt.Errorf("stat placeholder: %w", err)
	}
	if err := placeholder.Close(); err != nil {
		return 0, fmt.Errorf("close placeholder: %w", err)
	}
	return st.Mode().Perm(), nil
}
