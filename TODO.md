# TODO

This file tracks the current audit backlog for `fs-engine` after the end-to-end codebase review, runtime validation, test execution, and targeted stabilization work.

Status legend:
- `DONE`: issue validated and fixed
- `OPEN`: issue validated and still needs work
- `FOLLOW-UP`: improvement or hardening item, not an immediate correctness failure

Priority legend:
- `P0`: production-blocking
- `P1`: high risk / high impact
- `P2`: important but not immediately blocking
- `P3`: quality or optimization follow-up

---

## Audit Summary

Validated during audit:
- backend server boot and fresh format
- API contract behavior with real HTTP requests
- filesystem format, fsck, read/write/rename/unlink flows
- WebSocket shell handshake and command execution
- frontend production build
- full Go test suite
- full Go race test suite

Main outcome:
- several real production issues were found and fixed
- the system is substantially more stable now
- one major architectural risk remains around crash consistency and journaling guarantees

---

## Fixed Tickets

### FS-001
- Status: `DONE`
- Priority: `P0`
- Title: Freshly formatted filesystem reported inconsistent superblock free counts
- Area: `internal/fs`
- Root cause:
  `seedFS()` mutated allocator state while creating seeded directories/files, but the final superblock counters were not persisted after seeding completed.
- Impact:
  A brand new filesystem could fail `fsck`, and tests were compensating by manually reconciling metadata instead of validating the real behavior.
- Fix applied:
  Persisted the final superblock after seeding in [internal/fs/mkfs.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/internal/fs/mkfs.go:144).
- Validation:
  Fresh `go run ./cmd/server -format` followed by `POST /api/fs/fsck` returned `{"clean":true}`.

### FS-002
- Status: `DONE`
- Priority: `P0`
- Title: Frontend API client did not match backend routes or response formats
- Area: `web/src`
- Root cause:
  The React client was written against a different API contract:
  POST instead of GET for some reads, wrong route names, wrong JSON field names, wrong assumptions about response shapes.
- Impact:
  Core UI flows were broken or unreliable:
  explorer, inode lookup by path, file reads/writes, terminal integration, and other inspect flows.
- Fix applied:
  Added an adapter layer in [web/src/api/client.ts](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/web/src/api/client.ts:5) to map the actual backend contract into the frontend’s expected shapes.
- Validation:
  Frontend production build passed with `npm run build`.

### FS-003
- Status: `DONE`
- Priority: `P0`
- Title: WebSocket shell upgrades failed behind middleware
- Area: `api`
- Root cause:
  `middleware.Compress` and the custom logging response writer stripped `http.Hijacker`, which Gorilla WebSocket requires for upgrades.
- Impact:
  `/ws/shell` returned `500`, so the terminal page could never connect.
- Fix applied:
  Added upgrade-safe compression and `Hijack`/`Flush` passthrough in [api/middleware.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/api/middleware.go:41).
- Validation:
  Live WebSocket session to `/ws/shell` succeeded and executed `pwd`.

### FS-004
- Status: `DONE`
- Priority: `P1`
- Title: Terminal frontend used the wrong WebSocket endpoint and wrong message protocol
- Area: `web/src/pages`
- Root cause:
  The page connected to `/ws` instead of `/ws/shell`, expected raw text frames, and streamed keystrokes while the backend expects command messages.
- Impact:
  Even with a working backend shell, the terminal UI would not behave correctly.
- Fix applied:
  Updated the terminal page in [web/src/pages/Terminal.tsx](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/web/src/pages/Terminal.tsx:108) to:
  connect to `/ws/shell`, parse JSON responses, and submit commands on Enter.
- Validation:
  Manual WebSocket shell test passed.

### FS-005
- Status: `DONE`
- Priority: `P1`
- Title: Background goroutines were not shut down cleanly
- Area: `internal/cache`, `internal/metrics`, `internal/fs`
- Root cause:
  The writeback flusher and metrics ticker used fire-and-forget goroutines without coordinated shutdown or wait semantics.
- Impact:
  Risk of goroutine leaks, nondeterministic teardown, and dirty lifecycle behavior during unmount/reformat/tests.
