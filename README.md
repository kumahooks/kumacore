# kumacore

`kumacore` is a Go and HTMX project starter generator developed from my personal website.

It targets single-server deployment on cheap VPS infrastructure with SQLite as the only runtime database.

## Goals

- keep generated apps small and explicit
- preserve server-first Go and HTMX architecture
- copy editable starter modules into each generated app
- keep SQL explicit and local to each app module
- fail early on invalid configuration, module registration, or migration state

## Available Modules

`kumacore init <project-name>` lets the user select which starter modules to include.

- `home` default home page
- `auth` login, logout, and authenticated user status
- `health` liveness and readiness endpoints

In interactive mode, the CLI prints the available modules and defaults to `home`.

## Generated App Start

```sh
go run ./cmd/kumacore/main.go init myapp
cd myapp
go run cmd/server/main.go
```

Open `http://127.0.0.1:3000`.

Runtime files are under `data/`.
