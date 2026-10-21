package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

func parseAddTopicResult(t *testing.T, res *mcp.CallToolResult) AddTopicResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out AddTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse add topic JSON: %v", err)
	}
	return out
}

func parseAddTopicsBulkResult(t *testing.T, res *mcp.CallToolResult) AddTopicsBulkResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out AddTopicsBulkResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse add topics bulk JSON: %v", err)
	}
	return out
}

func parseMoveTopicResult(t *testing.T, res *mcp.CallToolResult) MoveTopicResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out MoveTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse move topic JSON: %v", err)
	}
	return out
}

func parseDuplicateTopicResult(t *testing.T, res *mcp.CallToolResult) DuplicateTopicResponse {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got tool error: %s", textContent(t, res))
	}
	var out DuplicateTopicResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("parse duplicate topic JSON: %v", err)
	}
	return out
}

func readZipContentJSON(t *testing.T, path string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.Name != "content.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	t.Fatal("content.json not found in zip")
	return nil
}

func assertNestedBulkL1L2L3(t *testing.T, rt *xmind.Topic, bulk AddTopicsBulkResponse, rootID string) {
	t.Helper()
	if bulk.ParentID != rootID {
		t.Fatalf("unexpected parentId: got %q want %q", bulk.ParentID, rootID)
	}
	if rt.Children == nil || len(rt.Children.Attached) != 1 {
		t.Fatal("expected one top-level branch")
	}
	l1 := &rt.Children.Attached[0]
	if bulk.RootTopicIDs[0] != l1.ID {
		t.Fatalf("rootTopicIds[0] %q != L1 id %q", bulk.RootTopicIDs[0], l1.ID)
	}
	assertTopicTitleOneChild(t, l1, "L1", "L2")
	l2 := &l1.Children.Attached[0]
	assertTopicTitleOneChild(t, l2, "L2", "L3")
	l3 := &l2.Children.Attached[0]
	if l3.Title != "L3" {
		t.Fatalf("L3: %+v", l3)
	}
}

func assertTopicTitleOneChild(t *testing.T, topic *xmind.Topic, wantTitle, childTitle string) {
	t.Helper()
	if topic.Title != wantTitle {
		t.Fatalf("want title %q, got %+v", wantTitle, topic)
	}
	if topic.Children == nil || len(topic.Children.Attached) != 1 {
		t.Fatalf("%s: want one child, got %+v", wantTitle, topic.Children)
	}
	if topic.Children.Attached[0].Title != childTitle {
		t.Fatalf("%s: want child %q, got %+v", wantTitle, childTitle, topic.Children.Attached[0])
	}
}

func assertTopicInlineMetadata(t *testing.T, topic *xmind.Topic, wantTitle, wantNote, wantLink string) {
	t.Helper()
	if topic == nil {
		t.Fatal("topic not found after add")
	}
	if topic.Title != wantTitle {
		t.Fatalf("title: got %q", topic.Title)
	}
	assertTopicNotesPlainAndHTML(t, topic, wantNote)
	assertTopicLabelsAB(t, topic)
	assertTopicMarkersPriorityDone(t, topic)
	if topic.Href != wantLink {
		t.Fatalf("href: got %q want %q", topic.Href, wantLink)
	}
}

func assertTopicNotesPlainAndHTML(t *testing.T, topic *xmind.Topic, wantNote string) {
	t.Helper()
	if topic.Notes == nil || topic.Notes.Plain == nil || topic.Notes.Plain.Content != wantNote {
		t.Fatalf("notes plain: %+v", topic.Notes)
	}
	if topic.Notes.RealHTML == nil || topic.Notes.RealHTML.Content == "" {
		t.Fatalf("notes realHTML missing: %+v", topic.Notes)
	}
}

func assertTopicLabelsAB(t *testing.T, topic *xmind.Topic) {
	t.Helper()
	if len(topic.Labels) != 2 || topic.Labels[0] != "a" || topic.Labels[1] != "b" {
		t.Fatalf("labels: %+v", topic.Labels)
	}
}

func assertTopicMarkersPriorityDone(t *testing.T, topic *xmind.Topic) {
	t.Helper()
	if len(topic.Markers) != 2 || topic.Markers[0].MarkerID != "priority-1" || topic.Markers[1].MarkerID != "task-done" {
		t.Fatalf("markers: %+v", topic.Markers)
	}
}

func assertBulkMetaParentChild(t *testing.T, rt *xmind.Topic) {
	t.Helper()
	if rt.Children == nil || len(rt.Children.Attached) != 1 {
		t.Fatal("expected one top-level branch")
	}
	parent := &rt.Children.Attached[0]
	assertBulkMetaParent(t, parent)
	if parent.Children == nil || len(parent.Children.Attached) != 1 {
		t.Fatal("expected one child")
	}
	assertBulkMetaChild(t, &parent.Children.Attached[0])
}

func assertBulkMetaParent(t *testing.T, parent *xmind.Topic) {
	t.Helper()
	if parent.Title != "Parent" || parent.Notes == nil || parent.Notes.Plain == nil || parent.Notes.Plain.Content != "parent note" {
		t.Fatalf("parent metadata: %+v", parent)
	}
}

func assertBulkMetaChild(t *testing.T, child *xmind.Topic) {
	t.Helper()
	if child.Title != "Child" || len(child.Markers) != 1 || child.Markers[0].MarkerID != "task-ongoing" {
		t.Fatalf("child metadata: %+v", child)
	}
}

func assertMoveResponseBasic(t *testing.T, mv MoveTopicResponse, topicID, parentID string, pos int, sib int) {
	t.Helper()
	if mv.TopicID != topicID || mv.ParentID != parentID || mv.Position != pos || mv.SiblingCount != sib {
		t.Fatalf("unexpected move response: %+v", mv)
	}
}

func assertRootOnlyChildIs(t *testing.T, root *xmind.Topic, childID string) {
	t.Helper()
	if root.Children == nil || len(root.Children.Attached) != 1 || root.Children.Attached[0].ID != childID {
		t.Fatalf("root should have only %s as direct child; got %+v", childID, root.Children)
	}
}

