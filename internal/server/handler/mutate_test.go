package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

func parseAddTopicResult(t *testing.T, res *mcp.CallToolResult) addTopicResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out addTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse add topic JSON: %v", err)
	}
	return out
}

func parseAddTopicsBulkResult(t *testing.T, res *mcp.CallToolResult) addTopicsBulkResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out addTopicsBulkResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse add topics bulk JSON: %v", err)
	}
	return out
}

func parseMoveTopicResult(t *testing.T, res *mcp.CallToolResult) moveTopicResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out moveTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse move topic JSON: %v", err)
	}
	return out
}

func TestAddTopicAndRevisionID(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "m.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Root"})
	if res.IsError {
		t.Fatalf("CreateMap: %s", textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	revBefore := sheets[0].RevisionID
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopic, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"title":     "Child",
	})
	if res.IsError {
		t.Fatalf("AddTopic: %s", textContent(t, res))
	}
	added := parseAddTopicResult(t, res)
	newID := added.ID
	if _, err := uuid.Parse(newID); err != nil {
		t.Fatalf("new topic id is not a valid UUID: %q", newID)
	}
	if added.Position != 0 || added.SiblingCount != 1 {
		t.Fatalf("unexpected add topic response: %+v", added)
	}

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if sheets[0].RevisionID == revBefore {
		t.Fatal("expected sheet RevisionID to change after AddTopic")
	}
	rt := sheets[0].RootTopic
	if rt.Children == nil || len(rt.Children.Attached) != 1 {
		t.Fatalf("expected one attached child, got %+v", rt.Children)
	}
	if rt.Children.Attached[0].ID != newID || rt.Children.Attached[0].Title != "Child" {
		t.Fatalf("unexpected child: %+v", rt.Children.Attached[0])
	}
}

func TestAddTopicTitleAmpersandWritesLiteralContentJSON(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "ampersand.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Root"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID
	const wantTitle = "Timeline & Milestones"
	res = callTool(t, h.AddTopic, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"title":     wantTitle,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	var raw []byte
	for _, f := range zr.File {
		if f.Name != "content.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if len(raw) == 0 {
		t.Fatal("content.json not found or empty in zip")
	}
	wantSub := []byte(`"title":"` + wantTitle + `"`)
	if !bytes.Contains(raw, wantSub) {
		n := len(raw)
		if n > 400 {
			n = 400
		}
		t.Fatalf("content.json should contain literal title %q as JSON substring; sample: %s", wantSub, raw[:n])
	}
	if bytes.Contains(raw, []byte(`\u0026`)) {
		t.Fatalf("content.json should not contain \\u0026")
	}
}

func TestAddTopicsBulkNested(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulk.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	topics := []any{
		map[string]any{
			"title": "L1",
			"children": []any{
				map[string]any{
					"title": "L2",
					"children": []any{
						map[string]any{"title": "L3"},
					},
				},
			},
		},
	}
	res = callTool(t, h.AddTopicsBulk, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"topics":    topics,
	})
	if res.IsError {
		t.Fatalf("AddTopicsBulk: %s", textContent(t, res))
	}
	bulk := parseAddTopicsBulkResult(t, res)
	if bulk.AddedCount != 3 || len(bulk.RootTopicIDs) != 1 || bulk.FirstPosition != 0 || bulk.SiblingCount != 1 {
		t.Fatalf("unexpected bulk response: %+v", bulk)
	}
	if bulk.ParentID != rootID {
		t.Fatalf("unexpected parentId: got %q want %q", bulk.ParentID, rootID)
	}

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rt := sheets[0].RootTopic
	if rt.Children == nil || len(rt.Children.Attached) != 1 {
		t.Fatal("expected one top-level branch")
	}
	l1 := &rt.Children.Attached[0]
	if bulk.RootTopicIDs[0] != l1.ID {
		t.Fatalf("rootTopicIds[0] %q != L1 id %q", bulk.RootTopicIDs[0], l1.ID)
	}
	if l1.Title != "L1" || l1.Children == nil || len(l1.Children.Attached) != 1 {
		t.Fatalf("L1: %+v", l1)
	}
	l2 := &l1.Children.Attached[0]
	if l2.Title != "L2" || l2.Children == nil || len(l2.Children.Attached) != 1 {
		t.Fatalf("L2: %+v", l2)
	}
	l3 := &l2.Children.Attached[0]
	if l3.Title != "L3" {
		t.Fatalf("L3: %+v", l3)
	}
}

func TestRenameTopic(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "r.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	rootID := sheets[0].RootTopic.ID
	addRes := callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "parent_id": rootID, "title": "Old",
	})
	tid := parseAddTopicResult(t, addRes).ID

	res := callTool(t, h.RenameTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": tid, "title": "New",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	topic := findTopicByID(&sheets[0].RootTopic, tid)
	if topic == nil || topic.Title != "New" {
		t.Fatalf("rename failed: %+v", topic)
	}
}

func TestDeleteTopicRootError(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "d.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.DeleteTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": sheets[0].RootTopic.ID,
	})
	if !res.IsError {
		t.Fatal("expected error deleting root")
	}
}

