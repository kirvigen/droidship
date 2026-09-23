package play

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testKeyJSON builds a service-account key file around a fresh RSA key.
func testKeyJSON(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	raw, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "proj-1",
		"client_email":   "sa@proj-1.iam.gserviceaccount.com",
		"client_id":      "12345",
		"private_key_id": "abc",
		"private_key":    pemKey,
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw, key
}

func TestParseServiceAccount(t *testing.T) {
	raw, _ := testKeyJSON(t)
	sa, err := ParseServiceAccount(raw)
	if err != nil {
		t.Fatalf("ParseServiceAccount: %v", err)
	}
	if sa.ClientEmail != "sa@proj-1.iam.gserviceaccount.com" {
		t.Errorf("ClientEmail = %q", sa.ClientEmail)
	}
	if sa.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q", sa.ProjectID)
	}
	if sa.key == nil {
		t.Error("private key was not parsed")
	}
}

func TestParseServiceAccountRejectsOAuthClientSecret(t *testing.T) {
	// The single most common mistake: downloading an OAuth client instead of
	// a service account key.
	raw := []byte(`{"installed":{"client_id":"x","client_secret":"y"}}`)
	_, err := ParseServiceAccount(raw)
	if err == nil {
		t.Fatal("expected an error for an OAuth client secret")
	}
	if !strings.Contains(err.Error(), "service_account") {
		t.Errorf("error should name the expected type, got: %v", err)
	}
}

func TestParseServiceAccountRejectsBrokenPEM(t *testing.T) {
	raw, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": "sa@x.iam.gserviceaccount.com",
		"private_key":  "not a pem block",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseServiceAccount(raw); err == nil {
		t.Fatal("expected an error for a non-PEM private key")
	}
}

func TestAssertionIsSignedAndScoped(t *testing.T) {
	raw, key := testKeyJSON(t)
	sa, err := ParseServiceAccount(raw)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	assertion, err := sa.assertion(now)
	if err != nil {
		t.Fatalf("assertion: %v", err)
	}

	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion has %d parts, want 3", len(parts))
	}

	var claims struct {
		Iss   string `json:"iss"`
		Scope string `json:"scope"`
		Aud   string `json:"aud"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if err := json.Unmarshal(body, &claims); err != nil {
		t.Fatalf("claims json: %v", err)
	}
	if claims.Scope != Scope {
		t.Errorf("scope = %q, want %q", claims.Scope, Scope)
	}
	if claims.Iss != sa.ClientEmail {
		t.Errorf("iss = %q, want %q", claims.Iss, sa.ClientEmail)
	}
	if claims.Iat != now.Unix() || claims.Exp != now.Add(time.Hour).Unix() {
		t.Errorf("iat/exp = %d/%d, want %d/%d", claims.Iat, claims.Exp, now.Unix(), now.Add(time.Hour).Unix())
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Errorf("signature does not verify: %v", err)
	}
}

// newTestClient wires a Client to a test server for both token and API calls.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	raw, _ := testKeyJSON(t)
	sa, err := ParseServiceAccount(raw)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewWithServiceAccount(sa)
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"
	return c, srv
}

func TestTokenIsFetchedOnceAndCached(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		calls++
		fmt.Fprint(w, `{"access_token":"tok-1","expires_in":3600,"token_type":"Bearer"}`)
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		tok, err := c.Token(ctx)
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if tok != "tok-1" {
			t.Fatalf("token = %q", tok)
		}
	}
	if calls != 1 {
		t.Errorf("token endpoint hit %d times, want 1 (cached)", calls)
	}
}

func TestTokenErrorMentionsTheClock(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Invalid JWT: token expired"}`)
	})
	_, err := c.Token(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") || !strings.Contains(err.Error(), "clock") {
		t.Errorf("error should surface the OAuth reason and the clock hint, got: %v", err)
	}
}
