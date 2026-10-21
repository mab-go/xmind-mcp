// Package handler implements MCP tool handlers for xmind-mcp.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

// XMindHandler handles tool calls; it is stateless.
type XMindHandler struct{}

// NewXMindHandler returns a new handler instance.
func NewXMindHandler() *XMindHandler {
	return &XMindHandler{}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
	}
}

// jsonResult marshals v to JSON and returns it as the tool result text body, with v as the
// result's structuredContent. The tool must declare v's type with OutputSchema.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal response: %w", err)
	}
	res := textResult(string(data))
	res.StructuredContent = v
	return res, nil
}

// countTopics returns 1 (self) plus all descendants in attached, detached, and summary.
func countTopics(t *xmind.Topic) int {
	if t == nil {
		return 0
	}
	n := 1
	if t.Children == nil {
		return n
	}
	for i := range t.Children.Attached {
		n += countTopics(&t.Children.Attached[i])
	}
	for i := range t.Children.Detached {
		n += countTopics(&t.Children.Detached[i])
	}
	for i := range t.Children.Summary {
		n += countTopics(&t.Children.Summary[i])
	}
	return n
}

// deepCloneTopic returns a deep copy of the topic subtree with JSON round-trip (preserves codec `extra`
// data), then assigns fresh UUIDs to every topic and to summary/boundary descriptors within the clone.
func deepCloneTopic(root *xmind.Topic) (xmind.Topic, error) {
	if root == nil {
		return xmind.Topic{}, fmt.Errorf("deepCloneTopic: nil root")
	}
	data, err := json.Marshal(root)
	if err != nil {
		return xmind.Topic{}, fmt.Errorf("marshal topic: %w", err)
	}
	var clone xmind.Topic
	if err := json.Unmarshal(data, &clone); err != nil {
		return xmind.Topic{}, fmt.Errorf("unmarshal topic: %w", err)
	}
	remapClonedTopicIDs(&clone)
	return clone, nil
}

// remapClonedTopicIDs reassigns topic IDs and summary/boundary IDs after a JSON clone. Pass 1 fills
// idMap; pass 2 updates Summary and Boundary records that reference topic IDs or need unique IDs.
// A summary descriptor whose topicId names no topic in the clone is left out of the copy; older
// versions left such descriptors behind when a summary topic was deleted.
func remapClonedTopicIDs(root *xmind.Topic) {
	idMap := make(map[string]string)
	walkTopics(root, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		oldID := t.ID
		newID := uuid.New().String()
		idMap[oldID] = newID
		t.ID = newID
		return true
	})
	walkTopics(root, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		kept := t.Summaries[:0]
		for _, s := range t.Summaries {
			s.ID = uuid.New().String()
			if s.TopicID != "" {
				newRef, ok := idMap[s.TopicID]
				if !ok {
					continue
				}
				s.TopicID = newRef
			}
			kept = append(kept, s)
		}
		if len(kept) == 0 {
			kept = nil
		}
		t.Summaries = kept
		for i := range t.Boundaries {
			t.Boundaries[i].ID = uuid.New().String()
		}
		return true
	})
}

// findSheetByID returns the sheet with the given id, or nil if not found.
func findSheetByID(sheets []xmind.Sheet, id string) *xmind.Sheet {
	for i := range sheets {
		if sheets[i].ID == id {
			return &sheets[i]
		}
	}
	return nil
}

// findTopicByID returns the first topic with the given id in preorder DFS, or nil.
func findTopicByID(root *xmind.Topic, id string) *xmind.Topic {
	var found *xmind.Topic
	walkTopics(root, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		if t.ID == id {
			found = t
			return false
		}
		return true
	})
	return found
}

// ancestryPath returns titles from root down to (but not including) the target topic.
// Returns nil if targetID is the root itself or not found.
// NOTE: does a full DFS per call; fine for typical map sizes (<1000 topics).
// If performance becomes a concern, build a parent-ID map in one pass instead.
func ancestryPath(root *xmind.Topic, targetID string) []string {
	var path []string
	if buildAncestryPath(root, targetID, &path) {
		return path
	}
	return nil
}

func buildAncestryPath(current *xmind.Topic, targetID string, path *[]string) bool {
	if current == nil {
		return false
	}
	if current.ID == targetID {
		return true
	}
	*path = append(*path, current.Title)
	if current.Children != nil {
		for i := range current.Children.Attached {
			if buildAncestryPath(&current.Children.Attached[i], targetID, path) {
				return true
			}
		}
		for i := range current.Children.Detached {
			if buildAncestryPath(&current.Children.Detached[i], targetID, path) {
				return true
			}
		}
		for i := range current.Children.Summary {
			if buildAncestryPath(&current.Children.Summary[i], targetID, path) {
				return true
			}
		}
	}
	*path = (*path)[:len(*path)-1]
	return false
}

