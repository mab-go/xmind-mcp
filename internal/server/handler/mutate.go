package handler

import (
	"context"
	"fmt"
	"html"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

const maxBulkTopicsDepth = 64
const maxBulkTopicsTotal = 10000

type addTopicResponse struct {
	ID           string `json:"id"`
	Position     int    `json:"position"`
	SiblingCount int    `json:"siblingCount"`
}

type addTopicsBulkResponse struct {
	AddedCount    int      `json:"addedCount"`
	ParentID      string   `json:"parentId"`
	FirstPosition int      `json:"firstPosition"`
	SiblingCount  int      `json:"siblingCount"`
	RootTopicIDs  []string `json:"rootTopicIds"`
}

type duplicateTopicResponse struct {
	SourceID     string `json:"sourceId"`
	NewRootID    string `json:"newRootId"`
	ParentID     string `json:"parentId"`
	CopiedCount  int    `json:"copiedCount"`
	Position     int    `json:"position"`
	SiblingCount int    `json:"siblingCount"`
}

type moveTopicResponse struct {
	TopicID      string `json:"topicId"`
	ParentID     string `json:"parentId"`
	Position     int    `json:"position"`
	SiblingCount int    `json:"siblingCount"`
}

// plainToRealHTML converts a plain-text note string to minimal XMind-compatible HTML.
// XMind's Notes struct requires both plain and realHTML to be populated; writing plain
// text into realHTML directly causes the rich-text panel to display it unstyled and
// without paragraph structure. This helper produces the same output XMind itself writes
// for a simple plain-text note: HTML-escaped content wrapped in <div> blocks, with
// newlines converted to </div><div> so each line is its own block element.
func plainToRealHTML(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString("<div>")
		} else {
			b.WriteString("</div><div>")
		}
		b.WriteString(html.EscapeString(line))
	}
	b.WriteString("</div>")
	return b.String()
}

// buildTopicsFromArgs parses MCP topics array into a forest of Topic values with fresh UUIDs.
// Each element is a map with required "title", optional "children" ([]any), and optional
// metadata keys (notes, labels, markers, link, remove_markers) handled by applyTopicPropertiesArgs.
func buildTopicsFromArgs(raw []any) ([]xmind.Topic, int, *mcp.CallToolResult, error) {
	var total int
	out, toolRes, err := buildTopicsFromArgsDepth(raw, 0, &total)
	if toolRes != nil {
		return nil, 0, toolRes, nil
	}
	if err != nil {
		return nil, 0, nil, err
	}
	return out, total, nil, nil
}

func topicFromBulkArgMap(m map[string]any, index int, depth int, total *int) (xmind.Topic, *mcp.CallToolResult, error) {
	titleVal, ok := m["title"]
	if !ok {
		return xmind.Topic{}, nil, fmt.Errorf("topics[%d]: missing title", index)
	}
	title, ok := titleVal.(string)
	if !ok {
		return xmind.Topic{}, nil, fmt.Errorf("topics[%d]: title must be a string", index)
	}
	if *total >= maxBulkTopicsTotal {
		return xmind.Topic{}, nil, fmt.Errorf("maximum topic count is %d", maxBulkTopicsTotal)
	}
	topic := xmind.Topic{
		ID:    uuid.New().String(),
		Title: title,
	}
	*total++
	if ch, has := m["children"]; has && ch != nil {
		arr, ok := ch.([]any)
		if !ok {
			return xmind.Topic{}, nil, fmt.Errorf("topics[%d]: children must be an array", index)
		}
		children, toolErr, err := buildTopicsFromArgsDepth(arr, depth+1, total)
		if toolErr != nil {
			return xmind.Topic{}, toolErr, nil
		}
		if err != nil {
			return xmind.Topic{}, nil, err
		}
		topic.Children = &xmind.Children{Attached: children}
	}
	if toolResult := applyTopicPropertiesArgs(m, &topic); toolResult != nil {
		return xmind.Topic{}, toolResult, nil
	}
	return topic, nil, nil
}

func buildTopicsFromArgsDepth(raw []any, depth int, total *int) ([]xmind.Topic, *mcp.CallToolResult, error) {
	if depth > maxBulkTopicsDepth {
		return nil, nil, fmt.Errorf("maximum nesting depth is %d", maxBulkTopicsDepth)
	}
	out := make([]xmind.Topic, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("topics[%d]: expected object", i)
		}
		topic, toolRes, err := topicFromBulkArgMap(m, i, depth, total)
		if toolRes != nil {
			return nil, toolRes, nil
		}
		if err != nil {
			return nil, nil, err
		}
		out = append(out, topic)
	}
	return out, nil, nil
}