func assertGammaHasOnlyChildAlpha(t *testing.T, root *xmind.Topic, gammaID, alphaID string) {
	t.Helper()
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

func assertSetTopicPropertiesRoot(t *testing.T, topic *xmind.Topic) {
	t.Helper()
	assertNoteBodyDiv(t, topic)
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

func assertNoteBodyDiv(t *testing.T, topic *xmind.Topic) {
	t.Helper()
	if topic.Notes == nil || topic.Notes.Plain == nil || topic.Notes.Plain.Content != "Note body" ||
		topic.Notes.RealHTML == nil || topic.Notes.RealHTML.Content != "<div>Note body</div>" {
		t.Fatalf("notes: %+v", topic.Notes)
	}
}

func setupSummaryRangeKitchenSink(t *testing.T, h *XMindHandler, path string, sid, rid string) {
	t.Helper()
	for _, title := range []string{"A", "B", "C"} {
		callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": title})
	}
	sheets, err := xmind.ReadMap(path)
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
}

func topicIDWithTitle(root *xmind.Topic, title string) string {
	if root.Children == nil {
		return ""
	}
	for i := range root.Children.Attached {
		if root.Children.Attached[i].Title == title {
			return root.Children.Attached[i].ID
		}
	}
	return ""
}

func mustSetTopicProperties(t *testing.T, h *XMindHandler, args map[string]any) {
	t.Helper()
	res := callTool(t, h.SetTopicProperties, args)
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
}

func assertRootMarkerCount(t *testing.T, path string, want int, desc string) {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	m := sheets[0].RootTopic.Markers
	if len(m) != want {
		t.Fatalf("%s: want %d markers, got %+v", desc, want, m)
	}
}

func assertRootMarkersOnlyB(t *testing.T, path string) {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	m := sheets[0].RootTopic.Markers
	if len(m) != 1 || m[0].MarkerID != "b" {
		t.Fatalf("markers then remove: want [b], got %+v", m)
	}
}

func assertDuplicateHappyPathTree(t *testing.T, root *xmind.Topic, mid, newID string) {
	t.Helper()
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
	dup := root.Children.Attached[1]
	if dup.Children == nil || len(dup.Children.Attached) != 1 ||
		dup.Children.Attached[0].Title != "Leaf" {
		t.Fatalf("duplicate should include Leaf: %+v", dup.Children)
	}
}

func assertPostAddTopicChildPersisted(t *testing.T, path, revBefore, newID string) {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
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

	assertPostAddTopicChildPersisted(t, path, revBefore, newID)
}

// callResult is one handler call's outcome, collected off the test goroutine.
type callResult struct {
	res *mcp.CallToolResult
	err error
}

// addTopicsConcurrently calls AddTopic n times at once under parentID, with titles T1..Tn,
// and returns each call's outcome by index.
func addTopicsConcurrently(h *XMindHandler, path, sheetID, parentID string, n int) []callResult {
	results := make([]callResult, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
				"path":      path,
				"sheet_id":  sheetID,
				"parent_id": parentID,
				"title":     fmt.Sprintf("T%d", i+1),
			}}}
			res, err := h.AddTopic(context.Background(), req)
			results[i] = callResult{res, err}
		})
	}
	close(start)
	wg.Wait()
	return results
}

// TestConcurrentAddTopicNoLostUpdates checks that overlapping AddTopic calls on one file,
// as the stdio server's worker pool runs them, each persist their topic instead of the
// last commit overwriting the others.
func TestConcurrentAddTopicNoLostUpdates(t *testing.T) {
	const n = 20
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sheetID, root := sheets[0].ID, &sheets[0].RootTopic
	before := len(root.Children.Attached)

	wantIDs := make(map[string]bool, n)
	for i, r := range addTopicsConcurrently(h, path, sheetID, root.ID, n) {
		if r.err != nil {
			t.Fatalf("call %d: protocol error: %v", i+1, r.err)
		}
		wantIDs[parseAddTopicResult(t, r.res).ID] = true
	}

	after, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	attached := after[0].RootTopic.Children.Attached
	if len(attached) != before+n {
		t.Fatalf("root has %d attached topics, want %d (%d before + %d added)", len(attached), before+n, before, n)
	}
	titles := make(map[string]bool, len(attached))
	for _, c := range attached {
		titles[c.Title] = true
		delete(wantIDs, c.ID)
	}
	for i := 1; i <= n; i++ {
		if !titles[fmt.Sprintf("T%d", i)] {
			t.Errorf("topic T%d missing after concurrent adds", i)
		}
	}
	if len(wantIDs) != 0 {
		t.Errorf("returned ids missing from the file: %v", wantIDs)
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
	raw := readZipContentJSON(t, path)
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

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rt := &sheets[0].RootTopic
	assertNestedBulkL1L2L3(t, rt, bulk, rootID)
}

func TestAddTopicWithInlineMetadata(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Root"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	const wantNote = "line1\nline2"
	wantLink := "https://example.com/doc"
	res = callTool(t, h.AddTopic, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"title":     "WithMeta",
		"notes":     wantNote,
		"labels":    []any{"a", "b"},
		"markers":   []any{"priority-1", "task-done"},
		"link":      wantLink,
	})
	if res.IsError {
		t.Fatalf("AddTopic: %s", textContent(t, res))
	}
	added := parseAddTopicResult(t, res)
	newID := added.ID

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, newID)
	assertTopicInlineMetadata(t, topic, "WithMeta", wantNote, wantLink)
}

func TestAddTopicsBulkWithPerTopicMetadata(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulk-meta.xmind")
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
			"title": "Parent",
			"notes": "parent note",
			"children": []any{
				map[string]any{
					"title":   "Child",
					"markers": []any{"task-ongoing"},
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
	if bulk.AddedCount != 2 {
		t.Fatalf("addedCount: got %d want 2", bulk.AddedCount)
	}

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	assertBulkMetaParentChild(t, &sheets[0].RootTopic)
}

func TestAddTopicsBulkMalformedMarkersToolError(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulk-bad.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopicsBulk, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"topics": []any{
			map[string]any{
				"title":   "T",
				"markers": []any{"ok", 42},
			},
		},
	})
	if !res.IsError {
		t.Fatalf("expected tool error, got: %s", textContent(t, res))
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "markers[1]") {
		t.Fatalf("expected markers index in error, got: %q", msg)
	}
}

func TestAddTopicsBulkNestedMalformedMarkersToolError(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bulk-nested-bad.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopicsBulk, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"topics": []any{
			map[string]any{
				"title": "P",
				"children": []any{
					map[string]any{
						"title":   "C",
						"markers": []any{"ok", 42},
					},
				},
			},
		},
	})
	if !res.IsError {
		t.Fatalf("expected tool error, got: %s", textContent(t, res))
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "markers[1]") {
		t.Fatalf("expected markers index in error, got: %q", msg)
	}
}

func TestAddTopicInvalidNotesType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-notes.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopic, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"title":     "T",
		"notes":     123,
	})
	if !res.IsError {
		t.Fatalf("expected tool error, got: %s", textContent(t, res))
	}
	if !strings.Contains(textContent(t, res), "invalid argument notes") {
		t.Fatalf("expected notes error, got: %q", textContent(t, res))
	}
}

func TestAddTopicInvalidLabelsElement(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-labels.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopic, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"title":     "T",
		"labels":    []any{"a", 1},
	})
	if !res.IsError {
		t.Fatalf("expected tool error, got: %s", textContent(t, res))
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "labels[1]") {
		t.Fatalf("expected labels index in error, got: %q", msg)
	}
}