// walkTopics visits topic in preorder depth-first: attached, then detached, then summary.
// fn returns false to stop the walk entirely. walkTopics returns false if the walk was stopped early.
func walkTopics(topic *xmind.Topic, depth int, parent *xmind.Topic, fn func(t *xmind.Topic, depth int, parent *xmind.Topic) bool) bool {
	if topic == nil {
		return true
	}
	if !fn(topic, depth, parent) {
		return false
	}
	if topic.Children == nil {
		return true
	}
	for i := range topic.Children.Attached {
		if !walkTopics(&topic.Children.Attached[i], depth+1, topic, fn) {
			return false
		}
	}
	for i := range topic.Children.Detached {
		if !walkTopics(&topic.Children.Detached[i], depth+1, topic, fn) {
			return false
		}
	}
	for i := range topic.Children.Summary {
		if !walkTopics(&topic.Children.Summary[i], depth+1, topic, fn) {
			return false
		}
	}
	return true
}

// findDirectChildInLists reports whether targetID is an immediate child of parent and its index/list.
func findDirectChildInLists(parent *xmind.Topic, targetID string) (idx int, listType string, ok bool) {
	if parent == nil || parent.Children == nil {
		return -1, "", false
	}
	for i := range parent.Children.Attached {
		if parent.Children.Attached[i].ID == targetID {
			return i, "attached", true
		}
	}
	for i := range parent.Children.Detached {
		if parent.Children.Detached[i].ID == targetID {
			return i, "detached", true
		}
	}
	for i := range parent.Children.Summary {
		if parent.Children.Summary[i].ID == targetID {
			return i, "summary", true
		}
	}
	return -1, "", false
}

func searchChildrenForParent(root *xmind.Topic, targetID string) (*xmind.Topic, int, string) {
	if root == nil || root.Children == nil {
		return nil, -1, ""
	}
	for i := range root.Children.Attached {
		if p, idx, lt := findParentOfTopic(&root.Children.Attached[i], targetID); p != nil {
			return p, idx, lt
		}
	}
	for i := range root.Children.Detached {
		if p, idx, lt := findParentOfTopic(&root.Children.Detached[i], targetID); p != nil {
			return p, idx, lt
		}
	}
	for i := range root.Children.Summary {
		if p, idx, lt := findParentOfTopic(&root.Children.Summary[i], targetID); p != nil {
			return p, idx, lt
		}
	}
	return nil, -1, ""
}

// findParentOfTopic returns the parent of targetID, the child's index within the parent's
// attached/detached/summary slice, and which list ("attached", "detached", "summary").
// Returns nil, -1, "" if target is the root, not found, or root is nil.
func findParentOfTopic(root *xmind.Topic, targetID string) (*xmind.Topic, int, string) {
	if root == nil {
		return nil, -1, ""
	}
	if root.ID == targetID {
		return nil, -1, ""
	}
	if idx, lt, ok := findDirectChildInLists(root, targetID); ok {
		return root, idx, lt
	}
	return searchChildrenForParent(root, targetID)
}

// isDescendantOf reports whether descendantID matches the ancestor topic or any node
// in its subtree (preorder: attached, detached, summary).
func isDescendantOf(ancestor *xmind.Topic, descendantID string) bool {
	if ancestor == nil {
		return false
	}
	return !walkTopics(ancestor, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		return t.ID != descendantID
	})
}

// topicIDSet returns the IDs of root and every topic below it (attached, detached and summary).
func topicIDSet(root *xmind.Topic) map[string]struct{} {
	ids := make(map[string]struct{})
	walkTopics(root, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		ids[t.ID] = struct{}{}
		return true
	})
	return ids
}

// absPathFromArgs resolves and absolutes args["path"]; returns a tool-level error result if invalid.
func absPathFromArgs(args map[string]any) (abs string, toolErr *mcp.CallToolResult) {
	if args == nil {
		return "", mcp.NewToolResultError("missing required argument: path")
	}
	raw, ok := args["path"]
	if !ok {
		return "", mcp.NewToolResultError("missing required argument: path")
	}
	path, ok := raw.(string)
	if !ok {
		return "", mcp.NewToolResultError("invalid argument path: expected a string")
	}
	if path == "" {
		return "", mcp.NewToolResultError("missing required argument: path")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", mcp.NewToolResultError("invalid path: " + err.Error())
	}
	return absPath, nil
}

