package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

type outlinePair struct {
	depth int
	title string
}

type outlineNode struct {
	title    string
	children []*outlineNode
}

// FlattenToOutline exports attached subtree as text or markdown.
func (h *XMindHandler) FlattenToOutline(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	format, includeNotes, optErr := outlineExportOptionsFromArgs(args)
	if optErr != nil {
		return optErr, nil
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

	start, stErr := outlineStartTopic(sh, args)
	if stErr != nil {
		return stErr, nil
	}

	var b strings.Builder
	flattenTopicWrite(&b, start, 0, format, includeNotes)
	return textResult(strings.TrimRight(b.String(), "\n")), nil
}

func outlineExportOptionsFromArgs(args map[string]any) (format string, includeNotes bool, toolErr *mcp.CallToolResult) {
	format = "markdown"
	if v, ok := args["format"].(string); ok && v != "" {
		format = v
	}
	if format != "text" && format != "markdown" {
		return "", false, mcp.NewToolResultError(`invalid argument format: expected "text" or "markdown"`)
	}
	var err *mcp.CallToolResult
	includeNotes, err = parseBoolOptional(args, "include_notes")
	if err != nil {
		return "", false, err
	}
	return format, includeNotes, nil
}

func outlineStartTopic(sh *xmind.Sheet, args map[string]any) (*xmind.Topic, *mcp.CallToolResult) {
	if v, ok := args["topic_id"].(string); ok && v != "" {
		start := findTopicByID(&sh.RootTopic, v)
		if start == nil {
			return nil, mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", v))
		}
		return start, nil
	}
	return &sh.RootTopic, nil
}

func flattenTopicWriteText(b *strings.Builder, title, plain string, depth int, hasNotes bool) {
	b.WriteString(strings.Repeat(" ", depth*2))
	b.WriteString(title)
	b.WriteByte('\n')
	if !hasNotes {
		return
	}
	noteIndent := strings.Repeat(" ", depth*2+4)
	for _, line := range strings.Split(plain, "\n") {
		b.WriteString(noteIndent)
		b.WriteString("[note] ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func flattenTopicWriteMarkdown(b *strings.Builder, title, plain string, depth int, hasNotes bool) {
	// Use heading lines at every depth so import heading-mode parses the full tree (list lines would be ignored).
	level := depth + 1
	if level > 6 {
		level = 6
	}
	b.WriteString(strings.Repeat("#", level))
	b.WriteString(" ")
	b.WriteString(title)
	b.WriteByte('\n')
	if hasNotes {
		for _, line := range strings.Split(plain, "\n") {
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
		return
	}
	if depth == 0 {
		b.WriteByte('\n')
	}
}

func flattenTopicWrite(b *strings.Builder, t *xmind.Topic, depth int, format string, includeNotes bool) {
	title := ""
	plain := ""
	if t != nil {
		title = t.Title
		plain = plainNoteContent(t)
	}
	hasNotes := includeNotes && plain != ""

	switch format {
	case "text":
		flattenTopicWriteText(b, title, plain, depth, hasNotes)
	case "markdown":
		flattenTopicWriteMarkdown(b, title, plain, depth, hasNotes)
	}
	if t == nil || t.Children == nil {
		return
	}
	for i := range t.Children.Attached {
		flattenTopicWrite(b, &t.Children.Attached[i], depth+1, format, includeNotes)
	}
}

// ImportFromOutline parses outline text and writes topics into the map.
func (h *XMindHandler) ImportFromOutline(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	outline, terr := requireString(args, "outline")
	if terr != nil {
		return terr, nil
	}
	sheetIDArg, parentIDArg, aerr := parseImportOutlineSheetArgs(args)
	if aerr != nil {
		return aerr, nil
	}

	pairs := parseOutlineToPairs(outline)
	if len(pairs) == 0 {
		return mcp.NewToolResultError("outline is empty or has no topic lines"), nil
	}

	normalizeOutlineDepths(pairs)

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	if sheetIDArg == "" {
		return importOutlineNewSheet(absPath, sheets, pairs)
	}
	return importOutlineAppend(absPath, sheets, sheetIDArg, parentIDArg, pairs)
}

func parseImportOutlineSheetArgs(args map[string]any) (sheetID, parentID string, toolErr *mcp.CallToolResult) {
	if v, ok := args["sheet_id"].(string); ok {
		sheetID = v
	}
	if v, ok := args["parent_id"].(string); ok && v != "" {
		parentID = v
	}
	if parentID != "" && sheetID == "" {
		return "", "", mcp.NewToolResultError("invalid arguments: parent_id requires sheet_id (omit parent_id when creating a new sheet)")
	}
	return sheetID, parentID, nil
}

// importOutlineNewSheet: single tree; first line is sheet+root title; extra depth-0 lines are children of root.
func importOutlineNewSheet(absPath string, sheets []xmind.Sheet, pairs []outlinePair) (*mcp.CallToolResult, error) {
	root := buildOutlineTreeSingleRoot(pairs)
	if root == nil {
		return mcp.NewToolResultError("could not build outline tree"), nil
	}
	bumpAllSheetsRevisionID(sheets)
	sh := newSheet(root.title, root.title)
	ch := ensureChildren(&sh.RootTopic)
	for _, c := range root.children {
		ch.Attached = append(ch.Attached, outlineNodeToTopics(c))
	}
	sheets = append(sheets, sh)
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	n := countTopics(&sh.RootTopic)
	return textResult(fmt.Sprintf("imported %d topics into new sheet id %s", n, sh.ID)), nil
}

func importOutlineAppend(absPath string, sheets []xmind.Sheet, sheetID, parentID string, pairs []outlinePair) (*mcp.CallToolResult, error) {
	sh := findSheetByID(sheets, sheetID)
	if sh == nil {
		return mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}
	var parent *xmind.Topic
	if parentID == "" {
		parent = &sh.RootTopic
	} else {
		parent = findTopicByID(&sh.RootTopic, parentID)
		if parent == nil {
			return mcp.NewToolResultError(fmt.Sprintf("topic not found: %s", parentID)), nil
		}
	}
	forest := buildOutlineForest(pairs)
	ch := ensureChildren(parent)
	total := 0
	for _, r := range forest {
		t := outlineNodeToTopics(r)
		ch.Attached = append(ch.Attached, t)
		total += countTopics(&t)
	}
	sh.RevisionID = uuid.New().String()
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}
	return textResult(fmt.Sprintf("imported %d topics", total)), nil
}

func firstNonEmptyLineIndex(lines []string) (idx int, ok bool) {
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return i, true
	}
	return 0, false
}

func detectOutlineMode(trimmedFirst string) string {
	switch {
	case strings.HasPrefix(trimmedFirst, "#"):
		return "heading"
	case strings.HasPrefix(trimmedFirst, "-") || strings.HasPrefix(trimmedFirst, "*"):
		return "list"
	default:
		return "plain"
	}
}

func headingHashPrefixLen(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	return n
}

func appendOutlinePairHeading(line string, pairs *[]outlinePair) {
	s := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(s, "#") {
		return
	}
	n := headingHashPrefixLen(s)
	if n == 0 {
		return
	}
	rest := strings.TrimSpace(s[n:])
	*pairs = append(*pairs, outlinePair{depth: n - 1, title: rest})
}

func appendOutlinePairList(line string, pairs *[]outlinePair) {
	lead := countLeadingSpaceTabs(line)
	s := strings.TrimLeft(line, " \t")
	var title string
	switch {
	case strings.HasPrefix(s, "- "):
		title = strings.TrimSpace(s[2:])
	case strings.HasPrefix(s, "* "):
		title = strings.TrimSpace(s[2:])
	case s == "-" || s == "*":
		title = ""
	default:
		return
	}
	*pairs = append(*pairs, outlinePair{depth: lead / 2, title: title})
}

func appendOutlinePairPlain(line string, pairs *[]outlinePair) {
	lead := countLeadingSpaceTabs(line)
	title := strings.TrimSpace(line)
	*pairs = append(*pairs, outlinePair{depth: lead / 2, title: title})
}

func parseOutlineToPairs(outline string) []outlinePair {
	lines := strings.Split(outline, "\n")
	firstIdx, ok := firstNonEmptyLineIndex(lines)
	if !ok {
		return nil
	}
	mode := detectOutlineMode(strings.TrimLeft(lines[firstIdx], " \t"))

	var pairs []outlinePair
	for _, line := range lines[firstIdx:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch mode {
		case "heading":
			appendOutlinePairHeading(line, &pairs)
		case "list":
			appendOutlinePairList(line, &pairs)
		case "plain":
			appendOutlinePairPlain(line, &pairs)
		}
	}
	return pairs
}

func countLeadingSpaceTabs(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n
		}
	}
	return n
}

