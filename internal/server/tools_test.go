package server

import (
	"context"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/server/handler"
)

// expectedAnnotations lists the title and behavior hints every registered tool must declare.
// Values are literal (not derived from withAnnotations) so a misclassified tool fails here.
// openWorldHint is false for every tool and is checked separately.
var expectedAnnotations = map[string]struct {
	title       string
	readOnly    bool
	destructive bool
	idempotent  bool
}{
	"xmind_open_map":                  {"Open Map", true, false, true},
	"xmind_list_sheets":               {"List Sheets", true, false, true},
	"xmind_create_map":                {"Create Map", false, false, true},
	"xmind_add_sheet":                 {"Add Sheet", false, false, false},
	"xmind_delete_sheet":              {"Delete Sheet", false, true, true},
	"xmind_get_subtree":               {"Get Subtree", true, false, true},
	"xmind_get_topic_properties":      {"Get Topic Properties", true, false, true},
	"xmind_search_topics":             {"Search Topics", true, false, true},
	"xmind_find_topic":                {"Find Topic", true, false, true},
	"xmind_add_topic":                 {"Add Topic", false, false, false},
	"xmind_add_topics_bulk":           {"Add Topics (Bulk)", false, false, false},
	"xmind_duplicate_topic":           {"Duplicate Topic", false, false, false},
	"xmind_rename_topic":              {"Rename Topic", false, true, true},
	"xmind_delete_topic":              {"Delete Topic", false, true, true},
	"xmind_move_topic":                {"Move Topic", false, true, true},
	"xmind_reorder_children":          {"Reorder Children", false, true, true},
	"xmind_set_topic_properties":      {"Set Topic Properties", false, true, true},
	"xmind_set_topic_properties_bulk": {"Set Topic Properties (Bulk)", false, true, true},
	"xmind_add_floating_topic":        {"Add Floating Topic", false, false, false},
	"xmind_add_relationship":          {"Add Relationship", false, false, false},
	"xmind_list_relationships":        {"List Relationships", true, false, true},
	"xmind_delete_relationship":       {"Delete Relationship", false, true, true},
	"xmind_add_summary":               {"Add Summary", false, false, false},
	"xmind_add_boundary":              {"Add Boundary", false, false, false},
	"xmind_flatten_to_outline":        {"Flatten to Outline", true, false, true},
	"xmind_import_from_outline":       {"Import from Outline", false, false, false},
	"xmind_find_and_replace":          {"Find and Replace", false, true, false},
}

func TestRegisteredToolAnnotations(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())
	tools := s.ListTools()

	if len(tools) != len(expectedAnnotations) {
		t.Fatalf("registered %d tools, expectedAnnotations lists %d", len(tools), len(expectedAnnotations))
	}

	for name, st := range tools {
		t.Run(name, func(t *testing.T) {
			want, ok := expectedAnnotations[name]
			if !ok {
				t.Fatalf("tool %q has no entry in expectedAnnotations", name)
			}

			a := st.Tool.Annotations
			if a.Title != want.title {
				t.Errorf("title = %q, want %q", a.Title, want.title)
			}
			checkHint(t, "readOnlyHint", a.ReadOnlyHint, want.readOnly)
			checkHint(t, "destructiveHint", a.DestructiveHint, want.destructive)
			checkHint(t, "idempotentHint", a.IdempotentHint, want.idempotent)
			checkHint(t, "openWorldHint", a.OpenWorldHint, false)
		})
	}
}

func checkHint(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Errorf("%s is unset, want %v", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %v, want %v", name, *got, want)
	}
}

func TestWithAnnotationsPanicsOnUnknownEffect(t *testing.T) {
	for name, effect := range map[string]toolEffect{
		"zero value":   toolEffect(0),
		"past the end": effectDestructiveIdempotent + 1,
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("withAnnotations(%d) did not panic", effect)
				}
			}()
			withAnnotations("Bad Tool", effect)
		})
	}
}