- Fix applied:
  Added `sync.Once` and `WaitGroup` based shutdown in:
  [internal/cache/writeback.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/internal/cache/writeback.go:10)
  [internal/metrics/window.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/internal/metrics/window.go:24)
  and closed metrics on unmount in [internal/fs/fs.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/internal/fs/fs.go:118).
- Validation:
  `go test ./... -race -timeout 4m` passed.

### FS-006
- Status: `DONE`
- Priority: `P2`
- Title: Explorer tree expand/collapse only updated top-level nodes
- Area: `web/src/pages`
- Root cause:
  Tree state updates only mapped root-level nodes, so nested expand/collapse behavior was wrong.
- Impact:
  Nested browsing in the file explorer could become inconsistent.
- Fix applied:
  Added recursive tree update logic in [web/src/pages/Explorer.tsx](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/web/src/pages/Explorer.tsx:45).
- Validation:
  Build passed; logic now updates nested nodes correctly.

### FS-007
- Status: `DONE`
- Priority: `P1`
- Title: API and WebSocket origin policy was fully open
- Area: `api`
- Root cause:
  CORS allowed `*` and WebSocket `CheckOrigin` unconditionally returned true.
- Impact:
  Browser-origin trust boundary was too permissive for a production service.
- Fix applied:
  Added same-host plus configurable allowlist origin checks in:
  [api/origin.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/api/origin.go:10)
  [api/middleware.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/api/middleware.go:16)
  [api/handler_shell.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/api/handler_shell.go:11)
- Validation:
  Allowed local dev origin worked; invalid preflight origin returned `403`.

---

## Open Tickets

### FS-008
- Status: `DONE`
- Priority: `P0`
- Title: Crash consistency guarantees were weaker than the design and README implied
- Area: `internal/ops`, `internal/journal`, `internal/cache`
- Root cause:
  File writes dirty data blocks in cache, but normal write flow journals only the inode block and does not enforce true ordered journaling for file data.
  See:
  [internal/ops/file_ops.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/internal/ops/file_ops.go:84)
  [internal/journal/transaction.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/internal/journal/transaction.go:35)
- Impact:
  A crash at the wrong point can lose recent file contents or produce semantics weaker than “descriptor -> data -> commit -> home write” for user data.
- Why this matters:
  The current system is stable for normal operation, but not yet trustworthy if the product promise is ext3-style ordered journaling.
 - Fix applied:
  1. Added snapshot-based mutation commits so mutating ops journal the actual changed block set.
  2. Forced the superblock into the committed mutation set.
  3. Changed journal commit ordering to persist journal blocks before home-block writes.
  4. Added device reload on simulated crash so recovery tests validate on-disk durability instead of in-memory state.
  5. Extended journal descriptor format to support large multi-block transactions.
  6. Added crash durability tests for write and rename flows.
 - Validation:
  `go test ./... -timeout 5m` passed, including integration crash tests and large-file write coverage.

### FS-009
- Status: `DONE`
- Priority: `P1`
- Title: SSE/event model is underspecified relative to the frontend visualization goals
- Area: `internal/fs`, `api`, `web`
- Root cause:
  Backend emits coarse events like `create`, `write`, `mkdir`, while the frontend originally assumed richer events like `block_alloc`, `cache_evict`, `journal_commit`, `inode_alloc`.
- Impact:
  Live visualizations are functional but less expressive than intended; some UI features now rely on polling or degrade gracefully.
- Fix applied:
  1. Defined a versioned SSE/event schema with `version`, `type`, `source`, `path`, `data`, and `ts`.
  2. Added allocator, cache, and journal source-of-truth event emission for block/inode allocation, cache eviction/flush, journal commits, and recovery replay.
  3. Updated the frontend SSE adapter/store to consume the stable payload shape directly.
  4. Added API regression coverage for the connected-event schema and preserved legacy parsing fallback in the client hook.
- Validation:
  `go test ./api -timeout 4m` passed, including SSE schema coverage. Browser pages consuming SSE also passed under Playwright.