func parseSummaryRange(r string) (from, to int, ok bool) {
	r = strings.TrimSpace(r)
	if len(r) < 5 || r[0] != '(' || r[len(r)-1] != ')' {
		return 0, 0, false
	}
	inner := strings.TrimSpace(r[1 : len(r)-1])
	comma := strings.IndexByte(inner, ',')
	if comma < 0 {
		return 0, 0, false
	}
	f, err1 := strconv.Atoi(strings.TrimSpace(inner[:comma]))
	t, err2 := strconv.Atoi(strings.TrimSpace(inner[comma+1:]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if f < 0 || t < 0 || f > t {
		return 0, 0, false
	}
	return f, t, true
}

func formatSummaryRange(from, to int) string {
	return fmt.Sprintf("(%d,%d)", from, to)
}

func validateAddSummary(parent *xmind.Topic, fromIdx, toIdx int) *mcp.CallToolResult {
	if parent.Children == nil || len(parent.Children.Attached) == 0 {
		return mcp.NewToolResultError("parent has no attached children to summarize")
	}
	n := len(parent.Children.Attached)
	if fromIdx > toIdx {
		return mcp.NewToolResultError("invalid summary range: from_index must be <= to_index")
	}
	if toIdx >= n {
		return mcp.NewToolResultError(fmt.Sprintf("to_index %d is out of range for %d attached children", toIdx, n))
	}
	return nil
}

// applyAddSummary appends the summary topic and range descriptor (double-write per XMind schema).
func applyAddSummary(parent *xmind.Topic, fromIdx, toIdx int, title string) string {
	summaryTopicID := uuid.New().String()
	summaryRowID := uuid.New().String()
	summaryTopic := xmind.Topic{
		ID:    summaryTopicID,
		Title: title,
	}
	ch := ensureChildren(parent)
	ch.Summary = append(ch.Summary, summaryTopic)
	parent.Summaries = append(parent.Summaries, xmind.Summary{
		ID:      summaryRowID,
		Range:   formatSummaryRange(fromIdx, toIdx),
		TopicID: summaryTopicID,
	})
	return summaryTopicID
}

// shiftSummaryRangesAfterRemove returns updated summary descriptors and topic IDs whose summary
// children should be removed after deleting the attached child at removedIndex.
func shiftSummaryRangesAfterRemove(summaries []xmind.Summary, removedIndex int) ([]xmind.Summary, map[string]struct{}) {
	N := removedIndex
	removeTopicIDs := make(map[string]struct{})
	var newSummaries []xmind.Summary
	for _, s := range summaries {
		from, to, ok := parseSummaryRange(s.Range)
		if !ok {
			newSummaries = append(newSummaries, s)
			continue
		}
		newFrom, newTo := from, to
		if N >= newFrom && N <= newTo {
			newTo--
		} else if newFrom > N {
			newFrom--
			newTo--
		}
		if newFrom > newTo {
			removeTopicIDs[s.TopicID] = struct{}{}
			continue
		}
		s.Range = formatSummaryRange(newFrom, newTo)
		newSummaries = append(newSummaries, s)
	}
	return newSummaries, removeTopicIDs
}

func filterSummaryChildren(summaryChildren []xmind.Topic, removeIDs map[string]struct{}) []xmind.Topic {
	if len(removeIDs) == 0 {
		return summaryChildren
	}
	var kept []xmind.Topic
	for i := range summaryChildren {
		if _, drop := removeIDs[summaryChildren[i].ID]; !drop {
			kept = append(kept, summaryChildren[i])
		}
	}
	return kept
}

// adjustSummariesAfterAttachedRemove updates parent.Summaries and Children.Summary after
// removing the attached child at removedIndex.
func adjustSummariesAfterAttachedRemove(parent *xmind.Topic, removedIndex int) {
	if parent == nil || parent.Children == nil {
		return
	}
	newSummaries, removeTopicIDs := shiftSummaryRangesAfterRemove(parent.Summaries, removedIndex)
	parent.Summaries = newSummaries
	if len(parent.Summaries) == 0 {
		parent.Summaries = nil
	}
	if len(removeTopicIDs) == 0 {
		return
	}
	parent.Children.Summary = filterSummaryChildren(parent.Children.Summary, removeTopicIDs)
	if len(parent.Children.Summary) == 0 {
		parent.Children.Summary = nil
	}
}

// adjustSummariesAfterAttachedInsert shifts summary ranges after inserting one attached child at insertIndex.
func adjustSummariesAfterAttachedInsert(parent *xmind.Topic, insertIndex int) {
	if parent == nil || len(parent.Summaries) == 0 {
		return
	}
	P := insertIndex
	newSummaries := make([]xmind.Summary, 0, len(parent.Summaries))
	for _, s := range parent.Summaries {
		from, to, ok := parseSummaryRange(s.Range)
		if !ok {
			newSummaries = append(newSummaries, s)
			continue
		}
		newFrom, newTo := from, to
		switch {
		case P > newTo:
			// insert after this range
		case P <= newFrom:
			newFrom++
			newTo++
		default:
			// from < P <= to
			newTo++
		}
		s.Range = formatSummaryRange(newFrom, newTo)
		newSummaries = append(newSummaries, s)
	}
	parent.Summaries = newSummaries
	if len(parent.Summaries) == 0 {
		parent.Summaries = nil
	}
}

func childrenFullyEmpty(c *xmind.Children) bool {
	if c == nil {
		return true
	}
	return len(c.Attached) == 0 && len(c.Detached) == 0 && len(c.Summary) == 0
}

func nilEmptyChildren(parent *xmind.Topic) {
	if parent == nil {
		return
	}
	if childrenFullyEmpty(parent.Children) {
		parent.Children = nil
	}
}

func ensureChildren(t *xmind.Topic) *xmind.Children {
	if t.Children == nil {
		t.Children = &xmind.Children{}
	}
	return t.Children
}

// parsePositionOptional parses optional JSON number as non-negative int index for sibling insert.
func parsePositionOptional(raw any) (*int, *mcp.CallToolResult) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, mcp.NewToolResultError("invalid argument position: must be a finite number")
		}
		if v < 0 {
			return nil, mcp.NewToolResultError("invalid argument position: must be non-negative")
		}
		trunc := math.Trunc(v)
		if trunc != v {
			return nil, mcp.NewToolResultError("invalid argument position: must be a whole number")
		}
		p := int(trunc)
		return &p, nil
	case int:
		if v < 0 {
			return nil, mcp.NewToolResultError("invalid argument position: must be non-negative")
		}
		return &v, nil
	default:
		return nil, mcp.NewToolResultError("invalid argument position: expected a number")
	}
}

