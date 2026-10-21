package xmind

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// ErrUndecodableField is returned by MapWriter.Commit and WriteMap when the map holds a value
// that could not be decoded in a field the tools edit. The value is kept raw, so the map still
// reads, but a save could overwrite or drop it, so none is allowed.
var ErrUndecodableField = errors.New("has an unexpected shape")

// fieldPolicy says what happens when a known key's value can't be decoded into its typed field.
// Under every policy the raw value stays in the object's extra map and the typed field stays
// empty, so the map still reads.
type fieldPolicy int

const (
	// strict marks a field some handler writes on an existing object. A save could overwrite
	// or drop the raw value, so checkSavable refuses every save while the map holds one.
	strict fieldPolicy = iota + 1
	// keepRaw marks a field no handler writes. The raw value is written back unchanged.
	keepRaw
	// passthrough marks a field kept as raw JSON and never decoded, so it can't fail.
	passthrough
)

// codecField is one known key of the codec type T: its policy, how to decode a value into T's
// field, and whether that field is empty.
type codecField[T any] struct {
	key    string
	policy fieldPolicy
	decode func(*T, json.RawMessage) error
	empty  func(*T) bool
}

// typed returns a field that decodes into *get(x). It decodes into a fresh value and assigns it
// only on success, because json.Unmarshal partly fills slices and pointers when it fails. It
// calls json.Unmarshal rather than a type's UnmarshalJSON, which would make the field tables an
// initialization cycle.
func typed[T, F any](key string, policy fieldPolicy, get func(*T) *F) codecField[T] {
	return codecField[T]{
		key:    key,
		policy: policy,
		decode: func(x *T, raw json.RawMessage) error {
			var v F
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			*get(x) = v
			return nil
		},
		empty: func(x *T) bool { return isEmptyValue(reflect.ValueOf(get(x)).Elem()) },
	}
}

// rawField returns a passthrough field: its value is copied into *get(x) as raw JSON.
func rawField[T any](key string, get func(*T) *json.RawMessage) codecField[T] {
	return codecField[T]{
		key:    key,
		policy: passthrough,
		decode: func(x *T, raw json.RawMessage) error {
			*get(x) = append(json.RawMessage(nil), raw...)
			return nil
		},
		empty: func(x *T) bool { return len(*get(x)) == 0 },
	}
}

// isEmptyValue reports whether v is empty as encoding/json's omitempty judges it, or, for a
// struct, whether it is the zero value.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	default:
		return v.IsZero()
	}
}

// fieldByKey returns the field in fields with the given key.
func fieldByKey[T any](fields []codecField[T], key string) (codecField[T], bool) {
	for _, f := range fields {
		if f.key == key {
			return f, true
		}
	}
	return codecField[T]{}, false
}

func unmarshalObjectMap(data []byte) (map[string]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]json.RawMessage{}, nil
	}
	return m, nil
}

// decodeObject decodes the JSON object data into x's fields and returns x's extra map: the
// unknown keys and the known keys whose values could not be decoded, or nil when there are
// none. Input that is not an object or null is an error.
func decodeObject[T any](x *T, data []byte, fields []codecField[T]) (map[string]json.RawMessage, error) {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		if v, ok := raw[f.key]; ok && f.decode(x, v) == nil {
			delete(raw, f.key)
		}
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}

// marshalJSONNoHTMLEscape marshals v like json.Marshal but disables HTML-sensitive
// escaping of &, <, and > so content.json matches XMind's on-disk style (literal
// characters inside JSON strings, not & / < / >).
func marshalJSONNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

// encodeToRawMap marshals v, the typed fields of x, then writes each of x's extra values over
// the result, except a known field whose typed value is not empty: a handler set it, and its
// value wins. A known key is in extra only when its value could not be decoded, and its raw
// value replaces the zero value the encoder writes for a field without omitempty (Topic
// "id":"", Position "x":0). Unknown keys never collide with typed ones.
func encodeToRawMap[T any](v any, x *T, extra map[string]json.RawMessage, fields []codecField[T]) ([]byte, error) {
	data, err := marshalJSONNoHTMLEscape(v)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return data, nil
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}
	for k, raw := range extra {
		if f, known := fieldByKey(fields, k); known && !f.empty(x) {
			continue
		}
		base[k] = raw
	}
	return marshalJSONNoHTMLEscape(base)
}

