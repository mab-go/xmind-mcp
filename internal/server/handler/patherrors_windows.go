package handler

import "syscall"

// Win32 error numbers. The syscall package names only a few, and on Windows its ENAMETOOLONG,
// ELOOP and EROFS are invented values (1<<29 and up) that the OS never returns.
const (
	errorWriteProtect        syscall.Errno = 19   // ERROR_WRITE_PROTECT
	errorInvalidName         syscall.Errno = 123  // ERROR_INVALID_NAME: a character such as * or ?
	errorBadPathname         syscall.Errno = 161  // ERROR_BAD_PATHNAME
	errorFilenameExcedRange  syscall.Errno = 206  // ERROR_FILENAME_EXCED_RANGE: a name too long
	errorCantResolveFilename syscall.Errno = 1921 // ERROR_CANT_RESOLVE_FILENAME: a symlink loop
)

// errReadOnlyFS is the error for a write to a write-protected volume.
const errReadOnlyFS = errorWriteProtect

// osPathAccessErrors are the errors, besides those isPathAccessError checks on every platform,
// that mean a path can't be used: an invalid or too-long name, a symlink loop, or a
// write-protected volume.
var osPathAccessErrors = []error{
	errorInvalidName, errorBadPathname, errorFilenameExcedRange, errorCantResolveFilename, errReadOnlyFS,
}