func insertAttached(parent *xmind.Topic, topic xmind.Topic, pos *int) *mcp.CallToolResult {
	ch := ensureChildren(parent)
	if pos == nil {
		ch.Attached = append(ch.Attached, topic)
		return nil
	}
	p := *pos
	if p < 0 || p > len(ch.Attached) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument position: %d is out of range for %d children", p, len(ch.Attached)))
	}
	ch.Attached = slices.Insert(ch.Attached, p, topic)
	return nil
}

// insertAttachedWithSummaryAdjust runs insertAttached, derives the insert index, and adjusts
// existing summary ranges when the parent has summaries.
func insertAttachedWithSummaryAdjust(parent *xmind.Topic, topic xmind.Topic, pos *int) (insertIdx int, toolErr *mcp.CallToolResult) {
	if terr := insertAttached(parent, topic, pos); terr != nil {
		return 0, terr
	}
	if pos == nil {
		insertIdx = len(parent.Children.Attached) - 1
	} else {
		insertIdx = *pos
	}
	if len(parent.Summaries) > 0 {
		adjustSummariesAfterAttachedInsert(parent, insertIdx)
	}
	return insertIdx, nil
}

func removeAttachedChild(parent *xmind.Topic, idx int) (*xmind.Topic, *mcp.CallToolResult) {
	if idx < 0 || idx >= len(parent.Children.Attached) {
		return nil, mcp.NewToolResultError("internal error: attached child index out of range")
	}
	removed := parent.Children.Attached[idx]
	parent.Children.Attached = slices.Delete(parent.Children.Attached, idx, idx+1)
	if len(parent.Children.Attached) == 0 {
		parent.Children.Attached = nil
	}
	adjustSummariesAfterAttachedRemove(parent, idx)
	nilEmptyChildren(parent)
	return &removed, nil
}

func removeDetachedChild(parent *xmind.Topic, idx int) (*xmind.Topic, *mcp.CallToolResult) {
	if idx < 0 || idx >= len(parent.Children.Detached) {
		return nil, mcp.NewToolResultError("internal error: detached child index out of range")
	}
	removed := parent.Children.Detached[idx]
	parent.Children.Detached = slices.Delete(parent.Children.Detached, idx, idx+1)
	if len(parent.Children.Detached) == 0 {
		parent.Children.Detached = nil
	}
	nilEmptyChildren(parent)
	return &removed, nil
}

func removeSummaryChild(parent *xmind.Topic, idx int) (*xmind.Topic, *mcp.CallToolResult) {
	if idx < 0 || idx >= len(parent.Children.Summary) {
		return nil, mcp.NewToolResultError("internal error: summary child index out of range")
	}
	removed := parent.Children.Summary[idx]
	parent.Children.Summary = slices.Delete(parent.Children.Summary, idx, idx+1)
	if len(parent.Children.Summary) == 0 {
		parent.Children.Summary = nil
	}
	nilEmptyChildren(parent)
	return &removed, nil
}

func removeChildAt(parent *xmind.Topic, idx int, listType string) (*xmind.Topic, *mcp.CallToolResult) {
	if parent == nil || parent.Children == nil {
		return nil, mcp.NewToolResultError("internal error: parent has no children")
	}
	switch listType {
	case "attached":
		return removeAttachedChild(parent, idx)
	case "detached":
		return removeDetachedChild(parent, idx)
	case "summary":
		return removeSummaryChild(parent, idx)
	default:
		return nil, mcp.NewToolResultError("internal error: unknown child list type")
	}
}

// AddTopic adds one child under parent_id.
func (h *XMindHandler) AddTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	parts, terr := requireMapStrings(args, []string{"sheet_id", "parent_id", "title"})
	if terr != nil {
		return terr, nil
	}
	sheetID, parentID, title := parts[0], parts[1], parts[2]
	pos, perr := parsePositionOptional(args["position"])
	if perr != nil {
		return perr, nil
	}

	sheets, sh, parent, mapErr, err := loadSheetAndParentTopic(absPath, sheetID, parentID)
	if err != nil {
		return nil, err
	}
	if mapErr != nil {
		return mapErr, nil
	}

	topic := xmind.Topic{
		ID:    uuid.New().String(),
		Title: title,
	}
	if toolResult := applyTopicPropertiesArgs(args, &topic); toolResult != nil {
		return toolResult, nil
	}
	insertIdx, terr := insertAttachedWithSummaryAdjust(parent, topic, pos)
	if terr != nil {
		return terr, nil
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return jsonResult(addTopicResponse{
		ID:           topic.ID,
		Position:     insertIdx,
		SiblingCount: len(parent.Children.Attached),
	})
}

// DuplicateTopic deep-clones a topic subtree and attaches it as an attached child of target_parent_id.
func (h *XMindHandler) DuplicateTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	parts, terr := requireMapStrings(args, []string{"sheet_id", "topic_id", "target_parent_id"})
	if terr != nil {
		return terr, nil
	}
	sheetID, topicID, targetParentID := parts[0], parts[1], parts[2]
	pos, perr := parsePositionOptional(args["position"])
	if perr != nil {
		return perr, nil
	}

	sheets, sh, tErr, err := loadSheetByID(absPath, sheetID)
	if err != nil {
		return nil, err
	}
	if tErr != nil {
		return tErr, nil
	}
	source, targetParent, rerr := resolveDuplicateTopicPair(&sh.RootTopic, topicID, targetParentID)
	if rerr != nil {
		return rerr, nil
	}

	clone, err := deepCloneTopic(source)
	if err != nil {
		return nil, fmt.Errorf("duplicate topic: %w", err)
	}
	newRootID := clone.ID
	n := countTopics(&clone)

	insertIdx, terr := insertAttachedWithSummaryAdjust(targetParent, clone, pos)
	if terr != nil {
		return terr, nil
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return jsonResult(duplicateTopicResponse{
		SourceID:     topicID,
		NewRootID:    newRootID,
		ParentID:     targetParentID,
		CopiedCount:  n,
		Position:     insertIdx,
		SiblingCount: len(targetParent.Children.Attached),
	})
}