func TestDeleteTopic(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "d2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	rootID := sheets[0].RootTopic.ID
	addRes := callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "parent_id": rootID, "title": "X",
	})
	tid := parseAddTopicResult(t, addRes).ID

	res := callTool(t, h.DeleteTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": tid,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if findTopicByID(&sheets[0].RootTopic, tid) != nil {
		t.Fatal("topic still present")
	}
	if sheets[0].RootTopic.Children != nil && len(sheets[0].RootTopic.Children.Attached) != 0 {
		t.Fatal("expected no attached children")
	}
}

func TestMoveTopicCycleError(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "mv.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rootID := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})).ID
	bID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": aID, "title": "B",
	})).ID

	res := callTool(t, h.MoveTopic, map[string]any{
		"path":          path,
		"sheet_id":      sid,
		"topic_id":      aID,
		"new_parent_id": bID,
	})
	if !res.IsError {
		t.Fatal("expected cycle error")
	}
}

func TestMoveTopic(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "mv2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rootID := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})).ID
	bID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "B",
	})).ID

	res := callTool(t, h.MoveTopic, map[string]any{
		"path":          path,
		"sheet_id":      sid,
		"topic_id":      bID,
		"new_parent_id": aID,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	mv := parseMoveTopicResult(t, res)
	if mv.TopicID != bID || mv.ParentID != aID || mv.Position != 0 || mv.SiblingCount != 1 {
		t.Fatalf("unexpected move response: %+v", mv)
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	if root.Children == nil || len(root.Children.Attached) != 1 || root.Children.Attached[0].ID != aID {
		t.Fatalf("root should have only A after move; got %+v", root.Children)
	}
	a := findTopicByID(root, aID)
	if a == nil || a.Children == nil || len(a.Children.Attached) != 1 || a.Children.Attached[0].ID != bID {
		t.Fatalf("B not under A: %+v", a)
	}
	if findTopicByID(root, bID) == nil {
		t.Fatal("B should still exist under A")
	}
}

// TestMoveTopicSiblingForward is the regression test for the stale-pointer bug:
// moving a topic to a sibling that appears AFTER it in the same Attached slice.
// Before the fix, newParent became stale after removeChildAt and the moved topic
// was silently dropped.
func TestMoveTopicSiblingForward(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sib_fwd.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID

	// Add Alpha then Gamma so Alpha is at index 0, Gamma at index 1.
	alphaID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Alpha",
	})).ID
	gammaID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Gamma",
	})).ID

	// Move Alpha (index 0) under Gamma (index 1) — destination is AFTER source.
	res := callTool(t, h.MoveTopic, map[string]any{
		"path":          path,
		"sheet_id":      sid,
		"topic_id":      alphaID,
		"new_parent_id": gammaID,
	})
	if res.IsError {
		t.Fatalf("MoveTopic: %s", textContent(t, res))
	}
	mv := parseMoveTopicResult(t, res)
	if mv.TopicID != alphaID || mv.ParentID != gammaID || mv.Position != 0 || mv.SiblingCount != 1 {
		t.Fatalf("unexpected move response: %+v", mv)
	}

	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic

	// Root should have only Gamma as a direct child.
	if root.Children == nil || len(root.Children.Attached) != 1 {
		t.Fatalf("root should have exactly 1 child after move; got %+v", root.Children)
	}
	if root.Children.Attached[0].ID != gammaID {
		t.Fatalf("expected Gamma as root's only child; got %+v", root.Children.Attached[0])
	}

	// Gamma should have Alpha as a child.
	gamma := findTopicByID(root, gammaID)
	if gamma == nil {
		t.Fatal("Gamma not found")
	}
	if gamma.Children == nil || len(gamma.Children.Attached) != 1 {
		t.Fatalf("Gamma should have exactly 1 child; got %+v", gamma.Children)
	}
	if gamma.Children.Attached[0].ID != alphaID {
		t.Fatalf("expected Alpha under Gamma; got %+v", gamma.Children.Attached[0])
	}

	// Alpha must still exist in the tree (was being silently dropped before fix).
	if findTopicByID(root, alphaID) == nil {
		t.Fatal("Alpha was lost from the tree (stale-pointer bug)")
	}
}

