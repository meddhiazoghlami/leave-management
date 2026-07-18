# Leave Management

[![CI](https://github.com/meddhiazoghlami/leave-management/actions/workflows/ci.yml/badge.svg)](https://github.com/meddhiazoghlami/leave-management/actions/workflows/ci.yml)

A small but complete **employee leave-management** web app: employees request leave, managers approve or reject, balances are tracked per leave type, and there's a team calendar and an admin area for leave types, allocations, and public holidays.

It was built as a **progressive learning project** — each phase (0–8) introduced exactly one piece of the stack on "hello dzovi" demos, and **Phase 9** assembles all of it into this real app. See [`docs/learning/phases.md`](docs/learning/phases.md) for the roadmap and [`docs/learning/learning.md`](docs/learning/learning.md) for the per-phase write-ups.

> **Server-rendered, no SPA.** HTML comes from the server (templ); HTMX swaps fragments for server-driven interactions; Alpine handles client-only UI state (modals, toasts, calendar navigation). There is no JSON API and no client-side framework.

---

## Tech stack

| Layer | Technology | Notes |
|---|---|---|
| Language | **Go 1.25** | single binary |
| CLI | **Cobra** | one binary, `serve` + `seed` subcommands |
| Dependency injection | **Wire** | compile-time DI; composition root in `internal/app` |
| HTTP | **Gin** | routing + middleware groups |
| HTML templating | **templ** | type-safe Go components (`.templ` → generated `.go`) |
| Styling | **Tailwind CSS v4** | utility-first, purged at build time |
| Server interactions | **HTMX** | HTML-over-the-wire fragment swaps |
| Client UI state | **Alpine.js** | modals, toast stack, calendar month nav |
| Database | **PostgreSQL** | via `pgx/v5` connection pool |
| DB access | **sqlc** | typed Go generated from `sql/queries/` + migrations |
| Migrations | **golang-migrate** | `sql/migrations/*.sql` |
| Task runner | **Make** | `make help` for all targets |
| Deployment | **Docker + Compose** | multi-stage image; Postgres + migrations + app |
| Asset pipeline | **Vite** | bundles/hashes JS + CSS into `public/build/` |
| Auth | **bcrypt** (`golang.org/x/crypto`) | password hashing + Postgres-backed sessions |

---

## Features

- **Authentication** — email + password login, bcrypt hashing, server-side sessions in Postgres, HttpOnly/SameSite cookie, logout that truly invalidates the session.
- **Account bootstrap** — on `serve` startup the app auto-provisions an admin and an HR account from `BOOTSTRAP_ADMIN_EMAIL`/`BOOTSTRAP_HR_EMAIL`, generating a random password and emailing it over SMTP; existing accounts are skipped. See [Bootstrap accounts](#environment-variables).
- **Roles** — `employee`, `manager`, `admin`, `hr`, enforced by middleware plus per-request ownership checks. `hr` is org-wide and currently mirrors `admin` (approves anyone, sees the full directory, manages leave types/holidays/allocations/settings); kept as its own role so admin-only powers can diverge later.
- **Dashboard** — your balances per leave type and your recent requests; managers see a pending-approval badge.
- **Leave requests** — submit via an Alpine modal + HTMX form; working days are computed excluding weekends and public holidays and snapshotted on the request. Cancel your own pending requests.
- **Approvals** — managers approve/reject pending requests from their direct reports (admins and HR can act on anyone); cards swap out live with a toast.
- **Balances** — computed as *allocated days − approved working days* for the year (no denormalized counter).
- **Team** — managers see their reports, admins and HR see everyone; click through to a profile with balances + request history.
- **Calendar** — month grid of approved leave and public holidays; Alpine drives prev/next, HTMX fetches each month.
- **Admin** — manage leave types, public holidays, and per-employee yearly allocations.
- **Toasts** — server sets an `HX-Trigger` header, HTMX dispatches an event, an Alpine host renders the toast stack.

---

## Architecture

### Request flow

```
Browser
  │  (cookie: session=<token>)
  ▼
Gin router (internal/server)
  │  RequireAuth  → resolves session cookie → employee on context
  │  RequireRole  → manager / admin / hr gates
  ▼
Handlers (internal/handlers)         ── one file per feature
  │  validate input, authorize, orchestrate
  ▼
Store (internal/store)               ── domain methods, pgtype ↔ plain Go
  │
  ▼
sqlc Queries (internal/db)           ── typed SQL, generated
  │
  ▼
PostgreSQL

Rendering: handlers render templ components (views/) → HTML.
  Full page  → server-rendered layout + page
  Fragment   → HTMX swaps just the returned component (e.g. #request-list)
Assets: Vite builds views' Tailwind classes + Alpine/HTMX into public/build/;
        the templ layout emits hashed <link>/<script> tags via assets/.
```

### Project layout

```
main.go                     Thin entrypoint → cli.Execute()
internal/
  app/                      Composition root: App struct + Wire injectors (wire_gen.go)
  cli/                      Cobra command tree (root, serve, seed)
  config/                   Environment → typed Config
  db/                       sqlc-generated queries + models (DO NOT EDIT)
  store/                    pgx pool + domain methods (wraps db.Queries)
  auth/                     bcrypt, random passwords, session tokens/cookies, RequireAuth / RequireRole
  leave/                    Pure WorkingDays() business rule (+ unit test)
  seed/                     Re-runnable demo-data seeder (used by `seed` command)
  bootstrap/                Ensures admin + HR accounts exist on serve (emails a random password)
  mailer/                   SMTP transport (net/smtp) behind a small Send interface
  handlers/                 HTTP handlers: auth, dashboard, requests, approvals,
                            employees, calendar, admin (+ router_test.go)
  server/                   Route table + middleware wiring
views/                      templ components (flat package) + view models
assets/                     Vite manifest bridge (dev server vs built bundle)
sql/
  migrations/               golang-migrate SQL (up/down)
  queries/                  sqlc input (query.sql) — no loose SQL at the repo root
sqlc.yaml                   sqlc config (schema: sql/migrations, queries: sql/queries)
web/                        Vite project (package.json, vite.config.js, src/)
public/build/               Vite output (gitignored; regenerate with npm run build)
Makefile                    Common tasks — run `make help`
Dockerfile                  Multi-stage build (assets → binary → alpine runtime)
docker-compose.yml          Prod-like stack: db + migrations + app (+ seed profile)
.env.example                Config template — copy to .env (gitignored, auto-loaded)
docs/learning/              Learning roadmap & per-phase write-ups (phases, learning, project)
```

### Design decisions

- **`internal/` packages by concern** — adding a feature is a predictable walk: a query → a store method → a handler → a templ component.
- **Dependency injection via Wire** — `internal/app` is the composition root. `InitializeApp` (config → store → handlers → router) and `InitializeStore` (DB-only, for `seed`) are Wire-*generated*, so the graph is explicit and compile-checked instead of hand-wired, and the DB pool's cleanup is threaded through automatically.
- **TEXT + CHECK for enums** (`role`, `status`) rather than native Postgres enums — trivial to evolve in a migration; sqlc maps both to Go `string`.
- **Working days snapshotted** on the request at submit time, so later holiday-calendar edits can't retroactively resize an approved request.
- **Balances computed in SQL** (`ListBalances`) via a LEFT JOIN of allocations against `SUM(working_days)` of approved requests.
- **Nullable columns stay in the store layer** — sqlc emits `pgtype.*` for nullable columns; the store translates so handlers deal in plain `int64` / `time.Time`.

---

## Entity-relationship diagram

```mermaid
erDiagram
    employees ||--o{ employees          : "manages (manager_id)"
    employees ||--o{ sessions           : "has"
    employees ||--o{ leave_allocations  : "allocated"
    employees ||--o{ leave_requests     : "submits"
    leave_types ||--o{ leave_allocations : "of type"
    leave_types ||--o{ leave_requests    : "of type"

    employees {
        bigint      id PK
        text        name
        text        email UK
        text        password_hash
        text        role "employee | manager | admin | hr"
        bigint      manager_id FK "nullable → employees.id"
        timestamptz created_at
    }
    sessions {
        text        token PK
        bigint      employee_id FK
        timestamptz created_at
        timestamptz expires_at
    }
    leave_types {
        bigint      id PK
        text        name UK
        int         default_days
        text        color
        timestamptz created_at
    }
    leave_allocations {
        bigint id PK
        bigint employee_id FK
        bigint leave_type_id FK
        int    year
        int    days
    }
    leave_requests {
        bigint      id PK
        bigint      employee_id FK
        bigint      leave_type_id FK
        date        start_date
        date        end_date
        int         working_days "weekdays − holidays, snapshotted"
        text        reason
        text        status "pending | approved | rejected | cancelled"
        bigint      decided_by FK "nullable → employees.id"
        timestamptz decided_at "nullable"
        timestamptz created_at
    }
    public_holidays {
        bigint      id PK
        text        name
        date        holiday_date UK
        timestamptz created_at
    }
```

`leave_allocations` is unique on `(employee_id, leave_type_id, year)`. Deleting an employee cascades to their sessions, allocations, and requests; deleting a manager sets reports' `manager_id` to NULL.

---

## Test users

All seeded accounts share the password **`password`**.

| Email | Name | Role | Reports to |
|---|---|---|---|
| `admin@acme.test` | Dara Admin | **admin** | — |
| `manager@acme.test` | Mona Manager | **manager** | Dara Admin |
| `hr@acme.test` | Hana HR | **hr** | Dara Admin |
| `sam@acme.test` | Sam Employee | employee | Mona Manager |
| `nadia@acme.test` | Nadia Employee | employee | Mona Manager |
| `youssef@acme.test` | Youssef Employee | employee | Mona Manager |

The seed also creates leave types **Annual (25 days)**, **Sick (12)**, **Unpaid (0)**, this-year allocations, a handful of public holidays, and two sample pending requests (from Sam and Nadia) so the manager has something to approve.

**Try this flow:** log in as `manager@acme.test` → **Approvals** → approve Sam's request → log in as `sam@acme.test` → the dashboard shows the Annual balance dropping by the approved working days.

---

## Running the project

### Prerequisites

- **Go 1.25+**
- **Node 20+** (for the Vite asset build)
- **PostgreSQL 12+** (Docker is easiest)
- **[golang-migrate](https://github.com/golang-migrate/migrate) CLI** (`migrate`) — to apply migrations
- **Make** — to use the task runner (recommended)
- Optional (only to regenerate code): **[sqlc](https://sqlc.dev)** and **[templ](https://templ.guide)** CLIs

### Fast path (Make)

```bash
cp .env.example .env    # config (the app has NO built-in DATABASE_URL default)
make db-docker          # start Postgres 16 in Docker and create the database
make setup              # install deps, run migrations, seed data, build assets
make run                # serve on http://localhost:8080
```

`make help` lists every target. Then open **http://localhost:8080** and log in (see [test users](#test-users)).

Configuration comes from the environment or a local `.env` (auto-loaded by the app and by `make`). **`DATABASE_URL` is required** — the app exits with an error if it's unset; there is no hardcoded fallback.

### Step by step (what `make setup` runs under the hood)

```bash
# 1. Postgres + database (skip if you already have one)
docker run --name leave-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres:16
docker exec -it leave-pg createdb -U postgres leave_management

# 2. Point at the database — or `cp .env.example .env` (the app auto-loads it)
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable"

# 3. Apply migrations  (make migrate-up)
migrate -path sql/migrations -database "$DATABASE_URL" up

# 4. Seed demo data     (make seed)
go run . seed

# 5. Build assets → public/build/, gitignored  (make assets)
( cd web && npm install && npm run build )

# 6. Run                (make run)
go run . serve
```

The binary is a small Cobra CLI: `go run . --help` lists the `serve` and `seed` subcommands (`go run . serve`, `go run . seed`).

### Development mode (Vite HMR)

Run Vite and Go side by side so front-end edits hot-reload without a rebuild:

```bash
make web-dev      # terminal 1: Vite dev server on :5173
make serve-dev    # terminal 2: Go in dev mode (VITE_DEV=true), assets from :5173
```

Remember: after editing `.templ` run `make templ`; after editing `sql/queries` or a migration run `make sqlc` (or `make generate` for both).

### Run with Docker (production-like)

A multi-stage `Dockerfile` and `docker-compose.yml` stand up the whole stack — Postgres, a one-shot migration runner, and the app:

```bash
docker compose up -d --build                  # db (healthcheck) → migrate → app
docker compose --profile seed run --rm seed   # optional: load demo data
# → http://localhost:8080   (readiness probe at /healthz)
docker compose down                           # stop (keep data); `down -v` wipes the volume
```

- **`db`** — Postgres 16 with a named volume and a `pg_isready` healthcheck.
- **`migrate`** — runs `migrate up`, then exits; the app waits on it via `depends_on: service_completed_successfully`.
- **`app`** — the built image, running as a non-root user, with a container `HEALTHCHECK` hitting `/healthz`.
- **`seed`** — the same image with the seed entrypoint, gated behind the `seed` profile so it never runs against a real deployment by accident.

Credentials and ports default to `postgres` / `leave_management` / `8080` and are overridable via env (`POSTGRES_PASSWORD`, `DB_PORT`, `APP_PORT`, …) or a `.env` file. Make shortcuts: `make docker-up`, `make docker-seed`, `make docker-logs`, `make docker-down`, `make docker-clean`.

> Assets are baked into the image (Vite build runs in the Dockerfile); rebuild with `--build` to pick up asset changes.

### Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | **required** (no default) | pgx connection string; the app exits if unset |
| `ADDR` | `:8080` | listen address |
| `BASE_URL` | `http://localhost:8080` | externally-reachable root; used to build the login link in bootstrap email |
| `VITE_DEV` | *(unset)* | `true` → serve assets from the Vite dev server (HMR) instead of the built bundle |
| `AUTO_MIGRATE` | *(unset)* | `true` → apply pending migrations on `serve`/`seed` startup |
| `BOOTSTRAP_ADMIN_EMAIL` | *(unset)* | email of the admin account to auto-provision on `serve` (blank → skip) |
| `BOOTSTRAP_HR_EMAIL` | *(unset)* | email of the HR account to auto-provision on `serve` (blank → skip) |
| `SMTP_HOST` / `SMTP_PORT` | *(unset)* / `587` | SMTP relay for the bootstrap credential email |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | *(unset)* | SMTP auth (omit for an unauthenticated relay) |
| `SMTP_FROM` | *(unset)* | `From:` address on outbound mail |

Values are read from the environment or a local `.env` file — copy `.env.example` to `.env` to get started. `.env` is auto-loaded (via `godotenv`) and gitignored, so it never lands in version control or the Docker image; real deployments inject the environment directly. Sessions last 7 days (constant in `internal/config`).

**Bootstrap accounts.** On `serve` startup the app ensures an admin and an HR account exist for `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_HR_EMAIL`. For each one that doesn't exist yet it generates a random password, emails it over SMTP, and only then creates the account — so a failed send aborts startup rather than leaving an account whose password nobody knows. Accounts that already exist are left untouched, and the SMTP transport is only required when an account actually needs creating. This is separate from the `seed` command, which loads demo data for local play. See `internal/bootstrap` and `internal/mailer`.

---

## Routes

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/login` | public | login page |
| POST | `/login` | public | authenticate, set session cookie |
| POST | `/logout` | authed | destroy session |
| GET | `/` | authed | dashboard (balances + recent requests) |
| GET | `/requests` | authed | own requests + submit modal |
| POST | `/requests` | authed | submit a request (HTMX) |
| POST | `/requests/:id/cancel` | authed | cancel own pending request |
| GET | `/calendar` | authed | month calendar (Alpine shell) |
| GET | `/calendar/month` | authed | month grid fragment (HTMX) |
| GET | `/approvals` | manager/admin/hr | pending requests from reports |
| POST | `/approvals/:id/approve` | manager/admin/hr | approve |
| POST | `/approvals/:id/reject` | manager/admin/hr | reject |
| GET | `/employees` | manager/admin/hr | team directory |
| GET | `/employees/:id` | manager/admin/hr | employee profile |
| GET | `/admin` | admin/hr | leave types, holidays, allocations |
| POST | `/admin/leave-types` | admin/hr | add a leave type |
| POST | `/admin/holidays` | admin/hr | add a holiday |
| POST | `/admin/holidays/:id/delete` | admin/hr | remove a holiday |
| POST | `/admin/allocations` | admin/hr | set an employee's yearly allocation |

---

## Testing

```bash
make test              # unit tests (the store integration test is skipped without a DB)
make test-integration  # all tests incl. the DB-gated store test
```

- `internal/leave` — pure unit test of the working-days calculation (weekends + holidays).
- `internal/handlers` — no-DB router test: an unauthenticated request redirects to `/login`.
- `internal/store` — sqlc integration test (create → approve → balances), **skipped** unless `TEST_DATABASE_URL` is set (which `make test-integration` does).

---

## Regenerating generated code

```bash
make sqlc      # after editing sql/queries or a migration → internal/db/
make templ     # after editing a .templ file → *_templ.go
make wire      # after changing providers/injectors → internal/app/wire_gen.go
make generate  # all three
```

---

## Make targets

Run `make` (or `make help`) for the full list. Most useful:

| Target | What it does |
|---|---|
| `make setup` | First-time setup: deps → migrate → seed → build assets |
| `make run` | Run the server (expects assets built) |
| `make build` | Regenerate code, build assets, compile `bin/leave-management` |
| `make web-dev` / `make serve-dev` | Vite HMR + Go dev mode (two terminals) |
| `make generate` | `sqlc` + `templ` generation |
| `make migrate-up` / `make migrate-down` | Apply / roll back migrations |
| `make migrate-create name=...` | Scaffold a new migration pair in `sql/migrations` |
| `make seed` | Seed demo data |
| `make check` | Regenerate, `vet`, and `test` — the pre-commit sweep |
| `make docker-up` / `docker-down` | Run / stop the full Docker stack |
| `make docker-seed` | Seed the Dockerized DB (opt-in) |
| `make clean` | Remove `bin/` and `public/build/` |

---

## Limitations / not in scope

Deliberately kept simple for a learning project: no self-registration or password reset, no CSRF token (relies on `SameSite=Lax`), cookie `Secure` is off for local HTTP (flip it behind TLS), no half-day leave, no email notifications, no multi-level approval chains, no pagination. Balances are per calendar year.