// AddTopicsBulk adds a nested tree under parent_id.
func (h *XMindHandler) AddTopicsBulk(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	parts, terr := requireMapStrings(args, []string{"sheet_id", "parent_id"})
	if terr != nil {
		return terr, nil
	}
	sheetID, parentID := parts[0], parts[1]
	rawTopics, aerr := parseBulkTopicsArray(args)
	if aerr != nil {
		return aerr, nil
	}

	topics, count, topicsToolErr, perr := buildTopicsFromArgs(rawTopics)
	if topicsToolErr != nil {
		return topicsToolErr, nil
	}
	if perr != nil {
		return mcp.NewToolResultError("invalid argument topics: " + perr.Error()), nil
	}

	sheets, sh, parent, mapErr, err := loadSheetAndParentTopic(absPath, sheetID, parentID)
	if err != nil {
		return nil, err
	}
	if mapErr != nil {
		return mapErr, nil
	}

	ch := ensureChildren(parent)
	prevLen := len(ch.Attached)
	ch.Attached = append(ch.Attached, topics...)
	rootIDs := make([]string, len(topics))
	for i := range topics {
		rootIDs[i] = topics[i].ID
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return jsonResult(addTopicsBulkResponse{
		AddedCount:    count,
		ParentID:      parentID,
		FirstPosition: prevLen,
		SiblingCount:  len(ch.Attached),
		RootTopicIDs:  rootIDs,
	})
}

// RenameTopic sets a topic title.
func (h *XMindHandler) RenameTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	title, terr := requireString(args, "title")
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
	topic := findTopicByID(&sh.RootTopic, topicID)
	if topic == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID)), nil
	}

	topic.Title = title
	topic.TitleUnedited = false
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("renamed topic %s", topicID)), nil
}

func deleteNonRootTopic(sh *xmind.Sheet, topicID string) *mcp.CallToolResult {
	if topicID == sh.RootTopic.ID {
		return mcp.NewToolResultError("cannot delete the root topic of a sheet")
	}
	parent, idx, listType := findParentOfTopic(&sh.RootTopic, topicID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID))
	}
	_, rerr := removeChildAt(parent, idx, listType)
	return rerr
}

// DeleteTopic removes a topic and its subtree (not the sheet root).
func (h *XMindHandler) DeleteTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if rerr := deleteNonRootTopic(sh, topicID); rerr != nil {
		return rerr, nil
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("deleted topic %s", topicID)), nil
}

func validateMoveTopicPreconditions(sh *xmind.Sheet, topicID, newParentID string, topic, _ *xmind.Topic) *mcp.CallToolResult {
	if topicID == sh.RootTopic.ID {
		return mcp.NewToolResultError("cannot move the root topic")
	}
	if isDescendantOf(topic, newParentID) {
		return mcp.NewToolResultError("cannot move a topic into its own subtree")
	}
	return nil
}

func detachTopicFromTree(root *xmind.Topic, topicID string) (*xmind.Topic, *mcp.CallToolResult) {
	parent, idx, listType := findParentOfTopic(root, topicID)
	if parent == nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID))
	}
	return removeChildAt(parent, idx, listType)
}

// attachDetachedToParent re-resolves newParent after a remove (stale pointer if sibling in same Attached slice).
func attachDetachedToParent(root *xmind.Topic, newParentID string, removed *xmind.Topic, pos *int) (insertIdx, siblingCount int, toolErr *mcp.CallToolResult) {
	newParent := findTopicByID(root, newParentID)
	if newParent == nil {
		return 0, 0, mcp.NewToolResultError(fmt.Sprintf("new parent topic not found after remove: %s", newParentID))
	}
	if terr := insertAttached(newParent, *removed, pos); terr != nil {
		return 0, 0, terr
	}
	if pos == nil {
		insertIdx = len(newParent.Children.Attached) - 1
	} else {
		insertIdx = *pos
	}
	if len(newParent.Summaries) > 0 {
		adjustSummariesAfterAttachedInsert(newParent, insertIdx)
	}
	return insertIdx, len(newParent.Children.Attached), nil
}

func moveTopicFromArgs(args map[string]any) (absPath, sheetID, topicID, newParentID string, pos *int, toolErr *mcp.CallToolResult) {
	absPath, toolErr = absPathFromArgs(args)
	if toolErr != nil {
		return "", "", "", "", nil, toolErr
	}
	sheetID, toolErr = requireString(args, "sheet_id")
	if toolErr != nil {
		return absPath, "", "", "", nil, toolErr
	}
	topicID, toolErr = requireString(args, "topic_id")
	if toolErr != nil {
		return absPath, sheetID, "", "", nil, toolErr
	}
	newParentID, toolErr = requireString(args, "new_parent_id")
	if toolErr != nil {
		return absPath, sheetID, topicID, "", nil, toolErr
	}
	pos, perr := parsePositionOptional(args["position"])
	if perr != nil {
		return absPath, sheetID, topicID, newParentID, nil, perr
	}
	return absPath, sheetID, topicID, newParentID, pos, nil
}

