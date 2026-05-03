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
go run cmd/server/main.go
```

Runtime-generated files live under `data/`.

## Runtime Model

Startup is deterministic:

1. load runtime config
2. create the renderer from the app filesystem
3. open the SQLite adapter
4. construct repositories, services, middleware, handlers, and jobs explicitly
5. validate and run migrations
6. build the HTTP stack
7. register app worker jobs when worker runtime is enabled
8. start the optional worker runtime
9. start the server

## Database And Migrations

Currently it supports SQLite only at runtime.

The codebase keeps small DB seams so repositories and services do not couple directly to SQLite details.

Migrations are SQL-only and centralized:

- `app/migrations/sqlite/app/*.sql`
- `app/migrations/sqlite/worker/*.sql`

Startup validates the migration stream before executing anything. Invalid filenames, sequence gaps, duplicate sequences, checksum mismatches, or missing applied files abort startup and run zero migrations.

## Public Contract

Surfaces and behaviors are documented in [CONTRACT.md](./CONTRACT.md).

