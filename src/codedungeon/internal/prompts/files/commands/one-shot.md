# One Shot

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

Minimal CodeDungeon workflow for a small change that still needs the safety rail of branch, commit, PR, and adversarial review.

Use when the request is narrow enough for one planner pass and one implementation pass. Do not split into task files, do not run the phase pipeline, and do not call `codedungeon-loop`.

## Parameters

- `$ARGUMENTS` - feature prompt or bugfix request.

## Required Outcome

End with:

- non-protected feature branch
- implementation committed
- branch pushed
- GitHub PR created
- `/code-review` run against the PR
- review verdict reported

## Step 0: Validate

If `$ARGUMENTS` is empty:

```text
Usage: /one-shot <feature or bugfix request>
```

Stop.

Run:

```bash
CD=.claude/bin/codedungeon
[ -x "$CD" ] || CD=codedungeon
$CD git guard --repo .
```

If guard fails, stop. Never edit, commit, or push on `main`, `master`, `develop`, `dev`, `staging`, `production`, or `release`.

Check `gh`:

```bash
gh auth status
```

If missing or unauthenticated, stop with the exact blocker.

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

## Step 2: Branch

Create or switch to a feature branch:

```bash
BRANCH="feat/$(echo "$ARGUMENTS" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//' | cut -c1-50)"
[ "$BRANCH" = "feat/" ] && BRANCH="feat/one-shot"
git switch -c "$BRANCH" 2>/dev/null || git switch "$BRANCH"
$CD git guard --repo .
```

## Step 3: Implement

Implement directly from the plan. Keep edits scoped. Add or run the smallest meaningful tests before claiming done.

Do not create `.codedungeon/tasks/*` for this workflow.

## Step 4: Verify

Run every verification command from the plan. If no command was obvious, run the nearest project check from the manifest:

- Go: `go test ./...`
- Rust: `cargo test`
- Node: `npm test` or package script closest to test
- Python: `python -m pytest`

If verification cannot run, record the blocker in the final report and continue only if the change can still be reviewed.

## Step 5: Commit, Push, PR

Run:

```bash
$CD git guard --repo .
git status --short
git add -A
git commit -m "feat: one-shot update"
git push -u origin "$(git branch --show-current)"
gh pr create --fill
```

If the PR already exists, reuse it.

## Step 6: Review

Run:

```text
/code-review .
```

If review returns `CHANGES_REQUESTED`, fix the findings directly, commit, push, and rerun `/code-review`. Maximum 3 review cycles. After 3 cycles, stop with `MAX_REVIEW_CYCLES`.

## Step 7: Report

Return:

```text
ONE_SHOT_COMPLETE
BRANCH: <branch>
PR_URL: <url>
REVIEW_VERDICT: APPROVED|CHANGES_REQUESTED|MAX_REVIEW_CYCLES
VERIFY: <commands and results>
CHANGED_FILES: <summary>
RISKS: <none or list>
```