// MoveTopic reparents a topic under new_parent_id.
func (h *XMindHandler) MoveTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	absPath, sheetID, topicID, newParentID, pos, aerr := moveTopicFromArgs(req.GetArguments())
	if aerr != nil {
		return aerr, nil
	}

	sheets, sh, topic, newParent, ctxErr, err := loadSheetMoveSubjects(absPath, sheetID, topicID, newParentID)
	if err != nil {
		return nil, err
	}
	if ctxErr != nil {
		return ctxErr, nil
	}
	if terr := validateMoveTopicPreconditions(sh, topicID, newParentID, topic, newParent); terr != nil {
		return terr, nil
	}

	removed, rerr := detachTopicFromTree(&sh.RootTopic, topicID)
	if rerr != nil {
		return rerr, nil
	}

	moveInsertIdx, n, aerr := attachDetachedToParent(&sh.RootTopic, newParentID, removed, pos)
	if aerr != nil {
		return aerr, nil
	}

	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return jsonResult(moveTopicResponse{
		TopicID:      topicID,
		ParentID:     newParentID,
		Position:     moveInsertIdx,
		SiblingCount: n,
	})
}

func loadSheetParentTopic(absPath, sheetID, parentID string) ([]xmind.Sheet, *xmind.Sheet, *xmind.Topic, *mcp.CallToolResult, error) {
	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if toolErr2 != nil {
		return nil, nil, nil, toolErr2, nil
	}
	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return sheets, nil, nil, mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return sheets, sh, nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}
	return sheets, sh, parent, nil, nil
}

func loadSheetMoveSubjects(absPath, sheetID, topicID, newParentID string) ([]xmind.Sheet, *xmind.Sheet, *xmind.Topic, *xmind.Topic, *mcp.CallToolResult, error) {
	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if toolErr2 != nil {
		return nil, nil, nil, nil, toolErr2, nil
	}
	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return sheets, nil, nil, nil, mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}
	topic := findTopicByID(&sh.RootTopic, topicID)
	if topic == nil {
		return sheets, sh, nil, nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID)), nil
	}
	newParent := findTopicByID(&sh.RootTopic, newParentID)
	if newParent == nil {
		return sheets, sh, topic, nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", newParentID)), nil
	}
	return sheets, sh, topic, newParent, nil, nil
}

func parseOrderedIDs(rawIDs []any) ([]string, *mcp.CallToolResult) {
	orderedIDs := make([]string, 0, len(rawIDs))
	for i, v := range rawIDs {
		s, ok := v.(string)
		if !ok || s == "" {
			return nil, mcp.NewToolResultError(fmt.Sprintf("ordered_ids[%d]: expected non-empty string", i))
		}
		orderedIDs = append(orderedIDs, s)
	}
	return orderedIDs, nil
}

func validateReorderAttached(parent *xmind.Topic, orderedIDs []string) ([]xmind.Topic, *mcp.CallToolResult) {
	if parent.Children == nil || len(parent.Children.Attached) == 0 {
		return nil, mcp.NewToolResultError("parent has no attached children to reorder")
	}
	if len(orderedIDs) != len(parent.Children.Attached) {
		return nil, mcp.NewToolResultError(fmt.Sprintf("ordered_ids length %d does not match attached child count %d", len(orderedIDs), len(parent.Children.Attached)))
	}
	byID := make(map[string]xmind.Topic, len(parent.Children.Attached))
	for _, t := range parent.Children.Attached {
		byID[t.ID] = t
	}
	seen := make(map[string]struct{}, len(orderedIDs))
	newOrder := make([]xmind.Topic, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, dup := seen[id]; dup {
			return nil, mcp.NewToolResultError(fmt.Sprintf("duplicate id in ordered_ids: %s", id))
		}
		seen[id] = struct{}{}
		t, ok := byID[id]
		if !ok {
			return nil, mcp.NewToolResultError(fmt.Sprintf("ordered_ids contains unknown id: %s", id))
		}
		newOrder = append(newOrder, t)
	}
	if len(seen) != len(byID) {
		return nil, mcp.NewToolResultError("ordered_ids must list every attached child exactly once")
	}
	return newOrder, nil
}

func reorderChildrenFromArgs(args map[string]any) (absPath, sheetID, parentID string, orderedIDs []string, toolErr *mcp.CallToolResult) {
	absPath, toolErr = absPathFromArgs(args)
	if toolErr != nil {
		return "", "", "", nil, toolErr
	}
	sheetID, toolErr = requireString(args, "sheet_id")
	if toolErr != nil {
		return absPath, "", "", nil, toolErr
	}
	parentID, toolErr = requireString(args, "parent_id")
	if toolErr != nil {
		return absPath, sheetID, "", nil, toolErr
	}
	rawIDs, ok := args["ordered_ids"].([]any)
	if !ok || rawIDs == nil {
		return absPath, sheetID, parentID, nil, mcp.NewToolResultError("missing or invalid argument: ordered_ids (expected array)")
	}
	orderedIDs, oerr := parseOrderedIDs(rawIDs)
	if oerr != nil {
		return absPath, sheetID, parentID, nil, oerr
	}
	return absPath, sheetID, parentID, orderedIDs, nil
}

// ReorderChildren reorders attached children of parent_id.
func (h *XMindHandler) ReorderChildren(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	absPath, sheetID, parentID, orderedIDs, aerr := reorderChildrenFromArgs(req.GetArguments())
	if aerr != nil {
		return aerr, nil
	}

	sheets, sh, parent, ctxErr, err := loadSheetParentTopic(absPath, sheetID, parentID)
	if err != nil {
		return nil, err
	}
	if ctxErr != nil {
		return ctxErr, nil
	}
	newOrder, verr := validateReorderAttached(parent, orderedIDs)
	if verr != nil {
		return verr, nil
	}

	parent.Children.Attached = newOrder
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("reordered %d children under %s", len(newOrder), parentID)), nil
}

func notesArgActionable(args map[string]any) bool {
	_, has := args["notes"]
	return has
}

func nonNilArgActionable(args map[string]any, key string) bool {
	v, has := args[key]
	return has && v != nil
}

