package xmind

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// ReadMap opens a .xmind zip, reads content.json, and unmarshals it into sheets.
func ReadMap(path string) ([]Sheet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open xmind file: %w", err)
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat xmind file: %w", err)
	}

	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}

	var contentEntry *zip.File
	for _, zf := range zr.File {
		if zf.Name == "content.json" {
			contentEntry = zf
			break
		}
	}
	if contentEntry == nil {
		return nil, fmt.Errorf("xmind zip missing content.json")
	}

	rc, err := contentEntry.Open()
	if err != nil {
		return nil, fmt.Errorf("open content.json: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read content.json: %w", err)
	}

	var sheets []Sheet
	if err := json.Unmarshal(data, &sheets); err != nil {
		return nil, fmt.Errorf("parse content.json: %w", err)
	}

	return sheets, nil
}
