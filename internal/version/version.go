// Package version holds the droidship version. Release builds set it:
//
//	go build -ldflags "-X github.com/kirvigen/droidship/internal/version.Version=1.2.3"
//
// A `go install …@v1.2.3` build has no ldflags; String falls back to the
// module version Go records in the binary.
package version

import "runtime/debug"

// Version is the droidship release this binary was built from.
var Version = "dev"

// String is the version to show: the ldflags value, else the module version.
func String() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
