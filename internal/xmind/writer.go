package xmind

import (
	"archive/zip"
	_ "embed"
	"fmt"
	"io"
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

// openZipReaderAtPath opens path as a zip and returns a reader; the caller must close the file.
func openZipReaderAtPath(path string) (*zip.Reader, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open xmind file: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("stat xmind file: %w", err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("read zip: %w", err)
	}
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

func finishZipTempFile(zw *zip.Writer, tmp *os.File, tmpPath, dst string) error {
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("close zip writer: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
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
// via raw compressed-byte copy (OpenRaw / CreateRaw).
func WriteMap(path string, sheets []Sheet) error {
	oldZR, oldFile, err := openZipReaderAtPath(path)
	if err != nil {
		return err
	}
	defer func() { _ = oldFile.Close() }()

	data, err := marshalSheetsForContentJSON(sheets)
	if err != nil {
		return fmt.Errorf("marshal content.json: %w", err)
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
			_ = os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmp)

	if err := copyZipEntriesExceptContentJSON(zw, oldZR); err != nil {
		abortZipWriterAndFile(zw, tmp)
		return err
	}

	if err := writeZipBytes(zw, "content.json", data); err != nil {
		abortZipWriterAndFile(zw, tmp)
		return err
	}

	if err := finishZipTempFile(zw, tmp, tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

// CreateNewMap writes a new .xmind zip with content.json, metadata.json, manifest.json,
// and the standard XMind compatibility content.xml stub.
func CreateNewMap(path string, sheets []Sheet) error {
	data, err := marshalSheetsForContentJSON(sheets)
	if err != nil {
		return fmt.Errorf("marshal content.json: %w", err)
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
			_ = os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmp)

	if err := writeNewMapStandardEntries(zw, data); err != nil {
		abortZipWriterAndFile(zw, tmp)
		return err
	}

	if err := finishZipTempFile(zw, tmp, tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
