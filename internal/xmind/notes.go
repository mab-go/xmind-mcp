package xmind

import (
	"html"
	"strings"
)

// NotesFromPlainText builds a topic note from plain text, filling both the plain and the realHTML
// bodies. XMind needs both: with only plain, its rich-text panel is blank, and plain text written
// into realHTML renders unstyled and without paragraph structure. An empty text is no note, so it
// returns nil.
func NotesFromPlainText(text string) *Notes {
	if text == "" {
		return nil
	}
	return &Notes{
		Plain:    &NoteContent{Content: text},
		RealHTML: &NoteContent{Content: plainToRealHTML(text)},
	}
}

// plainToRealHTML converts a plain-text note string to minimal XMind-compatible HTML.
// It produces the same output XMind itself writes for a simple plain-text note:
// HTML-escaped content wrapped in <div> blocks, with newlines converted to </div><div>
// so each line is its own block element.
func plainToRealHTML(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString("<div>")
		} else {
			b.WriteString("</div><div>")
		}
		b.WriteString(html.EscapeString(line))
	}
	b.WriteString("</div>")
	return b.String()
}
