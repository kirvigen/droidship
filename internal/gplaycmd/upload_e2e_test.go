package gplaycmd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirvigen/droidship/internal/play"
)

// fakePlay is a minimal stand-in for the Play Developer API.
type fakePlay struct {
	t *testing.T

	commitQuery   string
	commits       int
	validates     int
	refuseReview  bool // answer the first commit the way Play does for some apps
	deletedEdits  []string
	trackReceived play.Track
	uploaded      bool

	// Served to the unified-verb adapter tests.
	tracks  []play.Track
	reviews string // raw JSON of a reviews list response
	listing string // raw JSON of a listing
	replied string
}

func (f *fakePlay) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/token":
			fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
		case r.Method == "POST" && strings.HasSuffix(p, "/edits"):
			fmt.Fprint(w, `{"id":"edit-1"}`)
		case r.Method == "POST" && strings.Contains(p, "/upload/") && strings.HasSuffix(p, "/bundles"):
			f.uploaded = true
			fmt.Fprint(w, `{"versionCode":14,"sha256":"abc123"}`)
		case r.Method == "GET" && strings.HasSuffix(p, "/tracks"):
			_ = json.NewEncoder(w).Encode(map[string]any{"tracks": f.tracks})
		case r.Method == "GET" && strings.Contains(p, "/tracks/"):
			name := p[strings.LastIndex(p, "/")+1:]
			for _, tr := range f.tracks {
				if tr.Track == name {
					_ = json.NewEncoder(w).Encode(tr)
					return
				}
			}
			_ = json.NewEncoder(w).Encode(play.Track{Track: name})
		case r.Method == "GET" && strings.HasSuffix(p, "/reviews"):
			fmt.Fprint(w, f.reviews)
		case r.Method == "POST" && strings.HasSuffix(p, ":reply"):
			var body struct {
				ReplyText string `json:"replyText"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.replied = body.ReplyText
			fmt.Fprint(w, `{"result":{"replyText":"ok"}}`)
		case r.Method == "GET" && strings.Contains(p, "/listings/"):
			fmt.Fprint(w, f.listing)
		case r.Method == "PUT" && strings.Contains(p, "/tracks/"):
			if err := json.NewDecoder(r.Body).Decode(&f.trackReceived); err != nil {
				f.t.Errorf("decode track: %v", err)
			}
			_ = json.NewEncoder(w).Encode(f.trackReceived)
		case strings.HasSuffix(p, ":validate"):
			f.validates++
			fmt.Fprint(w, `{"id":"edit-1"}`)
		case strings.HasSuffix(p, ":commit"):
			f.commits++
			f.commitQuery = r.URL.Query().Get("changesNotSentForReview")
			if f.refuseReview && f.commitQuery == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"code":400,"message":"Changes cannot be sent for review automatically. Please set the query parameter changesNotSentForReview to true.","status":"INVALID_ARGUMENT"}}`)
				return
			}
			fmt.Fprint(w, `{"id":"edit-1"}`)
		case r.Method == "DELETE" && strings.Contains(p, "/edits/"):
			f.deletedEdits = append(f.deletedEdits, p)
			w.WriteHeader(http.StatusNoContent)
		default:
			f.t.Errorf("unexpected %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// testClient wires a play.Client to the fake server.
func testClient(t *testing.T, f *fakePlay) *play.Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "proj",
		"client_email": "sa@proj.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	sa, err := play.ParseServiceAccount(raw)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c := play.NewWithServiceAccount(sa)
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"
	return c
}

