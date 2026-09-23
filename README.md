<p align="center"><img src="docs/assets/banner.png" alt="droidship: one CLI for Google Play, RuStore and Huawei AppGallery" width="100%"></p>

# droidship

**Ship Android apps to Google Play, RuStore and Huawei AppGallery from one CLI.** Upload a build,
stage it, roll it out, answer reviews — the same commands for every store, from a terminal or an
AI agent.

[![ci](https://github.com/kirvigen/droidship/actions/workflows/ci.yml/badge.svg)](https://github.com/kirvigen/droidship/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/kirvigen/droidship)](https://github.com/kirvigen/droidship/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/kirvigen/droidship)](go.mod)
[![license](https://img.shields.io/github/license/kirvigen/droidship)](LICENSE)

[Читать на русском](README.ru.md)

```console
$ droidship status com.example
STORE       TRACK       VERSION      STATUS                 ROLLOUT  ID
gplay       production  30           completed
rustore     manual      1.4.16 (29)  MODERATION                      2051234567
appgallery  latest      1.4.16 (29)  pending update review           1998765432101234567
appgallery  live        1.4.11 (23)  on shelf
```

- **One command, three consoles.** `publish`, `release`, `rollout`, `reviews` and `reply` work the
  same way in every store. `--store all` runs them everywhere at once.
- **Safe by default.** `publish` uploads and stages a build. Nobody sees it until you run `release`.
- **Nothing lost.** Every store-specific feature is still there under the store's name: Play Games
  achievements, screenshots, RuStore drafts, AppGallery phased windows.
- **Built for agents.** Every command takes `--json`, exit codes are a contract, and skills for
  Claude Code ship in the repo.
- **One static binary.** Go standard library only, no dependencies.

## Install

```bash
go install github.com/kirvigen/droidship/cmd/droidship@latest
```

Or download a binary for macOS, Linux or Windows from
[Releases](https://github.com/kirvigen/droidship/releases).

## Quick start

```bash
droidship auth                                    # which stores are configured, and as whom
droidship status com.example                      # what is live where
droidship publish com.example --store all \
  --aab app-release.aab --notes-file whatsnew.txt # upload and stage everywhere
droidship release com.example --store gplay --percent 10
droidship reviews com.example --unanswered        # every store, newest first
```

## What each store can do

| Command | Google Play | RuStore | AppGallery |
|---|:-:|:-:|:-:|
| `auth` | ✓ | ✓ | ✓ |
| `status` | ✓ | ✓ | ✓ |
| `publish` | ✓ AAB only | ✓ APK / AAB | ✓ APK / AAB |
| `release` | ✓ | ✓ | ✓ |
| `rollout` | ✓ | ✓ steps of 5–100 % | ✓ |
| `notes` | ✓ | — ¹ | ✓ |
| `reviews` | ✓ last 7 days ² | ✓ | ✓ |
| `reply` | ✓ 350 chars | ✓ 500 chars | ✓ |
| `listing` | ✓ | — ³ | ✓ |

¹ RuStore takes release notes only with a new version: `publish --notes`.
² The Play API serves only reviews from the last week.
³ The RuStore API has no store-page read.

An unsupported command exits with code **3** and a one-line reason. The other stores still run.

## Credentials

Every store reads its credentials in this order, first match wins:

1. a flag (`--key` for Google Play; `--app-id`, `--region` in the AppGallery namespace);
2. `DROIDSHIP_*` environment variables;
3. the variables the standalone tools read (`GPLAY_SA_JSON`, `RUSTORE_KEY_ID`, `HSTORE_*`, …);
4. `~/.config/droidship/config.json`, or the file named by `$DROIDSHIP_CONFIG`;
5. the files the standalone tools read (`~/.config/gplay/*.json`, `~/.config/hstore/credentials.json`).

A machine set up for gplay, rstore or hstore works without changes. `droidship auth` shows where each
store's credentials came from, never the values.

```json
{
  "gplay":      { "key": "~/.config/droidship/play-service-account.json" },
  "rustore":    { "key_id": "123", "private_key": "MIIEvQIBADAN…" },
  "appgallery": { "client_id": "…", "client_secret": "…", "region": "global" }
}
```

Keep the file private: `chmod 600 ~/.config/droidship/config.json`.

### Google Play

droidship signs in as a **Google Cloud service account**. Two things must be true:

1. **The key exists.** Create a service account in a Cloud project with the Google Play Android
   Developer API enabled:

   ```bash
   gcloud services enable androidpublisher.googleapis.com --project=PROJECT
   gcloud iam service-accounts create droidship --project=PROJECT
   gcloud iam service-accounts keys create ~/.config/droidship/play-service-account.json \
     --iam-account=droidship@PROJECT.iam.gserviceaccount.com
   ```

   The account needs no IAM roles in the Cloud project.
2. **The key has access to the app.** Play Console → *Users and permissions* → *Invite new users* →
   the service account email → the app → release permissions. A fresh grant takes a few minutes.

`droidship auth` proves step 1. Only `droidship status <package> --store gplay` proves step 2.
Variable: `DROIDSHIP_GPLAY_KEY` (path to the JSON key).

### RuStore

RuStore Console → *Company* (or *Developer*) → *API RuStore* → *Create a key*. Pick the apps and at
least the *App upload and publication* methods. droidship signs a timestamp with the private key
(SHA512withRSA) and trades the signature for a short-lived token; the key never leaves your machine.

Variables: `DROIDSHIP_RUSTORE_KEY_ID`, `DROIDSHIP_RUSTORE_PRIVATE_KEY` (base64, one line, as issued).

### Huawei AppGallery

AppGallery Connect → *Users and permissions* → *API key* → *Connect API* → *Create*. Releases need the
*App administrator* role; replying to reviews needs *Customer service*. **Set Project to N/A**: a
project-scoped client gets 403. The id and secret are shown once.

Variables: `DROIDSHIP_APPGALLERY_CLIENT_ID`, `DROIDSHIP_APPGALLERY_CLIENT_SECRET`, and optionally
`DROIDSHIP_APPGALLERY_APP_ID`, `…_PACKAGE`, `…_REGION` (`global`, `ru`, `eu`, `sg`). An account lives
in one data centre; `droidship appgallery auth --probe` finds which. The JSON with
`key_id`/`private_key` that the console offers for download is a different scheme; droidship says so
if you point it there.

## Unified commands

```text
droidship auth    [--store S]                          which stores are configured, and as whom
droidship status  <pkg> [--store S]                    versions, tracks, review state
droidship publish <pkg> --store S (--aab F | --apk F)  upload and stage a build
                  [--notes S | --notes-file F] [--lang ru-RU] [--percent P] [--go-live]
droidship release <pkg> --store S [--version V] [--percent P]
droidship rollout <pkg> --store S --percent P [--version V]
droidship notes   <pkg> --store S [--lang ru-RU] (--text S | --text-file F)
droidship reviews <pkg> [--store S] [--stars N] [--unanswered] [--days 7] [--limit 50]
droidship reply   <pkg> <reviewId> --store S (--text S | --text-file F)
droidship listing <pkg> [--store S] [--lang ru-RU]
```

`--store` is `gplay`, `rustore`, `appgallery`, a comma list, or `all`. Read commands default to every
configured store. Commands that change something require `--store`, so nothing ships by accident.
`reply` takes exactly one store, because a review id belongs to one store.

### Safe by default

| Store | `publish` | `publish --go-live` | `release` |
|---|---|---|---|
| Google Play | draft release on production | rollout starts (`--percent` = staged) | rolls out the draft |
| RuStore | moderation, then waits for you | goes live once moderation passes | publishes a moderated version |
| AppGallery | uploaded and attached, not submitted | submitted; live once approved | submits for review |

Percentages are always 0–100; droidship converts them (Play wants a fraction, RuStore a fixed step,
AppGallery a phased window of 7 days). Languages are always BCP-47, like `ru-RU`.

## Store-specific commands

Everything the standalone tools did, with the same flags. `droidship <store> help` lists it all.

**Google Play** — `droidship gplay …`

- `tracks`, `bundles`, `upload` (any track, `--mapping`, `--validate-only`), `release`, `rollout`;
- `listing` / `listing set`: title, short and full description, with length checks and `--dry-run`;
- `details` / `details set`: contact email, phone, website;
- `screenshots`, `screenshots upload --replace`, `screenshots delete`: every image type, validated
  before upload;
- `achievements sync|list|publish|delete`: Play Games achievements from a JSON spec, with a lock file
  that maps your ids to Google's.

**RuStore** — `droidship rustore …`

- `apps`, `versions`, `publish` (`--hms-apk`, `--publish-type`, `--publish-date`, `--partial`,
  `--priority`, `--skip-commit`), `release`, `rollout`, `draft delete`.

**Huawei AppGallery** — `droidship appgallery …`

- `auth --probe` to find the data centre, `apps`, `info --phased`;
- `publish` with `--remark`, `--release-time`, phased windows (`--phased`, `--phased-from`,
  `--phased-to`), `--upload-only`, `--wait`;
- `submit`, `withdraw`, `notes` (with `--app-name`, `--brief`, `--description`);
- `reviews` by country, rating and date window; `reply` with `--update-reply-id`.

## Use with AI agents

droidship is meant to be driven by agents: every command takes `--json`, errors go to stderr, and the
exit code says what happened.

| Exit | Meaning |
|---|---|
| 0 | done |
| 1 | an API, network or validation error |
| 2 | the command line is wrong |
| 3 | the store cannot do this (the others still ran) |

**Claude Code.** The repository is a plugin marketplace:

```text
/plugin marketplace add kirvigen/droidship
/plugin install droidship@droidship
```

This installs two skills:

- `droidship`: releases, rollouts and reviews through the CLI, with one hard rule: nothing reaches
  users without a human's explicit yes;
- `play-console-browser`: the Play Console work no API can do (creating an app, Data safety, IARC),
  through a browser.

Other agents: copy `skills/droidship/SKILL.md` into the agent's skills or instructions.
`AGENTS.md` describes the codebase for agents that change it.

Things to ask your agent:

> *Ship 1.5.0 to all three stores, staged only.*
> The agent runs `droidship status`, then `droidship publish <pkg> --store all --aab … --json`, and
> reports each store's state with the command that would make it live. It waits for your yes.

> *Answer today's one-star reviews in RuStore.*
> `droidship reviews <pkg> --store rustore --stars 1 --days 1 --json`, a draft reply per review for
> you to approve, then `droidship reply … --store rustore`.

> *Where is 1.4.16 stuck?*
> `droidship status <pkg> --json`: in moderation in RuStore, pending review in AppGallery, a draft in Play.

## Migrating from gplay, rstore, hstore

| Before | After |
|---|---|
| `gplay upload com.example --aab app.aab` | `droidship gplay upload com.example --aab app.aab` |
| `rstore publish com.example --apk app.apk` | `droidship rustore publish com.example --apk app.apk` |
| `hstore publish --aab app.aab` | `droidship appgallery publish --aab app.aab` |

Commands and flags are unchanged, and so are the environment variables and config files. A test in
`internal/cli/parity_test.go` keeps it that way.

## Sharp edges

- **Google Play:** updating a track replaces its release list; `release` and `rollout` read the track
  first so release notes survive. Some apps cannot have changes auto-sent for review; droidship
  notices and commits with `changesNotSentForReview`. An edit that is not committed is always
  discarded, so a failed run leaves nothing behind.
- **RuStore:** draft fields you omit are inherited from the active version. Creating a draft while
  another exists silently replaces it. The rollout percentage can only grow. AAB upload needs a
  signing key set up in the console.
- **AppGallery:** the reviewer note (`--remark`) must be 10–300 characters. The same version cannot
  be submitted twice. An AAB needs a few minutes to compile; droidship waits (`--wait`, 20 minutes).
  Review queries cover at most six months. Creating an app and the content rating are web-only.

## Development

```bash
make test     # gofmt-clean, vet and unit tests: offline
make build
SMOKE_PACKAGE=com.example make smoke   # read-only checks against the live stores
```

## License

[MIT](LICENSE)
