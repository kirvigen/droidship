package play

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// routes serves the token endpoint plus a table of API responses.
func routes(t *testing.T, table map[string]func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
			return
		}
		key := r.Method + " " + r.URL.Path
		h, ok := table[key]
		if !ok {
			t.Errorf("unexpected request %s", key)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		h(w, r)
	})
	return c
}

func TestAPIErrorHints(t *testing.T) {
	cases := []struct {
		name   string
		status int
		msg    string
		want   string
	}{
		{"no app access", 401, "The current user has insufficient permissions to perform the requested operation.", "Users and permissions"},
		{"api disabled", 403, "Google Play Android Developer API has not been used in project 1 before or it is disabled.", "androidpublisher.googleapis.com"},
		{"unknown package", 404, "No application was found for the given package name.", "applicationId"},
		{"duplicate version", 400, "APK specifies a version code that has already been used.", "Bump versionCode"},
		{"edit in flight", 409, "Edit is not valid.", "Another edit"},
		{"nothing to say", 500, "Internal error.", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &APIError{HTTPStatus: tc.status, Message: tc.msg, Method: "GET", Path: "/x"}
			hint := e.Hint()
			if tc.want == "" {
				if hint != "" {
					t.Errorf("expected no hint, got %q", hint)
				}
				return
			}
			if !strings.Contains(hint, tc.want) {
				t.Errorf("hint %q does not mention %q", hint, tc.want)
			}
			if !strings.Contains(e.Error(), tc.want) {
				t.Error("Error() should include the hint")
			}
		})
	}
}

func TestNewAPIErrorParsesGoogleEnvelope(t *testing.T) {
	raw := []byte(`{"error":{"code":403,"message":"boom","status":"PERMISSION_DENIED"}}`)
	err := newAPIError(403, raw, "POST", "/p")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T, want *APIError", err)
	}
	if apiErr.Status != "PERMISSION_DENIED" || apiErr.Message != "boom" {
		t.Errorf("parsed = %+v", apiErr)
	}
}

func TestNewAPIErrorFallsBackToRawBody(t *testing.T) {
	err := newAPIError(502, []byte("<html>bad gateway</html>"), "GET", "/p")
	if !strings.Contains(err.Error(), "bad gateway") {
		t.Errorf("raw body should survive, got: %v", err)
	}
}

func TestCreateEditAndDelete(t *testing.T) {
	deleted := false
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /androidpublisher/v3/applications/com.example/edits": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"id":"edit-7","expiryTimeSeconds":"1700000000"}`)
		},
		"DELETE /androidpublisher/v3/applications/com.example/edits/edit-7": func(w http.ResponseWriter, r *http.Request) {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		},
	})
	ctx := context.Background()
	edit, err := c.CreateEdit(ctx, "com.example")
	if err != nil {
		t.Fatalf("CreateEdit: %v", err)
	}
	if edit.ID != "edit-7" {
		t.Fatalf("edit id = %q", edit.ID)
	}
	if err := c.DeleteEdit(ctx, "com.example", edit.ID); err != nil {
		t.Fatalf("DeleteEdit: %v", err)
	}
	if !deleted {
		t.Error("DeleteEdit did not reach the server")
	}
}

func TestCommitEditSendForReviewFlag(t *testing.T) {
	for _, tc := range []struct {
		send bool
		want string
	}{{true, ""}, {false, "true"}} {
		var got string
		c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
			"POST /androidpublisher/v3/applications/com.example/edits/e1:commit": func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query().Get("changesNotSentForReview")
				fmt.Fprint(w, `{}`)
			},
		})
		if err := c.CommitEdit(context.Background(), "com.example", "e1", tc.send); err != nil {
			t.Fatalf("CommitEdit: %v", err)
		}
		if got != tc.want {
			t.Errorf("sendForReview=%v → changesNotSentForReview=%q, want %q", tc.send, got, tc.want)
		}
	}
}

func TestUpdateTrackSendsTheWholeTrack(t *testing.T) {
	var body Track
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"PUT /androidpublisher/v3/applications/com.example/edits/e1/tracks/production": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(body)
		},
	})
	in := Track{Track: "production", Releases: []TrackRelease{{
		VersionCodes: []string{"14"},
		Status:       StatusDraft,
		ReleaseNotes: []ReleaseNote{{Language: "ru-RU", Text: "правки"}},
	}}}
	out, err := c.UpdateTrack(context.Background(), "com.example", "e1", in)
	if err != nil {
		t.Fatalf("UpdateTrack: %v", err)
	}
	if len(body.Releases) != 1 || body.Releases[0].Status != StatusDraft {
		t.Fatalf("server saw %+v", body)
	}
	if body.Releases[0].ReleaseNotes[0].Text != "правки" {
		t.Errorf("release notes lost in transit: %+v", body.Releases[0].ReleaseNotes)
	}
	if out.Track != "production" {
		t.Errorf("out.Track = %q", out.Track)
	}
	// A draft release must never carry a userFraction — Play rejects it.
	if body.Releases[0].UserFraction != 0 {
		t.Errorf("draft release leaked userFraction %v", body.Releases[0].UserFraction)
	}
}

func TestUploadBundleStreamsTheFile(t *testing.T) {
	dir := t.TempDir()
	aab := filepath.Join(dir, "app.aab")
	payload := strings.Repeat("A", 4096)
	if err := os.WriteFile(aab, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotBody []byte
	var gotType, gotUploadType string
	var gotLength int64
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /upload/androidpublisher/v3/applications/com.example/edits/e1/bundles": func(w http.ResponseWriter, r *http.Request) {
			gotType = r.Header.Get("Content-Type")
			gotUploadType = r.URL.Query().Get("uploadType")
			gotLength = r.ContentLength
			gotBody, _ = io.ReadAll(r.Body)
			fmt.Fprint(w, `{"versionCode":14,"sha256":"deadbeef"}`)
		},
	})
	var progress strings.Builder
	b, err := c.UploadBundle(context.Background(), "com.example", "e1", aab, &progress)
	if err != nil {
		t.Fatalf("UploadBundle: %v", err)
	}
	if b.VersionCode != 14 || b.SHA256 != "deadbeef" {
		t.Errorf("bundle = %+v", b)
	}
	if string(gotBody) != payload {
		t.Errorf("uploaded %d bytes, want %d", len(gotBody), len(payload))
	}
	if gotType != "application/octet-stream" {
		t.Errorf("Content-Type = %q", gotType)
	}
	if gotUploadType != "media" {
		t.Errorf("uploadType = %q", gotUploadType)
	}
	if gotLength != int64(len(payload)) {
		t.Errorf("Content-Length = %d, want %d", gotLength, len(payload))
	}
}

func TestUploadBundleSurfacesAPIError(t *testing.T) {
	dir := t.TempDir()
	aab := filepath.Join(dir, "app.aab")
	if err := os.WriteFile(aab, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /upload/androidpublisher/v3/applications/com.example/edits/e1/bundles": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"code":401,"message":"The current user has insufficient permissions to perform the requested operation.","status":"UNAUTHENTICATED"}}`)
		},
	})
	_, err := c.UploadBundle(context.Background(), "com.example", "e1", aab, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Users and permissions") {
		t.Errorf("the hint should tell the user what to do, got: %v", err)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 2048: "2.0 KiB", 23618781: "22.5 MiB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
