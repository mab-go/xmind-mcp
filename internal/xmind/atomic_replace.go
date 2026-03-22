package xmind

import (
	"fmt"
	"os"
	"runtime"
)

// replaceTempFile moves tmpPath to dst as the final step of an atomic write.
// On Unix, os.Rename replaces an existing dst. On Windows, Rename fails if dst
// exists, so we remove dst first (not atomic with rename; acceptable for local .xmind I/O).
func replaceTempFile(tmpPath, dst string) error {
	if runtime.GOOS == "windows" {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing file: %w", err)
		}
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("rename temp to target: %w", err)
	}
	return nil
}
