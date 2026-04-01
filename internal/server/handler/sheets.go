package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	"github.com/mark3labs/mcp-go/mcp"
)

// openMapResponse is the JSON shape returned by xmind_open_map.
type openMapResponse struct {
	Path       string         `json:"path"`
	SheetCount int            `json:"sheetCount"`
	Sheets     []openMapSheet `json:"sheets"`
}

type openMapSheet struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	RootTopicTitle string   `json:"rootTopicTitle"`
	TopLevelTopics []string `json:"topLevelTopics"`
	TopicCount     int      `json:"topicCount"`
}

// OpenMap parses a .xmind file and returns a structural summary.
func (h *XMindHandler) OpenMap(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	resp := openMapResponse{
		Path:       absPath,
		SheetCount: len(sheets),
		Sheets:     make([]openMapSheet, 0, len(sheets)),
	}

	for i := range sheets {
		sh := &sheets[i]
		top := topLevelTopicTitles(&sh.RootTopic)
		resp.Sheets = append(resp.Sheets, openMapSheet{
			ID:             sh.ID,
			Title:          sh.Title,
			RootTopicTitle: sh.RootTopic.Title,
			TopLevelTopics: top,
			TopicCount:     countTopics(&sh.RootTopic),
		})
	}

	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal open_map response: %w", err)
	}

	return textResult(string(out)), nil
}

func topLevelTopicTitles(root *xmind.Topic) []string {
	if root == nil || root.Children == nil {
		return nil
	}
	out := make([]string, 0, len(root.Children.Attached))
	for i := range root.Children.Attached {
		out = append(out, root.Children.Attached[i].Title)
	}
	return out
}

type listSheetsResponse struct {
	Path   string           `json:"path"`
	Sheets []listSheetsItem `json:"sheets"`
}

type listSheetsItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ListSheets returns id and title for every sheet in the workbook.
func (h *XMindHandler) ListSheets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	sheets, toolErr2, err := statAndReadMap(absPath)
	if err != nil {
		return nil, err
	}
	if toolErr2 != nil {
		return toolErr2, nil
	}

	resp := listSheetsResponse{
		Path:   absPath,
		Sheets: make([]listSheetsItem, 0, len(sheets)),
	}
	for i := range sheets {
		resp.Sheets = append(resp.Sheets, listSheetsItem{ID: sheets[i].ID, Title: sheets[i].Title})
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal list_sheets response: %w", err)
	}
	return textResult(string(out)), nil
}

// listRelationshipsResponse is the JSON shape returned by xmind_list_relationships.
type listRelationshipsResponse struct {
	SheetID           string                  `json:"sheetId"`
	RelationshipCount int                     `json:"relationshipCount"`
	Relationships     []listRelationshipsItem `json:"relationships"`
}

type listRelationshipsItem struct {
	ID        string `json:"id"`
	End1ID    string `json:"end1Id"`
	End1Title string `json:"end1Title"`
	End2ID    string `json:"end2Id"`
	End2Title string `json:"end2Title"`
	Title     string `json:"title,omitempty"`
}

func listRelationshipsItems(sh *xmind.Sheet) []listRelationshipsItem {
	rels := make([]listRelationshipsItem, 0, len(sh.Relationships))
	for i := range sh.Relationships {
		rel := &sh.Relationships[i]
		item := listRelationshipItemFrom(sh, rel)
		rels = append(rels, item)
	}
	return rels
}

func listRelationshipItemFrom(sh *xmind.Sheet, rel *xmind.Relationship) listRelationshipsItem {
	item := listRelationshipsItem{
		ID:     rel.ID,
		End1ID: rel.End1ID,
		End2ID: rel.End2ID,
	}
	if t1 := findTopicByID(&sh.RootTopic, rel.End1ID); t1 != nil {
		item.End1Title = t1.Title
	}
	if t2 := findTopicByID(&sh.RootTopic, rel.End2ID); t2 != nil {
		item.End2Title = t2.Title
	}
	if rel.Title != "" {
		item.Title = rel.Title
	}
	return item
}

