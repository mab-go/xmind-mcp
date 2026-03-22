---
name: ask-questions
description: Forces the agent to use the AskQuestion tool to gather structured multiple-choice input from the user. Use when invoked via /ask-questions, or when the user says phrases like "ask me questions", "clarify first", "gather requirements", "make sure you have context", or "what do you need to know".
---

# Ask Questions

When this skill is triggered, you MUST use the `AskQuestion` tool to gather structured input. Do not ask questions conversationally - always invoke the tool.

## Workflow

1. **Identify the topic**: From the slash command argument (e.g. `/ask-questions about the deployment strategy`) or surrounding context.
2. **Draft questions**: Identify the unknowns that would most change your approach; focus on high-signal decisions.
3. **Invoke AskQuestion**: Ask all questions in a single call unless a later question depends on the answer to an earlier one.
4. **Proceed contextually**: After receiving answers, take the most appropriate next action (implement, plan, summarize, etc.); only pause to ask permission if you have no basis to determine what to do next.

## Question design

- Prefer 3–5 options per question; cover the realistic space without being exhaustive. Never make up options just to reach the 3-5 guidance, however.
- Include an "Agent decides" or "Other" escape hatch only when the option space is genuinely open-ended.
- Set `allow_multiple` to `true` for questions where several answers can co-exist (e.g. "Which concerns apply?", "Which features do you want?").
- Keep question text concise; one sentence per prompt.
