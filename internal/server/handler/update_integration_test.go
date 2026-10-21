package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

// writeContentJSONMap writes a map whose only entry is content.json, holding contentJSON, to a
// new temp directory and returns its path.
func writeContentJSONMap(t *testing.T, contentJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.xmind")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("content.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(contentJSON)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

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

// TestIntegration_UndecodableFieldBlocksSavesOnly checks a map whose topic c2 has wrong-shaped
// labels and notes, fields the tools edit. Reads, and calls that change nothing, still succeed.
// Every call that would save is refused with a tool error naming the field, even one that
// deletes c2. The file never changes.
func TestIntegration_UndecodableFieldBlocksSavesOnly(t *testing.T) {
	const content = `[{"id":"s1","class":"sheet","title":"S","rootTopic":{"id":"r","title":"R",
		"children":{"attached":[{"id":"c1","title":"c1"},{"id":"c2","title":"c2","labels":"x","notes":[1]}]}}}]`
	h := NewXMindHandler()
	path := writeContentJSONMap(t, content)
	before := readZipContentJSON(t, path)
	refused := fmt.Sprintf("cannot save map at %s: Topic.labels on topic c2 has an unexpected shape; the map is unchanged", path)
	for _, tc := range []struct {
		name    string
		fn      func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
		refused bool
		absent  []string // keys a successful reply must not contain
	}{
		{"get_topic_properties", h.GetTopicProperties, map[string]any{"topic_id": "c2"}, false, []string{`"labels"`, `"notes"`}},
		{"get_subtree", h.GetSubtree, map[string]any{}, false, nil},
		{"rename to the same title", h.RenameTopic, map[string]any{"topic_id": "c1", "title": "c1"}, false, nil},
		{"clear notes", h.SetTopicProperties, map[string]any{"topic_id": "c2", "notes": ""}, false, nil},
		{"rename", h.RenameTopic, map[string]any{"topic_id": "c1", "title": "n"}, true, nil},
		{"set labels", h.SetTopicProperties, map[string]any{"topic_id": "c2", "labels": []any{"a"}}, true, nil},
		{"delete", h.DeleteTopic, map[string]any{"topic_id": "c2"}, true, nil},
		{"duplicate", h.DuplicateTopic, map[string]any{"topic_id": "c2", "target_parent_id": "r"}, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := maps.Clone(tc.args)
			args["path"], args["sheet_id"] = path, "s1"
			res := callTool(t, tc.fn, args)
			text := textContent(t, res)
			switch {
			case tc.refused && (!res.IsError || text != refused):
				t.Errorf("got %q, want the tool error %q", text, refused)
			case !tc.refused && res.IsError:
				t.Errorf("got the tool error %q, want success", text)
			}
			for _, key := range tc.absent {
				if strings.Contains(text, key) {
					t.Errorf("reply %s contains %s", text, key)
				}
			}
			if after := readZipContentJSON(t, path); !bytes.Equal(after, before) {
				t.Fatalf("content.json changed:\n%s", after)
			}
		})
	}
}
