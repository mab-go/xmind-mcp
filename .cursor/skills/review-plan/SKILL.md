---
name: review-plan
description: Review plans from Cursor Plan mode for accuracy, correctness, and clarity. Use when reviewing a plan before implementation or when the user asks for plan review.
---

# Plan Review

When reviewing a plan (e.g. from Cursor's Plan mode), apply the following review.

## Review directive

Take another look at the plan. Double-check it for accuracy and correctness. Are there any areas where you're being vague or ambiguous? Are there places where you're saying something outright incorrect?

It's important that you be sure, because the agent implementing this plan may not be you: it may be an agent with a lower skill level than your own, so you must make sure the plan is rock-solid. Don't spell things out like you're addressing an unintelligent coding agent, though.

Strike a balance between completeness/correctness and being overly prescriptive or verbose.

## What to check

- **Accuracy**: Facts, file paths, API names, and technical claims are correct.
- **Correctness**: Steps are logically sound and will achieve the stated goal; no contradictions or impossible sequences.
- **Clarity**: No vague or ambiguous wording; a different agent could follow the plan without guessing.
- **Actionability**: Each step is concrete enough to implement; "consider X" is only used where real alternatives exist.
- **Unknowns/Assumptions**: Identify missing context that prevents a confident review and flag assumptions that need confirmation.
- **Brevity**: No unnecessary detail or condescending over-explanation; assume a capable implementer.

## Output

- Summarize what you verified and any issues found.
- If you find errors, vagueness, or ambiguity, state them clearly and suggest concrete fixes. **Do not edit the plan yet.** Present findings to the user and offer to apply the changes.
- If required context is missing, ask targeted clarifying question(s) before proposing final fixes so recommendations are grounded.
- If the plan is solid, say so briefly; no need to repeat the plan.

## Applying changes (only after user approval)

- Prefer **editing the plan document in-place**. If a substantial portion of the plan would change (e.g. \>40%), you may create a new document instead of updating in place.
- Proceed with updates only after the user explicitly approves.
