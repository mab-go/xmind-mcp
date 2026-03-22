// Package handler implements MCP tool handlers for xmind-mcp.
package handler

import (
	"fmt"
	"os"
	"path/filepath"

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
	if root.Children != nil {
		for i := range root.Children.Attached {
			if root.Children.Attached[i].ID == targetID {
				return root, i, "attached"
			}
		}
		for i := range root.Children.Detached {
			if root.Children.Detached[i].ID == targetID {
				return root, i, "detached"
			}
		}
		for i := range root.Children.Summary {
			if root.Children.Summary[i].ID == targetID {
				return root, i, "summary"
			}
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
	}
	return nil, -1, ""
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

// statAndReadMap checks that absPath exists, reads the workbook, and maps I/O and parse failures
// to tool errors vs protocol errors per 00-OVERVIEW.
func statAndReadMap(absPath string) (sheets []xmind.Sheet, toolErr *mcp.CallToolResult, err error) {
	if _, statErr := os.Stat(absPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, mcp.NewToolResultError(fmt.Sprintf("file not found: %s", absPath)), nil
		}
		return nil, nil, fmt.Errorf("stat file: %w", statErr)
	}
	sheets, readErr := xmind.ReadMap(absPath)
	if readErr != nil {
		return nil, mcp.NewToolResultError(readErr.Error()), nil
	}
	return sheets, nil, nil
}
