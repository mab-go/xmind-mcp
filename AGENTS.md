# AGENTS.md — xmind-mcp

This file is the authoritative briefing for any AI agent working on this project. Read it in full before writing any code or making any changes.

---

## Project Summary

`xmind-mcp` is a Go MCP server that provides tools for reading and writing local XMind mind map files (`.xmind`). The module path is `github.com/mab-go/xmind-mcp` and the project lives at `/home/matt/Projects/mcp/xmind-mcp`.

---

## Project Structure

```
Dockerfile                — multi-stage image (Alpine build, distroless runtime)
.dockerignore             — Docker build context exclusions
.github/
  workflows/
    ci.yml                — test, lint, and cyclomatic-complexity report artifact on push and PR
    docker-publish.yml    — multi-platform image build and push to GHCR
cmd/
  xmind-mcp/
    main.go               — cobra command; calls server.RunStdioServer()
  gen-example/
    main.go               — dev utility; generates the README example map via xmind package directly
internal/
  server/
    server.go             — MCP server setup and tool registration
    tools.go              — tool definitions as package-level var declarations
    hooks.go              — MCP lifecycle hooks (before/after initialize)
    handler/
      handler.go          — XMindHandler struct and shared utilities (textResult, jsonResult, etc.)
      handler_test.go     — tests for handler helpers (e.g. deepCloneTopic)
      sheets.go           — handlers for Tier 1: file & sheet management tools
      find.go             — handlers for Tier 2: search/find tools
      mutate.go           — handlers for Tier 3: topic mutation tools
      utils.go            — handlers for Tier 4: utility tools
  version/
    version.go            — build metadata (Version, Commit, Date) injected via ldflags
  xmind/
    types.go              — Go structs mirroring the content.json schema
    json_codec.go         — custom JSON: preserve unknown keys, notes shape
    reader.go             — open zip, parse content.json → []Sheet
    writer.go             — serialize []Sheet → content.json, write back to zip
    atomic_replace.go     — replaceTempFile: cross-platform atomic rename helper
    defaults.go           — DefaultTheme and DefaultSheetExtensions for new maps
    default_theme.json    — embedded default theme blob (sourced from kitchen-sink fixture)
    stub_content.xml      — embedded legacy content.xml written into every new zip
  logging/                — context-carried logging helpers; do not modify
testdata/
  kitchen-sink.xmind      — primary test fixture; exercises every supported feature
.cursor/
  rules/
    keep-docs-current.mdc — always-on rule: update docs after structural changes
  skills/                 — agent skill definitions (invoke via /skill-name)
```

---

## Essential Reading

Before touching any code, read these files in order:

1. **`internal/xmind/types.go`** — the Go struct definitions that mirror `content.json`. Everything else depends on getting this right.
2. **`internal/xmind/reader.go`** and **`writer.go`** — the zip I/O layer. Understand the read/write lifecycle before writing any handler.
3. **`internal/xmind/json_codec.go`** — the custom marshal/unmarshal logic that preserves unknown keys. Read this before touching any struct that has a custom codec.

---

## Read/Write Lifecycle

Every mutating tool call follows this exact pattern — do not deviate from it:

```
1. Open the .xmind zip archive
2. Read and parse content.json into []Sheet
3. Apply the change to the in-memory structs
4. Serialize the updated structs back to JSON
5. Write the updated content.json to a temp file, then atomically swap it in
6. Return result
```

**Atomicity is required.** Always write to a temp file first and rename/swap on success. If a write fails mid-way, the original file must be left untouched. There is no session state and no temp files left behind on failure.

### `content.json` fidelity (unknown keys)

Major JSON object types in the workbook tree use custom `MarshalJSON` / `UnmarshalJSON` in **`internal/xmind/json_codec.go`**. Unknown keys on those objects are stored in an unexported `extra` field (`json:"-"` on the struct) and merged back on marshal so XMind-specific or forward-compatible properties survive a read → edit → write cycle.

When you add a **new first-class field** to `Sheet`, `Topic`, `Children`, `Relationship`, `Boundary`, the summary-range `Summary`, `Marker`, `Position`, `TopicImage`, or `AttributedTitleItem`, you **must** add the JSON key to the corresponding allowlist in `json_codec.go` (e.g. `topicKnownKeys`, `sheetKnownKeys`, or the `deleteKeys(...)` list for that type). Otherwise the new field will be treated as opaque preserved data and will not populate the typed field.

