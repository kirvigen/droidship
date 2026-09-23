package rustorecmd

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kirvigen/droidship/internal/rustore"
)

func TestParsePublishArgs(t *testing.T) {
	apk := writeTemp(t, "a.apk")
	aab := writeTemp(t, "b.aab")

	t.Run("happy path", func(t *testing.T) {
		cfg, err := parsePublishArgs([]string{"com.gdebenz.win", "--apk", apk,
			"--whats-new", "Фиксы", "--publish-type", "MANUAL", "--partial", "25", "--priority", "3"})
		if err != nil {
			t.Fatalf("parsePublishArgs: %v", err)
		}
		if cfg.Package != "com.gdebenz.win" || cfg.APK != apk || cfg.WhatsNew != "Фиксы" ||
			cfg.PublishType != "MANUAL" || cfg.Partial != 25 || cfg.Priority != 3 {
			t.Errorf("cfg = %+v", cfg)
		}
	})

	errCases := []struct {
		name string
		args []string
		want string
	}{
		{"no package", []string{"--apk", apk}, "package"},
		{"no file", []string{"com.x"}, "--apk or --aab"},
		{"both files", []string{"com.x", "--apk", apk, "--aab", aab}, "--apk or --aab"},
		{"hms without main", []string{"com.x", "--hms-apk", apk}, "--hms-apk"},
		{"bad partial", []string{"com.x", "--apk", apk, "--partial", "33"}, "partial"},
		{"bad priority", []string{"com.x", "--apk", apk, "--priority", "9"}, "priority"},
		{"bad type", []string{"com.x", "--apk", apk, "--publish-type", "SOON"}, "publish-type"},
		{"date without delayed", []string{"com.x", "--apk", apk, "--publish-date", "2026-09-01T10:00:00+03:00"}, "DELAYED"},
		{"delayed without date", []string{"com.x", "--apk", apk, "--publish-type", "DELAYED"}, "--publish-date"},
		{"missing apk file", []string{"com.x", "--apk", "/no/such.apk"}, "no/such.apk"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePublishArgs(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestResolveWhatsNew(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(f, []byte("Из файла\n"), 0o644)

	if got, _ := resolveWhatsNew(publishConfig{WhatsNew: "прямо"}); got != "прямо" {
		t.Errorf("inline whats-new = %q", got)
	}
	if got, _ := resolveWhatsNew(publishConfig{WhatsNewFile: f}); got != "Из файла" {
		t.Errorf("file whats-new = %q (want trimmed)", got)
	}
	if _, err := resolveWhatsNew(publishConfig{WhatsNewFile: "/no/such"}); err == nil {
		t.Error("want error for missing whats-new file")
	}
}

// fakeStore is a minimal stateful RuStore for orchestration tests.
type fakeStore struct {
	mu       sync.Mutex
	calls    []string
	draftID  int64
	deleted  []int64
	hasDraft bool
}

func (f *fakeStore) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		f.calls = append(f.calls, key)
		switch {
		case key == "POST /public/auth":
			fmt.Fprint(w, `{"code":"OK","body":{"jwe":"tok","ttl":900},"timestamp":"x"}`)
		case key == "POST /public/v1/application/com.x/version":
			if f.hasDraft {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"code":"ERROR","message":"You already have draft version with ID = %d","body":null,"timestamp":"x"}`, f.draftID)
				return
			}
			f.hasDraft = true
			f.draftID++
			fmt.Fprintf(w, `{"code":"OK","body":%d,"timestamp":"x"}`, f.draftID)
		case strings.HasPrefix(key, "DELETE /public/v1/application/com.x/version/"):
			f.hasDraft = false
			var id int64
			fmt.Sscanf(r.URL.Path, "/public/v1/application/com.x/version/%d", &id)
			f.deleted = append(f.deleted, id)
			fmt.Fprint(w, `{"code":"OK","body":null,"timestamp":"x"}`)
		case strings.HasSuffix(key, "/apk") || strings.HasSuffix(key, "/aab"),
			strings.HasSuffix(key, "/commit"):
			fmt.Fprint(w, `{"code":"OK","body":null,"timestamp":"x"}`)
		default:
			t.Errorf("unexpected call %s", key)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"code":"ERROR","message":"nope","body":null,"timestamp":"x"}`)
		}
	}
}

func newFakeClient(t *testing.T, f *fakeStore) *rustore.Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	c := rustore.NewWithKey("42", key)
	c.BaseURL = srv.URL
	return c
}

func writeTemp(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *fakeStore) apiCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if c != "POST /public/auth" {
			out = append(out, c)
		}
	}
	return out
}

func TestRunPublishHappyPath(t *testing.T) {
	f := &fakeStore{}
	c := newFakeClient(t, f)
	var out bytes.Buffer

	err := runPublish(t.Context(), c, publishConfig{
		Package: "com.x", APK: writeTemp(t, "a.apk"), WhatsNew: "Фиксы",
	}, &out)
	if err != nil {
		t.Fatalf("runPublish: %v", err)
	}
	want := []string{
		"POST /public/v1/application/com.x/version",
		"POST /public/v1/application/com.x/version/1/apk",
		"POST /public/v1/application/com.x/version/1/commit",
	}
	if got := f.apiCalls(); !equalSlices(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "1") {
		t.Errorf("output should mention version id, got %q", out.String())
	}
}

func TestRunPublishSkipCommit(t *testing.T) {
	f := &fakeStore{}
	c := newFakeClient(t, f)
	var out bytes.Buffer

	err := runPublish(t.Context(), c, publishConfig{
		Package: "com.x", APK: writeTemp(t, "a.apk"), SkipCommit: true,
	}, &out)
	if err != nil {
		t.Fatalf("runPublish: %v", err)
	}
	for _, call := range f.apiCalls() {
		if strings.HasSuffix(call, "/commit") {
			t.Errorf("commit must not be called with SkipCommit, calls = %v", f.apiCalls())
		}
	}
}

func TestRunPublishReplacesExistingDraft(t *testing.T) {
	f := &fakeStore{hasDraft: true, draftID: 555}
	c := newFakeClient(t, f)
	var out bytes.Buffer

	err := runPublish(t.Context(), c, publishConfig{
		Package: "com.x", APK: writeTemp(t, "a.apk"), ReplaceDraft: true,
	}, &out)
	if err != nil {
		t.Fatalf("runPublish: %v", err)
	}
	if len(f.deleted) != 1 || f.deleted[0] != 555 {
		t.Errorf("deleted drafts = %v, want [555]", f.deleted)
	}
	// After the replace the new draft 556 must have been used.
	found := false
	for _, call := range f.apiCalls() {
		if call == "POST /public/v1/application/com.x/version/556/apk" {
			found = true
		}
	}
	if !found {
		t.Errorf("new draft not used, calls = %v", f.apiCalls())
	}
}

func TestRunPublishExistingDraftWithoutReplaceFails(t *testing.T) {
	f := &fakeStore{hasDraft: true, draftID: 555}
	c := newFakeClient(t, f)
	var out bytes.Buffer

	err := runPublish(t.Context(), c, publishConfig{
		Package: "com.x", APK: writeTemp(t, "a.apk"),
	}, &out)
	if err == nil {
		t.Fatal("want error when draft exists and ReplaceDraft is off")
	}
	if !strings.Contains(err.Error(), "555") || !strings.Contains(err.Error(), "--replace-draft") {
		t.Errorf("error should mention draft id and --replace-draft flag: %v", err)
	}
	if len(f.deleted) != 0 {
		t.Errorf("nothing must be deleted, deleted = %v", f.deleted)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
