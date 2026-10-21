# AGENTS.md — xmind-mcp

This file is the authoritative briefing for any AI agent working on this project. Read it in full before writing any code or making any changes.

---

## Project Summary

`xmind-mcp` is a Go MCP server that provides tools for reading and writing local XMind mind map files (`.xmind`). The module path is `github.com/mab-go/xmind-mcp`.

---

## Project Structure

```
Dockerfile                — multi-stage image (Alpine build, distroless runtime)
CLAUDE.md                 — symlink → AGENTS.md (Claude Code auto-loads this)
.dockerignore             — Docker build context exclusions
.goreleaser.yaml          — goreleaser config; cross-builds Linux/Windows archives on `v*` tags
codecov.yml               — Codecov config; informational (non-blocking) project & patch coverage; ignores cmd/gen-example
.github/
  workflows/
    ci.yml                — test (+ coverage across packages with -coverpkg=./..., uploaded to Codecov), lint, and cyclomatic-complexity report artifact on push and PR
    docker-publish.yml    — multi-platform image build and push to GHCR
    release.yml           — goreleaser release on `v*` tag push (binaries to GitHub Releases)
cmd/
  xmind-mcp/
    main.go               — cobra command; calls server.RunStdioServer()
  gen-example/
    main.go               — dev utility; generates the README example map via xmind package directly
internal/
  server/
    server.go             — MCP server setup and tool registration
    server_test.go        — asserts server capabilities (no MCP logging), server info, instructions, server/discover, lifecycle hook logging, and panic recovery
    tools.go              — tool definitions as package-level var declarations
    tools_test.go         — asserts every tool's title and annotation hints, and that whole-number parameters are declared as non-negative integers
    output_schema_test.go — asserts which tools declare an outputSchema and that their structured output matches it
    error_contract_test.go — calls every tool with bad paths, bad IDs, bad numbers, unreachable paths and read-only maps; each must return a tool error and change nothing on disk
    hooks.go              — MCP lifecycle hooks (initialize and server/discover)
    recovery.go           — tool-handler middleware: recovers panics, logs tool.panic with a stack trace
    instructions.go       — server instructions sent in the initialize and server/discover results
    events.go             — logging event-name constants (logging.Event)
    handler/
      handler.go          — XMindHandler struct and shared utilities (textResult, jsonResult, etc.)
      patherrors_other.go — Unix errors for a path that can't be used (name too long, symlink loop, read-only filesystem)
      patherrors_windows.go — the Win32 equivalents (Go's Windows ENAMETOOLONG, ELOOP and EROFS are values Windows never returns)
      handler_test.go     — tests for handler helpers (e.g. deepCloneTopic)
      schema.go           — OutputSchema[T]: outputSchema tool option generated from a response type
      sheets.go           — handlers for Tier 1: file & sheet management tools
      sheets_test.go      — tests for the Tier 1 handlers
      find.go             — handlers for Tier 2: search/find tools
      find_test.go        — tests for the Tier 2 handlers
      mutate.go           — handlers for Tier 3: topic mutation tools
      mutate_test.go      — tests for the Tier 3 handlers, including idempotent-repeat and concurrent-write tests
      utils.go            — handlers for Tier 4: utility tools
      utils_test.go       — tests for the Tier 4 handlers, including outline round trips and find-and-replace
      integration_test.go — multi-tool workflows and edge cases (find then mutate, build from scratch, fixture preservation, Unicode)
      update_integration_test.go — MapWriter paths through the handlers (no-match close, post-open tool errors, zip-entry preservation)
      testhelpers_test.go — TestMain (fails fast if the fixture is a Git LFS stub) and copyFixture
  version/
    version.go            — build metadata (Version, Commit, Date) injected via ldflags
  xmind/
    types.go              — Go structs mirroring the content.json schema
    json_codec.go         — custom JSON: preserve unknown keys, notes shape
    json_codec_test.go    — unknown-key and notes round trips
    reader.go             — open zip, parse content.json → []Sheet (ReadMap, parseContentFromZip)
    reader_test.go        — ReadMap errors, fixture parsing, and the WriteMap round trip
    writer.go             — serialize []Sheet → content.json, write back to zip (WriteMap, writeMapBody)
    writer_test.go        — WriteMap/CreateNewMap: entry preservation, atomic failure, no-overwrite, file modes, read-only maps refused
    update.go             — OpenMapForUpdate/MapWriter: single-open read-then-commit handle for mutations
    update_test.go        — Commit/WriteMap parity, MapWriter lifecycle guards, writes through symlinks
    pathlock.go           — lockPath/resolveMapPath: in-process per-file write lock, keyed by the symlink-resolved path
    pathlock_test.go      — path resolution, lock release and waiting
    atomic_replace.go     — replaceTempFile/replaceViaBackup: atomic replace (backup-restore on Windows)
    atomic_replace_test.go — replaceViaBackup: success, restore on failure, double fault
    testmain_test.go      — TestMain: fails fast if the fixture is a Git LFS stub
    defaults.go           — DefaultTheme and DefaultSheetExtensions for new maps
    default_theme.json    — embedded default theme blob (sourced from kitchen-sink fixture)
    stub_content.xml      — embedded legacy content.xml written into every new zip
docs/
  example.xmind           — README example map; generated by `make gen-example` (Git LFS)
  xmind-mcp-repo-mind-map.png — README screenshot of example.xmind, taken by hand in XMind
testdata/
  kitchen-sink.xmind      — primary test fixture; exercises every supported feature
.agents/
  skills/
    commit-message/        — Git commit message generation from diffs
    review-plan/           — plan review for accuracy, correctness, and task items
    verify-changes/        — build/test/lint verification and doc update checks
    xmind-mcp-smoke-test/  — release pre-validation: exercises the full MCP tool surface against a scratch .xmind
.claude/
  skills/                 — symlink → ../.agents/skills/
.cursor/
  rules/
    keep-docs-current.mdc — always-on rule: update docs after structural changes
  skills/                 — symlink → ../.agents/skills/
```