func removeMarkersArgActionable(args map[string]any) bool {
	v, has := args["remove_markers"]
	if !has || v == nil {
		return false
	}
	arr, ok := v.([]any)
	if !ok {
		return true
	}
	return len(arr) > 0
}

// hasActionableTopicPropertyArgs reports whether bulk set-topic-properties has at least one
// property argument that would run a mutating branch (same rules as the single-topic tool).
// Keep in sync with applyTopicPropertiesArgs: notes ⇒ key present; labels, markers, link ⇒ key present and value non-nil;
// remove_markers ⇒ key present and non-nil, and (wrong type OR len(slice) > 0).
func hasActionableTopicPropertyArgs(args map[string]any) bool {
	return notesArgActionable(args) ||
		nonNilArgActionable(args, "labels") ||
		nonNilArgActionable(args, "markers") ||
		nonNilArgActionable(args, "link") ||
		removeMarkersArgActionable(args)
}

func applyNotesFromArgs(args map[string]any, topic *xmind.Topic) *mcp.CallToolResult {
	v, has := args["notes"]
	if !has {
		return nil
	}
	if v == nil {
		topic.Notes = nil
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return mcp.NewToolResultError("invalid argument notes: expected a string")
	}
	if s == "" {
		topic.Notes = nil
		return nil
	}
	topic.Notes = &xmind.Notes{
		Plain:    &xmind.NoteContent{Content: s},
		RealHTML: &xmind.NoteContent{Content: plainToRealHTML(s)},
	}
	return nil
}

func applyLabelsFromArgs(args map[string]any, topic *xmind.Topic) *mcp.CallToolResult {
	v, has := args["labels"]
	if !has || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return mcp.NewToolResultError("invalid argument labels: expected an array")
	}
	labels, err := stringSliceFromAnyArray(arr, "labels")
	if err != nil {
		return err
	}
	topic.Labels = labels
	return nil
}

func applyMarkersFromArgs(args map[string]any, topic *xmind.Topic) *mcp.CallToolResult {
	v, has := args["markers"]
	if !has || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return mcp.NewToolResultError("invalid argument markers: expected an array")
	}
	ss, err := stringSliceFromAnyArray(arr, "markers")
	if err != nil {
		return err
	}
	markers := make([]xmind.Marker, len(ss))
	for i, s := range ss {
		markers[i] = xmind.Marker{MarkerID: s}
	}
	topic.Markers = markers
	return nil
}

func applyRemoveMarkersFromArgs(args map[string]any, topic *xmind.Topic) *mcp.CallToolResult {
	v, has := args["remove_markers"]
	if !has || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return mcp.NewToolResultError("invalid argument remove_markers: expected an array")
	}
	if len(arr) == 0 {
		return nil
	}
	remove, err := stringSetFromAnyArray(arr, "remove_markers")
	if err != nil {
		return err
	}
	out := make([]xmind.Marker, 0, len(topic.Markers))
	for _, m := range topic.Markers {
		if _, drop := remove[m.MarkerID]; !drop {
			out = append(out, m)
		}
	}
	topic.Markers = out
	return nil
}

func applyLinkFromArgs(args map[string]any, topic *xmind.Topic) *mcp.CallToolResult {
	v, has := args["link"]
	if !has || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return mcp.NewToolResultError("invalid argument link: expected a string")
	}
	topic.Href = s
	return nil
}

// stringSliceFromAnyArray parses arr into []string, using key as the argument name for errors ("labels[0]: …").
func stringSliceFromAnyArray(arr []any, key string) ([]string, *mcp.CallToolResult) {
	out := make([]string, 0, len(arr))
	for i, el := range arr {
		s, ok := el.(string)
		if !ok {
			return nil, mcp.NewToolResultError(fmt.Sprintf("%s[%d]: expected string", key, i))
		}
		out = append(out, s)
	}
	return out, nil
}

// stringSetFromAnyArray parses arr into a set of strings for remove_markers-style args.
func stringSetFromAnyArray(arr []any, key string) (map[string]struct{}, *mcp.CallToolResult) {
	remove := make(map[string]struct{}, len(arr))
	for i, el := range arr {
		s, ok := el.(string)
		if !ok {
			return nil, mcp.NewToolResultError(fmt.Sprintf("%s[%d]: expected string", key, i))
		}
		remove[s] = struct{}{}
	}
	return remove, nil
}

// applyTopicPropertiesArgs applies notes, labels, markers, remove_markers, and link from args
// to topic using the same semantics as xmind_set_topic_properties.
func applyTopicPropertiesArgs(args map[string]any, topic *xmind.Topic) *mcp.CallToolResult {
	if r := applyNotesFromArgs(args, topic); r != nil {
		return r
	}
	if r := applyLabelsFromArgs(args, topic); r != nil {
		return r
	}
	if r := applyMarkersFromArgs(args, topic); r != nil {
		return r
	}
	if r := applyRemoveMarkersFromArgs(args, topic); r != nil {
		return r
	}
	if r := applyLinkFromArgs(args, topic); r != nil {
		return r
	}
	return nil
}

// parseTopicIDsArgs validates args["topic_ids"] as a non-empty slice of unique non-empty strings.
func parseTopicIDsArgs(args map[string]any) ([]string, *mcp.CallToolResult) {
	raw, has := args["topic_ids"]
	if !has {
		return nil, mcp.NewToolResultError("missing required argument: topic_ids")
	}
	rawIDs, ok := raw.([]any)
	if !ok {
		return nil, mcp.NewToolResultError("invalid argument topic_ids: expected an array")
	}
	return collectUniqueTopicIDs(rawIDs)
}

