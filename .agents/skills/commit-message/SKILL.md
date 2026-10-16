---
name: commit-message
description: Generates Git commit messages in the Chris Beams style from staged or unstaged diffs, with imperative subjects of 50 characters or fewer, a body that explains what and why, issue footers when applicable, and ASCII-only output. Use when invoked via /commit-message, or when the user asks for a commit message, help committing, or a summary of changes for a commit.
---

# Commit Message

Read the project's agent instructions (e.g. AGENTS.md), including any nested AGENTS.md in directories the change touches, before applying this skill. Project commit conventions override the defaults below.

## Pre-flight

Before generating a message:

1. **Check for failures:** If the project has a build/test/lint workflow, confirm it passes or the user has acknowledged failures. Do not generate a commit message for code that does not build.
2. **Determine scope:** If both staged and unstaged changes exist, ask the user which set the message should cover. Do not silently choose.
3. **Check what belongs in the commit:**
   - If the project tracks generated files (lockfiles, generated code, schema dumps, generated docs), they are committed together with the source change that produced them. Generated files the project ignores stay out.
   - Build output, coverage reports, local environment files, and secrets never belong in a commit. If any are in the diff, flag them before generating a message.

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
- Never use smart punctuation (e.g. curly quotes, em dashes, the ellipsis character). Use straight apostrophe (`'`), straight quotes (`"`), hyphens (`-`), and three periods (`...`) only.
- Self-check for non-ASCII before returning. Replace any with ASCII equivalents.

## Required format

```
<Subject line>

<Body (optional)>

Closes #<ID> (only if an ID is provided)
```

### Subject line

- **Imperative mood, capitalized, no trailing period.**
- No `area:` or `type:` prefix (e.g. `api: ...`, `feat: ...`) unless the project's conventions use one.
- Keep it to 50 characters or fewer. Needing more usually means the subject is vague or the commit does too much (see Multi-concern diffs). 72 is the hard limit.
- Must pass: prepend "If applied, this commit will " to the subject; the result must read as natural English **and** say what actually changes. "Improve the parser" passes the grammar test and says nothing.

Examples:

- `Add retry limit to export job`
- `Fix off-by-one in pagination cursor`
- `Update logging library to v2`

Counterexamples (do not do this):

- `Added retry limit to export job` (past tense)
- `fix pagination bug` (not capitalized)
- `export: Add retry limit` (area prefix, when the project does not use them)
- `Update logging library to v2.` (trailing period)

### Body (optional; what and why, not how)

- **Omit** if the diff is truly trivial (e.g. typo, formatting, comment-only) **and** there is no meaningful motivation to record.
- Wrap at 72 characters.
- Explain what was wrong or missing, why this change fixes it, and any consequences a reader would not see in the diff (side effects, limitations, follow-up work). Leave *how* to the diff.
- When a change alters what MCP clients see -- a tool's arguments, response fields, annotations, or error behavior -- say so, and say whether existing clients are affected.
- Write prose paragraphs separated by blank lines. Do not restate the subject. Opening with "This commit ..." is fine when it reads naturally.
- Bullets (`-`) are fine for a list of distinct reasons or effects. Never use them to list the edits file by file or step by step; the diff already shows that. Sub-bullets: indent 2 spaces. Wrapped continuation: 2 additional spaces.
- Do not add labeled sections such as `Features:`, `Changes:`, or `Fixes:`. They turn the body into a changelog, and `Fixes:` can be mistaken for an issue trailer.
- Reference files, modules, functions, or symbols in `backticks` when it helps the reader.
- Quantify impact only when supported by the diff or context (e.g. lines removed, build time).

### Trailers

- Do not add Co-Authored-By, Signed-off-by, or other trailers unilaterally. Include only when the user or environment explicitly requests them.
- Place trailers after a blank line following the body (or subject if no body).
- Issue references are trailers too (e.g. `Closes #123`). They go at the bottom, never in the body.
- ASCII-only rule applies to trailers.

### Contextual logic

- Infer the primary intent from the diff (e.g. "Extract helper", "Replace duplicated logic", "Standardize scripts").
- **Do not invent** issue IDs, performance numbers, or behavior claims not evident from the diff or context.
- If no issue ID is provided, **omit** the footer entirely.

### Multi-concern diffs

When a diff spans multiple unrelated concerns (e.g. a refactor + a new feature + a typo fix):

1. **Flag it:** Tell the user the diff contains distinct concerns and suggest splitting into separate commits.
2. **If the user wants one commit:** Use the primary change as the subject. Give each secondary concern its own short paragraph in the body explaining what and why.
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
Extract date parsing into shared helper

Three call sites parsed date strings with slightly different
rules, so the same input could be accepted in one place and
rejected in another. This commit moves parsing into one helper
so all callers behave the same way.

The helper uses the strictest of the existing rules. Inputs that
only the most lenient call site accepted, such as two-digit
years, are now rejected everywhere.
```

**With bullets and an issue reference:**

```
Add retry limit to export job

Failed exports were retried indefinitely, so a permanent error
kept a worker busy forever. This commit caps retries and marks
the job failed once the limit is reached.

This has two visible effects:

- A job that hits the limit now shows as failed instead of
  staying in progress forever
- Workers stuck on permanent errors are freed for other jobs

Closes #123
```

## Related skills

- Run `/verify-changes` before generating a commit message to ensure the build passes.
