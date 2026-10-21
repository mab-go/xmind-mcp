---
name: review-plan
description: Reviews implementation plans, including any associated task items, for accuracy, correctness, clarity, and completeness. Use when invoked via /review-plan, when the user asks for a plan review, before implementing a plan or handing it to another agent, or as a self-check after producing or updating a plan.
---

# Plan Review

Read the project's agent instructions (e.g. AGENTS.md), plus any nested AGENTS.md in directories the plan touches, before applying this skill.

## Review directive

The agent implementing this plan may not be you -- it may be an agent without your context. The plan must be rock-solid: accurate, unambiguous, and complete enough to execute without guessing. But do not over-explain for its own sake. Assume a capable implementer.

## Verification procedures

For each category, here is **how** to verify -- not just what to check.

### Accuracy

- **File paths:** Run `ls` or `find` to confirm referenced files exist.
- **Names:** Confirm every function, type, config key, and build or script target the plan names exists and has the expected signature or definition. Use LSP (gopls) for Go symbols and grep for everything else.
- **API behavior claims:** Read the relevant source to verify.
- **Dependency claims:** Check the dependency manifest (e.g. `go.mod`, `package.json`).

### Correctness

- **Execution simulation:** For each step, mentally simulate: does the preceding step produce what this step needs as input?
- **Contradictions:** Does step N undo or conflict with step M?
- **Broken intermediate states:** Would the plan leave the project broken if stopped after any given step? Common cases:
  - Something defined but never registered or wired in where the project discovers it
  - A new entry of a kind the project lists in several places (registries, docs, config) that misses one of them
  - A check, test, or tool that hard-codes a list the change extends, so it silently skips the new entry
  - A new dependency direction that the project's architecture rules or linters forbid
  - New permissions or credentials that lack access to something they need indirectly
  - Generated artifacts not regenerated after their source changes
  - In this project, also check for:
    - A new tool not registered in `internal/server/server.go`, or missing from `expectedAnnotations` in `internal/server/tools_test.go` or `contractCases` in `internal/server/error_contract_test.go`
    - A new JSON key on a tree type missing from its allowlist in `internal/xmind/json_codec.go`, so it is kept as opaque data instead of populating the typed field
    - A new idempotent tool missing from `idempotentToolCases` in `internal/server/handler/mutate_test.go`; the idempotency tests skip it without failing
    - A new or renamed tool missing from the Tools section of `README.md`, the `xmind-mcp-smoke-test` skill, or the example map in `cmd/gen-example` (all three state the tool count; regenerate the map with `make gen-example`)
    - A tool whose handler returns `jsonResult` without `handler.OutputSchema[...]`, missing from `jsonOutputTools` in `internal/server/output_schema_test.go`, or without a `TestStructuredOutputMatchesSchema` case for each success path
    - A whole-number parameter declared with `mcp.WithNumber` instead of `mcp.WithInteger(..., mcp.Min(0))`, or missing from `nonNegativeIntParams` in `internal/server/tools_test.go`
    - A new, renamed, or removed tool, or a workflow change, that leaves `serverInstructions` in `internal/server/instructions.go` stale (it may name only registered tools and must stay within 150 words)
    - An import between `internal/xmind` and `internal/server/handler` (see Layer Dependencies in AGENTS.md)
- **Invariants:** If the project's agent instructions declare invariants (things that must never happen), any step that would break one is a blocker. These are always blockers:
  - Editing a migration that has already been applied
  - Weakening a test, constraint, or lint rule to make something pass
  - Removing, stubbing, or disabling existing behavior to clear an error without saying so explicitly
  - In this project, breaking a rule AGENTS.md declares in Fields to never modify, Topic IDs, the temp-then-swap write (Read/Write Lifecycle), or the no-write rule for idempotent tools (Argument extraction)

### Clarity

- **Different-agent test:** Could someone without the surrounding conversation context follow each step unambiguously?
- **Vague language:** Flag uses of "appropriate," "relevant," "as needed," "properly" that lack specifics about what they mean concretely.
- **Actionability:** Each step should be concrete enough to implement. Reserve "consider X" for places where real alternatives exist, and say what decides between them.

### Completeness

- Does the plan include a verification step (build/test/lint)?
- Does it include documentation or config updates if structural changes are made?
- Does it follow established patterns visible in the codebase?
- **Unknowns and assumptions:** Does the plan rely on anything it has not confirmed? Flag assumptions that need checking, and missing context that prevents a confident review.

## Severity levels

Classify each finding:

- **Blocker:** Plan will fail or produce incorrect results if executed as-is. (e.g. wrong file path, missing step that breaks the build)
- **Warning:** Plan will work but has a gap that could cause problems. (e.g. missing test step, vague wording an agent might misinterpret)
- **Suggestion:** Plan is correct but could be improved. (e.g. reordering for efficiency, noting an edge case)

## Task item review

If the plan has associated task items (YAML frontmatter todos, a checklist, or structured tasks), review each item for:

- **Incomplete:** Lacks enough detail to implement unambiguously. A capable agent should not need to make assumptions.
- **Composite:** Covers multiple distinct changes that could succeed or fail independently. Should be split.
- **Ambiguous:** No clear completion criterion. You cannot tell when it is done.
- **Trivial:** Will be completed automatically as a side-effect of another item, or is so self-evident it adds no value.
- **Misordered:** Sequenced in a way that creates a blocker (a step depends on a later step).
- **Coverage gap:** A significant section of the plan body has no matching task item.

### Task item examples

**Incomplete:** "Update the parser" -- which parser? What change? Better: "Reject empty `name` values in `parseConfig`"

**Composite:** "Add the CSV exporter and write its tests" -- split into "Implement `CsvExporter`" and "Add tests for `CsvExporter`"

**Ambiguous:** "Clean up the code" -- no completion criterion. Better: "Extract duplicate path-validation into a shared helper"

**Trivial:** "Import the uuid package" -- this happens automatically when writing the code that uses it. Remove.

**Misordered:** "Add tests for `CsvExporter`" before "Implement `CsvExporter`" -- swap.

## Output

- Summarize what you verified and list findings with severity levels.
- For errors, vagueness, or ambiguity: state the issue clearly, suggest a concrete fix. **Do not edit the plan yet.**
- If required context is missing, ask targeted clarifying questions before proposing fixes.
- If the plan is solid, say so briefly. No need to repeat it.

## Applying changes (only after user approval)

Edit the plan document in-place. If the plan needs a fundamentally different approach (not just fixes), propose a replacement rather than a series of edits that would be harder to review. Proceed only after the user explicitly approves.

## Related skills

- After plan approval and implementation, run `/verify-changes` to confirm the build passes.
