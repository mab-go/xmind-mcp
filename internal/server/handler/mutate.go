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
// Each element is a map with required "title" and optional "children" ([]any).
func buildTopicsFromArgs(raw []any) ([]xmind.Topic, int, error) {
	var total int
	out, err := buildTopicsFromArgsDepth(raw, 0, &total)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func buildTopicsFromArgsDepth(raw []any, depth int, total *int) ([]xmind.Topic, error) {
	if depth > maxBulkTopicsDepth {
		return nil, fmt.Errorf("maximum nesting depth is %d", maxBulkTopicsDepth)
	}
	out := make([]xmind.Topic, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("topics[%d]: expected object", i)
		}
		titleVal, ok := m["title"]
		if !ok {
			return nil, fmt.Errorf("topics[%d]: missing title", i)
		}
		title, ok := titleVal.(string)
		if !ok {
			return nil, fmt.Errorf("topics[%d]: title must be a string", i)
		}
		if *total >= maxBulkTopicsTotal {
			return nil, fmt.Errorf("maximum topic count is %d", maxBulkTopicsTotal)
		}
		topic := xmind.Topic{
			ID:    uuid.New().String(),
			Title: title,
		}
		*total++
		if ch, has := m["children"]; has && ch != nil {
			arr, ok := ch.([]any)
			if !ok {
				return nil, fmt.Errorf("topics[%d]: children must be an array", i)
			}
			children, err := buildTopicsFromArgsDepth(arr, depth+1, total)
			if err != nil {
				return nil, err
			}
			topic.Children = &xmind.Children{Attached: children}
		}
		out = append(out, topic)
	}
	return out, nil
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

// adjustSummariesAfterAttachedRemove updates parent.Summaries and Children.Summary after
// removing the attached child at removedIndex.
func adjustSummariesAfterAttachedRemove(parent *xmind.Topic, removedIndex int) {
	if parent == nil || parent.Children == nil {
		return
	}
	N := removedIndex
	removeTopicIDs := make(map[string]struct{})
	var newSummaries []xmind.Summary
	for _, s := range parent.Summaries {
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
	parent.Summaries = newSummaries
	if len(parent.Summaries) == 0 {
		parent.Summaries = nil
	}
	if len(removeTopicIDs) == 0 {
		return
	}
	var kept []xmind.Topic
	for i := range parent.Children.Summary {
		if _, drop := removeTopicIDs[parent.Children.Summary[i].ID]; !drop {
			kept = append(kept, parent.Children.Summary[i])
		}
	}
	parent.Children.Summary = kept
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

func removeChildAt(parent *xmind.Topic, idx int, listType string) (*xmind.Topic, *mcp.CallToolResult) {
	if parent == nil || parent.Children == nil {
		return nil, mcp.NewToolResultError("internal error: parent has no children")
	}
	switch listType {
	case "attached":
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
	case "detached":
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
	case "summary":
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
	sheetID, terr := requireString(args, "sheet_id")
	if terr != nil {
		return terr, nil
	}
	parentID, terr := requireString(args, "parent_id")
	if terr != nil {
		return terr, nil
	}
	title, terr := requireString(args, "title")
	if terr != nil {
		return terr, nil
	}
	pos, perr := parsePositionOptional(args["position"])
	if perr != nil {
		return perr, nil
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
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}

	topic := xmind.Topic{
		ID:    uuid.New().String(),
		Title: title,
	}
	if terr := insertAttached(parent, topic, pos); terr != nil {
		return terr, nil
	}
	insertIdx := 0
	if pos == nil {
		insertIdx = len(parent.Children.Attached) - 1
	} else {
		insertIdx = *pos
	}
	if len(parent.Summaries) > 0 {
		adjustSummariesAfterAttachedInsert(parent, insertIdx)
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added topic id %s", topic.ID)), nil
}

// AddTopicsBulk adds a nested tree under parent_id.
func (h *XMindHandler) AddTopicsBulk(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	rawTopics, ok := args["topics"].([]any)
	if !ok || rawTopics == nil {
		return mcp.NewToolResultError("missing or invalid argument: topics (expected array)"), nil
	}

	topics, count, perr := buildTopicsFromArgs(rawTopics)
	if perr != nil {
		return mcp.NewToolResultError("invalid argument topics: " + perr.Error()), nil
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
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}

	ch := ensureChildren(parent)
	ch.Attached = append(ch.Attached, topics...)
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added %d topics under parent %s", count, parentID)), nil
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
	if topicID == sh.RootTopic.ID {
		return mcp.NewToolResultError("cannot delete the root topic of a sheet"), nil
	}
	parent, idx, listType := findParentOfTopic(&sh.RootTopic, topicID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID)), nil
	}

	_, rerr := removeChildAt(parent, idx, listType)
	if rerr != nil {
		return rerr, nil
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("deleted topic %s", topicID)), nil
}

// MoveTopic reparents a topic under new_parent_id.
func (h *XMindHandler) MoveTopic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	newParentID, terr := requireString(args, "new_parent_id")
	if terr != nil {
		return terr, nil
	}
	pos, perr := parsePositionOptional(args["position"])
	if perr != nil {
		return perr, nil
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
	if topicID == sh.RootTopic.ID {
		return mcp.NewToolResultError("cannot move the root topic"), nil
	}
	newParent := findTopicByID(&sh.RootTopic, newParentID)
	if newParent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", newParentID)), nil
	}
	if isDescendantOf(topic, newParentID) {
		return mcp.NewToolResultError("cannot move a topic into its own subtree"), nil
	}

	parent, idx, listType := findParentOfTopic(&sh.RootTopic, topicID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", topicID)), nil
	}

	removed, rerr := removeChildAt(parent, idx, listType)
	if rerr != nil {
		return rerr, nil
	}

	// Re-resolve newParent after the slice mutation: the pointer obtained before
	// removeChildAt may be stale if newParent was a sibling of the removed topic
	// in the same Attached slice (slices.Delete shifts elements in place).
	newParent = findTopicByID(&sh.RootTopic, newParentID)
	if newParent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("new parent topic not found after remove: %s", newParentID)), nil
	}

	if terr := insertAttached(newParent, *removed, pos); terr != nil {
		return terr, nil
	}
	moveInsertIdx := 0
	if pos == nil {
		moveInsertIdx = len(newParent.Children.Attached) - 1
	} else {
		moveInsertIdx = *pos
	}
	if len(newParent.Summaries) > 0 {
		adjustSummariesAfterAttachedInsert(newParent, moveInsertIdx)
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("moved topic %s under %s", topicID, newParentID)), nil
}

