---
name: make-todos
description: Creates a structured TodoWrite task list for the current work. Use when the user invokes /make-todos, or when the task involves multiple steps where later steps depend on earlier ones completing first.
---

# Make Todos

Invoke the `TodoWrite` tool immediately. Do not describe what you are about to do; just call the tool.

## When to apply

- User explicitly invokes `/make-todos`
- The work has 3+ steps where later steps depend on earlier ones completing first

Do **not** create todos for single-step tasks, trivial changes, or purely conversational requests.

## Task decomposition guidelines

- Each todo should represent one focused, completable unit of work; not a whole feature, not a single line change.
- Order todos so earlier ones unblock later ones.
- Name todos as actions: "Add X", "Update Y", "Fix Z" - not nouns like "Error handling".
- If the full scope is unclear, create todos for what is known and add more as the work unfolds.

## Behavior after creating todos

1. Mark the first todo `in_progress`.
2. Start working on it in the same response - do not pause to announce the plan.
3. Mark each todo `completed` immediately when done, then mark the next todo `in_progress`.
4. Only one todo should be `in_progress` at a time.