---

## Essential Reading

Before touching any code, read these files in order:

1. **`internal/xmind/types.go`** — the Go struct definitions that mirror `content.json`. Everything else depends on getting this right.
2. **`internal/xmind/reader.go`** and **`writer.go`** — the zip I/O layer. Understand the read/write lifecycle before writing any handler.
3. **`internal/xmind/json_codec.go`** — the custom marshal/unmarshal logic that preserves unknown keys. Read this before touching any struct that has a custom codec.

**Prefer LSP for semantic Go queries.** For finding definitions, references,
implementations, call hierarchy, or type signatures in Go code, use the LSP
tool over grep -- gopls is configured for this repo and returns semantic
answers that disambiguate same-named symbols across packages. Grep is still
fine for plain text searches, non-code files, and quick locate-by-string.

---

## Read/Write Lifecycle

Every mutating tool call follows this exact pattern — do not deviate from it:

```
1. Acquire the in-process per-path lock and open the .xmind zip archive
2. Read and parse content.json into []Sheet
3. Apply the change to the in-memory structs
4. Serialize the updated structs back to JSON
5. Write the updated content.json to a temp file, then atomically swap it in
6. Release the lock (Commit or Close)
7. Return result
```

**Atomicity is required.** Always write to a temp file first and rename/swap on success. If a write fails mid-way, the original file must be left untouched. There is no session state and no temp files left behind on failure. Close every file handle in a `defer`, guarded by a flag when ownership passes to the caller (as `WriteMap` and `OpenMapForUpdate` do), not only on error returns, so no early exit, including a panic, can leak it.

**Permissions survive the swap.** `os.CreateTemp` always creates files with mode `0600`, so `finishZipTempFile` sets the finished temp file's permission bits before the rename: an edit keeps the original file's bits (`writeMapBody` stats the target), and `CreateNewMap` creates its placeholder with `0666` and lets the umask filter it (as `os.Create` does), then gives the new map the same bits. Any new temp-then-swap path must pass a mode through the same way. Ownership is not restored: when another user edits a group-shared map, the replaced file belongs to that user. Extended attributes and ACLs are not carried over either.

**Read-only maps are not replaced.** The swap needs write access only to the map's folder, so a rename would replace even a map the server can't write. Before writing anything, `writeMapBody` refuses with `xmind.ErrMapReadOnly` (which matches `fs.ErrPermission`) a map with no write permission bits (even as root) or one it is refused opening for writing (an owner without the write bit on a group-writable map, an ACL, an immutable file), so `WriteMap` and `Commit` leave it as it is. An edit also needs write access to the map's folder, because the temp file is created there. Handlers report both cases as tool errors (see `commitFailure` under Argument extraction).

