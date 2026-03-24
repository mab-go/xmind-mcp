package handler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"
)

func TestFlattenToOutlineMarkdownKitchenSink(t *testing.T) {
	h := NewXMindHandler()
	sid := firstKitchenSinkSheetID(t)
	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path":     kitchenSinkPath(t),
		"sheet_id": sid,
		"format":   "markdown",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	out := textContent(t, res)
	if !strings.HasPrefix(out, "# Central Topic\n\n") {
		prefixLen := 40
		if len(out) < prefixLen {
			prefixLen = len(out)
		}
		t.Fatalf("expected markdown root heading, got %q", out[:prefixLen])
	}
	if !strings.Contains(out, "##") {
		t.Fatalf("expected heading depth for children: %s", out)
	}
}

func TestFlattenToOutlineText(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Root"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "B"})

	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "text",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	out := textContent(t, res)
	want := "Root\n  A\n  B"
	if strings.TrimSpace(out) != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestFlattenToOutlineSubtree(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	aID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})), "added topic id ")
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": aID, "title": "Leaf"})

	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": aID, "format": "markdown",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	out := textContent(t, res)
	if !strings.HasPrefix(out, "# A\n\n## Leaf") {
		t.Fatalf("unexpected subtree output: %q", out)
	}
}

func TestFlattenToOutlineSkipsDetached(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "det.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "A"})
	callTool(t, h.AddFloatingTopic, map[string]any{"path": path, "sheet_id": sid, "title": "Float"})

	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "text",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	out := textContent(t, res)
	if strings.Contains(out, "Float") {
		t.Fatalf("detached topic should not appear: %q", out)
	}
}

func TestFlattenToOutlineInvalidFormat(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "badfmt.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "format": "xml",
	})
	if !res.IsError {
		t.Fatal("expected invalid format error")
	}
}

func TestFlattenToOutlineIncludeNotesMarkdown(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes-md.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": rid, "notes": "Line1\nLine2",
	})
	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "markdown", "include_notes": true,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	out := textContent(t, res)
	want := "# R\n> Line1\n> Line2"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestFlattenToOutlineIncludeNotesText(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes-txt.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.SetTopicProperties, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": rid, "notes": "Line1\nLine2",
	})
	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "text", "include_notes": true,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	out := textContent(t, res)
	want := "R\n    [note] Line1\n    [note] Line2"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestFlattenToOutlineInvalidIncludeNotesType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-notes.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "include_notes": "yes",
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-boolean include_notes")
	}
}

func TestImportFromOutlineHeadingMode(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "imp1.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "X"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID

	outline := "# Root\n## A\n### B\n## C\n"
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "outline": outline,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	rt := &sheets[0].RootTopic
	if rt.Children == nil || len(rt.Children.Attached) != 1 || rt.Children.Attached[0].Title != "Root" {
		t.Fatalf("expected one top-level Root: %+v", rt.Children)
	}
	root := &rt.Children.Attached[0]
	if len(root.Children.Attached) != 2 {
		t.Fatalf("want 2 children under Root, got %d", len(root.Children.Attached))
	}
}

func TestImportFromOutlineListMode(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "imp2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "X"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	outline := "- Root\n  - A\n    - B\n  - C\n"
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "outline": outline,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	top := sheets[0].RootTopic.Children.Attached[0]
	if top.Title != "Root" {
		t.Fatalf("top title: %q", top.Title)
	}
	if len(top.Children.Attached) != 2 {
		t.Fatalf("want 2 branches, got %d", len(top.Children.Attached))
	}
}

func TestImportFromOutlinePlainMode(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "imp3.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "X"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	outline := "Line0\n  Line1\n    Line2\n"
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "outline": outline,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	top := sheets[0].RootTopic.Children.Attached[0]
	if top.Title != "Line0" || len(top.Children.Attached) != 1 {
		t.Fatalf("unexpected tree: %+v", top)
	}
}

