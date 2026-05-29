package xmind

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestNotesPlainOnlyNoRealHTML(t *testing.T) {
	const in = `{"id":"t1","class":"topic","title":"T","notes":{"plain":{"content":"hello"}}}`
	var topic Topic
	if err := json.Unmarshal([]byte(in), &topic); err != nil {
		t.Fatal(err)
	}
	if topic.Notes == nil || topic.Notes.Plain == nil || topic.Notes.Plain.Content != "hello" {
		t.Fatalf("notes plain: %+v", topic.Notes)
	}
	if topic.Notes.RealHTML != nil {
		t.Fatalf("expected no realHTML, got %+v", topic.Notes.RealHTML)
	}
	out, err := json.Marshal(&topic)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("realHTML")) {
		t.Fatalf("marshal should omit realHTML: %s", out)
	}
}

// TestNoteContentPreservesUnknownSiblingKeys covers issue #42 case 1: a key alongside
// "content" inside a note body (e.g. notes.plain) must survive a round-trip.
func TestNoteContentPreservesUnknownSiblingKeys(t *testing.T) {
	const in = `{"content":"hi","foo":1}`
	var nc NoteContent
	if err := json.Unmarshal([]byte(in), &nc); err != nil {
		t.Fatal(err)
	}
	if nc.Content != "hi" {
		t.Fatalf("content: got %q want %q", nc.Content, "hi")
	}
	out, err := json.Marshal(&nc)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["foo"]; !ok {
		t.Fatalf("unknown sibling key foo dropped from NoteContent: %s", out)
	}
}

// TestNotesPreservesUnknownSiblingKeys covers issue #42 case 2: a key alongside
// "plain"/"realHTML" on the notes object must survive a round-trip.
func TestNotesPreservesUnknownSiblingKeys(t *testing.T) {
	const in = `{"plain":{"content":"hi"},"bar":2}`
	var n Notes
	if err := json.Unmarshal([]byte(in), &n); err != nil {
		t.Fatal(err)
	}
	if n.Plain == nil || n.Plain.Content != "hi" {
		t.Fatalf("plain: %+v", n.Plain)
	}
	out, err := json.Marshal(&n)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["bar"]; !ok {
		t.Fatalf("unknown sibling key bar dropped from Notes: %s", out)
	}
}

