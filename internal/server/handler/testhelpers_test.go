package handler

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	path := filepath.Join("..", "..", "..", "testdata", "kitchen-sink.xmind")
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "handler tests: cannot read %s: %v\n", path, err)
		os.Exit(1)
	}
	if len(b) >= 2 && b[0] == 0x50 && b[1] == 0x4b { // ZIP local file header "PK"
		os.Exit(m.Run())
	}
	if bytes.HasPrefix(b, []byte("version https://git-lfs.github.com/spec/v1")) {
		fmt.Fprintf(os.Stderr, "handler tests: %s is a Git LFS pointer stub, not the real .xmind file.\n"+
			"Run: git lfs pull\n", path)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "handler tests: %s is not a valid .xmind zip (expected PK header); got %d bytes\n", path, len(b))
	os.Exit(1)
}

// copyFixture copies src (.xmind) into a fresh file under t.TempDir() and returns the new path.
func copyFixture(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	dst := filepath.Join(dir, filepath.Base(src))
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create temp copy: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatalf("copy: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return dst
}
