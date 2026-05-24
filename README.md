# fs-engine

A from-scratch, inode-based filesystem engine structurally identical to ext2 with ext3-style ordered journaling, a Go HTTP/WebSocket API, and a React visualizer.

## Quick Start (3 commands)

```bash
# 1. Start the backend (formats a fresh 64 MB disk image on first run)
cd fs-engine && go run ./cmd/server

# 2. In another terminal, start the React frontend
cd fs-engine/web && npm install && npm run dev

# 3. Open the browser
open http://localhost:5173
```

> Backend runs on **:8080**, frontend dev server on **:5173** (proxies `/api`, `/sse`, `/ws` to :8080).

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Browser                              │
│  ┌──────────┐ ┌──────────┐ ┌────────────┐ ┌─────────────┐  │
│  │  Block   │ │  Inode   │ │  Journal   │ │   Terminal  │  │
│  │   Map    │ │Inspector │ │  Viewer    │ │  (xterm.js) │  │
│  └────┬─────┘ └────┬─────┘ └─────┬──────┘ └──────┬──────┘  │
│       └────────────┴─────────────┴───────────────┘          │
│                   REST /api/*  │  SSE /sse  │  WS /ws/shell  │
└───────────────────────────────┼────────────┼────────────────┘
                                │            │
┌───────────────────────────────▼────────────▼────────────────┐
│                    Go HTTP Server (:8080)                    │
│  ┌──────────┐  ┌────────────┐  ┌──────────┐  ┌──────────┐  │
│  │  REST    │  │    SSE     │  │    WS    │  │  Shell   │  │
│  │ handlers │  │  pub/sub   │  │  shell   │  │ (20 cmds)│  │
│  └────┬─────┘  └────────────┘  └──────────┘  └──────────┘  │
│       │                                                      │
│  ┌────▼──────────────────────────────────────────────────┐  │
│  │                    Filesystem Core                    │  │
│  │  ┌──────────┐  ┌─────────┐  ┌──────────┐             │  │
│  │  │  Buffer  │  │  Inode  │  │ Journal  │             │  │
│  │  │  Cache   │  │  Cache  │  │ (JBD2)   │             │  │
│  │  │ LRU 256  │  │ LRU 512 │  │ 128 blks │             │  │
│  │  └────┬─────┘  └────┬────┘  └────┬─────┘             │  │
│  │       └─────────────┴────────────┘                    │  │
│  │                   POSIX ops layer                     │  │
│  │   open/read/write/stat/mkdir/link/symlink/chmod/...   │  │
│  └───────────────────────────┬───────────────────────────┘  │
│                              │                               │
│  ┌───────────────────────────▼───────────────────────────┐  │
│  │               Block Device (disk.img)                 │  │
│  │  64 MB binary file · 16384 blocks · 4096 bytes/block  │  │
│  └───────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

### Disk Layout

| Block(s)    | Purpose               |
|-------------|-----------------------|
| 0           | Boot block (reserved) |
| 1           | Superblock (`0xEF53`) |
| 2           | Inode bitmap          |
| 3           | Block bitmap          |
| 4 – 131     | Inode table (4096 inodes × 128 bytes) |
| 132 – 2179  | Journal (2048 blocks, JBD2-style) |
| 2180 – 16383 | Data blocks           |

---

## Frontend Pages

| Page | Route | What it shows |
|------|-------|---------------|
| **Block Map** | `/` | 128×128 SVG grid of all 16384 blocks, color-coded by type (boot, superblock, inode bitmap, block bitmap, inode table, journal, data-used, data-free, cache-dirty). Animates in real-time via SSE. |
| **Explorer** | `/explorer` | File tree browser. Click directories to expand, click files to view content. |
| **Inode Inspector** | `/inode` | Enter any inode number to see mode, UID/GID, size, timestamps, direct/indirect block pointers, and data block list. |
| **Journal Viewer** | `/journal` | Ring buffer of the last 64 committed journal transactions: sequence ID, timestamp, block list, operation tags. |
| **Stats** | `/stats` | Live Recharts graphs of cache hits/misses/evictions, filesystem df (total/used/free), and per-operation metrics counters. |
| **Terminal** | `/terminal` | Full xterm.js terminal connected to the built-in shell over WebSocket. Supports 20 commands (see `help`). |

---

## Built-in Shell Commands

```
ls [-l] [path]     ll [path]         cd [path]        pwd
mkdir <path>       rmdir <path>      touch <path>     rm [-r] <path>
cat <path>         write <path> <data>                 mv <src> <dst>
cp <src> <dst>     ln <src> <dst>    stat <path>
chmod <mode> <path>  chown <uid>:<gid> <path>
df                 find <path> [-name <pat>]
echo [args...]     help
```

---

## API Reference

All REST endpoints are prefixed with `/api`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Liveness check |
| GET | `/api/fs/stat?path=` | Stat a path |
| GET | `/api/fs/lstat?path=` | Lstat a path |
| GET | `/api/fs/ls?path=` | List directory |
| POST | `/api/fs/mkdir` | Create directory |
| DELETE | `/api/fs/rmdir?path=` | Remove empty directory |
| POST | `/api/fs/write` | Write file |
| POST | `/api/fs/create` | Create file |
| GET | `/api/fs/read?path=` | Read file |
| DELETE | `/api/fs/unlink?path=` | Delete file |
| POST | `/api/fs/rename` | Rename/move |
| POST | `/api/fs/symlink` | Create symlink |
| POST | `/api/fs/link` | Create hard link |
| POST | `/api/fs/chmod` | Change mode |
| POST | `/api/fs/chown` | Change owner/group |
| POST | `/api/fs/utimes` | Update timestamps |
| GET | `/api/fs/readlink?path=` | Read symlink target |
| POST | `/api/fs/truncate` | Truncate file |
| GET | `/api/inspect/superblock` | Superblock fields |
| GET | `/api/inspect/inode/{num}` | Inode detail |
| GET | `/api/inspect/bitmap/blocks` | Bitmap counts |
| GET | `/api/inspect/bitmap/inodes` | Bitmap counts |
| GET | `/api/inspect/cache` | Buffer cache stats |
| GET | `/api/inspect/disk` | Full 16384-block map |
| GET | `/api/inspect/block/{addr}` | Hex dump + parse |
| GET | `/api/inspect/journal` | Recent transactions |
| GET | `/api/inspect/tree` | Recursive dir tree |
| GET | `/api/inspect/fragmentation` | Free-run analysis |
| GET | `/api/inspect/df` | Disk usage |
| GET | `/api/metrics` | Op counters |
| POST | `/api/fs/format` | Reformat (destructive) |
| POST | `/api/fs/fsck` | Consistency check |
| POST | `/api/fs/crash` | Simulate crash + replay |
| POST | `/api/shell` | Run one shell command |
| GET/WS | `/ws/shell` | Interactive shell (WebSocket) |
| GET | `/sse` | Server-sent filesystem events |

---

## Running Tests

```bash
# All backend packages with race detector
make race

# Integration smoke tests only
go test ./internal/integration/... -race -v

# API tests only
go test ./api/... -race -v
```

GitHub Actions CI enforces four required checks:
- `Backend Test` runs `go test ./... -timeout 8m`
- `Backend Race` runs the backend race suite in two slices
- `Frontend Build` runs `npm ci && npm run build`
- `Browser E2E` runs Playwright against the live Go server and Vite frontend

For local parity with CI:

```bash
go test ./... -timeout 8m
go test ./api ./internal/fs ./internal/cache ./internal/disk -race -timeout 10m
go test ./internal/integration ./internal/journal ./internal/ops ./internal/shell -race -timeout 10m
cd web && npm ci && npm run build && npm run test:e2e
```

All 60+ tests pass, including:
- 10 integration smoke tests (1 MB write/read, 100 files, hard links, symlink chains, ELOOP, journal recovery, permission denied, disk full, 5 MB file, fsck clean)
- 19 API handler tests
- Cache, disk, fs, journal, ops, shell unit tests — all with `-race`

---

## Key Design Decisions

- **No OS filesystem calls** — the entire stack (block device → inode → directory → ops) is pure Go; `disk.img` is a flat `[]byte` in memory, synced to file on demand.
- **Import-cycle-free inode cache** — `InodeCache` stores raw `[128]byte` blobs; encoding/decoding happens at the `fs` layer boundary.
- **Ordered journaling (JBD2-style)** — descriptor block → data blocks → CRC32 commit block → real block writes → fsync. Crash recovery replays any committed-but-not-applied transactions.
- **LRU buffer cache with dirty tracking** — 256-slot write-back cache; `CacheStats.DirtyAddrs` exposes current dirty set for the block map visualizer.
- **Fast symlinks** — targets < 60 bytes stored inline in the inode's block pointer fields (`Blocks512 = 0`); no data block allocated.
- **Sticky bit** — `/tmp` is created with mode `01777`; `Unlink` enforces sticky-bit semantics.

## Deployment Notes

### Allowed Origins

The API and WebSocket shell no longer accept every browser origin by default.

Use `FS_ENGINE_ALLOWED_ORIGINS` to set a comma-separated allowlist:

```bash
export FS_ENGINE_ALLOWED_ORIGINS="https://app.example.com,https://admin.example.com"
```

Default local development allowlist:
- `http://localhost:5173`
- `http://127.0.0.1:5173`
- `http://localhost:8080`
- `http://127.0.0.1:8080`

### Recommended Commands

Use the repo tasks instead of raw `go test ./...` so frontend dependency code under `web/node_modules` is excluded from backend package discovery:

```bash
make test
make race
make lint
```

## Repo Governance

- CI workflows live in `.github/workflows`
- Issue intake is standardized with issue forms in `.github/ISSUE_TEMPLATE`
- Pull requests use `.github/PULL_REQUEST_TEMPLATE.md`
- Branch protection can be applied with `.github/scripts/apply-branch-protection.sh <owner/repo> [branch]`

## Releases

Release and versioning policy is documented in [VERSIONING.md](./VERSIONING.md).

- Semantic tags use `vX.Y.Z`
- `.github/workflows/release.yml` builds backend binaries, frontend bundles, and checksums

## Containers and Deployment

- Backend container: [Dockerfile](./Dockerfile)
- Frontend container: [web/Dockerfile](./web/Dockerfile)
- Local production-like compose: [docker-compose.prod.yml](./docker-compose.prod.yml)
- Kubernetes manifests: [deploy/kubernetes](./deploy/kubernetes)

## Observability

- Prometheus config: [deploy/observability/prometheus/prometheus.yml](./deploy/observability/prometheus/prometheus.yml)
- Grafana provisioning and dashboard: [deploy/observability/grafana](./deploy/observability/grafana)
- Local observability compose stack: [docker-compose.observability.yml](./docker-compose.observability.yml)

Metrics scrape target:

```text
GET /metrics?format=prometheus
```

## Security

- Security reporting and policy: [SECURITY.md](./SECURITY.md)
- Dependency update automation: [.github/dependabot.yml](./.github/dependabot.yml)
- Security workflow: [.github/workflows/security.yml](./.github/workflows/security.yml)
- Go runtime baseline: patch-level updates should stay at or above the latest fixed release line required by `govulncheck`

## Performance

- Benchmark workflow: [.github/workflows/perf.yml](./.github/workflows/perf.yml)
- Local benchmark docs: [perf/README.md](./perf/README.md)
- Make targets: `make perf`, `make perf-go`, `make perf-k6`
