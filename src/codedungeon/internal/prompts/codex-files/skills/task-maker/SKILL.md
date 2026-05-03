---
name: task-maker
description: Shape unclear implementation requests into a minimal design and a ready-to-run English `$codedungeon --full` prompt.
---

# task-maker

Use this skill when the user asks to shape a task, make a run-full prompt, clarify an implementation request, or prepare work before `$codedungeon --full`.

Task Maker is a pre-run helper. It does not replace planning, task splitting, QA, review, PR gates, or finalization inside CodeDungeon.

## Conversation

- Stay in the user's language while clarifying.
- Keep the exchange short and ask one material question per turn.
- Ask only about choices that materially change the final prompt: goal, scope, constraints, success criteria, verification, repo/context, and explicit exclusions.
- If ambiguity is minor, record an assumption instead of blocking.
- Do not produce large tables, exhaustive specs, or hidden chain-of-thought requests.

## Confirmation

When the design is clear, summarize the minimal design briefly and ask the user to confirm before rendering.

After confirmation, the final output is always in English. Write `.codedungeon/task-maker/sessions/<timestamp-slug>/request.json` with these fields:

```json
{
  "title": "",
  "goal": "",
  "current_state": "",
  "target_outcome": "",
  "in_scope": "",
  "out_of_scope": "",
  "constraints": "",
  "success_criteria": "",
  "verification": "",
  "assumptions": ""
}
```

The required fields are `goal`, `target_outcome`, and `success_criteria`.

Render and persist the confirmed output with the project-local CLI:

```bash
./.codex/bin/codedungeon task-maker render --input .codedungeon/task-maker/sessions/<timestamp-slug>/request.json --out .codedungeon/task-maker/sessions/<timestamp-slug> --print
```

Use `codedungeon task-maker render` only after the user confirms the design.

## Output Contract

The rendered response must have this shape:

```md
# Task Maker Output

## Minimal Design
Goal:
Current State:
Target Outcome:
In Scope:
Out of Scope:
Constraints:
Success Criteria:
Verification:
Assumptions:

## Run Full Prompt
<single polished English prompt, concise but complete>

## Command
$codedungeon --full "<prompt>"
```

The run-full prompt should lead with the desired outcome, include acceptance criteria and verification expectations, state explicit constraints and exclusions, avoid unnecessary process micromanagement, and say when to ask for clarification versus proceed with assumptions.

The skill must not start `$codedungeon --full` automatically. Only run it if the user explicitly asks after reviewing the prompt.
