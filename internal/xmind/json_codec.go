package xmind

import (
	"bytes"
	"encoding/json"
)

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

func cloneJSONMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

func deleteKeys(m map[string]json.RawMessage, keys ...string) {
	for _, k := range keys {
		delete(m, k)
	}
}

func mergePreserved(base map[string]json.RawMessage, preserved map[string]json.RawMessage) {
	for k, v := range preserved {
		if _, ok := base[k]; !ok {
			base[k] = v
		}
	}
}

func jsonValueIsPresent(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

// marshalJSONNoHTMLEscape marshals v like json.Marshal but disables HTML-sensitive
// escaping of &, <, and > so content.json matches XMind's on-disk style (literal
// characters inside JSON strings, not \u0026 / \u003c / \u003e).
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

func encodeToRawMap(v any, preserved map[string]json.RawMessage) ([]byte, error) {
	data, err := marshalJSONNoHTMLEscape(v)
	if err != nil {
		return nil, err
	}
	if len(preserved) == 0 {
		return data, nil
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}
	mergePreserved(base, preserved)
	return marshalJSONNoHTMLEscape(base)
}

// --- Marker ---

// UnmarshalJSON implements json.Unmarshaler.
func (m *Marker) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "markerId")
	if v, ok := raw["markerId"]; ok {
		_ = json.Unmarshal(v, &m.MarkerID)
	}
	m.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m *Marker) MarshalJSON() ([]byte, error) {
	type alias Marker
	return encodeToRawMap((*alias)(m), m.extra)
}

// --- Summary (range descriptor) ---

// UnmarshalJSON implements json.Unmarshaler.
func (s *Summary) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "id", "range", "topicId")
	if v, ok := raw["id"]; ok {
		_ = json.Unmarshal(v, &s.ID)
	}
	if v, ok := raw["range"]; ok {
		_ = json.Unmarshal(v, &s.Range)
	}
	if v, ok := raw["topicId"]; ok {
		_ = json.Unmarshal(v, &s.TopicID)
	}
	s.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s *Summary) MarshalJSON() ([]byte, error) {
	type alias Summary
	return encodeToRawMap((*alias)(s), s.extra)
}

// --- Boundary ---

// UnmarshalJSON implements json.Unmarshaler.
func (b *Boundary) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "id", "range", "title")
	if v, ok := raw["id"]; ok {
		_ = json.Unmarshal(v, &b.ID)
	}
	if v, ok := raw["range"]; ok {
		_ = json.Unmarshal(v, &b.Range)
	}
	if v, ok := raw["title"]; ok {
		_ = json.Unmarshal(v, &b.Title)
	}
	b.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (b *Boundary) MarshalJSON() ([]byte, error) {
	type alias Boundary
	return encodeToRawMap((*alias)(b), b.extra)
}

// --- Relationship ---

// UnmarshalJSON implements json.Unmarshaler.
func (r *Relationship) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "id", "end1Id", "end2Id", "title", "controlPoints")
	if v, ok := raw["id"]; ok {
		_ = json.Unmarshal(v, &r.ID)
	}
	if v, ok := raw["end1Id"]; ok {
		_ = json.Unmarshal(v, &r.End1ID)
	}
	if v, ok := raw["end2Id"]; ok {
		_ = json.Unmarshal(v, &r.End2ID)
	}
	if v, ok := raw["title"]; ok {
		_ = json.Unmarshal(v, &r.Title)
	}
	if v, ok := raw["controlPoints"]; ok {
		var cp map[string]json.RawMessage
		if err := json.Unmarshal(v, &cp); err != nil {
			return err
		}
		if len(cp) > 0 {
			r.ControlPoints = make(map[string]Position, len(cp))
			for k, rv := range cp {
				var p Position
				if err := json.Unmarshal(rv, &p); err != nil {
					return err
				}
				r.ControlPoints[k] = p
			}
		}
	}
	r.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (r *Relationship) MarshalJSON() ([]byte, error) {
	type alias Relationship
	return encodeToRawMap((*alias)(r), r.extra)
}

// --- Position ---

// UnmarshalJSON implements json.Unmarshaler.
func (p *Position) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "x", "y")
	if v, ok := raw["x"]; ok {
		_ = json.Unmarshal(v, &p.X)
	}
	if v, ok := raw["y"]; ok {
		_ = json.Unmarshal(v, &p.Y)
	}
	p.extra = ex
	return nil
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
	return encodeToRawMap(aux, p.extra)
}

// --- TopicImage ---

// UnmarshalJSON implements json.Unmarshaler.
func (im *TopicImage) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "src", "isMathJaxImage")
	if v, ok := raw["src"]; ok {
		_ = json.Unmarshal(v, &im.Src)
	}
	if v, ok := raw["isMathJaxImage"]; ok {
		_ = json.Unmarshal(v, &im.IsMathJaxImage)
	}
	im.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (im *TopicImage) MarshalJSON() ([]byte, error) {
	type alias TopicImage
	return encodeToRawMap((*alias)(im), im.extra)
}

// --- AttributedTitleItem ---

// UnmarshalJSON implements json.Unmarshaler.
func (a *AttributedTitleItem) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "text")
	if v, ok := raw["text"]; ok {
		_ = json.Unmarshal(v, &a.Text)
	}
	a.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (a *AttributedTitleItem) MarshalJSON() ([]byte, error) {
	type alias AttributedTitleItem
	return encodeToRawMap((*alias)(a), a.extra)
}

// --- Children ---

