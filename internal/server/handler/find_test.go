package handler

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"
)

// Stable topic ID from kitchen-sink Sheet 1 (Mind Map): "Alpha" under Subtopic 1.
const kitchenSinkAlphaTopicID = "169d72af-6345-47ad-90b0-5b587f1f9619"

// Parent of "Alpha" on Sheet 1 - Mind Map.
const kitchenSinkSubtopic1TopicID = "61cc4754-20ec-4479-9e58-f7eaa985520a"

// Ancestry path from sheet root to parent of "Alpha" on Sheet 1 - Mind Map.
var kitchenSinkAlphaAncestryPath = []string{"Central Topic", "Main Topic 1", "Subtopic 1"}

const kitchenSinkSheet10Title = "Sheet 10 - Topic Properties"

// Topic IDs from internal/xmind/reader_test.go (Sheet 10 — Topic Properties).
const (
	kitchenSinkSheet10NoteTopicID = "83785e34-fcdb-4049-8c42-15052c20d8d6"
	kitchenSinkSheet10HrefTopicID = "e0d8096f-cc1a-4c33-bcc5-db9006481f85"
)

func kitchenSinkSheetIDByTitle(t *testing.T, title string) string {
	t.Helper()
	sheets, err := xmind.ReadMap(kitchenSinkPath(t))
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	for i := range sheets {
		if sheets[i].Title == title {
			return sheets[i].ID
		}
	}
	t.Fatalf("sheet not found: %s", title)
	return ""
}

func assertSubtreeNoNotesField(t *testing.T, n *subtreeNode) {
	t.Helper()
	if n.Notes != "" {
		t.Fatalf("expected no notes on node id %q, got %q", n.ID, n.Notes)
	}
	for _, c := range n.Children {
		assertSubtreeNoNotesField(t, c)
	}
}

func assertSubtreeNoHrefField(t *testing.T, n *subtreeNode) {
	t.Helper()
	if n.Href != "" {
		t.Fatalf("expected no href on node id %q, got %q", n.ID, n.Href)
	}
	for _, c := range n.Children {
		assertSubtreeNoHrefField(t, c)
	}
}

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
	if out.ID != kitchenSinkAlphaTopicID {
		t.Fatalf("id: got %q want %q", out.ID, kitchenSinkAlphaTopicID)
	}
	if !slices.Equal(out.AncestryPath, kitchenSinkAlphaAncestryPath) {
		t.Fatalf("ancestryPath: got %#v want %#v", out.AncestryPath, kitchenSinkAlphaAncestryPath)
	}
	if out.ParentTitle != kitchenSinkAlphaAncestryPath[len(kitchenSinkAlphaAncestryPath)-1] {
		t.Fatalf("parentTitle should equal last ancestry segment: got %q", out.ParentTitle)
	}
}

func TestFindTopicAncestryPathScopedStillAbsolute(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Alpha",
		"parent_id": kitchenSinkSubtopic1TopicID,
	})
	if res.IsError {
		t.Fatalf("FindTopic: %s", textContent(t, res))
	}
	var out findTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Equal(out.AncestryPath, kitchenSinkAlphaAncestryPath) {
		t.Fatalf("scoped search must still return sheet-root ancestry: got %#v want %#v", out.AncestryPath, kitchenSinkAlphaAncestryPath)
	}
}

func TestSearchTopicsAncestryPath(t *testing.T) {
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
		t.Fatalf("no Alpha in matches")
	}
	if !slices.Equal(alpha.AncestryPath, kitchenSinkAlphaAncestryPath) {
		t.Fatalf("ancestryPath: got %#v want %#v", alpha.AncestryPath, kitchenSinkAlphaAncestryPath)
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
	if !slices.Equal(alpha.AncestryPath, kitchenSinkAlphaAncestryPath) {
		t.Fatalf("ancestryPath: got %#v want %#v", alpha.AncestryPath, kitchenSinkAlphaAncestryPath)
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

func TestFindTopicParentIDWrongType(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Alpha",
		"parent_id": 123,
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-string parent_id")
	}
	if !strings.Contains(textContent(t, res), "expected a string") {
		t.Fatalf("error text: %s", textContent(t, res))
	}
}

func TestFindTopicParentIDEmptyString(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Alpha",
		"parent_id": "",
	})
	if !res.IsError {
		t.Fatal("expected tool error for empty parent_id")
	}
	if !strings.Contains(textContent(t, res), "non-empty string") {
		t.Fatalf("error text: %s", textContent(t, res))
	}
}

