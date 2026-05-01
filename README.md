# kumacore

`kumacore` is a modular Go and HTMX project starter stack developed from my personal website.

It targets single-server deployment on cheap VPS infrastructure with SQLite as the only runtime database.

## Goals

- keep runtime behavior small and explicit
- preserve server-first Go and HTMX architecture
- use opt-in module
- keep SQL explicit and local to each module
- fail early on invalid configuration, module graphs, or migration state

