# Design

droidship merges three single-store CLIs (gplay, rstore, hstore) into one binary without dropping a
single command, and adds a layer of commands that work the same across stores.

## Two layers

```
droidship <store> <command> …     store namespace: every command and flag of the original tool
droidship <verb> … --store S      unified verb: same flags everywhere, fans out over stores
```

The store namespaces are the original CLIs, moved into packages with an exported `Run`. They keep
their flags, output and behaviour byte for byte; `internal/cli/parity_test.go` lists every command
the standalone tools had and fails if one stops routing.

The unified verbs sit on one interface:

```go
type Store interface {
	Auth(ctx) (AuthInfo, error)
	Status(ctx, pkg) ([]Release, error)
	Publish(ctx, PublishRequest, log) (PublishResult, error)
	Release(ctx, ReleaseRequest, log) error
	Rollout(ctx, RolloutRequest, log) error
	Notes(ctx, pkg, lang, text) error
	Reviews(ctx, ReviewsQuery) ([]Review, error)
	Reply(ctx, pkg, reviewID, text) error
	Listing(ctx, pkg, lang) (Listing, error)
	ReplyLimit() int
}
```

Each store package implements it in `store.go`, reusing the namespace's own pipelines (upload,
commit, retry-while-processing) rather than duplicating them. A verb a store's API cannot do returns
`store.Unsupported(store, verb, reason)`; the CLI prints the reason and exits 3.

```
cmd/droidship ─▶ internal/cli ─┬─▶ internal/gplaycmd ─────▶ internal/play        Google Play Developer API
                               ├─▶ internal/rustorecmd ───▶ internal/rustore     RuStore Public API
                               ├─▶ internal/appgallerycmd ▶ internal/appgallery  AppGallery Connect API
                               └─▶ internal/store (interface), internal/config (credentials)
```

## Decisions

- **Safe by default.** `publish` stages a build in every store (a Play draft, RuStore moderation
  with manual publication, an AppGallery upload without submission). Going live is a separate
  command or an explicit `--go-live`. A wrong flag on a release tool reaches every user; a re-run
  does not fix that.
- **Write commands need `--store`.** Read commands default to every configured store; anything that
  changes a store must name it.
- **Normalised inputs.** Percent is always 0–100 and converted per store (Play: a fraction; RuStore:
  a fixed step; AppGallery: a phased window). Languages are BCP-47. Reply length is checked against
  the store's limit before any request.
- **Exit codes are a contract** (0 ok, 1 error, 2 usage, 3 unsupported) and every command has
  `--json`, so agents can act on results without parsing tables. With several stores, any error
  wins over unsupported.
- **Credentials resolve in one place.** Flag, `DROIDSHIP_*`, the old tools' variables, the droidship
  config file, the old tools' files. Existing setups keep working; `auth` reports where each
  credential came from and never its value.
- **Standard library only.** One static binary, nothing to audit but this repository.

## Testing

- Unit tests run against `httptest` fakes of each API; they never reach the network and blank all
  credential variables.
- API behaviour not covered by documentation (RuStore review endpoints, AppGallery listing and
  phased-release updates) was checked against the live APIs read-only before the adapters were
  written; `scripts/smoke.sh` repeats those read-only checks.
