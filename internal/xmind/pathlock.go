package xmind

import (
	"path/filepath"
	"sync"
)

// pathLockEntry is one path's lock plus the number of goroutines holding or waiting on it.
type pathLockEntry struct {
	mu   sync.Mutex
	refs int
}

// pathLocks serializes writers of the same file within this process. Entries are
// refcounted and removed when the last holder or waiter leaves, so the map holds only
// paths that are in use.
var pathLocks = struct {
	mu sync.Mutex
	m  map[string]*pathLockEntry
}{m: make(map[string]*pathLockEntry)}

// resolveMapPath returns the absolute, symlink-resolved path that a write to path
// replaces. The full path is resolved when it exists; otherwise only its directory is
// (the file may not be created yet), so a path keys the same before and after creation.
// When neither resolves, the cleaned absolute path is returned. Case-insensitive
// filesystems and hard links are not unified.
func resolveMapPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(dir, filepath.Base(abs))
	}
	return abs
}

// lockPath blocks until this process holds the write lock for path, then returns the
// resolved path (the lock key, and the file a write should replace) and a function that
// releases the lock. The lock is in-process only: it does not coordinate with other
// processes or applications writing the same file. It is not reentrant: a goroutine that
// holds it must not call lockPath for the same file again.
func lockPath(path string) (resolved string, unlock func()) {
	resolved = resolveMapPath(path)

	pathLocks.mu.Lock()
	e, ok := pathLocks.m[resolved]
	if !ok {
		e = &pathLockEntry{}
		pathLocks.m[resolved] = e
	}
	e.refs++
	pathLocks.mu.Unlock()

	e.mu.Lock()
	return resolved, func() {
		e.mu.Unlock()
		pathLocks.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(pathLocks.m, resolved)
		}
		pathLocks.mu.Unlock()
	}
}
