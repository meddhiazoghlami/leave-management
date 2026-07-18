# Learning Journal

Per-phase notes: what each new tool is, what changed from the previous phase, what value it added, and what trade-offs it introduced. Written *after* each phase is implemented and working — not in advance.

---

## Phase 0 — Go + Gin, JSON only

**Status:** Baseline. No previous phase to diff against — this entry just defines what we have.

### Tech: Go

Go is a statically typed, compiled language built at Google around 2009. Designed for simplicity, fast compilation, and easy concurrency (goroutines, channels). It has a tiny standard library philosophy: small surface area, opinionated tooling (`go fmt`, `go mod`, `go test` all built in).

Key things to internalize early:
- A **module** is the unit of dependency management. `go mod init <path>` creates `go.mod`.
- A **package** is one directory of `.go` files sharing a name. `package main` with a `func main()` makes an executable.
- Dependencies are declared in `go.mod`; transitive ones are tracked as `// indirect` until you import them directly. `go mod tidy` cleans this up.

### Tech: Gin

Gin is a Go web framework — thin, fast, and very popular. It sits on top of Go's stdlib `net/http`. Compared to writing raw `net/http`, Gin gives you:
- A **router** with parameterized paths (`/users/:id`)
- **Middleware** pipeline (logging, recovery, auth, etc.)
- A **`*gin.Context`** object passed to every handler — wraps request + response, has helpers like `c.JSON`, `c.HTML`, `c.Param`, `c.Bind`
- `gin.Default()` returns a router with logger + panic-recovery middleware already attached

It is **not** a batteries-included framework like Rails. There's no ORM, no template system of its own, no auth — those are choices you make. That's exactly what we want here.

### What got built

```
.
├── go.mod
├── go.sum
└── main.go    ← single file, ~12 lines
```

`main.go`:
```go
package main

import "github.com/gin-gonic/gin"

func main() {
    r := gin.Default()
    r.GET("/hello", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "hello dzovi"})
    })
    r.Run(":8080")
}
```

### What I observed

- `go run .` compiles and runs in one step — no `node_modules`, no setup ceremony.
- Gin prints its own structured log line per request (`[GIN] 200 | 60µs | GET "/hello"`) thanks to `gin.Default()`'s logger middleware.
- `gin.H` is just `map[string]any` — a shorthand for ad-hoc JSON responses.
- `c.JSON(200, ...)` automatically sets `Content-Type: application/json; charset=utf-8` and serializes the value. Verified with `curl -i`.

### Value added (vs. nothing)

- A working HTTP server in **12 lines** of code. No config files.
- Zero-cost JSON serialization — the handler doesn't even mention "json" beyond the method name.
- Compiled binary: `go build` produces a single static executable. No runtime to install on the server.

### Trade-offs / what's missing

- Responses are JSON only — browsers see raw text, not a rendered page. (Phase 1 fixes this.)
- No persistence — restart the server, all state is gone. (Phase 6.)
- No structure beyond `main.go` — fine for now, but a real app needs handlers/services/db split out.
- `gin.H{...}` is untyped — typos in keys won't be caught at compile time. For real APIs, you'd use a struct with json tags.

### Commands cheat-sheet

```bash
go mod init github.com/meddhiazoghlami/leave-management  # once, at project start
go get github.com/gin-gonic/gin                # add a dependency
go mod tidy                                     # clean up indirect/unused deps
go run .                                        # compile + run from current dir
go build -o leave-management                    # produce a binary
```

---

## Phase 1 — Gin returns HTML (stdlib `html/template`)

**Diff from Phase 0:** same endpoint, same server, but the response is now a rendered HTML page instead of JSON. No new dependency — we used Go's stdlib.

### Tech: `html/template` (Go stdlib)

`html/template` is Go's built-in HTML templating package. It is a sibling of `text/template` — same syntax (`{{ .Field }}`, `{{ if }}`, `{{ range }}`), but with **context-aware auto-escaping**: it knows whether you're inside an HTML element, an attribute, a JS block, or a URL, and escapes accordingly. This is its biggest selling point — XSS protection is on by default.

Key concepts:
- Templates are **parsed at runtime** from files (or strings).
- Data is passed in as a single argument (any type — usually a struct or a map). Inside the template, `{{ . }}` refers to that root.
- Action delimiters are `{{ }}`. Comments are `{{/* ... */}}`.
- Templates can be **named**, **composed** (`{{ template "name" . }}`), and have **functions** registered.

### Tech: Gin's HTML helpers

Gin doesn't ship its own template engine — it wraps stdlib `html/template`:
- `r.LoadHTMLGlob("templates/*")` parses every matching file once at startup and registers them under their **filename** (not file path) as the template name.
- `c.HTML(status, "filename", data)` looks up the template by name, executes it with `data`, and writes the result with `Content-Type: text/html; charset=utf-8`.

### What changed in the code

```diff
- import "github.com/gin-gonic/gin"
+ import (
+   "time"
+   "github.com/gin-gonic/gin"
+ )

  r := gin.Default()
+ r.LoadHTMLGlob("templates/*")

  r.GET("/hello", func(c *gin.Context) {
-   c.JSON(200, gin.H{"message": "hello dzovi"})
+   c.HTML(200, "hello.html", gin.H{
+     "Name":       "dzovi",
+     "RenderedAt": time.Now().Format(time.RFC3339),
+   })
  })
```

Plus a new file: `templates/hello.html` with `{{ .Name }}` and `{{ .RenderedAt }}` interpolation.

### What I observed

