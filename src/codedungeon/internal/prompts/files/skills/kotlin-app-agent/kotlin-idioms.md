# kotlin-idioms.md

Kotlin/CMP-specific enrichment guidance. Ported from kotlin-specialist Plan-mode.

## Module Placement (commonMain / androidMain / iosMain)
- Default commonMain. Platform source-set only for: platform APIs (camera, biometrics, FS, keychain/keystore), platform-specific UI (iOS swipe-back, Android back handler), platform-only libs.
- If expect/actual needed: name expect decl + what each actual does.

## Composable Architecture
- Every composable: name, location, params with types, renders what, stateful/stateless.
- Data class used as composable param → `@Immutable` or `@Stable` annotation.
- Separate stateful wrapper (in screen file) from stateless child (reusable).
- Cross-screen reuse → shared composable.

## Recomposition & Performance
- Flag every param causing unnecessary recompose. Fix: `@Stable` / `key` / `derivedStateOf` / lambda-based modifier.
- `LazyColumn` / `LazyRow`: `key = { it.id }` mandatory.
- Defer state read: lambda-based `Modifier.offset { }` for hot state.
- Heavy composable (chart/map): `remember`, limit recompose scope, `SubcomposeLayout` if needed.
- Images: existing loader (Coil, Kamel). Specify placeholder + error composable.

## State Management
- `StateFlow` in ViewModel + `collectAsStateWithLifecycle()` (Android) / `collectAsState()` (common).
- UI state data class per screen: name + every field typed. Example `PropertyListUiState(isLoading, properties, error, isEmpty)`.
- Sealed interface for nav events / side effects.
- One ViewModel per screen. No sharing across unrelated screens.

## Coroutines
- Dispatcher: `Main` (UI), `IO` (net/DB), `Default` (CPU).
- All launches in `viewModelScope`. No `GlobalScope`. No raw `CoroutineScope` without lifecycle.
- Parallel: `async` + `awaitAll` OR `coroutineScope { launch; launch }`.
- Error: `try/catch` in coroutine → update UI state. Never unhandled.
- Flow: `catch`, `onStart` (loading), `onCompletion`.

## Navigation
- Use codebase's nav lib exactly (Navigation-CMP / Decompose / Voyager).
- Define route objects/classes + args with types.
- Back stack: which screens removed (e.g., login after auth), which persist.
- Deep link: data needed + redirect if not authed.

## Data Layer
- Repository fn: name, params, return (`Flow<T>` observed / `suspend fun` one-shot).
- Source: Ktor (net), Room/SQLDelight (local), or both offline-first.
- Offline-first: observe local DB + refresh from net background + update local.
- DTO → domain mapping: named mapper / ext fn.

## Platform-Specific
- Android: ProGuard/R8 rules, WorkManager, Baseline Profiles.
- iOS: UIKit interop (native pickers/maps), Kotlin/Native GC differences.
- Min API level: fallback for features unavailable on min SDK.

## Accessibility
- Content descriptions on images/icons.
- Touch target ≥ 48dp.
- Dynamic text sizing (sp; layouts survive 200% scale).
- `semantics { heading() }` on section titles.

## App Size & Startup
- Flag new dep adding significant size.
- Defer new init (lazy / background coroutine). Not in `Application.onCreate`.
- New resources: compression / format requirements.

## Output subsection template

```
#### kotlin
- source-set: commonMain
- composables: [PropertyListScreen @ features/property/presentation/ stateful wrapper collecting PropertyListUiState; PropertyCard @ features/property/presentation/components/ stateless props (property: PropertyItem)]
- state: [PropertyListUiState(isLoading: Boolean, properties: List<PropertyItem>, error: String?, isEmpty: Boolean)]
- viewmodel: [PropertyListViewModel uses PropertyRepository.observeList()]
- data: [PropertyRepository.observeList(): Flow<List<PropertyItem>>]
- navigation: [PropertyListRoute; from BottomNav; no back-stack mutation]
- recomposition: [PropertyItem @Immutable; LazyColumn key = { it.id }]
- platform: [n/a — commonMain only]
- accessibility: [content descriptions on thumbnails, touch target 48dp+]
- references: [features/auth/presentation/LoginScreen.kt]
```

Omit empty subsections. When unsure about API: `verify fn X exists in lib version Y`.