func collectUniqueTopicIDs(rawIDs []any) ([]string, *mcp.CallToolResult) {
	topicIDs := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for i, v := range rawIDs {
		s, ok := v.(string)
		if !ok || s == "" {
			return nil, mcp.NewToolResultError(fmt.Sprintf("topic_ids[%d]: expected non-empty string", i))
		}
		if _, dup := seen[s]; dup {
			return nil, mcp.NewToolResultError(fmt.Sprintf("duplicate id in topic_ids: %s", s))
		}
		seen[s] = struct{}{}
		topicIDs = append(topicIDs, s)
	}
	if len(topicIDs) == 0 {
		return nil, mcp.NewToolResultError("topic_ids must be non-empty")
	}
	return topicIDs, nil
}

// resolveTopicsForSheet returns pointers to topics for each id in order, or a tool error listing missing ids.
func resolveTopicsForSheet(root *xmind.Topic, topicIDs []string) ([]*xmind.Topic, *mcp.CallToolResult) {
	topics := make([]*xmind.Topic, 0, len(topicIDs))
	missing := make([]string, 0)
	for _, id := range topicIDs {
		t := findTopicByID(root, id)
		if t == nil {
			missing = append(missing, id)
		} else {
			topics = append(topics, t)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", strings.Join(missing, ", ")))
	}
	return topics, nil
}

// loadSheetByID reads the map and returns the sheet with sheetID, or a tool/protocol error.
func loadSheetByID(absPath, sheetID string) ([]xmind.Sheet, *xmind.Sheet, *mcp.CallToolResult, error) {
	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if toolErr2 != nil {
		return nil, nil, toolErr2, nil
	}
	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return nil, nil, mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}
	return sheets, sh, nil, nil
}

// loadSheetAndParentTopic loads the map and resolves parentID under the sheet root.
func loadSheetAndParentTopic(absPath, sheetID, parentID string) ([]xmind.Sheet, *xmind.Sheet, *xmind.Topic, *mcp.CallToolResult, error) {
	sheets, sh, tErr, err := loadSheetByID(absPath, sheetID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if tErr != nil {
		return nil, nil, nil, tErr, nil
	}
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return sheets, sh, nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}
	return sheets, sh, parent, nil, nil
}

func parseBulkTopicsArray(args map[string]any) ([]any, *mcp.CallToolResult) {
	rawTopics, ok := args["topics"].([]any)
	if !ok || rawTopics == nil {
		return nil, mcp.NewToolResultError("missing or invalid argument: topics (expected array)")
	}
	return rawTopics, nil
}

func requireRelationshipEndpoints(root *xmind.Topic, fromID, toID string) *mcp.CallToolResult {
	if findTopicByID(root, fromID) == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", fromID))
	}
	if findTopicByID(root, toID) == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", toID))
	}
	return nil
}

func appendRelationship(sh *xmind.Sheet, fromID, toID, label string) string {
	relID := uuid.New().String()
	sh.Relationships = append(sh.Relationships, xmind.Relationship{
		ID:     relID,
		End1ID: fromID,
		End2ID: toID,
		Title:  label,
	})
	return relID
}

func validateParentHasAttachedForBoundary(parent *xmind.Topic) *mcp.CallToolResult {
	if parent.Children == nil || len(parent.Children.Attached) == 0 {
		return mcp.NewToolResultError("parent has no attached children for a boundary")
	}
	return nil
}

func resolveDuplicateTopicPair(root *xmind.Topic, topicID, targetParentID string) (*xmind.Topic, *xmind.Topic, *mcp.CallToolResult) {
	source := findTopicByID(root, topicID)
	if source == nil {
		return nil, nil, mcp.NewToolResultError(fmt.Sprintf("source topic not found: %s", topicID))
	}
	targetParent := findTopicByID(root, targetParentID)
	if targetParent == nil {
		return nil, nil, mcp.NewToolResultError(fmt.Sprintf("target parent not found: %s", targetParentID))
	}
	return source, targetParent, nil
}

func applyTopicPropertiesToTopics(args map[string]any, topics []*xmind.Topic) *mcp.CallToolResult {
	for _, topic := range topics {
		if r := applyTopicPropertiesArgs(args, topic); r != nil {
			return r
		}
	}
	return nil
}

// SetTopicProperties updates optional metadata on a topic.
func (h *XMindHandler) SetTopicProperties(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	topic := findTopicByID(&sh.RootTopic, topicID)
	if topic == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID)), nil
	}

	if toolResult := applyTopicPropertiesArgs(args, topic); toolResult != nil {
		return toolResult, nil
	}

	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("updated topic %s", topicID)), nil
}

// SetTopicPropertiesBulk updates optional metadata on multiple topics in a single read/write cycle.
func (h *XMindHandler) SetTopicPropertiesBulk(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	topicIDs, idErr := parseTopicIDsArgs(args)
	if idErr != nil {
		return idErr, nil
	}

	if !hasActionableTopicPropertyArgs(args) {
		return mcp.NewToolResultError("missing property updates: provide at least one of notes, labels, markers, remove_markers, or link"), nil
	}

	sheets, sh, sheetErr, err := loadSheetByID(absPath, sheetID)
	if err != nil {
		return nil, err
	}
	if sheetErr != nil {
		return sheetErr, nil
	}

	topics, missErr := resolveTopicsForSheet(&sh.RootTopic, topicIDs)
	if missErr != nil {
		return missErr, nil
	}

	if toolResult := applyTopicPropertiesToTopics(args, topics); toolResult != nil {
		return toolResult, nil
	}

	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("updated %d topics", len(topicIDs))), nil
}

