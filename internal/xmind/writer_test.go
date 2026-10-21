package xmind

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

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
	before := fileSHA256(t, dst)
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
	if after := fileSHA256(t, dst); after != before {
		t.Fatalf("original file changed after a failed write: before %s, after %s", before, after)
	}
}

// TestWriteMapPreservesOriginalOnSwapFailure exercises the full atomic path: the
// temp file is written successfully, but the final swap-into-place fails. The
// original destination must be left byte-identical and no temp residue may
// remain. The chmod-based test above blocks temp-file creation, so the swap step
// never runs there; this test forces a failure at the swap by injecting a rename
// error via the osRename seam.
func TestWriteMapPreservesOriginalOnSwapFailure(t *testing.T) {
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
	before := fileSHA256(t, dst)

	orig := osRename
	t.Cleanup(func() { osRename = orig })
	osRename = func(string, string) error {
		return errors.New("injected swap failure")
	}

	if err := WriteMap(dst, sheets); err == nil {
		t.Fatal("expected WriteMap to fail when the swap rename fails")
	}

	if after := fileSHA256(t, dst); after != before {
		t.Fatalf("original file changed after a failed swap: before %s, after %s", before, after)
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

func openZipReader(t *testing.T, path string) *zip.Reader {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

func assertZipHasCoreEntries(t *testing.T, zr *zip.Reader) {
	t.Helper()
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
}

func assertMetadataCreatorName(t *testing.T, zr *zip.Reader, want string) {
	t.Helper()
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
	if meta.Creator.Name != want {
		t.Fatalf("creator.name: got %q want %q", meta.Creator.Name, want)
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
	zr := openZipReader(t, path)
	assertZipHasCoreEntries(t, zr)
	assertMetadataCreatorName(t, zr, "xmind-mcp")
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

func minimalSheet() Sheet {
	return Sheet{
		ID:        uuid.New().String(),
		Class:     "sheet",
		Title:     "S",
		RootTopic: Topic{ID: uuid.New().String(), Class: "topic", Title: "R"},
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
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

// TestCreateNewMapRefusesExistingFile checks CreateNewMap never overwrites: an existing
// path is left byte-identical and the error wraps fs.ErrExist.
func TestCreateNewMapRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.xmind")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := CreateNewMap(path, []Sheet{minimalSheet()})
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("CreateNewMap on existing file: err = %v, want fs.ErrExist", err)
	}
	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "original" {
		t.Fatalf("existing file was modified: %q", got)
	}
	assertNoTempFiles(t, dir)
}

// TestCreateNewMapRemovesPlaceholderOnFailure checks that when the final rename fails,
// CreateNewMap removes the placeholder it claimed and leaves no temp files.
func TestCreateNewMapRemovesPlaceholderOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.xmind")

	orig := osRename
	t.Cleanup(func() { osRename = orig })
	osRename = func(string, string) error {
		return errors.New("injected swap failure")
	}

	if err := CreateNewMap(path, []Sheet{minimalSheet()}); err == nil {
		t.Fatal("expected CreateNewMap to fail when the swap rename fails")
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("placeholder left behind after failure: stat err = %v", err)
	}
	assertNoTempFiles(t, dir)
}

// skipUnlessUnixModes skips tests that assert Unix permission bits; Windows tracks only a
// read-only flag.
func skipUnlessUnixModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
}

// assertPerm fails unless path's permission bits are want.
func assertPerm(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", filepath.Base(path), got, want)
	}
}

func TestWritesPreserveFileMode(t *testing.T) {
	skipUnlessUnixModes(t)
	// 0640 differs from both os.CreateTemp's 0600 and a typical umask default.
	const want fs.FileMode = 0o640

	t.Run("Commit", func(t *testing.T) {
		dst := copyKitchenSink(t)
		if err := os.Chmod(dst, want); err != nil {
			t.Fatal(err)
		}
		sheets, mw, err := OpenMapForUpdate(dst)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = mw.Close() }()
		if err := mw.Commit(sheets); err != nil {
			t.Fatal(err)
		}
		assertPerm(t, dst, want)
	})

	t.Run("WriteMap", func(t *testing.T) {
		dst := copyKitchenSink(t)
		if err := os.Chmod(dst, want); err != nil {
			t.Fatal(err)
		}
		sheets, err := ReadMap(dst)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteMap(dst, sheets); err != nil {
			t.Fatal(err)
		}
		assertPerm(t, dst, want)
	})
}

// mapWrites are the two ways to save a map: a MapWriter's Commit and a fresh WriteMap.
var mapWrites = []struct {
	name  string
	write func(t *testing.T, dst string) error
}{
	{"Commit", func(t *testing.T, dst string) error {
		sheets, mw, err := OpenMapForUpdate(dst)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = mw.Close() }()
		return mw.Commit(sheets)
	}},
	{"WriteMap", func(t *testing.T, dst string) error {
		sheets, err := ReadMap(dst)
		if err != nil {
			t.Fatal(err)
		}
		return WriteMap(dst, sheets)
	}},
}

