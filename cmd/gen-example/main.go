// gen-example generates the example mind map used for the
// xmind-mcp README screenshot. It calls the xmind package directly — no MCP,
// no LLM — so the file can be regenerated at any time with:
//
//	go run ./cmd/gen-example [output-path]
//
// If no output path is given it defaults to ./example.xmind.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"
)

const structureClass = "org.xmind.ui.map.unbalanced"

func main() {
	outPath := "example.xmind"
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}

	sheets := []xmind.Sheet{buildSheet()}
	if err := xmind.CreateNewMap(outPath, sheets); err != nil {
		log.Fatalf("create map: %v", err)
	}
	fmt.Printf("wrote %s\n", outPath)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func id() string { return uuid.New().String() }

func topic(title string, children ...xmind.Topic) xmind.Topic {
	t := xmind.Topic{ID: id(), Title: title}
	if len(children) > 0 {
		t.Children = &xmind.Children{Attached: children}
	}
	return t
}

func leaf(title string) xmind.Topic {
	return xmind.Topic{ID: id(), Title: title}
}

func withMarkers(t xmind.Topic, ids ...string) xmind.Topic {
	for _, m := range ids {
		t.Markers = append(t.Markers, xmind.Marker{MarkerID: m})
	}
	return t
}

func withLabels(t xmind.Topic, labels ...string) xmind.Topic {
	t.Labels = append(t.Labels, labels...)
	return t
}

func withNote(t xmind.Topic, note string) xmind.Topic {
	t.Notes = &xmind.Notes{
		Plain:    &xmind.NoteContent{Content: note},
		RealHTML: &xmind.NoteContent{Content: "<div>" + note + "</div>"},
	}
	return t
}

func withLink(t xmind.Topic, url string) xmind.Topic {
	t.Href = url
	return t
}

func withTaskStatus(t xmind.Topic, status string) xmind.Topic {
	t.Extensions = append(t.Extensions, xmind.Extension{
		Provider: "org.xmind.ui.task",
		Content:  map[string]any{"status": status},
	})
	return t
}

// ── map construction ─────────────────────────────────────────────────────────