func TestImportFromOutlineNewSheet(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "newsh.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "OldRoot"})
	outline := "# FirstSheet\n## A\n### B\n"
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "outline": outline,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 {
		t.Fatalf("expected 2 sheets, got %d", len(sheets))
	}
	sh := &sheets[1]
	if sh.Title != "FirstSheet" || sh.RootTopic.Title != "FirstSheet" {
		t.Fatalf("sheet/root title: %+v / %+v", sh.Title, sh.RootTopic.Title)
	}
	if sh.RootTopic.Children == nil || len(sh.RootTopic.Children.Attached) != 1 {
		t.Fatalf("expected one child A: %+v", sh.RootTopic.Children)
	}
	if sh.RootTopic.Children.Attached[0].Title != "A" {
		t.Fatal("expected child A")
	}
}

func TestImportFromOutlineSheetRootOnly(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "improot.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	outline := "- Only\n"
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "sheet_id": sid, "outline": outline,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	ch := sheets[0].RootTopic.Children.Attached
	if len(ch) != 1 || ch[0].Title != "Only" {
		t.Fatalf("got %+v", ch)
	}
}

func TestImportFromOutlineParentWithoutSheet(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "badpar.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	rid := sheets[0].RootTopic.ID
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "outline": "- x\n", "parent_id": rid,
	})
	if !res.IsError {
		t.Fatal("expected error when parent_id is set without sheet_id")
	}
}

func TestFlattenImportRoundTripMarkdownUnderParent(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rtpar.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	hubID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Hub",
	})), "added topic id ")
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": hubID, "title": "Branch A"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": hubID, "title": "Branch B"})

	flat1 := textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "topic_id": hubID, "format": "markdown",
	}))

	path2 := filepath.Join(dir, "rtpar2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path2, "root_title": "R2"})
	sheets2, _ := xmind.ReadMap(path2)
	sid2 := sheets2[0].ID
	rid2 := sheets2[0].RootTopic.ID
	hub2ID := strings.TrimPrefix(textContent(t, callTool(t, h.AddTopic, map[string]any{
		"path": path2, "sheet_id": sid2, "parent_id": rid2, "title": "Hub2",
	})), "added topic id ")

	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path2, "sheet_id": sid2, "parent_id": hub2ID, "outline": flat1,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets2, _ = xmind.ReadMap(path2)
	hub2 := findTopicByID(&sheets2[0].RootTopic, hub2ID)
	if hub2 == nil || hub2.Children == nil || len(hub2.Children.Attached) != 1 {
		t.Fatalf("expected one imported subtree under Hub2: %+v", hub2)
	}
	importedRootID := hub2.Children.Attached[0].ID
	flat2 := textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
		"path": path2, "sheet_id": sid2, "topic_id": importedRootID, "format": "markdown",
	}))
	if flat1 != flat2 {
		t.Fatalf("round-trip under parent mismatch:\n---\n%s\n---\n%s\n", flat1, flat2)
	}
}

func TestFlattenImportRoundTripMarkdown(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "rt.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Central Topic"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Branch A"})
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Branch B"})

	flat1 := textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "markdown",
	}))

	path2 := filepath.Join(dir, "rt2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path2, "root_title": "Placeholder"})
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path2, "outline": flat1,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets2, _ := xmind.ReadMap(path2)
	if len(sheets2) < 2 {
		t.Fatalf("expected new sheet from import, got %d sheets", len(sheets2))
	}
	sid2 := sheets2[1].ID
	flat2 := textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
		"path": path2, "sheet_id": sid2, "format": "markdown",
	}))
	if flat1 != flat2 {
		t.Fatalf("round-trip mismatch:\n---\n%s\n---\n%s\n", flat1, flat2)
	}
}

func TestFindAndReplaceEmptyFind(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frem.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "find": "", "replace": "x",
	})
	if !res.IsError {
		t.Fatal("expected error for empty find")
	}
}

