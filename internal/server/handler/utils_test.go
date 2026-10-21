package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
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
	aID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "A",
	})).ID
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
	hubID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "title": "Hub",
	})).ID
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
	hub2ID := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
		"path": path2, "sheet_id": sid2, "parent_id": rid2, "title": "Hub2",
	})).ID

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

// TestFlattenImportRoundTripDeepMarkdown checks that Markdown flatten writes topics below heading
// level 6 as nested list items, with their notes inside the item, and that importing that output
// rebuilds the same tree.
func TestFlattenImportRoundTripDeepMarkdown(t *testing.T) {
	h := NewXMindHandler()
	path := filepath.Join(t.TempDir(), "deep.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	sid := sheets[0].ID
	addChild := func(parentID, title string) string {
		t.Helper()
		res := callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": parentID, "title": title})
		if res.IsError {
			t.Fatal(textContent(t, res))
		}
		return parseAddTopicResult(t, res).ID
	}
	// R -> T1 -> ... -> T9, plus T8b as a second child of T7.
	ids := []string{sheets[0].RootTopic.ID}
	for i := 1; i <= 9; i++ {
		ids = append(ids, addChild(ids[i-1], fmt.Sprintf("T%d", i)))
	}
	addChild(ids[7], "T8b")
	callTool(t, h.SetTopicProperties, map[string]any{"path": path, "sheet_id": sid, "topic_id": ids[7], "notes": "note a\nnote b"})

	outline := textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
		"path": path, "sheet_id": sid, "format": "markdown", "include_notes": true,
	}))
	const want = "# R\n\n## T1\n### T2\n#### T3\n##### T4\n###### T5\n" +
		"- T6\n  - T7\n    > note a\n    > note b\n\n    - T8\n      - T9\n    - T8b"
	if outline != want {
		t.Fatalf("Markdown outline:\n%s\nwant:\n%s", outline, want)
	}

	if res := callTool(t, h.ImportFromOutline, map[string]any{"path": path, "outline": outline}); res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, _ = xmind.ReadMap(path)
	textOutline := func(sheetID string) string {
		return textContent(t, callTool(t, h.FlattenToOutline, map[string]any{"path": path, "sheet_id": sheetID, "format": "text"}))
	}
	if orig, imported := textOutline(sid), textOutline(sheets[len(sheets)-1].ID); orig != imported {
		t.Fatalf("imported tree differs:\n--- original\n%s\n--- imported\n%s", orig, imported)
	}
}

// TestFlattenImportRoundTripKitchenSinkMarkdown flattens each kitchen-sink sheet to Markdown,
// imports that as a new sheet, and checks the new sheet has the same tree. The trees are compared
// as text outlines, which indent by real depth: Markdown would hide depth lost past heading level
// 6, because both flattens would lose it the same way. Trailing spaces are ignored, because import
// trims titles.
func TestFlattenImportRoundTripKitchenSinkMarkdown(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	flatten := func(t *testing.T, sheetID, format string) string {
		t.Helper()
		return textContent(t, callTool(t, h.FlattenToOutline, map[string]any{"path": path, "sheet_id": sheetID, "format": format}))
	}
	for _, sh := range sheets {
		t.Run(sh.Title, func(t *testing.T) {
			res := callTool(t, h.ImportFromOutline, map[string]any{"path": path, "outline": flatten(t, sh.ID, "markdown")})
			if res.IsError {
				t.Fatal(textContent(t, res))
			}
			after, err := xmind.ReadMap(path)
			if err != nil {
				t.Fatal(err)
			}
			want, got := trimLineEnds(flatten(t, sh.ID, "text")), trimLineEnds(flatten(t, after[len(after)-1].ID, "text"))
			if got != want {
				t.Fatalf("imported tree differs:\n--- original\n%s\n--- imported\n%s", want, got)
			}
		})
	}
}