// AddFloatingTopic adds a detached topic on the sheet root.
func (h *XMindHandler) AddFloatingTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	title, terr := requireString(args, "title")
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
	root := &sh.RootTopic
	n := 0
	if root.Children != nil {
		n = len(root.Children.Detached)
	}
	topic := xmind.Topic{
		ID:       uuid.New().String(),
		Title:    title,
		Position: &xmind.Position{X: 200, Y: float64(200 + n*60)},
	}
	ch := ensureChildren(root)
	ch.Detached = append(ch.Detached, topic)
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added floating topic id %s", topic.ID)), nil
}

// AddRelationship appends a sheet-level relationship between two topics.
func (h *XMindHandler) AddRelationship(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	fromID, terr := requireString(args, "from_id")
	if terr != nil {
		return terr, nil
	}
	toID, terr := requireString(args, "to_id")
	if terr != nil {
		return terr, nil
	}
	label, oerr := optionalStringArg(args, "label")
	if oerr != nil {
		return oerr, nil
	}

	sheets, sh, tErr, err := loadSheetByID(absPath, sheetID)
	if err != nil {
		return nil, err
	}
	if tErr != nil {
		return tErr, nil
	}
	if e := requireRelationshipEndpoints(&sh.RootTopic, fromID, toID); e != nil {
		return e, nil
	}

	relID := appendRelationship(sh, fromID, toID, label)
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added relationship id %s", relID)), nil
}

// DeleteRelationship removes a sheet-level relationship by id.
func (h *XMindHandler) DeleteRelationship(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	relationshipID, terr := requireString(args, "relationship_id")
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

	idx := slices.IndexFunc(sh.Relationships, func(r xmind.Relationship) bool { return r.ID == relationshipID })
	if idx < 0 {
		return mcp.NewToolResultError(fmt.Sprintf("relationship not found on sheet %s: %s", sheetID, relationshipID)), nil
	}
	sh.Relationships = slices.Delete(sh.Relationships, idx, idx+1)
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("deleted relationship id %s", relationshipID)), nil
}

// AddSummary adds a summary topic and range descriptor on a parent (double-write).
func (h *XMindHandler) AddSummary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	parts, terr := requireMapStrings(args, []string{"sheet_id", "parent_id"})
	if terr != nil {
		return terr, nil
	}
	sheetID, parentID := parts[0], parts[1]
	fromIdx, terr := requireNonNegativeInt(args, "from_index")
	if terr != nil {
		return terr, nil
	}
	toIdx, terr := requireNonNegativeInt(args, "to_index")
	if terr != nil {
		return terr, nil
	}
	title, oerr := optionalStringArg(args, "title")
	if oerr != nil {
		return oerr, nil
	}

	sheets, sh, parent, mapErr, err := loadSheetAndParentTopic(absPath, sheetID, parentID)
	if err != nil {
		return nil, err
	}
	if mapErr != nil {
		return mapErr, nil
	}
	if verr := validateAddSummary(parent, fromIdx, toIdx); verr != nil {
		return verr, nil
	}

	summaryTopicID := applyAddSummary(parent, fromIdx, toIdx, title)

	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added summary topic id %s", summaryTopicID)), nil
}

// AddBoundary adds a boundary around all attached children of a parent.
func (h *XMindHandler) AddBoundary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	parentID, terr := requireString(args, "parent_id")
	if terr != nil {
		return terr, nil
	}
	title, oerr := optionalStringArg(args, "title")
	if oerr != nil {
		return oerr, nil
	}

	sheets, sh, parent, mapErr, err := loadSheetAndParentTopic(absPath, sheetID, parentID)
	if err != nil {
		return nil, err
	}
	if mapErr != nil {
		return mapErr, nil
	}
	if verr := validateParentHasAttachedForBoundary(parent); verr != nil {
		return verr, nil
	}

	boundaryID := uuid.New().String()
	b := xmind.Boundary{
		ID:    boundaryID,
		Range: "master",
		Title: title,
	}
	parent.Boundaries = append(parent.Boundaries, b)

	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added boundary id %s", boundaryID)), nil
}

// optionalStringArg returns ("", nil) when key is absent or null; on wrong type returns a tool error.
func optionalStringArg(args map[string]any, key string) (string, *mcp.CallToolResult) {
	v, has := args[key]
	if !has || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", mcp.NewToolResultError(fmt.Sprintf("invalid argument %s: expected a string", key))
	}
	return s, nil
}

func requireNonNegativeInt(args map[string]any, key string) (int, *mcp.CallToolResult) {
	v, ok := args[key]
	if !ok {
		return 0, mcp.NewToolResultError("missing required argument: " + key)
	}
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, mcp.NewToolResultError("invalid argument " + key + ": must be a finite number")
		}
		trunc := math.Trunc(n)
		if trunc != n {
			return 0, mcp.NewToolResultError("invalid argument " + key + ": must be a whole number")
		}
		i := int(trunc)
		if i < 0 {
			return 0, mcp.NewToolResultError("invalid argument " + key + ": must be non-negative")
		}
		return i, nil
	case int:
		if n < 0 {
			return 0, mcp.NewToolResultError("invalid argument " + key + ": must be non-negative")
		}
		return n, nil
	default:
		return 0, mcp.NewToolResultError("invalid argument " + key + ": expected a number")
	}
}

func requireString(args map[string]any, key string) (string, *mcp.CallToolResult) {
	v, ok := args[key]
	if !ok {
		return "", mcp.NewToolResultError("missing required argument: " + key)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", mcp.NewToolResultError("invalid argument " + key + ": expected a non-empty string")
	}
	return s, nil
}

// requireMapStrings returns requireString values for keys in order.
func requireMapStrings(args map[string]any, keys []string) ([]string, *mcp.CallToolResult) {
	out := make([]string, len(keys))
	for i, k := range keys {
		s, err := requireString(args, k)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}
