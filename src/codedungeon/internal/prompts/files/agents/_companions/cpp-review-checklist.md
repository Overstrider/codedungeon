# cpp-review-checklist.md

C++ review concerns. cpp-reviewer reads on demand.

## Types & Ownership
- Exact classes/structs: members with types + access specifiers.
- Ownership per resource: `std::unique_ptr` / `std::shared_ptr` / value semantics. Justify.
- No raw `new` / `delete` in app code. Flag legacy raw pointers if present.
- Move semantics: which classes need move ctors / which must be non-copyable (`= delete`).
- Value types: aggregate init / ctor / factory. Follow codebase.
- `std::optional` (may-not-exist) / `std::variant` (tagged union) / `std::expected` or Result type (error-or-value).

## RAII & Resources
- Every resource (memory / FD / socket / lock / GPU buf) wrapped in RAII.
- Destructors never throw. Log + swallow cleanup failures.
- Rule of Five: if class manages resource, define-or-delete all five.
- Locks: `std::lock_guard` (scoped) / `std::unique_lock` (conditional wait / deferred) / `std::scoped_lock` (multiple mutexes). Never manual lock/unlock.

## Error Handling
- Follow codebase: exceptions / error codes + `[[nodiscard]]` / `std::expected` / Result. No mixing within a module.
- Exceptions: domain-specific types (derive `std::runtime_error`). `noexcept` where applicable.
- Error codes: return type + `[[nodiscard]]` + caller check pattern.
- API boundary / FFI: convert to boundary-appropriate format.
- Never ignore errors. Every failure path specified.

## Concurrency
- Model: `std::thread` / `std::jthread` (C++20) / codebase thread pool / `std::async`.
- Shared state: mutex type (`std::mutex` / `std::shared_mutex`) + lock type + critical section scope.
- Lock-free: atomic types + memory ordering. Only if profiling justifies.
- Async: `std::future` / `std::promise` / C++20 coroutines.
- Thread safety doc per class. If not thread-safe, explicit note.
- Producer-consumer: codebase concurrent queue or `std::mutex` + `std::condition_variable` + `std::deque`.

## Memory & Performance
- Allocation: stack / heap smart pointer / arena-pool / placement-new hot path.
- Pre-size containers (`reserve`) when size known.
- `std::string_view` + `std::span` for non-owning refs.
- Move over copy. Specify param passing: by-value (enable move) / const ref / rvalue ref.
- Hot loop: cache-friendly layout (SoA vs AoS), no allocations inside, avoid virtual dispatch if profiled.
- `constexpr` / `consteval` for compile-time values / fns.

## Build System
- CMake target (library / executable) per new file.
- New sources added to target. New headers exposed via proper interface.
- New dep: name + version + fetch (vcpkg / Conan / FetchContent / system). Confirm no duplicate.
- Compile flags: warning levels, sanitizers for dev (`-fsanitize=address,undefined`), opt for release.
- C++20 modules: interface vs implementation unit if codebase uses.

## Headers & API
- Follow existing include structure.
- Public header: minimize includes (forward-declare), Pimpl if codebase uses.
- `#pragma once` vs include guards per codebase convention.
- Template code: header vs `.tpp` / `.ipp` per pattern.
- Attrs on public API: `[[nodiscard]]` / `noexcept` / `constexpr`.

## Testing
- Framework: GoogleTest / Catch2 / doctest. Follow existing.
- Per component: unit tests + case names + assertions.
- Mocking: GMock / manual test doubles per pattern.
- Concurrent: ThreadSanitizer (`-fsanitize=thread`) + stress tests.
- Memory: AddressSanitizer in CI.
- Perf-critical: benchmarks via codebase tool (Google Benchmark / Catch2 / custom).

## Observability
- Logging: codebase logger (spdlog / glog / custom). Levels + structured fields.
- Tracing: OpenTelemetry C++ / custom. Span names.
- Metrics: Prometheus / custom. New counters / histograms.