**Single-open mutation path.** Mutating handlers open the `.xmind` once via `xmind.OpenMapForUpdate` (through the `statAndOpenMapForUpdate` / `loadSheet…ForUpdate` helpers), which returns the parsed `[]Sheet` plus a `*xmind.MapWriter` that keeps the zip reader open. The handler mutates the sheets in memory and calls `mw.Commit(sheets)` to persist them, raw-copying the still-open non-`content.json` entries (steps 1–5 above happen across one file open instead of two). `Commit` closes the read handle *before* the atomic swap, so on Windows the swap runs with no open handle on the original. `Commit` is single-use (a second `Commit`, or one after `Close`, errors). The free-standing `xmind.WriteMap` (which re-reads the file fresh) is retained for tests and any read-fresh caller; `xmind.CreateNewMap` writes brand-new maps and never overwrites: it claims the destination with `O_CREATE|O_EXCL` before writing (an existing path fails with an error wrapping `fs.ErrExist`), renames the finished temp file over that empty placeholder, and removes the placeholder on failure. Callers must not add their own existence check, which would race with a concurrent creator; to replace a file deliberately (e.g. `cmd/gen-example`), remove it first. The temp-then-swap mechanics below are unchanged and shared by both `WriteMap` and `Commit` via the internal `writeMapBody`, which swaps onto the symlink-resolved path (creating the temp file in the real file's directory), so an edit through a symlink to the map updates the real file and leaves the link in place.

**Per-path write lock.** Steps 1–6 hold an in-process lock for the file (`lockPath` in `internal/xmind/pathlock.go`), so overlapping tool calls (mcp-go runs `tools/call` on a worker pool) on one file run one at a time instead of each committing over the others' changes. `OpenMapForUpdate` acquires it and the `MapWriter` holds it until a successful `Commit` or `Close`; the mandatory `defer mw.Close()` is what releases it on every other path, so handlers need no lock code of their own. `WriteMap` holds it for its own re-read and swap (not for a caller's earlier read), and `CreateNewMap` holds it from before the `O_EXCL` claim until the rename, so a locked writer never opens the empty placeholder. The key is the absolute path with symlinks resolved (best effort: the directory alone for a file not yet created), the same path the swap replaces, so a symlinked directory or a symlink to the file shares the real file's lock. Reads take no lock: the rename is atomic, so a reader sees the old or the new file (or, during `CreateNewMap`, the empty placeholder). The lock is not reentrant: while a goroutine holds a `MapWriter` for a file, it must not call `OpenMapForUpdate`, `WriteMap`, or `CreateNewMap` on that file. What it does **not** cover:

- **Other processes.** A second xmind-mcp process, the XMind app, or any other editor writing the same file can still overwrite changes. There is no cross-process file lock.
- **Request ordering.** Concurrent or pipelined calls run in no defined order. A client that sends `xmind_create_map` and then `xmind_open_map` without awaiting the first response can still get "file not found". Ordering is the client's job (await the response).
- **Other names for the same file.** On a case-insensitive filesystem, paths that differ only in case get separate locks, and so do hard links to the same file.

The temp-then-swap happens in `replaceTempFile` (`internal/xmind/atomic_replace.go`). On Unix, `os.Rename` atomically replaces the destination. On Windows, `os.Rename` cannot overwrite an existing file, so `replaceViaBackup` moves the original aside to a uniquely named backup first, renames the temp into place, then removes the backup — restoring the backup if the rename fails. This keeps the original recoverable on Windows; never reintroduce a plain remove-then-rename there, which can destroy the original if the rename fails.

### `content.json` fidelity (unknown keys)

Major JSON object types in the workbook tree use custom `MarshalJSON` / `UnmarshalJSON` in **`internal/xmind/json_codec.go`**. Unknown keys on those objects are stored in an unexported `extra` field (a `map[string]json.RawMessage` with no JSON tag; the custom marshaler never reflects over the struct directly) and merged back on marshal so XMind-specific or forward-compatible properties survive a read → edit → write cycle.

When you add a **new first-class field** to `Sheet`, `Topic`, `Children`, `Relationship`, `Boundary`, the summary-range `Summary`, `Marker`, `Position`, `TopicImage`, `AttributedTitleItem`, `Notes`, or `NoteContent`, you **must** add the JSON key to the corresponding allowlist in `json_codec.go` (e.g. `topicKnownKeys`, `sheetKnownKeys`, or the `deleteKeys(...)` list for that type — `Notes` uses `"plain"`/`"realHTML"`, `NoteContent` uses `"content"`). Otherwise the new field will be treated as opaque preserved data and will not populate the typed field.