func TestRunUploadCreatesADraftAndSaysWhatIsLeftToDo(t *testing.T) {
	f := &fakePlay{t: t}
	c := testClient(t, f)
	aab := writeTempFile(t, "app.aab", strings.Repeat("A", 1024))

	cfg, err := parseUploadArgs([]string{"com.gdebenz.win", "--aab", aab, "--whats-new", "Починили карту"})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := runUpload(context.Background(), c, cfg, &out); err != nil {
		t.Fatalf("runUpload: %v", err)
	}

	if !f.uploaded {
		t.Error("the bundle was never uploaded")
	}
	if len(f.trackReceived.Releases) != 1 {
		t.Fatalf("track = %+v", f.trackReceived)
	}
	rel := f.trackReceived.Releases[0]
	if rel.Status != play.StatusDraft {
		t.Errorf("status = %q, want draft", rel.Status)
	}
	if len(rel.VersionCodes) != 1 || rel.VersionCodes[0] != "14" {
		t.Errorf("versionCodes = %v, want [14]", rel.VersionCodes)
	}
	if len(rel.ReleaseNotes) != 1 || rel.ReleaseNotes[0].Text != "Починили карту" {
		t.Errorf("releaseNotes = %+v", rel.ReleaseNotes)
	}
	if f.commits != 1 {
		t.Errorf("commits = %d, want 1", f.commits)
	}
	if len(f.deletedEdits) != 0 {
		t.Errorf("a committed edit must not be deleted, got %v", f.deletedEdits)
	}
	if !strings.Contains(out.String(), "DRAFT") || !strings.Contains(out.String(), "Start rollout") {
		t.Errorf("the operator must be told the build is not live yet, got:\n%s", out.String())
	}
}

func TestRunUploadRetriesWhenPlayRefusesAutoReview(t *testing.T) {
	f := &fakePlay{t: t, refuseReview: true}
	c := testClient(t, f)
	aab := writeTempFile(t, "app.aab", "x")

	cfg, err := parseUploadArgs([]string{"com.example", "--aab", aab})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := runUpload(context.Background(), c, cfg, &out); err != nil {
		t.Fatalf("runUpload: %v", err)
	}
	if f.commits != 2 {
		t.Errorf("commits = %d, want 2 (one rejected, one retried)", f.commits)
	}
	if f.commitQuery != "true" {
		t.Errorf("the retry should set changesNotSentForReview, got %q", f.commitQuery)
	}
}

func TestRunUploadValidateOnlyChangesNothing(t *testing.T) {
	f := &fakePlay{t: t}
	c := testClient(t, f)
	aab := writeTempFile(t, "app.aab", "x")

	cfg, err := parseUploadArgs([]string{"com.example", "--aab", aab, "--validate-only"})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := runUpload(context.Background(), c, cfg, &out); err != nil {
		t.Fatalf("runUpload: %v", err)
	}
	if f.commits != 0 {
		t.Errorf("--validate-only must not commit, got %d commits", f.commits)
	}
	if f.validates != 1 {
		t.Errorf("--validate-only must call :validate, got %d calls", f.validates)
	}
	if len(f.deletedEdits) != 1 {
		t.Errorf("--validate-only must discard the edit, deleted = %v", f.deletedEdits)
	}
	if !strings.Contains(out.String(), "nothing was published") {
		t.Errorf("output should say nothing happened, got:\n%s", out.String())
	}
}

func TestRunUploadDiscardsTheEditWhenTheTrackUpdateFails(t *testing.T) {
	f := &fakePlay{t: t}
	c := testClient(t, f)
	// Make the track update fail by pointing at a track the fake rejects.
	srvHandler := f.handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/tracks/") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"code":403,"message":"does not have permission","status":"PERMISSION_DENIED"}}`)
			return
		}
		srvHandler(w, r)
	}))
	t.Cleanup(srv.Close)
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"

	aab := writeTempFile(t, "app.aab", "x")
	cfg, err := parseUploadArgs([]string{"com.example", "--aab", aab})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err = runUpload(context.Background(), c, cfg, &out)
	if err == nil {
		t.Fatal("expected the track update to fail")
	}
	if !strings.Contains(err.Error(), "Users and permissions") {
		t.Errorf("error should carry the hint, got: %v", err)
	}
	if len(f.deletedEdits) != 1 {
		t.Errorf("a failed run must discard its edit, deleted = %v", f.deletedEdits)
	}
	if !strings.Contains(out.String(), "nothing changed") {
		t.Errorf("output should reassure that nothing changed, got:\n%s", out.String())
	}
}
