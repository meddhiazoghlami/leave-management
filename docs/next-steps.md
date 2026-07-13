# Next Steps — toward an Odoo-style Time Off app

## Vision

Today the app enforces **fixed, hardcoded policy**: the weekend is Saturday/Sunday, the
"leave year" is the calendar year, allocations are flat days-per-year set by hand,
approval is a single manager step, and leave types only carry a name/color/default days.

The goal is to make it work like **Odoo's Time Off**: the admin configures *everything* —
leave types, how many days accrue and how often (per month or per year), sick-leave
limits, public holidays, and the working week (including **Friday/Saturday** weekends used
in many countries) — with **no policy baked into code**. Every rule becomes a
database-backed setting editable from the admin UI.

## How your requirements map to the milestones

| You asked for | Delivered in |
|---|---|
| Configure the weekend (e.g. Fri/Sat) | **M1** — working week |
| Configure public holidays / public days | **M4** (basic version exists today) |
| Configure amount of days per **year or month** | **M3** — allocations & accrual |
| Configure number of **sick leaves** per year/month | **M3** (per-leave-type accrual) |
| Admin can configure everything from the UI | **M6** — admin hub, threaded through M1–M5 |

## Guiding principles

- **Config over code.** Anything currently hardcoded becomes a row in the DB with an admin screen.
- **Keep the stack and the workflow.** templ + HTMX + Alpine for UI, sqlc for queries, one numbered migration per change in `sql/migrations`, regenerate with `make generate` (sqlc + templ + wire).
- **Each milestone ships end-to-end:** schema → `sql/queries` → `internal/store` → `internal/handlers` → `views` admin UI → tests.
- **Derive, don't store, policy outcomes.** Balances are computed from the current config (allocation/accrual − approved usage), except the per-request `working_days` which stays snapshotted at submit time.
- **New collaborators get Wire providers** in `internal/app`.

## The hardcoded assumptions we're removing (the gap)

- Weekend = Sat/Sun — `internal/leave/workdays.go` (`WorkingDays` hardcodes `time.Saturday/Sunday`).
- Leave year = Jan–Dec — `ListBalances` in `internal/store` filters by `EXTRACT(YEAR ...)`.
- Allocation = flat `days` per `year`, entered manually — `leave_allocations`; no accrual.
- Approval = one manager decision — `leave_requests.status`; no HR / none / both.
- Leave types carry only `name, default_days, color` — `leave_types`.
- No company / working-schedule concept at all.

---

## Milestones

### M0 — Prep: make working-days configurable (do this first)

`WorkingDays` is a dependency of M1, M3 and M4, so refactor it before anything else.

- **Code:** change `leave.WorkingDays(start, end, holidays)` →
  `leave.WorkingDays(start, end, workingWeekdays map[time.Weekday]bool, holidays map[string]bool)`.
  Remove the hardcoded Sat/Sun switch; a day counts if its weekday is in the working set and it's not a holiday.
- **Tests:** extend `workdays_test.go` with a Fri/Sat-weekend case (Sun–Thu ⇒ 5 days).

### M1 — Configurable working week & company settings (the Fri/Sat weekend)

**Goal:** the admin picks which weekdays are working and when the leave year starts; all working-days math uses it.

- **Schema (new migration):**
  - `companies` (or a single `settings` row for now): `id, name, country_code, timezone, leave_year_start_month INT DEFAULT 1`.
  - `working_schedules (id, name, company_id)` + `working_schedule_days (schedule_id, weekday SMALLINT 0–6, is_working BOOLEAN)`.
  - `employees.working_schedule_id` (nullable → falls back to company default).
- **Code:** load the working-weekday set from the schedule; pass it into `WorkingDays` on submit. Replace the calendar-year assumption in `ListBalances` with the configured leave-year window.
- **Admin UI:** a **General** settings section — Mon–Sun working-day checkboxes, leave-year start month.
- **Acceptance:** set the weekend to Fri/Sat, submit a Sun–Thu request → counts **5** working days; Fri/Sat and holidays excluded.

### M2 — Rich leave-type configuration

**Goal:** leave types carry real policy instead of just a default number.

- **Schema:** `ALTER TABLE leave_types ADD`
  - `unit TEXT CHECK (unit IN ('day','half_day','hour')) DEFAULT 'day'`
  - `requires_allocation BOOLEAN DEFAULT true` (false ⇒ unlimited, e.g. Unpaid)
  - `allow_negative BOOLEAN DEFAULT false`, `max_negative INT DEFAULT 0`
  - `is_paid BOOLEAN DEFAULT true`
  - `approval TEXT CHECK (approval IN ('none','manager','hr','both')) DEFAULT 'manager'`
  - `requires_document BOOLEAN DEFAULT false`, `active BOOLEAN DEFAULT true`
- **Code:** submit/approve honor these — unlimited types skip the balance check; requests can go negative within `max_negative`; `approval` drives routing (see M5).
- **Admin UI:** a full leave-type create/edit form; list shows the config.
- **Acceptance:** an "Unpaid" type needs no allocation; an `approval='none'` type auto-approves on submit.