func normalizeOutlineDepths(pairs []outlinePair) {
	if len(pairs) == 0 {
		return
	}
	minD := pairs[0].depth
	for i := 1; i < len(pairs); i++ {
		if pairs[i].depth < minD {
			minD = pairs[i].depth
		}
	}
	for i := range pairs {
		pairs[i].depth -= minD
	}
}

// buildOutlineTreeSingleRoot: first depth-0 is root; further depth-0 become children of that root.
func buildOutlineTreeSingleRoot(pairs []outlinePair) *outlineNode {
	if len(pairs) == 0 {
		return nil
	}
	root := &outlineNode{title: pairs[0].title}
	stack := []*outlineNode{root}
	for i := 1; i < len(pairs); i++ {
		d, title := pairs[i].depth, pairs[i].title
		node := &outlineNode{title: title}
		if d == 0 {
			for len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			root.children = append(root.children, node)
			stack = []*outlineNode{root, node}
			continue
		}
		for len(stack) > d {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		parent.children = append(parent.children, node)
		stack = append(stack, node)
	}
	return root
}

// buildOutlineForest: each depth-0 line starts a new top-level tree.
func buildOutlineForest(pairs []outlinePair) []*outlineNode {
	var groups [][]outlinePair
	var cur []outlinePair
	for _, p := range pairs {
		if p.depth == 0 && len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
		cur = append(cur, p)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	out := make([]*outlineNode, 0, len(groups))
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		r := &outlineNode{title: g[0].title}
		stack := []*outlineNode{r}
		for i := 1; i < len(g); i++ {
			d, title := g[i].depth, g[i].title
			node := &outlineNode{title: title}
			for len(stack) > d {
				stack = stack[:len(stack)-1]
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, node)
			stack = append(stack, node)
		}
		out = append(out, r)
	}
	return out
}

func outlineNodeToTopics(n *outlineNode) xmind.Topic {
	t := xmind.Topic{
		ID:    uuid.New().String(),
		Title: n.title,
	}
	if len(n.children) == 0 {
		return t
	}
	ch := &xmind.Children{}
	for _, c := range n.children {
		ch.Attached = append(ch.Attached, outlineNodeToTopics(c))
	}
	t.Children = ch
	return t
}

// FindAndReplace updates topic titles. If sheet_id is omitted, all sheets are updated.
func (h *XMindHandler) FindAndReplace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}
	findStr, terr := requireString(args, "find")
	if terr != nil {
		return terr, nil
	}
	replaceStr, rerr := stringArgAllowEmpty(args, "replace")
	if rerr != nil {
		return rerr, nil
	}
	var exact bool
	if raw, has := args["exact_match"]; has && raw != nil {
		v, ok := raw.(bool)
		if !ok {
			return mcp.NewToolResultError("invalid argument exact_match: expected a boolean"), nil
		}
		exact = v
	}

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	// Determine which sheets to update.
	var sheetsToUpdate []*xmind.Sheet
	if raw, has := args["sheet_id"]; has && raw != nil {
		sheetID, ok := raw.(string)
		if !ok || sheetID == "" {
			return mcp.NewToolResultError("invalid argument sheet_id: expected a non-empty string"), nil
		}
		sh := findSheetByID(sheets, sheetID)
		if sh == nil {
			return mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
		}
		sheetsToUpdate = []*xmind.Sheet{sh}
	} else {
		for i := range sheets {
			sheetsToUpdate = append(sheetsToUpdate, &sheets[i])
		}
	}

	type change struct {
		SheetID  string `json:"sheetId,omitempty"`
		ID       string `json:"id"`
		OldTitle string `json:"oldTitle"`
		NewTitle string `json:"newTitle"`
	}
	var changes []change

	var re *regexp.Regexp
	if !exact {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(findStr))
	}

	multiSheet := len(sheetsToUpdate) > 1
	for _, sh := range sheetsToUpdate {
		prevLen := len(changes)
		walkTopics(&sh.RootTopic, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
			old := t.Title
			var newTitle string
			if exact {
				if strings.EqualFold(old, findStr) {
					newTitle = replaceStr
					if newTitle == old {
						return true
					}
				} else {
					return true
				}
			} else {
				newTitle = re.ReplaceAllStringFunc(old, func(string) string { return replaceStr })
				if newTitle == old {
					return true
				}
			}
			c := change{ID: t.ID, OldTitle: old, NewTitle: newTitle}
			if multiSheet {
				c.SheetID = sh.ID
			}
			changes = append(changes, c)
			t.Title = newTitle
			t.TitleUnedited = false
			return true
		})
		if len(changes) > prevLen {
			sh.RevisionID = uuid.New().String()
		}
	}

	resp := struct {
		ChangedCount int      `json:"changedCount"`
		Changes      []change `json:"changes"`
	}{
		ChangedCount: len(changes),
		Changes:      changes,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal find_and_replace response: %w", err)
	}

	if len(changes) > 0 {
		if err := xmind.WriteMap(absPath, sheets); err != nil {
			return nil, fmt.Errorf("write map: %w", err)
		}
	}

	return textResult(string(out)), nil
}

func stringArgAllowEmpty(args map[string]any, key string) (string, *mcp.CallToolResult) {
	v, ok := args[key]
	if !ok {
		return "", mcp.NewToolResultError("missing required argument: " + key)
	}
	s, ok := v.(string)
	if !ok {
		return "", mcp.NewToolResultError("invalid argument " + key + ": expected a string")
	}
	return s, nil
}
