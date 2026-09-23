package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// legacyCommands lists every command of gplay, rstore and hstore as of the
// merge. None of them may disappear from droidship.
var legacyCommands = [][]string{
	{"gplay", "auth"}, {"gplay", "tracks", "com.x"}, {"gplay", "bundles", "com.x"},
	{"gplay", "upload", "com.x"}, {"gplay", "release", "com.x"}, {"gplay", "rollout", "com.x"},
	{"gplay", "reviews", "com.x"}, {"gplay", "reply", "com.x", "1"},
	{"gplay", "listing", "com.x"}, {"gplay", "listing", "set", "com.x"},
	{"gplay", "details", "com.x"}, {"gplay", "details", "set", "com.x"},
	{"gplay", "screenshots", "com.x"}, {"gplay", "screenshots", "upload", "com.x"},
	{"gplay", "screenshots", "delete", "com.x"},
	{"gplay", "achievements", "list"}, {"gplay", "achievements", "sync"},
	{"gplay", "achievements", "publish"}, {"gplay", "achievements", "delete"},
	{"rustore", "auth"}, {"rustore", "apps"}, {"rustore", "versions", "com.x"},
	{"rustore", "publish", "com.x"}, {"rustore", "release", "com.x", "1"},
	{"rustore", "rollout", "com.x", "1", "10"}, {"rustore", "draft", "delete", "com.x", "1"},
	{"appgallery", "auth"}, {"appgallery", "apps", "com.x"}, {"appgallery", "info"},
	{"appgallery", "publish"}, {"appgallery", "submit"}, {"appgallery", "withdraw"},
	{"appgallery", "notes"}, {"appgallery", "reviews"}, {"appgallery", "reply", "1", "hi"},
}

// credentialVars are every variable any store reads credentials from.
var credentialVars = []string{
	"GPLAY_SA_JSON", "RUSTORE_KEY_ID", "RUSTORE_API_KEY",
	"HSTORE_CLIENT_ID", "HSTORE_CLIENT_SECRET", "HSTORE_APP_ID", "HSTORE_PACKAGE", "HSTORE_REGION",
	"HSTORE_CREDENTIALS", "HSTORE_CONFIG_DIR",
	"HUAWEI_CLIENT_ID", "HUAWEI_CLIENT_SECRET", "HUAWEI_APP_ID", "HUAWEI_PACKAGE", "HUAWEI_REGION",
	"DROIDSHIP_CONFIG", "DROIDSHIP_GPLAY_KEY", "DROIDSHIP_RUSTORE_KEY_ID", "DROIDSHIP_RUSTORE_PRIVATE_KEY",
	"DROIDSHIP_APPGALLERY_CLIENT_ID", "DROIDSHIP_APPGALLERY_CLIENT_SECRET",
	"DROIDSHIP_APPGALLERY_APP_ID", "DROIDSHIP_APPGALLERY_PACKAGE", "DROIDSHIP_APPGALLERY_REGION",
}

// noCredentials makes every store's credential lookup come up empty.
func noCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, v := range credentialVars {
		t.Setenv(v, "")
	}
}

// captureStderr runs fn with os.Stderr redirected and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	fn()
	w.Close()
	os.Stderr = orig
	return <-done
}

func TestEveryLegacyCommandIsRouted(t *testing.T) {
	noCredentials(t)
	for _, args := range legacyCommands {
		var code int
		var errOut string
		stderr := captureStderr(t, func() { code, _, errOut = run(t, args...) })
		all := stderr + errOut
		if strings.Contains(all, "unknown command") {
			t.Errorf("%v: lost in the merge: %s", args, all)
		}
		if code == 0 {
			t.Errorf("%v: exit 0 without credentials — did it reach the network?", args)
		}
	}
}