// UnmarshalJSON implements json.Unmarshaler.
func (c *Children) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, "attached", "detached", "summary")
	if v, ok := raw["attached"]; ok {
		_ = json.Unmarshal(v, &c.Attached)
	}
	if v, ok := raw["detached"]; ok {
		_ = json.Unmarshal(v, &c.Detached)
	}
	if v, ok := raw["summary"]; ok {
		_ = json.Unmarshal(v, &c.Summary)
	}
	c.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (c *Children) MarshalJSON() ([]byte, error) {
	type alias Children
	return encodeToRawMap((*alias)(c), c.extra)
}

var topicKnownKeys = []string{
	"id", "class", "title", "titleUnedited", "attributedTitle", "structureClass",
	"labels", "markers", "href", "image", "notes", "children", "boundaries", "extensions", "summaries", "position",
}

// --- Topic ---

// UnmarshalJSON implements json.Unmarshaler.
func (t *Topic) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, topicKnownKeys...)
	if v, ok := raw["id"]; ok {
		_ = json.Unmarshal(v, &t.ID)
	}
	if v, ok := raw["class"]; ok {
		_ = json.Unmarshal(v, &t.Class)
	}
	if v, ok := raw["title"]; ok {
		_ = json.Unmarshal(v, &t.Title)
	}
	if v, ok := raw["titleUnedited"]; ok {
		_ = json.Unmarshal(v, &t.TitleUnedited)
	}
	if v, ok := raw["attributedTitle"]; ok {
		_ = json.Unmarshal(v, &t.AttributedTitle)
	}
	if v, ok := raw["structureClass"]; ok {
		_ = json.Unmarshal(v, &t.StructureClass)
	}
	if v, ok := raw["labels"]; ok {
		_ = json.Unmarshal(v, &t.Labels)
	}
	if v, ok := raw["markers"]; ok {
		_ = json.Unmarshal(v, &t.Markers)
	}
	if v, ok := raw["href"]; ok {
		_ = json.Unmarshal(v, &t.Href)
	}
	if v, ok := raw["image"]; ok && jsonValueIsPresent(v) {
		var img TopicImage
		if err := json.Unmarshal(v, &img); err != nil {
			return err
		}
		t.Image = &img
	}
	if v, ok := raw["notes"]; ok && jsonValueIsPresent(v) {
		var n Notes
		if err := json.Unmarshal(v, &n); err != nil {
			return err
		}
		t.Notes = &n
	}
	if v, ok := raw["children"]; ok && jsonValueIsPresent(v) {
		var ch Children
		if err := json.Unmarshal(v, &ch); err != nil {
			return err
		}
		t.Children = &ch
	}
	if v, ok := raw["boundaries"]; ok {
		_ = json.Unmarshal(v, &t.Boundaries)
	}
	if v, ok := raw["extensions"]; ok {
		_ = json.Unmarshal(v, &t.Extensions)
	}
	if v, ok := raw["summaries"]; ok {
		_ = json.Unmarshal(v, &t.Summaries)
	}
	if v, ok := raw["position"]; ok && jsonValueIsPresent(v) {
		var pos Position
		if err := json.Unmarshal(v, &pos); err != nil {
			return err
		}
		t.Position = &pos
	}
	t.extra = ex
	return nil
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
	return encodeToRawMap(aux, t.extra)
}

var sheetKnownKeys = []string{
	"id", "revisionId", "class", "title", "topicOverlapping", "rootTopic",
	"relationships", "extensions", "theme", "labelSortOrder",
}

// --- Sheet ---

// UnmarshalJSON implements json.Unmarshaler.
func (s *Sheet) UnmarshalJSON(data []byte) error {
	raw, err := unmarshalObjectMap(data)
	if err != nil {
		return err
	}
	ex := cloneJSONMap(raw)
	deleteKeys(ex, sheetKnownKeys...)
	if v, ok := raw["id"]; ok {
		_ = json.Unmarshal(v, &s.ID)
	}
	if v, ok := raw["revisionId"]; ok {
		_ = json.Unmarshal(v, &s.RevisionID)
	}
	if v, ok := raw["class"]; ok {
		_ = json.Unmarshal(v, &s.Class)
	}
	if v, ok := raw["title"]; ok {
		_ = json.Unmarshal(v, &s.Title)
	}
	if v, ok := raw["topicOverlapping"]; ok {
		_ = json.Unmarshal(v, &s.TopicOverlapping)
	}
	if v, ok := raw["rootTopic"]; ok {
		if err := json.Unmarshal(v, &s.RootTopic); err != nil {
			return err
		}
	}
	if v, ok := raw["relationships"]; ok {
		_ = json.Unmarshal(v, &s.Relationships)
	}
	if v, ok := raw["extensions"]; ok {
		s.Extensions = append(json.RawMessage(nil), v...)
	}
	if v, ok := raw["theme"]; ok {
		s.Theme = append(json.RawMessage(nil), v...)
	}
	if v, ok := raw["labelSortOrder"]; ok {
		_ = json.Unmarshal(v, &s.LabelSortOrder)
	}
	s.extra = ex
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s *Sheet) MarshalJSON() ([]byte, error) {
	aux := struct {
		ID               string          `json:"id"`
		RevisionID       string          `json:"revisionId,omitempty"`
		Class            string          `json:"class"`
		Title            string          `json:"title"`
		TopicOverlapping string          `json:"topicOverlapping,omitempty"`
		RootTopic        Topic           `json:"rootTopic"`
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
		RootTopic:        s.RootTopic,
		Relationships:    s.Relationships,
		Extensions:       s.Extensions,
		Theme:            s.Theme,
		LabelSortOrder:   s.LabelSortOrder,
	}
	return encodeToRawMap(aux, s.extra)
}
