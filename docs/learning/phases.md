# Leave Management — Progressive Learning Phases

A step-by-step path through the stack: **Go, Gin, Postgres, sqlc, templ, Tailwind, HTMX, Alpine.js, Vite**.

Each phase adds **one** new piece of tech on top of the previous one. The goal is that you can feel exactly what each tool brings to the table — what problem it solves, and what the code looked like before it existed.

The long-term target is a working **leave-management** app (employees request leave, managers approve, balances tracked). We don't build that until Phase 9. Everything before that is a "hello dzovi" style demo so the tech stays in focus.

---

## Phase 0 — Go + Gin, JSON only

**New tech:** Go, Gin
**Purpose:** Get a working HTTP server with one endpoint. Understand the Gin handler signature and how routing works.

**What you build:**
- `GET /hello` returns `{"message": "hello dzovi"}` with `Content-Type: application/json`.

**Things to learn:**
- `go mod init`, project layout (`main.go` is enough)
- `gin.Default()`, route registration, `c.JSON(200, ...)`
- Running with `go run .`

**Why it matters:** This is the smallest possible useful Go web app. Everything else is layered on top.

---

## Phase 1 — Gin returns HTML (stdlib `html/template`)

**New tech:** none new — using Go's built-in `html/template`
**Purpose:** See that the same handler can serve HTML instead of JSON. This makes the next phase (templ) a meaningful comparison.

**What you build:**
- `GET /hello` now returns an HTML page that says "hello dzovi".
- Use Go's stdlib `html/template` package via `r.LoadHTMLGlob("templates/*")` and `c.HTML(...)`.
- Pass data from the handler into the template (e.g. the name "dzovi").

**Things to learn:**
- The difference between `c.JSON` and `c.HTML`
- Template files, `{{ .Name }}` interpolation
- Why stdlib templates are awkward (no type safety, runtime errors, string-based)

**Why it matters:** You need to *feel* the pain of stringly-typed templates before templ becomes appealing.

---

## Phase 2 — Replace `html/template` with **templ**

**New tech:** templ
**Purpose:** Type-safe, component-based HTML written in Go. See the upgrade from raw templates.

**What you build:**
- Delete the `templates/` directory.
- Write `hello.templ` with a `Hello(name string)` component.
- Run `templ generate` to produce `.go` files.
- Handler calls `hello.Hello("dzovi").Render(c.Request.Context(), c.Writer)`.

**Things to learn:**
- Installing `templ` CLI (`go install github.com/a-h/templ/cmd/templ@latest`)
- `.templ` syntax: it's Go + HTML
- Components are just functions — you can compose them, pass props
- Generated code: open the `_templ.go` files to demystify it
- Add `templ generate` to your workflow (or `templ generate --watch`)

**Why it matters:** This is the moment "Go can do JSX-style components" clicks. Compile-time errors instead of runtime template panics.

---

## Phase 3 — Add **Tailwind** (CDN first)

**New tech:** Tailwind CSS (via CDN)
**Purpose:** Style the page without leaving HTML. Don't worry about the build pipeline yet — that's Phase 8.

**What you build:**
- A layout component in templ with `<head>` that includes the Tailwind Play CDN script.
- Style the hello page: centered card, nice typography, a button (does nothing yet).

**Things to learn:**
- Utility-first CSS philosophy (`text-2xl font-bold p-4` vs writing CSS classes)
- Tailwind's mental model: spacing scale, color scale, responsive prefixes (`md:`, `lg:`)
- Why the CDN is fine for learning but bad for production (huge file, no purging)

**Why it matters:** Tailwind is fast to learn but you need to *use* it on a real page to internalize it. Saving the build setup for later keeps the focus on the classes.

---

## Phase 4 — Add **HTMX**

**New tech:** HTMX
**Purpose:** Interactivity without writing JavaScript. The server returns HTML fragments; HTMX swaps them in.

**What you build:**
- A button on the page: "Greet me again". `hx-get="/greeting"` returns a new `<div>` with a fresh greeting (maybe includes timestamp), and HTMX swaps it into the page.
- A small form: enter a name, `hx-post="/greet"`, response replaces a target div.

**Things to learn:**
- `hx-get`, `hx-post`, `hx-target`, `hx-swap`
- The server returns **HTML partials**, not JSON — your Gin handler renders a templ component for just the fragment.
- Compare mentally to "fetch + React state update" — HTMX is the no-JS equivalent.

**Why it matters:** This is where the templ + Gin combo really pays off. Every interaction is just "another handler that returns a component." No JSON contracts, no client-side state.

---

## Phase 5 — Add **Alpine.js**

**New tech:** Alpine.js
**Purpose:** Tiny client-side state for cases where round-tripping to the server is overkill (dropdowns, modals, toggles, form validation hints).

**What you build:**
- A dropdown menu in the page header that toggles open/closed with Alpine (`x-data="{ open: false }"`, `@click="open = !open"`, `x-show="open"`).
- A modal dialog triggered by a button.
- Optional: a form input that shows a live character count.

