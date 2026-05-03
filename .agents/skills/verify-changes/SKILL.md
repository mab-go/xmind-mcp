---
name: verify-changes
description: Runs the project's build, test, and lint workflow and checks whether documentation needs updating. Use after completing implementation work, before committing, or when the user says "verify", "check", or "are we done".
---

# Verify Changes

Read the project's agent instructions (e.g. AGENTS.md) before applying this skill to ensure the correct verification commands and documentation requirements are known.

## When to apply

- After completing implementation work
- Before generating a commit message
- User says "verify," "check," or "are we done?"
- User invokes `/verify-changes`

## Procedure

1. **Identify the verification commands.** Check the project's agent instructions (AGENTS.md), Makefile, or CI config for the canonical build/test/lint commands. Common patterns: `make build test lint`, `npm test`, `cargo clippy && cargo test`.
2. **Run them.** Execute the full suite. Do not skip steps or run partial checks.
3. **Fix failures.** If any step fails, diagnose and fix the issue, then re-run from the top. Repeat until clean.
4. **Check documentation.** Based on what changed, determine whether project documentation needs updating:
   - New/removed files -- project structure docs
   - New utilities or helpers -- reference tables or API docs
   - New conventions established -- convention docs
   - New config or skill files -- reflect in agent instructions if they affect agent behavior
5. **Make doc updates** if needed.
6. **Re-run verification** if doc changes touched source files (e.g. doc comments in code, embedded docs).

## Anti-patterns

- Do not skip verification because "it's a small change." Small changes break builds too.
- Do not mark verification complete if there are known failures the user has not acknowledged.
- Do not run only tests without also running lint, or vice versa. Run the full suite.
- Do not silently suppress warnings or skip flaky tests without flagging them.

## Related skills

- This is a natural prerequisite for `/commit-message`.
- Include as the final item when using `/make-todos` to decompose work.
