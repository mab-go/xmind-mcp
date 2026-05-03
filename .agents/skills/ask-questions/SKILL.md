---
name: ask-questions
description: Gathers structured input from the user through targeted questions with discrete options. Use when invoked via /ask-questions, or when the user says phrases like "ask me questions", "clarify first", "gather requirements", "make sure you have context", or "what do you need to know".
---

# Ask Questions

Read the project's agent instructions (e.g. AGENTS.md) before applying this skill to ensure conventions are followed.

## When to apply

- User explicitly invokes `/ask-questions`
- User asks you to clarify, gather requirements, or check assumptions before proceeding
- You face unknowns that would significantly change your approach

Do **not** use this skill for:
- Single clarification questions mid-task (just ask inline)
- Things you can determine by reading project docs, code, or git history
- Implementation details with a single obvious answer given established patterns
- Whether to run tests, lint, or verify (always do it)

## Procedure

1. **Identify the topic** from the slash command argument (e.g. `/ask-questions about the deployment strategy`) or surrounding context.
2. **Categorize your unknowns** using the tiers below to prioritize what to ask.
3. **Present structured questions** with discrete options. Use a structured-question tool (AskQuestion, AskUserQuestion) if available; otherwise present numbered text options the user can reply to by number.
4. **Ask all questions at once** unless a later question depends on an earlier answer. Batch to minimize round trips.
5. **Proceed after answers:** take the most appropriate next action (implement, plan, explore). Only pause if you genuinely cannot determine what to do next.

## Question categories

Prioritize questions by how much they change your approach:

**Tier 1 -- Blocking unknowns:** Choices that change the implementation approach entirely. Ask these first.
- "Should this be a new handler or an extension of the existing one?"
- "REST API or GraphQL?"

**Tier 2 -- Preference unknowns:** Either option works, but the user has an opinion. Ask after Tier 1.
- "Return plain text or structured JSON?"
- "Single PR or split by concern?"

**Tier 3 -- Confirmable assumptions:** Things you believe are true based on code/docs, but want to verify. Ask last, or skip if confidence is high.
- "I'll follow the existing error handling pattern -- correct?"
- "This should go in the utils package based on the existing structure -- right?"

## Question design

- Match option count to the actual decision space. Binary questions get 2 options -- do not inflate to 3-5 artificially.
- For genuinely open-ended option spaces, include an escape hatch ("Other" or "Agent decides").
- Allow multiple selections when choices can co-exist (e.g. "Which concerns apply?").
- One sentence per question. Front-load the decision being made.

## After receiving answers

- If answers contradict established project conventions, conventions win. Explain why and ask the user to confirm they want to override.
- If the user says "you decide" or "just do it," proceed with your best judgment and state the assumptions you are making.
- Do not re-ask questions the user has already answered, even indirectly.

## Related skills

- After gathering answers, chain into `/make-todos` to decompose the work or `/explore-codebase` to investigate the relevant code.