func TestAddTopicInvalidLinkType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-link.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopic, map[string]any{
		"path":      path,
		"sheet_id":  sheets[0].ID,
		"parent_id": rootID,
		"title":     "T",
		"link":      99,
	})
	if !res.IsError {
		t.Fatalf("expected tool error, got: %s", textContent(t, res))
	}
	if !strings.Contains(textContent(t, res), "invalid argument link") {
		t.Fatalf("expected link error, got: %q", textContent(t, res))
	}
}

func TestAddTopicInlineRemoveMarkers(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "remove-m.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	rootID := sheets[0].RootTopic.ID

	res = callTool(t, h.AddTopic, map[string]any{
		"path":           path,
		"sheet_id":       sheets[0].ID,
		"parent_id":      rootID,
		"title":          "M",
		"markers":        []any{"priority-1", "task-done"},
		"remove_markers": []any{"priority-1"},
	})
	if res.IsError {
		t.Fatalf("AddTopic: %s", textContent(t, res))
	}
	added := parseAddTopicResult(t, res)

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, added.ID)
	if topic == nil {
		t.Fatal("topic not found")
	}
	if len(topic.Markers) != 1 || topic.Markers[0].MarkerID != "task-done" {
		t.Fatalf("markers: %+v", topic.Markers)
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
	if got, want := textContent(t, res), fmt.Sprintf("deleted topic %s; removed 0 relationship(s)", tid); got != want {
		t.Errorf("reply = %q, want %q", got, want)
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
	assertMoveResponseBasic(t, mv, bID, aID, 0, 1)
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	assertRootOnlyChildIs(t, root, aID)
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
	assertMoveResponseBasic(t, mv, alphaID, gammaID, 0, 1)

	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	assertRootOnlyChildIs(t, root, gammaID)
	assertGammaHasOnlyChildAlpha(t, root, gammaID, alphaID)
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
	assertMoveResponseBasic(t, mv, alphaID, gammaID, 0, 1)

	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	assertRootOnlyChildIs(t, root, gammaID)
	assertGammaHasOnlyChildAlpha(t, root, gammaID, alphaID)
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
	assertSetTopicPropertiesRoot(t, &sheets[0].RootTopic)
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
	setupSummaryRangeKitchenSink(t, h, path, sid, rid)
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	bID := topicIDWithTitle(&sheets[0].RootTopic, "B")
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
	root := &sheets[0].RootTopic
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

func assertRootNotesNilAfter(t *testing.T, path, label string) {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if sheets[0].RootTopic.Notes != nil {
		t.Fatalf("%s: want nil notes, got %+v", label, sheets[0].RootTopic.Notes)
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

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"notes": "keep", "labels": []any{"x"}, "markers": []any{"priority-1"}, "link": "https://a.example",
	})
	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "notes": "",
	})
	assertRootNotesNilAfter(t, path, "notes empty string")

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"notes": "n2", "labels": []any{"y"}, "markers": []any{"task-done"}, "link": "https://b.example",
	})
	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "notes": nil,
	})
	assertRootNotesNilAfter(t, path, "notes nil")

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "labels": []any{},
	})
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Labels) != 0 {
		t.Fatalf("labels: want empty, got %v", sheets[0].RootTopic.Labels)
	}

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "markers": []any{},
	})
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Markers) != 0 {
		t.Fatalf("markers: want empty, got %+v", sheets[0].RootTopic.Markers)
	}

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid, "link": "",
	})
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

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{"priority-1", "task-done"},
	})
	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{},
	})
	assertRootMarkerCount(t, path, 2, "empty remove_markers")

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{"unknown-id"},
	})
	assertRootMarkerCount(t, path, 2, "unknown remove id")

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{"priority-1"},
	})
	sheets, _ = xmind.ReadMap(path)
	m := sheets[0].RootTopic.Markers
	if len(m) != 1 || m[0].MarkerID != "task-done" {
		t.Fatalf("partial remove: want [task-done], got %+v", m)
	}

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{},
	})
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Markers) != 0 {
		t.Fatalf("markers empty array: want no markers, got %+v", sheets[0].RootTopic.Markers)
	}

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"remove_markers": []any{"still-unknown"},
	})
	sheets, _ = xmind.ReadMap(path)
	if len(sheets[0].RootTopic.Markers) != 0 {
		t.Fatalf("remove_markers with no markers: want empty, got %+v", sheets[0].RootTopic.Markers)
	}

	mustSetTopicProperties(t, h, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": tid,
		"markers": []any{"a", "b"}, "remove_markers": []any{"a"},
	})
	assertRootMarkersOnlyB(t, path)
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

func TestAddRelationshipSelfLoop(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rel_selfloop.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID

	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sid, "from_id": aID, "to_id": aID,
	})
	if !res.IsError {
		t.Fatal("expected error for self-loop relationship (from_id == to_id)")
	}
	if msg := textContent(t, res); !strings.Contains(msg, "must differ") {
		t.Fatalf("unexpected error message: %q", msg)
	}

	sheets, _ = xmind.ReadMap(path)
	if n := len(sheets[0].Relationships); n != 0 {
		t.Fatalf("expected 0 relationships after self-loop rejection, got %d: %+v", n, sheets[0].Relationships)
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

// setupTitleUneditedMap builds a fresh map with root + attached children A and B,
// sets titleUnedited=true on A, and persists it. It returns the handler, path,
// sheet id, root id, and the ids of A and B.
func setupTitleUneditedMap(t *testing.T) (h *XMindHandler, path, sid, rootID, idA, idB string) {
	t.Helper()
	h = NewXMindHandler()
	dir := t.TempDir()
	path = filepath.Join(dir, "uned.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid = sheets[0].ID
	rootID = sheets[0].RootTopic.ID
	idA = parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "A",
	})).ID
	idB = parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "B",
	})).ID
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topicA := findTopicByID(&sheets[0].RootTopic, idA)
	if topicA == nil {
		t.Fatal("topic A missing")
	}
	topicA.TitleUnedited = true
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}
	return h, path, sid, rootID, idA, idB
}

// assertTitleUneditedTrue re-reads path and asserts topic idA still carries
// titleUnedited=true.
func assertTitleUneditedTrue(t *testing.T, path, idA string) {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, idA)
	if topic == nil {
		t.Fatal("topic A missing after mutation")
	}
	if !topic.TitleUnedited {
		t.Fatalf("titleUnedited not preserved across unrelated mutation: %+v", topic)
	}
}

