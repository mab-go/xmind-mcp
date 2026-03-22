---
name: commit-message
description: Generates Git commit messages from staged or unstaged diffs using imperative subjects, optional scannable body, issue footers when applicable, and ASCII-only output. Use when the user asks for a commit message, help committing, or summarizing changes for a commit.
---

# Commit message

## When generating a message

**Input (in order of preference):**

1. If there are **staged changes** (`git diff --staged`): produce the message for the commit the user is about to make.
2. If nothing is staged but there are **unstaged changes** (`git diff`): produce a proposed message for those working tree changes.
3. If there are **no local changes**: ask whether to summarize the most recent commit (`git show -1 --stat` and `git show -1 -p`) for reference/amend use.

**Output (mandatory):** Return the full commit message inside a **single** Markdown code fence.

**Character policy (mandatory):**

- Use **ASCII only** in the commit message.
- Never use smart punctuation (for example: '’, ‘, “, ”'). Use straight apostrophe (`'`) and straight quotes (`"`) only.
- Before returning, self-check that the message contains only ASCII.
- If any non-ASCII character is present, replace it with an ASCII equivalent before responding.

If you have any questions for the user, ask them now. If anything needed to infer intent is missing, ask before generating. Otherwise generate the commit message.

---

## Required format

```
<Subject line>

<Body (optional)>

Closes #<ID> (only if an ID is provided)
```

### Subject line

- **Imperative mood, capitalized, no trailing period.**
- Aim for ~50 characters (hard max 72).
- Must pass: prepend `If applied, this commit will ` to the subject; the result must read as natural English (classic formulation: the subject fills in the blank after that phrase).

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
- Start with 1–2 sentences explaining motivation or impact (avoid restating the subject). "This commit ..." is encouraged if it sounds natural, but is not required.
- Add labeled sections for scannability when helpful, for example:

  ```
  Features:
  - ...
  Changes:
  - ...
  Fixes:
  - ...
  ```

- Use `-` bullets. Sub-bullets: indent 2 spaces. Wrapped continuation lines: 2 additional spaces.
- Reference files, modules, functions, or symbols in `backticks`.
- Quantify impact when supported by the diff or provided context (e.g. lines removed, build time). Use a Markdown table in the body only when it clarifies.

### Contextual logic

- Infer the primary intent from the diff (e.g. "Extract hook", "Replace duplicated logic", "Standardize scripts", "Harden Docker build").
- **Do not invent** issue IDs, performance numbers, or behavior claims not evident from the diff or context.
- If no issue ID is provided, **omit** the footer entirely (for example any `Closes #123` style line).

### Examples

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
