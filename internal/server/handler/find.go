package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

// subtreeNode is the JSON shape for xmind_get_subtree (not a raw xmind.Topic).
type subtreeNode struct {
	ID             string         `json:"id"`
	Title          string         `json:"title,omitempty"`
	StructureClass string         `json:"structureClass,omitempty"`
	Labels         []string       `json:"labels,omitempty"`
	Markers        []xmind.Marker `json:"markers,omitempty"`
	Notes          string         `json:"notes,omitempty"`
	Href           string         `json:"href,omitempty"`
	Children       []*subtreeNode `json:"children,omitempty"`
	ChildrenCount  int            `json:"childrenCount,omitempty"`
}

// subtreeOpts controls optional fields when building a subtree.
type subtreeOpts struct {
	includeNotes bool
	includeLinks bool
}

func directChildCount(t *xmind.Topic) int {
	if t == nil || t.Children == nil {
		return 0
	}
	return len(t.Children.Attached) + len(t.Children.Detached) + len(t.Children.Summary)
}

func plainNoteContent(t *xmind.Topic) string {
	if t == nil || t.Notes == nil || t.Notes.Plain == nil {
		return ""
	}
	return t.Notes.Plain.Content
}

func topicToSubtree(t *xmind.Topic, curDepth int, maxDepth *int, opts subtreeOpts) *subtreeNode {
	if t == nil {
		return nil
	}
	n := &subtreeNode{
		ID:             t.ID,
		Title:          t.Title,
		StructureClass: t.StructureClass,
		Labels:         append([]string(nil), t.Labels...),
		Markers:        append([]xmind.Marker(nil), t.Markers...),
	}
	if opts.includeNotes {
		if plain := plainNoteContent(t); plain != "" {
			n.Notes = plain
		}
	}
	if opts.includeLinks && t.Href != "" {
		n.Href = t.Href
	}
	if maxDepth == nil {
		n.Children = childSubtrees(t, curDepth, nil, opts)
		return n
	}
	if curDepth >= *maxDepth {
		n.ChildrenCount = directChildCount(t)
		return n
	}
	n.Children = childSubtrees(t, curDepth, maxDepth, opts)
	return n
}

func childSubtrees(t *xmind.Topic, curDepth int, maxDepth *int, opts subtreeOpts) []*subtreeNode {
	if t == nil || t.Children == nil {
		return nil
	}
	var out []*subtreeNode
	for i := range t.Children.Attached {
		out = append(out, topicToSubtree(&t.Children.Attached[i], curDepth+1, maxDepth, opts))
	}
	for i := range t.Children.Detached {
		out = append(out, topicToSubtree(&t.Children.Detached[i], curDepth+1, maxDepth, opts))
	}
	for i := range t.Children.Summary {
		out = append(out, topicToSubtree(&t.Children.Summary[i], curDepth+1, maxDepth, opts))
	}
	return out
}

// parseDepthOptional converts an optional MCP depth argument to *int. JSON numbers arrive as float64;
// non-whole values are rejected.
func parseDepthOptional(raw any) (*int, *mcp.CallToolResult) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, mcp.NewToolResultError("invalid argument depth: must be a finite number")
		}
		if v < 0 {
			return nil, mcp.NewToolResultError("invalid argument depth: must be non-negative")
		}
		trunc := math.Trunc(v)
		if trunc != v {
			return nil, mcp.NewToolResultError("invalid argument depth: must be a whole number")
		}
		d := int(trunc)
		return &d, nil
	case int:
		if v < 0 {
			return nil, mcp.NewToolResultError("invalid argument depth: must be non-negative")
		}
		return &v, nil
	default:
		return nil, mcp.NewToolResultError("invalid argument depth: expected a number")
	}
}

// parseBoolOptional reads an optional boolean MCP argument; missing or null is false.
func parseBoolOptional(args map[string]any, key string) (bool, *mcp.CallToolResult) {
	if raw, has := args[key]; has && raw != nil {
		v, ok := raw.(bool)
		if !ok {
			return false, mcp.NewToolResultError("invalid argument " + key + ": expected a boolean")
		}
		return v, nil
	}
	return false, nil
}

