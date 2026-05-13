# One Shot

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. Missing, draft, or stale rules are soft blockers while the agent is shaping work, but finalization must not claim READY_FOR_USER_REVIEW without the required Project Rules envelope.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

Deterministic completion gates:
- Use only `./.claude/bin/codedungeon` for CodeDungeon commands.
- Do not write review reports manually.
- Do not write final reports manually.
- Run standalone review with `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/plans/one-shot/PLAN.md --out .codedungeon/code-review --post`.
- After standalone review is approved, run verification with `./.claude/bin/codedungeon qa run --phase 6 --auto --fresh` or `./.claude/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`.
- Run `./.claude/bin/codedungeon run finalize`; READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`.

Minimal CodeDungeon workflow for a small change that still needs the safety rail of branch, commit, PR, and adversarial review.

Use when the request is narrow enough for one planner pass and one implementation pass. Do not split into task files, do not run the phase pipeline, and do not call `codedungeon-loop`.

## Parameters

- `$ARGUMENTS` - feature prompt or bugfix request.

## Required Outcome

End with:

- non-protected feature branch
- implementation committed
- branch pushed
- GitHub PR created or reused
- `codedungeon code-review` posted to the PR
- review verdict `APPROVED`
- final response in the standard CodeDungeon PR Report format

## Step 0: Validate

If `$ARGUMENTS` is empty:

```text
Usage: /one-shot <feature or bugfix request>
```

Stop.

Run:

```bash
CD=./.claude/bin/codedungeon
[ -x "$CD" ] || { echo "Status BLOCKED: run project-local codedungeon setup before /one-shot"; exit 2; }
git rev-parse --is-inside-work-tree >/dev/null
$CD run --oneshot --prompt "$ARGUMENTS"
```

If git repo validation fails, stop before editing and return a `BLOCKED` CodeDungeon PR Report. Treat missing `origin` or `gh` auth as finalization blockers reported by `codedungeon run status` / `codedungeon run finalize --dry-run`; do not stop local planning or implementation solely for GitHub readiness. Never edit, commit, or push on `main`, `master`, `develop`, `dev`, `staging`, `production`, or `release`.

## Step 1: Plan

Inspect only the files needed to understand the request. Write a short plan to:

```text
.codedungeon/plans/one-shot/PLAN.md
```

The plan must contain:

```markdown
# One Shot Plan

## Request
<original request>

## Implementation
- <small ordered implementation steps>

## Verification
- <commands or manual checks>

## Review Focus
- <risk areas for code-review>
```

If the request clearly needs multi-step decomposition, cross-repo coordination, or a full QA/report pipeline, stop and recommend `/main-quest` instead.

Record progress:

```bash
$CD run advance --step planning --status completed --summary "one-shot plan written" --artifact .codedungeon/plans/one-shot/PLAN.md
```

## Step 2: Branch

Create or switch to a feature branch:

```bash
BRANCH="feat/$(echo "$ARGUMENTS" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//' | cut -c1-50)"
[ "$BRANCH" = "feat/" ] && BRANCH="feat/one-shot"
git switch -c "$BRANCH" 2>/dev/null || git switch "$BRANCH"
$CD git guard --repo .
```

If guard fails here, stop before editing.

## Step 3: Implement

Implement directly from the plan. Keep edits scoped. Add or run the smallest meaningful tests before claiming done.

Do not create `.codedungeon/tasks/*` for this workflow.

Record progress:

```bash
$CD run advance --step execution --status completed --summary "one-shot implementation complete"
```

## Step 4: Commit, Push, PR

Run:

```bash
$CD git guard --repo .
git status --short
git add -A
git commit -m "feat: one-shot update"
git push -u origin "$(git branch --show-current)"
PR_URL=$(gh pr view --json url -q .url 2>/dev/null || true)
if [ -z "$PR_URL" ]; then
  PR_URL=$(gh pr create --fill)
fi
PR_NUMBER=$(gh pr view --json number -q .number)
echo "$PR_URL"
```

If the PR already exists, reuse it. If no PR exists, create one.
If `PR_URL` or `PR_NUMBER` is empty after this step, stop and return a `BLOCKED` CodeDungeon PR Report.

## Step 5: Review

Run `codedungeon code-review` until the PR review verdict is `APPROVED` or 9 cycles are exhausted.

Cycles:
- 1-3: full adversarial mode.
- 4-9: reduced adversarial mode. Keep all personas, but use fast model/effort and review only the fixes or new diff since the previous review cycle.

For each cycle:

```bash
REVIEW_CYCLE=<1-9>
REVIEW_MODE=<full|reduced>
$CD code-review --url "$PR_URL" --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/plans/one-shot/PLAN.md --out .codedungeon/code-review --post
```

After each review, verify that a review comment was posted to the PR:

```bash
ADV_REVIEW_COUNT=$(gh pr view "$PR_NUMBER" --comments --json comments -q '[.comments[] | select(.body | test("CodeDungeon Code Review"))] | length')
```

If `ADV_REVIEW_COUNT` is `0`, stop and return `BLOCKED`. If review returns `CHANGES_REQUESTED`, fix the findings directly, commit, push, and rerun `codedungeon code-review`. After 9 cycles, stop with `MAX_CYCLES_REACHED`.

Record progress:

```bash
$CD run advance --step code_review --status completed --summary "review approved" --artifact .codedungeon/code-review
```

## Step 6: Final Verification

After review is approved, run every verification command from the plan. If no command was obvious, run the nearest project check from the manifest:

- Go: `go test ./...`
- Rust: `cargo test`
- Node: `npm test` or package script closest to test
- Python: `python -m pytest`

If verification cannot run, record the blocker in the final report and continue only if the change can still be reviewed.

Record progress:

```bash
$CD run advance --step qa --status completed --summary "verification attempted" --artifact .codedungeon/qa
```

## Step 7: Report

Always return this exact format. `Status READY_FOR_USER_REVIEW` is valid only after `$CD run finalize` succeeds; GitHub readiness, review approval, branch push, QA, Project Rules, and final report gates are enforced there. Do not merge; the user performs final review and merge.

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        READY_FOR_USER_REVIEW|BLOCKED|MAX_CYCLES_REACHED
| Workflow      one-shot
| PR            #<number> <url>
| Branch        <branch>
| Review        APPROVED|CHANGES_REQUESTED|MAX_CYCLES_REACHED|NOT_RUN
| Cycles        <n>/9 | last mode: full|reduced|not_run
+------------------------------------------------+

Summary
<1-line task/result summary>

Review
- Adversarial comments: <n>
- Last review marker: CodeDungeon Code Review|none
- Remaining findings: <none or short list/count>

Work Done
- Tasks: n/a
- Changed files: <short summary or none>
- Verification: <commands/results or blocker>

PR
<url or "not created">

Next
<none or exact next human/agent action>
```