// TestNotesUnknownKeysSurviveTopicRoundTrip exercises both issue #42 cases at once through a
// full Topic unmarshal -> marshal, confirming the fix holds via the Topic decode path.
func TestNotesUnknownKeysSurviveTopicRoundTrip(t *testing.T) {
	const in = `{"id":"t1","class":"topic","title":"T",` +
		`"notes":{"plain":{"content":"hi","foo":1},"bar":2}}`
	var topic Topic
	if err := json.Unmarshal([]byte(in), &topic); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(&topic)
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		Notes struct {
			Plain map[string]json.RawMessage `json:"plain"`
			Bar   json.RawMessage            `json:"bar"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top.Notes.Plain["foo"]; !ok {
		t.Errorf("note-body sibling key foo dropped through Topic round-trip: %s", out)
	}
	if len(top.Notes.Bar) == 0 {
		t.Errorf("notes-object sibling key bar dropped through Topic round-trip: %s", out)
	}
}

// TestNotesAndNoteContentUnmarshalErrors covers the error branches of the note codecs:
// a non-object body, and a malformed plain/realHTML value that fails to decode as a note.
func TestNotesAndNoteContentUnmarshalErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		dst  json.Unmarshaler
	}{
		{"NoteContent non-object", `[1,2]`, new(NoteContent)},
		{"Notes non-object", `[1,2]`, new(Notes)},
		{"Notes malformed plain", `{"plain":[1]}`, new(Notes)},
		{"Notes malformed realHTML", `{"realHTML":[1]}`, new(Notes)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.in), tc.dst); err == nil {
				t.Fatalf("expected error unmarshalling %q, got nil", tc.in)
			}
		})
	}
}

func TestTopicTitleMarshalUsesRawAmpersandInJSON(t *testing.T) {
	topic := Topic{
		ID:    "t1",
		Class: "topic",
		Title: "Timeline & Milestones",
	}
	// Same helper as encodeToRawMap and marshalSheetsForContentJSON (not json.Marshal).
	out, err := marshalJSONNoHTMLEscape(&topic)
	if err != nil {
		t.Fatal(err)
	}
	want := `"title":"Timeline & Milestones"`
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("encoded JSON should contain %q, got %s", want, out)
	}
	if bytes.Contains(out, []byte(`\u0026`)) {
		t.Fatalf("encoded JSON should not use \\u0026 for ampersand, got %s", out)
	}
}

func TestSheetPreservesUnknownTopLevelKeys(t *testing.T) {
	const in = `{"id":"s1","class":"sheet","title":"S","rootTopic":{"id":"r","class":"topic","title":"R"},
		"legend":{"foo":1},"style":{"k":"v"}}`
	var sh Sheet
	if err := json.Unmarshal([]byte(in), &sh); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(&sh)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["legend"]; !ok {
		t.Fatalf("missing legend after marshal: %s", out)
	}
	if _, ok := m["style"]; !ok {
		t.Fatalf("missing style after marshal: %s", out)
	}
}

func TestTopicPreservesUnknownKeys(t *testing.T) {
	const in = `{"id":"x","class":"topic","title":"X","numbering":{"k":"v"},"style":{"a":1}}`
	var topic Topic
	if err := json.Unmarshal([]byte(in), &topic); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(&topic)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["numbering"]; !ok {
		t.Fatalf("missing numbering: %s", out)
	}
	if _, ok := m["style"]; !ok {
		t.Fatalf("missing style: %s", out)
	}
}

func TestRelationshipPreservesClassAndStyle(t *testing.T) {
	const in = `{"id":"r1","class":"relationship","end1Id":"a","end2Id":"b","style":{"x":1}}`
	var rel Relationship
	if err := json.Unmarshal([]byte(in), &rel); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(&rel)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["class"]) != `"relationship"` {
		t.Fatalf("class: got %s", m["class"])
	}
	if _, ok := m["style"]; !ok {
		t.Fatalf("missing style: %s", out)
	}
}

func TestChildrenPreservesUnknownBucket(t *testing.T) {
	const in = `{"attached":[{"id":"c","class":"topic","title":"C"}],"callout":[]}`
	var ch Children
	if err := json.Unmarshal([]byte(in), &ch); err != nil {
		t.Fatal(err)
	}
	if len(ch.Attached) != 1 {
		t.Fatalf("attached: %d", len(ch.Attached))
	}
	out, err := json.Marshal(&ch)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("callout")) {
		t.Fatalf("expected callout in output: %s", out)
	}
}

// TestWriteMapZipRoundTripPreservesSheetExtra checks a ReadMap -> WriteMap -> ReadMap cycle keeps bag keys on a sheet.
func TestWriteMapZipRoundTripPreservesSheetExtra(t *testing.T) {
	src := kitchenSinkPath(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "preserve.xmind")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) == 0 {
		t.Fatal("no sheets")
	}
	key := "_xmindMcpPreserveTest"
	sheets[0].extra = map[string]json.RawMessage{
		key: json.RawMessage(`{"roundTrip":true}`),
	}
	if err := WriteMap(dst, sheets); err != nil {
		t.Fatal(err)
	}
	sheets2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := sheets2[0].extra[key]
	if !ok {
		t.Fatalf("extra key %q missing after zip round-trip", key)
	}
	if !bytes.Contains(raw, []byte("roundTrip")) {
		t.Fatalf("unexpected preserved value: %s", raw)
	}
}

// TestSheetMarshalPreservesRootTopicUnknownKeys verifies that unknown keys on a
// sheet's rootTopic survive Sheet.MarshalJSON, identically to keys on child topics.
// Sheet.MarshalJSON must marshal the root topic through (*Topic).MarshalJSON so its
// preserved extra bag is merged back rather than dropped.
func TestSheetMarshalPreservesRootTopicUnknownKeys(t *testing.T) {
	const in = `{"id":"s1","class":"sheet","title":"S",
		"rootTopic":{"id":"r","class":"topic","title":"R","customRootKey":{"foo":1}}}`
	var sh Sheet
	if err := json.Unmarshal([]byte(in), &sh); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(&sh)
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		RootTopic map[string]json.RawMessage `json:"rootTopic"`
	}
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top.RootTopic["customRootKey"]; !ok {
		t.Fatalf("missing customRootKey on rootTopic after marshal: %s", out)
	}
}

// TestWriteMapZipRoundTripPreservesNotesExtra checks that a ReadMap -> WriteMap -> ReadMap cycle
// keeps preserved (unknown) keys at both the Notes level and the NoteContent level. Notes are
// attached to the root topic so the test does not depend on the fixture having note-bearing topics.
func TestWriteMapZipRoundTripPreservesNotesExtra(t *testing.T) {
	sheets, dst := readKitchenSinkCopy(t, "preserve-notes.xmind")
	const notesKey = "_xmindMcpNotesPreserve"
	const contentKey = "_xmindMcpNoteContentPreserve"
	sheets[0].RootTopic.Notes = &Notes{
		Plain: &NoteContent{
			Content: "n",
			extra:   map[string]json.RawMessage{contentKey: json.RawMessage(`{"at":"content"}`)},
		},
		extra: map[string]json.RawMessage{notesKey: json.RawMessage(`{"at":"notes"}`)},
	}
	if err := WriteMap(dst, sheets); err != nil {
		t.Fatal(err)
	}
	sheets2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	notes := sheets2[0].RootTopic.Notes
	if notes == nil {
		t.Fatal("notes lost after round-trip")
	}
	if _, ok := notes.extra[notesKey]; !ok {
		t.Errorf("Notes-level extra key %q dropped after zip round-trip", notesKey)
	}
	if notes.Plain == nil {
		t.Fatal("notes.plain lost after round-trip")
	}
	if _, ok := notes.Plain.extra[contentKey]; !ok {
		t.Errorf("NoteContent-level extra key %q dropped after zip round-trip", contentKey)
	}
}

// readKitchenSinkCopy copies the kitchen-sink fixture into a fresh temp file named name
// and returns the parsed sheets together with the temp path for write-back.
func readKitchenSinkCopy(t *testing.T, name string) ([]Sheet, string) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), name)
	if err := copyFile(kitchenSinkPath(t), dst); err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) == 0 {
		t.Fatal("no sheets")
	}
	return sheets, dst
}

// TestWriteMapZipRoundTripPreservesRootTopicExtra checks that a ReadMap -> WriteMap ->
// ReadMap cycle keeps preserved (unknown) keys on a sheet's root topic.
func TestWriteMapZipRoundTripPreservesRootTopicExtra(t *testing.T) {
	sheets, dst := readKitchenSinkCopy(t, "preserve-root.xmind")
	key := "_xmindMcpRootPreserveTest"
	sheets[0].RootTopic.extra = map[string]json.RawMessage{
		key: json.RawMessage(`{"roundTrip":true}`),
	}
	if err := WriteMap(dst, sheets); err != nil {
		t.Fatal(err)
	}
	sheets2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := sheets2[0].RootTopic.extra[key]
	if !ok {
		t.Fatalf("root topic extra key %q missing after zip round-trip", key)
	}
	if !bytes.Contains(raw, []byte("roundTrip")) {
		t.Fatalf("unexpected preserved value: %s", raw)
	}
}

// TestWriteMapZipRoundTripRootAndChildExtraParity verifies that the unknown-key
// fidelity guarantee is not position-dependent: preserved keys survive a ReadMap ->
// WriteMap -> ReadMap cycle on both the root topic and a child topic. The two assertions
// use t.Errorf (not t.Fatalf) so a failure on one does not hide the result of the other.
func TestWriteMapZipRoundTripRootAndChildExtraParity(t *testing.T) {
	sheets, dst := readKitchenSinkCopy(t, "parity.xmind")
	root := &sheets[0].RootTopic
	if root.Children == nil || len(root.Children.Attached) == 0 {
		t.Fatal("expected root topic to have at least one attached child")
	}
	const rootKey = "_xmindMcpRootParity"
	const childKey = "_xmindMcpChildParity"
	root.extra = map[string]json.RawMessage{rootKey: json.RawMessage(`{"at":"root"}`)}
	root.Children.Attached[0].extra = map[string]json.RawMessage{
		childKey: json.RawMessage(`{"at":"child"}`),
	}
	if err := WriteMap(dst, sheets); err != nil {
		t.Fatal(err)
	}
	sheets2, err := ReadMap(dst)
	if err != nil {
		t.Fatal(err)
	}
	root2 := &sheets2[0].RootTopic
	if _, ok := root2.extra[rootKey]; !ok {
		t.Errorf("root topic extra key %q dropped after round-trip", rootKey)
	}
	if root2.Children == nil || len(root2.Children.Attached) == 0 {
		t.Fatal("attached children lost after round-trip")
	}
	if _, ok := root2.Children.Attached[0].extra[childKey]; !ok {
		t.Errorf("child topic extra key %q dropped after round-trip", childKey)
	}
}