// GetSubtree returns a JSON subtree from the sheet root or a topic id.
func (h *XMindHandler) GetSubtree(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	rawSheet, ok := args["sheet_id"]
	if !ok {
		return mcp.NewToolResultError("missing required argument: sheet_id"), nil
	}
	sheetID, ok := rawSheet.(string)
	if !ok || sheetID == "" {
		return mcp.NewToolResultError("invalid argument sheet_id: expected a non-empty string"), nil
	}

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}

	var root *xmind.Topic
	if v, has := args["topic_id"]; has && v != nil {
		rawTopic, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument topic_id: expected a string"), nil
		}
		if rawTopic != "" {
			t := findTopicByID(&sh.RootTopic, rawTopic)
			if t == nil {
				return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", rawTopic)), nil
			}
			root = t
		}
	}
	if root == nil {
		root = &sh.RootTopic
	}

	var maxDepth *int
	if raw, has := args["depth"]; has {
		md, derr := parseDepthOptional(raw)
		if derr != nil {
			return derr, nil
		}
		maxDepth = md
	}

	includeNotes, berr := parseBoolOptional(args, "include_notes")
	if berr != nil {
		return berr, nil
	}
	includeLinks, berr2 := parseBoolOptional(args, "include_links")
	if berr2 != nil {
		return berr2, nil
	}
	opts := subtreeOpts{includeNotes: includeNotes, includeLinks: includeLinks}

	node := topicToSubtree(root, 0, maxDepth, opts)
	out, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("marshal get_subtree response: %w", err)
	}
	return textResult(string(out)), nil
}

type searchTopicsResponse struct {
	Query      string            `json:"query"`
	MatchCount int               `json:"matchCount"`
	Matches    []searchTopicItem `json:"matches"`
}