// trimLineEnds removes trailing spaces from every line of s.
func trimLineEnds(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
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

// childTitles returns the titles of t's attached children, in order.
func childTitles(t *xmind.Topic) []string {
	if t == nil || t.Children == nil {
		return nil
	}
	titles := make([]string, 0, len(t.Children.Attached))
	for i := range t.Children.Attached {
		titles = append(titles, t.Children.Attached[i].Title)
	}
	return titles
}

// importNewSheet imports outline into a new sheet of a fresh map, with any extra arguments,
// and returns the response text and the new sheet.
func importNewSheet(t *testing.T, outline string, extra map[string]any) (string, *xmind.Sheet) {
	t.Helper()
	h := NewXMindHandler()
	path := filepath.Join(t.TempDir(), "imp.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "Base"})
	args := map[string]any{"path": path, "outline": outline}
	for k, v := range extra {
		args[k] = v
	}
	res := callTool(t, h.ImportFromOutline, args)
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 {
		t.Fatalf("expected 2 sheets after import, got %d", len(sheets))
	}
	return textContent(t, res), &sheets[1]
}

// TestImportFromOutlineMixedHeadingAndListNewSheet is the smoke-test step 39 regression: list
// items under a heading used to be dropped because the first line fixed the parse mode.
func TestImportFromOutlineMixedHeadingAndListNewSheet(t *testing.T) {
	text, sh := importNewSheet(t, "# Imported\n- Child 1\n- Child 2", nil)
	if !strings.Contains(text, "imported 3 topics") {
		t.Errorf("response = %q, want 3 topics", text)
	}
	if sh.Title != "Imported" || sh.RootTopic.Title != "Imported" {
		t.Errorf("sheet/root title = %q / %q, want Imported", sh.Title, sh.RootTopic.Title)
	}
	if got, want := childTitles(&sh.RootTopic), []string{"Child 1", "Child 2"}; !slices.Equal(got, want) {
		t.Errorf("root children = %v, want %v", got, want)
	}
}

func TestImportFromOutlineMixedNestedUnderParent(t *testing.T) {
	h := NewXMindHandler()
	path := filepath.Join(t.TempDir(), "mixed.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "X"})
	sheets, _ := xmind.ReadMap(path)
	sid, rid := sheets[0].ID, sheets[0].RootTopic.ID

	outline := "# R\n## A\n- a1\n  * a1x\ntext under A\n## B\n- b1"
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "sheet_id": sid, "parent_id": rid, "outline": outline,
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	if text := textContent(t, res); text != "imported 7 topics" {
		t.Errorf("response = %q, want imported 7 topics", text)
	}
	sheets, _ = xmind.ReadMap(path)
	root := &sheets[0].RootTopic
	if got := childTitles(root); !slices.Equal(got, []string{"R"}) {
		t.Fatalf("parent children = %v, want [R]", got)
	}
	r := &root.Children.Attached[0]
	if got := childTitles(r); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("R children = %v, want [A B]", got)
	}
	a, b := &r.Children.Attached[0], &r.Children.Attached[1]
	if got := childTitles(a); !slices.Equal(got, []string{"a1", "text under A"}) {
		t.Fatalf("A children = %v, want [a1 text under A]", got)
	}
	if got := childTitles(&a.Children.Attached[0]); !slices.Equal(got, []string{"a1x"}) {
		t.Errorf("a1 children = %v, want [a1x]", got)
	}
	if got := childTitles(b); !slices.Equal(got, []string{"b1"}) {
		t.Errorf("B children = %v, want [b1]", got)
	}
}

func TestImportFromOutlineNewSheetMultipleTopLevel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		extra     map[string]any
		wantTitle string
	}{
		{"default root", nil, "Imported Outline"},
		{"root_title", map[string]any{"root_title": "Solos"}, "Solos"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, sh := importNewSheet(t, "- Solo A\n- Solo B", tc.extra)
			if !strings.Contains(text, "imported 3 topics") {
				t.Errorf("response = %q, want 3 topics (synthesized root counts)", text)
			}
			if sh.Title != tc.wantTitle || sh.RootTopic.Title != tc.wantTitle {
				t.Errorf("sheet/root title = %q / %q, want %q", sh.Title, sh.RootTopic.Title, tc.wantTitle)
			}
			if got, want := childTitles(&sh.RootTopic), []string{"Solo A", "Solo B"}; !slices.Equal(got, want) {
				t.Errorf("root children = %v, want %v", got, want)
			}
		})
	}
}

