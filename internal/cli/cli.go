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
		fmt.Fprintln(stdout, "droidship", version.Version)
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
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
	return ExitUsage
}

const usage = `droidship — ship Android apps to Google Play, RuStore and Huawei AppGallery

Usage:
  droidship <command> [flags]

Run 'droidship help' for the full list.
`
