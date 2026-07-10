# Leave Management

A small but complete **employee leave-management** web app: employees request leave, managers approve or reject, balances are tracked per leave type, and there's a team calendar and an admin area for leave types, allocations, and public holidays.

It was built as a **progressive learning project** — each phase (0–8) introduced exactly one piece of the stack on "hello dzovi" demos, and **Phase 9** assembles all of it into this real app. See [`docs/learning/phases.md`](docs/learning/phases.md) for the roadmap and [`docs/learning/learning.md`](docs/learning/learning.md) for the per-phase write-ups.

> **Server-rendered, no SPA.** HTML comes from the server (templ); HTMX swaps fragments for server-driven interactions; Alpine handles client-only UI state (modals, toasts, calendar navigation). There is no JSON API and no client-side framework.

---

## Tech stack

| Layer | Technology | Notes |
|---|---|---|
| Language | **Go 1.25** | single binary |
| HTTP | **Gin** | routing + middleware groups |
| HTML templating | **templ** | type-safe Go components (`.templ` → generated `.go`) |
| Styling | **Tailwind CSS v4** | utility-first, purged at build time |
| Server interactions | **HTMX** | HTML-over-the-wire fragment swaps |
| Client UI state | **Alpine.js** | modals, toast stack, calendar month nav |
| Database | **PostgreSQL** | via `pgx/v5` connection pool |
| DB access | **sqlc** | typed Go generated from `sql/queries/` + migrations |
| Migrations | **golang-migrate** | `sql/migrations/*.sql` |
| Task runner | **Make** | `make help` for all targets |
| Asset pipeline | **Vite** | bundles/hashes JS + CSS into `public/build/` |
| Auth | **bcrypt** (`golang.org/x/crypto`) | password hashing + Postgres-backed sessions |

---

## Features

- **Authentication** — email + password login, bcrypt hashing, server-side sessions in Postgres, HttpOnly/SameSite cookie, logout that truly invalidates the session.
- **Roles** — `employee`, `manager`, `admin`, enforced by middleware plus per-request ownership checks.
- **Dashboard** — your balances per leave type and your recent requests; managers see a pending-approval badge.
- **Leave requests** — submit via an Alpine modal + HTMX form; working days are computed excluding weekends and public holidays and snapshotted on the request. Cancel your own pending requests.
- **Approvals** — managers approve/reject pending requests from their direct reports (admins can act on anyone); cards swap out live with a toast.
- **Balances** — computed as *allocated days − approved working days* for the year (no denormalized counter).
- **Team** — managers see their reports, admins see everyone; click through to a profile with balances + request history.
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
  │  RequireRole  → manager / admin gates
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
main.go                     Thin bootstrap: config → store → assets → router → run
cmd/seed/main.go            Re-runnable data seeder
internal/
  config/                   Environment → typed Config
  db/                       sqlc-generated queries + models (DO NOT EDIT)
  store/                    pgx pool + domain methods (wraps db.Queries)
  auth/                     bcrypt, session tokens/cookies, RequireAuth / RequireRole
  leave/                    Pure WorkingDays() business rule (+ unit test)
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
docs/learning/              Learning roadmap & per-phase write-ups (phases, learning, project)
```

### Design decisions

- **`internal/` packages by concern** — adding a feature is a predictable walk: a query → a store method → a handler → a templ component.
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
        text        role "employee | manager | admin"
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
make db-docker    # start Postgres 16 in Docker and create the database
make setup        # install deps, run migrations, seed data, build assets
make run          # serve on http://localhost:8080
```

`make help` lists every target. Then open **http://localhost:8080** and log in (see [test users](#test-users)).

The app defaults to `postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable`; override with `DATABASE_URL` (e.g. `make migrate-up DATABASE_URL=...`).

### Step by step (what `make setup` runs under the hood)

```bash
# 1. Postgres + database (skip if you already have one)
docker run --name leave-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres:16
docker exec -it leave-pg createdb -U postgres leave_management

# 2. Point at the database
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable"

# 3. Apply migrations  (make migrate-up)
migrate -path sql/migrations -database "$DATABASE_URL" up

# 4. Seed demo data     (make seed)
go run ./cmd/seed

# 5. Build assets → public/build/, gitignored  (make assets)
( cd web && npm install && npm run build )

# 6. Run                (make run)
go run .
```

### Development mode (Vite HMR)

Run Vite and Go side by side so front-end edits hot-reload without a rebuild:

```bash
make web-dev      # terminal 1: Vite dev server on :5173
make serve-dev    # terminal 2: Go in dev mode (VITE_DEV=true), assets from :5173
```

Remember: after editing `.templ` run `make templ`; after editing `sql/queries` or a migration run `make sqlc` (or `make generate` for both).

### Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable` | pgx connection string |
| `ADDR` | `:8080` | listen address |
| `VITE_DEV` | *(unset)* | `true` → serve assets from the Vite dev server (HMR) instead of the built bundle |

Sessions last 7 days (constant in `internal/config`).

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
| GET | `/approvals` | manager/admin | pending requests from reports |
| POST | `/approvals/:id/approve` | manager/admin | approve |
| POST | `/approvals/:id/reject` | manager/admin | reject |
| GET | `/employees` | manager/admin | team directory |
| GET | `/employees/:id` | manager/admin | employee profile |
| GET | `/admin` | admin | leave types, holidays, allocations |
| POST | `/admin/leave-types` | admin | add a leave type |
| POST | `/admin/holidays` | admin | add a holiday |
| POST | `/admin/holidays/:id/delete` | admin | remove a holiday |
| POST | `/admin/allocations` | admin | set an employee's yearly allocation |

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
make generate  # both
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
| `make clean` | Remove `bin/` and `public/build/` |

---

## Limitations / not in scope

Deliberately kept simple for a learning project: no self-registration or password reset, no CSRF token (relies on `SameSite=Lax`), cookie `Secure` is off for local HTTP (flip it behind TLS), no half-day leave, no email notifications, no multi-level approval chains, no pagination. Balances are per calendar year.
