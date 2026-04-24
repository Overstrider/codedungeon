# frontend-patterns.md

Architectural patterns + frontendplan skeleton.

## Plan scope
- Follow existing component structure + naming.
- Separate data-fetching from presentation (container/presenter / hooks / Server Components).
- Shared components in shared/common; feature-specific stays with feature.
- Server state via existing data layer (React Query / SWR). Local state for UI.
- Global store only for truly app-wide state.
- All API calls via existing API client. No raw fetch/axios in components.

## Output structure (frontendplan.md)

```markdown
# frontendplan

## summary
[2-4 sentences]

## dependencies
- [external prereq, e.g. "backend endpoint X deployed"]

## changes

### [change-name]
- type: new-page | new-component | modify-component | new-hook | new-route | refactor | style
- location: [exact path]
- references: [existing file to follow]
- description: [what this does]
- props/inputs: [component props / hook params with types]
- state: [UI state / server cache / global store]
- api: [endpoints consumed, from cross-repo contracts]
- renders: [visual output — layout, key elements]
- errors: [error states + display]
- notes: [edge cases, a11y, perf]

#### nextjs
- [see nextjs-idioms.md]

## execution-order
1. [shared components/hooks first]
2. [pages consuming them]
reason: [dependency]

## testing-requirements
- [component render, user interaction, error states, a11y]

PLAN_COMPLETE: {repo}plan.md
```

## Rules
1. Component props with types.
2. Reference existing code by path.
3. API consumption explicit: endpoint + shape + loading/error handling.
4. Flag missing prereq as prerequisite.
5. Execution-order: shared first.
6. Omit empty sections.