// TestMoveTopicSiblingBackward moves a topic to a sibling that appears BEFORE it
// in the same Attached slice. This direction was already working before the fix
// but is included here for symmetry.
func TestMoveTopicSiblingBackward(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sib_bwd.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID

	// Add Gamma then Alpha so Gamma is at index 0, Alpha at index 1.
	gammaID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Gamma",
	})).ID
	alphaID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Alpha",
	})).ID

	// Move Alpha (index 1) under Gamma (index 0) — destination is BEFORE source.
	res := callTool(t, h.MoveTopic, map[string]any{
		"path":          path,
		"sheet_id":      sid,
		"topic_id":      alphaID,
		"new_parent_id": gammaID,
	})
	if res.IsError {
		t.Fatalf("MoveTopic: %s", textContent(t, res))
	}
	mv := parseMoveTopicResult(t, res)
	if mv.TopicID != alphaID || mv.ParentID != gammaID || mv.Position != 0 || mv.SiblingCount != 1 {
		t.Fatalf("unexpected move response: %+v", mv)
	}

	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic

	if root.Children == nil || len(root.Children.Attached) != 1 {
		t.Fatalf("root should have exactly 1 child after move; got %+v", root.Children)
	}
	if root.Children.Attached[0].ID != gammaID {
		t.Fatalf("expected Gamma as root's only child; got %+v", root.Children.Attached[0])
	}

	gamma := findTopicByID(root, gammaID)
	if gamma == nil {
		t.Fatal("Gamma not found")
	}
	if gamma.Children == nil || len(gamma.Children.Attached) != 1 {
		t.Fatalf("Gamma should have exactly 1 child; got %+v", gamma.Children)
	}
	if gamma.Children.Attached[0].ID != alphaID {
		t.Fatalf("expected Alpha under Gamma; got %+v", gamma.Children.Attached[0])
	}
	if findTopicByID(root, alphaID) == nil {
		t.Fatal("Alpha was lost from the tree")
	}
}

func TestReorderChildrenMismatch(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "ord.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rootID := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rootID, "title": "B"})

	res := callTool(t, h.ReorderChildren, map[string]any{
		"path":        path,
		"sheet_id":    sid,
		"parent_id":   rootID,
		"ordered_ids": []any{"not-a-real-id"},
	})
	if !res.IsError {
		t.Fatal("expected mismatch error")
	}
}

func TestReorderChildren(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "ord2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rootID := sheets[0].RootTopic.ID
	idA := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})).ID
	idB := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "B",
	})).ID

	res := callTool(t, h.ReorderChildren, map[string]any{
		"path":        path,
		"sheet_id":    sid,
		"parent_id":   rootID,
		"ordered_ids": []any{idB, idA},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	ch := sheets[0].RootTopic.Children.Attached
	if len(ch) != 2 || ch[0].Title != "B" || ch[1].Title != "A" {
		t.Fatalf("order: %+v", ch)
	}
}

func TestSetTopicProperties(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path":     path,
		"sheet_id": sid,
		"topic_id": tid,
		"notes":    "Note body",
		"labels":   []any{"l1", "l2"},
		"markers":  []any{"priority-1"},
		"link":     "https://example.com",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	topic := &sheets[0].RootTopic
	if topic.Notes == nil || topic.Notes.Plain == nil || topic.Notes.Plain.Content != "Note body" ||
		topic.Notes.RealHTML == nil || topic.Notes.RealHTML.Content != "<div>Note body</div>" {
		t.Fatalf("notes: %+v", topic.Notes)
	}
	if len(topic.Labels) != 2 || topic.Labels[0] != "l1" || topic.Labels[1] != "l2" {
		t.Fatalf("labels: %v", topic.Labels)
	}
	if len(topic.Markers) != 1 || topic.Markers[0].MarkerID != "priority-1" {
		t.Fatalf("markers: %+v", topic.Markers)
	}
	if topic.Href != "https://example.com" {
		t.Fatalf("href: %q", topic.Href)
	}
}

func TestAddFloatingTopic(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID

	res := callTool(t, h.AddFloatingTopic, map[string]any{
		"path": path, "sheet_id": sid, "title": "Float",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	rt := sheets[0].RootTopic
	if rt.Children == nil || len(rt.Children.Detached) != 1 {
		t.Fatalf("expected one detached: %+v", rt.Children)
	}
	if rt.Children.Detached[0].Title != "Float" || rt.Children.Detached[0].Position == nil {
		t.Fatalf("bad float topic: %+v", rt.Children.Detached[0])
	}
}

func TestAddTopicAtPositionZero(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "pos.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})
	callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B", "position": float64(0),
	})
	sheets, _ = xmind.ReadMap(path)
	ch := sheets[0].RootTopic.Children.Attached
	if len(ch) != 2 || ch[0].Title != "B" || ch[1].Title != "A" {
		t.Fatalf("want B then A, got %+v", ch)
	}
}

func TestDeleteTopicBumpsRevisionID(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "revdel.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	rev := sheets[0].RevisionID
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	addRes := callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "X"})
	tid := parseAddTopicResult(t, addRes).ID
	res := callTool(t, h.DeleteTopic, map[string]any{"path": path, "sheet_id": sid, "topic_id": tid})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RevisionID == rev {
		t.Fatal("expected RevisionID to change after DeleteTopic")
	}
}