func TestFindTopicParentIDUnknown(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Alpha",
		"parent_id": "00000000-0000-0000-0000-000000000000",
	})
	if !res.IsError {
		t.Fatal("expected tool error for unknown parent_id")
	}
	if !strings.Contains(textContent(t, res), "topic not found") {
		t.Fatalf("error text: %s", textContent(t, res))
	}
}

// "Central Topic" exists only as the sheet root; it is not under Subtopic 1's subtree.
func TestFindTopicScopedTitleNotUnderParent(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Central Topic",
		"parent_id": kitchenSinkSubtopic1TopicID,
	})
	if !res.IsError {
		t.Fatal("expected tool error when title exists on sheet but not under parent_id scope")
	}
	if !strings.Contains(textContent(t, res), "no topic with title") {
		t.Fatalf("error text: %s", textContent(t, res))
	}
}

func TestFindTopicScopedDescendant(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Alpha",
		"parent_id": kitchenSinkSubtopic1TopicID,
	})
	if res.IsError {
		t.Fatalf("FindTopic: %s", textContent(t, res))
	}
	var out findTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != kitchenSinkAlphaTopicID {
		t.Fatalf("id: got %q want %q", out.ID, kitchenSinkAlphaTopicID)
	}
	if out.ParentTitle != "Subtopic 1" {
		t.Fatalf("parentTitle: got %q want Subtopic 1", out.ParentTitle)
	}
}

func TestFindTopicParentIDSelfMatch(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":      kitchenSinkPath(t),
		"sheet_id":  sheetID,
		"title":     "Alpha",
		"parent_id": kitchenSinkAlphaTopicID,
	})
	if res.IsError {
		t.Fatalf("FindTopic: %s", textContent(t, res))
	}
	var out findTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != kitchenSinkAlphaTopicID {
		t.Fatalf("id: got %q want %q", out.ID, kitchenSinkAlphaTopicID)
	}
	if out.ParentTitle != "" {
		t.Fatalf("parentTitle: got %q want empty (scope root matched)", out.ParentTitle)
	}
	if !slices.Equal(out.AncestryPath, kitchenSinkAlphaAncestryPath) {
		t.Fatalf("ancestryPath must stay sheet-root-relative when scope root matches: got %#v want %#v", out.AncestryPath, kitchenSinkAlphaAncestryPath)
	}
	if len(out.SiblingTitles) != 0 {
		t.Fatalf("expected no siblingTitles when scope root matches, got %#v", out.SiblingTitles)
	}
}

func TestFindTopicSheetRootAncestryNil(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FindTopic, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"title":    "Central Topic",
	})
	if res.IsError {
		t.Fatalf("FindTopic: %s", textContent(t, res))
	}
	var out findTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.AncestryPath != nil {
		t.Fatalf("sheet root match: want nil ancestryPath, got %#v", out.AncestryPath)
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

func TestGetSubtreeSheet10StructureClassNotesHref(t *testing.T) {
	h := NewXMindHandler()
	sid := kitchenSinkSheetIDByTitle(t, kitchenSinkSheet10Title)

	res := callTool(t, h.GetSubtree, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sid,
	})
	if res.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, res))
	}
	var root subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, res)), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if root.Title != "Central Topic" {
		t.Fatalf("title: %q", root.Title)
	}
	if root.StructureClass != "org.xmind.ui.map.clockwise" {
		t.Fatalf("structureClass: got %q", root.StructureClass)
	}

	resNotes := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sid,
		"topic_id":      kitchenSinkSheet10NoteTopicID,
		"include_notes": true,
	})
	if resNotes.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, resNotes))
	}
	var noteNode subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, resNotes)), &noteNode); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(noteNode.Notes, "This is a simple, plain text note") {
		t.Fatalf("notes: %q", noteNode.Notes)
	}

	resNoNotes := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sid,
		"topic_id":      kitchenSinkSheet10NoteTopicID,
		"include_notes": false,
	})
	if resNoNotes.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, resNoNotes))
	}
	var noNotesTree subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, resNoNotes)), &noNotesTree); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertSubtreeNoNotesField(t, &noNotesTree)

	resNoHref := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sid,
		"topic_id":      kitchenSinkSheet10HrefTopicID,
		"include_links": false,
	})
	if resNoHref.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, resNoHref))
	}
	var noHrefTree subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, resNoHref)), &noHrefTree); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertSubtreeNoHrefField(t, &noHrefTree)

	resHref := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sid,
		"topic_id":      kitchenSinkSheet10HrefTopicID,
		"include_links": true,
	})
	if resHref.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, resHref))
	}
	var hrefNode subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, resHref)), &hrefNode); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if hrefNode.Href != "https://www.google.com" {
		t.Fatalf("href: %q", hrefNode.Href)
	}
}