// TestUnrelatedMutationsPreserveTitleUnedited pins issue #44 item 4: an unrelated
// mutation must not clear titleUnedited. Only the clear-on-rename direction was
// tested before. Each subtest sets titleUnedited=true on topic A, runs a mutation
// that does not touch A's title, and asserts the flag survives.
func TestUnrelatedMutationsPreserveTitleUnedited(t *testing.T) {
	t.Run("SetTopicProperties", func(t *testing.T) {
		h, path, sid, _, idA, _ := setupTitleUneditedMap(t)
		res := callTool(t, h.SetTopicProperties, map[string]any{
			"path": path, "sheet_id": sid, "topic_id": idA, "notes": "edited",
		})
		if res.IsError {
			t.Fatal(textContent(t, res))
		}
		assertTitleUneditedTrue(t, path, idA)
	})

	t.Run("MoveTopic", func(t *testing.T) {
		h, path, sid, rootID, idA, _ := setupTitleUneditedMap(t)
		res := callTool(t, h.MoveTopic, map[string]any{
			"path": path, "sheet_id": sid, "topic_id": idA, "new_parent_id": rootID, "position": float64(1),
		})
		if res.IsError {
			t.Fatal(textContent(t, res))
		}
		assertTitleUneditedTrue(t, path, idA)
	})

	t.Run("ReorderChildren", func(t *testing.T) {
		h, path, sid, rootID, idA, idB := setupTitleUneditedMap(t)
		res := callTool(t, h.ReorderChildren, map[string]any{
			"path": path, "sheet_id": sid, "parent_id": rootID, "ordered_ids": []any{idB, idA},
		})
		if res.IsError {
			t.Fatal(textContent(t, res))
		}
		assertTitleUneditedTrue(t, path, idA)
	})
}

// TestAddTopicPersistsTitleUneditedFalse pins the positive direction of issue #44
// item 4: a freshly created topic must persist titleUnedited=false (absent in JSON
// via omitempty, read back as the zero value), confirming AddTopic never sets it.
func TestAddTopicPersistsTitleUneditedFalse(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "unedfalse.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sid := sheets[0].ID
	rootID := sheets[0].RootTopic.ID
	tid := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rootID, "title": "Child",
	})).ID
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	topic := findTopicByID(&sheets[0].RootTopic, tid)
	if topic == nil {
		t.Fatal("created topic missing")
	}
	if topic.TitleUnedited {
		t.Fatalf("freshly created topic should persist titleUnedited=false, got %+v", topic)
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
	dup := parseDuplicateTopicResult(t, res)
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
	assertDuplicateHappyPathTree(t, root, mid, newID)
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

// TestDuplicateTopicDropsOrphanedSummaryRange duplicates a topic carrying a summary range whose
// topicId is missing from its subtree, as older versions left behind when a summary topic was
// deleted. The copy leaves that range out instead of failing, a valid summary next to it is still
// copied, and the original is untouched.
func TestDuplicateTopicDropsOrphanedSummaryRange(t *testing.T) {
	h := NewXMindHandler()
	path := filepath.Join(t.TempDir(), "orphan.xmind")
	sid, rootID, mid, validSumTopicID := writeOrphanedSummaryMap(t, h, path)

	dup := parseDuplicateTopicResult(t, callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": mid, "target_parent_id": rootID,
	}))

	root := readFirstRoot(t, path)
	cp := findTopicByID(root, dup.NewRootID)
	if cp == nil || len(cp.Summaries) != 1 || cp.Children == nil || len(cp.Children.Summary) != 1 {
		t.Fatalf("copy should keep only the valid summary, got %+v", cp)
	}
	if got := cp.Summaries[0].TopicID; got != cp.Children.Summary[0].ID || got == validSumTopicID {
		t.Errorf("copied summary topicId = %q, want the copy's new summary topic id %q", got, cp.Children.Summary[0].ID)
	}
	if orig := findTopicByID(root, mid); orig == nil || len(orig.Summaries) != 2 {
		t.Errorf("original summaries changed: %+v", orig)
	}
}

// writeOrphanedSummaryMap creates a map at path whose root has one child, Mid, carrying two
// summary ranges over its child Leaf: one whose topicId names no topic, and a valid one.
func writeOrphanedSummaryMap(t *testing.T, h *XMindHandler, path string) (sheetID, rootID, midID, validSumTopicID string) {
	t.Helper()
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	mid := uuid.New().String()
	validSumTopicID = uuid.New().String()
	sheets[0].RootTopic.Children = &xmind.Children{
		Attached: []xmind.Topic{
			{
				ID:    mid,
				Title: "Mid",
				Summaries: []xmind.Summary{
					{ID: uuid.New().String(), Range: "(0,0)", TopicID: "ghost-id"},
					{ID: uuid.New().String(), Range: "(0,0)", TopicID: validSumTopicID},
				},
				Children: &xmind.Children{
					Attached: []xmind.Topic{{ID: uuid.New().String(), Title: "Leaf"}},
					Summary:  []xmind.Topic{{ID: validSumTopicID, Title: "S"}},
				},
			},
		},
	}
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}
	return sheets[0].ID, sheets[0].RootTopic.ID, mid, validSumTopicID
}

func TestParseSummaryRange(t *testing.T) {
	cases := []struct {
		in       string
		from, to int
		ok       bool
	}{
		{"(0,2)", 0, 2, true},
		{" ( 1 , 3 ) ", 1, 3, true},
		{"(2,2)", 2, 2, true},
		{"", 0, 0, false},       // too short, fails the paren/length guard
		{"[0,2]", 0, 0, false},  // not paren-wrapped
		{"(0123)", 0, 0, false}, // paren-wrapped but no comma
		{"(a,2)", 0, 0, false},  // non-numeric bound
		{"(2,0)", 0, 0, false},  // from greater than to
		{"(-1,2)", 0, 0, false}, // negative bound
	}
	for _, c := range cases {
		from, to, ok := parseSummaryRange(c.in)
		if ok != c.ok || from != c.from || to != c.to {
			t.Errorf("parseSummaryRange(%q) = (%d,%d,%v), want (%d,%d,%v)",
				c.in, from, to, ok, c.from, c.to, c.ok)
		}
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

// newABCMap creates a map whose root has attached children A, B, C. It returns the path,
// sheet ID, root ID and the children's IDs in order.
func newABCMap(t *testing.T, h *XMindHandler) (path, sheetID, rootID string, ids []string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "abc.xmind")
	if res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"}); res.IsError {
		t.Fatalf("CreateMap: %s", textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sheetID, rootID = sheets[0].ID, sheets[0].RootTopic.ID
	for _, title := range []string{"A", "B", "C"} {
		res := callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sheetID, "parent_id": rootID, "title": title})
		if res.IsError {
			t.Fatalf("AddTopic %s: %s", title, textContent(t, res))
		}
		ids = append(ids, parseAddTopicResult(t, res).ID)
	}
	return path, sheetID, rootID, ids
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// idempotentToolCase is a call to a tool annotated idempotentHint: true that changes a map
// built by newABCMap the first time it runs.
type idempotentToolCase struct {
	name string
	fn   func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	args func(path, sheetID, rootID string, ids []string) map[string]any
}

func idempotentToolCases(h *XMindHandler) []idempotentToolCase {
	return []idempotentToolCase{
		{"rename_topic", h.RenameTopic, func(path, sheetID, _ string, ids []string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "topic_id": ids[0], "title": "A2"}
		}},
		{"move_topic", h.MoveTopic, func(path, sheetID, _ string, ids []string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "topic_id": ids[2], "new_parent_id": ids[0]}
		}},
		{"reorder_children", h.ReorderChildren, func(path, sheetID, rootID string, ids []string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "parent_id": rootID, "ordered_ids": []any{ids[2], ids[1], ids[0]}}
		}},
		{"set_topic_properties", h.SetTopicProperties, func(path, sheetID, _ string, ids []string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "topic_id": ids[1], "labels": []any{"x"}, "notes": "n"}
		}},
		{"set_topic_properties_bulk", h.SetTopicPropertiesBulk, func(path, sheetID, _ string, ids []string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "topic_ids": []any{ids[0], ids[1]}, "labels": []any{"x"}}
		}},
	}
}