### M3 — Allocations & Accrual plans  ← the core of your request

**Goal:** configure *how many* days accrue and *how often* (per month or per year), per leave type — including sick-leave limits.

- **Schema:**
  - `accrual_plans (id, name, leave_type_id, carryover TEXT CHECK IN ('lost','full','capped'), carryover_cap INT, active)`
  - `accrual_levels (id, plan_id, start_after_months INT, rate NUMERIC, frequency TEXT CHECK IN ('daily','weekly','monthly','yearly'), cap NUMERIC)` — supports milestones (e.g. more days after 1 year).
  - `leave_allocations ADD mode TEXT CHECK IN ('regular','accrual') DEFAULT 'regular', plan_id BIGINT NULL, accrued NUMERIC DEFAULT 0, last_accrued_on DATE NULL`.
- **Code:**
  - Keep manual grants as `mode='regular'` (today's behavior).
  - **Accrual engine:** for `mode='accrual'`, grant `rate` per elapsed period since `last_accrued_on`, respecting the level's `start_after_months` and `cap`.
  - **New Cobra subcommand `leave-management accrue`** (fits the CLI we just built) run by cron to advance accruals; **`expire-carryover`** at the leave-year rollover to apply `carryover` policy.
- **Admin UI:** Accrual Plans CRUD; attach a plan to a leave type / allocation. Configure "Annual = 2.5 days/month", "Sick = 1 day/month" or "12/year".
- **Acceptance:** running `accrue` monthly increments the Annual balance by 2.5; Sick is capped at its yearly limit; carryover is applied per policy at year end.

### M4 — Holidays & calendars

**Goal:** richer public-holiday management (basic CRUD already exists).

- **Schema:** `public_holidays ADD country_code TEXT, recurring BOOLEAN DEFAULT false` (annual), optional `working_schedule_id` scope.
- **Code:** expand recurring holidays across years; feed them into `WorkingDays`.
- **Admin UI:** recurring flag on the holiday form; optional "import a country's holidays" template (seed a set).
- **Acceptance:** a recurring holiday shows every year; editing weekend/holidays affects *future* requests only (snapshots protect the past).

### M5 — Configurable approval workflow

**Goal:** approval steps depend on the leave type (`none` / `manager` / `hr` / `both`).

- **Schema:** add an HR decision to `leave_requests` (`hr_decided_by`, `hr_decided_at`) or a generic `leave_request_approvals` table for a clean audit trail.
- **Code:** a small state machine: `pending → (manager) → (hr) → approved`; `none` auto-approves; balances only count `approved`.
- **Admin UI:** the per-type `approval` setting (from M2) + an HR approvals queue view.
- **Acceptance:** `approval='both'` needs manager **then** HR; `approval='none'` skips straight to approved.

### M6 — Admin configuration hub & RBAC

**Goal:** one place to configure everything, properly guarded.

- **Code:** grow `/admin` into sections — **General** (M1), **Leave Types** (M2), **Accrual Plans** (M3), **Holidays** (M4), **Approvals** (M5) — all HTMX CRUD like the current admin page. Add a config **audit log**. Consider splitting an **HR** role from **admin**.
- **Acceptance:** the admin changes any policy from the UI and it takes effect with no redeploy.

### M7 — Scheduling, carryover, and polish

- **Scheduling:** run `accrue` (e.g. monthly) and `expire-carryover` (leave-year start) via a k8s CronJob, a compose sidecar, or a scheduler — all just invoking the Cobra subcommands.
- **Half-day / hourly leave:** honor `leave_types.unit` in the request form and the duration math.
- **Extras:** email notifications, CSV exports/reports, richer calendar (per-team filters), document upload for `requires_document` types.

---

## Cross-cutting work

- **Migrations discipline:** one numbered up/down pair per change in `sql/migrations`; never edit an applied migration. `make generate` after each.
- **Balances rewrite:** `ListBalances` must use the configured leave-year window (M1) + accrued amounts (M3) + carryover (M7), not the calendar year + flat allocation.
- **Data migration / backfill:** create a default company + Mon–Fri schedule and attach existing employees; give existing `leave_types` sane defaults for the new columns so nothing breaks.
- **Tests:** unit (working-days with a configurable weekend, accrual math, carryover), integration (balance after N months of accrual), handler (admin config round-trips).
- **DI:** every new store/service gets a provider in `internal/app`; re-run `make wire`.

## Suggested order

**M0 (WorkingDays refactor) → M1 → M2 → M3 (headline) → M4 → M5 → M6 → M7.**
M3 is where your "days per year/month" and "sick leaves per year/month" land; M1 delivers the Fri/Sat weekend; M6 makes it all admin-editable.

## Out of scope (for now)

Multi-company / multi-tenant, timesheet/attendance integration, payroll, and full hourly working-time contracts beyond a simple day/half-day/hour `unit`.
