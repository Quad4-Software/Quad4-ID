# AGENTS.md

Quad4 ID — a passkey-only OIDC provider. Go backend serves a SvelteKit SPA (embedded in
the binary for production). The binary and container image are named quad4-id. The default
branch is master. This file lists what isn't obvious from reading the code.

## Layout

- `backend/` — Go module (gin, GORM, [ory/fosite fork](#backend-go)). Its own toolchain, not part of the repository.
- `frontend/` — SvelteKit 5 SPA. Builds into `backend/frontend/dist` and is embedded via `go:embed`.
- `tests/` — Playwright end-to-end tests (drives a Dockerized full stack).

## Build / test / lint

```sh
# backend/   — the exclude_frontend and unit tags are mandatory locally; CI uses them too
go test -tags=exclude_frontend,unit ./...                          # unit/integration tests
go test -tags=exclude_frontend,unit -run TestName ./internal/...   # a single test
# Tests that need Postgres are skipped unless POCKET_ID_TEST_POSTGRES_URL points at a server (each test creates its own database)
docker run -d --rm --name quad4-id-test-pg -e POSTGRES_PASSWORD=postgres -p 55432:5432 postgres:17
POCKET_ID_TEST_POSTGRES_URL='postgres://postgres:postgres@localhost:55432/postgres?sslmode=disable' go test -tags=exclude_frontend,unit ./...
golangci-lint run  # lint (config: backend/.golangci.yml - includes build tags)

# frontend/  (or root)
pnpm check        # svelte-check — the ONLY frontend type gate (no unit tests exist)
pnpm lint         # vp check — oxlint + oxfmt check via Vite+ (note: not enforced by CI)
pnpm format       # vp fmt — REQUIRED before opening a PR
```

End-to-end (needs Docker; **stop any local backend on `:1411` first** — see gotchas):

```sh
cd tests/setup && docker compose up -d --build   # rebuild after ANY code change, or you test stale code
# docker-compose-francis.yml runs the same suite against a standalone Francis runtime instead of the embedded one
# It needs FRANCIS_VERSION in tests/setup/.env: echo "FRANCIS_VERSION=$(./francis-version.sh)" > .env
cd ../.. && pnpm test                            # = playwright test in tests/
```

## Release

```sh
# bump .version and frontend/package.json version to X.Y.Z, commit, then:
git tag vX.Y.Z && git push origin vX.Y.Z   # the release workflow builds binaries and the GHCR image
```

The container image is built from the single `docker/Dockerfile` (multi-stage, from source) and runs
rootless as uid 65532 on a read-only filesystem: writable paths are /app/data (volume) and /tmp (tmpfs).

## Invites

Signup tokens are exposed to admins as invites (`backend/internal/usersignup`). A token is actor
state (no DB table) with a usage limit, TTL, optional note, user groups and a `requireApproval`
flag. `GET /api/signup/invites/:token` peeks a token without consuming it. When requireApproval is
set, the new user gets `users.approval_pending = true`: SessionAuthenticator restricts their
principal to the same scope set as unverified-email users (own account, passkey enrollment, sign
out), every sign-in path that mints an access token (WebAuthn login, device login, one-time access
token, OIDC grants) rejects them with `user_pending_approval`, SCIM reports them inactive, and the
SPA keeps them on `/signup/pending`. Admins approve via `POST /api/users/:id/approve` or reject by
deleting the user. Rejecting an invite is deleting its token.

## Critical gotchas

- **`exclude_frontend` and `unit` build tags.** Without them plain `go run`/`go test`/`golangci-lint` fail
  to run. Always pass `-tags exclude_frontend,unit` for backend dev/test/lint.
- **Never edit generated files:** `frontend/src/lib/paraglide/**` (Paraglide i18n output) and
  `backend/frontend/dist/**`. For i18n, edit `frontend/messages/en.json`; other locales are not
  synced anymore and are edited directly in `frontend/messages/`.
- **Migrations are split by DB.** Raw SQL via golang-migrate in
  `backend/resources/migrations/{sqlite,postgres}/` — separate files _and_ separate version
  timelines. Add a matching up/down pair to **both**. Not GORM AutoMigrate. SQLite migrations
  are not auto-wrapped in a transaction (`NoTxWrap`); wrap multi-statement ones manually
  (`PRAGMA foreign_keys=OFF; BEGIN; … COMMIT; PRAGMA foreign_keys=ON;`).
- **Column types must match across DBs for export/import.** A SQLite export must import into Postgres
  and vice versa, so every column must map to the same `DBExportKind` (`backend/internal/utils/db_util.go`)
  on both: JSON/binary is `JSONB`/`BYTEA` ↔ `BLOB`, timestamps are `TIMESTAMPTZ` ↔ `DATETIME` (never
  `INTEGER`/`TEXT`), and nullability must match. `TestDBSchemaParity` enforces this (needs Postgres, see above).

## Backend (Go)

- **Config:** global `common.EnvConfig` (caarlos0/env); any secret var supports a `*_FILE` variant.
- **Logging:** stdlib `log/slog` only (bridged to OpenTelemetry). No zerolog/logrus in app code.
- Fosite comes from the **fork** `github.com/pocket-id/fosite`, not `github.com/ory/fosite`. It ports upstream fixes from `ory/hydra`.
  The Go module path stays `github.com/pocket-id/pocket-id/backend` to keep upstream merges manageable.

## Frontend (SvelteKit)

- **Svelte 5 runes only:** `$state`, `$derived`, `$props`, `$bindable`. No `export let`. Event
  modifiers are gone — use `preventDefault` from `#lib/utils/event-util.ts` (`onsubmit={preventDefault(fn)}`).
- **Forms:** use the custom `createForm(schema, initial)` from `#lib/utils/form-util.ts` with
  `form-input.svelte` — match the surrounding file. Import zod as `import { z } from 'zod/v4'`.

## Coding Style Guidelines

### Comments

- Exactly one sentence per line
- There is NO maximum line width: never wrap a single sentence across multiple comment lines, no matter how long that sentence is
- A new line in a comment means a new sentence; a wrapped line does not exist
- No trailing period on single-line comments
- Prefer comments that explain intent, invariants, or why a branch exists
- Avoid comments that simply restate the next line of code
- For multi-step logic, use short section comments to separate the steps and explain why each step exists
- Inside a function, put a one-sentence comment above each major action; the comments double as visual separators between sections and should say what the step does and why, not how
- Favor a few well-placed section comments over a wall of code; a reader should be able to skim the comments and understand the method's flow

```go
// Wrong — one sentence wrapped across multiple lines
// This function performs the main validation logic. It checks
// the input against the schema and returns an error if the
// input is invalid.

// Wrong — trailing period on single-line comment
// Validate the input.

// Right — one sentence per line, each line as long as it needs to be
// This function performs the main validation logic
// It checks the input against the schema and returns an error if the input is invalid

// Right
// Validate the input

// Right
// Normalize the request host so callers can pass either Host or X-Forwarded-Host values

// Right
// Browsers do not accept a cookie Domain attribute set to an IP address
// Returning an empty domain tells the caller to set a host-only cookie instead

// Wrong — restates the code
// Trim whitespace and lowercase the host
host = strings.TrimSpace(strings.ToLower(host))
```

Section comments inside a function — one sentence per major action, describing what and why, acting as visual separators:
