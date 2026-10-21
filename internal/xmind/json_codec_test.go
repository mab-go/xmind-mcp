package xmind

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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

// TestNotesAndNoteContentUnmarshalErrors covers the error branch of the note codecs: a body
// that is not an object. A malformed plain/realHTML value is kept raw instead
// (TestCodecMalformedInput).
func TestNotesAndNoteContentUnmarshalErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		dst  json.Unmarshaler
	}{
		{"NoteContent non-object", `[1,2]`, new(NoteContent)},
		{"Notes non-object", `[1,2]`, new(Notes)},
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

// TestBadListElementKeepsSiblings checks that one element with a wrong-shaped field keeps its
// siblings: a topic whose notes is not an object, in a children list, and a relationship whose
// control point is not an object, in a sheet. Each list must keep all three elements, and the
// bad element must keep the value it could not decode.
func TestBadListElementKeepsSiblings(t *testing.T) {
	t.Run("children", func(t *testing.T) {
		const in = `{"attached":[{"id":"a"},{"id":"b","notes":[1]},{"id":"c"}]}`
		var c Children
		if err := json.Unmarshal([]byte(in), &c); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, tp := range c.Attached {
			ids = append(ids, tp.ID)
		}
		if !slices.Equal(ids, []string{"a", "b", "c"}) {
			t.Fatalf("attached = %v, want [a b c]", ids)
		}
		if got := string(c.Attached[1].extra["notes"]); got != "[1]" {
			t.Errorf("b's notes = %q, want it kept raw as [1]", got)
		}
	})
	t.Run("relationships", func(t *testing.T) {
		const in = `{"id":"s","title":"S","rootTopic":{"id":"r"},"relationships":[
			{"id":"a","end1Id":"x","end2Id":"y"},
			{"id":"b","end1Id":"x","end2Id":"y","controlPoints":{"0":5},"style":"keep"},
			{"id":"c","end1Id":"x","end2Id":"y"}]}`
		var s Sheet
		if err := json.Unmarshal([]byte(in), &s); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range s.Relationships {
			ids = append(ids, r.ID)
		}
		if !slices.Equal(ids, []string{"a", "b", "c"}) {
			t.Fatalf("relationships = %v, want [a b c]", ids)
		}
		b := s.Relationships[1]
		if got := string(b.extra["controlPoints"]); got != `{"0":5}` {
			t.Errorf("b's controlPoints = %q, want it kept raw", got)
		}
		if got := string(b.extra["style"]); got != `"keep"` {
			t.Errorf("b's style = %q, want it kept", got)
		}
	})
}

// TestCheckSavableNamesField checks the save check's error for a raw value in a strict field at
// each place one can be: it names the type, the key, and where it is. A raw value in a keepRaw
// field is no reason to refuse.
func TestCheckSavableNamesField(t *testing.T) {
	for _, tc := range []struct{ content, want string }{
		{`[{"id":"s1","revisionId":5,"rootTopic":{"id":"r"}}]`, "Sheet.revisionId in sheet s1"},
		{`[{"rootTopic":{"labels":"x"}}]`, "Topic.labels in sheet 1"},
		{`[{"id":"s1","rootTopic":{"id":"r","children":{"attached":[{"id":5}]}}}]`, "Topic.id under topic r"},
		{`[{"id":"s1","rootTopic":{"id":"r","children":{"detached":5}}}]`, "Children.detached on topic r"},
		{`[{"id":"s1","rootTopic":{"id":"r","summaries":[{"id":"m","range":7}]}}]`, "Summary.range on topic r"},
		{`[{"id":"s1","rootTopic":{"id":"r","boundaries":[{"id":[1],"range":"master"}]}}]`, "Boundary.id on topic r"},
		{`[{"id":"s1","rootTopic":{"id":"r","children":{"summary":[{"id":"t","notes":5}]}}}]`, "Topic.notes on topic t"},
		{`[{"id":"s1","title":5,"rootTopic":{"id":"r","position":"x"}}]`, ""},
	} {
		var sheets []Sheet
		if err := json.Unmarshal([]byte(tc.content), &sheets); err != nil {
			t.Fatal(err)
		}
		err := checkSavable(sheets)
		switch want := tc.want + " has an unexpected shape"; {
		case tc.want == "" && err != nil:
			t.Errorf("%s: err = %v, want nil", tc.content, err)
		case tc.want != "" && (!errors.Is(err, ErrUndecodableField) || err.Error() != want):
			t.Errorf("%s: err = %v, want %q", tc.content, err, want)
		}
	}
}

// codecCase is one codec type for the table-driven codec tests, built from its field table.
type codecCase struct {
	name      string
	typ       reflect.Type
	keys      []string
	policies  map[string]fieldPolicy
	newValue  func() any // returns a new *T
	extra     func(v any) map[string]json.RawMessage
	empty     func(v any, key string) bool
	strictKey func(v any) (string, bool)
}

func codecCaseFor[T any](name string, fields []codecField[T], extra func(*T) map[string]json.RawMessage) codecCase {
	c := codecCase{
		name:     name,
		typ:      reflect.TypeFor[T](),
		policies: map[string]fieldPolicy{},
		newValue: func() any { return new(T) },
		extra:    func(v any) map[string]json.RawMessage { return extra(v.(*T)) },
		empty: func(v any, key string) bool {
			f, _ := fieldByKey(fields, key)
			return f.empty(v.(*T))
		},
		strictKey: func(v any) (string, bool) { return firstStrictKey(extra(v.(*T)), fields) },
	}
	for _, f := range fields {
		c.keys = append(c.keys, f.key)
		c.policies[f.key] = f.policy
	}
	return c
}

var codecCases = []codecCase{
	codecCaseFor("Sheet", sheetFields, func(x *Sheet) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Topic", topicFields, func(x *Topic) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Children", childrenFields, func(x *Children) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Summary", summaryFields, func(x *Summary) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Boundary", boundaryFields, func(x *Boundary) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Relationship", relationshipFields, func(x *Relationship) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Marker", markerFields, func(x *Marker) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Position", positionFields, func(x *Position) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("TopicImage", topicImageFields, func(x *TopicImage) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("AttributedTitleItem", attributedTitleItemFields, func(x *AttributedTitleItem) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("Notes", notesFields, func(x *Notes) map[string]json.RawMessage { return x.extra }),
	codecCaseFor("NoteContent", noteContentFields, func(x *NoteContent) map[string]json.RawMessage { return x.extra }),
}

// jsonKey returns the key a struct field's json tag names, or "" when it has no tag.
func jsonKey(sf reflect.StructField) string {
	key, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
	return key
}

// wrongShapedValue returns JSON that can't decode into the field of typ whose key is key: a
// number for a struct, pointer or map field, and an object for any other.
func wrongShapedValue(typ reflect.Type, key string) string {
	for i := range typ.NumField() {
		if sf := typ.Field(i); jsonKey(sf) == key {
			switch sf.Type.Kind() {
			case reflect.Struct, reflect.Pointer, reflect.Map:
				return `5`
			default:
				return `{"bad":true}`
			}
		}
	}
	return ""
}

// TestCodecMalformedInput checks every codec type's decoding: input that isn't an object is an
// error, null gives the zero value, and a wrong-shaped value in any decoded key is kept raw
// (checkWrongShapedField).
func TestCodecMalformedInput(t *testing.T) {
	for _, c := range codecCases {
		t.Run(c.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`[1]`), c.newValue()); err == nil {
				t.Error("an array decoded without an error")
			}
			v := c.newValue()
			if err := json.Unmarshal([]byte(`null`), v); err != nil || !reflect.ValueOf(v).Elem().IsZero() {
				t.Errorf("null: err %v, value %+v; want the zero value", err, v)
			}
			for _, key := range c.keys {
				if c.policies[key] != passthrough {
					t.Run(key, func(t *testing.T) { checkWrongShapedField(t, c, key) })
				}
			}
		})
	}
}