func TestAddTopicShiftsSummaryRanges(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sumins.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	for _, title := range []string{"A", "B", "C"} {
		callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": title})
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root := &sheets[0].RootTopic
	sumTopicID := uuid.New().String()
	root.Summaries = []xmind.Summary{{ID: uuid.New().String(), Range: "(0,2)", TopicID: sumTopicID}}
	if root.Children == nil {
		t.Fatal("expected children")
	}
	root.Children.Summary = []xmind.Topic{{ID: sumTopicID, Title: "S"}}
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "NEW", "position": float64(0),
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root = &sheets[0].RootTopic
	if len(root.Summaries) != 1 || root.Summaries[0].Range != "(1,3)" {
		t.Fatalf("want summary range (1,3), got %+v", root.Summaries)
	}
}

func TestDeleteTopicCollapsesSummaryRange(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sumdel.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	for _, title := range []string{"A", "B", "C"} {
		callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": title})
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root := &sheets[0].RootTopic
	sumTopicID := uuid.New().String()
	root.Summaries = []xmind.Summary{{ID: uuid.New().String(), Range: "(0,2)", TopicID: sumTopicID}}
	if root.Children == nil {
		t.Fatal("expected children")
	}
	root.Children.Summary = []xmind.Topic{{ID: sumTopicID, Title: "S"}}
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}

	var bID string
	for i := range root.Children.Attached {
		if root.Children.Attached[i].Title == "B" {
			bID = root.Children.Attached[i].ID
			break
		}
	}
	if bID == "" {
		t.Fatal("topic B not found")
	}

	res := callTool(t, h.DeleteTopic, map[string]any{"path": path, "sheet_id": sid, "topic_id": bID})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root = &sheets[0].RootTopic
	if len(root.Summaries) != 1 || root.Summaries[0].Range != "(0,1)" {
		t.Fatalf("want summary range (0,1), got %+v", root.Summaries)
	}
}

func TestSetTopicPropertiesPartial(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"notes": "N1", "labels": []any{"a"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{"task-done"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	topic := &sheets[0].RootTopic
	if topic.Notes == nil || topic.Notes.Plain == nil || topic.Notes.Plain.Content != "N1" {
		t.Fatalf("notes should remain: %+v", topic.Notes)
	}
	if len(topic.Labels) != 1 || topic.Labels[0] != "a" {
		t.Fatalf("labels should remain: %v", topic.Labels)
	}
	if len(topic.Markers) != 1 || topic.Markers[0].MarkerID != "task-done" {
		t.Fatalf("markers: %+v", topic.Markers)
	}
}

func TestSetTopicPropertiesClearSemantics(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "clearsem.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"notes": "keep", "labels": []any{"x"}, "markers": []any{"priority-1"}, "link": "https://a.example",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "notes": "",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RootTopic.Notes != nil {
		t.Fatalf("notes empty string: want nil, got %+v", sheets[0].RootTopic.Notes)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"notes": "n2", "labels": []any{"y"}, "markers": []any{"task-done"}, "link": "https://b.example",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "notes": nil,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RootTopic.Notes != nil {
		t.Fatalf("notes nil: want nil, got %+v", sheets[0].RootTopic.Notes)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "labels": []any{},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Labels) != 0 {
		t.Fatalf("labels: want empty, got %v", sheets[0].RootTopic.Labels)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "markers": []any{},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Markers) != 0 {
		t.Fatalf("markers: want empty, got %+v", sheets[0].RootTopic.Markers)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "link": "",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RootTopic.Href != "" {
		t.Fatalf("link: want empty href, got %q", sheets[0].RootTopic.Href)
	}
}

func TestSetTopicPropertiesRemoveMarkers(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rmmarkers.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{"priority-1", "task-done"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	m := sheets[0].RootTopic.Markers
	if len(m) != 2 {
		t.Fatalf("empty remove_markers: want two markers, got %+v", m)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{"unknown-id"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	m = sheets[0].RootTopic.Markers
	if len(m) != 2 {
		t.Fatalf("unknown remove id: want two markers, got %+v", m)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{"priority-1"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	m = sheets[0].RootTopic.Markers
	if len(m) != 1 || m[0].MarkerID != "task-done" {
		t.Fatalf("partial remove: want [task-done], got %+v", m)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Markers) != 0 {
		t.Fatalf("markers empty array: want no markers, got %+v", sheets[0].RootTopic.Markers)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{"still-unknown"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Markers) != 0 {
		t.Fatalf("remove_markers with no markers: want empty, got %+v", sheets[0].RootTopic.Markers)
	}

	res = callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{"a", "b"}, "remove_markers": []any{"a"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	m = sheets[0].RootTopic.Markers
	if len(m) != 1 || m[0].MarkerID != "b" {
		t.Fatalf("markers then remove: want [b], got %+v", m)
	}
}

func nestBulkTopic(depth int) any {
	if depth <= 0 {
		return map[string]any{"title": "leaf"}
	}
	return map[string]any{"title": "n", "children": []any{nestBulkTopic(depth - 1)}}
}

func TestAddTopicsBulkExceedsMaxDepth(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "deep.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID

	res := callTool(t, h.AddTopicsBulk, map[string]any{
		"path":      path,
		"sheet_id":  sid,
		"parent_id": rid,
		"topics":    []any{nestBulkTopic(65)},
	})
	if !res.IsError {
		t.Fatal("expected tool error when bulk topics exceed max depth")
	}
}

func TestAddRelationship(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rel.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID
	bID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})).ID

	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": aID, "to_id": bID, "label": "relates",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	sh := &sheets[0]
	if len(sh.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %+v", sh.Relationships)
	}
	rel := sh.Relationships[0]
	if rel.End1ID != aID || rel.End2ID != bID || rel.Title != "relates" {
		t.Fatalf("unexpected relationship: %+v", rel)
	}
}

func TestDeleteRelationship(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "delrel.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID
	bID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})).ID

	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": aID, "to_id": bID,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	sh := &sheets[0]
	if len(sh.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %+v", sh.Relationships)
	}
	relID := sh.Relationships[0].ID

	res = callTool(t, h.DeleteRelationship, map[string]any{
		"path": path, "sheet_id": sid, "relationship_id": relID,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	if got, want := textContent(t, res), fmt.Sprintf("deleted relationship id %s", relID); got != want {
		t.Fatalf("success text: got %q want %q", got, want)
	}
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].Relationships) != 0 {
		t.Fatalf("expected 0 relationships after delete, got %+v", sheets[0].Relationships)
	}
}

func TestDeleteRelationshipNotFound(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "delrel_nf.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	res := callTool(t, h.DeleteRelationship, map[string]any{
		"path": path, "sheet_id": sid, "relationship_id": "00000000-0000-0000-0000-000000000001",
	})
	if !res.IsError {
		t.Fatal("expected tool error when relationship_id missing on sheet")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "relationship not found on sheet") || !strings.Contains(msg, sid) {
		t.Fatalf("unexpected error: %q", msg)
	}
}

func TestDeleteRelationshipInvalidSheetID(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "delrel_bad_sheet.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	res := callTool(t, h.DeleteRelationship, map[string]any{
		"path": path, "sheet_id": "00000000-0000-0000-0000-000000000000", "relationship_id": "00000000-0000-0000-0000-000000000001",
	})
	if !res.IsError {
		t.Fatal("expected tool error for unknown sheet_id")
	}
}

func TestAddRelationshipInvalidTopic(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rel2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID

	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": aID, "to_id": "00000000-0000-0000-0000-000000000000",
	})
	if !res.IsError {
		t.Fatal("expected error for missing to_id")
	}

	res = callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": "00000000-0000-0000-0000-000000000000", "to_id": aID,
	})
	if !res.IsError {
		t.Fatal("expected error for invalid from_id")
	}
}