func TestFindAndReplaceReplaceLiteralDollar(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frdol.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "p$x"})

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "p", "replace": "a$1b",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	ch := sheets[0].RootTopic.Children.Attached[0]
	want := "a$1b$x"
	if ch.Title != want {
		t.Fatalf("replace must be literal: got %q want %q", ch.Title, want)
	}
}

func TestFindAndReplaceExactPartialNoMatch(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frex2.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "PrefixExactSuffix"})

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "exact", "replace": "X", "exact_match": true,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	var resp struct {
		ChangedCount int `json:"changedCount"`
	}
	if err := json.Unmarshal([]byte(textContent(t, res)), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ChangedCount != 0 {
		t.Fatalf("exact_match should not replace partial title matches, changed=%d", resp.ChangedCount)
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RootTopic.Children.Attached[0].Title != "PrefixExactSuffix" {
		t.Fatal("title should be unchanged")
	}
}

func TestFindAndReplaceExactNoTitleChange(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frexnoop.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Same"})
	sheets, _ = xmind.ReadMap(path)
	rev := sheets[0].RevisionID

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "same", "replace": "Same", "exact_match": true,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	var resp struct {
		ChangedCount int `json:"changedCount"`
	}
	if err := json.Unmarshal([]byte(textContent(t, res)), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ChangedCount != 0 {
		t.Fatalf("exact match with identical resulting title should not count as a change, got %d", resp.ChangedCount)
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RevisionID != rev {
		t.Fatal("revision should not change when exact replace yields same string")
	}
	if sheets[0].RootTopic.Children.Attached[0].Title != "Same" {
		t.Fatalf("title should be unchanged: %q", sheets[0].RootTopic.Children.Attached[0].Title)
	}
}

func TestFindAndReplaceExactMatchWrongType(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frbool.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "find": "x", "replace": "y", "exact_match": "true",
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-boolean exact_match")
	}
}

func TestFindAndReplaceSubstring(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "fr.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Budget Q1"})

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "budget", "replace": "Forecast",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	var resp struct {
		ChangedCount int `json:"changedCount"`
		Changes      []struct {
			OldTitle string `json:"oldTitle"`
			NewTitle string `json:"newTitle"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(textContent(t, res)), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ChangedCount != 1 || resp.Changes[0].NewTitle != "Forecast Q1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	sheets, _ = xmind.ReadMap(path)
	topic := findTopicByID(&sheets[0].RootTopic, sheets[0].RootTopic.Children.Attached[0].ID)
	if topic.Title != "Forecast Q1" {
		t.Fatalf("title: %q", topic.Title)
	}
}

func TestFindAndReplaceExact(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frex.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	rid := sheets[0].RootTopic.ID
	callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "Exact"})

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "exact", "replace": "Replaced", "exact_match": true,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RootTopic.Title != "R" {
		t.Fatalf("root title should be unchanged: %q", sheets[0].RootTopic.Title)
	}
	ch := sheets[0].RootTopic.Children.Attached[0]
	if ch.Title != "Replaced" {
		t.Fatalf("got %q", ch.Title)
	}
}

func TestFindAndReplaceNoMatchNoWrite(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frno.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	rev := sheets[0].RevisionID
	sid := sheets[0].ID

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "nope", "replace": "x",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	var resp struct {
		ChangedCount int `json:"changedCount"`
	}
	if err := json.Unmarshal([]byte(textContent(t, res)), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ChangedCount != 0 {
		t.Fatalf("expected 0 changes, got %d", resp.ChangedCount)
	}
	sheets, _ = xmind.ReadMap(path)
	if sheets[0].RevisionID != rev {
		t.Fatal("revision should not change when nothing matched")
	}
}

func TestFindAndReplaceJSONShape(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "frjs.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "A"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID

	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "sheet_id": sid, "find": "A", "replace": "B", "exact_match": true,
	})
	raw := textContent(t, res)
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["changedCount"]; !ok {
		t.Fatal("missing changedCount")
	}
	if _, ok := m["changes"]; !ok {
		t.Fatal("missing changes")
	}
}
