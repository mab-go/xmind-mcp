---
name: review-plan-todos
description: Review To-Do items in a plan file's YAML frontmatter for completeness and clarity. Use after producing or updating a plan in Cursor Plan mode.
---

# To-Do Review

After producing or updating a plan in Cursor Plan mode, critically review each To-Do item in the plan file's YAML frontmatter.

## What to check

- **Incomplete**: The item lacks enough detail to implement unambiguously. A capable agent should not need to make assumptions to carry it out.
- **Composite**: The item covers multiple distinct changes that could succeed or fail independently. Split these into separate items.
- **Ambiguous**: The item has no clear completion criterion. You cannot tell, just by reading it, when it is done.
- **Trivial**: The item tracks nothing meaningful on its own - it will be completed automatically as a side-effect of another item, or is so self-evident it adds no value to the list.
- **Misordered**: Items are sequenced in a way that would cause a blocker (e.g., a step depends on a later step).

## Output

- List each problematic item, identify which category applies, and state the specific issue.
- Suggest a concrete fix (reword, split, reorder, or remove).
- If the list is already solid, say so briefly.
- **Do not edit the plan yet.** Present findings and offer to apply the changes.

## Applying changes (only after user approval)

Edit the YAML frontmatter in-place once the user approves.
