package handler

import (
	"encoding/json"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"
)

// Stable topic ID from kitchen-sink Sheet 1 (Mind Map): "Alpha" under Subtopic 1.
const kitchenSinkAlphaTopicID = "169d72af-6345-47ad-90b0-5b587f1f9619"

func firstKitchenSinkSheetID(t *testing.T) string {
	t.Helper()
	sheets, err := xmind.ReadMap(kitchenSinkPath(t))
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	if len(sheets) == 0 {
		t.Fatal("no sheets")
	}
	return sheets[0].ID
}

func TestGetSubtreeKitchenSinkRoot(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
	})
	if res.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, res))
	}
	var node subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, res)), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if node.Title != "Central Topic" {
		t.Fatalf("root title: got %q want Central Topic", node.Title)
	}
	if len(node.Children) < 1 {
		t.Fatalf("expected children on root, got %d", len(node.Children))
	}
}

func TestSearchTopicsKitchenSink(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"query":    "alpha",
	})
	if res.IsError {
		t.Fatalf("SearchTopics: %s", textContent(t, res))
	}
	var out searchTopicsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MatchCount < 1 {
		t.Fatalf("expected at least one match, got %d", out.MatchCount)
	}
}

func TestFindTopicKitchenSink(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"title":    "Alpha",
	})
	if res.IsError {
		t.Fatalf("FindTopic: %s", textContent(t, res))
	}
	var out findTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Title != "Alpha" {
		t.Fatalf("title: got %q", out.Title)
	}
	if out.ID == "" {
		t.Fatal("empty id")
	}
}

func TestGetSubtreeWithTopicID(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"topic_id": kitchenSinkAlphaTopicID,
	})
	if res.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, res))
	}
	var node subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, res)), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if node.Title != "Alpha" {
		t.Fatalf("root of subtree: got %q want Alpha", node.Title)
	}
	if node.ID != kitchenSinkAlphaTopicID {
		t.Fatalf("id: got %q", node.ID)
	}
}

func TestGetSubtreeNonexistentTopicID(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"topic_id": "00000000-0000-0000-0000-000000000000",
	})
	if !res.IsError {
		t.Fatal("expected tool error for missing topic_id")
	}
}

func TestGetSubtreeDepthLimit(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"depth":    float64(1),
	})
	if res.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, res))
	}
	var node subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, res)), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if node.Title != "Central Topic" {
		t.Fatalf("root title: %q", node.Title)
	}
	if len(node.Children) < 1 {
		t.Fatal("expected children at depth 1")
	}
	child := node.Children[0]
	if len(child.Children) != 0 {
		t.Fatalf("expected no grandchildren in output, got %d", len(child.Children))
	}
	if child.ChildrenCount < 1 {
		t.Fatalf("expected childrenCount on truncated node, got %d", child.ChildrenCount)
	}
}

func TestSearchTopicsNoMatches(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"query":    "ZZZNoMatchXYZ123",
	})
	if res.IsError {
		t.Fatalf("SearchTopics: %s", textContent(t, res))
	}
	var out searchTopicsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MatchCount != 0 || len(out.Matches) != 0 {
		t.Fatalf("expected no matches, got count=%d len=%d", out.MatchCount, len(out.Matches))
	}
}

func TestSearchTopicsResponseShape(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"query":    "alpha",
	})
	if res.IsError {
		t.Fatalf("SearchTopics: %s", textContent(t, res))
	}
	var out searchTopicsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var alpha *searchTopicItem
	for i := range out.Matches {
		if out.Matches[i].Title == "Alpha" {
			alpha = &out.Matches[i]
			break
		}
	}
	if alpha == nil {
		t.Fatalf("no Alpha in matches: %+v", out.Matches)
	}
	if alpha.ParentTitle != "Subtopic 1" {
		t.Fatalf("parentTitle: got %q want Subtopic 1", alpha.ParentTitle)
	}
	if alpha.Depth != 3 {
		t.Fatalf("depth: got %d want 3", alpha.Depth)
	}
}

