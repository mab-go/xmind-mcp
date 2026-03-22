package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

func kitchenSinkPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "testdata", "kitchen-sink.xmind")
}

func callTool(t *testing.T, fn func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	res, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return res
}

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func TestListSheetsKitchenSink(t *testing.T) {
	h := NewXMindHandler()
	res := callTool(t, h.ListSheets, map[string]any{"path": kitchenSinkPath(t)})
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", textContent(t, res))
	}
	var out listSheetsResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Sheets) != 15 {
		t.Fatalf("sheet count: got %d want 15", len(out.Sheets))
	}
}

func TestCreateMapOpenRead(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "new.xmind")
	res := callTool(t, h.CreateMap, map[string]any{
		"path":        path,
		"root_title":  "Root A",
		"sheet_title": "My Sheet",
	})
	if res.IsError {
		t.Fatalf("CreateMap: %s", textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatalf("ReadMap: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("sheet count: got %d want 1", len(sheets))
	}
	if got, want := sheets[0].Title, "My Sheet"; got != want {
		t.Fatalf("sheet title: got %q want %q", got, want)
	}
	if got, want := sheets[0].RootTopic.Title, "Root A"; got != want {
		t.Fatalf("root title: got %q want %q", got, want)
	}

	openRes := callTool(t, h.OpenMap, map[string]any{"path": path})
	if openRes.IsError {
		t.Fatalf("OpenMap after CreateMap: %s", textContent(t, openRes))
	}
	var om openMapResponse
	if err := json.Unmarshal([]byte(textContent(t, openRes)), &om); err != nil {
		t.Fatalf("OpenMap unmarshal: %v", err)
	}
	if om.SheetCount != 1 {
		t.Fatalf("OpenMap sheetCount: got %d want 1", om.SheetCount)
	}
	if len(om.Sheets) != 1 || om.Sheets[0].Title != "My Sheet" || om.Sheets[0].RootTopicTitle != "Root A" {
		t.Fatalf("OpenMap sheets: %+v", om.Sheets)
	}
}

func TestCreateMapFileExists(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.xmind")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if !res.IsError {
		t.Fatal("expected tool error when file exists")
	}
}

func TestAddSheetDeleteSheet(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "two.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatalf("CreateMap: %s", textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 {
		t.Fatalf("want 1 sheet after create, got %d", len(sheets))
	}

	res = callTool(t, h.AddSheet, map[string]any{
		"path":       path,
		"title":      "Second",
		"root_title": "R2",
	})
	if res.IsError {
		t.Fatalf("AddSheet: %s", textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 {
		t.Fatalf("want 2 sheets after add, got %d", len(sheets))
	}

	secondID := sheets[1].ID
	res = callTool(t, h.DeleteSheet, map[string]any{"path": path, "sheet_id": secondID})
	if res.IsError {
		t.Fatalf("DeleteSheet: %s", textContent(t, res))
	}
	sheets, err = xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 {
		t.Fatalf("want 1 sheet after delete, got %d", len(sheets))
	}
}

func expectedKitchenSinkSheetTitles() []string {
	return []string{
		"Sheet 1 - Mind Map",
		"Sheet 2 - Logic Chart",
		"Sheet 3 - Tree Chart",
		"Sheet 4 - Org Chart",
		"Sheet 5 - Fishbone",
		"Sheet 6 - Timeline",
		"Sheet 7 - Brace Map",
		"Sheet 8 - Tree Table",
		"Sheet 9 - Matrix",
		"Sheet 10 - Topic Properties",
		"Sheet 11 - Markers",
		"Sheet 12 - Relationships",
		"Sheet 13 - Tasks",
		"Sheet 14 - Visual Styling",
		"Sheet 15 - Edge Cases",
	}
}

func TestOpenMapKitchenSink(t *testing.T) {
	h := NewXMindHandler()
	res := callTool(t, h.OpenMap, map[string]any{"path": kitchenSinkPath(t)})
	if res.IsError {
		t.Fatalf("OpenMap: %s", textContent(t, res))
	}
	var om openMapResponse
	if err := json.Unmarshal([]byte(textContent(t, res)), &om); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if om.SheetCount != 15 {
		t.Fatalf("sheetCount: got %d want 15", om.SheetCount)
	}
	if len(om.Sheets) != 15 {
		t.Fatalf("sheets len: got %d", len(om.Sheets))
	}
	wantTitles := expectedKitchenSinkSheetTitles()
	for i := range om.Sheets {
		if om.Sheets[i].Title != wantTitles[i] {
			t.Fatalf("sheet[%d] title: got %q want %q", i, om.Sheets[i].Title, wantTitles[i])
		}
	}
	if om.Sheets[0].TopicCount < 1 {
		t.Fatalf("expected non-zero topicCount for sheet 1, got %d", om.Sheets[0].TopicCount)
	}
}

func TestDeleteSheetNonexistentID(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "two.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	callTool(t, h.AddSheet, map[string]any{"path": path, "title": "S2", "root_title": "R2"})
	res := callTool(t, h.DeleteSheet, map[string]any{
		"path": path, "sheet_id": "00000000-0000-0000-0000-000000000000",
	})
	if !res.IsError {
		t.Fatal("expected tool error for nonexistent sheet_id")
	}
	msg := textContent(t, res)
	if !strings.Contains(msg, "sheet not found") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestAddSheetMissingArgs(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "m.xmind")
	callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	res := callTool(t, h.AddSheet, map[string]any{"path": path, "root_title": "R2"})
	if !res.IsError {
		t.Fatal("expected error when title is missing")
	}
	res = callTool(t, h.AddSheet, map[string]any{"title": "T", "root_title": "R2"})
	if !res.IsError {
		t.Fatal("expected error when path is missing")
	}
}

func TestDeleteLastSheetError(t *testing.T) {
	h := NewXMindHandler()
	dir := t.TempDir()
	path := filepath.Join(dir, "one.xmind")
	res := callTool(t, h.CreateMap, map[string]any{"path": path, "root_title": "R"})
	if res.IsError {
		t.Fatalf("CreateMap: %s", textContent(t, res))
	}
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	res = callTool(t, h.DeleteSheet, map[string]any{"path": path, "sheet_id": sheets[0].ID})
	if !res.IsError {
		t.Fatal("expected tool error when deleting last sheet")
	}
}
