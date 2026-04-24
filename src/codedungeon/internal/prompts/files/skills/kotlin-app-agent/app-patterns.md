# app-patterns.md

Architectural patterns + appplan skeleton.

## Plan scope
- Mobile-only: Android + iOS. No desktop/web.
- Always consider both platforms. If change platform-specific → expect/actual, specify explicitly.
- Data layer → domain → presentation separation. No API calls from UI or VM directly.
- Use repository pattern.
- Background work via WorkManager (Android) / platform scheduler. No raw threads.

## Output structure (appplan.md)

```markdown
# appplan

## summary
[2-4 sentences]

## dependencies
- [external prereq]

## changes

### [change-name]
- type: new-screen | new-component | modify-screen | new-viewmodel | new-repository | new-usecase | migration | refactor | platform-specific
- location: [exact path]
- references: [existing file to follow]
- description: [what this does]
- shared-or-platform: [commonMain | androidMain | iosMain | both]
- inputs: [params, nav args, with types]
- state: [UI state / domain state / cached data]
- api: [endpoints consumed]
- ui: [renders — layout, key elements, states]
- offline: [behavior w/o network]
- errors: [error states + display]
- platform-notes: [Android/iOS specifics]
- notes: [edge cases, a11y, perf]

#### kotlin
- [see kotlin-idioms.md]

## execution-order
1. [data layer / shared modules]
2. [domain / use cases]
3. [ViewModels]
4. [screens]
reason: [dependency]

## testing-requirements
- [shared logic unit / UI / platform-specific / offline]

PLAN_COMPLETE: {repo}plan.md
```

## Rules
1. Full state contract per screen (loading/success/error/empty/offline + nav behavior).
2. Reference existing code by path.
3. Shared vs platform explicit per file.
4. Flag missing prereq.
5. Execution-order: data-layer-first.
6. Omit empty sections.