### FS-010
- Status: `DONE`
- Priority: `P1`
- Title: API contract is not formally versioned or centrally specified
- Area: `api`, `web`
- Root cause:
  Frontend and backend drifted without a shared typed schema or generated client.
- Impact:
  The repo regressed into a broken UI/backend contract without tests catching it.
- Fix applied:
  1. Added a served contract document at `/openapi.json` and `/api/openapi.json` that defines the supported frontend-facing routes and SSE schema version.
  2. Kept the frontend on centralized DTO adapters in `web/src/api/client.ts` and locked the current route surface into contract tests.
  3. Added API tests that validate the contract endpoint and the actual frontend-facing response surface.
- Validation:
  `go test ./api -timeout 4m` passed with the new contract coverage.

### FS-011
- Status: `DONE`
- Priority: `P2`
- Title: `go test ./...` traverses vendored Go code inside `web/node_modules`
- Area: `repo hygiene`
- Root cause:
  A Go package exists under frontend dependencies, so wildcard package discovery includes it.
- Impact:
  Slower/noisier test runs and surprising package surface.
 - Fix applied:
  Added `GO_PACKAGES` filtering in [Makefile](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/Makefile:3) so `make test`, `make race`, and `make lint` exclude `web/node_modules` Go packages.
 - Validation:
  `go list ./...` still exposes the vendored package, but repo task commands now use the filtered backend package set.

### FS-012
- Status: `DONE`
- Priority: `P2`
- Title: Observability is still limited for production diagnosis
- Area: `api`, `internal/*`
- Root cause:
  Logging is request-level only and mostly unstructured; subsystem-level failures do not carry correlation context.
- Impact:
  Debugging field failures, cache/journal anomalies, or crash-recovery incidents would be slower than necessary.
- Fix applied:
  1. Replaced plain request logs with structured JSON logs including request ID, status, duration, method, and path.
  2. Added journal transaction and recovery event emission carrying transaction IDs and block counts.
  3. Added counters for recovery runs/errors/blocks, mutation rollbacks, cache flush errors, cache evictions, and API error budget tracking.
  4. Added a Prometheus-friendly text mode on `/metrics?format=prometheus`.
- Validation:
  Structured request logs were exercised during Playwright runs. Metrics and recovery counters are covered by backend integration and API tests.

### FS-013
- Status: `DONE`
- Priority: `P2`
- Title: API error mapping is too coarse
- Area: `api`
- Root cause:
  Many handlers return `500` for filesystem-domain errors like permission denied, exists, invalid argument, not empty, or no space.
- Impact:
  Clients get weaker semantics and cannot reliably distinguish user errors from server failures.
 - Fix applied:
  Centralized filesystem error mapping in [api/handler_ops.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/api/handler_ops.go:27) and updated handlers to return semantic HTTP statuses for common domain failures.
 - Validation:
  Added API regression tests for `404`, `409`, and `400` cases in [api/server_test.go](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/api/server_test.go:57). `go test ./api/...` passed.

### FS-014
- Status: `DONE`
- Priority: `P2`
- Title: Crash simulation is useful but not a full failure-injection framework
- Area: `internal/fs`, `internal/journal`, `tests`
- Root cause:
  Current crash simulation replays recovery, but does not test many interleavings or partial persistence windows.
- Impact:
  Reliability confidence is good for happy-path recovery, but incomplete for real crash timing edges.
- Fix applied:
  1. Added deterministic fault injection hooks to the block device for read/write/sync operations.
  2. Added integration tests for interrupted home-block writes during rename and interrupted journal-sync during truncate.
  3. Validated post-crash invariants by forcing journal replay and checking durable names/content after recovery.
- Validation:
  `go test ./internal/integration -run 'TestFaultInjected|TestSmokeWriteSurvivesCrashWithoutFSync|TestSmokeRenameSurvivesCrash' -v -count=1 -timeout 4m` passed.

---

## Follow-Up Tickets

### FS-015
- Status: `DONE`
- Priority: `P3`
- Title: Reduce frontend bundle size
- Area: `web`
- Evidence:
  `npm run build` warns that the main JS bundle is over 1 MB before gzip.
- Impact:
  Not a correctness bug, but avoidable load cost.