// ReorderChildren reorders attached children of parent_id.
func (h *XMindHandler) ReorderChildren(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	rawIDs, ok := args["ordered_ids"].([]any)
	if !ok || rawIDs == nil {
		return mcp.NewToolResultError("missing or invalid argument: ordered_ids (expected array)"), nil
	}
	orderedIDs := make([]string, 0, len(rawIDs))
	for i, v := range rawIDs {
		s, ok := v.(string)
		if !ok || s == "" {
			return mcp.NewToolResultError(fmt.Sprintf("ordered_ids[%d]: expected non-empty string", i)), nil
		}
		orderedIDs = append(orderedIDs, s)
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
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}
	if parent.Children == nil || len(parent.Children.Attached) == 0 {
		return mcp.NewToolResultError("parent has no attached children to reorder"), nil
	}
	if len(orderedIDs) != len(parent.Children.Attached) {
		return mcp.NewToolResultError(fmt.Sprintf("ordered_ids length %d does not match attached child count %d", len(orderedIDs), len(parent.Children.Attached))), nil
	}

	byID := make(map[string]xmind.Topic, len(parent.Children.Attached))
	for _, t := range parent.Children.Attached {
		byID[t.ID] = t
	}
	seen := make(map[string]struct{}, len(orderedIDs))
	newOrder := make([]xmind.Topic, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, dup := seen[id]; dup {
			return mcp.NewToolResultError(fmt.Sprintf("duplicate id in ordered_ids: %s", id)), nil
		}
		seen[id] = struct{}{}
		t, ok := byID[id]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("ordered_ids contains unknown id: %s", id)), nil
		}
		newOrder = append(newOrder, t)
	}
	if len(seen) != len(byID) {
		return mcp.NewToolResultError("ordered_ids must list every attached child exactly once"), nil
	}

	parent.Children.Attached = newOrder
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("reordered %d children under %s", len(newOrder), parentID)), nil
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

	if v, has := args["notes"]; has && v != nil {
		s, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument notes: expected a string"), nil
		}
		topic.Notes = &xmind.Notes{
			Plain:    &xmind.NoteContent{Content: s},
			RealHTML: &xmind.NoteContent{Content: plainToRealHTML(s)},
		}
	}
	if v, has := args["labels"]; has && v != nil {
		arr, ok := v.([]any)
		if !ok {
			return mcp.NewToolResultError("invalid argument labels: expected an array"), nil
		}
		labels := make([]string, 0, len(arr))
		for i, el := range arr {
			s, ok := el.(string)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("labels[%d]: expected string", i)), nil
			}
			labels = append(labels, s)
		}
		topic.Labels = labels
	}
	if v, has := args["markers"]; has && v != nil {
		arr, ok := v.([]any)
		if !ok {
			return mcp.NewToolResultError("invalid argument markers: expected an array"), nil
		}
		markers := make([]xmind.Marker, 0, len(arr))
		for i, el := range arr {
			s, ok := el.(string)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("markers[%d]: expected string", i)), nil
			}
			markers = append(markers, xmind.Marker{MarkerID: s})
		}
		topic.Markers = markers
	}
	if v, has := args["link"]; has && v != nil {
		s, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument link: expected a string"), nil
		}
		topic.Href = s
	}

	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("updated topic %s", topicID)), nil
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
	var label string
	if v, has := args["label"]; has && v != nil {
		s, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument label: expected a string"), nil
		}
		label = s
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
	if findTopicByID(&sh.RootTopic, fromID) == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", fromID)), nil
	}
	if findTopicByID(&sh.RootTopic, toID) == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", toID)), nil
	}

	relID := uuid.New().String()
	rel := xmind.Relationship{
		ID:     relID,
		End1ID: fromID,
		End2ID: toID,
		Title:  label,
	}
	sh.Relationships = append(sh.Relationships, rel)
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("added relationship id %s", relID)), nil
}