**Things to learn:**
- `x-data`, `x-show`, `x-bind`, `x-on` (or `@`), `x-model`
- When to reach for Alpine vs HTMX:
  - HTMX = server owns the state, returns HTML
  - Alpine = ephemeral UI state, never needs to hit the server
- They compose beautifully: HTMX swaps content, Alpine handles the modal-open/closed state around it.

**Why it matters:** You'll understand the "HTML over the wire" stack: HTMX for server-owned dynamism, Alpine for client-only sugar. Together they replace most of what people use React for.

---

## Phase 6 — Add **Postgres** (raw SQL with pgx)

**New tech:** Postgres, `pgx` driver
**Purpose:** Persist data. Write SQL by hand so you feel what sqlc will replace later.

**What you build:**
- A `users` table (`id`, `name`, `created_at`).
- `POST /users` inserts a user. `GET /users` lists them.
- Render the list with a templ component.
- Hand-written SQL using `pgxpool` and `Query`/`Scan`.

**Things to learn:**
- Running Postgres locally (Docker is easiest: `docker run -p 5432:5432 postgres`)
- A simple migration approach — for now, a `schema.sql` you run manually, or `goose`/`golang-migrate` if you want to learn migrations early
- Connection pooling with `pgxpool.New`
- The pain of manual `rows.Scan(&u.ID, &u.Name, &u.CreatedAt)` — fragile, no type safety, easy to misalign

**Why it matters:** You need to write the boilerplate before sqlc removes it. Otherwise sqlc just feels like magic.

---

## Phase 7 — Replace raw SQL with **sqlc**

**New tech:** sqlc
**Purpose:** Generate type-safe Go code from `.sql` files. No more manual `Scan` calls.

**What you build:**
- Add `sqlc.yaml` and a `query.sql` file with annotated queries (`-- name: GetUser :one`, etc.)
- Run `sqlc generate`.
- Replace the hand-written DB code with calls to the generated `Queries` struct.

**Things to learn:**
- `sqlc.yaml` config (engine: postgresql, schema, queries paths)
- Query annotations: `:one`, `:many`, `:exec`
- How sqlc parses your schema to know column types
- The generated code is just Go — open it and read it

**Why it matters:** This is the most Go-idiomatic database layer there is. You write SQL (no DSL), and you get fully typed Go functions back. Migration cost from Phase 6 is small but the difference in feel is huge.

---

## Phase 8 — Add **Vite** for the asset pipeline

**New tech:** Vite
**Purpose:** Replace the Tailwind CDN with a real build. Bundle Alpine. Get fast HMR during development.

**What you build:**
- A `web/` (or `frontend/`) directory with `package.json`, `vite.config.js`.
- `app.css` that imports Tailwind. `app.js` that imports Alpine.
- Vite outputs a hashed bundle + a manifest into `public/build/`.
- Your templ layout reads `manifest.json` (at startup) and emits the correct `<link>` and `<script>` tags.
- In dev, you can run `vite` alongside Go and use HMR.

**Things to learn:**
- Vite config basics, entry points, manifest
- Tailwind via PostCSS (proper purging now happens — your CSS goes from ~3MB CDN to ~10KB)
- How to serve `public/build/` from Gin (`r.Static("/build", "./public/build")`)
- Splitting concerns: Go serves HTML, Vite serves static assets

**Why it matters:** This is the "production-ready" piece. You'll appreciate why people bother with a build tool only after you've shipped a few pages without one.

---

## Phase 9 — Build the actual Leave Management app

**New tech:** none — assemble everything
**Purpose:** Use the full stack on a real feature set. This is where you stop saying "hello dzovi" and build something you'd actually use.

**Suggested scope:**
- **Auth** — sessions or a simple login (could be its own mini-phase if you want)
- **Employees** — list, profile pages
- **Leave requests** — employee submits (form via HTMX), manager sees pending, approves/rejects
- **Leave balance** — annual leave, sick leave, computed from approved requests
- **Calendar view** — Alpine for client-side month navigation, HTMX for fetching the data per month
- **Notifications** — toast on approve/reject using Alpine
- **Admin** — manage leave types, public holidays

**Things to revisit:**
- Project structure (`internal/`, handlers, services, db)
- Error handling and middleware
- Testing — at least one handler test, one sqlc query test
- Deployment — single binary + static assets, easy to ship

**Why it matters:** Every phase before this was a single concept in isolation. Now you have to make decisions: where does business logic live, how do you organize templ components, when do you reach for Alpine vs HTMX, etc. This is where the stack stops being a checklist and becomes a way of working.

---

## How to use this document

- **Don't skip phases.** The whole point is to feel the delta each tool adds.
- **Commit at the end of every phase.** You'll want to look back at "what did the code look like before sqlc."
- **If a phase feels trivial, do it anyway.** Phase 0 and 1 take 20 minutes combined, but they anchor everything.
- **Take notes per phase** — one paragraph on "what surprised me." That's where the real learning sticks.
