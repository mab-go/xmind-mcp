package xmind

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func kitchenSinkPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "kitchen-sink.xmind")
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestReadMapErrorFileNotFound(t *testing.T) {
	_, err := ReadMap(filepath.Join(t.TempDir(), "nonexistent.xmind"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist in chain, got: %v", err)
	}
}

func TestReadMapErrorNotAZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notzip.xmind")
	if err := os.WriteFile(path, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadMap(path)
	if err == nil {
		t.Fatal("expected error for non-zip file")
	}
	if !strings.Contains(err.Error(), "read zip") {
		t.Fatalf("expected read zip error, got: %v", err)
	}
}

func TestReadMapErrorNoContentJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xmind")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = ReadMap(path)
	if err == nil {
		t.Fatal("expected error when content.json is missing")
	}
	if !strings.Contains(err.Error(), "missing content.json") {
		t.Fatalf("expected missing content.json error, got: %v", err)
	}
}

// writeContentJSONMap writes a map whose content.json is contentJSON, plus a metadata.json
// entry, to a new temp directory and returns its path.
func writeContentJSONMap(t *testing.T, contentJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.xmind")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for _, e := range []struct{ name, body string }{{"content.json", contentJSON}, {"metadata.json", "{}"}} {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadMapErrorMalformedJSON(t *testing.T) {
	path := writeContentJSONMap(t, "{not valid json")
	_, err := ReadMap(path)
	if err == nil {
		t.Fatal("expected error for malformed content.json")
	}
	if !strings.Contains(err.Error(), "parse content.json") {
		t.Fatalf("expected parse content.json error, got: %v", err)
	}
}

// writeDeclaredSizeMap writes a map whose content.json entry stores data uncompressed under a
// header that declares declared bytes, to a new temp directory, and returns its path. CreateRaw
// writes the header's sizes as given, so the file stays tiny whatever size it declares.
func writeDeclaredSizeMap(t *testing.T, data []byte, declared uint64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.xmind")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name:               "content.json",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(len(data)),
		UncompressedSize64: declared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// setContentJSONLimits lowers (or raises) the content.json limits for one test and restores them
// when it ends.
func setContentJSONLimits(t *testing.T, size uint64, elements int) {
	t.Helper()
	oldSize, oldElements := maxContentJSONSize, maxContentJSONElements
	t.Cleanup(func() { maxContentJSONSize, maxContentJSONElements = oldSize, oldElements })
	maxContentJSONSize, maxContentJSONElements = size, elements
}

// assertReadsRefuseMapTooLarge checks ReadMap and OpenMapForUpdate both refuse the map at path
// with ErrMapTooLarge and a message containing want, and that OpenMapForUpdate returns no
// MapWriter with its error.
func assertReadsRefuseMapTooLarge(t *testing.T, path, want string) {
	t.Helper()
	if _, err := ReadMap(path); !errors.Is(err, ErrMapTooLarge) || !strings.Contains(err.Error(), want) {
		t.Errorf("ReadMap error = %v, want ErrMapTooLarge containing %q", err, want)
	}
	_, mw, err := OpenMapForUpdate(path)
	if !errors.Is(err, ErrMapTooLarge) || !strings.Contains(err.Error(), want) {
		t.Errorf("OpenMapForUpdate error = %v, want ErrMapTooLarge containing %q", err, want)
	}
	if mw != nil {
		_ = mw.Close()
		t.Error("OpenMapForUpdate returned a MapWriter with its error")
	}
}

// TestReadMapErrorContentJSONTooLarge checks ReadMap and OpenMapForUpdate refuse a content.json
// whose header declares more than maxContentJSONSize without reading it: each file holds two
// bytes, whatever its header declares.
func TestReadMapErrorContentJSONTooLarge(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared uint64
	}{
		{"one byte over the limit", maxContentJSONSize + 1},
		{"a terabyte (zip64 header)", 1 << 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeDeclaredSizeMap(t, []byte("[]"), tc.declared)
			assertReadsRefuseMapTooLarge(t, path, "bytes uncompressed")
		})
	}
}

// TestReadMapErrorTooManyElements checks ReadMap and OpenMapForUpdate refuse, at the real
// limits, a content.json with more than maxContentJSONElements elements: a valid sheet whose
// unknown key holds 500,001 numbers, 500,009 elements in all. Decoding keeps that key as about
// 1 MB of raw bytes, so the test stays cheap even if the check stops working; elements that
// decode into structs, such as empty sheets, would take over a GB here.
func TestReadMapErrorTooManyElements(t *testing.T) {
	path := writeContentJSONMap(t, `[{"id":"s1","class":"sheet","title":"S","rootTopic":{"id":"r","title":"R"},"x":[0`+
		strings.Repeat(",0", 500_000)+`]}]`)
	assertReadsRefuseMapTooLarge(t, path, "JSON elements")
}

