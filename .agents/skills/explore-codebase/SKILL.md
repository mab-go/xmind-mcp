---
name: explore-codebase
description: Systematically explores the codebase before implementing changes. Use at the start of implementation tasks, when unfamiliar with the code, or when the task spans multiple modules or layers.
---

# Explore Codebase

## When to apply

- At the start of any non-trivial implementation task
- When unfamiliar with the codebase or the area of code being changed
- When the task spans multiple modules, packages, or layers
- User invokes `/explore-codebase`

Do **not** use this for:
- Trivial changes where the target file and pattern are already known
- Follow-up tasks in an area you have already explored in this conversation

## Procedure

1. **Read agent instructions.** Read the project's agent instructions (AGENTS.md or equivalent) in full. Pay attention to:
   - Project structure and layer dependencies
   - Conventions and patterns to follow
   - Critical notes or gotchas (e.g. codec allowlists, dual-write requirements)
   - Required verification steps

2. **Identify affected areas.** Based on the task, determine which layers, packages, or modules will be touched. Map the task to the project's architecture.

3. **Read existing code.** In the affected areas, read:
   - Existing implementations similar to what you are about to build (use as templates)
   - Shared utilities and helpers that should be reused rather than reimplemented
   - Test files to understand testing patterns and available fixtures

4. **Find patterns.** Grep for similar constructs:
   - If adding a new tool/handler/endpoint, find an existing one to follow
   - If adding a new type or field, find how existing ones are declared and registered
   - If writing tests, find how existing tests set up fixtures and assertions

5. **List your findings.** Before starting implementation, state:
   - Files that will need changes
   - Existing patterns each file should follow
   - Utilities or helpers to reuse
   - Anything surprising or noteworthy from the exploration

## Anti-patterns

- Do not start implementing without reading existing patterns first. Copy-then-modify beats writing from scratch.
- Do not assume patterns from other projects apply here. Read what this project actually does.
- Do not read every file in the repo. Focus on what is relevant to the task.
- Do not spend so long exploring that you never start implementing. This is reconnaissance, not research.

## Related skills

- Do this before `/make-todos` to ensure task decomposition reflects the actual code structure.
- If exploration reveals ambiguity about requirements, chain into `/ask-questions`.