**Limitation:** Sibling keys on each **`Extension`** object (alongside `provider`, `content`, `resourceRefs`) are still dropped. Non–`content.json` zip entries continue to be preserved by `WriteMap` and `MapWriter.Commit` via raw copy.

**String encoding:** When persisting `content.json`, use **`xmind.WriteMap`** / **`xmind.CreateNewMap`** only. They marshal with `encoding/json` HTML escaping disabled so characters such as `&`, `<`, and `>` appear literally in JSON strings (matching XMind on-disk files). Do not use `json.Marshal` on `[]Sheet` for that payload; default marshaling can rewrite `json.Marshaler` output in ways XMind mishandles.

**Notes:** `Notes` and `NoteContent` carry the same `extra` bag and custom codec as the other tree types, so unknown sibling keys (alongside `plain`/`realHTML` on the notes object, or alongside `content` inside a note body) survive a round-trip. `Notes` still uses `*NoteContent` with `omitempty` and only populates `Plain`/`RealHTML` when their keys are present, so topics that only have `notes.plain` in the file do not gain an empty `notes.realHTML` on round-trip.

---

## Conventions

These patterns are established conventions for this repository and must be followed exactly.

### Tool definitions

Define tools as package-level `var` declarations in `internal/server/tools.go`:

```go
var toolOpenMap = mcp.NewTool(
    "xmind_open_map",
    mcp.WithDescription("..."),
    withAnnotations("Open Map", effectReadOnly),
    handler.OutputSchema[handler.OpenMapResponse](),
    mcp.WithString("path", mcp.Required(), mcp.Description("...")),
)
```

Every tool must declare its display title and MCP behavior hints with `withAnnotations(title, effect)` (defined in `tools.go`), placed right after `mcp.WithDescription`. This is mandatory: without it, `mcp.NewTool` defaults every tool to destructive and open-world, so clients can't auto-approve reads. The title is set both as the tool's top-level `title` and as `annotations.title`, for clients that read only one of them. Pick the `toolEffect`:

| Effect | Use when the tool… |
|---|---|
| `effectReadOnly` | never writes the file |
| `effectAdditive` | only adds content, and each repeat call adds more |
| `effectAdditiveIdempotent` | only adds content, and a repeat call with the same arguments has no further effect |
| `effectDestructive` | may change or remove existing content, and a repeat call can change it again |
| `effectDestructiveIdempotent` | may change or remove existing content, and a repeat call has no further effect (renames, sets, deletes) |

`openWorldHint` is always `false`: every tool touches only the local file named by `path`. The `toolEffect` zero value is invalid and an unknown effect makes `withAnnotations` panic; since tools are package-level vars, that fails every test in the package and stops the binary at startup. When adding an effect, add a `case` for it. Add each new tool to `expectedAnnotations` in `internal/server/tools_test.go`; the test fails for any registered tool missing from that table. Also add it to `contractCases` in `internal/server/error_contract_test.go`, with base arguments that succeed on that test's fixture map and the names of its ID arguments; `TestToolErrorContractCoversAllTools` fails for any registered tool missing from that table, and `TestToolErrorContract` then checks the new tool returns tool errors, never protocol errors or writes, for bad paths, bad IDs, bad numbers, unreachable paths and read-only maps. It finds a tool's integer parameters in its input schema, so a new one is tested without editing the table.

A tool whose handler returns `jsonResult` must also declare its output schema with `handler.OutputSchema[handler.<Type>]()`, right after `withAnnotations`. `<Type>` is the exported struct the handler passes to `jsonResult` (e.g. `handler.OpenMapResponse`). Add the tool to `jsonOutputTools` in `internal/server/output_schema_test.go`, and add cases to `TestStructuredOutputMatchesSchema` that cover each success return path. Clients may check results against the schema, so a wrong schema on a writing tool can look like a failure after the file was saved. `OutputSchema` builds the schema with `jsonschema.For`, which requires fields without `omitempty` and rejects keys the struct does not name. A recursive type, or a type with custom JSON (like `xmind.Marker`, which writes back unknown keys), needs an entry in `outputTypeSchemas` in `handler/schema.go`. Like `withAnnotations`, `OutputSchema` panics on a schema error.