// TestIdempotentToolsRepeatDoesNotWrite checks the tools annotated idempotentHint: true leave
// the file byte-identical (no new revisionId, no rewrite) when a call is repeated.
func TestIdempotentToolsRepeatDoesNotWrite(t *testing.T) {
	h := NewXMindHandler()
	for _, tc := range idempotentToolCases(h) {
		t.Run(tc.name, func(t *testing.T) {
			path, sheetID, rootID, ids := newABCMap(t, h)
			args := tc.args(path, sheetID, rootID, ids)

			before := readFileBytes(t, path)
			first := callTool(t, tc.fn, args)
			if first.IsError {
				t.Fatalf("first call: %s", textContent(t, first))
			}
			afterFirst := readFileBytes(t, path)
			if bytes.Equal(before, afterFirst) {
				t.Fatal("first call did not change the file; test setup is not exercising a real change")
			}

			second := callTool(t, tc.fn, args)
			if second.IsError {
				t.Fatalf("second call: %s", textContent(t, second))
			}
			if got, want := textContent(t, second), textContent(t, first); got != want {
				t.Errorf("second response = %q, want same as first %q", got, want)
			}
			if !bytes.Equal(afterFirst, readFileBytes(t, path)) {
				t.Error("repeat call rewrote the file")
			}
		})
	}
}

// TestIdempotentToolsWriteFailure makes the map's directory read-only, so committing fails. A
// call that changes the map must return a "cannot save map" tool error and leave the file as it
// was; a repeat of a call already applied must still succeed, since it does not write.
func TestIdempotentToolsWriteFailure(t *testing.T) {
	h := NewXMindHandler()
	for _, tc := range idempotentToolCases(h) {
		t.Run(tc.name, func(t *testing.T) {
			path, sheetID, rootID, ids := newABCMap(t, h)
			args := tc.args(path, sheetID, rootID, ids)
			before := readFileBytes(t, path)

			dir := filepath.Dir(path)
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatal(err)
			}

			if res := callTool(t, tc.fn, args); !res.IsError || !strings.Contains(textContent(t, res), "cannot save map") {
				t.Fatalf("changing call in read-only dir: %q (isError %t), want a cannot save map tool error", textContent(t, res), res.IsError)
			}
			if !bytes.Equal(before, readFileBytes(t, path)) {
				t.Fatal("failed write changed the file")
			}

			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if res := callTool(t, tc.fn, args); res.IsError {
				t.Fatalf("first call: %s", textContent(t, res))
			}
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatal(err)
			}
			if res := callTool(t, tc.fn, args); res.IsError {
				t.Fatalf("repeat call in read-only dir: %s", textContent(t, res))
			}
		})
	}
}

// TestSetTopicPropertiesBulkInvalidPropertyNoWrite checks that a property error found while
// applying updates leaves the file untouched, even after earlier topics were updated in memory.
func TestSetTopicPropertiesBulkInvalidPropertyNoWrite(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, _, ids := newABCMap(t, h)
	before := readFileBytes(t, path)

	res := callTool(t, h.SetTopicPropertiesBulk, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_ids": []any{ids[0], ids[1]},
		"notes": "n", "labels": "not-an-array",
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-array labels")
	}
	if msg := textContent(t, res); !strings.Contains(msg, "labels: expected an array") {
		t.Errorf("error message = %q", msg)
	}
	if !bytes.Equal(before, readFileBytes(t, path)) {
		t.Error("failed bulk update rewrote the file")
	}
}

// TestMoveTopicSamePlaceKeepsSummary checks that moving a topic to where it already is leaves
// the file untouched. Detaching and re-inserting would drop a topic at a summary range edge
// out of the range.
func TestMoveTopicSamePlaceKeepsSummary(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, rootID, ids := newABCMap(t, h)

	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root := &sheets[0].RootTopic
	sumTopicID := uuid.New().String()
	root.Summaries = []xmind.Summary{{ID: uuid.New().String(), Range: "(0,2)", TopicID: sumTopicID}}
	root.Children.Summary = []xmind.Topic{{ID: sumTopicID, Title: "S"}}
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}
	before := readFileBytes(t, path)

	moves := []struct {
		name    string
		args    map[string]any
		wantPos int
	}{
		{"last topic, no position", map[string]any{"topic_id": ids[2]}, 2},
		{"first topic, position 0", map[string]any{"topic_id": ids[0], "position": float64(0)}, 0},
	}
	for _, m := range moves {
		args := map[string]any{"path": path, "sheet_id": sheetID, "new_parent_id": rootID}
		for k, v := range m.args {
			args[k] = v
		}
		res := callTool(t, h.MoveTopic, args)
		if res.IsError {
			t.Fatalf("%s: %s", m.name, textContent(t, res))
		}
		if got := parseMoveTopicResult(t, res); got.Position != m.wantPos || got.SiblingCount != 3 {
			t.Errorf("%s: response = %+v, want position %d of 3", m.name, got, m.wantPos)
		}
		if !bytes.Equal(before, readFileBytes(t, path)) {
			t.Errorf("%s: same-place move rewrote the file", m.name)
		}
	}

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root = &sheets[0].RootTopic
	if len(root.Summaries) != 1 || root.Summaries[0].Range != "(0,2)" || len(root.Children.Summary) != 1 {
		t.Fatalf("summary changed: summaries=%+v summaryTopics=%d", root.Summaries, len(root.Children.Summary))
	}
}

// Topics and relationships on kitchen-sink "Sheet 12 - Relationships" used by the summary,
// range and relationship integrity tests below.
const (
	kitchenSinkRelRootID             = "f3cf397d-d5df-4f3a-9136-4a5f97bca1d1"
	kitchenSinkRelMainTopic2ID       = "8e7bbcf3-0fc2-44d4-8aca-bd3d45d8f743" // ancestor of kitchenSinkRelSubtopic1bID
	kitchenSinkRelSubtopic1bID       = "468fa9d8-47b1-4b81-879f-9e1e02963ed5" // end1 of kitchenSinkRelPlainID
	kitchenSinkRelSubtopic1aID       = "374b38f2-55e5-4619-9ed4-ea3be23d2c54" // end2 of kitchenSinkRelLabelledID
	kitchenSinkRelMainTopic5ID       = "11bb41c5-b02e-400c-bad9-eabe0bc7f1b8" // parent of kitchenSinkRelSummaryOfLettersID
	kitchenSinkRelSummaryOfLettersID = "6a973ddc-5b28-44f5-8e80-46793541efc2"
	kitchenSinkRelFloating2ID        = "37c51b56-07e5-46ab-bc2b-ff008b875902"
	kitchenSinkRelFloating2ChildID   = "726bce28-d1dd-435e-bb4e-bd1b73c51cff"
	kitchenSinkRelPlainID            = "de0d5e7e-7126-4fed-b7db-2c95bf9dde71"
	kitchenSinkRelLabelledID         = "ac81423b-bee9-42cf-9f9d-2e10f327a9a3"
)