**Limitation:** Sibling keys on each **`Extension`** object (alongside `provider`, `content`, `resourceRefs`) are still dropped. Non–`content.json` zip entries continue to be preserved by `WriteMap` via raw copy.

**String encoding:** When persisting `content.json`, use **`xmind.WriteMap`** / **`xmind.CreateNewMap`** only. They marshal with `encoding/json` HTML escaping disabled so characters such as `&`, `<`, and `>` appear literally in JSON strings (matching XMind on-disk files). Do not use `json.Marshal` on `[]Sheet` for that payload; default marshaling can rewrite `json.Marshaler` output in ways XMind mishandles.

**Notes:** `Notes` uses `*NoteContent` with `omitempty` so topics that only have `notes.plain` in the file do not gain an empty `notes.realHTML` on round-trip.

---

## Conventions

These patterns are established conventions for this repository and must be followed exactly.

### Tool definitions

Define tools as package-level `var` declarations in `internal/server/tools.go`:

```go
var toolOpenMap = mcp.NewTool(
    "xmind_open_map",
    mcp.WithDescription("..."),
    mcp.WithString("path", mcp.Required(), mcp.Description("...")),
)
```

Register them in `server.go`:

```go
s.AddTool(toolOpenMap, h.OpenMap)
```

All tool names are prefixed with `xmind_` to avoid collisions in multi-server environments.

### Handler method signature

All handler methods must match the `mark3labs/mcp-go` contract exactly:

```go
func (h *XMindHandler) OpenMap(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
```

### Argument extraction

Every handler that accepts a `path` argument uses the two shared helpers defined in `handler.go`. Do not inline this logic:

```go
args := req.GetArguments()

// absPathFromArgs validates, resolves, and absolutizes args["path"].
// Returns a tool-level error result if the argument is missing or invalid.
absPath, toolErr := absPathFromArgs(args)
if toolErr != nil {
    return toolErr, nil
}

// statAndReadMap checks existence, reads the zip, and parses content.json.
// toolErr is non-nil for caller-fixable conditions (file not found, bad zip).
// err is non-nil for unexpected I/O or environment failures.
sheets, toolErr, err := statAndReadMap(absPath)
if toolErr != nil {
    return toolErr, nil
}
if err != nil {
    return nil, err
}
```

For additional non-path arguments (string, int, bool), extract them directly from `args` after the path helpers:

```go
title, _ := args["title"].(string)
if title == "" {
    return mcp.NewToolResultError("missing required argument: title"), nil
}
```

### Text response helper

Use `textResult` (defined in `handler.go`) for all successful text responses:

```go
func textResult(text string) *mcp.CallToolResult {
    return &mcp.CallToolResult{
        Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
    }
}
```

Use `jsonResult` when the successful response body should be JSON (e.g. structured data from mutating operations):

```go
func jsonResult(v any) (*mcp.CallToolResult, error) {
    data, err := json.Marshal(v)
    if err != nil {
        return nil, fmt.Errorf("marshal response: %w", err)
    }
    return textResult(string(data)), nil
}
```

### Shared traversal and lookup utilities

`handler.go` also defines helpers for navigating the topic tree. Use them rather than writing new traversal code:

| Helper | Purpose |
|---|---|
| `walkTopics(root, depth, parent, fn)` | Preorder DFS over attached → detached → summary; `fn` returns `false` to stop |
| `findTopicByID(root, id)` | First topic matching `id` in DFS order |
| `ancestryPath(root, targetID)` | Titles from root to (not including) the target; `nil` if target is root or not found |
| `findParentOfTopic(root, targetID)` | Parent topic, child index, and list name (`"attached"`, `"detached"`, `"summary"`) |
| `isDescendantOf(ancestor, descendantID)` | Reports whether a node is the ancestor or any node in its subtree |
| `countTopics(t)` | Total node count for a subtree (self + all descendants) |
| `deepCloneTopic(root)` | JSON round-trip clone of a subtree, then fresh UUIDs for topics and summary/boundary descriptors |

### Logging

Do not modify `internal/logging/`. Use the context-carried logger pattern throughout:

