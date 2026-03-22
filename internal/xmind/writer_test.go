package xmind

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func zipEntryNames(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	slices.Sort(names)
	return names
}

func TestWriteMapPreservesNonContentEntries(t *testing.T) {
	src := kitchenSinkPath(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	before := zipEntryNames(t, dst)
	sheets, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteMap(dst, sheets); err != nil {
		t.Fatal(err)
	}
	after := zipEntryNames(t, dst)
	if !slices.Equal(before, after) {
		t.Fatalf("zip entry names changed:\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestWriteMapRawMessageFieldsPreserved(t *testing.T) {
	src := kitchenSinkPath(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	s1, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) == 0 {
		t.Fatal("no sheets")
	}
	themeBefore := append([]byte(nil), s1[0].Theme...)
	if len(themeBefore) == 0 {
		t.Fatal("expected sheet[0].Theme to be non-empty in kitchen sink")
	}
	if err := WriteMap(dst, s1); err != nil {
		t.Fatal(err)
	}
	s2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(themeBefore, s2[0].Theme) {
		t.Fatalf("Theme RawMessage changed (len before %d after %d)", len(themeBefore), len(s2[0].Theme))
	}
}

func TestWriteMapDoesNotBumpRevisionID(t *testing.T) {
	src := kitchenSinkPath(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	s1, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	rev := make([]string, len(s1))
	for i := range s1 {
		rev[i] = s1[i].RevisionID
	}
	if err := WriteMap(dst, s1); err != nil {
		t.Fatal(err)
	}
	s2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s1 {
		if s2[i].RevisionID != rev[i] {
			t.Fatalf("sheet %d revisionId changed: was %q now %q", i, rev[i], s2[i].RevisionID)
		}
	}
}

func TestWriteMapAtomicSafetyOnFailure(t *testing.T) {
	src := kitchenSinkPath(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "k.xmind")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	err = WriteMap(dst, sheets)
	if err == nil {
		t.Fatal("expected WriteMap to fail when temp dir is not writable")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".xmind-tmp-") {
			t.Fatalf("leaked temp file: %s", e.Name())
		}
	}
}

func TestCreateNewMapFileStructure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.xmind")
	sh := Sheet{
		ID:               uuid.New().String(),
		RevisionID:       uuid.New().String(),
		Class:            "sheet",
		Title:            "S1",
		TopicOverlapping: "overlap",
		RootTopic: Topic{
			ID:             uuid.New().String(),
			Class:          "topic",
			Title:          "Root",
			StructureClass: "org.xmind.ui.map.clockwise",
		},
	}
	if err := CreateNewMap(path, []Sheet{sh}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{}{
		"content.json":  {},
		"metadata.json": {},
		"content.xml":   {},
		"manifest.json": {},
	}
	got := make(map[string]struct{})
	for _, zf := range zr.File {
		got[zf.Name] = struct{}{}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing zip entry %q, have %v", name, got)
		}
	}
	var metaEntry *zip.File
	for i := range zr.File {
		if zr.File[i].Name == "metadata.json" {
			metaEntry = zr.File[i]
			break
		}
	}
	if metaEntry == nil {
		t.Fatal("metadata.json entry not found")
	}
	rc, err := metaEntry.Open()
	if err != nil {
		t.Fatal(err)
	}
	metaBytes, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Creator struct {
			Name string `json:"name"`
		} `json:"creator"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("metadata.json: %v", err)
	}
	if meta.Creator.Name != "xmind-mcp" {
		t.Fatalf("creator.name: got %q want xmind-mcp", meta.Creator.Name)
	}
}

func TestCreateNewMapReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readback.xmind")
	sh := Sheet{
		ID:               uuid.New().String(),
		RevisionID:       uuid.New().String(),
		Class:            "sheet",
		Title:            "My Sheet",
		TopicOverlapping: "overlap",
		RootTopic: Topic{
			ID:             uuid.New().String(),
			Class:          "topic",
			Title:          "Central",
			StructureClass: "org.xmind.ui.map.clockwise",
		},
	}
	if err := CreateNewMap(path, []Sheet{sh}); err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 {
		t.Fatalf("sheet count: %d", len(sheets))
	}
	if sheets[0].Title != "My Sheet" || sheets[0].RootTopic.Title != "Central" {
		t.Fatalf("got %+v / %+v", sheets[0].Title, sheets[0].RootTopic)
	}
}

func TestCreateNewMapFloatPositionPrecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "floatpos.xmind")
	x, y := -291.5, 226.75
	floatID := uuid.New().String()
	sh := Sheet{
		ID:               uuid.New().String(),
		RevisionID:       uuid.New().String(),
		Class:            "sheet",
		Title:            "FloatSheet",
		TopicOverlapping: "overlap",
		RootTopic: Topic{
			ID:             uuid.New().String(),
			Class:          "topic",
			Title:          "Root",
			StructureClass: "org.xmind.ui.map.clockwise",
			Children: &Children{
				Detached: []Topic{
					{
						ID:    floatID,
						Title: "Float",
						Position: &Position{
							X: x,
							Y: y,
						},
					},
				},
			},
		},
	}
	if err := CreateNewMap(path, []Sheet{sh}); err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	dt := sheets[0].RootTopic.Children.Detached[0]
	if dt.Position == nil {
		t.Fatal("nil position")
	}
	if dt.Position.X != x || dt.Position.Y != y {
		t.Fatalf("position: got (%v,%v) want (%v,%v)", dt.Position.X, dt.Position.Y, x, y)
	}
}