func TestAddRelationshipLabelWrongType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rel3.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID
	bID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})).ID

	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": aID, "to_id": bID, "label": float64(42),
	})
	if !res.IsError {
		t.Fatal("expected error for non-string label")
	}
}

func TestAddSummary(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sum.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	for _, title := range []string{"A", "B", "C"} {
		callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": title})
	}

	res := callTool(t, h.AddSummary, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid,
		"from_index": float64(0), "to_index": float64(2), "title": "Sum",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	if root.Children == nil || len(root.Children.Summary) != 1 {
		t.Fatalf("expected one summary topic: %+v", root.Children)
	}
	if len(root.Summaries) != 1 || root.Summaries[0].Range != "(0,2)" {
		t.Fatalf("expected summaries range (0,2), got %+v", root.Summaries)
	}
	if root.Summaries[0].TopicID != root.Children.Summary[0].ID {
		t.Fatalf("topicId mismatch: %q vs %q", root.Summaries[0].TopicID, root.Children.Summary[0].ID)
	}
	if root.Summaries[0].ID == root.Children.Summary[0].ID {
		t.Fatal("summary row id must differ from summary topic id")
	}
	if root.Children.Summary[0].Title != "Sum" {
		t.Fatalf("summary topic title: got %q want Sum", root.Children.Summary[0].Title)
	}
}

func TestAddSummaryOutOfBounds(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sum2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})

	res := callTool(t, h.AddSummary, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid,
		"from_index": float64(0), "to_index": float64(5),
	})
	if !res.IsError {
		t.Fatal("expected out-of-bounds error")
	}
}

func TestAddSummaryNoAttachedChildren(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sum0.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	res := callTool(t, h.AddSummary, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid,
		"from_index": float64(0), "to_index": float64(0),
	})
	if !res.IsError {
		t.Fatal("expected error when parent has no attached children")
	}
}

func TestAddSummaryFromGreaterThanTo(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sum3.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})

	res := callTool(t, h.AddSummary, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid,
		"from_index": float64(2), "to_index": float64(0),
	})
	if !res.IsError {
		t.Fatal("expected from>to error")
	}
}

func TestAddBoundary(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bnd.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})

	res := callTool(t, h.AddBoundary, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Zone",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	if len(root.Boundaries) != 1 {
		t.Fatalf("expected one boundary on topic: %+v", root)
	}
	b := root.Boundaries[0]
	if b.Range != "master" || b.Title != "Zone" {
		t.Fatalf("unexpected boundary: %+v", b)
	}
}

func TestAddBoundaryNoChildren(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bnd2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID

	res := callTool(t, h.AddBoundary, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid,
	})
	if !res.IsError {
		t.Fatal("expected error when parent has no attached children")
	}
}