// TestWritesRefuseReadOnlyFile checks that both writes refuse a map this process can't write and
// leave it untouched: one with no write bits, and one only its group can write (0464, as
// chmod u-w leaves a group-writable map). The swap needs only a writable folder, so either would
// otherwise be replaced.
func TestWritesRefuseReadOnlyFile(t *testing.T) {
	skipUnlessUnixModes(t)
	for _, mode := range []fs.FileMode{0o444, 0o464} {
		for _, tc := range mapWrites {
			t.Run(fmt.Sprintf("%s/%04o", tc.name, mode), func(t *testing.T) {
				if mode&0o222 != 0 && os.Geteuid() == 0 {
					t.Skip("root can write a file that has any write bit")
				}
				dst := copyKitchenSink(t)
				t.Cleanup(func() { _ = os.Chmod(dst, 0o644) })
				if err := os.Chmod(dst, mode); err != nil {
					t.Fatal(err)
				}
				before := fileSHA256(t, dst)
				if err := tc.write(t, dst); !errors.Is(err, ErrMapReadOnly) || !errors.Is(err, fs.ErrPermission) {
					t.Errorf("write to a read-only map: err = %v, want ErrMapReadOnly, matching fs.ErrPermission", err)
				}
				assertMapUntouched(t, dst, before, mode)
			})
		}
	}
}

// assertMapUntouched fails unless dst still has the content hash before and the given mode, and
// no temp file was left beside it.
func assertMapUntouched(t *testing.T, dst, before string, mode fs.FileMode) {
	t.Helper()
	if after := fileSHA256(t, dst); after != before {
		t.Errorf("map changed: before %s, after %s", before, after)
	}
	assertPerm(t, dst, mode)
	ents, err := os.ReadDir(filepath.Dir(dst))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".xmind-tmp-") {
			t.Errorf("leaked temp file: %s", e.Name())
		}
	}
}

// rawCopyContentJSON is a minimal content.json that passes the save check.
const rawCopyContentJSON = `[{"id":"s1","class":"sheet","title":"S","rootTopic":{"id":"r","class":"topic","title":"R"}}]`

// TestRawCopyPreservesEntryHeadersAndBytes checks that both writes copy every entry other than
// content.json as it was: same order, same header, same compressed bytes. The fixture holds
// header values Go's zip writer never produces, so a copy that rebuilt headers would fail.
func TestRawCopyPreservesEntryHeadersAndBytes(t *testing.T) {
	for _, tc := range mapWrites {
		t.Run(tc.name, func(t *testing.T) {
			// Compare against an untouched original, never the file being saved.
			orig := rawCopyFixture(t)
			dst := filepath.Join(filepath.Dir(orig), "m.xmind")
			if err := copyFile(orig, dst); err != nil {
				t.Fatal(err)
			}
			if err := tc.write(t, dst); err != nil {
				t.Fatal(err)
			}
			want := entriesExceptContentJSON(openZipReader(t, orig))
			got := entriesExceptContentJSON(openZipReader(t, dst))
			assertRawCopyFixture(t, want)
			if !slices.Equal(entryNames(got), entryNames(want)) {
				t.Fatalf("entries = %v, want %v", entryNames(got), entryNames(want))
			}
			for i := range want {
				assertRawCopy(t, want[i], got[i])
			}
		})
	}
}