Declare a parameter that must be a whole number (an index, depth, or count) with `mcp.WithInteger(..., mcp.Min(0))`, never `mcp.WithNumber`, so the schema tells the model up front. Still parse it with `requireNonNegativeInt` or `optionalNonNegativeInt`: the server does not validate input against the schema, and JSON numbers arrive as `float64`, so they reject a fraction, a negative number, or one too large for an `int` (converting that would wrap differently on each CPU). Add the parameter to `nonNegativeIntParams` in `internal/server/tools_test.go`; the test fails for any number or integer parameter missing from that table. A signed or fractional parameter would need the test extended.

Register them in `server.go`:

```go
s.AddTool(toolOpenMap, h.OpenMap)
```

All tool names are prefixed with `xmind_` to avoid collisions in multi-server environments.

### Server instructions

`serverInstructions` in `internal/server/instructions.go` is sent in the initialize and server/discover results (`mcpserver.WithInstructions`); clients that support it add it to the model's context. It holds cross-tool workflow only (which tool to call first, where ids come from, when to use the bulk tools). Per-tool detail belongs in that tool's description. Every connected session pays for the text in context, so it must stay within 150 words (`TestInstructionsWordBudget`), and every `xmind_` tool it names must be registered (`TestInstructionsNameRegisteredTools`). Revisit it when a tool is added, renamed, or changes the workflow.

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
// toolErr is non-nil for caller-fixable conditions (file not found, a path that can't
// be accessed, bad zip).
// err is non-nil for unexpected I/O or environment failures.
// Use this in read-only handlers (find/list/get); it closes the file immediately.
sheets, toolErr, err := statAndReadMap(absPath)
if toolErr != nil {
    return toolErr, nil
}
if err != nil {
    return nil, err
}
```

**Mutating handlers** instead use `statAndOpenMapForUpdate` (or the `loadSheet…ForUpdate`
helpers in `mutate.go`), which open the file once and return a `*xmind.MapWriter` so the
write reuses the same open handle (see Read/Write Lifecycle). The mandatory rule: `defer
mw.Close()` **immediately** after the protocol-error check — before any branch that may
return without committing (a sheet/topic-not-found tool error, or the no-change path in
`FindAndReplace`). `mw.Close()` is idempotent and nil-safe (the open helpers return a nil
writer on a file-not-found tool error, which `defer mw.Close()` tolerates), and it is a
no-op after a successful `mw.Commit`.

```go
sheets, mw, toolErr, err := statAndOpenMapForUpdate(absPath)
if err != nil {
    return nil, err
}
defer func() { _ = mw.Close() }()
if toolErr != nil {
    return toolErr, nil
}
// …mutate sheets…
if err := mw.Commit(sheets); err != nil {
    return commitFailure(absPath, err)
}
```

Return a failed `Commit` through `commitFailure` (`handler.go`), never as a bare `write map` error. It returns a tool error ("cannot save map at …") when the map file is read-only (`xmind.ErrMapReadOnly`), the filesystem is read-only (`errReadOnlyFS`: `EROFS`, or `ERROR_WRITE_PROTECT` on Windows), or the map's folder can't be written (any other `fs.ErrPermission`), naming that cause; the map is unchanged in each case. Any other failure is a `write map` protocol error.

**Idempotent tools must not write when nothing changes.** A tool annotated `effectAdditiveIdempotent` or `effectDestructiveIdempotent` (see Tool definitions) must leave the file byte-identical when a call changes nothing, so a repeated call has no effect. Do not bump `revisionId` and commit unconditionally. Use the helpers in `mutate.go`: `commitSheetChange(mw, sheets, sh, changed)` bumps the sheet's `revisionId` and commits only when `changed` is true. Compute `changed` from a specific check (`setTopicTitle`, `attachedOrderMatches`) or wrap the mutation in `trackTopicChanges`, which compares JSON snapshots taken before and after. When the mutation itself is not reversible in place, check before mutating and return early, as `MoveTopic` does with `moveIsNoOp`. The no-op path returns the same success response as a real change. `TestIdempotentToolsRepeatDoesNotWrite` in `mutate_test.go` covers each idempotent tool.

For additional non-path arguments, use the shared extraction helpers rather than inlining `args[...]` lookups and type assertions. Each returns a `*mcp.CallToolResult` tool error you forward with `if … != nil { return …, nil }`:

| Helper | Location | Semantics |
|---|---|---|
| `requireString(args, key)` | `mutate.go` | Required string; rejects missing, non-string, and empty. |
| `stringArgAllowEmpty(args, key)` | `utils.go` | Required string; rejects missing and non-string, but permits empty. |
| `requireNonNegativeInt(args, key)` | `mutate.go` | Required whole number ≥ 0 that fits in an `int` (accepts JSON `float64` or `int`). |
| `optionalNonNegativeInt(args, key)` | `mutate.go` | Optional whole number with the same checks → `*int`; nil when absent/null. |
| `optionalString(args, key)` | `handler.go` | Optional string → `(val, present, toolErr)`. `present=false` when absent/null; errors only on a non-string value; empty string is returned as-is for the caller to interpret (default vs. reject). |

```go
title, terr := requireString(args, "title")
if terr != nil {
    return terr, nil
}
```

For an optional argument with a default, let the caller decide what empty means:

```go
sheetTitle, present, terr := optionalString(args, "sheet_title")
if terr != nil {
    return terr, nil
}
if !present || sheetTitle == "" {
    sheetTitle = "Sheet 1"
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

Use `jsonResult` whenever the successful response body is JSON; do not marshal by hand. It returns the JSON as text and the same value as the result's `structuredContent`, so the tool must declare an output schema (see Tool definitions):

```go
func jsonResult(v any) (*mcp.CallToolResult, error) {
    data, err := json.Marshal(v)
    if err != nil {
        return nil, fmt.Errorf("marshal response: %w", err)
    }
    res := textResult(string(data))
    res.StructuredContent = v
    return res, nil
}
```

When a handler writes the map, call `jsonResult` before `mw.Commit` if the response is fully known by then (as `finishFindReplace` does), so a marshal failure never follows a write.

### Shared traversal and lookup utilities

`handler.go` also defines helpers for navigating the topic tree. Use them rather than writing new traversal code:

| Helper | Purpose |
|---|---|
| `walkTopics(root, depth, parent, fn)` | Preorder DFS over attached → detached → summary; `fn` returns `false` to stop |
| `findTopicByID(root, id)` | First topic matching `id` in DFS order |
| `ancestryPath(root, targetID)` | Titles from root to (not including) the target; `nil` if target is root or not found |
| `findParentOfTopic(root, targetID)` | Parent topic, child index, and list name (`"attached"`, `"detached"`, `"summary"`) |
| `findDirectChildInLists(parent, targetID)` | If `targetID` is an immediate child of `parent`, returns its index and list name; else `ok` false |
| `searchChildrenForParent(root, targetID)` | Depth-first: recurse into attached, detached, summary to find which child subtree contains `targetID`’s parent |
| `isDescendantOf(ancestor, descendantID)` | Reports whether a node is the ancestor or any node in its subtree |
| `countTopics(t)` | Total node count for a subtree (self + all descendants) |
| `topicIDSet(root)` | Set of IDs of a subtree's topics (attached, detached, summary); compare sets from before and after a change to find removed topics |
| `deepCloneTopic(root)` | JSON round-trip clone of a subtree, then fresh UUIDs for topics and summary/boundary descriptors; a summary descriptor whose `topicId` is missing from the subtree is left out of the copy |

### Logging

Logging uses `github.com/mab-go/logging`. Log messages are identified by
`logging.Event` constants rather than ad-hoc strings: each package declares its
events as unexported constants (entity.verb naming, e.g. `server.starting`) in a
per-package `events.go` file. Passing an `Event` as the first argument to a level
method lifts it into a structured `event=` field and uses the event name as the
message. Use the context-carried logger pattern throughout:

```go
log, _ := logging.FromContext(ctx)
log.WithField("id", id).Debug(eventServerStarting)
```

The default level is `INFO`, set in `main.go`'s `RunE` via
`logging.SetDefaultConfig` before `server.RunStdioServer`. It is configurable
with the `--log-level` flag or the `XMIND_MCP_LOG_LEVEL` env var
(`debug`/`info`/`warn`/`error`); `parseLogLevel` in `main.go` rejects unknown
values. Reserve `Info` for events visible at the default level (e.g.
`server.ready`, `server.stopped`); keep routine per-connection detail at `Debug`.
The lifecycle hooks (`hooks.go`) log the client name and protocol version at
`Debug`: `server.starting`/`server.started` on `initialize` (protocol 2025-11-25
and earlier) and `server.discovering`/`server.discovered` on `server/discover`.
Clients on 2026-07-28 skip `initialize` and need not call `server/discover`
either, so such a client may log neither pair.
The server does not declare the MCP `logging` capability (no
`mcpserver.WithLogging()`): it never sends `notifications/message`, and MCP spec
2026-07-28 deprecates MCP logging in favor of stderr. Don't re-add it.

### main.go

Follow `cmd/xmind-mcp/main.go` for the entrypoint shape (cobra command that sets the log level, then calls `server.RunStdioServer()`):

```go
var cmd = &cobra.Command{
    Use:     "xmind-mcp",
    Short:   "XMind MCP Server",
    Long:    "An unofficial MCP server for reading and writing local XMind mind map files.",
    Version: fmt.Sprintf("xmind-mcp %s (%s; %s)", version.Version, version.ShortCommit(), version.Date),
    RunE: func(_ *cobra.Command, _ []string) error {
        lvl, err := parseLogLevel(viper.GetString("log-level"))
        if err != nil {
            return err
        }
        logging.SetDefaultConfig(logging.Config{Level: lvl})

        return server.RunStdioServer()
    },
}
```

### Dependencies

Use only these external dependencies — do not add others without good reason:

```
github.com/google/jsonschema-go
github.com/google/uuid
github.com/mab-go/logging
github.com/mark3labs/mcp-go
github.com/santhosh-tekuri/jsonschema/v6
github.com/spf13/cobra
github.com/spf13/viper
```

`github.com/spf13/pflag` is pulled in transitively by `cobra` and `viper`; do not import it directly.

`github.com/google/jsonschema-go` generates tool output schemas (`handler/schema.go`) and `github.com/santhosh-tekuri/jsonschema/v6` validates tool output in tests only. mcp-go already requires both, so keep them at the versions it requires.

For zip and JSON handling, use Go stdlib only: `archive/zip` and `encoding/json`.

### Verification before you finish

Treat a change as incomplete until **`make build test lint`** passes locally. The Makefile runs **golangci-lint** (including **revive**); do not rely on `go test` alone to catch style and API-surface rules.

**Cyclomatic complexity (report-only):** After `make setup`, **`make cyclo`** runs [gocyclo](https://github.com/fzipp/gocyclo) with **`-over 10`** on the module tree (`gocyclo` takes a directory path, not a Go package pattern—see the Makefile). It lists functions with complexity **strictly greater than 10** (including `*_test.go`). CI uploads a **`cyclo.txt`** artifact from a **non-blocking** job; it does not fail the workflow when violations exist.

Also verify that documentation reflects the change before finishing. Update **`AGENTS.md`**, **`.cursor/rules/`**, and **`.agents/skills/`** as needed:

| Change made | What to review/update |
|---|---|
| New file added or removed | `Project Structure` tree in `AGENTS.md` |
| New handler helper added to `handler.go` | Appropriate subsection in `AGENTS.md` (traversal/lookup table for tree helpers; Text response helper for `textResult` / `jsonResult`) |
| New tool registered, renamed, or removed | `expectedAnnotations` in `internal/server/tools_test.go` and `contractCases` in `internal/server/error_contract_test.go`; Tier tables and tool count in `README.md`; steps, coverage, and tool count in the `xmind-mcp-smoke-test` skill; tool leaves, tier notes, and counts in `cmd/gen-example/main.go` (then `make gen-example` and retake the README PNG); `serverInstructions` if the workflow changes; Conventions section in `AGENTS.md` if a new pattern is introduced |
| New struct field or codec allowlist entry | Schema Critical Notes in `AGENTS.md` |
| New convention established | `Conventions` section in `AGENTS.md` |
| New `.cursor/rules/` or `.agents/skills/` file | Reflect in `AGENTS.md` if it affects agent behavior |

---

## Error Handling

This is the most important convention to get right. There are two distinct failure modes and they must not be conflated.

### Tool errors (expected / recoverable)

Topic not found, file doesn't exist, bad arguments, file is not a valid `.xmind`, a path the server can't access (permission denied, not a directory, name too long, symlink loop, invalid characters such as NUL), a save refused because the map file or its filesystem is read-only or its folder can't be written — these are conditions the model can read and act on. Return them as tool-level errors:

```go
return mcp.NewToolResultError("topic not found: no topic with title \"Foo\" exists"), nil
```

The signature is `(*mcp.CallToolResult, nil)` — the Go error is `nil`. The `IsError: true` flag is set inside the result by `mcp.NewToolResultError`.

### Protocol errors (unexpected / unrecoverable)

I/O errors after a successful open, other failures while persisting the map (a full disk, an I/O error), internal invariant violations — these are genuine bugs or environmental failures. Return them as Go errors:

```go
data, err := json.Marshal(v)
if err != nil {
    return nil, fmt.Errorf("marshal response: %w", err)
}
```

The path and save helpers already split these: `statAndReadMap` and `statAndOpenMapForUpdate` turn a missing or inaccessible path into a tool error (`pathAccessToolError`) and return any other stat failure as a protocol error, and `commitFailure` does the same for a failed save. `isPathAccessError` checks `fs.ErrPermission`, `ENOTDIR` and `EINVAL` on every platform, plus `osPathAccessErrors` from `patherrors_other.go` or `patherrors_windows.go`. Go's Windows `ENAMETOOLONG`, `ELOOP` and `EROFS` are invented values that `errors.Is` never matches there, so Windows lists the Win32 errors instead (`ERROR_INVALID_NAME`, `ERROR_FILENAME_EXCED_RANGE`, and so on).

The signature is `(nil, error)`. These surface as MCP protocol errors and look like crashes to the model — reserve them for situations that are genuinely unrecoverable.

**Rule of thumb:** if the caller could fix it by passing different arguments or pointing at a different file, it's a tool error. If something has gone wrong inside the server that the caller can't influence, it's a protocol error.

### Panics

`recoverToolPanics` (`recovery.go`, registered in `server.go`) recovers a panic in any tool handler, logs `tool.panic` at Error level with a stack trace, and returns it as a protocol error, so one bad call cannot kill the stdio server. It is a safety net, not control flow: handlers must never panic on purpose. Return a tool error or a protocol error instead.

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

Removal is the same double-write in reverse: deleting a summary topic also removes its descriptor (`removeSummaryChild` in `mutate.go`). Summary topics cannot be moved; `xmind_move_topic` returns a tool error for one.

### Ranges follow attached-child indices

Summary ranges and boundary ranges (`Topic.Boundaries`, `"(from,to)"` like summaries) are indices into the parent's `Children.Attached`, so every insert into or removal from that list must shift them. Use `insertAttachedAdjustingRanges` and `removeAttachedChild` (both in `mutate.go`), which call `adjustRangesAfterAttachedInsert` / `adjustRangesAfterAttachedRemove`; never splice `Children.Attached` by hand except to append at the end, which cannot affect a range. A range that loses its last child is dropped, and a summary's summary topic goes with it. A boundary with range `"master"` (the whole parent) is never shifted. `xmind_reorder_children` leaves ranges on the same positions.

### Relationships live on the sheet, not on topics

Relationships are stored in `Sheet.Relationships` — a top-level array on the sheet object. They are **not** stored on either of the connected topics. Writing a relationship to a topic will appear to work but the relationship will not render in XMind.

Any change that removes topics must also remove the relationships connected to them. Take `topicIDSet(&sh.RootTopic)` before the change and pass it to `pruneRelationshipsToRemoved` (`mutate.go`) afterwards, as DeleteTopic and MoveTopic do; this also catches summary topics removed when their range collapses.

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
notes := &xmind.Notes{
    Plain:    &xmind.NoteContent{Content: plainText},
    RealHTML: &xmind.NoteContent{Content: plainToRealHTML(plainText)},
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
| I/O | `internal/xmind/reader.go`, `writer.go`, `update.go`, `pathlock.go`, `atomic_replace.go` | Schema layer |
| Handlers | `internal/server/handler/*.go` | I/O layer, Schema layer |
| Server wiring | `internal/server/server.go`, `tools.go`, `hooks.go`, `recovery.go`, `instructions.go` | Handlers |
| Entry point | `cmd/xmind-mcp/main.go` | Server wiring |

Changes to the Schema layer (adding/removing struct fields, modifying codec allowlists) will ripple into handlers. Changes to handlers are self-contained as long as the I/O layer API is stable. Never import handler packages from the xmind package or vice versa.

---

## Test Fixture

The kitchen sink file at `testdata/kitchen-sink.xmind` exercises every supported XMind feature: all 9 map structures, floating topics, relationships, summaries, boundaries, all marker types, task states, notes, labels, links, and LaTeX equations.

Use it as the primary fixture for all handler development and testing. Do not modify it — treat it as read-only ground truth. If a test needs to write to a file, copy it to a temp path first.

Add tests for new behavior next to the existing tests for the file you change (e.g. a new mutation tool's tests go in `handler/mutate_test.go`).
