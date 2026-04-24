# backend-patterns.md

Architectural patterns + backendplan output skeleton. Read once in plan mode.

## Plan scope

- Follow existing module/layer separation. No business logic in handlers. No data access in domain.
- New modules mirror existing dir structure + naming.
- Error types reuse existing patterns (no new response formats).
- Every endpoint: full contract — input types, output types, error cases.

## Output structure (backendplan.md)

```markdown
# backendplan

## summary
[2-4 sentences what plan accomplishes in this repo]

## dependencies
- [external dep that must exist first, e.g. "migration X must run"]

## changes

### [change-name]
- type: new-module | new-endpoint | modify-endpoint | migration | new-service | refactor
- location: [exact path]
- references: [existing file/module to follow]
- description: [what this does, specifically]
- input: [req/message shape with types]
- output: [res/return shape with types]
- errors: [error cases + handling]
- database: [tables/cols touched, indexes, migration details]
- notes: [edge cases, ordering, constraints]

#### rust
- [see rust-idioms.md — inline subsection per change]

### [change-name-2]
...

## execution-order
1. [migrations first]
2. [data layer]
3. [handlers]
4. [wire]
reason: [why this order]

## testing-requirements
- [unit / integration / scenarios]

PLAN_COMPLETE: {repo}plan.md
```

## Rules
1. Every endpoint: full contract (input types, output types, error cases).
2. Reference existing code by path.
3. Indexes explicit: `add index on properties(city, price)` not "appropriate indexes".
4. Flag what doesn't exist — prerequisite vs assumption.
5. Execution order: migrations-first.
6. Omit empty sections entirely.
