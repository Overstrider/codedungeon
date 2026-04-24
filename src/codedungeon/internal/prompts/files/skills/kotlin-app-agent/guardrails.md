# guardrails.md (kotlin-app subset of task.md §5)

Read only matching section.

## Mobile (Kotlin / KMP)
- Source set discipline: commonMain default; platform only for platform APIs.
- Every composable param risking recompose → stability annotation / key / derivedStateOf.
- `LazyColumn`/`LazyRow` ALWAYS `key = { ... }`.
- `viewModelScope` for all launches. No `GlobalScope`.
- Platform expect/actual: name both sides.

## Performance
- Cold start < 2s. Defer non-essential init (analytics, SDKs) past first frame.
- Lazy-init heavy deps.
- Per-frame < 16ms (60fps). Heavy compute off main thread.
- Images: WebP, density-resized, async + cached.
- Minimize third-party deps (each = APK/IPA size + startup hit).
- Release: resource shrinking + dead code removal.
- Large lists: lazy/recycled rendering only.

## Concurrency
- Dispatcher explicit: Main / IO / Default.
- Structured concurrency via `viewModelScope` / `coroutineScope { }`.
- Parallel: `async` + `awaitAll`.
- Error handling inside coroutine → UI state. Never unhandled.
- Flow operators: `catch`, `onStart`, `onCompletion`.

## Data
- Offline behavior per screen: cached + staleness / explicit offline state.
- API cache locally (Room / SQLDelight).
- All states: loading / success / error / empty / offline.
- Background work: WorkManager (Android) / platform scheduler. No raw threads.

## Security
- Secrets → Android Keystore / iOS Keychain. Never SharedPreferences/UserDefaults plain.
- Auth: OAuth 2.1 / OIDC + PKCE via system browser. Never WebView login.
- TLS 1.2+. Cert pinning for high-value data.
- R8/ProGuard + iOS symbol strip on release.
- Never log secrets. Structured logs with correlation IDs.
- Validate + sanitize input.
- Detect root/jailbreak → limit sensitive features.

## UX / Platform Conventions
- Material (Android) + HIG (iOS).
- Back stack correct; deep links resolve with state.
- User-facing strings in resources (no hardcoded in UI).
- Animation never blocks interaction.
- No raw API errors to user.
- Visual feedback per action.

## Accessibility
- Content descriptions on all interactive.
- Touch target 48dp+.
- WCAG 2.2 AA contrast.
- Dynamic text scaling support.
- TalkBack / VoiceOver nav works.

## Observability
- New screens/flows → analytics via existing event naming.
- Crashes → existing crash reporter. No silent swallow.
- Perf-critical ops instrumented if perf monitor present.