type searchTopicItem struct {
	SheetID     string `json:"sheetId,omitempty"`
	SheetTitle  string `json:"sheetTitle,omitempty"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	ParentTitle string `json:"parentTitle"`
	Depth       int    `json:"depth"`
}

// SearchTopics finds topics whose title contains the query (case-insensitive).
// If sheet_id is omitted, all sheets are searched and each result includes sheetId/sheetTitle.
func (h *XMindHandler) SearchTopics(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	rawQuery, ok := args["query"]
	if !ok {
		return mcp.NewToolResultError("missing required argument: query"), nil
	}
	query, ok := rawQuery.(string)
	if !ok {
		return mcp.NewToolResultError("invalid argument query: expected a string"), nil
	}
	if query == "" {
		return mcp.NewToolResultError("invalid argument query: expected a non-empty string"), nil
	}

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	// Determine which sheets to search.
	allSheets := false
	var sheetsToSearch []xmind.Sheet
	if raw, has := args["sheet_id"]; has && raw != nil {
		sheetID, ok := raw.(string)
		if !ok || sheetID == "" {
			return mcp.NewToolResultError("invalid argument sheet_id: expected a non-empty string"), nil
		}
		sh := findSheetByID(sheets, sheetID)
		if sh == nil {
			return mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
		}
		sheetsToSearch = []xmind.Sheet{*sh}
	} else {
		sheetsToSearch = sheets
		allSheets = true
	}

	qLower := strings.ToLower(query)
	var matches []searchTopicItem
	for i := range sheetsToSearch {
		sh := &sheetsToSearch[i]
		walkTopics(&sh.RootTopic, 0, nil, func(t *xmind.Topic, depth int, parent *xmind.Topic) bool {
			if strings.Contains(strings.ToLower(t.Title), qLower) {
				pt := ""
				if parent != nil {
					pt = parent.Title
				}
				item := searchTopicItem{
					ID:          t.ID,
					Title:       t.Title,
					ParentTitle: pt,
					Depth:       depth,
				}
				if allSheets {
					item.SheetID = sh.ID
					item.SheetTitle = sh.Title
				}
				matches = append(matches, item)
			}
			return true
		})
	}

	resp := searchTopicsResponse{
		Query:      query,
		MatchCount: len(matches),
		Matches:    matches,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal search_topics response: %w", err)
	}
	return textResult(string(out)), nil
}

type findTopicResponse struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	ParentTitle    string   `json:"parentTitle"`
	SiblingTitles  []string `json:"siblingTitles"`
	ChildrenTitles []string `json:"childrenTitles"`
}

// siblingTitlesInWalkOrder returns titles of the parent's children in the same order as walkTopics
// (attached, detached, summary), excluding the topic with excludeID.
func siblingTitlesInWalkOrder(parent *xmind.Topic, excludeID string) []string {
	if parent == nil || parent.Children == nil {
		return nil
	}
	var out []string
	for i := range parent.Children.Attached {
		t := &parent.Children.Attached[i]
		if t.ID != excludeID {
			out = append(out, t.Title)
		}
	}
	for i := range parent.Children.Detached {
		t := &parent.Children.Detached[i]
		if t.ID != excludeID {
			out = append(out, t.Title)
		}
	}
	for i := range parent.Children.Summary {
		t := &parent.Children.Summary[i]
		if t.ID != excludeID {
			out = append(out, t.Title)
		}
	}
	return out
}

// childTitlesInWalkOrder returns titles of all direct children in walkTopics order.
func childTitlesInWalkOrder(t *xmind.Topic) []string {
	if t == nil || t.Children == nil {
		return nil
	}
	var out []string
	for i := range t.Children.Attached {
		out = append(out, t.Children.Attached[i].Title)
	}
	for i := range t.Children.Detached {
		out = append(out, t.Children.Detached[i].Title)
	}
	for i := range t.Children.Summary {
		out = append(out, t.Children.Summary[i].Title)
	}
	return out
}

// FindTopic returns the first topic with an exact case-sensitive title (preorder DFS).
// If parent_id is set, the walk starts at that topic (which may itself match).
func (h *XMindHandler) FindTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	rawSheet, ok := args["sheet_id"]
	if !ok {
		return mcp.NewToolResultError("missing required argument: sheet_id"), nil
	}
	sheetID, ok := rawSheet.(string)
	if !ok || sheetID == "" {
		return mcp.NewToolResultError("invalid argument sheet_id: expected a non-empty string"), nil
	}

	rawTitle, ok := args["title"]
	if !ok {
		return mcp.NewToolResultError("missing required argument: title"), nil
	}
	wantTitle, ok := rawTitle.(string)
	if !ok {
		return mcp.NewToolResultError("invalid argument title: expected a string"), nil
	}
	if wantTitle == "" {
		return mcp.NewToolResultError("invalid argument title: expected a non-empty string"), nil
	}

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}

	searchRoot := &sh.RootTopic
	if v, has := args["parent_id"]; has && v != nil {
		pid, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument parent_id: expected a string"), nil
		}
		if pid == "" {
			return mcp.NewToolResultError("invalid argument parent_id: expected a non-empty string"), nil
		}
		topic := findTopicByID(&sh.RootTopic, pid)
		if topic == nil {
			return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", pid)), nil
		}
		searchRoot = topic
	}

	var found *xmind.Topic
	var foundParent *xmind.Topic
	walkTopics(searchRoot, 0, nil, func(t *xmind.Topic, _ int, parent *xmind.Topic) bool {
		if t.Title == wantTitle {
			found = t
			foundParent = parent
			return false
		}
		return true
	})

	if found == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: no topic with title %q", wantTitle)), nil
	}

	resp := findTopicResponse{
		ID:    found.ID,
		Title: found.Title,
	}
	if foundParent != nil {
		resp.ParentTitle = foundParent.Title
		resp.SiblingTitles = siblingTitlesInWalkOrder(foundParent, found.ID)
	}
	resp.ChildrenTitles = childTitlesInWalkOrder(found)

	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal find_topic response: %w", err)
	}
	return textResult(string(out)), nil
}
