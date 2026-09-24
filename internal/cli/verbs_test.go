package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirvigen/droidship/internal/store"
)

type fake struct {
	name      string
	reviews   []store.Review
	replied   string
	notesErr  error
	publish   store.PublishRequest
	published bool
	planErr   error
}

func (f *fake) Name() string { return f.name }
func (f *fake) Auth(context.Context) (store.AuthInfo, error) {
	return store.AuthInfo{Identity: f.name + "-id", Source: "test"}, nil
}
func (f *fake) Status(context.Context, string) ([]store.Release, error) {
	return []store.Release{{Track: "production", Version: "1.0 (10)", Status: "live"}}, nil
}
func (f *fake) Publish(_ context.Context, r store.PublishRequest, _ io.Writer) (store.PublishResult, error) {
	f.publish, f.published = r, true
	if r.GoLive {
		return store.PublishResult{State: "live"}, nil
	}
	return store.PublishResult{State: "staged", Next: "droidship release " + r.Package + " --store " + f.name}, nil
}
func (f *fake) Plan(_ context.Context, r store.PublishRequest) (store.PublishPlan, error) {
	if f.planErr != nil {
		return store.PublishPlan{}, f.planErr
	}
	return store.PublishPlan{Live: "1.0 (10)", Action: "upload " + r.AAB + " to " + f.name, Note: f.name + " note"}, nil
}
func (f *fake) Release(context.Context, store.ReleaseRequest, io.Writer) error { return nil }
func (f *fake) Rollout(context.Context, store.RolloutRequest, io.Writer) error { return nil }
func (f *fake) Notes(context.Context, string, string, string) error            { return f.notesErr }
func (f *fake) Reviews(context.Context, store.ReviewsQuery) ([]store.Review, error) {
	return f.reviews, nil
}
func (f *fake) Reply(_ context.Context, _, id, text string) error {
	f.replied = id + ":" + text
	return nil
}
func (f *fake) Listing(context.Context, string, string) (store.Listing, error) {
	return store.Listing{Lang: "ru-RU", Title: f.name}, nil
}
func (f *fake) ReplyLimit() int { return 10 }

// withFakes swaps the store factory for the duration of a test.
func withFakes(t *testing.T, stores ...*fake) {
	t.Helper()
	origOpen, origConfigured := openStore, isConfigured
	byName := map[string]*fake{}
	for _, s := range stores {
		byName[s.name] = s
	}
	openStore = func(name string) (store.Store, error) {
		if s, ok := byName[name]; ok {
			return s, nil
		}
		return nil, errors.New("not configured")
	}
	isConfigured = func(name string) bool { _, ok := byName[name]; return ok }
	t.Cleanup(func() { openStore, isConfigured = origOpen, origConfigured })
}

func TestReadVerbDefaultsToConfiguredStores(t *testing.T) {
	withFakes(t, &fake{name: "gplay"}, &fake{name: "rustore"})
	code, out, errOut := run(t, "status", "com.x")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "gplay") || !strings.Contains(out, "rustore") || strings.Contains(out, "appgallery") {
		t.Fatalf("configured stores only, got:\n%s", out)
	}
}

