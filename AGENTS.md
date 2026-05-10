# AGENTS.md

## Cursor Cloud specific instructions

### Overview

ZyHive is a single-binary AI Team Operating System — Go 1.22+ backend (Gin) with a Vue 3 + Vite frontend embedded via `go:embed`. No database; all state is filesystem-based (`agents/` directory). No Docker required.

### Quick reference

| Task | Command |
|------|---------|
| Install Go deps | `go mod download` |
| Install UI deps | `cd ui && npm ci` |
| Go lint | `go vet ./...` |
| Go tests | `go test ./... -count=1 -timeout=5m` |
| UI build | `cd ui && npm run build` |
| Full build (UI + Go) | `make build` |
| Go-only build (UI already built) | `make go-only` |
| Sync UI dist for embed | `make sync-ui` |

### Running the application

1. Copy config: `cp aipanel.example.json aipanel.json` (already in `.gitignore`).
2. Start Go backend: `AIPANEL_CONFIG=aipanel.json ./bin/aipanel` — serves on `:8080`.
3. Start Vite dev server: `cd ui && npx vite --host 0.0.0.0` — serves on `:5173` and proxies `/api` and `/ws` to the backend.
4. Auth token for the example config: `change-me-in-production`.

### Gotchas

- **Must use `make build`** — `go build` alone fails because `go:embed` expects `cmd/aipanel/ui_dist/` which is populated by `make sync-ui`. The committed `ui_dist/` is a stale snapshot; always rebuild before testing Go changes that touch the UI.
- **LLM API key required for chat**: Without a real API key in `aipanel.json`, the server starts but agents cannot converse. All other functionality (agent CRUD, workspace, settings, UI navigation) works without it.
- **CI checks**: `go vet ./...`, `go test ./... -count=1 -timeout=5m`, and `cd ui && npm ci && npm run build`. The `golangci-lint` job is advisory (non-blocking).
- **`aipanel.json` and `agents/`** are in `.gitignore` — never commit them.