func TestAddTopicParentNotFound(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "parent_id": "00000000-0000-0000-0000-000000000000", "title": "X",
	})
	if !res.IsError {
		t.Fatal("expected error for missing parent_id")
	}
}

func TestRenameTopicNonexistent(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rn.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.RenameTopic, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": "00000000-0000-0000-0000-000000000000", "title": "N",
	})
	if !res.IsError {
		t.Fatal("expected error for missing topic_id")
	}
}

func TestRenameTopicClearsTitleUnedited(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "uned.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	addRes := callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Child"})
	tid := parseAddTopicResult(t, addRes).ID
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, tid)
	if topic == nil {
		t.Fatal("topic missing")
	}
	topic.TitleUnedited = true
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, h.RenameTopic, map[string]any{"path": path, "sheet_id": sid, "topic_id": tid, "title": "Renamed"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic = findTopicByID(&sheets[0].RootTopic, tid)
	if topic == nil || topic.Title != "Renamed" || topic.TitleUnedited {
		t.Fatalf("after rename: %+v", topic)
	}
}

func TestMoveTopicToPosition(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "mpos.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	_ = parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID
	idB := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})).ID
	res := callTool(t, h.MoveTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": idB, "new_parent_id": rid, "position": float64(0),
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	mv := parseMoveTopicResult(t, res)
	if mv.TopicID != idB || mv.ParentID != rid || mv.Position != 0 || mv.SiblingCount != 2 {
		t.Fatalf("unexpected move response: %+v", mv)
	}
	sheets, _ = xmind.ReadMap(path)
	ch := sheets[0].RootTopic.Children.Attached
	if len(ch) != 2 || ch[0].Title != "B" || ch[1].Title != "A" {
		t.Fatalf("order: %+v", ch)
	}
}

func TestDeleteTopicAlsoRemovesDescendants(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "del3.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	l1 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "L1",
	})).ID
	l2 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": l1, "title": "L2",
	})).ID
	l3 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": l2, "title": "L3",
	})).ID
	res := callTool(t, h.DeleteTopic, map[string]any{"path": path, "sheet_id": sid, "topic_id": l1})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	for _, id := range []string{l1, l2, l3} {
		if findTopicByID(root, id) != nil {
			t.Fatalf("topic %s should be gone", id)
		}
	}
}

func TestSetTopicPropertiesMissingTopicID(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "stp.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "notes": "n",
	})
	if !res.IsError {
		t.Fatal("expected error when topic_id is missing")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "topic_id") {
		t.Fatalf("expected topic_id in error message, got %q", msg)
	}
}

func TestSetTopicPropertiesNotesWrongType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "noteswrong.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	tid := sheets[0].RootTopic.ID
	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": tid,
		"notes": float64(42),
	})
	if !res.IsError {
		t.Fatal("expected error when notes is not a string")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "invalid argument notes") {
		t.Fatalf("expected invalid argument notes in message, got %q", msg)
	}
}

func TestSetTopicPropertiesMarkersWrongType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "mkwrong.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	tid := sheets[0].RootTopic.ID
	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": tid,
		"markers": "not-an-array",
	})
	if !res.IsError {
		t.Fatal("expected error when markers is not an array")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "invalid argument markers") {
		t.Fatalf("expected invalid argument markers in message, got %q", msg)
	}
}

func TestSetTopicPropertiesMultilineNoteHTML(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "mlnote.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	tid := sheets[0].RootTopic.ID
	res := callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "topic_id": tid,
		"notes": "a\nb",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	n := sheets[0].RootTopic.Notes
	want := "<div>a</div><div>b</div>"
	if n == nil || n.RealHTML == nil || n.RealHTML.Content != want {
		t.Fatalf("multiline RealHTML: want %q, got %+v", want, n)
	}
}

func TestSetTopicPropertiesBulkHappyPath(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkhp.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	id1 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "One",
	})).ID
	id2 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Two",
	})).ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path":      path,
		"sheet_id":  sid,
		"topic_ids": []any{id1, id2},
		"labels":    []any{"bulk-a", "bulk-b"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	if got := textContent(t, res); got != "updated 2 topics" {
		t.Fatalf("message: got %q want %q", got, "updated 2 topics")
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	for _, id := range []string{id1, id2} {
		topic := findTopicByID(root, id)
		if topic == nil {
			t.Fatalf("topic %s not found", id)
		}
		if len(topic.Labels) != 2 || topic.Labels[0] != "bulk-a" || topic.Labels[1] != "bulk-b" {
			t.Fatalf("topic %s labels: %v", id, topic.Labels)
		}
	}
}

func TestSetTopicPropertiesBulkMarkersAndRemoveMarkers(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkmm.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	id1 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "One",
	})).ID
	id2 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Two",
	})).ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path":      path,
		"sheet_id":  sid,
		"topic_ids": []any{id1, id2},
		"markers":   []any{"priority-1", "task-done"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	res = callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path":           path,
		"sheet_id":       sid,
		"topic_ids":      []any{id1, id2},
		"remove_markers": []any{"priority-1"},
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	for _, id := range []string{id1, id2} {
		topic := findTopicByID(root, id)
		if topic == nil {
			t.Fatalf("topic %s not found", id)
		}
		if len(topic.Markers) != 1 || topic.Markers[0].MarkerID != "task-done" {
			t.Fatalf("topic %s markers: %+v", id, topic.Markers)
		}
	}
}

