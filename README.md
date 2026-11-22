# kumacore

`kumacore` is a Go and HTMX starter generator extracted from my personal servers.

It targets single-server deployment on cheap VPS infrastructure with SQLite as the only runtime database currently, with support to add new ones.

## Goals

- keep generated apps small and explicit
- preserve server-first Go and HTMX architecture
- keep SQL explicit and local to repositories

## Architecture

`kumacore` is split into three layers:

- `core/` engine runtime packages
- `app/` application packages built on the engine
- `cmd/` executables for local development and scaffolding

The generated app owns its copied application code under top-level `app/`.

HTTP-facing modules live under `app/modules/`. They own routes, handlers, and rendered pages. Worker jobs live under `app/jobs/`.

## Available Modules

`kumacore init <app-name>` lets the user select which scaffold modules to copy into the generated app.

- `home` default home page
- `auth` login, logout, session hydration, and authenticated user status
- `health` liveness and readiness endpoints

## Quickstart

```sh
go run ./cmd/kumacore/main.go init myapp
cd myapp
go run ./cmd/server
```

Runtime-generated files live under `data/`.

## Runtime Model

Startup is deterministic:

1. load runtime config
2. create the renderer from the app filesystem, parsing the error template at startup in production
3. open the SQLite adapter
4. construct repositories, services, middleware, handlers, and jobs explicitly
5. initialize logging, installing the `slog` default logger for stdout and the UTC-dated file under `data/logs`
6. validate and run migrations
7. build the HTTP stack, including the request ID, request logger, request body cap, security headers, and same-origin checks
8. register app worker jobs when worker runtime is enabled
9. start the optional worker runtime
10. start the server

## Database And Migrations

Currently it supports SQLite only at runtime.

The codebase keeps small DB seams so repositories and services do not couple directly to SQLite details.

Migrations are SQL-only and centralized:

- `app/migrations/sqlite/app/*.sql`
- `app/migrations/sqlite/worker/*.sql`

Startup validates the migration stream before executing anything. Invalid filenames, sequence gaps, duplicate sequences, checksum mismatches, or missing applied files abort startup and run zero migrations.

SQLite connections enable WAL, `synchronous=NORMAL`, foreign keys, and a 5000 ms busy timeout through DSN pragmas.

## Worker Runtime

Worker runtime is optional and gated by `CORE_WORKER_ENABLED`. It uses a dedicated SQLite database, resets orphaned running jobs at startup, and exposes queue counts through `Runtime.Stats`. Scheduled jobs first run one interval after start, then repeat at that interval.

## Public Contract

Surfaces and behaviors are documented in [CONTRACT.md](./CONTRACT.md).