func TestImportFromOutlineRootTitleSingleTopLevel(t *testing.T) {
	text, sh := importNewSheet(t, "# Only\n## Kid", map[string]any{"root_title": "Wrapper"})
	if !strings.Contains(text, "imported 3 topics") {
		t.Errorf("response = %q, want 3 topics", text)
	}
	if sh.Title != "Wrapper" || sh.RootTopic.Title != "Wrapper" {
		t.Errorf("sheet/root title = %q / %q, want Wrapper", sh.Title, sh.RootTopic.Title)
	}
	if got := childTitles(&sh.RootTopic); !slices.Equal(got, []string{"Only"}) {
		t.Fatalf("root children = %v, want [Only]", got)
	}
	if got := childTitles(&sh.RootTopic.Children.Attached[0]); !slices.Equal(got, []string{"Kid"}) {
		t.Errorf("Only children = %v, want [Kid]", got)
	}
}

func TestImportFromOutlineRootTitleWithSheetIDError(t *testing.T) {
	h := NewXMindHandler()
	path := filepath.Join(t.TempDir(), "rt-err.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	sheets, _ := xmind.ReadMap(path)
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "sheet_id": sheets[0].ID, "outline": "- x", "root_title": "Nope",
	})
	if !res.IsError {
		t.Fatal("expected tool error when root_title is set with sheet_id")
	}
	if msg := textContent(t, res); !strings.Contains(msg, "root_title only applies") {
		t.Errorf("error message = %q", msg)
	}
}

func TestImportFromOutlineRootTitleNotString(t *testing.T) {
	h := NewXMindHandler()
	path := filepath.Join(t.TempDir(), "rt-type.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	res := callTool(t, h.ImportFromOutline, map[string]any{
		"path": path, "outline": "- x", "root_title": float64(1),
	})
	if !res.IsError {
		t.Fatal("expected tool error for non-string root_title")
	}
	if msg := textContent(t, res); !strings.Contains(msg, "root_title") {
		t.Errorf("error message = %q", msg)
	}
}

// TestOutlineToolsRejectNonStringArgs checks that flatten_to_outline and import_from_outline
// reject a non-string format, topic_id, sheet_id or parent_id instead of treating it as
// absent, and leave the file unchanged.
func TestOutlineToolsRejectNonStringArgs(t *testing.T) {
	h := NewXMindHandler()
	for _, tc := range []struct {
		name, key string
		fn        func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args      func(path, sheetID string) map[string]any
	}{
		{"flatten format", "format", h.FlattenToOutline, func(path, sheetID string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "format": float64(5)}
		}},
		{"flatten topic_id", "topic_id", h.FlattenToOutline, func(path, sheetID string) map[string]any {
			return map[string]any{"path": path, "sheet_id": sheetID, "topic_id": float64(5)}
		}},
		{"import sheet_id", "sheet_id", h.ImportFromOutline, func(path, _ string) map[string]any {
			return map[string]any{"path": path, "outline": "- x", "sheet_id": float64(5)}
		}},
		{"import parent_id", "parent_id", h.ImportFromOutline, func(path, sheetID string) map[string]any {
			return map[string]any{"path": path, "outline": "- x", "sheet_id": sheetID, "parent_id": float64(5)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, sheetID, _, _ := newABCMap(t, h)
			before := readFileBytes(t, path)
			res := callTool(t, tc.fn, tc.args(path, sheetID))
			want := "invalid argument " + tc.key + ": expected a string"
			if got := textContent(t, res); !res.IsError || got != want {
				t.Errorf("result = %q (isError %t), want tool error %q", got, res.IsError, want)
			}
			if !bytes.Equal(before, readFileBytes(t, path)) {
				t.Error("call changed the file")
			}
		})
	}
}

