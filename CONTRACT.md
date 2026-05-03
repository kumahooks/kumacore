# kumacore Contract

This document defines the contract for `kumacore`.

## Stable Surfaces

### `core/app`

Purpose:
- deterministic application bootstrap and runtime lifecycle

What it does:
- startup follows the fixed boot order documented by the repository architecture
- initialization aborts on any startup failure
- static registration, route registration, migration execution, and optional worker startup are explicit lifecycle steps
- graceful shutdown closes runtime resources explicitly

### `core/config`

Purpose:
- runtime configuration loading and validation

What it does:
- runtime config domain for server, app mode, database, logs, sessions, and worker settings
- invalid critical runtime configuration fails startup early

### `core/render`

Purpose:
- Go template rendering with a HTMX behavior contract

What it does:
- direct requests render the full base page
- `HX-Request: true` renders the requested fragment
- history restore requests render a full page when required by runtime behavior
- template parsing preserves clone-based page isolation
- dev mode bypasses page cache and production mode caches parsed pages

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
- copied implementation files are app-owned source after generation
- generated bootstrap wiring is explicit
- runtime behavior does not depend on module directory scanning or hidden autoloading
- baseline app structure is generated under top-level `core/`, `app/`, and `cmd/`