// rawCopyFixture writes a map whose entries other than content.json each carry something a copy
// could lose, and returns its path. content.json is not last, and the entries are not sorted.
func rawCopyFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "orig.xmind")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)

	dir := &zip.FileHeader{Name: "dir/"}
	dir.SetMode(fs.ModeDir | 0o755)
	writeZipEntry(t, zw.CreateHeader, dir, nil)

	// Written raw, with header values CreateHeader would replace: it sets the reader version and
	// the creator version's low byte to 20 and adds the data-descriptor flag.
	stored := []byte("stored body")
	raw := &zip.FileHeader{
		Name:               "stored.bin",
		Method:             zip.Store,
		CreatorVersion:     0x3F,
		ReaderVersion:      10,
		CRC32:              crc32.ChecksumIEEE(stored),
		CompressedSize64:   uint64(len(stored)),
		UncompressedSize64: uint64(len(stored)),
		Extra:              []byte{0xFE, 0xCA, 0x04, 0x00, 1, 2, 3, 4}, // unregistered header ID 0xCAFE
	}
	raw.SetMode(0o640)
	writeZipEntry(t, zw.CreateRaw, raw, stored)

	writeZipEntry(t, zw.CreateHeader, &zip.FileHeader{Name: "content.json", Method: zip.Deflate}, []byte(rawCopyContentJSON))
	writeZipEntry(t, zw.CreateHeader, &zip.FileHeader{Name: "notes/comment.txt", Method: zip.Deflate, Comment: "kept"},
		bytes.Repeat([]byte("xmind "), 100))
	writeZipEntry(t, zw.CreateHeader, &zip.FileHeader{
		Name:     "dated.txt",
		Method:   zip.Deflate,
		Modified: time.Date(2024, 3, 9, 14, 30, 8, 0, time.FixedZone("", -5*3600)),
	}, []byte("dated"))

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeZipEntry adds an entry made by create (a zip.Writer's CreateHeader or CreateRaw) and
// writes body to it.
func writeZipEntry(t *testing.T, create func(*zip.FileHeader) (io.Writer, error), fh *zip.FileHeader, body []byte) {
	t.Helper()
	w, err := create(fh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
}

// assertRawCopyFixture fails unless the fixture's entries carry what the test is about, so a
// change to the fixture or to Go's zip writer can't quietly make the test easier.
func assertRawCopyFixture(t *testing.T, files []*zip.File) {
	t.Helper()
	f := make(map[string]*zip.File, len(files))
	for _, zf := range files {
		f[zf.Name] = zf
	}
	for _, name := range []string{"dir/", "stored.bin", "notes/comment.txt", "dated.txt"} {
		if f[name] == nil {
			t.Fatalf("fixture entries = %v, want one named %s", entryNames(files), name)
		}
	}
	for _, c := range []struct {
		what      string
		got, want any
	}{
		{"stored.bin reader version", f["stored.bin"].ReaderVersion, uint16(10)},
		{"stored.bin data-descriptor flag", f["stored.bin"].Flags & 0x8, uint16(0)},
		{"stored.bin has a 0xCAFE extra field", bytes.HasPrefix(f["stored.bin"].Extra, []byte{0xFE, 0xCA}), true},
		{"notes/comment.txt comment", f["notes/comment.txt"].Comment, "kept"},
		{"notes/comment.txt is compressed", f["notes/comment.txt"].CompressedSize64 < f["notes/comment.txt"].UncompressedSize64, true},
		{"dated.txt Modified is in UTC", f["dated.txt"].Modified.Location() == time.UTC, false},
		{"dir/ is a directory", f["dir/"].Mode().IsDir(), true},
	} {
		if c.got != c.want {
			t.Errorf("fixture: %s = %v, want %v", c.what, c.got, c.want)
		}
	}
}

// entriesExceptContentJSON returns zr's entries other than content.json, in zip order.
func entriesExceptContentJSON(zr *zip.Reader) []*zip.File {
	var files []*zip.File
	for _, f := range zr.File {
		if f.Name != "content.json" {
			files = append(files, f)
		}
	}
	return files
}

// entryNames returns the names of files, in order.
func entryNames(files []*zip.File) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	return names
}

// rawEntryBytes returns f's data as stored in the archive, still compressed.
func rawEntryBytes(t *testing.T, f *zip.File) []byte {
	t.Helper()
	r, err := f.OpenRaw()
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// assertRawCopy fails unless got has want's header and raw bytes. Modified is compared with
// Equal, because the reader builds a new time zone for each read.
func assertRawCopy(t *testing.T, want, got *zip.File) {
	t.Helper()
	w, g := want.FileHeader, got.FileHeader
	if !g.Modified.Equal(w.Modified) {
		t.Errorf("%s: Modified = %v, want %v", want.Name, g.Modified, w.Modified)
	}
	w.Modified, g.Modified = time.Time{}, time.Time{}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("%s: header = %+v, want %+v", want.Name, g, w)
	}
	if !bytes.Equal(rawEntryBytes(t, got), rawEntryBytes(t, want)) {
		t.Errorf("%s: raw bytes differ from the original's", want.Name)
	}
}

// TestWritesKeepUndecodableFields checks that both writes keep, unchanged, values a read could
// not decode in fields no handler writes: on the sheet, the root topic, its note, a boundary and
// a relationship.
func TestWritesKeepUndecodableFields(t *testing.T) {
	const content = `[{"id":"s1","class":"sheet","title":5,
		"rootTopic":{"id":"r","class":5,"title":"R","position":"x","image":7,
			"notes":{"plain":{"content":5}},
			"boundaries":[{"id":"b1","range":"master","title":5}],
			"children":{"attached":[{"id":"c","title":"C"}]}},
		"relationships":[{"id":"rel","end1Id":7,"end2Id":"c","controlPoints":{"0":5}}]}]`
	kept := []struct {
		path []any
		want string
	}{
		{[]any{0, "title"}, `5`},
		{[]any{0, "rootTopic", "class"}, `5`},
		{[]any{0, "rootTopic", "position"}, `"x"`},
		{[]any{0, "rootTopic", "image"}, `7`},
		{[]any{0, "rootTopic", "notes", "plain", "content"}, `5`},
		{[]any{0, "rootTopic", "boundaries", 0, "title"}, `5`},
		{[]any{0, "relationships", 0, "end1Id"}, `7`},
		{[]any{0, "relationships", 0, "controlPoints"}, `{"0":5}`},
	}
	for _, tc := range mapWrites {
		t.Run(tc.name, func(t *testing.T) {
			dst := writeContentJSONMap(t, content)
			if err := tc.write(t, dst); err != nil {
				t.Fatal(err)
			}
			saved := readContentJSON(t, dst)
			for _, k := range kept {
				if got := compactJSON(t, jsonAt(t, saved, k.path...)); got != k.want {
					t.Errorf("%v = %s, want %s", k.path, got, k.want)
				}
			}
		})
	}
}

// TestWritesRefuseUndecodableStrictField checks that both writes refuse a map holding a value
// that could not be decoded in a field the tools edit, and leave the map untouched. Commit
// refuses even when the sheets it is given no longer hold that value, because the open recorded
// it: a change that removes the topic must not drop the value with it.
func TestWritesRefuseUndecodableStrictField(t *testing.T) {
	const content = `[{"id":"s1","class":"sheet","title":"S","rootTopic":{"id":"r","title":"R",
		"children":{"attached":[{"id":"c","title":"C","labels":"x"}]}}}]`
	const want = "Topic.labels on topic c has an unexpected shape"
	writes := append(slices.Clone(mapWrites), struct {
		name  string
		write func(t *testing.T, dst string) error
	}{"Commit without the topic", func(t *testing.T, dst string) error {
		sheets, mw, err := OpenMapForUpdate(dst)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = mw.Close() }()
		sheets[0].RootTopic.Children = nil
		return mw.Commit(sheets)
	}})
	for _, tc := range writes {
		t.Run(tc.name, func(t *testing.T) {
			dst := writeContentJSONMap(t, content)
			st, err := os.Stat(dst)
			if err != nil {
				t.Fatal(err)
			}
			before := fileSHA256(t, dst)
			if err := tc.write(t, dst); !errors.Is(err, ErrUndecodableField) || err.Error() != want {
				t.Errorf("write: err = %v, want %q matching ErrUndecodableField", err, want)
			}
			assertMapUntouched(t, dst, before, st.Mode().Perm())
		})
	}
}