func buildSheet() xmind.Sheet {

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 1: Tools
	//
	// Each tier gets a handful of representative tool names as leaves,
	// with the full list in the note. Enough to look populated without
	// listing all 22 individually.
	// ═══════════════════════════════════════════════════════════════════════

	tier1 := topic("Tier 1 — File & Sheet Mgmt",
		withMarkers(leaf("open_map"), "priority-1"),
		withMarkers(leaf("create_map"), "priority-1"),
		withMarkers(leaf("add_sheet"), "priority-1"),
	)
	tier1 = withNote(tier1, "5 tools. Also: list_sheets, delete_sheet. Workbook-level operations — typically the first calls in any agentic workflow.")

	tier2 := topic("Tier 2 — Finding Topics",
		withMarkers(leaf("get_subtree"), "priority-2"),
		withMarkers(leaf("search_topics"), "priority-2"),
		withMarkers(leaf("find_topic"), "priority-2"),
	)
	tier2 = withNote(tier2, "Entry point for any write operation. Always call one of these first to obtain a topic ID.")

	tier3 := topic("Tier 3 — Topic Mutations",
		withMarkers(leaf("add_topic"), "priority-3"),
		withMarkers(leaf("rename_topic"), "priority-3"),
		withMarkers(leaf("delete_topic"), "priority-3"),
		withMarkers(leaf("move_topic"), "priority-3"),
		withMarkers(leaf("set_topic_properties"), "priority-3"),
		withMarkers(leaf("add_relationship"), "priority-3"),
		withMarkers(leaf("add_summary"), "priority-3"),
		withMarkers(leaf("add_boundary"), "priority-3"),
	)
	tier3 = withNote(tier3, "11 tools total. Also: add_topics_bulk, reorder_children, add_floating_topic. All require a topic_id from a Tier 2 call.")

	tier4 := topic("Tier 4 — Utilities",
		withMarkers(leaf("flatten_to_outline"), "priority-4"),
		withMarkers(leaf("import_from_outline"), "priority-4"),
		withMarkers(leaf("find_and_replace"), "priority-4"),
	)
	tier4 = withNote(tier4, "Export to text/markdown, import from outlines, bulk rename across a sheet.")

	tools := topic("🛠️ 22 Tools", tier1, tier2, tier3, tier4)
	tools = withNote(tools, "All prefixed with xmind_ to avoid collisions in multi-server MCP environments.")

	// Summary across all four tiers.
	toolsSummaryID := id()
	tools.Children.Summary = []xmind.Topic{{ID: toolsSummaryID, Title: "find first, then act"}}
	tools.Summaries = []xmind.Summary{{
		ID:      id(),
		Range:   "(0,3)",
		TopicID: toolsSummaryID,
	}}

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 2: How It Works
	//
	// Architecture (conceptual data flow), the read/write lifecycle, and
	// schema gotchas — one branch telling the whole design story.
	// ═══════════════════════════════════════════════════════════════════════

	// -- Architecture: the conceptual layers --

	typesNode := withNote(
		withLabels(leaf("content.json → Go structs"), "xmind/types.go"),
		"Custom UnmarshalJSON/MarshalJSON on every type. Unknown JSON fields are captured into an 'extra' map and merged back on marshal — future XMind fields survive round-trips without data loss.",
	)

	zipNode := withNote(
		withLabels(leaf("ZIP read/write layer"), "xmind/reader.go, writer.go"),
		"Reader opens .xmind zip, extracts content.json, unmarshals into []Sheet. Writer serializes sheets, writes to temp file, then atomic os.Rename swap. Non-content entries (images, XML stub) are copied byte-for-byte via OpenRaw/CreateRaw.",
	)

	handlersNode := withNote(
		withLabels(leaf("22 tool handlers"), "handler/*.go"),
		"Each handler: parse args → open file → find sheet → find topic → apply change → write file → return result. Stateless — no session, no in-memory cache between calls.",
	)

	mcpNode := withNote(
		withLabels(leaf("MCP server + stdio transport"), "server/server.go"),
		"Tool registration and stdio transport via mark3labs/mcp-go. Lifecycle hooks for logging. All tool names prefixed xmind_ for multi-server safety.",
	)

	arch := topic("Architecture",
		typesNode, zipNode, handlersNode, mcpNode,
	)
	arch = withMarkers(arch, "star-blue")

	// -- Mutation lifecycle --

	step1 := withMarkers(withTaskStatus(leaf("Open .xmind zip"), "done"), "task-start")
	step2 := withMarkers(withTaskStatus(leaf("Parse content.json"), "done"), "task-3oct")
	step3 := withMarkers(withTaskStatus(leaf("Mutate in memory"), "done"), "task-half")
	step4 := withMarkers(withTaskStatus(leaf("Write temp → atomic swap"), "done"), "task-7oct")
	step5 := withMarkers(withTaskStatus(leaf("Return result"), "done"), "task-done")

	lifecycle := topic("Mutation Lifecycle", step1, step2, step3, step4, step5)
	lifecycle = withNote(lifecycle, "Every mutating tool call follows this exact cycle. No session state, no temp files left behind. If a write fails mid-way, the original file is untouched.")

	// -- Gotchas --

	summaryGotcha := withNote(
		withMarkers(leaf("Summaries need a double-write"), "flag-red"),
		"Must write to BOTH children.summary (the topic) AND parent.summaries (the range descriptor). Omitting either produces a broken map in XMind.",
	)

	relGotcha := withNote(
		withMarkers(leaf("Relationships live on the sheet"), "flag-red"),
		"Sheet.Relationships[], NOT on any topic. Writing to a topic appears to work but won't render.",
	)

	errorGotcha := withNote(
		withMarkers(leaf("Tool errors ≠ protocol errors"), "flag-orange"),
		"Expected failures (not found, bad args) → return (*CallToolResult, nil) with IsError. Unexpected failures (I/O, marshal) → return (nil, error). Conflating these makes failures look like crashes to the model.",
	)

	preserveGotcha := withNote(
		withMarkers(leaf("Preserve unknown JSON fields"), "flag-yellow"),
		"json_codec.go captures unknown keys into an 'extra' map and merges them back on marshal. Prevents silent data loss when XMind ships new features.",
	)

	gotchas := topic("Gotchas", summaryGotcha, relGotcha, errorGotcha, preserveGotcha)

	// Boundary around the gotchas.
	gotchas.Boundaries = []xmind.Boundary{{
		ID:    id(),
		Range: "master",
		Title: "Here be dragons",
	}}

	design := topic("⚙️ How It Works", arch, lifecycle, gotchas)
	design = withLink(design, "https://github.com/mab-go/xmind-mcp")

	// Relationship: lifecycle's atomic swap ↔ error handling gotcha
	rel := xmind.Relationship{
		ID:     id(),
		End1ID: step4.ID,
		End2ID: errorGotcha.ID,
		Title:  "both protect file integrity",
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 3: What It Supports
	//
	// The XMind features the server can read and write — this is what a
	// user actually wants to know: "what can I do with this?"
	// ═══════════════════════════════════════════════════════════════════════

	topicFeatures := topic("Topic Markup",
		withMarkers(leaf("Notes (plain + HTML)"), "c_symbol_pen"),
		withMarkers(leaf("Labels"), "tag-blue"),
		withMarkers(leaf("Markers (7 categories)"), "star-yellow"),
		withMarkers(leaf("Links (web, file, cross-topic)"), "symbol-lightning"),
		withMarkers(leaf("Task checkboxes"), "task-done"),
	)

	sheetFeatures := topic("Sheet Features",
		leaf("Relationships between any two topics"),
		leaf("Summaries spanning sibling ranges"),
		leaf("Boundaries grouping children"),
		leaf("Floating topics (detached)"),
	)

	mapFeatures := topic("Map Features",
		leaf("All 9 structure types"),
		leaf("Multi-sheet workbooks"),
		leaf("Outline import/export"),
		withNote(leaf("Round-trip fidelity"), "Unknown fields, themes, extensions, and binary resources are preserved through read/write cycles."),
	)

	supports := topic("📄 What It Supports", topicFeatures, sheetFeatures, mapFeatures)
	supports = withNote(supports, "The server reads and writes .xmind files — ZIP archives containing content.json, metadata.json, a content.xml stub, and optional resources/ for images and attachments.")

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 4: Project Info (lightweight)
	// ═══════════════════════════════════════════════════════════════════════

	meta := topic("📋 Project",
		withLink(withLabels(leaf("github.com/mab-go/xmind-mcp"), "source"), "https://github.com/mab-go/xmind-mcp"),
		withLabels(leaf("MIT License"), "open source"),
		withLabels(leaf("Go 1.26.1+"), "prerequisite"),
		withLink(withLabels(leaf("mark3labs/mcp-go"), "MCP protocol"), "https://github.com/mark3labs/mcp-go"),
	)

	// ═══════════════════════════════════════════════════════════════════════
	// Root
	// ═══════════════════════════════════════════════════════════════════════

	root := xmind.Topic{
		ID:             id(),
		Class:          "topic",
		Title:          "xmind-mcp",
		StructureClass: structureClass,
		Children: &xmind.Children{
			Attached: []xmind.Topic{tools, design, supports, meta},
		},
	}
	root = withNote(root, "An MCP server for reading and writing local XMind mind map files. 22 tools let any MCP-compatible AI client create, navigate, and edit .xmind files directly on disk.")

	return xmind.Sheet{
		ID:               id(),
		RevisionID:       id(),
		Class:            "sheet",
		Title:            "xmind-mcp",
		TopicOverlapping: "overlap",
		RootTopic:        root,
		Relationships:    []xmind.Relationship{rel},
		Theme:            xmind.DefaultTheme,
		Extensions:       xmind.DefaultSheetExtensions(structureClass),
	}
}