// TestImportFromOutlineReadOnlyMap checks that both import paths, into a new sheet and under an
// existing topic, refuse a read-only map with a tool error and leave it unchanged.
func TestImportFromOutlineReadOnlyMap(t *testing.T) {
	h := NewXMindHandler()
	for _, tc := range []struct {
		name string
		args func(path, sheetID, rootID string) map[string]any
	}{
		{"new sheet", func(path, _, _ string) map[string]any {
			return map[string]any{"path": path, "outline": "- x"}
		}},
		{"under a topic", func(path, sheetID, rootID string) map[string]any {
			return map[string]any{"path": path, "outline": "- x", "sheet_id": sheetID, "parent_id": rootID}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, sheetID, rootID, _ := newABCMap(t, h)
			before := readFileBytes(t, path)
			t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
			if err := os.Chmod(path, 0o444); err != nil {
				t.Fatal(err)
			}
			res := callTool(t, h.ImportFromOutline, tc.args(path, sheetID, rootID))
			want := "cannot save map at " + path + ": the file is read-only"
			if got := textContent(t, res); !res.IsError || got != want {
				t.Errorf("result = %q (isError %t), want tool error %q", got, res.IsError, want)
			}
			if !bytes.Equal(before, readFileBytes(t, path)) {
				t.Error("call changed the file")
			}
		})
	}
}

func TestStripListMarker(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"- item", "item"},
		{"* item ", "item"},
		{"-", ""},
		{"*", ""},
		{"-item", "-item"},
		{"plain text", "plain text"},
	} {
		if got := stripListMarker(tc.in); got != tc.want {
			t.Errorf("stripListMarker(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFlattenImportRoundTripWithNotes checks that the note lines flatten_to_outline writes
// with include_notes ("> " in Markdown, "[note]" in text) do not become topics on import.
func TestFlattenImportRoundTripWithNotes(t *testing.T) {
	for _, format := range []string{"markdown", "text"} {
		t.Run(format, func(t *testing.T) {
			h := NewXMindHandler()
			path := filepath.Join(t.TempDir(), "notes-rt.xmind")
			callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
			sheets, _ := xmind.ReadMap(path)
			sid, rid := sheets[0].ID, sheets[0].RootTopic.ID
			c1 := parseAddTopicResult(t, callTool(t, h.AddTopic, map[string]any{
				"path": path, "sheet_id": sid, "parent_id": rid, "title": "C1",
			})).ID
			callTool(t, h.AddTopic, map[string]any{"path": path, "sheet_id": sid, "parent_id": rid, "title": "C2"})
			callTool(t, h.SetTopicProperties, map[string]any{"path": path, "sheet_id": sid, "topic_id": rid, "notes": "root note\nline 2"})
			callTool(t, h.SetTopicProperties, map[string]any{"path": path, "sheet_id": sid, "topic_id": c1, "notes": "child note"})

			withNotes := textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
				"path": path, "sheet_id": sid, "format": format, "include_notes": true,
			}))
			if !strings.Contains(withNotes, "child note") {
				t.Fatalf("flatten output has no note lines; test is not exercising notes:\n%s", withNotes)
			}
			res := callTool(t, h.ImportFromOutline, map[string]any{"path": path, "outline": withNotes})
			if res.IsError {
				t.Fatal(textContent(t, res))
			}
			sheets, _ = xmind.ReadMap(path)
			newSID := sheets[len(sheets)-1].ID

			flatten := func(sheetID string) string {
				return textContent(t, callTool(t, h.FlattenToOutline, map[string]any{
					"path": path, "sheet_id": sheetID, "format": format,
				}))
			}
			if orig, imported := flatten(sid), flatten(newSID); orig != imported {
				t.Fatalf("round trip with notes changed the tree:\n--- original\n%s\n--- imported\n%s", orig, imported)
			}
		})
	}
}