```go
log, _ := logging.FromContext(ctx)
log.WithField("path", path).Debug("Opening map")
```

### main.go

Follow `cmd/xmind-mcp/main.go` for the entrypoint shape (cobra command calling `server.RunStdioServer()`):

```go
var cmd = &cobra.Command{
    Use:   "xmind-mcp",
    Short: "XMind MCP Server",
    Long:  "An MCP server for reading and writing local XMind mind map files.",
    RunE: func(_ *cobra.Command, _ []string) error {
        return server.RunStdioServer()
    },
}
```

### Dependencies

Use only these external dependencies — do not add others without good reason:

```
github.com/google/uuid
github.com/mark3labs/mcp-go
github.com/sirupsen/logrus
github.com/spf13/cobra
github.com/spf13/viper
```

`github.com/spf13/pflag` is pulled in transitively by `cobra` and `viper`; do not import it directly.

For zip and JSON handling, use Go stdlib only: `archive/zip` and `encoding/json`.

### Verification before you finish

Treat a change as incomplete until **`make build test lint`** passes locally. The Makefile runs **golangci-lint** (including **revive**); do not rely on `go test` alone to catch style and API-surface rules.

**Cyclomatic complexity (report-only):** After `make setup`, **`make cyclo`** runs [gocyclo](https://github.com/fzipp/gocyclo) with **`-over 10`** on the module tree (`gocyclo` takes a directory path, not a Go package pattern—see the Makefile). It lists functions with complexity **strictly greater than 10** (including `*_test.go`). This is a stricter bar than [Go Report Card](https://goreportcard.com/)’s public check (**cyclo-over=15**); the badge’s **percentage is file-based** (any over-threshold function fails the file), while the CLI lists **individual functions**, so do not expect the same headline number. CI uploads a **`cyclo.txt`** artifact from a **non-blocking** job; it does not fail the workflow when violations exist.

Also verify that documentation reflects the change before finishing. Update **`AGENTS.md`**, **`.cursor/rules/`**, and **`.cursor/skills/`** as needed:

| Change made | What to review/update |
|---|---|
| New file added or removed | `Project Structure` tree in `AGENTS.md` |
| New handler helper added to `handler.go` | Appropriate subsection in `AGENTS.md` (traversal/lookup table for tree helpers; Text response helper for `textResult` / `jsonResult`) |
| New tool registered | Conventions section in `AGENTS.md` if a new pattern is introduced |
| New struct field or codec allowlist entry | Schema Critical Notes in `AGENTS.md` |
| New convention established | `Conventions` section in `AGENTS.md` |
| New `.cursor/rules/` or `.cursor/skills/` file | Reflect in `AGENTS.md` if it affects agent behavior |

---

## Error Handling

This is the most important convention to get right. There are two distinct failure modes and they must not be conflated.

### Tool errors (expected / recoverable)

Topic not found, file doesn't exist, bad arguments, file is not a valid `.xmind` — these are conditions the model can read and act on. Return them as tool-level errors:

```go
return mcp.NewToolResultError("topic not found: no topic with title \"Foo\" exists"), nil
```

The signature is `(*mcp.CallToolResult, nil)` — the Go error is `nil`. The `IsError: true` flag is set inside the result by `mcp.NewToolResultError`.

### Protocol errors (unexpected / unrecoverable)

I/O errors after a successful open, failures while persisting the map, internal invariant violations — these are genuine bugs or environmental failures. Return them as Go errors:

```go
if err := xmind.WriteMap(absPath, sheets); err != nil {
    return nil, fmt.Errorf("write map: %w", err)
}
```

The signature is `(nil, error)`. These surface as MCP protocol errors and look like crashes to the model — reserve them for situations that are genuinely unrecoverable.

**Rule of thumb:** if the caller could fix it by passing different arguments or pointing at a different file, it's a tool error. If something has gone wrong inside the server that the caller can't influence, it's a protocol error.

---

## Schema Critical Notes

These are the places where it is easy to produce a map that opens in XMind but is silently broken. Read carefully.

### Summaries require a double-write

Summaries must be written to **two places simultaneously** on the parent topic. Omitting either will produce a broken map:

1. The summary topic itself goes in `Children.Summary` on the parent topic.
2. A range descriptor goes in `Topic.Summaries` on the same parent topic.

```go
// Range format: zero-based inclusive tuple
// "(0,2)" means the summary spans children at indices 0, 1, and 2
type Summary struct {
    ID      string `json:"id"`
    Range   string `json:"range"`   // e.g. "(0,2)"
    TopicID string `json:"topicId"` // ID of the summary topic in Children.Summary
}
```

### Relationships live on the sheet, not on topics

Relationships are stored in `Sheet.Relationships` — a top-level array on the sheet object. They are **not** stored on either of the connected topics. Writing a relationship to a topic will appear to work but the relationship will not render in XMind.

### Fields to never modify

These fields must be passed through untouched on every read/write cycle unless the user explicitly requests a change:

| Field | Location | Why |
|---|---|---|
| `theme` | `Sheet` | Opaque blob; XMind will break if it's altered or dropped |
| `extensions` | `Sheet` | Contains sheet-level metadata XMind manages internally |
| `structureClass` | `Topic` | Overrides sheet-level layout for a subtree; dropping it changes the map layout |
| `titleUnedited` | `Topic` | Set `false` on user-created topics; preserve existing value on all others |

### Topic IDs

Always preserve existing UUIDs — never regenerate them. Generate new UUIDs only for newly created topics (use `github.com/google/uuid`). The `id` field on a topic must never be changed by a mutating operation.

### Notes require both `plain` and `realHTML`

When writing a topic note, both fields must be populated. Writing only `plain` leaves the rich-text panel blank in XMind; writing raw text into `realHTML` directly renders unstyled. Use the `plainToRealHTML` helper in `mutate.go` to convert:

```go
notes := &xmind.NoteContent{
    Plain: xmind.NotesPlain{Content: plainText},
    RealHTML: xmind.NotesRealHTML{Content: plainToRealHTML(plainText)},
}
```

`plainToRealHTML` wraps each newline-delimited line in `<div>…</div>` with HTML-escaped content, matching the output XMind itself produces.

### `title` field absence vs. empty string

When a topic has no title, the `title` field must be **absent** from the JSON — not an empty string. Use `omitempty` on the struct tag (already reflected in `types.go`).

### `structureClass` values

Use these exact Java-namespace strings — never friendly names or abbreviations:

| Layout | `structureClass` |
|---|---|
| Mind Map | `org.xmind.ui.map.clockwise` |
| Logic Chart | `org.xmind.ui.logic.right` |
| Tree Chart | `org.xmind.ui.tree.right` |
| Org Chart | `org.xmind.ui.org-chart.down` |
| Fishbone | `org.xmind.ui.fishbone.leftHeaded` |
| Timeline | `org.xmind.ui.timeline.horizontal` |
| Brace Map | `org.xmind.ui.brace.right` |
| Tree Table | `org.xmind.ui.treetable` |
| Matrix | `org.xmind.ui.matrix` |

---

## Layer Dependencies

The codebase is organized in layers. When making changes, understand which layer you are working in and what it may depend on.

| Layer | Packages / Files | Depends on |
|---|---|---|
| Schema | `internal/xmind/types.go`, `json_codec.go` | stdlib only |
| I/O | `internal/xmind/reader.go`, `writer.go`, `atomic_replace.go` | Schema layer |
| Handlers | `internal/server/handler/*.go` | I/O layer, Schema layer |
| Server wiring | `internal/server/server.go`, `tools.go`, `hooks.go` | Handlers |
| Entry point | `cmd/xmind-mcp/main.go` | Server wiring |

Changes to the Schema layer (adding/removing struct fields, modifying codec allowlists) will ripple into handlers. Changes to handlers are self-contained as long as the I/O layer API is stable. Never import handler packages from the xmind package or vice versa.

---

## Test Fixture

The kitchen sink file at `testdata/kitchen-sink.xmind` exercises every supported XMind feature: all 9 map structures, floating topics, relationships, summaries, boundaries, all marker types, task states, notes, labels, links, and LaTeX equations.

Use it as the primary fixture for all handler development and testing. Do not modify it — treat it as read-only ground truth. If a test needs to write to a file, copy it to a temp path first.

For the initial implementation pass, a basic smoke test or two per handler file is sufficient. Comprehensive test coverage will be added in a follow-up pass.
