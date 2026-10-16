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
//
// A MapWriter holds the file's in-process write lock (see lockPath) for its whole life,
// from OpenMapForUpdate until a successful Commit or Close, so concurrent read-modify-write
// cycles on the same file run one at a time instead of overwriting each other. The lock
// does not coordinate with other processes, and it is not reentrant: while a MapWriter is
// open, the same goroutine must not open, write, or create the same file again.
type MapWriter struct {
	target    string
	file      *os.File
	zr        *zip.Reader
	unlock    func()
	closed    bool
	committed bool
}

// OpenMapForUpdate acquires the file's write lock, opens path once, parses content.json
// into sheets, and keeps the zip open for a subsequent Commit. The returned MapWriter owns
// the open file handle and the lock and must be closed by the caller. Errors mirror
// ReadMap's exactly, since handlers surface them as user-visible tool errors; the lock is
// released before any error return.
func OpenMapForUpdate(path string) (sheets []Sheet, mw *MapWriter, err error) {
	target, unlock := lockPath(path)
	handedOff := false
	defer func() {
		if !handedOff {
			unlock()
		}
	}()

	zr, f, err := openZipReaderAtPath(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if !handedOff {
			_ = f.Close()
		}
	}()

	sheets, err = parseContentFromZip(zr)
	if err != nil {
		return nil, nil, err
	}

	handedOff = true
	return sheets, &MapWriter{target: target, file: f, zr: zr, unlock: unlock}, nil
}

// Commit serializes sheets back into the .xmind file, preserving every non-content.json
// entry via raw copy from the open handle, then atomically swaps the result into place.
// The swap replaces the symlink-resolved target, so a write through a symlink updates the
// real file and leaves the link intact. On success the read handle is closed before the
// swap, so the swap runs with no open handle on the original, and the write lock is
// released after it; if Commit fails, whatever it still holds is left for the caller's
// Close to release. Commit is single-use: a second call, or a call after Close, returns an
// error. Unlike Close, Commit is not nil-safe: OpenMapForUpdate always returns a non-nil
// writer when its error is nil, so a correct caller never reaches here with a nil receiver.
func (mw *MapWriter) Commit(sheets []Sheet) error {
	if mw.committed {
		return fmt.Errorf("map already committed")
	}
	if mw.closed {
		return fmt.Errorf("commit after close")
	}

	if err := writeMapBody(mw.target, sheets, mw.zr, func() error {
		mw.closed = true
		return mw.file.Close()
	}); err != nil {
		return err
	}

	mw.committed = true
	mw.releaseLock()
	return nil
}

// Close releases the open read handle and then the write lock. It is idempotent,
// nil-safe, and safe to call after Commit (which already released both). A nil receiver
// is a no-op so callers can defer Close even on paths where the open returned a tool error
// and left the writer nil. The lock is released even when the handle is already closed,
// as it is after a Commit that failed at the swap.
func (mw *MapWriter) Close() error {
	if mw == nil {
		return nil
	}
	defer mw.releaseLock()
	if mw.closed {
		return nil
	}
	mw.closed = true
	return mw.file.Close()
}

// releaseLock releases the write lock if this writer still holds it.
func (mw *MapWriter) releaseLock() {
	if mw.unlock != nil {
		unlock := mw.unlock
		mw.unlock = nil
		unlock()
	}
}
