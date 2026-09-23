// Command droidship ships Android apps to Google Play, RuStore and Huawei
// AppGallery from one CLI. See README.md.
package main

import (
	"os"

	"github.com/kirvigen/droidship/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