// --- Field tables, in struct order ---

var markerFields = []codecField[Marker]{
	typed("markerId", keepRaw, func(m *Marker) *string { return &m.MarkerID }),
}

var summaryFields = []codecField[Summary]{
	typed("id", strict, func(s *Summary) *string { return &s.ID }),
	typed("range", strict, func(s *Summary) *string { return &s.Range }),
	typed("topicId", strict, func(s *Summary) *string { return &s.TopicID }),
}

var boundaryFields = []codecField[Boundary]{
	typed("id", strict, func(b *Boundary) *string { return &b.ID }),
	typed("range", strict, func(b *Boundary) *string { return &b.Range }),
	typed("title", keepRaw, func(b *Boundary) *string { return &b.Title }),
}

var relationshipFields = []codecField[Relationship]{
	typed("id", keepRaw, func(r *Relationship) *string { return &r.ID }),
	typed("end1Id", keepRaw, func(r *Relationship) *string { return &r.End1ID }),
	typed("end2Id", keepRaw, func(r *Relationship) *string { return &r.End2ID }),
	typed("title", keepRaw, func(r *Relationship) *string { return &r.Title }),
	typed("controlPoints", keepRaw, func(r *Relationship) *map[string]Position { return &r.ControlPoints }),
}

var positionFields = []codecField[Position]{
	typed("x", keepRaw, func(p *Position) *float64 { return &p.X }),
	typed("y", keepRaw, func(p *Position) *float64 { return &p.Y }),
}

var topicImageFields = []codecField[TopicImage]{
	typed("src", keepRaw, func(im *TopicImage) *string { return &im.Src }),
	typed("isMathJaxImage", keepRaw, func(im *TopicImage) *bool { return &im.IsMathJaxImage }),
}

var attributedTitleItemFields = []codecField[AttributedTitleItem]{
	typed("text", keepRaw, func(a *AttributedTitleItem) *string { return &a.Text }),
}

var noteContentFields = []codecField[NoteContent]{
	typed("content", keepRaw, func(n *NoteContent) *string { return &n.Content }),
}

var notesFields = []codecField[Notes]{
	typed("plain", keepRaw, func(n *Notes) **NoteContent { return &n.Plain }),
	typed("realHTML", keepRaw, func(n *Notes) **NoteContent { return &n.RealHTML }),
}

var childrenFields = []codecField[Children]{
	typed("attached", strict, func(c *Children) *[]Topic { return &c.Attached }),
	typed("detached", strict, func(c *Children) *[]Topic { return &c.Detached }),
	typed("summary", strict, func(c *Children) *[]Topic { return &c.Summary }),
}

var topicFields = []codecField[Topic]{
	typed("id", strict, func(t *Topic) *string { return &t.ID }),
	typed("class", keepRaw, func(t *Topic) *string { return &t.Class }),
	typed("title", strict, func(t *Topic) *string { return &t.Title }),
	typed("titleUnedited", strict, func(t *Topic) *bool { return &t.TitleUnedited }),
	typed("attributedTitle", keepRaw, func(t *Topic) *[]AttributedTitleItem { return &t.AttributedTitle }),
	typed("structureClass", keepRaw, func(t *Topic) *string { return &t.StructureClass }),
	typed("labels", strict, func(t *Topic) *[]string { return &t.Labels }),
	typed("markers", strict, func(t *Topic) *[]Marker { return &t.Markers }),
	typed("href", strict, func(t *Topic) *string { return &t.Href }),
	typed("image", keepRaw, func(t *Topic) **TopicImage { return &t.Image }),
	typed("notes", strict, func(t *Topic) **Notes { return &t.Notes }),
	typed("children", strict, func(t *Topic) **Children { return &t.Children }),
	typed("boundaries", strict, func(t *Topic) *[]Boundary { return &t.Boundaries }),
	typed("extensions", keepRaw, func(t *Topic) *[]Extension { return &t.Extensions }),
	typed("summaries", strict, func(t *Topic) *[]Summary { return &t.Summaries }),
	typed("position", keepRaw, func(t *Topic) **Position { return &t.Position }),
}