// ListRelationships returns all relationships on a sheet with endpoint topic titles.
func (h *XMindHandler) ListRelationships(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	rels := listRelationshipsItems(sh)

	resp := listRelationshipsResponse{
		SheetID:           sh.ID,
		RelationshipCount: len(rels),
		Relationships:     rels,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal list_relationships response: %w", err)
	}
	return textResult(string(out)), nil
}

// bumpAllSheetsRevisionID assigns a new UUID v4 revisionId to every sheet in the slice (in place).
func bumpAllSheetsRevisionID(sheets []xmind.Sheet) {
	for i := range sheets {
		sheets[i].RevisionID = uuid.New().String()
	}
}

func newSheet(sheetTitle, rootTitle string) xmind.Sheet {
	// org.xmind.ui.map.unbalanced matches what the XMind app writes for new maps.
	// Do NOT use org.xmind.ui.map.clockwise — that forces a one-directional layout.
	const structureClass = "org.xmind.ui.map.unbalanced"
	return xmind.Sheet{
		ID:               uuid.New().String(),
		RevisionID:       uuid.New().String(),
		Class:            "sheet",
		Title:            sheetTitle,
		TopicOverlapping: "overlap",
		RootTopic: xmind.Topic{
			ID:             uuid.New().String(),
			Class:          "topic",
			Title:          rootTitle,
			StructureClass: structureClass,
		},
		Theme:      xmind.DefaultTheme,
		Extensions: xmind.DefaultSheetExtensions(structureClass),
	}
}

// CreateMap writes a new workbook with a single sheet.
func (h *XMindHandler) CreateMap(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	rawRoot, ok := args["root_title"]
	if !ok {
		return mcp.NewToolResultError("missing required argument: root_title"), nil
	}
	rootTitle, ok := rawRoot.(string)
	if !ok || rootTitle == "" {
		return mcp.NewToolResultError("invalid argument root_title: expected a non-empty string"), nil
	}

	sheetTitle := "Sheet 1"
	if v, ok := args["sheet_title"].(string); ok && v != "" {
		sheetTitle = v
	}

	if _, err := os.Stat(absPath); err == nil {
		return mcp.NewToolResultError(fmt.Sprintf("file already exists: %s", absPath)), nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	sh := newSheet(sheetTitle, rootTitle)
	if err := xmind.CreateNewMap(absPath, []xmind.Sheet{sh}); err != nil {
		return nil, fmt.Errorf("create map: %w", err)
	}

	msg := fmt.Sprintf("created map at %s with sheet id %s", absPath, sh.ID)
	return textResult(msg), nil
}

// AddSheet appends a sheet to an existing workbook.
func (h *XMindHandler) AddSheet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	sheetTitle, terr := requireString(args, "title")
	if terr != nil {
		return terr, nil
	}
	rootTitle, terr := requireString(args, "root_title")
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

	bumpAllSheetsRevisionID(sheets)
	sh := newSheet(sheetTitle, rootTitle)
	sheets = append(sheets, sh)
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}

	return textResult(fmt.Sprintf("added sheet id %s", sh.ID)), nil
}

// DeleteSheet removes one sheet; the last sheet cannot be deleted.
func (h *XMindHandler) DeleteSheet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	args := req.GetArguments()
	absPath, toolErr := absPathFromArgs(args)
	if toolErr != nil {
		return toolErr, nil
	}

	rawID, ok := args["sheet_id"]
	if !ok {
		return mcp.NewToolResultError("missing required argument: sheet_id"), nil
	}
	sheetID, ok := rawID.(string)
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

	if len(sheets) < 2 {
		return mcp.NewToolResultError("cannot delete the last sheet in a workbook"), nil
	}

	idx := slices.IndexFunc(sheets, func(s xmind.Sheet) bool { return s.ID == sheetID })
	if idx < 0 {
		return mcp.NewToolResultError(fmt.Sprintf("sheet not found: %s", sheetID)), nil
	}

	sheets = slices.Delete(sheets, idx, idx+1)
	bumpAllSheetsRevisionID(sheets)
	if err := xmind.WriteMap(absPath, sheets); err != nil {
		return nil, fmt.Errorf("write map: %w", err)
	}

	return textResult(fmt.Sprintf("deleted sheet id %s", sheetID)), nil
}
