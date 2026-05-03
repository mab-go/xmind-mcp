---
name: commit-message
description: Generates Git commit messages from staged or unstaged diffs using imperative subjects, optional scannable body, issue footers when applicable, and ASCII-only output. Use when the user asks for a commit message, help committing, or summarizing changes for a commit.
---

# Commit Message

Read the project's agent instructions (e.g. AGENTS.md) before applying this skill to ensure conventions are followed.

## When to apply

- User asks for a commit message, help committing, or summarizing changes
- User invokes `/commit-message`

## Pre-flight

Before generating a message:

1. **Check for failures:** If the project has a build/test/lint workflow, confirm it passes or the user has acknowledged failures. Do not generate a commit message for code that does not build.
2. **Determine scope:** If both staged and unstaged changes exist, ask the user which set the message should cover. Do not silently choose.

## Input (in order of preference)

1. If there are **staged changes** (`git diff --staged`): produce the message for the commit the user is about to make.
2. If nothing is staged but there are **unstaged changes** (`git diff`): produce a proposed message for those working tree changes.
3. If there are **no local changes**: ask whether to summarize the most recent commit (`git show -1 --stat` and `git show -1 -p`) for reference/amend use.

## Output

- Return the full commit message inside a **single** Markdown code fence.
- This skill generates the message text only; it does not execute `git commit`. If the user explicitly says "commit this" or "make the commit," execute the commit with the generated message.
- If anything needed to infer intent is missing, ask before generating.

## Character policy

- Use **ASCII only** in the commit message.
- Never use smart punctuation (e.g. curly quotes, em dashes). Use straight apostrophe (`'`), straight quotes (`"`), and hyphens (`-`) only.
- Self-check for non-ASCII before returning. Replace any with ASCII equivalents.

## Required format

```
<Subject line>

<Body (optional)>

Closes #<ID> (only if an ID is provided)
```

### Subject line

- **Imperative mood, capitalized, no trailing period.**
- Aim for ~50 characters (hard max 72).
- Must pass: prepend "If applied, this commit will " to the subject; the result must read as natural English.

Examples:

- `Add runtime startup validation`
- `Fix WebSocket reconnection logic`
- `Update Ant Design to v6`

Counterexamples (do not do this):

- `Added runtime startup validation` (past tense)
- `fix websocket bug` (not capitalized)
- `Update Ant Design to v6.` (trailing period)

### Body (optional; prefer WHY over HOW)

- **Omit** if the diff is truly trivial (e.g. typo, formatting, comment-only) **and** there is no meaningful motivation to record.
- Wrap at 72 characters.
- Start with 1-2 sentences explaining motivation or impact (avoid restating the subject). "This commit ..." is encouraged if natural, but not required.
- Add labeled sections for scannability when helpful:

  ```
  Features:
  - ...
  Changes:
  - ...
  Fixes:
  - ...
  ```

- Use `-` bullets. Sub-bullets: indent 2 spaces. Wrapped continuation: 2 additional spaces.
- Reference files, modules, functions, or symbols in `backticks`.
- Quantify impact when supported by the diff or context (e.g. lines removed, build time).

### Trailers

- Do not add Co-Authored-By, Signed-off-by, or other trailers unilaterally. Include only when the user or environment explicitly requests them.
- Place trailers after a blank line following the body (or subject if no body).
- ASCII-only rule applies to trailers.

### Contextual logic

- Infer the primary intent from the diff (e.g. "Extract hook", "Replace duplicated logic", "Standardize scripts").
- **Do not invent** issue IDs, performance numbers, or behavior claims not evident from the diff or context.
- If no issue ID is provided, **omit** the footer entirely.

### Multi-concern diffs

When a diff spans multiple unrelated concerns (e.g. a refactor + a new feature + a typo fix):

1. **Flag it:** Tell the user the diff contains distinct concerns and suggest splitting into separate commits.
2. **If the user wants one commit:** Use the primary change as the subject. Use labeled sections in the body to cover secondary concerns.
3. **If the user wants to split:** Suggest which files or hunks belong to which commit.

## Anti-patterns

- Do not generate a message and then immediately run `git commit` unless the user explicitly asked you to commit.
- Do not invent issue IDs or claim behavior changes not supported by the diff.
- Do not add trailers (Co-Authored-By, etc.) unless explicitly requested.

## Examples

**Simple (no body):**

```
Fix typo in README
```

**With body:**

```
Extract WebSocket connection logic into hook

This commit moves WebSocket connection management from the
component into a reusable hook. This reduces duplication
across three components and makes testing easier.

- Extract `useWebSocket` hook from `ChatComponent`
- Update `SessionComponent` and `BuilderComponent` to use hook
- Add unit tests for hook behavior
```

**With issue reference:**

```
Add runtime startup validation

This commit adds validation to ensure the runtime container
starts correctly before accepting connections. Prevents
cascading failures when the runtime is misconfigured.

Fixes:
- Runtime crashes on invalid environment variables
- Silent failures when Docker image is missing

Closes #123
```

## Related skills

- Run `/verify-changes` before generating a commit message to ensure the build passes.