var sheetFields = []codecField[Sheet]{
	typed("id", keepRaw, func(s *Sheet) *string { return &s.ID }),
	typed("revisionId", strict, func(s *Sheet) *string { return &s.RevisionID }),
	typed("class", keepRaw, func(s *Sheet) *string { return &s.Class }),
	typed("title", keepRaw, func(s *Sheet) *string { return &s.Title }),
	typed("topicOverlapping", keepRaw, func(s *Sheet) *string { return &s.TopicOverlapping }),
	typed("rootTopic", strict, func(s *Sheet) *Topic { return &s.RootTopic }),
	typed("relationships", strict, func(s *Sheet) *[]Relationship { return &s.Relationships }),
	rawField("extensions", func(s *Sheet) *json.RawMessage { return &s.Extensions }),
	rawField("theme", func(s *Sheet) *json.RawMessage { return &s.Theme }),
	typed("labelSortOrder", keepRaw, func(s *Sheet) *string { return &s.LabelSortOrder }),
}

// --- Marker ---

// UnmarshalJSON implements json.Unmarshaler.
func (m *Marker) UnmarshalJSON(data []byte) (err error) {
	m.extra, err = decodeObject(m, data, markerFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (m *Marker) MarshalJSON() ([]byte, error) {
	type alias Marker
	return encodeToRawMap((*alias)(m), m, m.extra, markerFields)
}

// WithoutRawFields returns a copy of m without the known keys whose values could not be decoded,
// keeping its unknown keys. Tool responses use it: their declared schema needs a string
// markerId, so a raw one reads as empty there, while the map itself keeps the raw value.
func (m *Marker) WithoutRawFields() Marker {
	out := Marker{MarkerID: m.MarkerID}
	for k, v := range m.extra {
		if _, known := fieldByKey(markerFields, k); !known {
			if out.extra == nil {
				out.extra = map[string]json.RawMessage{}
			}
			out.extra[k] = v
		}
	}
	return out
}

// --- Summary (range descriptor) ---

// UnmarshalJSON implements json.Unmarshaler.
func (s *Summary) UnmarshalJSON(data []byte) (err error) {
	s.extra, err = decodeObject(s, data, summaryFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (s *Summary) MarshalJSON() ([]byte, error) {
	type alias Summary
	return encodeToRawMap((*alias)(s), s, s.extra, summaryFields)
}

// --- Boundary ---

// UnmarshalJSON implements json.Unmarshaler.
func (b *Boundary) UnmarshalJSON(data []byte) (err error) {
	b.extra, err = decodeObject(b, data, boundaryFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (b *Boundary) MarshalJSON() ([]byte, error) {
	type alias Boundary
	return encodeToRawMap((*alias)(b), b, b.extra, boundaryFields)
}

// --- Relationship ---

// UnmarshalJSON implements json.Unmarshaler.
func (r *Relationship) UnmarshalJSON(data []byte) (err error) {
	r.extra, err = decodeObject(r, data, relationshipFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (r *Relationship) MarshalJSON() ([]byte, error) {
	type alias Relationship
	return encodeToRawMap((*alias)(r), r, r.extra, relationshipFields)
}

// --- Position ---

// UnmarshalJSON implements json.Unmarshaler.
func (p *Position) UnmarshalJSON(data []byte) (err error) {
	p.extra, err = decodeObject(p, data, positionFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (p *Position) MarshalJSON() ([]byte, error) {
	aux := struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}{
		X: p.X,
		Y: p.Y,
	}
	return encodeToRawMap(aux, p, p.extra, positionFields)
}

// --- TopicImage ---

// UnmarshalJSON implements json.Unmarshaler.
func (im *TopicImage) UnmarshalJSON(data []byte) (err error) {
	im.extra, err = decodeObject(im, data, topicImageFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (im *TopicImage) MarshalJSON() ([]byte, error) {
	type alias TopicImage
	return encodeToRawMap((*alias)(im), im, im.extra, topicImageFields)
}

// --- AttributedTitleItem ---

// UnmarshalJSON implements json.Unmarshaler.
func (a *AttributedTitleItem) UnmarshalJSON(data []byte) (err error) {
	a.extra, err = decodeObject(a, data, attributedTitleItemFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (a *AttributedTitleItem) MarshalJSON() ([]byte, error) {
	type alias AttributedTitleItem
	return encodeToRawMap((*alias)(a), a, a.extra, attributedTitleItemFields)
}

// --- NoteContent ---

// UnmarshalJSON implements json.Unmarshaler.
func (n *NoteContent) UnmarshalJSON(data []byte) (err error) {
	n.extra, err = decodeObject(n, data, noteContentFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (n *NoteContent) MarshalJSON() ([]byte, error) {
	type alias NoteContent
	return encodeToRawMap((*alias)(n), n, n.extra, noteContentFields)
}

// --- Notes ---

// UnmarshalJSON implements json.Unmarshaler.
// Plain and RealHTML are only populated when their keys are present (null leaves them nil), so a
// plain-only note does not gain an empty realHTML on round-trip.
func (n *Notes) UnmarshalJSON(data []byte) (err error) {
	n.extra, err = decodeObject(n, data, notesFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (n *Notes) MarshalJSON() ([]byte, error) {
	type alias Notes
	return encodeToRawMap((*alias)(n), n, n.extra, notesFields)
}

// --- Children ---

// UnmarshalJSON implements json.Unmarshaler.
func (c *Children) UnmarshalJSON(data []byte) (err error) {
	c.extra, err = decodeObject(c, data, childrenFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (c *Children) MarshalJSON() ([]byte, error) {
	type alias Children
	return encodeToRawMap((*alias)(c), c, c.extra, childrenFields)
}

// --- Topic ---

// UnmarshalJSON implements json.Unmarshaler.
func (t *Topic) UnmarshalJSON(data []byte) (err error) {
	t.extra, err = decodeObject(t, data, topicFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (t *Topic) MarshalJSON() ([]byte, error) {
	aux := struct {
		ID              string                `json:"id"`
		Class           string                `json:"class,omitempty"`
		Title           string                `json:"title,omitempty"`
		TitleUnedited   bool                  `json:"titleUnedited,omitempty"`
		AttributedTitle []AttributedTitleItem `json:"attributedTitle,omitempty"`
		StructureClass  string                `json:"structureClass,omitempty"`
		Labels          []string              `json:"labels,omitempty"`
		Markers         []Marker              `json:"markers,omitempty"`
		Href            string                `json:"href,omitempty"`
		Image           *TopicImage           `json:"image,omitempty"`
		Notes           *Notes                `json:"notes,omitempty"`
		Children        *Children             `json:"children,omitempty"`
		Boundaries      []Boundary            `json:"boundaries,omitempty"`
		Extensions      []Extension           `json:"extensions,omitempty"`
		Summaries       []Summary             `json:"summaries,omitempty"`
		Position        *Position             `json:"position,omitempty"`
	}{
		ID:              t.ID,
		Class:           t.Class,
		Title:           t.Title,
		TitleUnedited:   t.TitleUnedited,
		AttributedTitle: t.AttributedTitle,
		StructureClass:  t.StructureClass,
		Labels:          t.Labels,
		Markers:         t.Markers,
		Href:            t.Href,
		Image:           t.Image,
		Notes:           t.Notes,
		Children:        t.Children,
		Boundaries:      t.Boundaries,
		Extensions:      t.Extensions,
		Summaries:       t.Summaries,
		Position:        t.Position,
	}
	return encodeToRawMap(aux, t, t.extra, topicFields)
}

// --- Sheet ---

// UnmarshalJSON implements json.Unmarshaler.
func (s *Sheet) UnmarshalJSON(data []byte) (err error) {
	s.extra, err = decodeObject(s, data, sheetFields)
	return err
}

// MarshalJSON implements json.Marshaler.
func (s *Sheet) MarshalJSON() ([]byte, error) {
	aux := struct {
		ID               string          `json:"id"`
		RevisionID       string          `json:"revisionId,omitempty"`
		Class            string          `json:"class"`
		Title            string          `json:"title"`
		TopicOverlapping string          `json:"topicOverlapping,omitempty"`
		RootTopic        *Topic          `json:"rootTopic"`
		Relationships    []Relationship  `json:"relationships,omitempty"`
		Extensions       json.RawMessage `json:"extensions,omitempty"`
		Theme            json.RawMessage `json:"theme,omitempty"`
		LabelSortOrder   string          `json:"labelSortOrder,omitempty"`
	}{
		ID:               s.ID,
		RevisionID:       s.RevisionID,
		Class:            s.Class,
		Title:            s.Title,
		TopicOverlapping: s.TopicOverlapping,
		RootTopic:        &s.RootTopic,
		Relationships:    s.Relationships,
		Extensions:       s.Extensions,
		Theme:            s.Theme,
		LabelSortOrder:   s.LabelSortOrder,
	}
	return encodeToRawMap(aux, s, s.extra, sheetFields)
}

// --- Save check ---

// checkSavable returns an error wrapping ErrUndecodableField for the first field in sheets that
// a handler may write and that holds a value which could not be decoded. The error names the
// field and where it is. A save could overwrite or drop such a value, so MapWriter.Commit and
// writeMapBody refuse to save.
func checkSavable(sheets []Sheet) error {
	for i := range sheets {
		if err := checkSheet(&sheets[i], i); err != nil {
			return err
		}
	}
	return nil
}

// checkSheet checks one sheet, the i'th, and its topic tree.
func checkSheet(sh *Sheet, i int) error {
	where := fmt.Sprintf("in sheet %d", i+1)
	if sh.ID != "" {
		where = "in sheet " + sh.ID
	}
	if key, ok := firstStrictKey(sh.extra, sheetFields); ok {
		return undecodableField("Sheet", key, where)
	}
	return checkTopic(&sh.RootTopic, where)
}

// checkTopic checks t and its subtree. parent locates t when it has no id: "in sheet …" for a
// root, "under topic …" for a child.
func checkTopic(t *Topic, parent string) error {
	where, under := parent, parent
	if t.ID != "" {
		where, under = "on topic "+t.ID, "under topic "+t.ID
	}
	if key, ok := firstStrictKey(t.extra, topicFields); ok {
		return undecodableField("Topic", key, where)
	}
	if err := checkRanges(t, where); err != nil {
		return err
	}
	return checkChildren(t.Children, where, under)
}

// checkRanges checks the summary and boundary descriptors of the topic at where.
func checkRanges(t *Topic, where string) error {
	for i := range t.Summaries {
		if key, ok := firstStrictKey(t.Summaries[i].extra, summaryFields); ok {
			return undecodableField("Summary", key, where)
		}
	}
	for i := range t.Boundaries {
		if key, ok := firstStrictKey(t.Boundaries[i].extra, boundaryFields); ok {
			return undecodableField("Boundary", key, where)
		}
	}
	return nil
}

// checkChildren checks the child lists of the topic at where, and each child's subtree; under
// locates a child without an id.
func checkChildren(c *Children, where, under string) error {
	if c == nil {
		return nil
	}
	if key, ok := firstStrictKey(c.extra, childrenFields); ok {
		return undecodableField("Children", key, where)
	}
	for _, list := range [][]Topic{c.Attached, c.Detached, c.Summary} {
		for i := range list {
			if err := checkTopic(&list[i], under); err != nil {
				return err
			}
		}
	}
	return nil
}

// firstStrictKey returns the first key, in table order, that extra holds and fields marks strict.
func firstStrictKey[T any](extra map[string]json.RawMessage, fields []codecField[T]) (string, bool) {
	for _, f := range fields {
		if _, ok := extra[f.key]; ok && f.policy == strict {
			return f.key, true
		}
	}
	return "", false
}

func undecodableField(typeName, key, where string) error {
	return fmt.Errorf("%s.%s %s %w", typeName, key, where, ErrUndecodableField)
}
