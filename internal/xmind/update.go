package xmind

import (
	"archive/zip"
	"fmt"
	"os"
)

// MapWriter holds an open read handle on a .xmind file so a single mutate-then-commit
// cycle reuses one zip-directory scan instead of opening the path twice. OpenMapForUpdate
// returns the parsed sheets alongside the writer; the caller mutates the sheets in memory
// and calls Commit to persist them, raw-copying the still-open non-content.json entries.
// The caller MUST Close the writer (a deferred Close right after a successful open is the
// expected pattern); Close is idempotent and is a no-op after a successful Commit.
type MapWriter struct {
	path      string
	file      *os.File
	zr        *zip.Reader
	closed    bool
	committed bool
}

// OpenMapForUpdate opens path once, parses content.json into sheets, and keeps the zip
// open for a subsequent Commit. The returned MapWriter owns the open file handle and must
// be closed by the caller. Errors mirror ReadMap's exactly, since handlers surface them as
// user-visible tool errors.
func OpenMapForUpdate(path string) (sheets []Sheet, mw *MapWriter, err error) {
	zr, f, err := openZipReaderAtPath(path)
	if err != nil {
		return nil, nil, err
	}

	sheets, err = parseContentFromZip(zr)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}

	return sheets, &MapWriter{path: path, file: f, zr: zr}, nil
}

// Commit serializes sheets back into the .xmind file, preserving every non-content.json
// entry via raw copy from the open handle, then atomically swaps the result into place.
// On success the read handle is closed before the swap, so the swap runs with no open
// handle on the original; if Commit fails before that point the handle is left open for
// the caller's Close to release. Commit is single-use: a second call, or a call after
// Close, returns an error. Unlike Close, Commit is not nil-safe: OpenMapForUpdate always
// returns a non-nil writer when its error is nil, so a correct caller never reaches here
// with a nil receiver.
func (mw *MapWriter) Commit(sheets []Sheet) error {
	if mw.committed {
		return fmt.Errorf("map already committed")
	}
	if mw.closed {
		return fmt.Errorf("commit after close")
	}

	if err := writeMapBody(mw.path, sheets, mw.zr, func() error {
		mw.closed = true
		return mw.file.Close()
	}); err != nil {
		return err
	}

	mw.committed = true
	return nil
}

// Close releases the open read handle. It is idempotent, nil-safe, and safe to call after
// Commit (which already closes the handle). A nil receiver is a no-op so callers can defer
// Close even on paths where the open returned a tool error and left the writer nil.
func (mw *MapWriter) Close() error {
	if mw == nil || mw.closed {
		return nil
	}
	mw.closed = true
	return mw.file.Close()
}