// TestReadMapErrorContentJSONLongerThanHeader checks archive/zip fails an entry that produces
// more bytes than its header declares. readContentEntry relies on this: it trusts the declared
// size to bound the read.
func TestReadMapErrorContentJSONLongerThanHeader(t *testing.T) {
	path := writeDeclaredSizeMap(t, []byte("[]"), 1)
	if _, err := ReadMap(path); !errors.Is(err, zip.ErrFormat) {
		t.Fatalf("ReadMap error = %v, want zip.ErrFormat", err)
	}
}

// TestReadContentEntryLimits checks both limits are inclusive: content.json at exactly a limit is
// read, and one over it is refused with ErrMapTooLarge.
func TestReadContentEntryLimits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		content  string
		size     uint64
		elements int
		want     string // "" when the read succeeds
	}{
		{"bytes at the limit", "[]", 2, 1, ""},
		{"bytes over the limit", "[]", 1, 1, "bytes uncompressed"},
		{"elements at the limit", "[{}]", 4, 2, ""},
		{"elements over the limit", "[{}]", 4, 1, "JSON elements"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zr, err := zip.OpenReader(writeContentJSONMap(t, tc.content))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = zr.Close() }()
			var entry *zip.File
			for _, zf := range zr.File {
				if zf.Name == "content.json" {
					entry = zf
				}
			}
			if entry == nil {
				t.Fatal("no content.json entry")
			}

			setContentJSONLimits(t, tc.size, tc.elements)
			data, err := readContentEntry(entry)
			if tc.want == "" {
				if err != nil || string(data) != tc.content {
					t.Errorf("readContentEntry = %q, %v; want %q, nil", data, err, tc.content)
				}
				return
			}
			if !errors.Is(err, ErrMapTooLarge) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("readContentEntry error = %v, want ErrMapTooLarge containing %q", err, tc.want)
			}
		})
	}
}

// countJSONElementsCases are inputs and the counts countJSONElements must give; they also seed
// FuzzCountJSONElements.
var countJSONElementsCases = []struct {
	json string
	want int
}{
	{`[]`, 1},
	{`{}`, 1},
	{`[{},{}]`, 4},
	{`{"a":1,"b":[1,2]}`, 4},
	{`"x"`, 0},
	{`{"s":"{[,\"]}"}`, 1}, // braces, commas and an escaped quote inside a string don't count
	{`[[],[[]]]`, 5},
	{` [ 1 , "a,b" , null ] `, 3},
}

func TestCountJSONElements(t *testing.T) {
	for _, tc := range countJSONElementsCases {
		if got := countJSONElements([]byte(tc.json)); got != tc.want {
			t.Errorf("countJSONElements(%s) = %d, want %d", tc.json, got, tc.want)
		}
	}
}

// referenceJSONElementCount counts what countJSONElements must give for valid JSON, from the
// decoder's tokens instead of bytes: each object and array, plus, for each non-empty one, its
// items less one. Token returns an object's keys and values as separate tokens, and a nested
// object or array is one token of its parent.
func referenceJSONElementCount(t *testing.T, data []byte) int {
	t.Helper()
	type container struct {
		object bool
		tokens int
	}
	var stack []container
	n := 0
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // json.Valid accepts numbers a float64 can't hold, such as 1E700
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return n
		}
		if err != nil {
			t.Fatalf("Token on valid JSON %q: %v", data, err)
		}
		delim, isDelim := tok.(json.Delim)
		if len(stack) > 0 && (!isDelim || delim == '{' || delim == '[') {
			stack[len(stack)-1].tokens++
		}
		switch delim {
		case '{', '[':
			n++
			stack = append(stack, container{object: delim == '{'})
		case '}', ']':
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			items := c.tokens
			if c.object {
				items /= 2
			}
			n += max(items-1, 0)
		}
	}
}

