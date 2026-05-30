package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"
)

// firstSheetID returns the ID of the first sheet in the map at path.
func firstSheetID(t *testing.T, path string) string {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) == 0 {
		t.Fatal("no sheets")
	}
	return sheets[0].ID
}

// zipEntryNamesAt returns the sorted entry-name set of the zip at path.
func zipEntryNamesAt(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(zr.File))
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	slices.Sort(names)
	return names
}

// TestIntegration_FindAndReplaceNoMatchClosesCleanly exercises the conditional-write path:
// FindAndReplace opens the map for update but commits nothing when there are no matches.
// The defer mw.Close() must still release the handle, so a later mutation on the same path
// succeeds and the file is left byte-identical by the no-op call.
func TestIntegration_FindAndReplaceNoMatchClosesCleanly(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))

	before := readZipContentJSON(t, path)
	res := callTool(t, h.FindAndReplace, map[string]any{
		"path": path, "find": "zzz_no_such_text_zzz", "replace": "x",
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
	if after := readZipContentJSON(t, path); !bytes.Equal(before, after) {
		t.Fatal("no-match FindAndReplace must not modify content.json")
	}

	// A subsequent mutation on the same path must succeed (proves the handle was released).
	mres := callTool(t, h.AddFloatingTopic, map[string]any{
		"path": path, "sheet_id": firstSheetID(t, path), "title": "AfterNoOp",
	})
	if mres.IsError {
		t.Fatal(textContent(t, mres))
	}
}

// TestIntegration_MutateBadSheetIDToolError exercises the post-open tool-error path: the map
// opens for update but the sheet lookup fails. The handler must return a sheet-not-found tool
// error (its deferred mw.Close releases the handle without committing).
func TestIntegration_MutateBadSheetIDToolError(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))

	res := callTool(t, h.RenameTopic, map[string]any{
		"path": path, "sheet_id": "no-such-sheet", "topic_id": kitchenSinkAlphaTopicID, "title": "X",
	})
	if !res.IsError {
		t.Fatal("expected a tool error for an unknown sheet_id")
	}
	if msg := textContent(t, res); !bytes.Contains([]byte(msg), []byte("sheet not found")) {
		t.Fatalf("expected sheet-not-found tool error, got: %s", msg)
	}
}

// TestIntegration_CommitPreservesZipEntries asserts the single-open Commit path preserves the
// full set of non-content.json zip entries (embedded resources) through a handler mutation.
func TestIntegration_CommitPreservesZipEntries(t *testing.T) {
	h := NewXMindHandler()
	path := copyFixture(t, kitchenSinkPath(t))
	before := zipEntryNamesAt(t, path)

	res := callTool(t, h.RenameTopic, map[string]any{
		"path": path, "sheet_id": firstSheetID(t, path), "topic_id": kitchenSinkAlphaTopicID, "title": "EntriesPreserved",
	})
	if res.IsError {
		t.Fatal(textContent(t, res))
	}
	if after := zipEntryNamesAt(t, path); !slices.Equal(before, after) {
		t.Fatalf("zip entry set changed:\nbefore: %v\nafter:  %v", before, after)
	}
}
