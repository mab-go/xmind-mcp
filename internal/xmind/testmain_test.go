package xmind

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	path := filepath.Join("..", "..", "testdata", "kitchen-sink.xmind")
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xmind tests: cannot read %s: %v\n", path, err)
		os.Exit(1)
	}
	if len(b) >= 2 && b[0] == 0x50 && b[1] == 0x4b { // ZIP local file header "PK"
		os.Exit(m.Run())
	}
	if bytes.HasPrefix(b, []byte("version https://git-lfs.github.com/spec/v1")) {
		fmt.Fprintf(os.Stderr, "xmind tests: %s is a Git LFS pointer stub, not the real .xmind file.\n"+
			"Run: git lfs pull\n", path)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "xmind tests: %s is not a valid .xmind zip (expected PK header); got %d bytes\n", path, len(b))
	os.Exit(1)
}
