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

// ── map construction ─────────────────────────────────────────────────────────

func buildSheet() xmind.Sheet {

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 1: Tools
	//
	// All 27 tools, organized into four tiers with priority markers.
	// Every tool name is visible — this is the product.
	// ═══════════════════════════════════════════════════════════════════════

	tier1 := topic("Tier 1 — File & Sheet Mgmt",
		withMarkers(leaf("open_map"), "priority-1"),
		withMarkers(leaf("create_map"), "priority-1"),
		withMarkers(leaf("list_sheets"), "priority-1"),
		withMarkers(leaf("add_sheet"), "priority-1"),
		withMarkers(leaf("delete_sheet"), "priority-1"),
		withMarkers(leaf("list_relationships"), "priority-1"),
	)
	tier1 = withNote(tier1, "6 tools. Workbook-level operations — typically the first calls in any agentic workflow.")

	tier2 := topic("Tier 2 — Finding Topics",
		withMarkers(leaf("get_subtree"), "priority-2"),
		withMarkers(leaf("get_topic_properties"), "priority-2"),
		withMarkers(leaf("search_topics"), "priority-2"),
		withMarkers(leaf("find_topic"), "priority-2"),
	)
	tier2 = withNote(tier2, "Entry point for any write operation. Always call one of these first to obtain a topic ID.")

	tier3 := topic("Tier 3 — Topic Mutations",
		withMarkers(leaf("add_topic"), "priority-3"),
		withMarkers(leaf("add_topics_bulk"), "priority-3"),
		withMarkers(leaf("duplicate_topic"), "priority-3"),
		withMarkers(leaf("rename_topic"), "priority-3"),
		withMarkers(leaf("delete_topic"), "priority-3"),
		withMarkers(leaf("move_topic"), "priority-3"),
		withMarkers(leaf("reorder_children"), "priority-3"),
		withMarkers(leaf("set_topic_properties"), "priority-3"),
		withMarkers(leaf("set_topic_properties_bulk"), "priority-3"),
		withMarkers(leaf("add_floating_topic"), "priority-3"),
		withMarkers(leaf("add_relationship"), "priority-3"),
		withMarkers(leaf("delete_relationship"), "priority-3"),
		withMarkers(leaf("add_summary"), "priority-3"),
		withMarkers(leaf("add_boundary"), "priority-3"),
	)
	tier3 = withNote(tier3, "14 tools. All require an ID from a Tier 2 call.")

	tier4 := topic("Tier 4 — Utilities",
		withMarkers(leaf("flatten_to_outline"), "priority-4"),
		withMarkers(leaf("import_from_outline"), "priority-4"),
		withMarkers(leaf("find_and_replace"), "priority-4"),
	)
	tier4 = withNote(tier4, "Export to text/markdown, import from outlines, bulk rename across a sheet.")

	tools := topic("🛠️ 27 Tools", tier1, tier2, tier3, tier4)
	tools = withNote(tools, "All prefixed with xmind_ to avoid collisions in multi-server MCP environments.")

	// Summary across all four tiers.
	toolsSummaryID := id()
	tools.Children.Summary = []xmind.Topic{{ID: toolsSummaryID, Title: "Find → Then Act"}}
	tools.Summaries = []xmind.Summary{{
		ID:      id(),
		Range:   "(0,3)",
		TopicID: toolsSummaryID,
	}}

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 2: Design
	//
	// The engineering principles that make the project trustworthy.
	// Tight — four leaves with a boundary. Short titles, details in notes.
	// ═══════════════════════════════════════════════════════════════════════

	stateless := withNote(
		withMarkers(leaf("Stateless"), "star-blue"),
		"No session state, no in-memory cache between calls. Every tool opens the file, applies changes, and writes it back.",
	)

	atomicSwap := withNote(
		withMarkers(leaf("Atomic File Swap"), "star-blue"),
		"Writes to a temp file first, then os.Rename. If anything fails mid-write, the original .xmind is untouched.",
	)

	preserveUnknown := withNote(
		withMarkers(leaf("Unknown-Field Preservation"), "star-blue"),
		"Custom JSON codec captures unknown keys into an extra map and merges them back on marshal. Future XMind fields survive round-trips.",
	)

	roundTrip := withNote(
		withMarkers(leaf("Round-Trip Fidelity"), "star-blue"),
		"Themes, extensions, binary resources, and non-content ZIP entries are preserved byte-for-byte through read/write cycles.",
	)

	design := topic("⚙️ Design", stateless, atomicSwap, preserveUnknown, roundTrip)

	// Boundary around all design principles.
	design.Boundaries = []xmind.Boundary{{
		ID:    id(),
		Range: "master",
		Title: "Zero Data Loss",
	}}

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 3: XMind Features
	//
	// What the server can read and write. Each leaf is decorated with the
	// XMind feature it describes — the screenshot IS the proof.
	// ═══════════════════════════════════════════════════════════════════════

	topicMarkup := topic("Topic Markup",
		withMarkers(leaf("Notes (plain + HTML)"), "c_symbol_pen"),
		withMarkers(leaf("Labels"), "tag-blue"),
		withMarkers(leaf("Markers (7 categories)"), "star-yellow"),
		withMarkers(leaf("Links (web, file, cross-topic)"), "symbol-lightning"),
		withMarkers(leaf("Task checkboxes"), "task-done"),
	)

	sheetConstructs := topic("Sheet Constructs",
		leaf("Relationships between any two topics"),
		leaf("Summaries spanning sibling ranges"),
		leaf("Boundaries grouping children"),
		leaf("Floating topics (detached)"),
	)

	mapCapabilities := topic("Map Capabilities",
		leaf("All 9 structure types"),
		leaf("Multi-sheet workbooks"),
		leaf("Outline import / export"),
	)

	features := topic("📄 XMind Features", topicMarkup, sheetConstructs, mapCapabilities)
	features = withNote(features, "Reads and writes .xmind files — ZIP archives containing content.json and optional resources.")

	// ═══════════════════════════════════════════════════════════════════════
	// Branch 4: Project
	//
	// Lightweight metadata. Labels and links give visual variety.
	// The relationship between the repo and mcp-go shows a dependency.
	// ═══════════════════════════════════════════════════════════════════════

	repoLeaf := withLink(withLabels(leaf("github.com/mab-go/xmind-mcp"), "source"), "https://github.com/mab-go/xmind-mcp")
	mcpGoLeaf := withLink(withLabels(leaf("mark3labs/mcp-go"), "MCP protocol"), "https://github.com/mark3labs/mcp-go")

	project := topic("📋 Project",
		repoLeaf,
		withLabels(leaf("MIT License"), "open source"),
		withLabels(leaf("Go 1.26.1+"), "prerequisite"),
		mcpGoLeaf,
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
			Attached: []xmind.Topic{tools, design, features, project},
		},
	}
	root = withNote(root, "An MCP server for reading and writing local XMind mind map files. 27 tools let any MCP-compatible AI client create, navigate, and edit .xmind files directly on disk.")

	// ─── Relationship: repo depends on mcp-go ────────────────────────────
	dependsOn := xmind.Relationship{
		ID:     id(),
		End1ID: repoLeaf.ID,
		End2ID: mcpGoLeaf.ID,
		Title:  "Depends On",
	}

	return xmind.Sheet{
		ID:               id(),
		RevisionID:       id(),
		Class:            "sheet",
		Title:            "xmind-mcp",
		TopicOverlapping: "overlap",
		RootTopic:        root,
		Relationships:    []xmind.Relationship{dependsOn},
		Theme:            xmind.DefaultTheme,
		Extensions:       xmind.DefaultSheetExtensions(structureClass),
	}
}