func TestSetTopicPropertiesBulkEmptyTopicIDs(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkempty.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": []any{},
		"notes": "x",
	})
	if !res.IsError {
		t.Fatal("expected error for empty topic_ids")
	}
	if msg := textContent(t, res); !strings.Contains(msg, "topic_ids must be non-empty") {
		t.Fatalf("empty topic_ids: got %q", msg)
	}
	res = callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "notes": "x",
	})
	if !res.IsError {
		t.Fatal("expected error when topic_ids is missing")
	}
	if msg := textContent(t, res); !strings.Contains(msg, "missing required argument: topic_ids") {
		t.Fatalf("missing topic_ids: got %q", msg)
	}
}

func TestSetTopicPropertiesBulkTopicIDsWrongType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulktype.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": float64(1), "labels": []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error when topic_ids is not an array")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "invalid argument topic_ids: expected an array") {
		t.Fatalf("got %q", msg)
	}
}

func TestSetTopicPropertiesBulkTopicIDEmptyString(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkemptystr.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": []any{tid, ""}, "labels": []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error for empty string in topic_ids")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "topic_ids[1]: expected non-empty string") {
		t.Fatalf("got %q", msg)
	}
}

func TestSetTopicPropertiesBulkTopicIDNonString(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulknonstr.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": []any{tid, 42}, "labels": []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error for non-string topic_ids element")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "topic_ids[1]: expected non-empty string") {
		t.Fatalf("got %q", msg)
	}
}

func TestSetTopicPropertiesBulkSingleMissingID(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkonemiss.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	miss := "00000000-0000-0000-0000-00000000dead"

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": []any{miss}, "labels": []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error for missing topic")
	}
	msg := textContent(t, res)
	if !strings.HasPrefix(msg, "topic not found: ") || !strings.Contains(msg, miss) {
		t.Fatalf("got %q", msg)
	}
}

func TestSetTopicPropertiesBulkDuplicateTopicIDs(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkdup.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path":      path,
		"sheet_id":  sid,
		"topic_ids": []any{tid, tid},
		"labels":    []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error for duplicate topic_ids")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "duplicate id in topic_ids") {
		t.Fatalf("expected duplicate error, got %q", msg)
	}
}

func TestSetTopicPropertiesBulkMultipleMissingIDs(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulkmiss.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	m1 := "00000000-0000-0000-0000-000000000003"
	m2 := "00000000-0000-0000-0000-000000000001"
	m3 := "00000000-0000-0000-0000-000000000002"

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path":      path,
		"sheet_id":  sid,
		"topic_ids": []any{m1, m2, m3},
		"labels":    []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error for missing topics")
	}
	msg := textContent(t, res)
	if !strings.HasPrefix(msg, "topic not found: ") {
		t.Fatalf("error message: got %q", msg)
	}
	for _, id := range []string{m1, m2, m3} {
		if !strings.Contains(msg, id) {
			t.Fatalf("error message missing %s: %q", id, msg)
		}
	}
}

func TestSetTopicPropertiesBulkSheetNotFound(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulksh.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	tid := sheets[0].RootTopic.ID
	badSheet := "00000000-0000-0000-0000-00000000dead"

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path":      path,
		"sheet_id":  badSheet,
		"topic_ids": []any{tid},
		"labels":    []any{"x"},
	})
	if !res.IsError {
		t.Fatal("expected error for unknown sheet")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "sheet not found:") || !strings.Contains(msg, badSheet) {
		t.Fatalf("error message: got %q", msg)
	}
}

func TestSetTopicPropertiesBulkNoActionableProperty(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulknoact.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	tid := sheets[0].RootTopic.ID

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": []any{tid},
	})
	if !res.IsError {
		t.Fatal("expected error when no property updates")
	}
	res = callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sid, "topic_ids": []any{tid}, "labels": nil,
	})
	if !res.IsError {
		t.Fatal("expected error when only labels:null")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "missing property updates") {
		t.Fatalf("expected missing property updates, got %q", msg)
	}
}

