# kumacore Contract

This document defines the contract for `kumacore`.

## Stable Surfaces

### `core/app`

Purpose:
- deterministic application bootstrap and runtime lifecycle

What it does:
- startup follows the fixed boot order documented by the repository architecture
- initialization aborts on any startup failure
- logging is initialized first: the console and `data/logs` writer is opened, then the slog default logger is installed
- log files are named by UTC date and rotate at UTC midnight
- static registration, route registration, migration execution, and optional worker startup are explicit lifecycle steps
- graceful shutdown closes runtime resources explicitly
- the HTTP server sets a 5-minute read timeout alongside header, write, and idle timeouts

### `core/config`

Purpose:
- runtime configuration loading and validation

What it does:
- runtime config domain for server, app mode, database, logs, sessions, and worker settings
- `CORE_SERVER_MAX_BODY_BYTES` defaults to 64 MiB and must be positive
- invalid critical runtime configuration fails startup early

### `core/security`

Purpose:
- baseline HTTP security headers and same-origin enforcement

What it does:
- every response sets `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, a restrictive `Content-Security-Policy`, and `Permissions-Policy`
- `SameOriginMiddleware` rejects unsafe methods with a mismatched `Origin` or `Referer` host, or `Sec-Fetch-Site: cross-site`, with status 400
- only the request `Host` is accepted, while `X-Forwarded-Host` is ignored

### `core/httpx`

Purpose:
- request and cookie helpers shared by app code

What it does:
- `LimitRequestBody` caps request body reads with `http.MaxBytesReader`. Non-positive limits disable the cap
- `SetCookie` and `ClearCookie` write session cookies with Path `/`, HttpOnly, caller-selected Secure, and SameSite Strict
- `ClearCookie` expires the cookie with MaxAge -1

### `core/log`

Purpose:
- slog default logger configuration and request correlation

What it does:
- `Setup` installs the slog default logger, Debug in development and Info in production
- records use a single-line format: `(2006-01-02T15:04:05.000-07:00) [LEVEL - caller] message key=value`
- `caller` is passed as an attribute and rendered inside the level brackets
- `RequestIDMiddleware` trusts an inbound `X-Request-Id`, truncates it to 64 bytes, generates 16 random bytes when absent, stores it in the request context, and echoes it on the response
- `RequestLogger` emits one line per request with method, path, status, and duration
- records logged with a request context carry `request_id`
- the status recorder exposes `Unwrap` so `http.ResponseController` reaches the real connection

### `core/render`

Purpose:
- Go template rendering with a HTMX behavior contract

What it does:
- direct requests and history restores render the full base page
- `HX-Request-Type: partial` renders the requested fragment
- responses vary on `HX-Request-Type`
- `RenderError` renders the configured error template as a full page with the requested status
- partial HTMX errors respond with `HX-Redirect` and an empty body so the client re-enters as a direct load
- production validates the error template at startup, while dev re-reads it per request
- template parsing preserves clone-based page isolation
- module-local components under `modules/<name>/components/*.html` are parsed with their page
- duplicate define names across module component files abort the build
- template paths are rooted at `app/`

### `core/db` seams

Purpose:
- minimal database boundaries between app repositories and the runtime adapter

What it does:
- app repositories depend on small DB interfaces instead of concrete SQLite types
- dialect metadata stays minimal and runtime-focused
- SQLite open behavior remains the supported runtime path

### `core/db/migrate`

Purpose:
- validated SQL migration discovery, integrity checking, and execution

What it does:
- migration discovery ignores dotfiles
- filenames must match `NNNN_slug.sql`
- sequence numbers must be contiguous and unique
- applied migration checksums must match current file contents
- validation runs before execution and any invalid state aborts execution

### `core/worker`

Purpose:
- optional DB-backed worker runtime

What it does:
- worker runtime is gated by `CORE_WORKER_ENABLED`
- worker runtime uses a dedicated SQLite database
- job queue lifecycle supports pending, running, retry, complete, and graveyard behavior
- orphaned running jobs are reset during startup
- app job handlers are registered explicitly by bootstrap code

### Scaffold Copy Contract

Purpose:
- define what `kumacore init` guarantees about generated app code

What it does:
- selected HTTP-facing modules are copied into the generated app under `app/modules/`
- `errorscreen` is baseline, always copied, and wired to the router's `NotFound` path
- `app/embed.go` is generated with embed patterns for the selected modules that contain page templates, plus the
  baseline error screen and module component fragments
- copied implementation files are app-owned source after generation
- generated bootstrap wiring is explicit
- runtime behavior does not depend on module directory scanning or hidden autoloading
- baseline app structure is generated under top-level `core/`, `app/`, and `cmd/`