func TestGetSubtreeInvalidIncludeNotesType(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sheetID,
		"include_notes": "yes",
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-boolean include_notes")
	}
}

func TestGetSubtreeInvalidIncludeLinksType(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sheetID,
		"include_links": "yes",
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-boolean include_links")
	}
}

func TestGetSubtreeIncludeNotesAndLinksBothTrue(t *testing.T) {
	h := NewXMindHandler()
	sid := kitchenSinkSheetIDByTitle(t, kitchenSinkSheet10Title)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sid,
		"topic_id":      kitchenSinkSheet10NoteTopicID,
		"include_notes": true,
		"include_links": true,
	})
	if res.IsError {
		t.Fatalf("GetSubtree: %s", textContent(t, res))
	}
	var node subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, res)), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(node.Notes, "This is a simple, plain text note") {
		t.Fatalf("notes: %q", node.Notes)
	}
}

func TestGetSubtreeDepthWithIncludeFlags(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetSubtree, map[string]any{
		"path":          kitchenSinkPath(t),
		"sheet_id":      sheetID,
		"depth":         float64(1),
		"include_notes": true,
		"include_links": true,
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
}

func TestGetTopicPropertiesSheet10NotesHrefStructureClass(t *testing.T) {
	h := NewXMindHandler()
	sid := kitchenSinkSheetIDByTitle(t, kitchenSinkSheet10Title)

	sheets, err := xmind.ReadMap(kitchenSinkPath(t))
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	var rootID string
	for i := range sheets {
		if sheets[i].Title == kitchenSinkSheet10Title {
			rootID = sheets[i].RootTopic.ID
			break
		}
	}
	if rootID == "" {
		t.Fatal("sheet 10 not found")
	}

	resRoot := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sid,
		"topic_id": rootID,
	})
	if resRoot.IsError {
		t.Fatalf("GetTopicProperties: %s", textContent(t, resRoot))
	}
	var rootProps topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, resRoot)), &rootProps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rootProps.StructureClass != "org.xmind.ui.map.clockwise" {
		t.Fatalf("structureClass: got %q", rootProps.StructureClass)
	}

	resNotes := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sid,
		"topic_id": kitchenSinkSheet10NoteTopicID,
	})
	if resNotes.IsError {
		t.Fatalf("GetTopicProperties: %s", textContent(t, resNotes))
	}
	var noteProps topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, resNotes)), &noteProps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(noteProps.Notes, "This is a simple, plain text note") {
		t.Fatalf("notes: %q", noteProps.Notes)
	}

	resHref := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sid,
		"topic_id": kitchenSinkSheet10HrefTopicID,
	})
	if resHref.IsError {
		t.Fatalf("GetTopicProperties: %s", textContent(t, resHref))
	}
	var hrefProps topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, resHref)), &hrefProps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if hrefProps.Href != "https://www.google.com" {
		t.Fatalf("href: %q", hrefProps.Href)
	}
}

