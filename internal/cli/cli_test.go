package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := run(t, "version")
	if code != 0 || !strings.HasPrefix(out, "droidship ") {
		t.Fatalf("version: code %d, out %q", code, out)
	}
}

func TestNoArgsPrintsUsage(t *testing.T) {
	code, _, errOut := run(t)
	if code != 2 || !strings.Contains(errOut, "Usage:") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := run(t, "frobnicate")
	if code != 2 || !strings.Contains(errOut, `unknown command "frobnicate"`) {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}

func TestStoreNamespacesRoute(t *testing.T) {
	for _, store := range []string{"gplay", "rustore", "appgallery"} {
		// Legacy help prints to os.Stdout; exit 0 proves the namespace routed.
		if code, _, _ := run(t, store, "help"); code != 0 {
			t.Fatalf("%s help: exit %d", store, code)
		}
	}
}
