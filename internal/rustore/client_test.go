package rustore

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newAuthServer returns an httptest server whose /public/auth validates the
// signature against key's public part and hands out sequential tokens, plus a
// counter of auth calls. extra handles every other path.
func newAuthServer(t *testing.T, key *rsa.PrivateKey, keyID string, ttl int, extra http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var authCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/public/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("auth method = %s, want POST", r.Method)
		}
		var req struct {
			KeyID     string `json:"keyId"`
			Timestamp string `json:"timestamp"`
			Signature string `json:"signature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("auth body decode: %v", err)
		}
		if req.KeyID != keyID {
			t.Errorf("auth keyId = %q, want %q", req.KeyID, keyID)
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000-07:00", req.Timestamp); err != nil {
			t.Errorf("auth timestamp %q not in expected format: %v", req.Timestamp, err)
		}
		sig, err := base64.StdEncoding.DecodeString(req.Signature)
		if err != nil {
			t.Errorf("auth signature not base64: %v", err)
		}
		digest := sha512.Sum512([]byte(req.KeyID + req.Timestamp))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA512, digest[:], sig); err != nil {
			t.Errorf("auth signature does not verify: %v", err)
		}
		n := authCalls.Add(1)
		fmt.Fprintf(w, `{"code":"OK","message":null,"body":{"jwe":"tok-%d","ttl":%d},"timestamp":"x"}`, n, ttl)
	})
	if extra != nil {
		mux.HandleFunc("/", extra)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &authCalls
}

func newTestClient(key *rsa.PrivateKey, keyID, baseURL string) *Client {
	c := NewWithKey(keyID, key)
	c.BaseURL = baseURL
	return c
}

func TestTokenFetchesAndCaches(t *testing.T) {
	key := testKey(t)
	srv, authCalls := newAuthServer(t, key, "42", 900, nil)
	c := newTestClient(key, "42", srv.URL)

	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	tok, err := c.Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "tok-1" {
		t.Errorf("token = %q, want tok-1", tok)
	}
	if tok2, _ := c.Token(t.Context()); tok2 != "tok-1" {
		t.Errorf("second token = %q, want cached tok-1", tok2)
	}
	if got := authCalls.Load(); got != 1 {
		t.Errorf("auth calls = %d, want 1 (cached)", got)
	}

	// Past ttl minus the refresh margin the token must be re-fetched.
	now = now.Add(850 * time.Second)
	if tok3, _ := c.Token(t.Context()); tok3 != "tok-2" {
		t.Errorf("token after expiry = %q, want tok-2", tok3)
	}
	if got := authCalls.Load(); got != 2 {
		t.Errorf("auth calls after expiry = %d, want 2", got)
	}
}

func TestTokenAuthError(t *testing.T) {
	key := testKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/public/auth", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":"ERROR","message":"Signature encode error","body":null,"timestamp":"x"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := newTestClient(key, "42", srv.URL)

	_, err := c.Token(t.Context())
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T (%v), want *APIError", err, err)
	}
	if apiErr.HTTPStatus != 400 || apiErr.Message != "Signature encode error" {
		t.Errorf("APIError = %+v", apiErr)
	}
}

func TestDoSetsPublicTokenAndDecodesBody(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Public-Token"); got != "tok-1" {
			t.Errorf("Public-Token = %q, want tok-1", got)
		}
		if r.URL.Path != "/public/v1/thing" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "v" {
			t.Errorf("query q = %q, want v", got)
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":{"answer":41},"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	var out struct {
		Answer int `json:"answer"`
	}
	if err := c.get(t.Context(), "/public/v1/thing", map[string]string{"q": "v"}, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Answer != 41 {
		t.Errorf("answer = %d, want 41", out.Answer)
	}
}

func TestDoReturnsAPIErrorOnErrorCode(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"code":"ERROR","message":"Application not found","body":null,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	err := c.get(t.Context(), "/public/v1/thing", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T (%v), want *APIError", err, err)
	}
	if apiErr.HTTPStatus != 404 || apiErr.Code != "ERROR" || apiErr.Message != "Application not found" {
		t.Errorf("APIError = %+v", apiErr)
	}
}

func TestDoReturnsErrorOnNonJSON(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `<html>bad gateway</html>`)
	})
	c := newTestClient(key, "42", srv.URL)

	err := c.get(t.Context(), "/public/v1/thing", nil, nil)
	if err == nil {
		t.Fatal("want error for non-JSON 502 response")
	}
}
