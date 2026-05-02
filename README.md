# kumacore

`kumacore` is a Go and HTMX project starter generator developed from my personal website.

It targets single-server deployment on cheap VPS infrastructure with SQLite as the only runtime database.

## Goals

- keep generated apps small and explicit
- preserve server-first Go and HTMX architecture
- copy editable starter modules into each generated app
- keep SQL explicit and local to each app module
- fail early on invalid configuration, module registration, or migration state

## Local start

Run the current home-only development composition:

```sh
go run cmd/server/main.go
```

Open `http://127.0.0.1:3000`.

Runtime files are under data/.
