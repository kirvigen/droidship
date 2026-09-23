# Working on droidship

- Go 1.24, standard library only. Do not add dependencies.
- Before a commit: `gofmt -l .` prints nothing, and `go vet ./...` and `go test ./...` pass.
- Tests never touch the network. A test that reads credentials sets `HOME` to a temp dir and blanks
  every credential variable (see `noCredentials` in `internal/cli/parity_test.go`).

## Layout

| Path | What it is |
|---|---|
| `cmd/droidship` | entry point |
| `internal/cli` | router, unified commands, output, exit codes |
| `internal/store` | the `Store` interface every adapter implements |
| `internal/config` | credential lookup for all stores |
| `internal/play`, `internal/rustore`, `internal/appgallery` | thin API clients |
| `internal/gplaycmd`, `internal/rustorecmd`, `internal/appgallerycmd` | each store's commands (`droidship <store> …`) and its `Store` adapter (`store.go`) |
| `skills/` | agent skills shipped with the Claude Code plugin |

## Rules

- A new unified command: add it to `store.Store`, implement it in all three adapters (or return
  `store.Unsupported` with the reason), add it to `internal/cli`, and update the README matrix.
- Never remove a command or flag from a store namespace. `internal/cli/parity_test.go` guards every
  command the standalone gplay, rstore and hstore tools had.
- Live checks are read-only: `SMOKE_PACKAGE=com.example make smoke`.
