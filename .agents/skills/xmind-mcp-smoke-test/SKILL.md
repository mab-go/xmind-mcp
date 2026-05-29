---
name: xmind-mcp-smoke-test
description: Release pre-validation smoke test that exercises the entire xmind-mcp MCP tool surface (all 27 tools plus error paths) against a throwaway scratch .xmind file, verifying every write by reading it back through the tools. Use before tagging a release, when validating an xmind-mcp build/deployment, or when the user says "smoke test xmind-mcp", "validate the MCP surface", or "run the release smoke test".
---

# xmind-mcp Smoke Test

This skill is run by a Claude instance that has **xmind-mcp configured as an MCP server** and is pointed at a writable location on disk. Its job is to confirm that every tool in the xmind-mcp surface works end to end before a release ships.

It is a **smoke test**, not a unit test: it drives the live MCP server exactly as a real client would, and verifies each mutation by reading it back through the tools (Claude cannot open `.xmind` files visually, so read-back is the source of truth).

## Scope

- **Exercise all 27 tools at least once** — file/sheet management, find/search, topic mutation, and utilities.
- **Verify every write with a read-back** — after each mutating call, call the matching read tool (`xmind_get_subtree`, `xmind_get_topic_properties`, `xmind_list_relationships`, `xmind_list_sheets`) and assert the change persisted.
- **Exercise error paths** — deliberately trigger recoverable failures and confirm they come back as readable **tool errors** (`isError: true` with a usable message), not as protocol-level crashes.
- **Operate on a self-created scratch file** — never mutate the file the instance was pointed at. All destructive work happens on a throwaway `.xmind` created by `xmind_create_map`.

## Preconditions

1. Confirm the xmind-mcp tools are present (tool names prefixed `xmind_`). If they are missing, stop and report that the MCP server is not connected — the smoke test cannot run.
2. Determine paths **by string manipulation only** (do not assume a filesystem tool is available):
   - **Scratch file** (gets mutated): derive a sibling of the pointed-at path. Given `/dir/whatever.xmind`, use `/dir/xmind-smoke-scratch.xmind`. Given only a directory, use `<dir>/xmind-smoke-scratch.xmind`.
   - **Error-scenario file** (separate, single-sheet): `/dir/xmind-smoke-errors.xmind`.
   - Both must **not already exist** — `xmind_create_map` returns a tool error if the file is present. A pre-existing scratch from a prior run is itself a finding: report it and pick a unique suffix.
3. **Optional read-only sanity pass:** if the pointed-at file is a real `.xmind`, call `xmind_open_map`, `xmind_list_sheets`, and `xmind_flatten_to_outline` on it first. These are non-mutating and prove the read tools work against real-world content. Do not write to it.

## Capturing IDs

Several tools return IDs you must thread into later calls. Note where each comes from:

