package server

import "github.com/mark3labs/mcp-go/mcp"

var toolOpenMap = mcp.NewTool(
	"xmind_open_map",
	mcp.WithDescription(
		"Parse a .xmind file and return a structural summary of its sheets, "+
			"root topics, and node counts. Typically the first call in any workflow.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
)

var toolListSheets = mcp.NewTool(
	"xmind_list_sheets",
	mcp.WithDescription("List all sheets in a workbook with id and title."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
)

var toolCreateMap = mcp.NewTool(
	"xmind_create_map",
	mcp.WithDescription("Create a new .xmind file with one sheet and a root topic."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path where the new file will be written")),
	mcp.WithString("root_title", mcp.Required(), mcp.Description("Title for the root/central topic")),
	mcp.WithString("sheet_title", mcp.Description(`Title for the first sheet (default "Sheet 1")`)),
)

var toolAddSheet = mcp.NewTool(
	"xmind_add_sheet",
	mcp.WithDescription("Add a new sheet to an existing workbook."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("title", mcp.Required(), mcp.Description("Title of the new sheet")),
	mcp.WithString("root_title", mcp.Required(), mcp.Description("Title for the sheet's root topic")),
)

var toolDeleteSheet = mcp.NewTool(
	"xmind_delete_sheet",
	mcp.WithDescription("Remove a sheet from a workbook. At least one sheet must remain."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("ID of the sheet to delete")),
)

var toolGetSubtree = mcp.NewTool(
	"xmind_get_subtree",
	mcp.WithDescription(
		"Return the topic hierarchy from a topic (or the sheet root). Each node includes id, title, labels, markers, "+
			"and structureClass when the topic overrides sheet layout. Optional include_notes adds plain-text notes; "+
			"include_links adds hyperlink href. Optional depth limits nesting; truncated branches report childrenCount.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Sheet to read from")),
	mcp.WithString("topic_id", mcp.Description("Root of the subtree; omit for sheet root")),
	mcp.WithNumber("depth", mcp.Description("Max depth from subtree root (0 = root only with counts); omit for full tree; must be a whole number")),
	mcp.WithBoolean("include_notes", mcp.Description("If true, include plain-text notes (notes.plain content) on nodes that have note text; omitted when empty")),
	mcp.WithBoolean("include_links", mcp.Description("If true, include hyperlink href on nodes that have a link; omitted when empty")),
)

var toolGetTopicProperties = mcp.NewTool(
	"xmind_get_topic_properties",
	mcp.WithDescription(
		"Return JSON for a single topic: id, title, structureClass, labels, markers, plain-text notes, hyperlink href, "+
			"image source, position (floating topics), boundaries (with range and title), summaryCount (range descriptors on this topic), "+
			"relationships on the sheet that reference this topic (end1Id/end2Id; sheet-level storage), and childCounts as direct child counts "+
			"by list (attached, detached, summary — not the same as summaryCount). Use xmind_get_subtree for the full hierarchy. "+
			"Use after writes to verify what was stored.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Sheet containing the topic")),
	mcp.WithString("topic_id", mcp.Required(), mcp.Description("ID of the topic to inspect")),
)

var toolSearchTopics = mcp.NewTool(
	"xmind_search_topics",
	mcp.WithDescription(
		"Search topics by keyword (case-insensitive substring). "+
			"Omit sheet_id to search all sheets; results will include sheetId and sheetTitle fields.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Description("Sheet to search; omit or null to search all sheets")),
	mcp.WithString("query", mcp.Required(), mcp.Description("Keyword or phrase to search for")),
)

var toolFindTopic = mcp.NewTool(
	"xmind_find_topic",
	mcp.WithDescription(
		"Find a topic by exact title (case-sensitive). Returns the first depth-first match within the chosen scope. "+
			"Optional parent_id is the subtree root to search under (same role as topic_id on xmind_get_subtree); "+
			"it is not the structural parent used by xmind_add_topic and other write tools. "+
			"Omit parent_id or null for the whole sheet. The scope root is visited first, so it can match if its title equals the search title; "+
			"parentTitle and siblingTitles are relative to that walk, so they are empty when the match is the scope root. "+
			"Use xmind_search_topics for substring or cross-sheet search.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("title", mcp.Required(), mcp.Description("Exact topic title to find (non-empty)")),
	mcp.WithString("parent_id", mcp.Description(
		"Topic ID for the subtree root to search (omit or null for the entire sheet). "+
			"Unlike parent_id on add/import tools, this does not designate a parent for new topics.",
	)),
)

var toolAddTopic = mcp.NewTool(
	"xmind_add_topic",
	mcp.WithDescription("Add a new attached child topic under a parent topic. Returns JSON: {\"id\":\"…\",\"position\":N,\"siblingCount\":N}."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("parent_id", mcp.Required(), mcp.Description("ID of the parent topic")),
	mcp.WithString("title", mcp.Required(), mcp.Description("Title of the new topic")),
	mcp.WithNumber("position", mcp.Description("Sibling index to insert at; omit to append")),
)

var toolAddTopicsBulk = mcp.NewTool(
	"xmind_add_topics_bulk",
	mcp.WithDescription("Add multiple topics under a parent in one call; each item may nest children. Returns JSON: {\"addedCount\":N,\"parentId\":\"…\",\"firstPosition\":N,\"siblingCount\":N,\"rootTopicIds\":[\"…\"]}."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("parent_id", mcp.Required(), mcp.Description("ID of the parent topic")),
	mcp.WithArray("topics", mcp.Required(), mcp.Description(`Array of {title, children?} objects`)),
)

var toolDuplicateTopic = mcp.NewTool(
	"xmind_duplicate_topic",
	mcp.WithDescription(
		"Deep-clone a topic and its subtree (same sheet only) and attach the copy as an attached child of target_parent_id. "+
			"Sheet-level relationships are not copied. Hyperlinks or note text that reference other topic IDs by ID are not rewritten and may still point at the original topics. "+
			`Returns JSON: {"sourceId":"…","newRootId":"…","parentId":"…","copiedCount":N,"position":N,"siblingCount":N}.`,
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Sheet containing the source topic")),
	mcp.WithString("topic_id", mcp.Required(), mcp.Description("Root topic of the subtree to duplicate")),
	mcp.WithString("target_parent_id", mcp.Required(), mcp.Description("Parent topic to attach the copy under (attached children)")),
	mcp.WithNumber("position", mcp.Description("Zero-based index among the parent's attached children; omit to append")),
)

var toolRenameTopic = mcp.NewTool(
	"xmind_rename_topic",
	mcp.WithDescription("Rename an existing topic."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("topic_id", mcp.Required(), mcp.Description("ID of the topic to rename")),
	mcp.WithString("title", mcp.Required(), mcp.Description("New title")),
)

var toolDeleteTopic = mcp.NewTool(
	"xmind_delete_topic",
	mcp.WithDescription("Delete a topic and its descendants. Cannot delete the sheet root."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("topic_id", mcp.Required(), mcp.Description("ID of the topic to delete")),
)

var toolMoveTopic = mcp.NewTool(
	"xmind_move_topic",
	mcp.WithDescription("Move a topic (and subtree) to a new parent as an attached child. Use position to control insertion order; omit to append at the end. Returns JSON: {\"topicId\":\"…\",\"parentId\":\"…\",\"position\":N,\"siblingCount\":N}."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("topic_id", mcp.Required(), mcp.Description("ID of the topic to move")),
	mcp.WithString("new_parent_id", mcp.Required(), mcp.Description("ID of the new parent topic")),
	mcp.WithNumber("position", mcp.Description("Zero-based insertion index among the new parent's attached children (0 = first child); valid range is 0 to the current child count. Omit to append at end.")),
)

var toolReorderChildren = mcp.NewTool(
	"xmind_reorder_children",
	mcp.WithDescription("Reorder a topic's attached children; ordered_ids must list every child exactly once."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("parent_id", mcp.Required(), mcp.Description("ID of the parent whose children to reorder")),
	mcp.WithArray("ordered_ids", mcp.Required(), mcp.Description("Attached child topic IDs in the desired order")),
)

var toolSetTopicProperties = mcp.NewTool(
	"xmind_set_topic_properties",
	mcp.WithDescription(
		"Set optional metadata on a topic: notes, labels, markers, link, remove_markers. Only provided fields are updated. "+
			"Clearing cheat sheet: notes use empty string or null to clear; link uses empty string to clear (omit the key or pass null to leave the link unchanged). "+
			"labels or markers use an empty array to clear all. remove_markers is applied after markers when both are set; empty remove_markers removes nothing, "+
			"unlike an empty markers array which clears all markers. "+
			"Null semantics: only notes treats JSON null as clear; for labels, markers, remove_markers, and link, omit the key or pass null to leave that field unchanged.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("topic_id", mcp.Required(), mcp.Description("ID of the topic to update")),
	mcp.WithString("notes", mcp.Description("Plain text note (plain + HTML fields set); empty string or null clears notes (only this field uses null to clear)")),
	mcp.WithArray("labels", mcp.Description("List of label strings; empty array clears all labels; omit or null leaves labels unchanged")),
	mcp.WithArray("markers", mcp.Description(`Full marker ID list, e.g. "priority-1", "task-done"; empty array clears all markers; omit or null leaves markers unchanged`)),
	mcp.WithArray("remove_markers", mcp.Description(`Marker IDs to remove after any markers replace; empty array removes nothing; omit or null leaves markers unchanged; applied after markers when both are set`)),
	mcp.WithString("link", mcp.Description("URL, file path, or topic link href; empty string clears the link; omit or null leaves the link unchanged")),
)

var toolSetTopicPropertiesBulk = mcp.NewTool(
	"xmind_set_topic_properties_bulk",
	mcp.WithDescription(
		"Set optional metadata on multiple topics in one read/write: same fields and clearing rules as xmind_set_topic_properties (notes, labels, markers, link, remove_markers). "+
			"topic_ids must be non-empty with no duplicate IDs; every ID must exist on the sheet. "+
			"At least one property argument is required. remove_markers is applied after markers when both are set.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithArray("topic_ids", mcp.Required(), mcp.Description("Topic IDs to update (non-empty strings, no duplicates)")),
	mcp.WithString("notes", mcp.Description("Plain text note (plain + HTML fields set); empty string or null clears notes (only this field uses null to clear)")),
	mcp.WithArray("labels", mcp.Description("List of label strings; empty array clears all labels; omit or null leaves labels unchanged")),
	mcp.WithArray("markers", mcp.Description(`Full marker ID list, e.g. "priority-1", "task-done"; empty array clears all markers; omit or null leaves markers unchanged`)),
	mcp.WithArray("remove_markers", mcp.Description(`Marker IDs to remove after any markers replace; empty array removes nothing; omit or null leaves markers unchanged; applied after markers when both are set`)),
	mcp.WithString("link", mcp.Description("URL, file path, or topic link href; empty string clears the link; omit or null leaves the link unchanged")),
)

var toolAddFloatingTopic = mcp.NewTool(
	"xmind_add_floating_topic",
	mcp.WithDescription("Add a floating (detached) topic on the sheet, positioned under the root topic."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("title", mcp.Required(), mcp.Description("Title of the floating topic")),
)

var toolAddRelationship = mcp.NewTool(
	"xmind_add_relationship",
	mcp.WithDescription("Add a relationship (connector) between two topics. Stored at sheet level, not on topics."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("from_id", mcp.Required(), mcp.Description("Source topic ID (end1Id)")),
	mcp.WithString("to_id", mcp.Required(), mcp.Description("Target topic ID (end2Id)")),
	mcp.WithString("label", mcp.Description("Optional label on the connector")),
)

var toolListRelationships = mcp.NewTool(
	"xmind_list_relationships",
	mcp.WithDescription(
		"List all sheet-level relationships as JSON: sheetId, relationshipCount, and relationships (endpoint ids, "+
			"titles, optional connector title).",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description(
		"Sheet to read. Each item's end1Id/end2Id correspond to from_id/to_id on xmind_add_relationship; optional title matches the add tool's label.",
	)),
)

var toolDeleteRelationship = mcp.NewTool(
	"xmind_delete_relationship",
	mcp.WithDescription("Remove a sheet-level relationship by id (from xmind_list_relationships)."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Sheet containing the relationship")),
	mcp.WithString("relationship_id", mcp.Required(), mcp.Description("ID of the relationship to delete")),
)

var toolAddSummary = mcp.NewTool(
	"xmind_add_summary",
	mcp.WithDescription("Add a summary callout spanning a range of sibling attached children (double-write to children.summary and summaries)."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("parent_id", mcp.Required(), mcp.Description("Parent topic whose attached children are summarized")),
	mcp.WithNumber("from_index", mcp.Required(), mcp.Description("Zero-based index of first sibling (inclusive)")),
	mcp.WithNumber("to_index", mcp.Required(), mcp.Description("Zero-based index of last sibling (inclusive)")),
	mcp.WithString("title", mcp.Description("Optional summary topic title")),
)

var toolAddBoundary = mcp.NewTool(
	"xmind_add_boundary",
	mcp.WithDescription("Add a boundary grouping all attached children of a parent topic."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Target sheet")),
	mcp.WithString("parent_id", mcp.Required(), mcp.Description("Parent topic whose children are enclosed")),
	mcp.WithString("title", mcp.Description("Optional boundary label")),
)

var toolFlattenToOutline = mcp.NewTool(
	"xmind_flatten_to_outline",
	mcp.WithDescription(
		"Export a topic subtree as indented plain text or Markdown (attached children only). "+
			"Optional include_notes appends plain-text note lines below each topic: Markdown uses blockquotes (>), "+
			"plain text uses indented [note] lines.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Required(), mcp.Description("Sheet to read from")),
	mcp.WithString("topic_id", mcp.Description("Root of subtree; omit for sheet root")),
	mcp.WithString("format", mcp.Description(`Output format: "text" or "markdown" (default "markdown")`)),
	mcp.WithBoolean("include_notes", mcp.Description("If true, append each topic's plain note lines below its title line")),
)

var toolImportFromOutline = mcp.NewTool(
	"xmind_import_from_outline",
	mcp.WithDescription("Parse indented text or Markdown outline and attach topics to a sheet or parent. Omit sheet_id to create a new sheet; parent_id requires sheet_id."),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("outline", mcp.Required(), mcp.Description("Outline text (Markdown headings, list, or indented plain text)")),
	mcp.WithString("sheet_id", mcp.Description("Target sheet; omit to append a new sheet")),
	mcp.WithString("parent_id", mcp.Description("Parent topic; omit to use sheet root (only valid when sheet_id is set)")),
)

var toolFindAndReplace = mcp.NewTool(
	"xmind_find_and_replace",
	mcp.WithDescription(
		"Replace text in topic titles (case-insensitive substring or exact title match). "+
			"Omit sheet_id to apply across all sheets.",
	),
	mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the .xmind file")),
	mcp.WithString("sheet_id", mcp.Description("Target sheet; omit or null to apply to all sheets")),
	mcp.WithString("find", mcp.Required(), mcp.Description("Text to find")),
	mcp.WithString("replace", mcp.Required(), mcp.Description("Replacement text")),
	mcp.WithBoolean("exact_match", mcp.Description("If true, replace only when title equals find (case-insensitive)")),
)
