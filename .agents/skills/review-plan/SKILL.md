---
name: review-plan
description: Review implementation plans for accuracy, correctness, and clarity, including any associated task items. Use when reviewing a plan before implementation or when the user asks for plan review.
---

# Plan Review

Read the project's agent instructions (e.g. AGENTS.md) before applying this skill to ensure conventions are followed.

## When to apply

- User asks to review a plan before implementation
- User invokes `/review-plan`
- After producing or updating a plan and wanting a self-check

## Review directive

The agent implementing this plan may not be you -- it may be an agent without your context. The plan must be rock-solid: accurate, unambiguous, and complete enough to execute without guessing. But do not over-explain for its own sake. Assume a capable implementer.

## Verification procedures

For each category, here is **how** to verify -- not just what to check.

### Accuracy

- **File paths:** Run `ls` or `find` to confirm referenced files exist.
- **Function/type names:** Grep the codebase to confirm they exist and have the expected signature.
- **API behavior claims:** Read the relevant source to verify.
- **Dependency claims:** Check the dependency manifest (e.g. `go.mod`, `package.json`).

### Correctness

- **Execution simulation:** For each step, mentally simulate: does the preceding step produce what this step needs as input?
- **Contradictions:** Does step N undo or conflict with step M?
- **Missing steps:** Would the plan leave the project in a broken state if stopped after any given step? (e.g. a tool defined but not registered, a struct field added but not in the codec allowlist)

### Clarity

- **Different-agent test:** Could someone without the surrounding conversation context follow each step unambiguously?
- **Vague language:** Flag uses of "appropriate," "relevant," "as needed," "properly" that lack specifics about what they mean concretely.

### Completeness

- Does the plan include a verification step (build/test/lint)?
- Does it include documentation or config updates if structural changes are made?
- Does it follow established patterns visible in the codebase?

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

**Incomplete:** "Update the handler" -- which handler? What change? Better: "Add validation for the `title` argument in `CreateTopicHandler`"

**Composite:** "Add tool and write tests" -- split into "Define tool in tools.go" and "Add handler tests"

**Ambiguous:** "Clean up the code" -- no completion criterion. Better: "Extract duplicate path-validation into a shared helper"

**Trivial:** "Import the uuid package" -- this happens automatically when writing the code that uses it. Remove.

**Misordered:** "Write handler tests" before "Implement handler" -- swap.

## Output

- Summarize what you verified and list findings with severity levels.
- For errors, vagueness, or ambiguity: state the issue clearly, suggest a concrete fix. **Do not edit the plan yet.**
- If required context is missing, ask targeted clarifying questions before proposing fixes.
- If the plan is solid, say so briefly. No need to repeat it.

## Applying changes (only after user approval)

Edit the plan document in-place. If the plan needs a fundamentally different approach (not just fixes), propose a replacement rather than a series of edits that would be harder to review. Proceed only after the user explicitly approves.

## Related skills

- After plan approval and implementation, run `/verify-changes` to confirm the build passes.
