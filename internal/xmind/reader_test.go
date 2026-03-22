package xmind

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

func TestReadMapErrorMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badjson.xmind")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("content.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("{not valid json")); err != nil {
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
		t.Fatal("expected error for malformed content.json")
	}
	if !strings.Contains(err.Error(), "parse content.json") {
		t.Fatalf("expected parse content.json error, got: %v", err)
	}
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
	labelTopic := findTopicByID(&prop.RootTopic, "9b558db8-cdf3-47ab-a6b0-ee688aa3ad58")
	if labelTopic == nil || len(labelTopic.Labels) != 1 || labelTopic.Labels[0] != "foo" {
		t.Fatalf("labels: %+v", labelTopic)
	}
	noteTopic := findTopicByID(&prop.RootTopic, "83785e34-fcdb-4049-8c42-15052c20d8d6")
	if noteTopic == nil || noteTopic.Notes == nil || noteTopic.Notes.Plain == nil ||
		!strings.HasPrefix(noteTopic.Notes.Plain.Content, "This is a simple, plain text note") {
		t.Fatalf("notes: %+v", noteTopic)
	}
	hrefTopic := findTopicByID(&prop.RootTopic, "e0d8096f-cc1a-4c33-bcc5-db9006481f85")
	if hrefTopic == nil || hrefTopic.Href != "https://www.google.com" {
		t.Fatalf("href: %+v", hrefTopic)
	}
	audioTopic := findTopicByID(&prop.RootTopic, "9a8153a4-0ac2-4a5c-bc46-db9a0f38e47c")
	if audioTopic == nil || len(audioTopic.Extensions) != 1 {
		t.Fatalf("extensions count: %+v", audioTopic)
	}
	if audioTopic.Extensions[0].Provider != "org.xmind.ui.audionotes" || len(audioTopic.Extensions[0].ResourceRefs) != 1 {
		t.Fatalf("audio extension: %+v", audioTopic.Extensions[0])
	}

	markersSh := findSheetByTitle(sheets, "Sheet 11 - Markers")
	if markersSh == nil {
		t.Fatal("missing Sheet 11 - Markers")
	}
	mkTopic := findTopicByID(&markersSh.RootTopic, "30888fa1-f425-43e3-a7ce-98d5d7db9578")
	if mkTopic == nil || len(mkTopic.Markers) != 1 || mkTopic.Markers[0].MarkerID != "priority-1" {
		t.Fatalf("markers: %+v", mkTopic)
	}
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
	if len(sheets2) != len(sheets1) {
		t.Fatalf("sheet count after write: got %d want %d", len(sheets2), len(sheets1))
	}

	for i := range sheets1 {
		if sheets2[i].RootTopic.Title != sheets1[i].RootTopic.Title {
			t.Fatalf("sheet %d root title: got %q want %q", i, sheets2[i].RootTopic.Title, sheets1[i].RootTopic.Title)
		}
	}

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