// FuzzCountJSONElements checks countJSONElements on any input: it never counts more elements
// than there are bytes, and on valid JSON it matches the count from the decoder's tokens.
func FuzzCountJSONElements(f *testing.F) {
	for _, tc := range countJSONElementsCases {
		f.Add([]byte(tc.json))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got := countJSONElements(data)
		if got > len(data) {
			t.Fatalf("countJSONElements(%q) = %d, more than its %d bytes", data, got, len(data))
		}
		if !json.Valid(data) {
			return
		}
		if want := referenceJSONElementCount(t, data); got != want {
			t.Errorf("countJSONElements(%q) = %d, want %d", data, got, want)
		}
	})
}

func findTopicByID(root *Topic, id string) *Topic {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	if root.Children == nil {
		return nil
	}
	for i := range root.Children.Attached {
		if t := findTopicByID(&root.Children.Attached[i], id); t != nil {
			return t
		}
	}
	for i := range root.Children.Detached {
		if t := findTopicByID(&root.Children.Detached[i], id); t != nil {
			return t
		}
	}
	for i := range root.Children.Summary {
		if t := findTopicByID(&root.Children.Summary[i], id); t != nil {
			return t
		}
	}
	return nil
}

func findSheetByTitle(sheets []Sheet, title string) *Sheet {
	for i := range sheets {
		if sheets[i].Title == title {
			return &sheets[i]
		}
	}
	return nil
}

func assertKitchenSinkSheet10Labels(t *testing.T, root *Topic) {
	t.Helper()
	labelTopic := findTopicByID(root, "9b558db8-cdf3-47ab-a6b0-ee688aa3ad58")
	if labelTopic == nil || len(labelTopic.Labels) != 1 || labelTopic.Labels[0] != "foo" {
		t.Fatalf("labels: %+v", labelTopic)
	}
}

func assertKitchenSinkSheet10Notes(t *testing.T, root *Topic) {
	t.Helper()
	noteTopic := findTopicByID(root, "83785e34-fcdb-4049-8c42-15052c20d8d6")
	if noteTopic == nil || noteTopic.Notes == nil || noteTopic.Notes.Plain == nil ||
		!strings.HasPrefix(noteTopic.Notes.Plain.Content, "This is a simple, plain text note") {
		t.Fatalf("notes: %+v", noteTopic)
	}
}

func assertKitchenSinkSheet10Href(t *testing.T, root *Topic) {
	t.Helper()
	hrefTopic := findTopicByID(root, "e0d8096f-cc1a-4c33-bcc5-db9006481f85")
	if hrefTopic == nil || hrefTopic.Href != "https://www.google.com" {
		t.Fatalf("href: %+v", hrefTopic)
	}
}

func assertKitchenSinkSheet10AudioExtension(t *testing.T, root *Topic) {
	t.Helper()
	audioTopic := findTopicByID(root, "9a8153a4-0ac2-4a5c-bc46-db9a0f38e47c")
	if audioTopic == nil || len(audioTopic.Extensions) != 1 {
		t.Fatalf("extensions count: %+v", audioTopic)
	}
	if audioTopic.Extensions[0].Provider != "org.xmind.ui.audionotes" || len(audioTopic.Extensions[0].ResourceRefs) != 1 {
		t.Fatalf("audio extension: %+v", audioTopic.Extensions[0])
	}
}

func assertKitchenSinkSheet11Markers(t *testing.T, root *Topic) {
	t.Helper()
	mkTopic := findTopicByID(root, "30888fa1-f425-43e3-a7ce-98d5d7db9578")
	if mkTopic == nil || len(mkTopic.Markers) != 1 || mkTopic.Markers[0].MarkerID != "priority-1" {
		t.Fatalf("markers: %+v", mkTopic)
	}
}

// TestReadMapMetadataFields uses Sheet 10 (Topic Properties) and Sheet 11 (Markers) from the
// kitchen sink — the first sheet (Mind Map) has no labeled/marker/note/href/extension samples.
func TestReadMapMetadataFields(t *testing.T) {
	sheets, err := ReadMap(kitchenSinkPath(t))
	if err != nil {
		t.Fatal(err)
	}
	prop := findSheetByTitle(sheets, "Sheet 10 - Topic Properties")
	if prop == nil {
		t.Fatal("missing Sheet 10 - Topic Properties")
	}
	rt := &prop.RootTopic
	assertKitchenSinkSheet10Labels(t, rt)
	assertKitchenSinkSheet10Notes(t, rt)
	assertKitchenSinkSheet10Href(t, rt)
	assertKitchenSinkSheet10AudioExtension(t, rt)

	markersSh := findSheetByTitle(sheets, "Sheet 11 - Markers")
	if markersSh == nil {
		t.Fatal("missing Sheet 11 - Markers")
	}
	assertKitchenSinkSheet11Markers(t, &markersSh.RootTopic)
}