func TestGetTopicPropertiesRelationshipsFiltered(t *testing.T) {
	h := NewXMindHandler()
	path := kitchenSinkPath(t)
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	var sh *xmind.Sheet
	for i := range sheets {
		if sheets[i].ID == kitchenSinkRelationshipsSheetID {
			sh = &sheets[i]
			break
		}
	}
	if sh == nil {
		t.Fatal("relationships sheet not found")
	}
	if len(sh.Relationships) == 0 {
		t.Fatal("expected at least one relationship on kitchen-sink relationships sheet")
	}
	end1 := sh.Relationships[0].End1ID
	var wantCount int
	for i := range sh.Relationships {
		r := &sh.Relationships[i]
		if r.End1ID == end1 || r.End2ID == end1 {
			wantCount++
		}
	}

	res := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     path,
		"sheet_id": kitchenSinkRelationshipsSheetID,
		"topic_id": end1,
	})
	if res.IsError {
		t.Fatalf("GetTopicProperties: %s", textContent(t, res))
	}
	var out topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Relationships) != wantCount {
		t.Fatalf("relationships: got %d want %d", len(out.Relationships), wantCount)
	}
	byID := make(map[string]topicPropertiesRelationship, len(out.Relationships))
	for _, r := range out.Relationships {
		byID[r.ID] = r
	}
	for i := range sh.Relationships {
		r := &sh.Relationships[i]
		if r.End1ID != end1 && r.End2ID != end1 {
			continue
		}
		got, ok := byID[r.ID]
		if !ok {
			t.Fatalf("missing relationship id %s in response", r.ID)
		}
		if got.End1ID != r.End1ID || got.End2ID != r.End2ID {
			t.Fatalf("endpoint mismatch for %s: got %+v want End1=%q End2=%q", r.ID, got, r.End1ID, r.End2ID)
		}
	}
}

func TestGetTopicPropertiesKitchenSinkBoundarySummaryPosition(t *testing.T) {
	path := kitchenSinkPath(t)
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	var boundarySheetID, boundaryTopicID string
	var summarySheetID, summaryTopicID string
	var posSheetID, posTopicID string
	for si := range sheets {
		sh := &sheets[si]
		walkTopics(&sh.RootTopic, 0, nil, func(topic *xmind.Topic, _ int, _ *xmind.Topic) bool {
			if boundarySheetID == "" && len(topic.Boundaries) > 0 {
				boundarySheetID = sh.ID
				boundaryTopicID = topic.ID
			}
			if summarySheetID == "" && len(topic.Summaries) > 0 {
				summarySheetID = sh.ID
				summaryTopicID = topic.ID
			}
			if posSheetID == "" && topic.Position != nil {
				posSheetID = sh.ID
				posTopicID = topic.ID
			}
			return true
		})
	}
	if boundarySheetID == "" || boundaryTopicID == "" {
		t.Fatal("kitchen-sink fixture must include a topic with boundaries (see AGENTS.md test fixture)")
	}
	if summarySheetID == "" || summaryTopicID == "" {
		t.Fatal("kitchen-sink fixture must include a topic with summaries (see AGENTS.md test fixture)")
	}
	if posSheetID == "" || posTopicID == "" {
		t.Fatal("kitchen-sink fixture must include a topic with position (floating topic; see AGENTS.md test fixture)")
	}

	h := NewXMindHandler()

	resB := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     path,
		"sheet_id": boundarySheetID,
		"topic_id": boundaryTopicID,
	})
	if resB.IsError {
		t.Fatalf("GetTopicProperties (boundary): %s", textContent(t, resB))
	}
	var bOut topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, resB)), &bOut); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if bOut.BoundaryCount != len(bOut.Boundaries) || bOut.BoundaryCount < 1 {
		t.Fatalf("boundaryCount/boundaries: %+v", bOut)
	}

	resS := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     path,
		"sheet_id": summarySheetID,
		"topic_id": summaryTopicID,
	})
	if resS.IsError {
		t.Fatalf("GetTopicProperties (summary): %s", textContent(t, resS))
	}
	var sOut topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, resS)), &sOut); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sOut.SummaryCount < 1 {
		t.Fatalf("summaryCount: got %+v", sOut)
	}

	resP := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     path,
		"sheet_id": posSheetID,
		"topic_id": posTopicID,
	})
	if resP.IsError {
		t.Fatalf("GetTopicProperties (position): %s", textContent(t, resP))
	}
	var pOut topicPropertiesResponse
	if err := json.Unmarshal([]byte(textContent(t, resP)), &pOut); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pOut.Position == nil {
		t.Fatal("expected position on floating topic")
	}
}

func TestGetTopicPropertiesUnknownTopic(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
		"topic_id": "00000000-0000-0000-0000-000000000000",
	})
	if !res.IsError {
		t.Fatal("expected tool error for unknown topic_id")
	}
}

func TestGetTopicPropertiesMissingTopicID(t *testing.T) {
	h := NewXMindHandler()
	sheetID := firstKitchenSinkSheetID(t)
	res := callTool(t, h.GetTopicProperties, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sheetID,
	})
	if !res.IsError {
		t.Fatal("expected tool error for missing topic_id")
	}
}
