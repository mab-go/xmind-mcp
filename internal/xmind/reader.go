package xmind

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// maxContentJSONSize and maxContentJSONElements limit the content.json that ReadMap and
// OpenMapForUpdate read and that WriteMap, MapWriter.Commit and CreateNewMap write, so the server
// never writes a map it would refuse to read. The byte limit stops a zip bomb before anything is
// decompressed. The element limit (see countJSONElements) bounds decoding, which costs up to
// about 800 bytes per element (measured on an array of empty sheets, [{},{},…]) however few
// bytes the file has: 1 MiB of empty objects decodes into over 500 MiB. Real maps use about 25
// bytes of content.json per element (the kitchen-sink fixture: 150 KB, 5,963 elements). They
// are variables so tests can lower them.
var (
	maxContentJSONSize     uint64 = 64 << 20
	maxContentJSONElements        = 500_000
)

// ErrMapTooLarge is returned when a map's content.json is over maxContentJSONSize bytes or
// maxContentJSONElements elements: by ReadMap and OpenMapForUpdate, which refuse to read it, and
// by WriteMap, MapWriter.Commit and CreateNewMap, which refuse to write it and leave the file
// as it was.
var ErrMapTooLarge = errors.New("map is too large")

// ReadMap opens a .xmind zip, reads content.json, and unmarshals it into sheets.
func ReadMap(path string) ([]Sheet, error) {
	zr, f, err := openZipReaderAtPath(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return parseContentFromZip(zr)
}

// parseContentFromZip locates content.json in zr, reads it, and unmarshals it into sheets.
func parseContentFromZip(zr *zip.Reader) ([]Sheet, error) {
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

	data, err := readContentEntry(contentEntry)
	if err != nil {
		return nil, err
	}

	var sheets []Sheet
	if err := json.Unmarshal(data, &sheets); err != nil {
		return nil, fmt.Errorf("parse content.json: %w", err)
	}

	return sheets, nil
}

// readContentEntry reads the content.json entry. It refuses the entry before decompressing
// anything when its header declares more than maxContentJSONSize bytes uncompressed, and before
// it can be decoded when it has more than maxContentJSONElements elements. The declared size
// bounds the read: archive/zip fails with zip.ErrFormat once an entry produces more bytes than
// its header declares. Reading to EOF is what makes archive/zip check the length and the CRC.
func readContentEntry(entry *zip.File) ([]byte, error) {
	if entry.UncompressedSize64 > maxContentJSONSize {
		return nil, fmt.Errorf("%w: content.json is %d bytes uncompressed, over the %d-byte limit",
			ErrMapTooLarge, entry.UncompressedSize64, maxContentJSONSize)
	}

	rc, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open content.json: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read content.json: %w", err)
	}
	if err := checkContentJSONLimits(data); err != nil {
		return nil, err
	}
	return data, nil
}

// checkContentJSONLimits returns an error wrapping ErrMapTooLarge when data, a content.json, is
// over maxContentJSONSize bytes or maxContentJSONElements elements. It checks the bytes first,
// so an oversized input is never scanned.
func checkContentJSONLimits(data []byte) error {
	if uint64(len(data)) > maxContentJSONSize {
		return fmt.Errorf("%w: content.json is %d bytes, over the %d-byte limit",
			ErrMapTooLarge, len(data), maxContentJSONSize)
	}
	if n := countJSONElements(data); n > maxContentJSONElements {
		return fmt.Errorf("%w: content.json has %d JSON elements, over the %d-element limit",
			ErrMapTooLarge, n, maxContentJSONElements)
	}
	return nil
}

// countJSONElements counts the {, [ and , bytes in data outside strings. For valid JSON that is
// the number of objects and arrays plus, for each non-empty one, its items (an object's members)
// less one: about one per value decoding creates. It does not validate data; invalid input gets
// some count, and decoding rejects it afterwards.
func countJSONElements(data []byte) int {
	n := 0
	inString, escaped := false, false
	for _, b := range data {
		switch {
		case escaped:
			escaped = false
		case inString:
			switch b {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
		case b == '"':
			inString = true
		case b == '{', b == '[', b == ',':
			n++
		}
	}
	return n
}
