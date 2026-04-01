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

// requireSheetIDFromArgs reads and validates args["sheet_id"] like GetSubtree/FindTopic.
func requireSheetIDFromArgs(args map[string]any) (sheetID string, toolErr *mcp.CallToolResult) {
	rawSheet, ok := args["sheet_id"]
	if !ok {
		return "", mcp.NewToolResultError("missing required argument: sheet_id")
	}
	sid, ok := rawSheet.(string)
	if !ok || sid == "" {
		return "", mcp.NewToolResultError("invalid argument sheet_id: expected a non-empty string")
	}
	return sid, nil
}

// getSubtreeRootFromArgs resolves the subtree root from optional topic_id (empty string = sheet root).
func getSubtreeRootFromArgs(sh *xmind.Sheet, args map[string]any) (root *xmind.Topic, toolErr *mcp.CallToolResult) {
	if v, has := args["topic_id"]; has && v != nil {
		rawTopic, ok := v.(string)
		if !ok {
			return nil, mcp.NewToolResultError("invalid argument topic_id: expected a string")
		}
		if rawTopic != "" {
			t := findTopicByID(&sh.RootTopic, rawTopic)
			if t == nil {
				return nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", rawTopic))
			}
			return t, nil
		}
	}
	return &sh.RootTopic, nil
}

// readMapAndSheet loads the workbook and returns the sheet with sheetID.
func readMapAndSheet(absPath, sheetID string) (*xmind.Sheet, *mcp.CallToolResult, error) {
	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, nil, err
	}
	if toolErr2 != nil {
		return nil, toolErr2, nil
	}
	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}
	return sh, nil, nil
}

func getSubtreeDepthAndOpts(args map[string]any) (maxDepth *int, opts subtreeOpts, toolErr *mcp.CallToolResult) {
	if raw, has := args["depth"]; has {
		md, derr := parseDepthOptional(raw)
		if derr != nil {
			return nil, subtreeOpts{}, derr
		}
		maxDepth = md
	}
	includeNotes, berr := parseBoolOptional(args, "include_notes")
	if berr != nil {
		return nil, subtreeOpts{}, berr
	}
	includeLinks, berr2 := parseBoolOptional(args, "include_links")
	if berr2 != nil {
		return nil, subtreeOpts{}, berr2
	}
	return maxDepth, subtreeOpts{includeNotes: includeNotes, includeLinks: includeLinks}, nil
}

// GetSubtree returns a JSON subtree from the sheet root or a topic id.
func (h *XMindHandler) GetSubtree(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	sheetID, serr := requireSheetIDFromArgs(args)
	if serr != nil {
		return serr, nil
	}

	sh, teRead, err := readMapAndSheet(absPath, sheetID)
	if err != nil {
		return nil, err
	}
	if teRead != nil {
		return teRead, nil
	}

	root, rerr := getSubtreeRootFromArgs(sh, args)
	if rerr != nil {
		return rerr, nil
	}

	maxDepth, opts, derr := getSubtreeDepthAndOpts(args)
	if derr != nil {
		return derr, nil
	}

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
	SheetID      string   `json:"sheetId,omitempty"`
	SheetTitle   string   `json:"sheetTitle,omitempty"`
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	AncestryPath []string `json:"ancestryPath"`
	ParentTitle  string   `json:"parentTitle"`
	Depth        int      `json:"depth"`
}

// searchSheetsScopeFromArgs returns sheets to search and whether to attach sheet metadata per match.
func searchSheetsScopeFromArgs(sheets []xmind.Sheet, args map[string]any) (toSearch []xmind.Sheet, includeSheetMetadata bool, toolErr *mcp.CallToolResult) {
	if raw, has := args["sheet_id"]; has && raw != nil {
		sheetID, ok := raw.(string)
		if !ok || sheetID == "" {
			return nil, false, mcp.NewToolResultError("invalid argument sheet_id: expected a non-empty string")
		}
		sh := findSheetByID(sheets, sheetID)
		if sh == nil {
			return nil, false, mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID))
		}
		return []xmind.Sheet{*sh}, false, nil
	}
	return sheets, true, nil
}

