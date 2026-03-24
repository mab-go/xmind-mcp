package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"
)

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
	msg := textContent(t, res)
	const prefix = "added topic id "
	if !strings.HasPrefix(msg, prefix) {
		t.Fatalf("unexpected message: %q", msg)
	}
	newID := strings.TrimPrefix(msg, prefix)
	if _, err := uuid.Parse(newID); err != nil {
		t.Fatalf("new topic id is not a valid UUID: %q", newID)
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
	if !strings.Contains(textContent(t, res), "added 3 topics") {
		t.Fatalf("unexpected message: %s", textContent(t, res))
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
	tid := strings.TrimPrefix(textContent(t, addRes), "added topic id ")

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
	tid := strings.TrimPrefix(textContent(t, addRes), "added topic id ")

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
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})), "added topic id ")
	bID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": aID, "title": "B",
	})), "added topic id ")

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
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})), "added topic id ")
	bID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "B",
	})), "added topic id ")

	res := callTool(t, h.MoveTopic, map[string]any{
		"path":          path,
		"sheet_id":      sid,
		"topic_id":      bID,
		"new_parent_id": aID,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
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
	alphaID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Alpha",
	})), "added topic id ")
	gammaID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Gamma",
	})), "added topic id ")

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
	gammaID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Gamma",
	})), "added topic id ")
	alphaID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Alpha",
	})), "added topic id ")

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
	idA := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})), "added topic id ")
	idB := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "B",
	})), "added topic id ")

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
	if len(topic.Labels) != 2 || topic.Labels[0] != "l1" {
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
	tid := strings.TrimPrefix(textContent(t, addRes), "added topic id ")
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
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})), "added topic id ")
	bID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})), "added topic id ")

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
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})), "added topic id ")
	bID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})), "added topic id ")

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
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})), "added topic id ")

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
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})), "added topic id ")
	bID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})), "added topic id ")

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
	tid := strings.TrimPrefix(textContent(t, addRes), "added topic id ")
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
	_ = strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})), "added topic id ")
	idB := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "B",
	})), "added topic id ")
	res := callTool(t, h.MoveTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": idB, "new_parent_id": rid, "position": float64(0),
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

func TestDeleteTopicAlsoRemovesDescendants(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "del3.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	l1 := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "L1",
	})), "added topic id ")
	l2 := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": l1, "title": "L2",
	})), "added topic id ")
	l3 := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": l2, "title": "L3",
	})), "added topic id ")
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