func readFirstRoot(t *testing.T, path string) *xmind.Topic {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	return &sheets[0].RootTopic
}

// addSummaryViaTool adds a summary over parentID's attached children from..to and returns the
// new summary topic's ID.
func addSummaryViaTool(t *testing.T, h *XMindHandler, path, sheetID, parentID string, from, to int) string {
	t.Helper()
	res := callTool(t, h.AddSummary, map[string]any{
		"path": path, "sheet_id": sheetID, "parent_id": parentID,
		"from_index": float64(from), "to_index": float64(to),
	})
	requireToolOK(t, "add summary", res)
	id, ok := strings.CutPrefix(textContent(t, res), "added summary topic id ")
	if !ok {
		t.Fatalf("unexpected add summary reply %q", textContent(t, res))
	}
	return id
}

func addRelationshipViaTool(t *testing.T, h *XMindHandler, path, sheetID, fromID, toID string) {
	t.Helper()
	res := callTool(t, h.AddRelationship, map[string]any{
		"path": path, "sheet_id": sheetID, "from_id": fromID, "to_id": toID,
	})
	requireToolOK(t, "add relationship", res)
}

// TestDeleteSummaryTopicRemovesDescriptor checks that deleting a summary topic also removes its
// range descriptor, so the parent is not left with a summary that points at nothing.
func TestDeleteSummaryTopicRemovesDescriptor(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, rootID, _ := newABCMap(t, h)
	sumID := addSummaryViaTool(t, h, path, sheetID, rootID, 0, 2)

	requireToolOK(t, "delete summary topic", callTool(t, h.DeleteTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_id": sumID,
	}))

	root := readFirstRoot(t, path)
	if len(root.Summaries) != 0 {
		t.Errorf("summary descriptors left behind: %+v", root.Summaries)
	}
	if root.Children == nil || len(root.Children.Summary) != 0 || len(root.Children.Attached) != 3 {
		t.Errorf("want 3 attached children and no summary topics, got %+v", root.Children)
	}
}

// TestDeleteSummaryTopicThenDuplicateParent deletes a summary topic from the fixture and then
// duplicates its parent, which fails if the deleted topic's range was left behind.
func TestDeleteSummaryTopicThenDuplicateParent(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sid := kitchenSinkRelationshipsSheetID

	requireToolOK(t, "delete summary topic", callTool(t, h.DeleteTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": kitchenSinkRelSummaryOfLettersID,
	}))
	parseDuplicateTopicResult(t, callTool(t, h.DuplicateTopic, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": kitchenSinkRelMainTopic5ID, "target_parent_id": kitchenSinkRelRootID,
	}))
}

// TestMoveTopicRejectsSummaryTopic checks that a summary topic cannot be moved: it belongs to its
// range on the parent, and moving it would leave that range pointing at nothing.
func TestMoveTopicRejectsSummaryTopic(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, rootID, ids := newABCMap(t, h)
	sumID := addSummaryViaTool(t, h, path, sheetID, rootID, 0, 2)
	before := readFileBytes(t, path)

	res := callTool(t, h.MoveTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_id": sumID, "new_parent_id": ids[0],
	})
	if !res.IsError || !strings.Contains(textContent(t, res), "cannot move a summary topic") {
		t.Fatalf("want a summary-topic move error, got %q", textContent(t, res))
	}
	if !bytes.Equal(before, readFileBytes(t, path)) {
		t.Error("rejected move rewrote the file")
	}
}

// TestMoveTopicSameParentMaxPosition pins the position range for a move within the same parent:
// the topic is detached before it is inserted, so the last valid position is the child count
// minus one.
func TestMoveTopicSameParentMaxPosition(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, rootID, ids := newABCMap(t, h)
	before := readFileBytes(t, path)

	res := callTool(t, h.MoveTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_id": ids[0], "new_parent_id": rootID, "position": float64(3),
	})
	if !res.IsError || !strings.Contains(textContent(t, res), "out of range") {
		t.Fatalf("position 3 of 3: want out of range, got %q", textContent(t, res))
	}
	if !bytes.Equal(before, readFileBytes(t, path)) {
		t.Error("failed move rewrote the file")
	}

	res = callTool(t, h.MoveTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_id": ids[0], "new_parent_id": rootID, "position": float64(2),
	})
	assertMoveResponseBasic(t, parseMoveTopicResult(t, res), ids[0], rootID, 2, 3)
	if got := topicTitles(readFirstRoot(t, path).Children.Attached); strings.Join(got, ",") != "B,C,A" {
		t.Errorf("order after move = %v, want [B C A]", got)
	}
}

const (
	rangeFixtureBoundaryID = "ranged-boundary"
	rangeFixtureMasterID   = "master-boundary"
)

// rangeFixture is a map whose root has attached children A, B, C and D, with X under A.
type rangeFixture struct {
	path, sheetID, rootID string
	ids                   map[string]string // title -> topic ID
}

// setupRangedBoundary builds a rangeFixture whose root has a boundary over rng, a master
// boundary that must never change, and, when withSummary is set, a summary over the same rng.
func setupRangedBoundary(t *testing.T, h *XMindHandler, rng string, withSummary bool) rangeFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ranges.xmind")
	requireToolOK(t, "create map", callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"}))
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	f := rangeFixture{path: path, sheetID: sheets[0].ID, rootID: sheets[0].RootTopic.ID, ids: map[string]string{}}
	add := func(parentID, title string) {
		res := callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": f.sheetID, "parent_id": parentID, "title": title})
		f.ids[title] = parseAddTopicResult(t, res).ID
	}
	for _, title := range []string{"A", "B", "C", "D"} {
		add(f.rootID, title)
	}
	add(f.ids["A"], "X")

	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	root := &sheets[0].RootTopic
	root.Boundaries = []xmind.Boundary{{ID: rangeFixtureBoundaryID, Range: rng}, {ID: rangeFixtureMasterID, Range: "master"}}
	if withSummary {
		sumTopicID := uuid.New().String()
		root.Summaries = []xmind.Summary{{ID: uuid.New().String(), Range: rng, TopicID: sumTopicID}}
		root.Children.Summary = []xmind.Topic{{ID: sumTopicID, Title: "S"}}
	}
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}
	return f
}

