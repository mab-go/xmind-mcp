package handler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"
)

func TestIntegration_FindThenMutate(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	res := callTool(t, h.SearchTopics, map[string]any{
		"path": path, "sheet_id": sid, "query": "alpha",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	var st searchTopicsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &st); err != nil {
		t.Fatal(err)
	}
	var alphaID string
	for i := range st.Matches {
		if st.Matches[i].Title == "Alpha" {
			alphaID = st.Matches[i].ID
			break
		}
	}
	if alphaID == "" {
		t.Fatal("Alpha not found")
	}
	res = callTool(t, h.RenameTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": alphaID, "title": "AlphaRenamed",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, alphaID)
	if topic == nil || topic.Title != "AlphaRenamed" {
		t.Fatalf("rename not persisted: %+v", topic)
	}
	findRes := callTool(t, h.FindTopic, map[string]any{
		"path": path, "sheet_id": sid, "title": "Bravo",
	})
	if findRes.IsError {
		t.Fatal(textContent(t, findRes))
	}
}

func TestIntegration_BuildFromScratch(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "scratch.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Root"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "One"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Two"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Three"})
	topics := []any{
		map[string]any{
			"title": "BulkRoot",
			"children": []any{
				map[string]any{"title": "BulkChild"},
			},
		},
	}
	callTool(t, h.AddTopicsBulk, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "topics": topics,
	})
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	var idOne, idTwo string
	for _, ch := range sheets[0].RootTopic.Children.Attached {
		switch ch.Title {
		case "One":
			idOne = ch.ID
		case "Two":
			idTwo = ch.ID
		}
	}
	if idOne == "" || idTwo == "" {
		t.Fatalf("could not find One/Two: %+v", sheets[0].RootTopic.Children)
	}
	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": idOne, "to_id": idTwo, "label": "relates",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	flatRes := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "markdown",
	})
	if flatRes.IsError {
		t.Fatal(textContent(t, flatRes))
	}
	out := textContent(t, flatRes)
	for _, s := range []string{"Root", "One", "Two", "Three", "BulkRoot", "BulkChild"} {
		if !strings.Contains(out, s) {
			t.Fatalf("outline missing %q:\n%s", s, out)
		}
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets[0].Relationships) < 1 {
		t.Fatalf("expected relationship, got %+v", sheets[0].Relationships)
	}
}

func TestIntegration_KitchenSinkPreservation(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sheetsBefore, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheetsBefore) != 15 {
		t.Fatalf("want 15 sheets, got %d", len(sheetsBefore))
	}
	wantRoots := make([]string, 15)
	for i := range sheetsBefore {
		wantRoots[i] = sheetsBefore[i].RootTopic.Title
	}
	sid := sheetsBefore[0].ID
	res := callTool(t, h.RenameTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": kitchenSinkAlphaTopicID, "title": "AlphaTouched",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheetsAfter, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheetsAfter) != 15 {
		t.Fatalf("want 15 sheets after mutation, got %d", len(sheetsAfter))
	}
	for i := 1; i < 15; i++ {
		if got := sheetsAfter[i].RootTopic.Title; got != wantRoots[i] {
			t.Fatalf("sheet %d root title changed: got %q want %q", i, got, wantRoots[i])
		}
	}
	topic := findTopicByID(&sheetsAfter[0].RootTopic, kitchenSinkAlphaTopicID)
	if topic == nil || topic.Title != "AlphaTouched" {
		t.Fatalf("expected mutation on sheet 0 only: %+v", topic)
	}
}

func TestEdgeCase_UnicodeContent(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "uni.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	title := "日本語 🎉 עברית"
	addRes := callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": title,
	})
	if addRes.IsError {
		t.Fatal(textContent(t, addRes))
	}
	tid := parseAddTopicResult(t, addRes).ID
	note := "Note: مرحبا • emoji 🚀"
	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"notes": note, "labels": []any{"🏷️", "标签"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, tid)
	if topic == nil {
		t.Fatal("topic missing")
	}
	if topic.Title != title {
		t.Fatalf("title: got %q", topic.Title)
	}
	if topic.Notes == nil || topic.Notes.Plain == nil || topic.Notes.Plain.Content != note {
		t.Fatalf("notes: %+v", topic.Notes)
	}
	if len(topic.Labels) != 2 || topic.Labels[0] != "🏷️" {
		t.Fatalf("labels: %v", topic.Labels)
	}
}

func TestEdgeCase_FloatingOnlySheet(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "floatonly.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	res := callTool(t, h.AddFloatingTopic, map[string]any{
		"path": path, "sheet_id": sid, "title": "FloatOnly",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	subRes := callTool(t, h.GetSubtree, map[string]any{"path": path, "sheet_id": sid})
	if subRes.IsError {
		t.Fatal(textContent(t, subRes))
	}
	var node subtreeNode
	if err := json.Unmarshal([]byte(textContent(t, subRes)), &node); err != nil {
		t.Fatal(err)
	}
	if node.Title != "R" {
		t.Fatalf("root: %q", node.Title)
	}
	flatRes := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "text",
	})
	if flatRes.IsError {
		t.Fatal(textContent(t, flatRes))
	}
	if strings.TrimSpace(textContent(t, flatRes)) != "R" {
		t.Fatalf("expected only root line, got %q", textContent(t, flatRes))
	}
}