// optionalString reads args[key] when present. Returns present=false when the key
// is absent or null. It errors only on a non-string value; an empty string is
// returned as-is (present=true) for the caller to interpret (default vs. reject).
func optionalString(args map[string]any, key string) (val string, present bool, toolErr *mcp.CallToolResult) {
	v, has := args[key]
	if !has || v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, mcp.NewToolResultError("invalid argument " + key + ": expected a string")
	}
	return s, true, nil
}

// isPathAccessError reports whether err means the path itself can't be used: no permission, a
// file where a directory should be, a name too long, a symlink loop, an invalid path (such as
// one containing NUL), or a read-only filesystem. The caller can fix these by pointing at
// another path, so they are tool errors. osPathAccessErrors holds the errors whose numbers
// differ between Unix and Windows.
func isPathAccessError(err error) bool {
	for _, target := range []error{fs.ErrPermission, syscall.ENOTDIR, syscall.EINVAL} {
		if errors.Is(err, target) {
			return true
		}
	}
	for _, target := range osPathAccessErrors {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// pathErrorReason returns the cause of err without the operation and path an *fs.PathError
// adds, for messages that already name the path.
func pathErrorReason(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// pathAccessToolError maps a failure to stat absPath to a tool error: "file not found" for a
// missing file, "cannot access" for the failures isPathAccessError lists. It returns nil for
// anything else, which the caller returns as a protocol error.
func pathAccessToolError(absPath string, err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", absPath))
	case isPathAccessError(err):
		return mcp.NewToolResultError(fmt.Sprintf("cannot access %s: %v", absPath, pathErrorReason(err)))
	}
	return nil
}

// commitFailure maps a failed mw.Commit to the handler's return values. A map holding a value
// that could not be decoded in a field the tools edit, a map file that is read-only, a
// read-only filesystem, or a folder that can't be written is a tool error the caller can fix
// (the map was left unchanged); any other failure is a protocol error. Each tool error names
// its own cause: a permission error that is not about the file itself comes from creating the
// temp file in, or renaming it within, the map's folder.
func commitFailure(absPath string, err error) (*mcp.CallToolResult, error) {
	switch {
	case errors.Is(err, xmind.ErrUndecodableField):
		return mcp.NewToolResultError(fmt.Sprintf("cannot save map at %s: %v; the map is unchanged", absPath, err)), nil
	case errors.Is(err, xmind.ErrMapReadOnly):
		return mcp.NewToolResultError(fmt.Sprintf("cannot save map at %s: the file is read-only", absPath)), nil
	case errors.Is(err, errReadOnlyFS):
		return mcp.NewToolResultError(fmt.Sprintf("cannot save map at %s: %v", absPath, pathErrorReason(err))), nil
	case errors.Is(err, fs.ErrPermission):
		return mcp.NewToolResultError(fmt.Sprintf(
			"cannot save map at %s: %v (saving needs write access to the map's folder)", absPath, pathErrorReason(err))), nil
	}
	return nil, fmt.Errorf("write map: %w", err)
}

// statAndReadMap checks that absPath exists, reads the workbook, and maps failures per AGENTS.md
// Error Handling: a missing file, a path that can't be accessed, and a file that is not a valid
// map are tool errors; any other stat failure is a protocol error.
func statAndReadMap(absPath string) (sheets []xmind.Sheet, toolErr *mcp.CallToolResult, err error) {
	if _, statErr := os.Stat(absPath); statErr != nil {
		if accessErr := pathAccessToolError(absPath, statErr); accessErr != nil {
			return nil, accessErr, nil
		}
		return nil, nil, fmt.Errorf("stat file: %w", statErr)
	}
	sheets, readErr := xmind.ReadMap(absPath)
	if readErr != nil {
		return nil, mcp.NewToolResultError(readErr.Error()), nil
	}
	return sheets, nil, nil
}

// statAndOpenMapForUpdate is the mutating-handler counterpart to statAndReadMap: it
// checks that absPath exists, opens the workbook once, and returns a MapWriter that holds
// the zip open for a subsequent mw.Commit. Error mapping matches statAndReadMap. Callers
// MUST defer mw.Close() immediately after a nil-error return, before any branch that may
// return without committing (e.g. a sheet-not-found tool error or a no-change path).
func statAndOpenMapForUpdate(absPath string) (sheets []xmind.Sheet, mw *xmind.MapWriter, toolErr *mcp.CallToolResult, err error) {
	if _, statErr := os.Stat(absPath); statErr != nil {
		if accessErr := pathAccessToolError(absPath, statErr); accessErr != nil {
			return nil, nil, accessErr, nil
		}
		return nil, nil, nil, fmt.Errorf("stat file: %w", statErr)
	}
	sheets, mw, openErr := xmind.OpenMapForUpdate(absPath)
	if openErr != nil {
		return nil, nil, mcp.NewToolResultError(openErr.Error()), nil
	}
	return sheets, mw, nil, nil
}