func TestReadMapRelationships(t *testing.T) {
	sheets, err := ReadMap(kitchenSinkPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) < 12 {
		t.Fatalf("want at least 12 sheets, got %d", len(sheets))
	}
	sh := sheets[11]
	if !strings.Contains(sh.Title, "Relationships") {
		t.Fatalf("expected relationships sheet at index 11, got title %q", sh.Title)
	}
	if len(sh.Relationships) < 1 {
		t.Fatalf("expected relationships, got %+v", sh.Relationships)
	}
	rel := sh.Relationships[0]
	if rel.End1ID == "" || rel.End2ID == "" {
		t.Fatalf("relationship endpoints: %+v", rel)
	}
}

func TestReadMapFloatingTopicPosition(t *testing.T) {
	sheets, err := ReadMap(kitchenSinkPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) < 12 {
		t.Fatalf("want at least 12 sheets, got %d", len(sheets))
	}
	sh := sheets[11]
	rt := sh.RootTopic
	if rt.Children == nil || len(rt.Children.Detached) == 0 {
		t.Fatal("expected detached topics on relationships sheet")
	}
	var found bool
	for i := range rt.Children.Detached {
		d := &rt.Children.Detached[i]
		if d.Position == nil {
			continue
		}
		if d.Position.X != 0 || d.Position.Y != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a detached topic with non-zero position, got %+v", rt.Children.Detached)
	}
}

func TestReadMapKitchenSink(t *testing.T) {
	path := kitchenSinkPath(t)
	sheets, err := ReadMap(path)
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	if len(sheets) != 15 {
		t.Fatalf("sheet count: got %d want 15", len(sheets))
	}
	if got, want := sheets[0].Title, "Sheet 1 - Mind Map"; got != want {
		t.Fatalf("sheet[0].title: got %q want %q", got, want)
	}
	if got, want := sheets[1].Title, "Sheet 2 - Logic Chart"; got != want {
		t.Fatalf("sheet[1].title: got %q want %q", got, want)
	}
	if got, want := sheets[14].Title, "Sheet 15 - Edge Cases"; got != want {
		t.Fatalf("sheet[14].title: got %q want %q", got, want)
	}
}

func TestReadMapDoesNotModifyKitchenSink(t *testing.T) {
	path := kitchenSinkPath(t)
	before := fileSHA256(t, path)
	if _, err := ReadMap(path); err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	after := fileSHA256(t, path)
	if before != after {
		t.Fatalf("kitchen-sink.xmind changed on disk after ReadMap")
	}
}

func assertRoundTripRootTitlesMatch(t *testing.T, sheets1, sheets2 []Sheet) {
	t.Helper()
	if len(sheets2) != len(sheets1) {
		t.Fatalf("sheet count after write: got %d want %d", len(sheets2), len(sheets1))
	}
	for i := range sheets1 {
		if sheets2[i].RootTopic.Title != sheets1[i].RootTopic.Title {
			t.Fatalf("sheet %d root title: got %q want %q", i, sheets2[i].RootTopic.Title, sheets1[i].RootTopic.Title)
		}
	}
}

func assertMarshaledSheetsEqual(t *testing.T, sheets1, sheets2 []Sheet) {
	t.Helper()
	b1, err := json.Marshal(sheets1)
	if err != nil {
		t.Fatalf("marshal sheets1: %v", err)
	}
	b2, err := json.Marshal(sheets2)
	if err != nil {
		t.Fatalf("marshal sheets2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("round-trip JSON mismatch (len %d vs %d)", len(b1), len(b2))
	}
}

func TestWriteMapRoundTrip(t *testing.T) {
	src := kitchenSinkPath(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "roundtrip.xmind")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	sheets1, err := ReadMap(dst)
	if err != nil {
		t.Fatalf("ReadMap before write: %v", err)
	}
	if len(sheets1) != 15 {
		t.Fatalf("sheet count before write: got %d", len(sheets1))
	}

	if err := WriteMap(dst, sheets1); err != nil {
		t.Fatalf("WriteMap: %v", err)
	}

	sheets2, err := ReadMap(dst)
	if err != nil {
		t.Fatalf("ReadMap after write: %v", err)
	}
	assertRoundTripRootTitlesMatch(t, sheets1, sheets2)
	assertMarshaledSheetsEqual(t, sheets1, sheets2)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