func collectSearchTopicMatches(sh *xmind.Sheet, qLower string, includeSheetMetadata bool) []searchTopicItem {
	var matches []searchTopicItem
	walkTopics(&sh.RootTopic, 0, nil, func(t *xmind.Topic, depth int, parent *xmind.Topic) bool {
		if strings.Contains(strings.ToLower(t.Title), qLower) {
			pt := ""
			if parent != nil {
				pt = parent.Title
			}
			item := searchTopicItem{
				ID:           t.ID,
				Title:        t.Title,
				AncestryPath: ancestryPath(&sh.RootTopic, t.ID),
				ParentTitle:  pt,
				Depth:        depth,
			}
			if includeSheetMetadata {
				item.SheetID = sh.ID
				item.SheetTitle = sh.Title
			}
			matches = append(matches, item)
		}
		return true
	})
	return matches
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

	sheetsToSearch, allSheets, scopeErr := searchSheetsScopeFromArgs(sheets, args)
	if scopeErr != nil {
		return scopeErr, nil
	}

	qLower := strings.ToLower(query)
	var matches []searchTopicItem
	for i := range sheetsToSearch {
		sh := &sheetsToSearch[i]
		matches = append(matches, collectSearchTopicMatches(sh, qLower, allSheets)...)
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
	AncestryPath   []string `json:"ancestryPath"`
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

// findTopicSearchRoot returns the topic at which to start the FindTopic walk (sheet root or parent_id subtree).
func findTopicSearchRoot(sh *xmind.Sheet, args map[string]any) (searchRoot *xmind.Topic, toolErr *mcp.CallToolResult) {
	if v, has := args["parent_id"]; has && v != nil {
		pid, ok := v.(string)
		if !ok {
			return nil, mcp.NewToolResultError("invalid argument parent_id: expected a string")
		}
		if pid == "" {
			return nil, mcp.NewToolResultError("invalid argument parent_id: expected a non-empty string")
		}
		topic := findTopicByID(&sh.RootTopic, pid)
		if topic == nil {
			return nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", pid))
		}
		return topic, nil
	}
	return &sh.RootTopic, nil
}

func requireFindTopicTitleArgs(args map[string]any) (wantTitle string, toolErr *mcp.CallToolResult) {
	rawTitle, ok := args["title"]
	if !ok {
		return "", mcp.NewToolResultError("missing required argument: title")
	}
	title, ok := rawTitle.(string)
	if !ok {
		return "", mcp.NewToolResultError("invalid argument title: expected a string")
	}
	if title == "" {
		return "", mcp.NewToolResultError("invalid argument title: expected a non-empty string")
	}
	return title, nil
}

func findFirstTopicByExactTitle(searchRoot *xmind.Topic, wantTitle string) (found *xmind.Topic, foundParent *xmind.Topic) {
	walkTopics(searchRoot, 0, nil, func(t *xmind.Topic, _ int, parent *xmind.Topic) bool {
		if t.Title == wantTitle {
			found = t
			foundParent = parent
			return false
		}
		return true
	})
	return found, foundParent
}

func findTopicResponseFromMatch(sh *xmind.Sheet, found, foundParent *xmind.Topic) findTopicResponse {
	resp := findTopicResponse{
		ID:           found.ID,
		Title:        found.Title,
		AncestryPath: ancestryPath(&sh.RootTopic, found.ID),
	}
	if foundParent != nil {
		resp.ParentTitle = foundParent.Title
		resp.SiblingTitles = siblingTitlesInWalkOrder(foundParent, found.ID)
	}
	resp.ChildrenTitles = childTitlesInWalkOrder(found)
	return resp
}

func findTopicWorkflow(absPath, sheetID string, args map[string]any, wantTitle string) (findTopicResponse, *mcp.CallToolResult, error) {
	var zero findTopicResponse
	sh, te, err := readMapAndSheet(absPath, sheetID)
	if err != nil {
		return zero, nil, err
	}
	if te != nil {
		return zero, te, nil
	}
	searchRoot, re := findTopicSearchRoot(sh, args)
	if re != nil {
		return zero, re, nil
	}
	found, par := findFirstTopicByExactTitle(searchRoot, wantTitle)
	if found == nil {
		return zero, mcp.NewToolResultError(fmt.Sprintf("topic not found: no topic with title %q", wantTitle)), nil
	}
	return findTopicResponseFromMatch(sh, found, par), nil, nil
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

	sheetID, serr := requireSheetIDFromArgs(args)
	if serr != nil {
		return serr, nil
	}

	wantTitle, terr := requireFindTopicTitleArgs(args)
	if terr != nil {
		return terr, nil
	}

	resp, te, err := findTopicWorkflow(absPath, sheetID, args, wantTitle)
	if err != nil {
		return nil, err
	}
	if te != nil {
		return te, nil
	}

	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal find_topic response: %w", err)
	}
	return textResult(string(out)), nil
}

// topicPropertiesResponse is the JSON shape for xmind_get_topic_properties.
type topicPropertiesResponse struct {
	ID             string                        `json:"id"`
	Title          string                        `json:"title,omitempty"`
	StructureClass string                        `json:"structureClass,omitempty"`
	Labels         []string                      `json:"labels,omitempty"`
	Markers        []xmind.Marker                `json:"markers,omitempty"`
	Notes          string                        `json:"notes,omitempty"`
	Href           string                        `json:"href,omitempty"`
	ImageSrc       string                        `json:"imageSrc,omitempty"`
	Position       *topicPropertiesPosition      `json:"position,omitempty"`
	BoundaryCount  int                           `json:"boundaryCount,omitempty"`
	Boundaries     []topicPropertiesBoundary     `json:"boundaries,omitempty"`
	SummaryCount   int                           `json:"summaryCount,omitempty"`
	Relationships  []topicPropertiesRelationship `json:"relationships,omitempty"`
	ChildCounts    *topicPropertiesChildCounts   `json:"childCounts,omitempty"`
}

type topicPropertiesPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type topicPropertiesBoundary struct {
	ID    string `json:"id"`
	Range string `json:"range"`
	Title string `json:"title,omitempty"`
}

type topicPropertiesRelationship struct {
	ID     string `json:"id"`
	End1ID string `json:"end1Id"`
	End2ID string `json:"end2Id"`
	Title  string `json:"title,omitempty"`
}

type topicPropertiesChildCounts struct {
	Attached int `json:"attached,omitempty"`
	Detached int `json:"detached,omitempty"`
	Summary  int `json:"summary,omitempty"`
}

func topicPropertiesBase(topic *xmind.Topic) topicPropertiesResponse {
	out := topicPropertiesResponse{
		ID:             topic.ID,
		Title:          topic.Title,
		StructureClass: topic.StructureClass,
	}
	if len(topic.Labels) > 0 {
		out.Labels = append([]string(nil), topic.Labels...)
	}
	if len(topic.Markers) > 0 {
		out.Markers = append([]xmind.Marker(nil), topic.Markers...)
	}
	return out
}

func applyTopicPropertiesMedia(out *topicPropertiesResponse, topic *xmind.Topic) {
	if plain := plainNoteContent(topic); plain != "" {
		out.Notes = plain
	}
	if topic.Href != "" {
		out.Href = topic.Href
	}
	if topic.Image != nil && topic.Image.Src != "" {
		out.ImageSrc = topic.Image.Src
	}
	if topic.Position != nil {
		out.Position = &topicPropertiesPosition{X: topic.Position.X, Y: topic.Position.Y}
	}
}

func applyTopicPropertiesBoundaries(out *topicPropertiesResponse, topic *xmind.Topic) {
	n := len(topic.Boundaries)
	if n == 0 {
		return
	}
	out.BoundaryCount = n
	out.Boundaries = make([]topicPropertiesBoundary, 0, n)
	for i := range topic.Boundaries {
		b := &topic.Boundaries[i]
		out.Boundaries = append(out.Boundaries, topicPropertiesBoundary{
			ID:    b.ID,
			Range: b.Range,
			Title: b.Title,
		})
	}
}

func applyTopicPropertiesSummaryCount(out *topicPropertiesResponse, topic *xmind.Topic) {
	if sc := len(topic.Summaries); sc > 0 {
		out.SummaryCount = sc
	}
}

func appendTopicRelationshipsForTopic(out *topicPropertiesResponse, sh *xmind.Sheet, topicID string) {
	if sh == nil || len(sh.Relationships) == 0 {
		return
	}
	for i := range sh.Relationships {
		rel := &sh.Relationships[i]
		if rel.End1ID != topicID && rel.End2ID != topicID {
			continue
		}
		item := topicPropertiesRelationship{
			ID:     rel.ID,
			End1ID: rel.End1ID,
			End2ID: rel.End2ID,
		}
		if rel.Title != "" {
			item.Title = rel.Title
		}
		out.Relationships = append(out.Relationships, item)
	}
}

func topicChildCountsForResponse(topic *xmind.Topic) *topicPropertiesChildCounts {
	if topic.Children == nil {
		return nil
	}
	a := len(topic.Children.Attached)
	d := len(topic.Children.Detached)
	s := len(topic.Children.Summary)
	if a == 0 && d == 0 && s == 0 {
		return nil
	}
	cc := &topicPropertiesChildCounts{}
	if a > 0 {
		cc.Attached = a
	}
	if d > 0 {
		cc.Detached = d
	}
	if s > 0 {
		cc.Summary = s
	}
	return cc
}

func topicPropertiesResponseFrom(sh *xmind.Sheet, topic *xmind.Topic) topicPropertiesResponse {
	if topic == nil {
		return topicPropertiesResponse{}
	}
	out := topicPropertiesBase(topic)
	applyTopicPropertiesMedia(&out, topic)
	applyTopicPropertiesBoundaries(&out, topic)
	applyTopicPropertiesSummaryCount(&out, topic)
	appendTopicRelationshipsForTopic(&out, sh, topic.ID)
	out.ChildCounts = topicChildCountsForResponse(topic)
	return out
}

// GetTopicProperties returns JSON with a single topic's metadata for verification after writes.
func (h *XMindHandler) GetTopicProperties(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	sheetID, terr := requireString(args, "sheet_id")
	if terr != nil {
		return terr, nil
	}
	topicID, terr := requireString(args, "topic_id")
	if terr != nil {
		return terr, nil
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

	t := findTopicByID(&sh.RootTopic, topicID)
	if t == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID)), nil
	}

	resp := topicPropertiesResponseFrom(sh, t)
	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal get_topic_properties response: %w", err)
	}
	return textResult(string(out)), nil
}
