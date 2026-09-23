// Package version holds the droidship version. Release builds override it:
//
//	go build -ldflags "-X github.com/kirvigen/droidship/internal/version.Version=v1.2.3"
package version

// Version is the droidship release this binary was built from.
var Version = "dev"