// checkWrongShapedField decodes an object whose key has a value of the wrong shape for its field.
// The decode must succeed, keep the value in extra, and leave the field empty; marshaling must
// write the value back unchanged; and the save check must flag the key exactly when it is strict.
func checkWrongShapedField(t *testing.T, c codecCase, key string) {
	t.Helper()
	bad := wrongShapedValue(c.typ, key)
	v := c.newValue()
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{%q:%s}`, key, bad)), v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := string(c.extra(v)[key]); got != bad {
		t.Errorf("extra[%q] = %q, want %q", key, got, bad)
	}
	if !c.empty(v, key) {
		t.Errorf("the field is not empty: %+v", v)
	}
	out, err := marshalJSONNoHTMLEscape(v)
	if err != nil {
		t.Fatal(err)
	}
	if got := compactJSON(t, jsonAt(t, out, key)); got != bad {
		t.Errorf("written back as %s, want %s", got, bad)
	}
	flagged, ok := c.strictKey(v)
	if want := c.policies[key] == strict; ok != want || (ok && flagged != key) {
		t.Errorf("save check flags %q (%v), want a flag only for a strict key", flagged, ok)
	}
}

// TestCodecFieldTablesMatchStructs checks that each codec type's field table lists its struct's
// json keys, in struct order, and that marshaling a value with every field set writes exactly
// those keys. A new field therefore needs a table entry and a policy, and the hand-kept aux
// structs in the Topic, Sheet and Position marshalers can't drift from the table.
func TestCodecFieldTablesMatchStructs(t *testing.T) {
	for _, c := range codecCases {
		t.Run(c.name, func(t *testing.T) {
			var tags []string
			for i := range c.typ.NumField() {
				if key := jsonKey(c.typ.Field(i)); key != "" {
					tags = append(tags, key)
				}
			}
			if !slices.Equal(c.keys, tags) {
				t.Errorf("table keys %v, struct keys %v", c.keys, tags)
			}
			v := c.newValue()
			fillFields(reflect.ValueOf(v).Elem())
			out, err := marshalJSONNoHTMLEscape(v)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(out, &m); err != nil {
				t.Fatal(err)
			}
			if got, want := slices.Sorted(maps.Keys(m)), slices.Sorted(slices.Values(tags)); !slices.Equal(got, want) {
				t.Errorf("marshaled keys %v, want %v", got, want)
			}
		})
	}
}

// TestCodecFieldTablesReadOwnField checks that each field table entry reads its own struct field:
// with only that field set, the entry reports it not empty and every other entry reports its
// field empty. The entry's decode uses the same accessor, so an entry wired to another field
// fails here. A struct field is skipped, because fillField leaves it zero.
func TestCodecFieldTablesReadOwnField(t *testing.T) {
	for _, c := range codecCases {
		t.Run(c.name, func(t *testing.T) {
			for i := range c.typ.NumField() {
				sf := c.typ.Field(i)
				key := jsonKey(sf)
				if key == "" || sf.Type.Kind() == reflect.Struct {
					continue
				}
				v := c.newValue()
				fillField(reflect.ValueOf(v).Elem().Field(i))
				for _, k := range c.keys {
					if got, want := c.empty(v, k), k != key; got != want {
						t.Errorf("with only %q set, %q empty = %v, want %v", key, k, got, want)
					}
				}
			}
		})
	}
}

// fillFields sets every exported field of the struct v with fillField.
func fillFields(v reflect.Value) {
	for i := range v.NumField() {
		fillField(v.Field(i))
	}
}

// fillField sets f, if it is exported, to a non-empty value: "s", true, 1, a one-element slice or
// map, a pointer to a zero value, or the raw JSON 1. A struct field is left zero; the marshalers
// always write one.
func fillField(f reflect.Value) {
	if !f.CanSet() {
		return
	}
	if f.Type() == reflect.TypeFor[json.RawMessage]() {
		f.SetBytes([]byte("1"))
		return
	}
	switch f.Kind() {
	case reflect.String:
		f.SetString("s")
	case reflect.Bool:
		f.SetBool(true)
	case reflect.Float64:
		f.SetFloat(1)
	case reflect.Slice:
		f.Set(reflect.MakeSlice(f.Type(), 1, 1))
	case reflect.Map:
		f.Set(reflect.MakeMap(f.Type()))
		f.SetMapIndex(reflect.ValueOf("k"), reflect.Zero(f.Type().Elem()))
	case reflect.Pointer:
		f.Set(reflect.New(f.Type().Elem()))
	}
}

// FuzzCodecRoundTrip checks every codec type (sel picks one) on any input that decodes: the
// output decodes again and marshals to the same bytes, and every key the decode kept raw (an
// unknown key, or a value it could not decode) is written back with the same value. It compares
// output with output: a typed field legitimately rewrites input such as null, 1E2 or "A".
func FuzzCodecRoundTrip(f *testing.F) {
	f.Add(byte(0), []byte(`{"id":"s","class":"sheet","title":"S","theme":{"k":1},"rootTopic":{"id":"r","title":"R",`+
		`"children":{"attached":[{"id":"a","title":"A","labels":["l"],"markers":[{"markerId":"m"}],`+
		`"notes":{"plain":{"content":"n"},"realHTML":{"content":"<div>n</div>"}}}]},`+
		`"summaries":[{"id":"s1","range":"(0,0)","topicId":"x"}],"boundaries":[{"id":"b","range":"master"}]},`+
		`"relationships":[{"id":"r1","end1Id":"a","end2Id":"r","controlPoints":{"0":{"x":1,"y":2}}}]}`))
	for i, c := range codecCases {
		f.Add(byte(i), []byte(`{}`))
		for _, key := range c.keys {
			f.Add(byte(i), []byte(fmt.Sprintf(`{%q:%s,"unknown":[1, 2]}`, key, wrongShapedValue(c.typ, key))))
		}
	}
	f.Fuzz(func(t *testing.T, sel byte, data []byte) {
		if len(data) > 1<<16 {
			t.Skip("nested decoding re-scans each subtree, so large inputs only slow the fuzzer")
		}
		c := codecCases[int(sel)%len(codecCases)]
		v1 := c.newValue()
		if json.Unmarshal(data, v1) != nil {
			return
		}
		out1 := marshalForFuzz(t, v1)
		v2 := c.newValue()
		if err := json.Unmarshal(out1, v2); err != nil {
			t.Fatalf("output does not decode: %v\n%s", err, out1)
		}
		if out2 := marshalForFuzz(t, v2); !bytes.Equal(out1, out2) {
			t.Fatalf("output is not stable:\n%s\n%s", out1, out2)
		}
		for key, raw := range c.extra(v1) {
			if got, want := compactJSON(t, jsonAt(t, out1, key)), compactJSON(t, raw); got != want {
				t.Errorf("%q written back as %s, want %s", key, got, want)
			}
		}
		if got, want := slices.Sorted(maps.Keys(c.extra(v2))), slices.Sorted(maps.Keys(c.extra(v1))); !slices.Equal(got, want) {
			t.Errorf("kept raw after a round trip: %v, want %v", got, want)
		}
	})
}

func marshalForFuzz(t *testing.T, v any) []byte {
	t.Helper()
	out, err := marshalJSONNoHTMLEscape(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}
