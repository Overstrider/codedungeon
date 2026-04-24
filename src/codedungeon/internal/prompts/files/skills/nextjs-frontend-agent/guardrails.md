# guardrails.md (nextjs-frontend subset of task.md §5)

Read only the section matching THIS change.

## Frontend (Next.js / React)
- Server Component default. `'use client'` only when needed.
- Data via existing fetch layer (React Query / SWR / Server Component fetch).
- Props typed — TS interface per component. No `any`.
- Shared components in shared dir; feature-specific with feature.
- All API calls through existing API client. No raw fetch in components.

## Performance
- Route code-split + lazy load. No heavy component in main bundle.
- `next/image` + modern format (WebP/AVIF) + lazy + responsive `sizes`.
- Memoize only where profiling shows need. Do not preempt.
- Virtualize large lists (>100 items).
- Core Web Vitals: LCP ≤ 2.5s, INP ≤ 200ms, CLS ≤ 0.1. Flag regressions.
- `next/dynamic` with `ssr: false` for heavy non-critical.
- `next/font` for fonts. No external CDN.

## Security
- Never trust user input. Sanitize before render / process.
- No `dangerouslySetInnerHTML` unless sanitized with library (DOMPurify).
- Auth tokens in httpOnly secure cookies. Never localStorage.
- Respect existing auth guards + RBAC.
- Third-party scripts: SRI where possible.
- No secrets in client code / URLs / logs.
- Form validation: client-side (UX) + server-side (security). Plan both.

## Accessibility
- Interactive elements keyboard-navigable.
- `alt` on images (empty if decorative).
- Form inputs + `<label>` association.
- Semantic HTML (nav/main/section/article/button) over div+onClick.
- WCAG 2.2 AA contrast (4.5:1 text, 3:1 large text).
- Modal: focus trap + Escape dismiss.

## Observability
- Client errors → existing error boundary / Sentry equivalent.
- Analytics events follow existing naming convention.
- No raw API errors / stack traces surfaced to user.

## Testing
- Component render + user interaction + error states + a11y (axe).
- E2E in Playwright — covered by phase-6-tests / test-frontend agent.
- Mock server state in tests; do not hit real backend (except E2E).