// TestWriteMapRefusesSheetsThatDoNotMarshal checks that WriteMap fails, writing nothing, when the
// sheets it is given can't be marshaled: here a theme that is not valid JSON, which a read never
// produces but a caller building sheets could pass.
func TestWriteMapRefusesSheetsThatDoNotMarshal(t *testing.T) {
	dst := copyKitchenSink(t)
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	sheets[0].Theme = json.RawMessage(`{`)
	before := fileSHA256(t, dst)
	err = WriteMap(dst, sheets)
	if err == nil || !strings.HasPrefix(err.Error(), "marshal content.json: ") || errors.Is(err, ErrUndecodableField) {
		t.Errorf("write: err = %v, want a marshal content.json error", err)
	}
	assertMapUntouched(t, dst, before, st.Mode().Perm())
}

// jsonAt returns the JSON value at path in doc, where a string steps into an object's key and an
// int into an array's index. It fails the test when the value is missing.
func jsonAt(t *testing.T, doc []byte, path ...any) json.RawMessage {
	t.Helper()
	cur := json.RawMessage(doc)
	for _, step := range path {
		var ok bool
		switch s := step.(type) {
		case string:
			var m map[string]json.RawMessage
			_ = json.Unmarshal(cur, &m)
			cur, ok = m[s]
		case int:
			var a []json.RawMessage
			_ = json.Unmarshal(cur, &a)
			if ok = s < len(a); ok {
				cur = a[s]
			}
		}
		if !ok {
			t.Fatalf("%v: no value at %v", path, step)
		}
	}
	return cur
}

// compactJSON returns raw with insignificant whitespace removed.
func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestCreateNewMapUsesUmaskDefaultMode(t *testing.T) {
	skipUnlessUnixModes(t)
	dir := t.TempDir()

	// A file created with 0666 shows what the current umask allows, as os.Create would.
	ref := filepath.Join(dir, "ref")
	f, err := os.OpenFile(ref, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(ref)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "new.xmind")
	if err := CreateNewMap(path, []Sheet{{ID: uuid.New().String(), Title: "S"}}); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, path, st.Mode().Perm())
}