// rangeFixtureBoundaries returns the range of the fixture's ranged boundary ("" if it is gone)
// and whether its master boundary is still "master".
func rangeFixtureBoundaries(root *xmind.Topic) (ranged string, master bool) {
	for _, b := range root.Boundaries {
		switch b.ID {
		case rangeFixtureBoundaryID:
			ranged = b.Range
		case rangeFixtureMasterID:
			master = b.Range == "master"
		}
	}
	return ranged, master
}

// assertRootRanges checks the root's ranged boundary and, when withSummary is set, its summary
// against want ("" means the range was dropped), and that the master boundary is unchanged.
func assertRootRanges(t *testing.T, f rangeFixture, want string, withSummary bool) {
	t.Helper()
	root := readFirstRoot(t, f.path)
	gotBoundary, master := rangeFixtureBoundaries(root)
	if !master {
		t.Errorf("master boundary changed or lost: %+v", root.Boundaries)
	}
	if gotBoundary != want {
		t.Errorf("boundary range = %q, want %q", gotBoundary, want)
	}
	if !withSummary {
		return
	}
	var gotSummary string
	switch len(root.Summaries) {
	case 0:
	case 1:
		gotSummary = root.Summaries[0].Range
	default:
		t.Fatalf("want at most one summary, got %+v", root.Summaries)
	}
	if gotSummary != want {
		t.Errorf("summary range = %q, want %q", gotSummary, want)
	}
	hasSummaryTopic := root.Children != nil && len(root.Children.Summary) == 1
	if hasSummaryTopic != (want != "") {
		t.Errorf("summary topic present = %v, want %v", hasSummaryTopic, want != "")
	}
}

type rangeOp func(t *testing.T, h *XMindHandler, f rangeFixture) *mcp.CallToolResult

// addRangeTopic adds a topic under the root; pos nil appends.
func addRangeTopic(pos any) rangeOp {
	return func(t *testing.T, h *XMindHandler, f rangeFixture) *mcp.CallToolResult {
		args := map[string]any{"path": f.path, "sheet_id": f.sheetID, "parent_id": f.rootID, "title": "N"}
		if pos != nil {
			args["position"] = pos
		}
		return callTool(t, h.AddTopic, args)
	}
}

func duplicateRangeTopic(title string, pos float64) rangeOp {
	return func(t *testing.T, h *XMindHandler, f rangeFixture) *mcp.CallToolResult {
		return callTool(t, h.DuplicateTopic, map[string]any{
			"path": f.path, "sheet_id": f.sheetID, "topic_id": f.ids[title], "target_parent_id": f.rootID, "position": pos,
		})
	}
}

// moveRangeTopic moves title under newParentTitle ("" is the root); pos nil appends.
func moveRangeTopic(title, newParentTitle string, pos any) rangeOp {
	return func(t *testing.T, h *XMindHandler, f rangeFixture) *mcp.CallToolResult {
		parentID := f.rootID
		if newParentTitle != "" {
			parentID = f.ids[newParentTitle]
		}
		args := map[string]any{"path": f.path, "sheet_id": f.sheetID, "topic_id": f.ids[title], "new_parent_id": parentID}
		if pos != nil {
			args["position"] = pos
		}
		return callTool(t, h.MoveTopic, args)
	}
}

func deleteRangeTopic(title string) rangeOp {
	return func(t *testing.T, h *XMindHandler, f rangeFixture) *mcp.CallToolResult {
		return callTool(t, h.DeleteTopic, map[string]any{"path": f.path, "sheet_id": f.sheetID, "topic_id": f.ids[title]})
	}
}

// TestAttachedMutationsShiftRanges checks that summary and boundary ranges follow the attached
// children they cover when a tool inserts or removes a child of their parent. The root starts
// with children A, B, C, D.
func TestAttachedMutationsShiftRanges(t *testing.T) {
	cases := []struct {
		name        string
		rng         string // initial range of the boundary, and of the summary when withSummary
		withSummary bool
		op          rangeOp
		want        string // "" means the range is dropped
	}{
		{"add before range", "(1,2)", true, addRangeTopic(float64(0)), "(2,3)"},
		{"add at range start", "(1,2)", true, addRangeTopic(float64(1)), "(2,3)"},
		{"add inside range", "(1,2)", true, addRangeTopic(float64(2)), "(1,3)"},
		{"add just after range", "(1,2)", true, addRangeTopic(float64(3)), "(1,2)"},
		{"append", "(1,2)", true, addRangeTopic(nil), "(1,2)"},
		{"duplicate before range", "(1,2)", true, duplicateRangeTopic("D", 0), "(2,3)"},
		{"move into range", "(1,2)", true, moveRangeTopic("X", "", float64(2)), "(1,3)"},
		{"move out of range", "(1,2)", true, moveRangeTopic("B", "D", nil), "(1,1)"},
		{"move within parent", "(1,2)", true, moveRangeTopic("D", "", float64(0)), "(2,3)"},
		{"delete before range", "(1,2)", true, deleteRangeTopic("A"), "(0,1)"},
		{"delete inside range", "(1,2)", true, deleteRangeTopic("C"), "(1,1)"},
		{"delete after range", "(1,2)", true, deleteRangeTopic("D"), "(1,2)"},
		{"delete only member", "(1,1)", true, deleteRangeTopic("B"), ""},
		{"no summary, add before range", "(1,2)", false, addRangeTopic(float64(0)), "(2,3)"},
		{"no summary, delete before range", "(1,2)", false, deleteRangeTopic("A"), "(0,1)"},
		{"no summary, move out of range", "(1,1)", false, moveRangeTopic("B", "D", nil), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewXMindHandler()
			f := setupRangedBoundary(t, h, tc.rng, tc.withSummary)
			requireToolOK(t, tc.name, tc.op(t, h, f))
			assertRootRanges(t, f, tc.want, tc.withSummary)
		})
	}
}

// TestDeleteTopicRemovesRelationships checks that deleting a topic removes every relationship
// with an endpoint in the deleted subtree, keeps the others, and reports how many it removed.
func TestDeleteTopicRemovesRelationships(t *testing.T) {
	cases := []struct {
		name, topicID, keptRelID string
	}{
		{"endpoint", kitchenSinkRelSubtopic1aID, kitchenSinkRelPlainID},
		{"ancestor of endpoint", kitchenSinkRelMainTopic2ID, kitchenSinkRelLabelledID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewXMindHandler()
			path := copyFixture(t, kitchenSinkPath(t))
			res := callTool(t, h.DeleteTopic, map[string]any{
				"path": path, "sheet_id": kitchenSinkRelationshipsSheetID, "topic_id": tc.topicID,
			})
			requireToolOK(t, "delete", res)
			if got, want := textContent(t, res), fmt.Sprintf("deleted topic %s; removed 1 relationship(s)", tc.topicID); got != want {
				t.Errorf("reply = %q, want %q", got, want)
			}
			rels := findKitchenSinkSheetByID(t, path, kitchenSinkRelationshipsSheetID).Relationships
			if len(rels) != 1 || rels[0].ID != tc.keptRelID {
				t.Errorf("want only relationship %s left, got %+v", tc.keptRelID, rels)
			}
		})
	}
}

