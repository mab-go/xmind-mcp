// Package xmind defines types for XMind content.json and zip I/O.
package xmind

import "encoding/json"

// Sheet is one tab/sheet in a workbook.
type Sheet struct {
	ID               string          `json:"id"`
	RevisionID       string          `json:"revisionId"`
	Class            string          `json:"class"`
	Title            string          `json:"title"`
	TopicOverlapping string          `json:"topicOverlapping,omitempty"`
	RootTopic        Topic           `json:"rootTopic"`
	Relationships    []Relationship  `json:"relationships,omitempty"`
	Extensions       json.RawMessage `json:"extensions,omitempty"`
	Theme            json.RawMessage `json:"theme,omitempty"`
	LabelSortOrder   string          `json:"labelSortOrder,omitempty"`
	extra            map[string]json.RawMessage
}

// Topic is a node in the mind map.
type Topic struct {
	ID            string `json:"id"`
	Class         string `json:"class,omitempty"`
	Title         string `json:"title,omitempty"`
	TitleUnedited bool   `json:"titleUnedited,omitempty"`
	// AttributedTitle holds the display-only label shown when the topic has a link href.
	// XMind writes this field; we preserve it on round-trip but never generate it ourselves.
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
	extra           map[string]json.RawMessage
}

// AttributedTitleItem is one run of text in an attributed (rich-link) topic title.
// XMind uses this to store the human-readable label for a topic that has an href.
// Treat as opaque: preserve on round-trip, never write from handler logic.
type AttributedTitleItem struct {
	Text  string `json:"text"`
	extra map[string]json.RawMessage
}

// Children holds attached, detached, and summary topics.
type Children struct {
	Attached []Topic `json:"attached,omitempty"`
	Detached []Topic `json:"detached,omitempty"`
	Summary  []Topic `json:"summary,omitempty"`
	extra    map[string]json.RawMessage
}

// Summary is a range descriptor on a parent topic (Topic.Summaries).
type Summary struct {
	ID      string `json:"id"`
	Range   string `json:"range"`
	TopicID string `json:"topicId"`
	extra   map[string]json.RawMessage
}

// Boundary groups children visually.
type Boundary struct {
	ID    string `json:"id"`
	Range string `json:"range"`
	Title string `json:"title,omitempty"`
	extra map[string]json.RawMessage
}

// Relationship connects two topics at sheet level.
type Relationship struct {
	ID            string              `json:"id"`
	End1ID        string              `json:"end1Id"`
	End2ID        string              `json:"end2Id"`
	Title         string              `json:"title,omitempty"`
	ControlPoints map[string]Position `json:"controlPoints,omitempty"`
	extra         map[string]json.RawMessage
}

// Marker is a topic marker reference.
type Marker struct {
	MarkerID string `json:"markerId"`
	extra    map[string]json.RawMessage
}

// TopicImage references embedded image resource.
type TopicImage struct {
	Src            string `json:"src"`
	IsMathJaxImage bool   `json:"isMathJaxImage,omitempty"`
	extra          map[string]json.RawMessage
}

// Notes holds plain and HTML note content.
// Either or both of Plain and RealHTML may be set; omitted keys are not reintroduced on marshal.
type Notes struct {
	Plain    *NoteContent `json:"plain,omitempty"`
	RealHTML *NoteContent `json:"realHTML,omitempty"`
}

// NoteContent wraps note body.
type NoteContent struct {
	Content string `json:"content"`
}

// Extension is task/audio/math metadata on a topic.
type Extension struct {
	Provider     string   `json:"provider"`
	Content      any      `json:"content,omitempty"`
	ResourceRefs []string `json:"resourceRefs,omitempty"`
}

// Position is used for floating (detached) topics.
type Position struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	extra map[string]json.RawMessage
}
