# Changelog

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