// AddSummary adds a summary topic and range descriptor on a parent (double-write).
func (h *XMindHandler) AddSummary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	fromIdx, terr := requireNonNegativeInt(args, "from_index")
	if terr != nil {
		return terr, nil
	}
	toIdx, terr := requireNonNegativeInt(args, "to_index")
	if terr != nil {
		return terr, nil
	}
	var title string
	if v, has := args["title"]; has && v != nil {
		s, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument title: expected a string"), nil
		}
		title = s
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
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}
	if parent.Children == nil || len(parent.Children.Attached) == 0 {
		return mcp.NewToolResultError("parent has no attached children to summarize"), nil
	}
	n := len(parent.Children.Attached)
	if fromIdx > toIdx {
		return mcp.NewToolResultError("invalid summary range: from_index must be <= to_index"), nil
	}
	if toIdx >= n {
		return mcp.NewToolResultError(fmt.Sprintf("to_index %d is out of range for %d attached children", toIdx, n)), nil
	}

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
	var title string
	if v, has := args["title"]; has && v != nil {
		s, ok := v.(string)
		if !ok {
			return mcp.NewToolResultError("invalid argument title: expected a string"), nil
		}
		title = s
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
	parent := findTopicByID(&sh.RootTopic, parentID)
	if parent == nil {
		return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
	}
	if parent.Children == nil || len(parent.Children.Attached) == 0 {
		return mcp.NewToolResultError("parent has no attached children for a boundary"), nil
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