- Fix applied:
  Implemented route-level lazy loading and explicit chunk splitting for React core, charts, terminal, UI primitives, and vendor code in the Vite build.
- Validation:
  `vite build` now emits separated chunks (`charts`, `terminal`, `react`, `vendor`) instead of a single oversized entry bundle.

### FS-016
- Status: `DONE`
- Priority: `P3`
- Title: Add end-to-end browser tests for the shipped UI
- Area: `web`, `api`
- Root cause:
  The frontend/backend contract broke without automated browser-level coverage.
- Fix applied:
  Added Playwright configuration with coordinated backend/frontend web servers and browser coverage for explorer file creation/inspection plus journal, stats, and terminal page load flows.
- Validation:
  `npm run test:e2e` passed.

### FS-017
- Status: `DONE`
- Priority: `P3`
- Title: Document deployment origin configuration
- Area: `docs`
- Root cause:
  Origin policy is now configurable through `FS_ENGINE_ALLOWED_ORIGINS`, but this is not documented.
 - Fix applied:
 Added origin allowlist documentation and recommended repo task commands to [README.md](/Users/sanskar/dev/Research/Projects/Filesystem/fs-engine/README.md:161).

### FS-018
- Status: `DONE`
- Priority: `P4`
- Title: Stats page emits Recharts container sizing warnings during dev/E2E
- Area: `web`
- Root cause:
  Recharts mounts before its container has a stable measured size in some dev/test render paths, so it logs width/height warnings.
- Impact:
  No known production correctness issue, but browser test output and local dev logs are noisy.
- Fix applied:
  Gated stats chart rendering on measured container dimensions and passed concrete width/height values into the chart container path so Recharts no longer mounts against an invalid size.
- Validation:
  `npm run test:e2e` no longer emits the earlier Recharts width/height warnings on the stats page path.

### FS-019
- Status: `DONE`
- Priority: `P4`
- Title: Vite dev websocket proxy logs `EPIPE` when test clients disconnect abruptly
- Area: `web dev server`
- Root cause:
  Playwright page teardown can close proxied websocket connections while Vite is still writing to them, producing noisy `EPIPE` logs.
- Impact:
  No functional regression in the app, but test/dev output is noisier than necessary and can obscure real websocket issues.
- Fix applied:
  Changed the terminal page to connect directly to the backend websocket in local dev, avoiding Vite's websocket proxy for that path. Expanded local-dev origin defaults to allow the Playwright/Vite dev origin to connect directly.
- Validation:
  Browser E2E runs no longer emit the prior Vite websocket proxy `EPIPE` noise when terminal pages close.

### FS-020
- Status: `DONE`
- Priority: `P4`
- Title: Raw `go test ./...` still traverses vendored Go code under `web/node_modules`
- Area: `repo hygiene`
- Root cause:
  `go list ./...` includes the vendored Go package exposed by frontend dependencies, even though repo task commands filter it out.
- Impact:
  Direct wildcard Go test invocations still include unexpected package surface and noisier output.
- Fix applied:
  Added a frontend `postinstall` patch that writes a nested `go.mod` under `web/node_modules/flatted/golang`, isolating that vendored dependency from the repository's wildcard Go package discovery.
- Validation:
  `go list ./...` no longer includes the vendored `web/node_modules` Go package.

---

## Recommended Execution Order

All tracked tickets in this audit backlog are now completed. New work should be added as fresh tickets instead of reusing this sequence.

---

## Validation Snapshot

Validated after fixes:
- `go test ./api -timeout 4m`
- `go test ./api ./internal/fs ./internal/cache ./internal/disk -race -timeout 10m`
- `go test ./internal/integration ./internal/journal ./internal/ops ./internal/shell -race -timeout 10m`
- `go test ./internal/integration -run 'TestFaultInjected|TestSmokeWriteSurvivesCrashWithoutFSync|TestSmokeRenameSurvivesCrash' -v -count=1 -timeout 4m`
- `go test ./... -timeout 8m`
- `vite build`
- `npm run test:e2e`
- fresh browser flows:
  explorer load -> create file -> inspect file
- runtime pages:
  journal load, stats load, terminal connect
