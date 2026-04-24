# nextjs-idioms.md

Next.js-specific enrichment guidance. Ported from nextjs-specialist Plan-mode.

## Rendering & Data Strategy
- Default Server Component. `'use client'` only for state / effects / event handlers.
- Push `'use client'` down tree. Page = Server Component with Client islands.
- Per page: static (default) / dynamic (`force-dynamic`) / ISR (`revalidate: N`). Justify.
- Data fetch: Server Component with `cache`/`revalidate` OR client via existing data layer (React Query / SWR).
- Mutations: Server Action (`'use server'`) preferred for form submits. API Route for non-form.
- Parallel fetch: `Promise.all`. Flag sequential as intentional.
- Streaming: `Suspense` + `loading.tsx` for above-fold deferred content.

## Component Architecture
- Name every component. Location per existing convention.
- Classify: shared (shared/common dir) vs feature (with its route).
- Props typed — TS interface, every field. No `any`.
- Per component: Server/Client, renders what, state managed, events handled.
- Reuse design system / component library. No reinvented button/input/card/modal.
- Identify cross-screen reusable pattern — plan shared before duplication.

## Layout & Responsive
- Layout hierarchy: which `layout.tsx`, nested layouts.
- Mobile-first. Breakpoint behavior: mobile → tablet → desktop.
- Forms single-column on mobile. Short related fields can pair on desktop.
- Max-width: `max-w-lg` forms, `max-w-4xl` content pages.

## Forms & Input
- >5 fields → multi-step with progress indicator.
- Validation: on-blur fields vs on-submit. Zod/Yup schema named.
- States: loading → success → error. What user sees per stage.
- Optimistic update where applicable (toggles, favorites).
- Inline error next to field, not top banner.

## Navigation & Loading
- `loading.tsx` per data-fetching route. Skeleton layout described.
- `error.tsx` boundary where API failure likely.
- `<Link prefetch>` vs `router.push`. Specify.
- Deep link: auth check, redirect, data prereq.

## SEO (public-facing)
- Metadata API: title, description, og:image, canonical.
- Dynamic pages: `generateMetadata` with data source.
- JSON-LD for searchable entities (product, article, listing).

## Performance
- Heavy/non-critical → `next/dynamic` with `ssr: false`.
- Images: `next/image` + `sizes` + `priority` (above-fold) + format opt.
- Fonts: `next/font`. Never external CDN.
- Flag new heavy JS needing code-split.
- Core Web Vitals targets: LCP ≤ 2.5s, INP ≤ 200ms, CLS ≤ 0.1.

## Output subsection template

```
#### nextjs
- rendering: [Server Component | Client Component | mixed — specify parts]
- data: [fetch strategy, cache, revalidate]
- components: [PropertyCard @ src/components/property/ Server, props PropertyCardProps { property: {...} }, renders ...]
- layout: [mobile stacked, desktop 3-col grid]
- ux: [form step 1 of 3, validation on-blur, skeleton card on load]
- seo: [Metadata API title/desc; JSON-LD Product]
- performance: [next/image sizes=(max-w-full), priority above-fold]
- references: [src/app/(dashboard)/properties/page.tsx]
```

Omit empty subsections.
