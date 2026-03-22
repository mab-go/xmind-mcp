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

// WriteMap replaces content.json in an existing .xmind zip, preserving all other entries
// via raw compressed-byte copy (OpenRaw / CreateRaw).
func WriteMap(path string, sheets []Sheet) error {
	oldFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open xmind file: %w", err)
	}
	defer func() { _ = oldFile.Close() }()

	st, err := oldFile.Stat()
	if err != nil {
		return fmt.Errorf("stat xmind file: %w", err)
	}

	oldZR, err := zip.NewReader(oldFile, st.Size())
	if err != nil {
		return fmt.Errorf("read zip: %w", err)
	}

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

	for _, src := range oldZR.File {
		if src.Name == "content.json" {
			continue
		}
		fh := src.FileHeader
		if len(fh.Extra) > 0 {
			fh.Extra = append([]byte(nil), fh.Extra...)
		}
		dst, err := zw.CreateRaw(&fh)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("zip create raw %q: %w", src.Name, err)
		}
		raw, err := src.OpenRaw()
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("zip open raw %q: %w", src.Name, err)
		}
		if _, err := io.Copy(dst, raw); err != nil {
			if closer, ok := raw.(io.Closer); ok {
				_ = closer.Close()
			}
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("copy zip entry %q: %w", src.Name, err)
		}
		if closer, ok := raw.(io.Closer); ok {
			_ = closer.Close()
		}
	}

	cw, err := zw.Create("content.json")
	if err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return fmt.Errorf("zip create content.json: %w", err)
	}
	if _, err := cw.Write(data); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return fmt.Errorf("write content.json: %w", err)
	}

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

	if err := replaceTempFile(tmpPath, path); err != nil {
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

	writeEntry := func(name string, body []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("zip create %q: %w", name, err)
		}
		if _, err := w.Write(body); err != nil {
			return fmt.Errorf("zip write %q: %w", name, err)
		}
		return nil
	}

	if err := writeEntry("content.json", data); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := writeEntry("metadata.json", []byte(metadataJSON)); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := writeEntry("content.xml", stubContentXML); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := writeEntry("manifest.json", []byte(manifestJSON)); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}

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

	if err := replaceTempFile(tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