func TestSearchTopicsAllSheets(t *testing.T) {
	h := NewXMindHandler()
	// "Central Topic" is the root on every sheet — should match across many sheets.
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":  kitchenSinkPath(t),
		"query": "Central Topic",
		// sheet_id intentionally omitted
	})
	if res.IsError {
		t.Fatalf("SearchTopics (all sheets): %s", textContent(t, res))
	}
	var out searchTopicsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// kitchen-sink has 15 sheets, each with a "Central Topic" root.
	if out.MatchCount < 15 {
		t.Fatalf("expected >=15 matches across all sheets, got %d", out.MatchCount)
	}
	// Every result should carry sheetId and sheetTitle.
	for i, m := range out.Matches {
		if m.SheetID == "" {
			t.Errorf("match[%d] missing sheetId", i)
		}
		if m.SheetTitle == "" {
			t.Errorf("match[%d] missing sheetTitle", i)
		}
	}
}

func TestSearchTopicsSingleSheetStillWorks(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"query":    "alpha",
	})
	if res.IsError {
		t.Fatalf("SearchTopics (single sheet): %s", textContent(t, res))
	}
	var out searchTopicsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MatchCount < 1 {
		t.Fatalf("expected at least one match, got %d", out.MatchCount)
	}
	// Single-sheet results must NOT carry sheetId/sheetTitle (omitempty).
	for i, m := range out.Matches {
		if m.SheetID != "" {
			t.Errorf("match[%d] unexpected sheetId in single-sheet response", i)
		}
	}
}

func TestSearchTopicsInvalidSheetID(t *testing.T) {
	h := NewXMindHandler()
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": "00000000-0000-0000-0000-000000000000",
		"query":    "alpha",
	})
	if !res.IsError {
		t.Fatal("expected tool error for unknown sheet_id")
	}
}

func TestSearchTopicsMissingQuery(t *testing.T) {
	h := NewXMindHandler()
	res := callTool(t, h.SearchTopics, map[string]any{
		"path": kitchenSinkPath(t),
	})
	if !res.IsError {
		t.Fatal("expected tool error for missing query")
	}
}

func TestSearchTopicsWrongTypeQuery(t *testing.T) {
	h := NewXMindHandler()
	res := callTool(t, h.SearchTopics, map[string]any{
		"path":  kitchenSinkPath(t),
		"query": 123,
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-string query")
	}
}

// Sheet 12 has floating (detached) topics as siblings of main branches under the root.
const kitchenSinkRelationshipsSheetID = "258a14a7-8ffb-4a09-8293-849af85c49e0"

func TestFindTopicIncludesDetachedSiblings(t *testing.T) {
	h := NewXMindHandler()
	res := callTool(t, h.FindTopic, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": kitchenSinkRelationshipsSheetID,
		"title":    "Main Topic 1",
	})
	if res.IsError {
		t.Fatalf("FindTopic: %s", textContent(t, res))
	}
	var out findTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var hasFloating bool
	for _, s := range out.SiblingTitles {
		if s == "Floating Topic" || s == "Floating Topic (2)" {
			hasFloating = true
			break
		}
	}
	if !hasFloating {
		t.Fatalf("expected detached floating topics in siblingTitles, got %#v", out.SiblingTitles)
	}
}

func TestFindTopicNotFound(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"title":    "DefinitelyMissingTitleXYZ123",
	})
	if !res.IsError {
		t.Fatal("expected tool error for missing topic")
	}
}

func TestGetSubtreeDepthNonInteger(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"depth":    1.5,
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-whole depth")
	}
}

func TestFindTopicEmptyTitle(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"title":    "",
	})
	if !res.IsError {
		t.Fatal("expected tool error for empty title")
	}
}

func TestGetSubtreeDepthZero(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"depth":    float64(0),
	})
	if res.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, res))
	}
	var node subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, res)), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if node.Children != nil {
		t.Fatalf("expected no children at depth 0, got %d", len(node.Children))
	}
	if node.ChildrenCount < 1 {
		t.Fatalf("expected childrenCount, got %d", node.ChildrenCount)
	}
}
