# Changelog

## v0.2.0 — 2026-09-24

### Added
- `publish --dry-run`: checks every selected store without changing anything (access to the app,
  build format, rollout step) and shows what is live and what publishing would do, with warnings
  such as a version already in review.
- Tables fit the terminal: the review text and listing columns shrink to the terminal width
  (or `$COLUMNS`) instead of wrapping.

## v0.1.0 — 2026-09-23

First release. Merges three standalone CLIs into one:

- `gplay` (Google Play) → `droidship gplay …`
- `rstore` (RuStore) → `droidship rustore …`
- `hstore` (Huawei AppGallery) → `droidship appgallery …`

Every command and flag of the three tools works unchanged under its store's name, and so do their
environment variables and config files.

### Added

- Unified commands across stores: `auth`, `status`, `publish`, `release`, `rollout`, `notes`,
  `reviews`, `reply`, `listing`, with `--store`, `--json`, and exit code 3 for "unsupported by the store".
- `publish` stages builds in every store; `--go-live` ships them.
- RuStore: reviews and replies.
- AppGallery: changing a running phased rollout, reading the store listing.
- One config file for every store (`~/.config/droidship/config.json`) and `DROIDSHIP_*` variables.
- Agent skills (`droidship`, `play-console-browser`) and a Claude Code plugin marketplace.
- Release binaries for macOS, Linux and Windows.
