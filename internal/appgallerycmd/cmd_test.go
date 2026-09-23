package appgallerycmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirvigen/droidship/internal/appgallery"
)

func TestParsePublishArgsRequiresExactlyOnePackage(t *testing.T) {
	apk := writeFile(t, "app.apk", "x")
	cases := []struct {
		name string
		args []string
	}{
		{"nothing", nil},
		{"both", []string{"--apk", apk, "--aab", apk}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parsePublishArgs(tc.args); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestParsePublishArgsChecksTheFileExists(t *testing.T) {
	if _, _, err := parsePublishArgs([]string{"--aab", "/no/such/app.aab"}); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

func TestParsePublishArgsRejectsConflictingNotes(t *testing.T) {
	apk := writeFile(t, "app.apk", "x")
	notes := writeFile(t, "notes.txt", "hi")
	_, _, err := parsePublishArgs([]string{"--apk", apk, "--whats-new", "hi", "--whats-new-file", notes})
	if err == nil {
		t.Fatal("want an error when both note flags are given")
	}
}

func TestParsePublishArgsKeepsSharedFlags(t *testing.T) {
	apk := writeFile(t, "app.apk", "x")
	cfg, g, err := parsePublishArgs([]string{"--apk", apk, "--app-id", "118236677", "--region", "ru", "--upload-only"})
	if err != nil {
		t.Fatalf("parsePublishArgs: %v", err)
	}
	if g.AppID != "118236677" || g.Region != "ru" {
		t.Fatalf("shared flags = %+v", g)
	}
	if !cfg.UploadOnly || cfg.Lang != "ru-RU" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestReleaseOptsParams(t *testing.T) {
	t.Run("full release", func(t *testing.T) {
		p, err := (&releaseOpts{Remark: "Обновили карту и поиск"}).params()
		if err != nil {
			t.Fatalf("params: %v", err)
		}
		if p.Phased != nil || p.Remark == "" {
			t.Fatalf("params = %+v", p)
		}
	})

	t.Run("phased", func(t *testing.T) {
		p, err := (&releaseOpts{
			Phased:     10,
			PhasedFrom: "2026-09-05T10:00:00+0300",
			PhasedTo:   "2026-09-12T10:00:00+0300",
		}).params()
		if err != nil {
			t.Fatalf("params: %v", err)
		}
		if p.Phased == nil || p.Phased.Percent != "10.00" {
			t.Fatalf("phased = %+v", p.Phased)
		}
		if p.Phased.Description == "" {
			t.Fatal("a phased rollout needs a description for AppGallery")
		}
	})

	t.Run("rejects", func(t *testing.T) {
		cases := map[string]releaseOpts{
			"short remark":     {Remark: "коротко"},
			"percent only":     {Phased: 10},
			"window without %": {PhasedFrom: "a", PhasedTo: "b"},
			"percent too big":  {Phased: 101, PhasedFrom: "a", PhasedTo: "b"},
			"time and phased":  {Phased: 10, PhasedFrom: "a", PhasedTo: "b", ReleaseTime: "2026-09-05T10:00:00+0300"},
		}
		for name, opts := range cases {
			if _, err := opts.params(); err == nil {
				t.Fatalf("%s: want an error", name)
			}
		}
	})
}

func TestTimeWindow(t *testing.T) {
	begin, end, err := timeWindow("", "", 7)
	if err != nil {
		t.Fatalf("timeWindow: %v", err)
	}
	if d := end.Sub(begin); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Fatalf("default window = %s", d)
	}

	begin, end, err = timeWindow("2026-08-01", "2026-08-10", 7)
	if err != nil {
		t.Fatalf("timeWindow: %v", err)
	}
	if begin.Format("2006-01-02") != "2026-08-01" || end.Format("2006-01-02") != "2026-08-10" {
		t.Fatalf("window = %s .. %s", begin, end)
	}

	if _, _, err := timeWindow("2026-08-10", "2026-08-01", 7); err == nil {
		t.Fatal("want an error for a backwards window")
	}
	if _, _, err := timeWindow("2025-01-01", "2026-08-01", 7); err == nil {
		t.Fatal("want an error for a window over six months")
	}
	if _, _, err := timeWindow("вчера", "", 7); err == nil {
		t.Fatal("want an error for an unreadable date")
	}
}

func TestParseTimeAcceptsSeveralLayouts(t *testing.T) {
	for _, value := range []string{"2026-08-01", "2026-08-01 10:00", "2026-08-01 10:00:00", "2026-08-01T10:00:00+03:00"} {
		if _, err := parseTime(value); err != nil {
			t.Fatalf("parseTime(%q): %v", value, err)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{12: "12 B", 2048: "2.0 KB", 44 * 1024 * 1024: "44.0 MB"}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("две\n  строки", 40); got != "две строки" {
		t.Fatalf("oneLine = %q", got)
	}
	if got := oneLine(strings.Repeat("а", 50), 10); len([]rune(got)) != 10 {
		t.Fatalf("oneLine did not clip: %q", got)
	}
}

// isolateCredentials points the credential lookup at an empty temporary
// directory and clears the environment, so tests never read the real machine.
func isolateCredentials(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DROIDSHIP_CONFIG", "")
	t.Setenv("HSTORE_CONFIG_DIR", dir)
	t.Setenv("HSTORE_CREDENTIALS", "")
	for _, prefix := range []string{"DROIDSHIP_APPGALLERY_", "HSTORE_", "HUAWEI_"} {
		for _, name := range []string{"CLIENT_ID", "CLIENT_SECRET", "APP_ID", "REGION", "PACKAGE"} {
			t.Setenv(prefix+name, "")
		}
	}
	return dir
}

func TestNewSessionNeedsCredentials(t *testing.T) {
	isolateCredentials(t)
	_, err := newSession(&globalOpts{})
	if err == nil {
		t.Fatal("want an error without credentials")
	}
	if !strings.Contains(err.Error(), "credentials.json") {
		t.Fatalf("the error should say what to create: %v", err)
	}
}

func TestCredentialsFromEitherEnvPrefix(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("HUAWEI_CLIENT_ID", "from-huawei")
	t.Setenv("HUAWEI_CLIENT_SECRET", "secret")

	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds.ClientID != "from-huawei" {
		t.Fatalf("client id = %q", creds.ClientID)
	}

	// The tool's own prefix wins when both are set.
	t.Setenv("HSTORE_CLIENT_ID", "from-hstore")
	creds, err = loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds.ClientID != "from-hstore" {
		t.Fatalf("client id = %q", creds.ClientID)
	}
}

func TestCredentialsFromFile(t *testing.T) {
	dir := isolateCredentials(t)
	writeAt(t, filepath.Join(dir, "credentials.json"),
		`{"client_id":"cid","client_secret":"secret","app_id":"118236677","region":"ru"}`)

	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds.ClientID != "cid" || creds.AppID != "118236677" {
		t.Fatalf("credentials = %+v", creds)
	}

	s, err := newSession(&globalOpts{})
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	if s.client.BaseURL != appgallery.DomainRussia {
		t.Fatalf("region from the file was ignored: %s", s.client.BaseURL)
	}
}

func TestCredentialsFileIsFoundByExtension(t *testing.T) {
	dir := isolateCredentials(t)
	writeAt(t, filepath.Join(dir, "gdebenz-agc.json"), `{"client_id":"cid","client_secret":"secret"}`)
	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds.ClientID != "cid" {
		t.Fatalf("credentials = %+v", creds)
	}

	// Two candidates are ambiguous and must be reported instead of guessed.
	writeAt(t, filepath.Join(dir, "second.json"), `{"client_id":"other","client_secret":"secret"}`)
	if _, err := loadCredentials(); err == nil {
		t.Fatal("want an error when several files could be meant")
	}
}

func TestCredentialsRejectThePrivateKeyFile(t *testing.T) {
	dir := isolateCredentials(t)
	// This is the file AppGallery Connect hands out for the other auth scheme.
	writeAt(t, filepath.Join(dir, "credentials.json"),
		`{"key_id":"fdb5","private_key":"-----BEGIN PRIVATE KEY-----","sub_account":"118836173"}`)

	_, err := loadCredentials()
	if err == nil {
		t.Fatal("want an error for a private key file")
	}
	if !strings.Contains(err.Error(), "private key file") {
		t.Fatalf("the error should name the problem: %v", err)
	}
}

func TestSessionAppIDPrefersTheFlag(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("HSTORE_CLIENT_ID", "cid")
	t.Setenv("HSTORE_CLIENT_SECRET", "secret")
	t.Setenv("HSTORE_APP_ID", "from-env")

	s, err := newSession(&globalOpts{AppID: "from-flag"})
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	got, err := s.appID(context.Background())
	if err != nil {
		t.Fatalf("appID: %v", err)
	}
	if got != "from-flag" {
		t.Fatalf("app id = %q", got)
	}
}

func TestSessionAppIDWithoutAnythingToGoOn(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("HSTORE_CLIENT_ID", "cid")
	t.Setenv("HSTORE_CLIENT_SECRET", "secret")

	s, err := newSession(&globalOpts{})
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	if _, err := s.appID(context.Background()); err == nil {
		t.Fatal("want an error when neither the app id nor the package is known")
	}
}

// fakeAppGallery is a stand-in for the Publishing API covering one release.
type fakeAppGallery struct {
	srv *httptest.Server

	aabChecks    int32
	submits      int32
	submitBefore int32 // this many submissions answer "still processing"

	uploaded    string
	attached    string
	notes       string
	submitQuery string
}

func newFakeAppGallery(t *testing.T) *fakeAppGallery {
	t.Helper()
	f := &fakeAppGallery{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		switch r.URL.Path {
		case "/api/oauth2/v1/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":172800}`))
		case "/api/publish/v2/upload-url/for-obs":
			_, _ = w.Write([]byte(`{"ret":{"code":0},"urlInfo":{"url":"http://` + r.Host + `/obs","objectId":"obs://pkg","method":"PUT","headers":{}}}`))
		case "/obs":
			f.uploaded = body.String()
			w.WriteHeader(http.StatusOK)
		case "/api/publish/v2/app-file-info":
			f.attached = body.String()
			_, _ = w.Write([]byte(`{"ret":{"code":0},"pkgVersion":["1040000300001"]}`))
		case "/api/publish/v2/aab/complile/status":
			n := atomic.AddInt32(&f.aabChecks, 1)
			status := appgallery.AABCompileSuccess
			if n == 1 {
				status = appgallery.AABCompileProcessing
			}
			_, _ = w.Write([]byte(`{"ret":{"code":0},"pkgStateList":[{"pkgId":"1040000300001","aabCompileStatus":` + strconv.Itoa(status) + `}]}`))
		case "/api/publish/v2/app-language-info":
			f.notes = body.String()
			_, _ = w.Write([]byte(`{"ret":{"code":0}}`))
		case "/api/publish/v2/app-submit":
			n := atomic.AddInt32(&f.submits, 1)
			f.submitQuery = r.URL.RawQuery
			if n <= f.submitBefore {
				_, _ = w.Write([]byte(`{"ret":{"code":204144660,"msg":"It may take 2-5 minutes"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"ret":{"code":0,"msg":"success"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAppGallery) client() *appgallery.Client {
	c := appgallery.New("cid", "secret")
	c.BaseURL = f.srv.URL
	c.HTTP = f.srv.Client()
	return c
}

func TestRunPublishAAB(t *testing.T) {
	noSleep(t)
	fake := newFakeAppGallery(t)
	fake.submitBefore = 1 // the first submission lands while the package is processing

	aab := writeFile(t, "app-release.aab", "bundle-bytes")
	cfg := publishConfig{
		AAB:      aab,
		WhatsNew: "Починили карту",
		Lang:     "ru-RU",
		Wait:     time.Minute,
		Release:  releaseOpts{Remark: "Обновили карту и ускорили поиск"},
	}

	var out bytes.Buffer
	if err := runPublish(context.Background(), fake.client(), "118236677", cfg, &out); err != nil {
		t.Fatalf("runPublish: %v", err)
	}

	if fake.uploaded != "bundle-bytes" {
		t.Fatalf("uploaded %q", fake.uploaded)
	}
	if !strings.Contains(fake.attached, `"fileDestUrl":"obs://pkg"`) {
		t.Fatalf("attached payload = %s", fake.attached)
	}
	if !strings.Contains(fake.notes, "Починили карту") {
		t.Fatalf("release notes = %s", fake.notes)
	}
	if fake.aabChecks < 2 {
		t.Fatalf("the compiling bundle was polled %d times, want at least 2", fake.aabChecks)
	}
	if fake.submits != 2 {
		t.Fatalf("submitted %d times, want 2 (one retry)", fake.submits)
	}
	if !strings.Contains(fake.submitQuery, "releaseType=1") {
		t.Fatalf("submit query = %s", fake.submitQuery)
	}
	if !strings.Contains(out.String(), "submitted for review") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRunPublishAPKUploadOnly(t *testing.T) {
	noSleep(t)
	fake := newFakeAppGallery(t)

	apk := writeFile(t, "app-release.apk", "apk-bytes")
	cfg := publishConfig{APK: apk, Lang: "ru-RU", UploadOnly: true, Wait: time.Minute}

	var out bytes.Buffer
	if err := runPublish(context.Background(), fake.client(), "118236677", cfg, &out); err != nil {
		t.Fatalf("runPublish: %v", err)
	}
	if fake.submits != 0 {
		t.Fatalf("--upload-only must not submit, got %d submissions", fake.submits)
	}
	if fake.aabChecks != 0 {
		t.Fatalf("an apk must not be polled for aab compilation")
	}
	if !strings.Contains(out.String(), "--upload-only") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestRunPublishPhasedUsesTheRolloutReleaseType(t *testing.T) {
	noSleep(t)
	fake := newFakeAppGallery(t)

	apk := writeFile(t, "app-release.apk", "apk-bytes")
	cfg := publishConfig{
		APK:  apk,
		Lang: "ru-RU",
		Wait: time.Minute,
		Release: releaseOpts{
			Phased:     10,
			PhasedFrom: "2026-09-05T10:00:00+0300",
			PhasedTo:   "2026-09-12T10:00:00+0300",
		},
	}
	var out bytes.Buffer
	if err := runPublish(context.Background(), fake.client(), "118236677", cfg, &out); err != nil {
		t.Fatalf("runPublish: %v", err)
	}
	if !strings.Contains(fake.attached, `"fileType":5`) {
		t.Fatalf("attached payload = %s", fake.attached)
	}
	if !strings.Contains(fake.submitQuery, "releaseType=3") {
		t.Fatalf("submit query = %s", fake.submitQuery)
	}
	if !strings.Contains(out.String(), "10.00%") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestSubmitStopsRetryingAfterTheDeadline(t *testing.T) {
	noSleep(t)
	fake := newFakeAppGallery(t)
	fake.submitBefore = 100 // never finishes processing

	var out bytes.Buffer
	err := submitWithRetry(context.Background(), fake.client(), "118236677", appgallery.SubmitParams{}, -time.Second, &out)
	if err == nil {
		t.Fatal("want an error once the deadline passes")
	}
	if !strings.Contains(err.Error(), "droidship appgallery submit") {
		t.Fatalf("the error should say how to finish later: %v", err)
	}
}

func TestWaitForAABFailsOnABrokenBundle(t *testing.T) {
	noSleep(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth2/v1/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":172800}`))
			return
		}
		_, _ = w.Write([]byte(`{"ret":{"code":0},"pkgStateList":[{"pkgId":"1","aabCompileStatus":3}]}`))
	}))
	defer srv.Close()

	c := appgallery.New("cid", "secret")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()

	var out bytes.Buffer
	if err := waitForAAB(context.Background(), c, "1", []string{"1"}, time.Minute, &out); err == nil {
		t.Fatal("want an error when the bundle fails to compile")
	}
}

func TestRunRejectsUnknownCommands(t *testing.T) {
	if code := Run([]string{"deploy"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if code := Run(nil); code != 2 {
		t.Fatalf("exit code without arguments = %d, want 2", code)
	}
	if code := Run([]string{"version"}); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
}

// noSleep makes the waiting loops return immediately.
func noSleep(t *testing.T) {
	t.Helper()
	original := sleep
	sleep = func(time.Duration) {}
	t.Cleanup(func() { sleep = original })
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeAt(t, path, content)
	return path
}

func writeAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