- `Content-Type` flipped from `application/json` to `text/html; charset=utf-8` automatically.
- The `+` in the RFC3339 timezone (`+01:00`) was rendered as `&#43;01:00` in the response. That's `html/template` doing **context-aware HTML escaping** — anything that *could* be interpreted as markup gets entity-encoded. Same data via `c.JSON` would have stayed as `+`. This is the safety net.
- Templates are parsed once at startup. If you edit `hello.html` and reload the browser, you see *no change* — the parsed templates are cached. You have to restart the server. (Real apps fix this by reloading templates in dev mode, but that's friction.)

### Value added

- The browser now renders a real page — `<h1>`, `<p>`, you could add a `<form>` next.
- Auto-escaping protects against XSS without any code from us. Pass `Name = "<script>alert(1)</script>"` and you'd see the literal text, not a popup.
- The handler is still tiny — Gin's `c.HTML(...)` is just as ergonomic as `c.JSON(...)`.

### Trade-offs / pain (this is the point — feel this before Phase 2)

1. **No compile-time checks — and worse, silent failures with maps.** Typo'd `{{ .Nmae }}` in the template while passing a `gin.H` (which is `map[string]any`). The server returned `200 OK` with `<h1>hello </h1>` — empty string. No error logged, no panic, no warning. The bug only surfaces if you eyeball the rendered page. (With a struct + the `missingkey=error` option you can force a runtime error, but the default is silent.) This is the most insidious failure mode of stdlib templates.
2. **Stringly typed data.** I'm passing `gin.H{...}` (a `map[string]any`). The template assumes keys exist. Refactor a field name and nothing fails until the page is hit.
3. **Template name = filename.** `LoadHTMLGlob("templates/*")` registers `hello.html` as the literal string `"hello.html"`. If two folders have files with the same name, they collide.
4. **No IDE support.** My editor sees `hello.html` as plain HTML; it can't autocomplete `{{ .Name }}` or jump to the Go code that supplies it.
5. **Composition is awkward.** Layouts and partials require `{{ define "name" }}` blocks, `{{ template "name" . }}` invocations, and careful glob ordering. Doable, but verbose.

**Every one of these pains is what templ (Phase 2) fixes.**

### Commands cheat-sheet (additions)

```bash
# Nothing new — html/template is stdlib, no install, no codegen.
# Just `go run .` after editing main.go or templates/*.html.
```

---

## Phase 2 — Replace `html/template` with **templ**

**Diff from Phase 1:** the same `/hello` endpoint and the same rendered HTML, but the template is now a typed Go function generated from a `.templ` file instead of a runtime-parsed `.html` string.

### Tech: templ

templ is a **type-safe HTML templating language for Go**, created by Adrian Hesketh. It consists of two pieces:

1. **A small DSL** in `.templ` files — Go syntax + HTML mixed together. Looks a lot like JSX, but for Go.
2. **A CLI** (`templ generate`) that parses `.templ` files and emits regular `.go` files (one alongside each `.templ`, named `*_templ.go`).

Each `templ Foo(...) { ... }` block becomes a Go function `func Foo(...) templ.Component`. A `templ.Component` has a `Render(ctx context.Context, w io.Writer) error` method that writes HTML directly to the response.

Key design choices:
- **No new runtime engine.** Templates compile to plain Go. At request time, you're just calling Go functions and writing strings to the response writer.
- **Composable like Go functions.** Components take typed arguments; you can call one component from another (`@OtherComponent(arg)`). No more "register partials" ceremony.
- **Context-aware escaping.** templ escapes interpolated expressions by default, similar to `html/template`, but with sharper awareness of attribute vs text vs URL contexts.

### What changed in the code

```diff
- import "github.com/gin-gonic/gin"
+ import (
+   "time"
+   "github.com/meddhiazoghlami/leave-management/views"
+   "github.com/gin-gonic/gin"
+ )

  r := gin.Default()
- r.LoadHTMLGlob("templates/*")

  r.GET("/hello", func(c *gin.Context) {
-   c.HTML(200, "hello.html", gin.H{
-     "Name":       "dzovi",
-     "RenderedAt": time.Now().Format(time.RFC3339),
-   })
+   component := views.Hello("dzovi", time.Now().Format(time.RFC3339))
+   c.Status(200)
+   c.Header("Content-Type", "text/html; charset=utf-8")
+   _ = component.Render(c.Request.Context(), c.Writer)
  })
```

Project layout shift:
- **Removed:** `templates/hello.html` and the whole `templates/` directory.
- **Added:** `views/hello.templ` (source), `views/hello_templ.go` (generated, committed).

### What the generated code looks like

`templ generate` produced `views/hello_templ.go` — ~65 lines of straightforward Go: it grabs the response buffer, writes the static HTML chunks as plain string literals, and for each `{ name }` / `{ renderedAt }` interpolation it calls `templ.JoinStringErrs(...)` + `templ.EscapeString(...)` and writes the result. No parsing, no reflection at request time. Worth opening once to demystify it.

### What I observed

- The handler no longer mentions a filename. `views.Hello(...)` is just a function call — IDE go-to-definition works, autocompletion shows the parameters with their types.
- Response is `Content-Type: text/html`, `Transfer-Encoding: chunked` — templ streams to the response writer instead of buffering a full string. (Phase 1 had a fixed `Content-Length`.)
- Escaping is still on: dangerous strings would be encoded. But it's slightly less aggressive than stdlib in cosmetic cases — the `+01:00` timezone came through as `+01:00` instead of `&#43;01:00`. Still safe, less noise.
- `c.HTML(...)` is no longer used. We dropped to `c.Status` + `c.Header` + `component.Render`. There are third-party adapters (`gin-templ`) that re-add a one-liner, but it's optional.

### Value added (the payoff for the typo from Phase 1)

1. **Compile-time safety.** If I rename the `name` parameter and forget to update the call site, `go build` fails. If I typo `{ nmae }` in the template, `templ generate` fails *before* a single byte ever reaches the server. The Phase 1 silent-empty-string bug is structurally impossible here.
2. **Real Go types.** Parameters have types (`name string`, `renderedAt string`). I could change `renderedAt` to `time.Time` and format it inside the component — no stringly conversion at the boundary.
3. **Components are functions.** Composition is just calling them: `@Layout("Hello") { @Hello("dzovi") }`. No `{{ define }}` / `{{ template }}` ceremony.
4. **IDE everything.** Go-to-definition, rename, find-references all work on components and their parameters. The templ VS Code extension does syntax highlighting + LSP for the `.templ` files themselves.
5. **One less thing at runtime.** No template parsing, no glob loading, no "did I forget to redeploy the templates" foot-gun. The binary contains the rendered structure.

### Trade-offs / new pain

- **A code generation step.** You must run `templ generate` after editing `.templ` files. Forgetting → stale Go code → confusing errors. Easy mitigation: `templ generate --watch` during development.
- **Two files per template.** `hello.templ` (source) and `hello_templ.go` (generated). The generated file *is* committed to git (so `go build` works on a fresh checkout without templ installed) but you should never edit it. Code review noise increases.
- **An installed tool.** `templ` CLI is required on every dev machine and in CI. Trivial via `go install`, but it's now part of your toolchain contract.
- **Slightly more verbose handler.** We lost Gin's `c.HTML(...)` one-liner. Could be hidden behind a helper or the `gin-templ` adapter — left explicit here to make the wiring visible.

### Commands cheat-sheet (additions)

```bash
go install github.com/a-h/templ/cmd/templ@latest   # one-time, installs the CLI
go get github.com/a-h/templ                         # add the runtime as a Go dep
templ generate                                      # regenerate *_templ.go from *.templ
templ generate --watch                              # rebuild on every .templ change (dev mode)
templ fmt views/                                    # format .templ files (like gofmt)
```

---

## Phase 3 — Add **Tailwind CSS** (via CDN)

**Diff from Phase 2:** the same `/hello` page, but now styled. We also gained a real **Layout** component that wraps page content — composition was hinted at in Phase 2 but happens for real here.

### Tech: Tailwind CSS

Tailwind is a **utility-first CSS framework**. Instead of writing semantic class names (`.card-header`, `.btn-primary`) and authoring CSS for them, you compose styles directly in your markup from a fixed vocabulary of low-level utilities:

- `p-8` = `padding: 2rem` (Tailwind's spacing scale is 4px per unit)
- `text-3xl font-bold text-slate-900` = font size, weight, and color
- `hover:bg-indigo-500` = pseudo-class variant — applies on hover
- `md:flex-row` = responsive variant — applies at the `md` breakpoint and up

There is no `.css` file to maintain. The utility classes *are* the styles.

**What's on the CDN.** Tailwind ships a "browser build" (`<script src="https://unpkg.com/@tailwindcss/browser@4"></script>`) that observes the DOM, scans for utility classes you actually used, and injects only the matching CSS at runtime. This is great for prototyping — zero build step — but pays a cost at every page load (the script itself is large and the work happens in the browser).

### Tech: templ children + Layout composition

Inside a templ component, the special expression `{ children... }` renders whatever was passed between the braces of the caller's `@Component { ... }` block. This is templ's equivalent of React's `props.children`. Calling pattern:

```templ
@Layout("Hello") {
    <main>...page content...</main>
}
```

The `<main>` block becomes the `children` of `Layout`. That's how a single `Layout` component can wrap many different pages — exactly the pain that stdlib `{{ define }} / {{ template }}` did poorly.

### What changed in the code

- **New file:** `views/layout.templ` — defines `Layout(title string)` with `<html>/<head>/<body>` shell, viewport meta, page title, and the Tailwind CDN script tag. The `<body>` contains `{ children... }`.
- **Refactored:** `views/hello.templ` — no longer owns the document shell. It now opens with `@Layout("Hello") { ... }` and emits only the page-specific markup. Tailwind utility classes added throughout: card layout, typography scale, an indigo CTA button.
- `main.go` is **unchanged** — the handler still calls `views.Hello(...).Render(...)`. The handler doesn't even know that Hello now delegates to Layout. That's the win of components.
- Regenerated: `views/hello_templ.go` and the new `views/layout_templ.go`.

### What I observed

- The rendered HTML now contains a `<script src="https://unpkg.com/@tailwindcss/browser@4"></script>` tag in `<head>` and many `class="..."` attributes. Server-side I can only inspect the markup with `curl`; the actual styling materializes in the browser when the script runs and injects CSS into the page.
- Reload time in the browser is noticeably slower than Phase 2 — the CDN script has to download, then scan the DOM, then write a `<style>` tag. For one tiny page it's fine; for a real app this is the strongest argument to move to a build pipeline in Phase 8.
- Tailwind v4 dropped the older `cdn.tailwindcss.com` Play CDN — the supported browser build is now `unpkg.com/@tailwindcss/browser@4`. Same idea, different URL.

### Value added

1. **No naming things.** I never invented a class like `.greeting-card`. Each style decision is local to the element. Editing the button color doesn't require finding the right CSS file.
2. **Consistent design tokens.** Spacing (`p-2 / p-4 / p-8`), font sizes (`text-sm / text-base / text-lg / text-xl ...`), colors (`slate-50 / slate-100 / ... / slate-900`) come from a fixed scale. You can't accidentally invent `padding: 13px` — and the result looks intentional.
3. **Pseudo-states and responsiveness are inline.** `hover:bg-indigo-500`, `md:flex-row`, `dark:bg-slate-900` live right next to the base styles instead of in a separate `:hover` block at the bottom of a stylesheet.
4. **Composes with templ perfectly.** A "styled button" is just a few utility classes inside a templ component — write it once, then `@PrimaryButton("Save")` everywhere. No design system framework needed.
5. **Layout composition is now real.** `Hello` doesn't repeat `<!DOCTYPE html>` and `<head>` ceremony. Adding a `Settings` page tomorrow means writing only the page body and wrapping it in `@Layout(...)`.

### Trade-offs / new pain

- **Crowded class lists.** A styled button can carry 8–12 utilities. It's verbose to read until your eye adapts. (Component extraction via templ blunts this — wrap the long class string inside a `PrimaryButton` component once.)
- **CDN cost.** The browser Tailwind script is large and runs on every page load. Acceptable for learning, **not** for production — Phase 8 (Vite) replaces it with a purged, minified, ~10KB stylesheet.
- **No `@apply`, no custom theme tokens via CDN.** The CDN build doesn't support a `tailwind.config.js` the way a local build does. Anything beyond default colors / breakpoints waits for Phase 8.
- **No editor sorting / linting yet.** Tools like `prettier-plugin-tailwindcss` and the Tailwind IntelliSense LSP exist but need configuration we haven't done. For now, class lists are sorted by hand (or not at all).

### Commands cheat-sheet (additions)

```bash
# Nothing new to install — Tailwind is loaded by the browser from the CDN.
# Just `templ generate` after editing .templ files, then `go run .`.
# Open http://localhost:8080/hello in a browser to actually see the styling.
```

---

## Phase 4 — Add **HTMX**

**Diff from Phase 3:** the "Greet me again" button is no longer dead. The page can now mutate parts of itself by asking the server for new HTML and swapping it in — without us writing a single line of JavaScript.

### Tech: HTMX

HTMX is a tiny (~14KB minified) JavaScript library that extends HTML with attributes for triggering AJAX requests and swapping the response into the DOM. The core idea: **the server returns HTML fragments, HTMX patches them into the page.**

The mental model is the opposite of a SPA:
- **SPA / React:** server returns JSON, client owns the DOM and re-renders from state.
- **HTMX:** server owns the rendering. Each interaction is a request that returns the new HTML for some region of the page. The DOM *is* the state.

The full HTMX vocabulary used in this phase:

| Attribute | Meaning |
|---|---|
| `hx-get="/url"` | On the natural trigger event (click for `<button>`, submit for `<form>`), issue a `GET`. |
| `hx-post="/url"` | Same, but `POST`. For `<form>`, HTMX serializes inputs as `application/x-www-form-urlencoded`. |
| `hx-target="#id"` | CSS selector for the element that should be updated with the response. Without it, the triggering element itself is the target. |
| `hx-swap="innerHTML"` | How to swap: replace inner HTML (default), `outerHTML`, `beforebegin`, `afterend`, `delete`, etc. |
| `hx-trigger="..."` | (Not used here.) Override the default trigger — e.g. `keyup changed delay:300ms` for live search. |

Because the response is HTML, **no JSON contract exists between client and server.** The server is free to restructure the response markup; as long as the HTML still looks right inside the target, nothing breaks.

### What changed in the code

- **`views/layout.templ`** — added `<script src="https://unpkg.com/htmx.org@2.0.4"></script>` after the Tailwind script. That single line activates HTMX on every page that uses the layout.
- **`views/greeting.templ`** *(new)* — a tiny component that renders only `<h1>hello { name }</h1>` and `<p>Rendered at { renderedAt }</p>`. No outer wrapper, no document shell. This is the swappable fragment.
- **`views/hello.templ`** — the page now contains:
  - A `<div id="greeting">` wrapping `@Greeting(name, renderedAt)` (the initial server-rendered greeting).
  - A button with `hx-get="/greeting" hx-target="#greeting" hx-swap="innerHTML"` — clicking it asks the server for a fresh greeting fragment and replaces what's inside `#greeting`.
  - A `<form hx-post="/greet" hx-target="#greeting" hx-swap="innerHTML">` with a text `<input name="name">` and a submit button. Submitting POSTs the form, gets back a greeting for whatever name was typed, swaps it in.
- **`main.go`** — two new handlers:
  - `GET /greeting` — renders only `views.Greeting("dzovi", now())`. No layout, no `<head>`, no `<body>`.
  - `POST /greet` — reads `c.PostForm("name")`, falls back to `"stranger"` on empty, renders the same `views.Greeting(name, now())` fragment.

The full-page `GET /hello` handler is unchanged.

### What I observed

- **The two endpoint shapes are visibly different.** `curl /hello` returns a full `<!doctype html>...</html>` document. `curl /greeting` returns *only* the two greeting tags — no head, no body, no doctype. This is exactly what HTMX wants: paste-ready HTML for the target slot.
- **No JavaScript was written.** All interactivity comes from declarative attributes on the HTML elements. The HTMX runtime reads them and does the fetch + swap.
- **The form input was magically serialized.** `name="alice"` in the input became `name=alice` in the POST body, picked up by `c.PostForm("name")`. HTMX uses the same form-encoding browsers do for plain `<form>` submission — no JSON, no fetch wrapper.
- **No client-side state.** Both handlers compute `time.Now()` server-side. The DOM is the only state — when HTMX swaps in a new `<p>Rendered at ...</p>`, that *is* the new "state."
- **History/URL didn't change** (and that's correct — these are partial updates, not navigations). HTMX *can* push history with `hx-push-url`, but for ephemeral fragment swaps you usually don't want it.

### Value added

1. **Interactivity without JavaScript.** The two new behaviors (refresh greeting, greet someone else) were added in pure HTML + Go. No `fetch()`, no event listeners, no state management, no React.
2. **The Go + templ + Gin combo finally shines.** Each interaction is just another handler that returns a templ component. Same patterns as full-page handlers; same type safety. There is no "API layer" separate from the "UI layer."
3. **Same component renders full page and fragment.** `views.Greeting(...)` is used both inside `@Layout` (initial page load) and standalone (HTMX response). Write it once, reuse in both places. This is the part that's impossible in a JSON-API + SPA split.
4. **Refactor-safe contracts.** If I rename a field in the Greeting component, both call sites (initial render and `/greeting` handler) break at compile time. No "did I forget to update the frontend after the API change" class of bug.
5. **Tiny payloads.** The fragment response is ~210 bytes. A JSON-then-rerender equivalent would ship JSON *and* still need client code to apply it.

### Trade-offs / new pain

- **Complex client-side state isn't HTMX's job.** Modal open/closed, dropdown expand, optimistic UI — these don't want to round-trip the server. That's why **Phase 5 (Alpine.js)** exists right next to HTMX.
- **Two response shapes per "page."** I now have to think about which handlers serve full pages and which serve fragments. A common pattern is a helper `RenderFull` vs `RenderPartial` or detecting the `HX-Request` header to switch. Worth introducing when the duplication appears.
- **Easy to bloat HTML.** Repeated big class strings appear in each fragment. The fix is component extraction (templ makes this cheap) — but the pull toward inline-everything is real.
- **No first-class type contract for form fields.** `c.PostForm("name")` is a string lookup by string key. Mistyping `"nme"` would silently return `""`. Gin has `c.ShouldBind(&struct{...})` for struct binding with validation — worth reaching for as forms grow.
- **HTMX needs the HTML to be the right shape.** If the server returns a fragment with the wrong outer structure, the swap produces broken markup. CSS that depends on parent-child structure can be surprised.

### Commands cheat-sheet (additions)

```bash
# Nothing new to install — HTMX is loaded by the browser from the CDN.
# Iteration loop is now:
#   1. Edit .templ files
#   2. templ generate
#   3. go run .
#   4. Browser interactions hit /greeting or /greet — no full page reloads
```

---

## Phase 5 — Add **Alpine.js**

**Diff from Phase 4:** behaviors that should never round-trip the server (dropdown open/closed, modal open/closed, live char count of an input) are now handled directly in the markup via Alpine. HTMX still owns server-driven swaps. The two coexist on the same page without stepping on each other.

### Tech: Alpine.js

Alpine is a tiny (~15KB minified) JavaScript framework that adds **reactivity to HTML through attributes**, the same shape as HTMX. Vue's mental model (templates with reactive expressions) shrunk down to a few `x-*` directives and dropped into existing HTML. No build step, no components — it's designed to be sprinkled in.

The full Alpine vocabulary used in this phase:

| Directive | Purpose |
|---|---|
| `x-data="{ open: false }"` | Declare a reactive scope. Everything inside this element sees `open`. |
| `x-show="open"` | Toggle the element's visibility (`display: none`) based on the expression. |
| `x-model="name"` | Two-way bind a form input to a reactive property. |
| `x-text="name.length"` | Set the element's text content from an expression. |
| `x-transition.opacity` | Fade in/out when `x-show` toggles. |
| `@click="..."` | Shorthand for `x-on:click`. Runs the expression on click. |
| `@click.outside="..."` | Fires when a click happens *outside* this element — perfect for closing dropdowns. |
| `@click.self="..."` | Fires only when the click target *is* this element (not a child) — perfect for "click backdrop to close modal." |
| `@keydown.escape.window="..."` | Listen for the Escape key on the window — Esc-to-close modal. |
| `:class="{ 'rotate-180': open }"` | Shorthand for `x-bind:class`. Conditionally apply classes from an expression. |
| `$dispatch('open-about')` | Magic helper that emits a custom DOM event. Lets components talk across the page without shared state. |
| `@open-about.window="..."` | Listen for that custom event at the window level. The pair makes a publish/subscribe across far-apart elements. |

The reactivity model: change a property inside `x-data`, every `x-show` / `x-text` / `:class` that reads it re-evaluates. That's it. No virtual DOM, no component instances.

### What changed in the code

- **`views/layout.templ`** — added `<script defer src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js"></script>` after the HTMX script. The `defer` matters: Alpine initialises after DOM parse. Also wrapped the body in a flex column with a new `<header>` containing the brand text and a **dropdown menu** built entirely with Alpine: a button toggles `open`, a `<div x-show="open" @click.outside="open = false">` holds the menu items, the chevron icon rotates via `:class="{ 'rotate-180': open }"`.
- **`views/hello.templ`** — three new pieces of Alpine usage on the page:
  1. **"What's this?" button.** `x-data` (empty scope just to enable Alpine on the element), `@click="$dispatch('open-about')"` — fires a custom DOM event that the modal listens for. The button doesn't know the modal exists.
  2. **Modal.** Rendered at the bottom of the page. `x-data="{ open: false }"`, `@open-about.window="open = true"` listens for the dispatched event, `@keydown.escape.window="open = false"` handles Esc, `@click.self="open = false"` closes when clicking the backdrop, `x-transition.opacity` fades it in.
  3. **Live char counter inside the HTMX form.** The form gained `x-data="{ name: '' }"`. The input is bound with `x-model="name"`. A `<p x-show="name.length > 0">` below the row shows `<span x-text="name.length"></span> char(s) typed`, with the trailing `s` toggled by another `x-show="name.length !== 1"` for proper pluralization. **Crucially**, the input still has `name="name"` (HTML attribute) — so when HTMX serializes and POSTs the form, the value goes along correctly. Alpine binds and observes, HTMX submits; both see the same `<input>` element.
- The two new elements that need to start hidden (`x-show="open"`) carry an inline `style="display: none"`. This is the standard trick to prevent a **flash of unstyled content** while Alpine is still loading from the CDN — Alpine will clear the style as soon as it evaluates `x-show`.
- **`main.go`** — unchanged. Alpine is entirely client-side; no new endpoints, no new handlers.

### What I observed

- All three Alpine features (dropdown, modal, char count) work **purely from `curl`-able markup**. The server response contains the `x-*` attributes verbatim; the browser does the rest. Same as HTMX, but for client-only state.
- Alpine and HTMX **don't fight over the form input**. HTMX serializes by reading the `name` attribute and `.value` of inputs at submit time. Alpine binds `.value` reactively. They read the same DOM property — no conflict.
- The `$dispatch` + `@open-about.window` pattern decouples the trigger from the modal. The button does not know about `open: false`; the modal does not know which button fired it. This is how Alpine scales without prop drilling. (It's also the answer to "how do I share state between two Alpine scopes" without a store.)
- The HTML now contains a lot of inline expressions (`x-data`, `@click`, `x-show`, `x-text`, `:class`). After a few minutes the eye adapts, but on first read it looks busy.

### Value added

1. **Zero round-trip for ephemeral state.** Dropdown open/closed, modal open/closed, char count — none of these hit the server. The user gets instant feedback; the network stays quiet.
2. **Composes cleanly with HTMX.** The form has *both* `x-data` (Alpine) and `hx-post` (HTMX). One sees keystrokes, the other sees submit. Each owns the layer it's best at.
3. **State stays local.** Each `x-data` is its own scope — the dropdown's `open` and the modal's `open` are independent variables that happen to share a name. No global store needed.
4. **No build step.** Same as HTMX and the Tailwind CDN — script tag, done. This won't survive Phase 8 (Vite) but for learning the absence of tooling is a feature.
5. **Custom events are the escape hatch.** When two distant Alpine scopes need to talk (button → modal), `$dispatch` + `@event.window` handles it without elevating state into a parent component. The DOM itself is the message bus.

### Trade-offs / new pain

- **Two layers of interactivity now.** Every new behavior needs a decision: HTMX (server owns it) or Alpine (client owns it). Some good rules:
  - State that other users would care about → server → **HTMX**.
  - State that disappears on reload and only matters to this user, this session → client → **Alpine**.
  - State that's both (e.g. a modal whose *contents* come from the server) → Alpine wraps an HTMX-loaded fragment.
- **No types, no compile-time checks for Alpine.** `x-text="name.lenght"` is a string in an HTML attribute — typo = silent failure (the text is just empty). Phase 1's pain in a new outfit. Mitigations exist (Alpine LSP, linters) but we haven't set them up.
- **Inline expressions are HTML-noisy.** Long modals end up with `x-data`, `x-show`, `@click.self`, `@keydown.escape.window` all on the same element. The fix is the same as Tailwind's: extract templ components like `@Modal("open-about") { ... children ... }` that hide the Alpine wiring once and reuse it.
- **FOUC trap.** Anything `x-show="false initially"` will flash visible for a tick on first page load unless you add `style="display: none"` (manually, as I did) or use the `[x-cloak] { display: none !important; }` trick. Easy to forget.
- **No SSR for Alpine state.** Whatever's inside `x-data` is initialised in the browser. If a user disables JS, the dropdown menu is permanently closed. Acceptable for these enhancements, *not* acceptable for primary navigation.

### Mental model summary so far

| Layer | Owner | Reaches for |
|---|---|---|
| Document structure & data | Server (Go + templ) | `views.Hello(...)` |
| Style | Tailwind (CDN for now) | Utility classes |
| Server-driven interactions | HTMX | `hx-get`, `hx-post`, `hx-target` |
| Client-only UI state | Alpine | `x-data`, `x-show`, `@click`, `$dispatch` |

This is the full "HTML over the wire" stack. Phases 6–8 strengthen the backend (real persistence, type-safe queries, asset pipeline); Phase 9 builds the actual leave-management features on top.

### Commands cheat-sheet (additions)

```bash
# Nothing new to install — Alpine loads from the CDN.
# Iteration loop unchanged:
#   1. Edit .templ files
#   2. templ generate
#   3. go run .
#   4. Click around in the browser to exercise Alpine's reactivity
```

---

## Phase 6 — Add **Postgres** (raw SQL with pgx)

**Diff from Phase 5:** the app finally has **state that survives a restart**. Everything so far was computed per-request (`now()`) or lived only in the browser (Alpine). Now there's a `users` table in Postgres, a new `/users` page that lists rows and a form that inserts them. The DB code is written **by hand** — we build the SQL string, run it, and map columns onto struct fields with explicit `rows.Scan(...)`. That manual mapping is the deliberate pain of this phase; Phase 7 (sqlc) deletes it.

### Tech: Postgres + pgx (+ golang-migrate for schema)

- **Postgres** — the relational database. We connect to an already-running local instance (Docker container `postgres-mafwr`, `postgis/postgis:16`) and gave this project its **own database** `leave_management` so it never collides with the other DBs on that server.
- **pgx v5** (`github.com/jackc/pgx/v5`) — the modern, Postgres-native Go driver. We use its **connection pool**, `pgxpool.Pool`, not the generic `database/sql`. pgx speaks the Postgres wire protocol directly, so it's faster and supports Postgres-specific types better.
- **golang-migrate** — versioned schema migrations. Each change is a numbered pair of files: `NNNNNN_title.up.sql` (apply) and `.down.sql` (roll back). A `schema_migrations` table in the DB tracks which version is applied, so `migrate up` is idempotent and `migrate down` reverses the last step. This is the first phase where the schema is a **tracked artifact**, not something typed once into psql.

| pgx piece | Purpose |
|---|---|
| `pgxpool.New(ctx, url)` | Open a pool of connections. Safe for concurrent handlers. |
| `pool.Ping(ctx)` | Verify connectivity at startup — fail fast if the DB is down. |
| `pool.QueryRow(ctx, sql, args...).Scan(...)` | One-row queries (our `INSERT ... RETURNING`). |
| `pool.Query(ctx, sql, args...)` then `rows.Next()` / `rows.Scan(...)` / `rows.Err()` | Multi-row queries (our `SELECT`). |
| `$1`, `$2` placeholders | Postgres parameterized queries — the driver sends values separately, so **no SQL injection** even with raw string queries. |

### What changed in the code

- **`migrations/000001_create_users.up.sql` / `.down.sql`** (new) — create/drop the `users` table (`id BIGINT GENERATED ALWAYS AS IDENTITY`, `name TEXT NOT NULL`, `created_at TIMESTAMPTZ DEFAULT now()`).
- **`store/store.go`** (new package) — the hand-written data layer:
  - `Store` wraps a `*pgxpool.Pool`. `New(ctx, url)` opens + pings; `Close()` releases it.
  - `User` struct mirrors the table **by hand** — nothing enforces that Go and SQL agree.
  - `CreateUser` runs `INSERT ... RETURNING id, created_at` and scans two columns back.
  - `ListUsers` runs a `SELECT` and loops `rows.Next()` / `rows.Scan(&u.ID, &u.Name, &u.CreatedAt)` / `rows.Err()`.
- **`views/users.templ`** (new) — `UsersPage([]store.User)` is the full page (Tailwind card + an HTMX form + the list); `UserList([]store.User)` is the fragment. Same "one component, two entry points" trick from Phase 4: the page renders the list once, and after a POST the server returns *just* `UserList` for HTMX to swap into `#user-list`. `hx-on::after-request="this.reset()"` clears the input after a successful add.
- **`main.go`** — now connects to Postgres at startup (`store.New`, `log.Fatalf` if it fails), and added `GET /users` (render page) + `POST /users` (insert, then return the refreshed list fragment). Also factored the repeated status/header/render dance into a small `html(c, render)` helper and added `dbURL()` which reads `DATABASE_URL` or falls back to the local default.

### What I observed

- **State persists.** Added "Dzovi" and "Mary" via `curl -X POST`, restarted nothing, and `SELECT * FROM users` in psql showed both rows with real `id`s (1, 2) and server-side `created_at`. This is the first thing in the whole project that outlives the process.
- **`INSERT ... RETURNING` is one round trip.** The DB generates `id` and `created_at`; `RETURNING` hands them straight back, so `CreateUser` returns a fully-populated `User` without a second `SELECT`.
- **Ordering works as written.** `ORDER BY created_at DESC` — the POST-Mary response listed Mary above Dzovi. Nothing sorts client-side.
- **The fragile part is exactly where expected.** In `ListUsers`, the three `&u.ID, &u.Name, &u.CreatedAt` arguments must line up positionally with the three `SELECT` columns. The compiler has no idea these two lists are related. Reorder the SELECT and it still compiles — it just breaks (or silently misassigns) at runtime.

### Value added

1. **Real persistence.** The app has a memory now. This is the precondition for everything in Phase 9.
2. **Connection pooling for free.** `pgxpool` hands each concurrent request a connection and recycles it. No per-request connect cost, no manual pool management.
3. **Safe parameterization by default.** `$1` placeholders mean user input never gets string-concatenated into SQL. Injection is structurally prevented even though we're writing raw SQL.
4. **Migrations make the schema reproducible.** `migrate up` on a fresh DB rebuilds the exact structure; `migrate down` reverses it. The schema is now versioned alongside the code instead of living in someone's terminal history.

### Trade-offs / new pain (this is the point — feel this before Phase 7)

- **Manual `Scan` is fragile and positional.** Column order, argument order, and types must all agree, and *nothing checks that at compile time*. This is Phase 1's stringly-typed pain wearing a database costume.
- **Struct ↔ schema drift is unguarded.** Add a `email` column to the table and the Go `User` struct won't know. Add a field to the struct and the SQL won't populate it. You keep them in sync by discipline alone.
- **Boilerplate scales linearly.** Every new query = build string + `Query`/`QueryRow` + a `Scan` loop + `rows.Err()`. Ten queries = ten near-identical hand-written blocks, each an opportunity to misalign a column.
- **Errors are runtime-only.** A typo in a column name, a wrong type in a `Scan` target — all discovered when the request runs, not when the code builds. **Every one of these is what sqlc (Phase 7) fixes** by generating the structs and `Scan` calls from the SQL itself.
- **A live DB is now a dependency.** The server `log.Fatalf`s on startup if Postgres is unreachable. The "just `go run .`" simplicity of Phases 0–5 is gone — you need the container up and the migration applied first.

### Mental model summary so far

| Layer | Owner | Reaches for |
|---|---|---|
| Document structure & data | Server (Go + templ) | `views.UsersPage(...)` |
| Style | Tailwind (CDN for now) | Utility classes |
| Server-driven interactions | HTMX | `hx-get`, `hx-post`, `hx-target` |
| Client-only UI state | Alpine | `x-data`, `x-show`, `@click` |
| **Persistence** | **Postgres via pgx (raw SQL)** | **`pool.Query` + manual `Scan`** |

### Commands cheat-sheet (additions)

```bash
# One-time install of the migrate CLI (with the postgres driver):
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Connection string (or export DATABASE_URL to override the default in main.go):
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable"

# Apply / roll back / inspect migrations:
migrate -path migrations -database "$DATABASE_URL" up
migrate -path migrations -database "$DATABASE_URL" down 1
migrate -path migrations -database "$DATABASE_URL" version

# Iteration loop now:
#   1. (once) container up + migrate up
#   2. Edit .templ / .go files
#   3. templ generate
#   4. go run .
#   5. POST/GET /users — data survives restarts
```

---

## Phase 7 — Replace raw SQL with **sqlc**

**Diff from Phase 6:** the app behaves **identically** — same `/users` page, same insert-and-swap, same rows in the same database. What changed is entirely internal: the hand-written `User` struct and every `rows.Scan(...)` from Phase 6 are gone, replaced by Go code that sqlc **generates** from the schema and a `query.sql` file. `store/store.go` went from ~90 lines of manual SQL to ~30 lines that just delegate. This is the payoff for writing the fragile boilerplate in Phase 6.

### Tech: sqlc

sqlc is a **compiler, not an ORM**. You give it two things — your schema and your SQL queries — and it generates fully typed Go functions. It does this by actually *parsing* the SQL against the schema: it knows `users.id` is `bigint` and `name` is `text`, so it knows `ListUsers` returns `[]User` with an `int64` and a `string`. There's no query DSL, no struct tags describing tables, no reflection at runtime. The generated code is plain Go you can read.

The flow:

```
migrations/*.up.sql   ─┐
                       ├─→  sqlc generate  ─→  db/{db.go, models.go, query.sql.go}
query.sql (annotated) ─┘
```

Query annotations tell sqlc what shape to return:

| Annotation | Meaning | Generated signature |
|---|---|---|
| `-- name: CreateUser :one` | returns exactly one row | `CreateUser(ctx, name) (User, error)` |
| `-- name: ListUsers :many` | returns a slice | `ListUsers(ctx) ([]User, error)` |
| `-- name: DeleteUser :exec` | returns no rows | `DeleteUser(ctx, id) error` |

### What changed in the code

- **`sqlc.yaml`** (new) — the config. `engine: postgresql`, `schema: "migrations"` (sqlc reads the golang-migrate `*.up.sql` files as the schema and **ignores** the `*.down.sql` ones — same source of truth as the live DB), `queries: "query.sql"`, output package `db`. Two important knobs:
  - `sql_package: "pgx/v5"` — generate against the pgx pool we already use, not `database/sql`. The generated `DBTX` interface is satisfied by `*pgxpool.Pool`.
  - an `overrides` entry mapping `timestamptz → time.Time` (the column is `NOT NULL`, so no need for `pgtype.Timestamptz`). This keeps the templ `CreatedAt.Format(...)` call working unchanged.
- **`query.sql`** (new) — the two annotated queries (`CreateUser :one`, `ListUsers :many`). This file plus the schema is the *entire* human input; everything else is generated.
- **`db/`** (generated, do not edit) — three files:
  - `models.go` — the `User` struct (`ID int64`, `Name string`, `CreatedAt time.Time`), derived from the table.
  - `db.go` — the `DBTX` interface, `Queries` struct, `New(DBTX)`, `WithTx(pgx.Tx)`.
  - `query.sql.go` — `CreateUser` and `ListUsers`, each with the exact `Scan` we wrote by hand in Phase 6 — but now generated *from* the SQL, so they can never drift from it.
- **`store/store.go`** — gutted. Deleted the hand-written `User` struct and both raw-SQL methods. Now `Store` holds the pool **and** a `*db.Queries` (`db.New(pool)`), and `CreateUser` / `ListUsers` are one-line delegations returning `db.User`. Public API is unchanged, so handlers didn't move.
- **`views/users.templ`** — the only signature change rippling outward: `UsersPage` / `UserList` now take `[]db.User` instead of `[]store.User`. The markup is untouched — `u.Name` and `u.CreatedAt.Format(...)` still work because the override kept `CreatedAt` a `time.Time`.
- **`main.go`** — completely unchanged. It still calls `db.ListUsers` / `db.CreateUser` through the `store` facade with the same signatures.

### What I observed

- **Regenerating is the whole workflow.** Edit `query.sql`, run `sqlc generate`, and the Go changes appear. Add a column to a migration, regenerate, and the `User` struct updates itself. The schema is the single source of truth now.
- **The generated `Scan` is byte-for-byte what I wrote by hand** in Phase 6 (`row.Scan(&i.ID, &i.Name, &i.CreatedAt)`) — sqlc just guarantees it always matches the `SELECT`, because it reads the same SQL to produce both.
- **Errors moved from runtime to `sqlc generate` time.** I tested a typo'd column name in `query.sql`; `sqlc generate` refused with a clear parse error *before* any Go compile — earlier than even the compiler would catch it. That's the Phase 6 "runtime-only" pain fully eliminated.
- **App behavior is identical.** Added "Sqlc-Sam" via `curl`; it appeared newest-first above Mary and Dzovi, and Gin logged a 200 in <1ms for the follow-up GET. From the outside, nothing about Phase 7 is visible — which is exactly the point.
- **`emit_empty_slices: true`** makes `ListUsers` return `[]User{}` instead of `nil` on an empty table, so the templ `len(users) == 0` branch and JSON callers behave predictably.

### Value added

1. **No more manual `Scan`.** The single most error-prone part of Phase 6 — lining up `&field` pointers with `SELECT` columns — is generated and therefore always correct.
2. **Schema/struct drift is impossible.** The `User` struct is *derived from* the schema. Change the table, regenerate, and the struct follows. You can't forget to update it.
3. **Errors caught at generate time.** Typo a column, use a wrong type, reference a missing table — `sqlc generate` fails immediately with a SQL-aware message. Phase 6's "compiles fine, breaks at runtime" class of bug is gone.
4. **You still write plain SQL.** Unlike an ORM, there's no query builder to learn and no leaky abstraction. `LEFT JOIN`, window functions, CTEs — anything Postgres does, you write directly and sqlc types the result.
5. **The store shrank ~3×.** Real, deletable boilerplate disappeared. Adding the next query is: write annotated SQL, regenerate, call the new method.

### Trade-offs / new pain

- **A build step now stands between SQL and Go.** Forget to run `sqlc generate` after editing `query.sql` and you're calling stale functions. (Same discipline as `templ generate` — and `sqlc generate --watch` exists.)
- **Toolchain surface grew.** sqlc is another CLI to install and pin (it pulled a newer Go toolchain to build). Generated `db/` code is committed and must be regenerated on schema changes — reviewers see generated diffs.
- **It's SQL-first, not Go-first.** Dynamic queries (optional filters, variable `IN` lists) are awkward — sqlc wants each query fully known at generate time. Those cases fall back to hand-written pgx or `sqlc.narg`/`sqlc.slice` helpers.
- **Generated types can leak into the app.** `db.User` now appears in the view signatures. For a bigger app you might map generated rows onto your own domain structs at the store boundary — but for this project, using `db.User` directly keeps things honest and simple.

### Mental model summary so far

| Layer | Owner | Reaches for |
|---|---|---|
| Document structure & data | Server (Go + templ) | `views.UsersPage(...)` |
| Style | Tailwind (CDN for now) | Utility classes |
| Server-driven interactions | HTMX | `hx-get`, `hx-post`, `hx-target` |
| Client-only UI state | Alpine | `x-data`, `x-show`, `@click` |
| Persistence | Postgres | the running database |
| **DB access code** | **sqlc (generated from SQL)** | **`query.sql` + `sqlc generate` → `db.Queries`** |

### Commands cheat-sheet (additions)

```bash
# One-time install of the sqlc CLI:
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Regenerate the db/ package after editing query.sql or a migration:
sqlc generate

# Sanity-check queries against the schema without generating:
sqlc vet     # (optional) lint; sqlc compile also parses without writing files

# Iteration loop now:
#   1. (once) container up + migrate up
#   2. Edit query.sql / migrations / .templ files
#   3. sqlc generate   (if SQL changed)   +   templ generate   (if .templ changed)
#   4. go run .
#   5. POST/GET /users — same behavior, now fully typed & generated
```

---

## Phase 8 — Add **Vite** for the asset pipeline

**Diff from Phase 7:** the three CDN `<script>` tags in `layout.templ` (Tailwind browser build, HTMX, Alpine) are gone. Those libraries are now npm dependencies **compiled by Vite** into a single hashed, self-hosted JS bundle + a purged CSS file. Go stops trusting a third-party CDN at runtime and instead serves its own `public/build/` folder, reading a `manifest.json` to know which hashed filenames to emit. There are now two modes — a Vite **dev server with HMR** for development, and a **built bundle** for production — and the templ layout branches between them.

### The problem Vite solves (why we waited until now)

Since Phase 3 everything loaded from `unpkg.com` at runtime. That was deliberately simple, but it carried real costs the CDN can't fix:

- **The Tailwind browser build is huge and runs in the browser** — it shipped the whole engine to every visitor and *generated CSS on every page load* by scanning the DOM.
- **No purging** — no way to strip unused utilities.
- **A hard third-party dependency** — every load hit unpkg; if it's slow/down/blocked, the app breaks. No offline. Versions pinned as strings in URLs, no lockfile.
- **Many unminified requests**, and a caching-vs-freshness bind (cache hard → stale after deploy; don't cache → re-download every load).

Vite fixes all of these at once by moving the work from *the browser at runtime* to *a build step at deploy time*.

### Tech: Vite (+ @tailwindcss/vite, npm)

Vite is a frontend build tool with two jobs:
1. **Dev server** (`vite`) — serves source modules directly with **HMR** (edit `app.css` → browser updates instantly, no full reload).
2. **Bundler** (`vite build`) — compiles, bundles, minifies, and **content-hashes** assets into `public/build/`, plus a `manifest.json` mapping each entry to its hashed output.

It does **not** touch the HTML. Division of labor: **Go/templ serves HTML, Vite serves static assets.**

| Piece | Role |
|---|---|
| `web/package.json` | npm project; `dev`/`build` scripts; deps pinned in `package-lock.json` |
| `@tailwindcss/vite` | Tailwind v4's first-party Vite plugin — compiles `@import "tailwindcss"` ahead of time |
| `@source "../../views"` (in app.css) | tells Tailwind which files to scan for class names → generate-and-purge |
| content-hashed filenames (`app-CAmv5NGh.js`) | cache forever; a new build = new hash = automatic cache-bust |
| `manifest.json` | the contract between Vite's output and Go: entry → `{ file, css }` |

### What changed in the code

- **`web/`** (new frontend project):
  - `package.json` — `"type": "module"`, `dev`/`build` scripts. Deps: `vite`, `@tailwindcss/vite`, `tailwindcss` (dev) + `alpinejs`, `htmx.org` (runtime).
  - `vite.config.js` — the Tailwind plugin, `build.manifest: true`, `outDir: '../public/build'`, single entry `src/app.js`, and `server.cors: true` so the Go app on `:8080` can load ES modules from the dev server on `:5173`.
  - `src/app.css` — `@import "tailwindcss";` plus `@source "../../views";` so Tailwind scans our `.templ` / `_templ.go` files.
  - `src/app.js` — the single entry: `import './app.css'`, `import htmx from 'htmx.org'`, `import Alpine from 'alpinejs'`, set them on `window`, `Alpine.start()`. These three imports replace the three old CDN tags.
- **`assets/assets.go`** (new Go package) — the bridge. `Init(dev bool)`: in prod it reads `public/build/.vite/manifest.json` once at startup; in dev it does nothing. Then `Client()` (the HMR client URL), `Entry("src/app.js")` (dev server URL *or* `/build/<hashed>.js`), and `Styles("src/app.js")` (nil in dev — Vite injects CSS via JS; the hashed `/build/<hashed>.css` in prod).
- **`views/layout.templ`** — the three `unpkg.com` scripts replaced by: an `if assets.Dev` block emitting `@vite/client`, a `for` over `assets.Styles(...)` emitting `<link rel="stylesheet">`, and a single `<script type="module">` from `assets.Entry(...)`. The dev/prod split is visible right there in the template.
- **`main.go`** — `assets.Init(os.Getenv("VITE_DEV") == "true")` at startup (fatal if the manifest is missing in prod), and `r.Static("/build", "./public/build")` to serve the built assets.

### What I observed

- **CSS went from multi-MB (CDN) to 13KB** (`app-DxaTOiea.css`, 3.4KB gzip) — that's the whole Tailwind engine + runtime DOM scan replaced by a static, purged stylesheet. The JS bundle (htmx + Alpine) is 117KB / 35KB gzip, minified into one request.
- **Prod HTML is fully self-hosted.** The served page references `/build/assets/app-<hash>.{css,js}` and contains **zero** `unpkg.com` references; both hashed files return 200 with correct content types from Gin's static handler.
- **Dev mode emits different tags automatically.** With `VITE_DEV=true` the same page instead emits `http://localhost:5173/@vite/client` + `http://localhost:5173/src/app.js` and **no** stylesheet `<link>` (Vite injects CSS through JS so it can hot-swap it). The dev module is reachable cross-origin from `:8080` (CORS 200) — HMR works.
- **The manifest is the contract.** `manifest.json` mapped `src/app.js → { file: "assets/app-CAmv5NGh.js", css: ["assets/app-DxaTOiea.css"] }`; `assets.go` just prefixes `/build/` and emits the tags. Rebuild → new hashes in the manifest → Go emits the new URLs with no code change.

### Value added

1. **Purged, minified, self-hosted assets.** ~13KB CSS instead of a multi-MB CDN doing runtime work. No third-party dependency, works offline, no privacy leak to unpkg.
2. **Cache-busting for free.** Content hashes let the browser cache assets forever; a deploy changes the hash, so users never get stale code — and never needlessly re-download unchanged code.
3. **Real dependency management.** npm + `package-lock.json` pin exact versions reproducibly, replacing version strings hand-typed into URLs.
4. **HMR in dev.** Instant feedback while editing styles/scripts, without losing page state.
5. **Clean separation, one seam.** Go owns HTML, Vite owns assets, and the `manifest.json` is the single, well-defined contract between them. The dev/prod difference lives in one `if assets.Dev` block.

### Trade-offs / new pain

- **A whole Node toolchain now.** `node_modules`, a `package.json`, a build step, and a second dev process (`vite` alongside `go run .`). The "just `go run .`" era is fully over.
- **Two things can be out of sync.** Forget `npm run build` after changing assets and prod serves stale/missing files (the app `log.Fatal`s if the manifest is absent — a deliberate loud failure). Forget to run the dev server in dev mode and the page loads nothing.
- **Dev/prod divergence is a new class of bug.** Something can work with HMR but break in the built bundle (or vice-versa). Both paths must be tested — which is why we verified each here.
- **htmx uses `eval` internally** (for `hx-on`/expression features), which triggers a build-time warning from the bundler. Benign for our usage, but worth knowing if a strict CSP later forbids `eval`.
- **More moving parts to deploy.** Shipping now means "build Go binary **and** run `vite build` and ship `public/build/`." Still simple (a binary + a static folder), but no longer a single artifact.

### Mental model summary so far

| Layer | Owner | Reaches for |
|---|---|---|
| Document structure & data | Server (Go + templ) | `views.UsersPage(...)` |
| **Styling & client JS build** | **Vite (Tailwind + Alpine + HTMX bundled)** | **`npm run build` → `public/build/` + manifest** |
| Server-driven interactions | HTMX | `hx-get`, `hx-post`, `hx-target` |
| Client-only UI state | Alpine | `x-data`, `x-show`, `@click` |
| Persistence | Postgres | the running database |
| DB access code | sqlc (generated from SQL) | `query.sql` + `sqlc generate` → `db.Queries` |

Every tool in the stack is now in place. Phase 9 stops saying "hello dzovi" and assembles all of it into the real leave-management app.

### Commands cheat-sheet (additions)

```bash
# One-time: install the frontend toolchain
cd web && npm install

# --- Production ---
cd web && npm run build          # writes ../public/build/ + .vite/manifest.json
go run .                         # reads the manifest, serves /build (self-hosted assets)

# --- Development (HMR) ---
# Terminal 1: Vite dev server on :5173
cd web && npm run dev
# Terminal 2: Go in dev mode so the layout points at the dev server
VITE_DEV=true go run .
# Edit web/src/app.css or a .templ class -> browser updates without full reload

# Remember: after editing .templ you still run `templ generate`;
# after editing query.sql you still run `sqlc generate`.
```

---

## Phase 9 — Build the real Leave Management app (assemble the stack)

**Diff from Phase 8:** *no new tool.* Everything before this proved one piece of tech in isolation on "hello dzovi" content. Phase 9 deletes the demo (`/hello`, `/greeting`, `/users`, and the `users` table) and uses the whole stack — Go/Gin/templ/Tailwind/HTMX/Alpine/Postgres/sqlc/Vite — on the real domain: employees, roles, leave requests, approvals, balances, a calendar, and admin. Two structural changes come with it: the flat `main.go` + `store` + `db` layout becomes `internal/` packages, and one genuinely new *concept* (not tool) lands — **cookie-session authentication**.

### What "assembly" actually demanded

A single feature in isolation needs almost no scaffolding. A real app needed all of this at once, and that's the lesson of the phase:

- **Identity & access** — who are you (login), how do we remember it (sessions), what may you do (roles: employee / manager / admin).
- **Authorization at every mutation** — not just "are you a manager" (middleware) but "is this *your* report" (per-request check in the handler).
- **Structure** — one `main.go` with inline closures doesn't scale to ~15 routes; packages by concern do.
- **Validation & error UX** — forms can be wrong; the server has to say so without losing the user's input.
- **Seed & tests** — a real app is unusable empty and unmaintainable untested.

### New concept: session auth (bcrypt + Postgres sessions)

No new dependency beyond `golang.org/x/crypto/bcrypt`. The flow:

1. **Login** (`POST /login`) — look up the employee by email, `bcrypt.CompareHashAndPassword` the submitted password. Same error message for "no such email" and "wrong password" so we don't leak which accounts exist.
2. **Session** — mint a 256-bit `crypto/rand` token, insert a `sessions` row (`token`, `employee_id`, `expires_at`), and set it as an **HttpOnly, SameSite=Lax** cookie. State lives server-side; the cookie is just the key.
3. **Middleware** (`RequireAuth`) — read the cookie, resolve token → employee in one query, stash the employee on the Gin context. No cookie → redirect before touching the DB (which is why the router test needs no database). `RequireRole(...)` composes on top for manager/admin routes.
4. **Logout** — delete the session row and expire the cookie.

The server-side session (vs a signed stateless cookie) means logout genuinely invalidates — verified: after `POST /logout`, reusing the old cookie 302s to `/login`.

### Project structure (the restructure)

```
main.go                     thin bootstrap (config → store → assets → router → run)
cmd/seed/main.go            re-runnable data seeder
internal/
  config/    env → typed Config
  db/        sqlc-generated (moved here from ./db)
  store/     pgx pool + domain methods (moved from ./store, expanded)
  leave/     pure WorkingDays() calc  (+ unit test)
  auth/      bcrypt, session token/cookie, RequireAuth / RequireRole
  handlers/  one file per feature (auth, dashboard, requests, approvals, employees, calendar, admin)
  server/    the routing table
views/       templ components (stayed at root so Tailwind's @source keeps scanning)
assets/      Vite bridge (unchanged from Phase 8)
```

The `store` stayed thin (delegations), but it now earns its keep: it's the one place that translates between plain Go types the handlers like (`int64`, `time.Time`) and the `pgtype` wrappers sqlc emits for NULL-able columns.

### Domain model

Six tables (`employees`, `sessions`, `leave_types`, `leave_allocations`, `leave_requests`, `public_holidays`) in one migration that also `DROP`s the demo `users` table. Choices worth recording:

- **TEXT + CHECK for enums** (`role`, `status`), not native Postgres `ENUM`. sqlc maps both to plain Go `string`, but a CHECK is a one-line migration to change; a native enum needs `ALTER TYPE` (and historically couldn't run in a transaction). Cheaper to evolve.
- **Working days snapshotted on the request.** Weekdays-minus-holidays is computed in Go at submit time and stored in `leave_requests.working_days`, so a later change to the holiday calendar can't retroactively resize an approved request.
- **Balances are computed, not stored.** `ListBalances` LEFT JOINs allocation days against `SUM(working_days)` of approved requests for the year — one SQL query, no denormalized counter to keep in sync.

### What changed in the code (diff)

- **Deleted:** `views/{hello,greeting,users}.templ`, the `CreateUser`/`ListUsers` queries, the flat `db/` and `store/` dirs, and the inline demo handlers in `main.go`.
- **`query.sql`** grew from 2 queries to ~25, grouped by area. New sqlc features leaned on: `sqlc.embed(e)` (session lookup returns a nested `Employee`), named params with casts (`@decided_by::bigint`, `@manager_id::bigint = 0 OR ...`) to control inferred Go types, and aggregate/`COALESCE` projections for balances.
- **`views/`** went from 3 files to ~9, still a single flat `views` package. New: role-aware `Layout` nav with a pending-approval badge, a standalone `LoginPage`, an Alpine toast host, an Alpine modal + HTMX form for requests, a server-built calendar grid, and small shared components (`balanceCards`, `requestRows`, `statusBadge`, `colorDot`).
- **`server/router.go`** — middleware *groups*: a `RequireAuth` group, a `RequireRole(manager, admin)` subgroup for approvals/team, and a `RequireRole(admin)` subgroup for `/admin`.
- **`cmd/seed`** and three tests (pure `WorkingDays`, no-DB router redirect, DB-gated `ListBalances` round-trip).

### What I observed

- **sqlc respects nullability under a global override — the worry was unfounded.** The config maps `timestamptz`/`date` → `time.Time`, but sqlc still emitted `pgtype.Timestamptz` for the *nullable* `decided_at` and `pgtype.Int8` for `manager_id`/`decided_by`, while NOT NULL dates became clean `time.Time`. So the override is "for the non-null ones," exactly what you want, with no runtime NULL-scan panic.
- **HTMX processes `HX-Trigger` even on a 4xx.** The submit-request handler returns `400` + an error-toast header on validation failure (end-before-start). Verified: the toast fires *and* HTMX doesn't swap the list — so the modal stays open with the user's input. That's the whole error-UX strategy in one response.
- **The Alpine⇄HTMX seam is clean in both directions.** Toasts: the server sets `HX-Trigger: {"toast":{...}}`, HTMX dispatches a `toast` event, an Alpine component on the layout catches it on `window` and renders the stack. Calendar: Alpine owns the displayed month (client state, instant prev/next label) and calls `window.htmx.ajax()` to fetch the new grid — Alpine for ephemeral UI, HTMX for server data, the exact division the earlier phases set up.
- **Roles + ownership both enforced.** Employee `sam` gets `403` on `/approvals` and `/admin` (middleware); a manager can only approve their own reports (per-request `canDecide` check), not by guessing an id.
- **Balances math is right end-to-end.** Seeded a Mon–Wed request, approved it as the manager, and Sam's Annual balance showed **3 used / 25** — weekends correctly excluded, and untouched until *approval* (pending doesn't count).
- **Tailwind purge still works after a big view expansion.** Nine templ files of utilities compiled to an **18.5KB** stylesheet (4.4KB gzip). Dynamic colors (leave-type hex) had to be inline `style=` since JIT can't see runtime values.

### Value added

1. **A real app, not a demo.** Every tool now pulls weight simultaneously: templ renders role-aware pages, HTMX swaps request rows and approval cards, Alpine runs the modal/toasts/calendar-nav, sqlc types every query, Postgres holds the truth, Vite ships the assets.
2. **Security posture.** Hashed passwords, server-side sessions, HttpOnly/SameSite cookies, auth middleware, and defense-in-depth authorization (role *and* ownership).
3. **Maintainable shape.** `internal/` packages by concern; adding a feature is "a query + a store method + a handler + a templ," each in an obvious place.
4. **Confidence.** Business logic (`WorkingDays`) is a pure, tested function; the auth gate has a no-DB test; the query layer has an integration test.

### Trade-offs / new pain

- **The generation dance is now three-wide.** Change a query → `sqlc generate`; change a view → `templ generate`; change assets → `npm run build`. Miss one and you get a stale-code bug that compiles.
- **Nullable columns are a papercut.** The `pgtype.Int8`/`.Valid` wrappers are correct but noisy; keeping them contained to the store layer took deliberate query design (`LEFT JOIN ... COALESCE`, SQL-side filtering) so handlers never see them.
- **Auth I deliberately kept simple.** `Secure=false` on the cookie (fine for local http, must flip behind TLS), no CSRF token (SameSite=Lax is the only mitigation), no password reset / rate limiting / self-registration. Real, but not production-hardened.
- **HTMX partial vs full-page is a constant small decision.** Every mutation is "return a fragment (and which target?) or redirect?" — approvals swap a card out, requests swap a tbody, allocations swap nothing (toast only). Powerful, but you decide the swap contract per endpoint.

### Mental model — the finished stack

| Layer | Owner | Reaches for |
|---|---|---|
| Identity / access | `internal/auth` | bcrypt, session cookie, `RequireAuth` / `RequireRole` |
| Routing & HTTP | Gin + `internal/server` | middleware groups, one handler per route |
| Document structure & data | templ | `views.DashboardPage(...)`, shared components |
| Server-driven interactions | HTMX | `hx-post`, `hx-target`, `HX-Trigger`, `closest [data-request]` |
| Client-only UI state | Alpine | modal `open`, toast stack, calendar month nav |
| Business rules | plain Go (`internal/leave`) | `WorkingDays()` — pure, unit-tested |
| DB access code | sqlc | `query.sql` → typed `db.Queries` (`sqlc.embed`, named casts) |
| Persistence | Postgres | 6 tables, TEXT+CHECK enums, computed balances |
| Assets | Vite | `npm run build` → hashed `public/build/` + manifest |

**The stack is complete.** The checklist became a way of working: a new feature is a predictable walk down that table.

### Commands cheat-sheet (additions)

```bash
# One-time / on schema change: apply migrations, then seed
migrate -path migrations -database "$DATABASE_URL" up
go run ./cmd/seed          # admin@ / manager@ / sam@…@acme.test, password "password"

# Run (prod assets)
cd web && npm run build && cd ..
go run .                   # http://localhost:8080  → /login

# Tests
go test ./...                                          # unit + no-DB handler test
TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/store/   # + sqlc integration test

# The full regen loop when touching everything:
#   sqlc generate   (query.sql / migrations changed)
#   templ generate  (.templ changed)
#   npm run build   (assets/new Tailwind classes)
#   go build ./...
```

---

## Phase 10 — Put the whole stack under test (testcontainers + a real safety net)

Before evolving the app toward configurable, Odoo-style policy (see `docs/next-steps.md`),
the goal was a **safety net**: prove every layer works today so future refactors have
something to break loudly against. Target: *comfortably past 95% of the hand-written code.*

### The problem: how do integration/e2e tests get a Postgres?

The store, handlers, seed and `RequireAuth` all run **real SQL** through sqlc/pgx
(`EXTRACT(YEAR ...)`, `UPSERT`, `LEFT JOIN … COALESCE`). Mocking sqlc would test a mock,
not the schema. So the tests need an actual Postgres — but one that:
- runs on `go test` with no manual "start a DB and set `TEST_DATABASE_URL`" ceremony,
- is disposable and identical for everyone (and CI),
- applies the **real** `sql/migrations` so tests bind to the true schema.

### Tech: testcontainers-go (throwaway Postgres in Docker)

`internal/testsupport` boots `postgres:16-alpine` in a container, feeds the migration
`*.up.sql` files in as **init scripts** (Postgres runs them on first boot — handles
multi-statement DDL that pgx's extended protocol won't), and hands back a live
`*store.Store`. One container per test **binary** (lazy `sync.Once`), each test
`TRUNCATE … RESTART IDENTITY` for isolation. If Docker is down it `t.Skip`s (and
`-short` skips all DB tests), so the unit suite still runs anywhere in ~4s.

### New concept: consumer-side interfaces as test seams

Handlers, middleware and seed took a *concrete* `*store.Store`, so their error branches
(`c.String(500, …)`) were unreachable without breaking a real DB mid-request. Fix: each
consumer now declares the **narrow interface it actually uses** —
`handlers.Store` (22 methods), `auth.SessionStore` (1), `seed.Store` (9) — and the
concrete store satisfies all three. Two payoffs:
- a **fake store** can return `boom` on any single method, so every 500/503/404 branch
  is drivable through the real router (`fault_test.go`);
- Wire still assembles the app — two `wire.Bind(new(handlers.Store), new(*store.Store))`
  lines teach it the concrete type fills the interface. `make generate` stays green.

### The testing pyramid that resulted

| Level | Where | How |
|---|---|---|
| **Unit** | `leave`, `config`, `auth`, plus each handler's error paths | pure funcs + table tests; `httptest` for cookies/middleware; fake store for faults |
| **Integration** | `store`, `seed` | every method against the container DB (drives generated `db` too) |
| **E2E** | `handlers` + `server` + `views` | real Gin router + real DB + real session cookies; every route, role gate, 400/403/404 |

### What changed in the code (diff)

- **New:** `internal/testsupport/postgres.go` (the harness), `*_test.go` across
  `leave/config/auth/store/seed/handlers/cli/assets` and `main`, plus a `fault_test.go`
  fake in `handlers` and `seed`.
- **Seams:** `handlers.Store`, `auth.SessionStore`, `seed.Store` interfaces;
  `handlers.New` / `server.New` / `seed.Run` take interfaces; `wire.Bind` in `internal/app`.
- **Tooling:** `make test` (`-p 1`, full incl. Docker), `make test-short` (unit only),
  `make cover` (`-count=1`, writes `coverage.out` + `coverage.html`).

### What I observed

- **Hand-written coverage: ~96.5%.** 7 packages at 100% (`leave`, `config`, `store`,
  `server`, `cli`, `app`, `assets`), `handlers` 99.6%, `auth`/`seed` ~97%.
- **Total incl. generated code: ~77%.** The gap is *not* untested logic — it's
  **unreachable branches**: every templ write emits `if err != nil { return err }` that
  can't fire against a buffer (`views` ~76%); sqlc's row-scan error paths (`db` ~89%);
  `main`'s `os.Exit(1)`; `NewToken`'s `crypto/rand` failure. That's the honest ceiling.
- **Parallel containers bite.** `go test ./...` runs package binaries concurrently → 4
  Postgres + 4 Ryuk containers at once → Docker Desktop chokes and some DB tests
  *silently skip* (a skip isn't a failure). `-p 1` serialises them; coverage must also be
  `-count=1` or a stale cached (flaky) profile lies to you.

### Value added

1. **A real regression net.** The M0/M1 refactor in `next-steps.md` (make `WorkingDays`
   configurable, change the leave-year window) now has ~140 assertions watching it.
2. **The refactor paid forward.** The interface seams aren't just for tests — they're the
   clean extension points the roadmap's new stores/services will plug into.
3. **`go test ./...` "just works."** No DB to install, no env var to remember; Docker is
   the only prerequisite, and its absence downgrades gracefully to the unit suite.

### Trade-offs / new pain

- **Docker is now a test dependency.** Full coverage needs a running daemon; `-short`
  is the escape hatch (and what a fast pre-commit hook should use).
- **`-p 1` trades speed for reliability.** Serialising the ~4 container-spinning packages
  costs a few seconds but removes the flaky-skip; worth it.
- **Generated code caps the *total* number.** Chasing the last ~18% would mean testing
  templ/sqlc's own error handling — low value. The meaningful figure is hand-written %.
- **One more interface per consumer.** `handlers.Store` lists 22 methods; it's boilerplate
  to keep in sync when a handler calls a new store method — but it's what makes the
  handler layer unit-testable at all.

### Commands cheat-sheet (additions)

```bash
make test         # everything incl. DB integration/e2e via testcontainers (needs Docker)
make test-short   # fast unit-only suite, no Docker
make cover        # fresh full run -> prints total, writes coverage.out + coverage.html

# The honest hand-written number (excludes generated templ/sqlc/wire):
go test -p 1 -count=1 -coverpkg=./... -coverprofile=coverage.out ./...
grep -vE '_templ\.go:|/internal/db/|wire_gen\.go:' coverage.out > /tmp/hand.out   # keep the `mode:` header
go tool cover -func=/tmp/hand.out | tail -1
```

---

## Phase 11 — Continuous integration (GitHub Actions)

Phase 10 built a test suite that was "disposable and identical for everyone (and CI)" —
but nothing actually *ran* it on every change yet. This phase closes that loop: every push
to `main` and every PR now builds, vets, formats-checks and runs the full suite on a clean
machine, so a broken commit is caught before it's merged instead of on the next `git pull`.

### Tech: GitHub Actions

A single workflow, `.github/workflows/ci.yml`, defines one `build-test` job on
`ubuntu-latest`. A workflow is YAML: `on:` says *when* (push/PR to `main`), `jobs:` say
*what*, and each `steps:` entry is either a reusable **action** (`uses:`) or a shell command
(`run:`). Two actions do the heavy lifting — `actions/checkout` clones the repo and
`actions/setup-go` installs Go and restores the module/build cache.

### What the workflow does

| Step | Command | Why |
|---|---|---|
| **Setup** | `setup-go` with `go-version-file: go.mod` | CI can't drift from the toolchain — the version comes from `go.mod` (1.25.5), not a hardcoded string |
| **Format** | `gofmt -l .` (fail if non-empty) | a formatting gate; mirrors `make fmt` |
| **Vet** | `go vet ./...` | cheap static checks |
| **Build** | `go build ./...` | everything compiles |
| **Test** | `go test -p 1 -count=1 ./...` | the full pyramid, incl. the testcontainers DB tests |

### The key realisation: CI is where the testcontainers bet pays off

The Phase 10 harness `t.Skip`s when Docker is unreachable — locally that's a graceful
downgrade to the unit suite. **`ubuntu-latest` ships a Docker daemon**, so the *same* skip
logic means the integration + e2e tests run **for real** in CI without any service
containers, DSNs or secrets to wire up. The suite that "just works" on a laptop also "just
works" on a fresh runner — that's the whole payoff of booting Postgres from inside the test.

Because the generated code (templ/sqlc/wire) is **committed**, CI installs none of those
tools — it builds and tests exactly what's in the tree. And with no `go:embed` of the Vite
output, the Go build needs no Node step at all; the front-end bundle isn't in the binary.

### What changed in the code (diff)

- **New:** `.github/workflows/ci.yml` — the entire feature is this one file.
- **Fixups:** ran `gofmt -w` on two new test files (`handlers/e2e_test.go`,
  `handlers/fault_test.go`) that had mis-aligned comments — otherwise the format gate would
  have gone red on its first run.

### What I observed

- **`-p 1 -count=1` matters as much in CI as locally.** `-p 1` serialises the
  container-spinning packages (same reason as Phase 10); `-count=1` disables the test cache
  so a green check always means the suite *actually executed*, not "inputs unchanged".
- **`go-version-file` beats a pinned string.** One source of truth (`go.mod`); bumping Go is
  a one-line change that the toolchain and CI both follow.
- **`concurrency: cancel-in-progress`** stops a rapid second push from wasting a runner on
  the now-stale first commit.

### Value added

1. **The safety net is now automatic.** Phase 10's ~140 assertions guard the `next-steps.md`
   refactors on *every* PR, not just when someone remembers to run `make test`.
2. **Zero-config DB tests in CI.** No Postgres service, no secrets — testcontainers +
   Docker-on-the-runner means the integration suite is truly portable.
3. **A visible green/red signal** on every commit and PR, backed by the real schema.

### Trade-offs / new pain

- **CI is minutes, not seconds.** Pulling `postgres:16-alpine` and booting containers per
  package dominates the run; acceptable for a merge gate, but a fast pre-commit hook should
  still use `make test-short`.
- **No "is generated code stale?" guard yet.** CI trusts the committed templ/sqlc/wire
  output. Catching a forgotten `make generate` would mean installing those tools and
  `git diff`-ing after a regen — a worthwhile future step, left out to keep CI lean.
- **The Docker image isn't cached** between runs, so it's re-pulled each time. Fine for now.

### Commands cheat-sheet (additions)

```bash
# Reproduce the CI checks locally before pushing:
gofmt -l .              # must print nothing
go vet ./...
go build ./...
go test -p 1 -count=1 ./...   # == the CI test step (needs Docker for the full suite)

# Watch the run after pushing (GitHub CLI):
gh run watch
gh run list --workflow=ci.yml
```

---

## Phase 12 — Sending email (stdlib `net/smtp`) + account bootstrap

Every account so far was born from the demo `seed` command with the shared password `password`.
That's fine for playing locally, but a real deployment needs a way to get its *first* real
users — an admin and an HR — without hardcoding a password anywhere. This phase adds outbound
**email** (the one new piece of tech) and uses it to bootstrap those two accounts on startup:
generate a random password, email it, and only then create the account.

### Tech: `net/smtp` (Go standard library)

No third-party dependency — the standard library already speaks SMTP. `smtp.SendMail(addr, auth,
from, to, msg)` opens a connection, optionally authenticates (`smtp.PlainAuth`), and writes an
RFC 5322 message (CRLF line endings, a header block, a blank line, then the body). The whole
transport is wrapped behind a one-method interface so the rest of the app never imports
`net/smtp`:

```go
type Mailer interface { Send(to, subject, body string) error }
```

`internal/mailer` has the concrete `*SMTP` implementation; message assembly is split into a pure
`buildMessage()` so the header/MIME formatting is unit-testable without a live server.

### New concept: send-before-persist for a safe bootstrap

The ordering is the interesting bit. `internal/bootstrap.Run` walks the configured admin/HR
emails and, for each account that doesn't exist yet:

1. generates a random password (`auth.GeneratePassword`, `crypto/rand` → base64),
2. **sends the email first**, and
3. only creates the DB row (with the bcrypt hash) if the send succeeded.

If SMTP is down, startup aborts *before* any account is persisted — so you never end up with a
user whose password was emailed into the void and is unknown to everyone. Because existence is
checked per-account, a re-run skips whoever already exists (idempotent), and the SMTP transport
is built **lazily** — a fully-provisioned database boots with no SMTP config at all.

### What changed in the code (diff)

- **`internal/auth/password.go`** — added `GeneratePassword(bytes)` (crypto/rand → `base64.RawURLEncoding`).
- **`internal/mailer/`** (new) — `Mailer` interface, `*SMTP` over `net/smtp`, testable `buildMessage`.
- **`internal/bootstrap/`** (new) — `Run(ctx, Store, Options, newMailer)`; consumer-side `Store`/`Mailer` interfaces; send-before-create; skip-if-exists; lazy mailer.
- **`internal/config/config.go`** — `BASE_URL`, `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_HR_EMAIL`, `SMTP_*`.
- **`internal/cli/serve.go`** — runs `bootstrap.Run` on startup (alongside the existing session cleanup / asset init), aborting on error.
- **Tests** — `GeneratePassword`, `buildMessage`/`NewSMTP` validation, and a bootstrap suite (create+email, skip existing, no-mailer-when-idle, abort-on-send-failure, abort-on-config-error, skip-blank-email) using fake store + fake mailer.

### Value added

1. **A production-usable first login** — no shared demo password baked into a real deployment.
2. **Reusable email transport** — the `Mailer` interface is now available for future notifications (approval decisions, etc.).
3. **Fail-safe provisioning** — the send-before-persist ordering means a mail outage can never strand an account with an unknown password.

### Trade-offs / new pain

- **No local SMTP in dev** — `net/smtp` needs a real relay; there's no console/log fallback (a deliberate "SMTP only" choice), so exercising the *create* path locally means pointing at something like Mailpit/Mailhog or a real provider.
- **Plaintext email** — credentials travel over whatever transport security the relay offers (STARTTLS on 587); acceptable for a bootstrap password the user should change, but there's no "force password reset on first login" yet.
- **Bootstrap vs. seed overlap** — two ways to create users now (`seed` demo data vs. startup bootstrap); kept separate on purpose, but it's one more thing to explain.

### Commands cheat-sheet (additions)

```bash
# Provision the first accounts on startup (needs a reachable SMTP relay):
export BOOTSTRAP_ADMIN_EMAIL=admin@yourco.com BOOTSTRAP_HR_EMAIL=hr@yourco.com
export SMTP_HOST=smtp.yourco.com SMTP_PORT=587 SMTP_USERNAME=... SMTP_PASSWORD=... SMTP_FROM=no-reply@yourco.com
go run . serve      # emails each new account its password, then serves

# Local SMTP sink for testing the create path (example: Mailpit on :1025):
#   docker run -p 1025:1025 -p 8025:8025 axllent/mailpit
#   SMTP_HOST=localhost SMTP_PORT=1025 SMTP_FROM=no-reply@test go run . serve
```