- `xmind_create_map` / `xmind_add_sheet` return the **sheet id in their text result** (`"...with sheet id <ID>"`, `"added sheet id <ID>"`). Parse it out.
- Root-topic IDs are **not** returned by `create_map` or `open_map`. Get them from `xmind_get_subtree` (the root node's `id`).
- `xmind_add_topic`, `xmind_add_topics_bulk`, `xmind_duplicate_topic`, `xmind_move_topic` return JSON with the new/affected IDs.
- Relationship IDs come from `xmind_list_relationships`.

Keep a running scratch-pad of `sheetMain`, `rootId`, `alphaId`, etc. as you go.

## Test plan

Run the phases in order; later phases depend on structure built earlier. For each numbered step: make the call, then make its **assert** read-back. Record PASS/FAIL with the observed result. A failed read-back is a FAIL even if the write call "succeeded".

### Phase 1 — File & sheet management
1. `xmind_create_map` → scratch path, `root_title="Smoke Root"`, `sheet_title="Main"`. Capture `sheetMain`. **Assert:** success text.
2. `xmind_open_map` → scratch. **Assert:** top-level `sheetCount=1`, and in `sheets[0]`: title `"Main"`, `rootTopicTitle="Smoke Root"`, `topicCount=1` (these are nested per-sheet, not top-level).
3. `xmind_list_sheets`. **Assert:** one sheet, id = `sheetMain`.
4. `xmind_get_subtree` → `sheetMain`. Capture `rootId` (root node `id`). **Assert:** title `"Smoke Root"`, no children.
5. `xmind_add_sheet` → `title="Secondary"`, `root_title="Secondary Root"`. Capture `sheetSecondary`.
6. `xmind_list_sheets`. **Assert:** 2 sheets now.
7. `xmind_delete_sheet` → `sheetSecondary`.
8. `xmind_list_sheets`. **Assert:** back to 1 sheet; `sheetSecondary` gone.

### Phase 2 — Topic creation & bulk add
9. `xmind_add_topic` → parent `rootId`, `title="Alpha"`. Capture `alphaId`. **Assert:** `position=0`.
10. `xmind_add_topic` → parent `rootId`, `title="Beta"`. Capture `betaId`.
11. `xmind_add_topic` → parent `rootId`, `title="Gamma"`. Capture `gammaId`.
12. `xmind_get_subtree` → `sheetMain`. **Assert:** root has children `[Alpha, Beta, Gamma]` in order.
13. `xmind_add_topics_bulk` → parent `alphaId`, `topics=[{"title":"A1","children":[{"title":"A1a"}]},{"title":"A2"}]`. Capture `rootTopicIds`. **Assert:** `addedCount=3` (counts the nested `A1a`, not just top-level items) and `len(rootTopicIds)=2` (top-level only — do not conflate the two).
14. `xmind_get_subtree` → `sheetMain`, `topic_id=alphaId`. **Assert:** children `A1` (with child `A1a`) and `A2`.

### Phase 3 — Topic properties (set / get / bulk / clear)
15. `xmind_set_topic_properties` → `betaId`, `notes="Beta note"`, `labels=["lbl1","lbl2"]`, `markers=["priority-1","task-done"]`, `link="https://example.com"`.
16. `xmind_get_topic_properties` → `betaId`. **Assert:** `notes` contains "Beta note", `labels` match, `markers` match, `href="https://example.com"`.
17. `xmind_set_topic_properties_bulk` → `topic_ids=[alphaId, gammaId]`, `labels=["bulk"]`.
18. `xmind_get_topic_properties` → `alphaId` **and** `gammaId`. **Assert:** both have label `"bulk"`.
19. `xmind_set_topic_properties` → `betaId`, `markers=["priority-1","priority-2"]`, `remove_markers=["priority-2"]`. **Assert** via `xmind_get_topic_properties`: markers = `["priority-1"]` only (remove applied after replace).
20. **Clear semantics:** `xmind_set_topic_properties` → `betaId`, `notes=""`, `link=""`, `labels=[]`, `markers=[]`. **Assert** via `xmind_get_topic_properties`: notes, href, labels, markers all absent.

### Phase 4 — Find & search
21. `xmind_search_topics` → omit `sheet_id`, `query="a"` (all sheets). **Assert:** `matchCount>0` and matches carry `sheetId`/`sheetTitle`.
22. `xmind_search_topics` → `sheet_id=sheetMain`, `query="Beta"`. **Assert:** matches Beta; no sheet metadata on items (single-sheet scope).
23. `xmind_find_topic` → `sheetMain`, `title="Alpha"`. **Assert:** `id=alphaId`; `childrenTitles` include `A1`, `A2`.
24. `xmind_find_topic` → `sheetMain`, `title="A1"`, `parent_id=alphaId` (scoped). **Assert:** found under the scope.

### Phase 5 — Structure: rename, move, reorder, duplicate, floating, delete
25. `xmind_rename_topic` → `gammaId`, `title="Gamma Renamed"`. **Assert** via `xmind_get_subtree`: title updated.
26. `xmind_move_topic` → `gammaId`, `new_parent_id=betaId`, `position=0`. **Assert:** result `parentId=betaId`; `xmind_get_subtree(betaId)` shows Gamma as child. (Root children now `[Alpha, Beta]`.)
27. `xmind_reorder_children` → parent `rootId`, `ordered_ids=[betaId, alphaId]` (must list every attached child exactly once). **Assert** via `xmind_get_subtree`: root children now `[Beta, Alpha]`.
28. `xmind_duplicate_topic` → `topic_id=alphaId`, `target_parent_id=rootId`. Capture `newRootId`, `copiedCount`. **Assert:** `newRootId != alphaId`; `xmind_get_subtree` shows a second Alpha subtree with fresh IDs.
29. `xmind_add_floating_topic` → `sheetMain`, `title="Floater"`. **Assert** via `xmind_get_topic_properties(rootId)`: `childCounts.detached >= 1`.
30. `xmind_delete_topic` → `newRootId` (the duplicate). **Assert** via `xmind_get_subtree`: the duplicated subtree is gone; original `Alpha` remains.

### Phase 6 — Relationships (sheet-level)
31. `xmind_add_relationship` → `from_id=alphaId`, `to_id=betaId`, `label="rel"`.
32. `xmind_list_relationships` → `sheetMain`. Capture `relId`. **Assert:** count = 1; `end1Id=alphaId`, `end2Id=betaId`, title `"rel"`.
33. `xmind_get_topic_properties` → `alphaId`. **Assert:** `relationships` includes `relId` (confirms sheet-level storage is surfaced per-topic).
34. `xmind_delete_relationship` → `relId`. **Assert** via `xmind_list_relationships`: count = 0.

### Phase 7 — Summaries & boundaries
35. `xmind_add_summary` → parent `alphaId`, `from_index=0`, `to_index=1`, `title="Sum"` (spans `A1`,`A2`). **Assert** via `xmind_get_topic_properties(alphaId)`: `summaryCount >= 1` **and** `childCounts.summary >= 1` — this confirms the required double-write (range descriptor + summary topic).
36. `xmind_add_boundary` → parent `alphaId`, `title="Bound"`. **Assert** via `xmind_get_topic_properties(alphaId)`: `boundaryCount >= 1`, a boundary with title `"Bound"`.

### Phase 8 — Utilities: outline export/import, find & replace
37. `xmind_flatten_to_outline` → `sheetMain`, `format="markdown"`, `include_notes=true`. **Assert:** Markdown text listing the topic titles.
38. `xmind_flatten_to_outline` → `topic_id=alphaId`, `format="text"`. **Assert:** indented plain-text outline of Alpha's subtree.
39. `xmind_import_from_outline` → `outline="# Imported\n- Child 1\n- Child 2"`, omit `sheet_id` (creates a new sheet). **Assert** via `xmind_list_sheets`: a new sheet was added.
40. `xmind_import_from_outline` → `outline="- Extra A\n- Extra B"`, `sheet_id=sheetMain`, `parent_id=rootId`. **Assert** via `xmind_get_subtree`: `Extra A` and `Extra B` now under root.
41. `xmind_find_and_replace` → `sheet_id=sheetMain`, `find="Alpha"`, `replace="Alfa"`. **Assert** via `xmind_search_topics`: `Alfa` present, `Alpha` no longer matches.
42. `xmind_find_and_replace` → omit `sheet_id` (all sheets), `find="Beta"`, `replace="Beta!"`, `exact_match=true`. **Assert** via `xmind_search_topics`: exact-titled `Beta` renamed.

### Phase 9 — Error paths (must return tool errors, not crashes)
For each, PASS = a recoverable **tool error** with a readable message; FAIL = protocol error/crash, a misleading message, or unexpected success.

E1. `xmind_open_map` → a path that does not exist. **Expect:** tool error (file not found).
E2. `xmind_create_map` → the existing scratch path. **Expect:** tool error ("file already exists").
E3. `xmind_get_topic_properties` → `sheetMain`, `topic_id="does-not-exist"`. **Expect:** tool error ("topic not found").
E4. `xmind_delete_topic` → `sheetMain`, `topic_id=rootId`. **Expect:** tool error (cannot delete the sheet root).
E5. `xmind_reorder_children` → `rootId` with an `ordered_ids` list that keeps the same length but replaces one real child id with a fake id (e.g. `["fake-id", alphaId]`). **Expect:** tool error ("ordered_ids contains unknown id"). Note: the handler checks list length first, so a list of the *wrong length* instead returns a length-mismatch error — also a valid tool error, but a different message.
E6. `xmind_move_topic` → move `alphaId` under one of its own descendants (e.g. `A1`). **Expect:** tool error (cannot move a topic into its own subtree).
E7. Create the **error-scenario file** with `xmind_create_map` (single sheet). Then `xmind_delete_sheet` on its only sheet. **Expect:** tool error ("cannot delete the last sheet").

## Cleanup

After the run, remove the scratch and error-scenario files so the next run starts clean. There is no MCP "delete map" tool, so:
- If a filesystem/shell tool is available, delete both files.
- If not, **report their full paths** and ask the operator to delete them (a leftover file will make the next `xmind_create_map` fail).

## Reporting

Produce a **per-tool PASS/FAIL report** suitable for release sign-off. Structure:

1. **Header:** scratch path used, date, and the xmind-mcp version if exposed by the MCP server handshake (no tool returns a version — `xmind_open_map` returns only `path`, `sheetCount`, `sheets`).
2. **Coverage table** — one row per tool, with PASS/FAIL and a one-line observed result:

   | Tool | Result | Notes |
   |---|---|---|
   | `xmind_create_map` | PASS | created scratch, sheet id captured |
   | ... | ... | ... |

   Every one of the 27 tools must appear. Mark any not reached as **NOT RUN** with the reason.
3. **Error-path table** — E1–E7 with PASS/FAIL and the actual message returned.
4. **Final verdict:** `RELEASE OK` only if every tool ran and every assertion + error path passed. Otherwise `RELEASE BLOCKED` with the failing items listed first.

State failures plainly with the exact tool call and the observed vs. expected result. Do not soften or omit a failure. If a step could not run because an earlier step failed, mark it BLOCKED and say which step blocked it.

## Anti-patterns

- Do not mutate the file the instance was pointed at — always work on the self-created scratch file.
- Do not treat a successful write call as a pass on its own; the read-back assertion is what counts.
- Do not skip tools to save time — release validation requires the full surface. A skipped tool is `NOT RUN`, never an implied PASS.
- Do not report `RELEASE OK` with any `NOT RUN`, `BLOCKED`, or `FAIL` rows.
- Do not invent tool arguments; use only the documented parameters for each `xmind_*` tool.
