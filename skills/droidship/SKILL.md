---
name: droidship
description: Use when shipping an Android app to Google Play, RuStore or Huawei AppGallery, or reading and answering their reviews — "release to the stores", "upload the aab", "publish to RuStore", "roll out to 20%", "what's live in Play", "answer the 1-star reviews", "update what's new". Drives the droidship CLI.
---

# droidship

One CLI for Google Play, RuStore and Huawei AppGallery.
Install: `go install github.com/kirvigen/droidship/cmd/droidship@latest`.

## Rules

- **Never make a build visible to users without the human's explicit yes in this conversation.**
  `publish` without `--go-live` is safe: it stages the build. `release`, `rollout` and
  `publish --go-live` are not. Say what will happen in each store, then wait.
- Replies to reviews are public. Show the text and get a yes before `reply`.
- Pass `--json` and read the exit code: 0 ok, 1 error, 2 usage, 3 unsupported by that store.
  Exit 3 is not something to retry. Tell the human which store cannot do it.
- Read before you write: run `droidship status <pkg>` first.

## Start

    droidship auth --json          # which stores are configured, as whom, and from where

If a store is missing here, it needs credentials: point the human at the Credentials section of
the README. Never ask them to paste a secret into the chat.

## Release a build

    droidship status  com.example --json
    droidship publish com.example --store all --aab app-release.aab --dry-run --json
    # show the human the plan and every "note" (a version already in review, a draft that
    # would be replaced), then:
    droidship publish com.example --store gplay,rustore,appgallery --aab app-release.aab \
        --notes-file whatsnew-ru.txt --json

Each store reports its state and the command that makes the build live. What `publish` does:

| Store | `publish` | `release` |
|---|---|---|
| Google Play | draft release on production | starts the rollout of that draft |
| RuStore | sent to moderation, manual publication | publishes once status is `READY_FOR_PUBLICATION` |
| AppGallery | uploaded, not submitted | submits for Huawei review; goes live once approved |

Google Play takes only `--aab`. RuStore and AppGallery take `--apk` or `--aab`.

After the human approves:

    droidship release com.example --store gplay --percent 10
    droidship release com.example --store rustore
    droidship release com.example --store appgallery

Widen a staged rollout: `droidship rollout com.example --store gplay --percent 50`.
RuStore rolls out in steps of 5, 10, 25, 50, 75 and 100. `--percent 100` completes a rollout.

## Reviews

    droidship reviews com.example --unanswered --json      # every store, newest first
    droidship reply com.example <id> --store rustore --text "…"

Reply limits: Google Play 350 characters, RuStore 500. Google Play only returns the last 7 days.

## Store-specific work

Everything else lives under the store's name, with the flags of the original tools:

- `droidship gplay listing set|screenshots|details|achievements|bundles|tracks …`
- `droidship rustore versions|draft delete …`
- `droidship appgallery info|submit|withdraw|notes --app-name … --brief … --description …`

Run `droidship <store> help` for the full list.

Creating an app, Data safety and the content rating are not in any API. For those, use the
`play-console-browser` skill.