func TestExplicitAllSkipsUnconfiguredWithANote(t *testing.T) {
	withFakes(t, &fake{name: "gplay"})
	code, _, errOut := run(t, "auth", "--store", "all")
	if code != 0 || !strings.Contains(errOut, "appgallery: not configured, skipped") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestNothingConfigured(t *testing.T) {
	withFakes(t)
	if code, _, errOut := run(t, "auth"); code != 1 || !strings.Contains(errOut, "no store is configured") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestUnknownStoreIsAUsageError(t *testing.T) {
	withFakes(t, &fake{name: "gplay"})
	if code, _, _ := run(t, "status", "com.x", "--store", "appstore"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestWriteVerbNeedsAnExplicitStore(t *testing.T) {
	withFakes(t, &fake{name: "gplay"})
	code, _, errOut := run(t, "publish", "com.x", "--aab", "a.aab")
	if code != 2 || !strings.Contains(errOut, "--store is required") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestPublishStagesByDefault(t *testing.T) {
	g := &fake{name: "gplay"}
	withFakes(t, g)
	code, out, _ := run(t, "publish", "com.x", "--store", "gplay", "--aab", tempFile(t, "a.aab"), "--whats-new", "fixes", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var res []map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if res[0]["store"] != "gplay" || res[0]["state"] != "staged" {
		t.Fatalf("got %v", res)
	}
	if g.publish.GoLive || g.publish.Notes != "fixes" || g.publish.Lang != "ru-RU" {
		t.Fatalf("request %+v", g.publish)
	}
}

func TestPublishDryRunPlansWithoutPublishing(t *testing.T) {
	g, r := &fake{name: "gplay"}, &fake{name: "rustore"}
	withFakes(t, g, r)
	aab := tempFile(t, "app.aab")
	code, out, errOut := run(t, "publish", "com.x", "--store", "all", "--aab", aab, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if g.published || r.published {
		t.Fatal("--dry-run must not publish")
	}
	for _, want := range []string{"LIVE NOW", "WOULD DO", "upload " + aab + " to gplay", "rustore note", "dry run"} {
		if !strings.Contains(out+errOut, want) {
			t.Fatalf("missing %q in:\n%s%s", want, out, errOut)
		}
	}
}

func TestPublishDryRunReportsAStoreThatWouldFail(t *testing.T) {
	withFakes(t, &fake{name: "gplay", planErr: errors.New("Google Play accepts only an Android App Bundle")}, &fake{name: "rustore"})
	code, out, errOut := run(t, "publish", "com.x", "--store", "gplay,rustore", "--apk", tempFile(t, "app.apk"), "--dry-run", "--json")
	if code != 1 || !strings.Contains(errOut, "gplay: Google Play accepts only") || !strings.Contains(out, `"store": "rustore"`) {
		t.Fatalf("exit %d\nout %s\nerr %s", code, out, errOut)
	}
}

func TestPublishChecksTheFileExists(t *testing.T) {
	g := &fake{name: "gplay"}
	withFakes(t, g)
	code, _, errOut := run(t, "publish", "com.x", "--store", "gplay", "--aab", "/nope/app.aab")
	if code != 1 || !strings.Contains(errOut, "/nope/app.aab") || g.published {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func tempFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPublishNeedsExactlyOneFile(t *testing.T) {
	withFakes(t, &fake{name: "gplay"})
	if code, _, _ := run(t, "publish", "com.x", "--store", "gplay"); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestUnsupportedExitsThree(t *testing.T) {
	withFakes(t, &fake{name: "rustore", notesErr: store.Unsupported("rustore", "notes", "why")})
	code, out, errOut := run(t, "notes", "com.x", "--store", "rustore", "--text", "hi")
	if code != 3 || !strings.Contains(errOut, "unsupported by rustore") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if out != "" {
		t.Fatalf("no table when every store failed, got %q", out)
	}
}

func TestErrorBeatsUnsupported(t *testing.T) {
	withFakes(t,
		&fake{name: "gplay", notesErr: errors.New("boom")},
		&fake{name: "rustore", notesErr: store.Unsupported("rustore", "notes", "why")})
	code, _, errOut := run(t, "notes", "com.x", "--store", "gplay,rustore", "--text", "hi")
	if code != 1 || !strings.Contains(errOut, "gplay: boom") {
		t.Fatalf("exit %d, want 1; stderr %q", code, errOut)
	}
}

func TestReplyNeedsOneStoreAndChecksTheLimit(t *testing.T) {
	g := &fake{name: "gplay"}
	withFakes(t, g, &fake{name: "rustore"})
	if code, _, _ := run(t, "reply", "com.x", "r1", "--store", "all", "--text", "hi"); code != 2 {
		t.Fatalf("reply to all: exit %d, want 2", code)
	}
	if code, _, _ := run(t, "reply", "com.x", "r1", "--store", "gplay", "--text", "this is far too long"); code != 1 {
		t.Fatalf("over the limit: exit %d, want 1", code)
	}
	if code, _, _ := run(t, "reply", "com.x", "r1", "--store", "gplay", "--text", "thanks"); code != 0 || g.replied != "r1:thanks" {
		t.Fatalf("reply: exit %d, replied %q", code, g.replied)
	}
}

func TestReviewsMergeAcrossStores(t *testing.T) {
	withFakes(t,
		&fake{name: "gplay", reviews: []store.Review{{Store: "gplay", ID: "g1", Stars: 5, Text: "good"}}},
		&fake{name: "rustore", reviews: []store.Review{{Store: "rustore", ID: "r1", Stars: 1, Text: "bad"}}})
	code, out, _ := run(t, "reviews", "com.x", "--json")
	var got []store.Review
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got) != 2 {
		t.Fatalf("exit %d, out %s", code, out)
	}
}

func TestFlagsBeforePositionals(t *testing.T) {
	withFakes(t, &fake{name: "gplay"})
	if code, _, errOut := run(t, "status", "--store", "gplay", "com.x"); code != 0 {
		t.Fatalf("flags before the package: exit %d, %s", code, errOut)
	}
}

func TestMissingPackageIsAUsageError(t *testing.T) {
	withFakes(t, &fake{name: "gplay"})
	if code, _, errOut := run(t, "status"); code != 2 || !strings.Contains(errOut, "<package>") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}
