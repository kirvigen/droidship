// Package cli routes droidship's command line: store namespaces go to the
// legacy per-store CLIs, everything else is a unified verb.
package cli

import (
	"fmt"
	"io"

	"github.com/kirvigen/droidship/internal/appgallerycmd"
	"github.com/kirvigen/droidship/internal/gplaycmd"
	"github.com/kirvigen/droidship/internal/rustorecmd"
	"github.com/kirvigen/droidship/internal/version"
)

// Exit codes. They are part of the public contract; agents rely on them.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitUnsupported = 3
)

// Run executes one droidship invocation and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintln(stdout, "droidship", version.String())
		return ExitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	case "gplay":
		return gplaycmd.Run(args[1:])
	case "rustore":
		return rustorecmd.Run(args[1:])
	case "appgallery":
		return appgallerycmd.Run(args[1:])
	}
	if _, ok := verbs[args[0]]; ok {
		return runVerb(args[0], args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
	return ExitUsage
}

const usage = `droidship — ship Android apps to Google Play, RuStore and Huawei AppGallery

Usage:
  droidship <command> [flags]

Unified commands (the same flags for every store):
  auth    [--store S]                          which stores are configured, and as whom
  status  <pkg> [--store S]                    versions, tracks, review state
  publish <pkg> --store S (--aab F | --apk F)  upload and stage a build
          [--notes S | --notes-file F] [--lang ru-RU] [--percent P] [--go-live]
          [--dry-run]                          check every store, change nothing
  release <pkg> --store S [--version V] [--percent P]
  rollout <pkg> --store S --percent P [--version V]
  notes   <pkg> --store S [--lang ru-RU] (--text S | --text-file F)
  reviews <pkg> [--store S] [--stars N] [--unanswered] [--days 7] [--limit 50]
  reply   <pkg> <reviewId> --store S (--text S | --text-file F)
  listing <pkg> [--store S] [--lang ru-RU]

  --store is gplay, rustore, appgallery, a comma list, or all. Read commands
  default to every configured store; write commands need --store. Every
  command takes --json. publish stages the build: nobody sees it until release.

Store-specific commands (everything the standalone tools could do):
  droidship gplay help         tracks, bundles, listing, details, screenshots, achievements
  droidship rustore help       apps, versions, draft delete
  droidship appgallery help    info, submit, withdraw, notes, regions

Exit codes: 0 ok, 1 error, 2 usage, 3 unsupported by the store.
`