func TestDuplicateTopicHappyPath(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	mid := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Mid",
	})).ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": mid, "title": "Leaf"})
	res := callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": mid, "target_parent_id": rid,
	})
	if res.IsError {
		t.Fatalf("DuplicateTopic: %s", textContent(t, res))
	}
	var dup duplicateTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &dup); err != nil {
		t.Fatalf("parse duplicate topic JSON: %v", err)
	}
	if dup.SourceID != mid || dup.ParentID != rid || dup.CopiedCount != 2 {
		t.Fatalf("unexpected duplicate response: %+v", dup)
	}
	if dup.Position != 1 || dup.SiblingCount != 2 {
		t.Fatalf("unexpected duplicate response position/siblingCount: %+v", dup)
	}
	newID := dup.NewRootID
	if newID == mid {
		t.Fatalf("expected new root id to differ from source: %s", newID)
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root := &sheets[0].RootTopic
	if root.Children == nil || len(root.Children.Attached) != 2 {
		t.Fatalf("want 2 attached children on root (original Mid + duplicate), got %+v", root.Children)
	}
	if root.Children.Attached[0].ID != mid {
		t.Fatalf("first child should be original Mid %s", mid)
	}
	if root.Children.Attached[1].ID != newID {
		t.Fatalf("second child should be duplicate root %s, got %s", newID, root.Children.Attached[1].ID)
	}
	if root.Children.Attached[1].Title != "Mid" {
		t.Fatalf("duplicate title: %+v", root.Children.Attached[1])
	}
	if root.Children.Attached[1].Children == nil || len(root.Children.Attached[1].Children.Attached) != 1 ||
		root.Children.Attached[1].Children.Attached[0].Title != "Leaf" {
		t.Fatalf("duplicate should include Leaf: %+v", root.Children.Attached[1].Children)
	}
}

func TestDuplicateTopicPositionInsertAtZeroAndAppend(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "duppos.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "B"})
	src := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Src",
	})).ID
	callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": src, "target_parent_id": rid, "position": float64(0),
	})
	sheets, _ = xmind.ReadMap(path)
	ch := sheets[0].RootTopic.Children.Attached
	if len(ch) != 4 || ch[0].Title != "Src" || ch[1].Title != "A" {
		t.Fatalf("want Src first after insert at 0, got titles %v", topicTitles(ch))
	}

	// Append: omit position — duplicate should be last; duplicate root id must differ from original Src.
	callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": src, "target_parent_id": rid,
	})
	sheets, _ = xmind.ReadMap(path)
	ch = sheets[0].RootTopic.Children.Attached
	if len(ch) != 5 || ch[4].Title != "Src" || ch[4].ID == src || ch[3].ID != src {
		t.Fatalf("want original Src at index 3 and appended duplicate at index 4, got titles %v ids last=%s src=%s",
			topicTitles(ch), ch[4].ID, src)
	}
}

func topicTitles(topics []xmind.Topic) []string {
	var s []string
	for i := range topics {
		s = append(s, topics[i].Title)
	}
	return s
}

func TestDuplicateTopicErrors(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "duperr.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	child := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "C",
	})).ID

	res := callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": "00000000-0000-0000-0000-000000000099", "topic_id": child, "target_parent_id": rid,
	})
	if !res.IsError || !strings.Contains(textContent(t, res), "sheet not found") {
		t.Fatalf("expected sheet not found, got %v %q", res.IsError, textContent(t, res))
	}
	res = callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": "00000000-0000-0000-0000-000000000099", "target_parent_id": rid,
	})
	if !res.IsError || !strings.Contains(textContent(t, res), "source topic not found") {
		t.Fatalf("expected source topic not found, got %q", textContent(t, res))
	}
	res = callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": child, "target_parent_id": "00000000-0000-0000-0000-000000000099",
	})
	if !res.IsError || !strings.Contains(textContent(t, res), "target parent not found") {
		t.Fatalf("expected target parent not found, got %q", textContent(t, res))
	}
	res = callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": child, "target_parent_id": rid, "position": float64(99),
	})
	if !res.IsError || !strings.Contains(textContent(t, res), "out of range") {
		t.Fatalf("expected position out of range, got %q", textContent(t, res))
	}
}

func TestDuplicateTopicKitchenSinkSummariesSmoke(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	var summarySheetID, summaryTopicID string
outer:
	for si := range sheets {
		sh := &sheets[si]
		walkTopics(&sh.RootTopic, 0, nil, func(topic *xmind.Topic, _ int, _ *xmind.Topic) bool {
			if len(topic.Summaries) > 0 {
				summarySheetID = sh.ID
				summaryTopicID = topic.ID
				return false
			}
			return true
		})
		if summarySheetID != "" {
			break outer
		}
	}
	if summarySheetID == "" || summaryTopicID == "" {
		t.Fatal("fixture must include a topic with summaries")
	}
	sh := findSheetByID(sheets, summarySheetID)
	if sh == nil {
		t.Fatal("sheet not found")
	}
	rootID := sh.RootTopic.ID
	res := callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": summarySheetID, "topic_id": summaryTopicID, "target_parent_id": rootID,
	})
	if res.IsError {
		t.Fatalf("DuplicateTopic: %s", textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = sheets // round-trip succeeded
}

func TestWalkTopicsStopsOnSiblings(t *testing.T) {
	// Regression: early stop must not continue into later sibling subtrees.
	root := &xmind.Topic{
		ID: "root",
		Children: &xmind.Children{
			Attached: []xmind.Topic{
				{ID: "a"},
				{ID: "b"},
			},
		},
	}
	var visited []string
	walkTopics(root, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		visited = append(visited, t.ID)
		return t.ID != "a"
	})
	for _, id := range visited {
		if id == "b" {
			t.Fatalf("should not visit b after stopping at a, visited=%v", visited)
		}
	}
}
