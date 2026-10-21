//go:build !windows

package handler

import "syscall"

// errReadOnlyFS is the error for a write to a read-only filesystem.
const errReadOnlyFS = syscall.EROFS

// osPathAccessErrors are the errors, besides those isPathAccessError checks on every platform,
// that mean a path can't be used: a name too long, a symlink loop, or a read-only filesystem.
var osPathAccessErrors = []error{syscall.ENAMETOOLONG, syscall.ELOOP, errReadOnlyFS}