// newSummaryRelationshipMap builds an A, B, C map with a summary over B alone and a relationship
// from A to that summary topic. It returns the path, sheet ID, child IDs and summary topic ID.
func newSummaryRelationshipMap(t *testing.T, h *XMindHandler) (path, sheetID string, ids []string, sumID string) {
	t.Helper()
	path, sheetID, rootID, ids := newABCMap(t, h)
	sumID = addSummaryViaTool(t, h, path, sheetID, rootID, 1, 1)
	addRelationshipViaTool(t, h, path, sheetID, ids[0], sumID)
	return path, sheetID, ids, sumID
}

func assertNoRelationshipsAndTopicGone(t *testing.T, path, topicID string) {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if findTopicByID(&sheets[0].RootTopic, topicID) != nil {
		t.Fatalf("topic %s should have been removed", topicID)
	}
	if len(sheets[0].Relationships) != 0 {
		t.Errorf("want no relationships, got %+v", sheets[0].Relationships)
	}
}

// TestDeleteTopicRemovesRelationshipToCollapsedSummary deletes the only topic in a summary's
// range, which removes the summary topic too, so a relationship to it must go as well.
func TestDeleteTopicRemovesRelationshipToCollapsedSummary(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, ids, sumID := newSummaryRelationshipMap(t, h)

	res := callTool(t, h.DeleteTopic, map[string]any{"path": path, "sheet_id": sheetID, "topic_id": ids[1]})
	requireToolOK(t, "delete", res)
	if got, want := textContent(t, res), fmt.Sprintf("deleted topic %s; removed 1 relationship(s)", ids[1]); got != want {
		t.Errorf("reply = %q, want %q", got, want)
	}
	assertNoRelationshipsAndTopicGone(t, path, sumID)
}

// TestMoveTopicPrunesRelationshipToCollapsedSummary moves the only topic out of a summary's
// range, which removes the summary topic, so a relationship to it must go as well.
func TestMoveTopicPrunesRelationshipToCollapsedSummary(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, ids, sumID := newSummaryRelationshipMap(t, h)

	parseMoveTopicResult(t, callTool(t, h.MoveTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_id": ids[1], "new_parent_id": ids[0],
	}))
	assertNoRelationshipsAndTopicGone(t, path, sumID)
}

// TestDeleteTopicKeepsPreexistingDanglingRelationship checks that a delete only removes
// relationships to the topics it deleted, not ones that were already broken.
func TestDeleteTopicKeepsPreexistingDanglingRelationship(t *testing.T) {
	h := NewXMindHandler()
	path, sheetID, _, ids := newABCMap(t, h)
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	sheets[0].Relationships = []xmind.Relationship{{ID: "dangling", End1ID: "ghost-id", End2ID: ids[0]}}
	if err := xmind.WriteMap(path, sheets); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, h.DeleteTopic, map[string]any{"path": path, "sheet_id": sheetID, "topic_id": ids[2]})
	requireToolOK(t, "delete", res)
	if got, want := textContent(t, res), fmt.Sprintf("deleted topic %s; removed 0 relationship(s)", ids[2]); got != want {
		t.Errorf("reply = %q, want %q", got, want)
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets[0].Relationships) != 1 || sheets[0].Relationships[0].ID != "dangling" {
		t.Errorf("pre-existing relationship changed: %+v", sheets[0].Relationships)
	}
}

// TestDeleteFloatingTopicAndChild deletes topics outside the attached tree: a child of a floating
// topic, the floating topic itself, and a child of a summary topic.
func TestDeleteFloatingTopicAndChild(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sid := kitchenSinkRelationshipsSheetID
	for _, id := range []string{kitchenSinkRelFloating2ChildID, kitchenSinkRelFloating2ID} {
		requireToolOK(t, "delete "+id, callTool(t, h.DeleteTopic, map[string]any{"path": path, "sheet_id": sid, "topic_id": id}))
		if findTopicByID(&findKitchenSinkSheetByID(t, path, sid).RootTopic, id) != nil {
			t.Fatalf("topic %s still present", id)
		}
	}

	path, sheetID, rootID, _ := newABCMap(t, h)
	sumID := addSummaryViaTool(t, h, path, sheetID, rootID, 0, 2)
	childID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "parent_id": sumID, "title": "Under summary",
	})).ID
	requireToolOK(t, "delete child of summary topic", callTool(t, h.DeleteTopic, map[string]any{
		"path": path, "sheet_id": sheetID, "topic_id": childID,
	}))
	root := readFirstRoot(t, path)
	if findTopicByID(root, childID) != nil || findTopicByID(root, sumID) == nil {
		t.Errorf("want the child gone and the summary topic kept, got %+v", root.Children)
	}
}

func TestShiftRangeForInsert(t *testing.T) {
	inserts := []struct{ from, to, idx, wantFrom, wantTo int }{
		{1, 2, 0, 2, 3}, // before the range
		{1, 2, 1, 2, 3}, // at its start
		{1, 2, 2, 1, 3}, // inside it
		{1, 2, 3, 1, 2}, // just after it
		{0, 0, 0, 1, 1},
	}
	for _, c := range inserts {
		if f, to := shiftRangeForInsert(c.from, c.to, c.idx); f != c.wantFrom || to != c.wantTo {
			t.Errorf("insert at %d into (%d,%d) = (%d,%d), want (%d,%d)", c.idx, c.from, c.to, f, to, c.wantFrom, c.wantTo)
		}
	}
}

func TestShiftRangeForRemove(t *testing.T) {
	removes := []struct {
		from, to, idx, wantFrom, wantTo int
		wantKeep                        bool
	}{
		{1, 2, 0, 0, 1, true}, // before the range
		{1, 2, 1, 1, 1, true}, // its first child
		{1, 2, 2, 1, 1, true}, // its last child
		{1, 2, 3, 1, 2, true}, // after it
		{1, 1, 1, 1, 0, false},
	}
	for _, c := range removes {
		f, to, keep := shiftRangeForRemove(c.from, c.to, c.idx)
		if keep != c.wantKeep || (keep && (f != c.wantFrom || to != c.wantTo)) {
			t.Errorf("remove %d from (%d,%d) = (%d,%d, keep %v), want (%d,%d, keep %v)",
				c.idx, c.from, c.to, f, to, keep, c.wantFrom, c.wantTo, c.wantKeep)
		}
	}
}

// TestShiftRangeInsertThenRemove checks that removing a child just inserted at any index
// restores the original range.
func TestShiftRangeInsertThenRemove(t *testing.T) {
	for from := range 4 {
		for to := from; to < 4; to++ {
			for idx := range 6 {
				f, tt := shiftRangeForInsert(from, to, idx)
				if gf, gt, keep := shiftRangeForRemove(f, tt, idx); !keep || gf != from || gt != to {
					t.Errorf("insert then remove at %d: (%d,%d) became (%d,%d, keep %v)", idx, from, to, gf, gt, keep)
				}
			}
		}
	}
}
