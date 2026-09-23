package appgallery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient wires a Client to a test server and pre-answers the token call.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth2/v1/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":172800}`))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c := New("cid", "secret")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	return c, srv
}

func TestTokenIsCachedUntilItNearlyExpires(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":600}`))
	}))
	defer srv.Close()

	now := time.Now()
	c := New("cid", "secret")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	c.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := c.Token(context.Background()); err != nil {
			t.Fatalf("Token: %v", err)
		}
	}
	if hits != 1 {
		t.Fatalf("token fetched %d times, want 1", hits)
	}

	// 600s ttl minus the 5 minute margin leaves 5 minutes of cache.
	now = now.Add(6 * time.Minute)
	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("Token after expiry: %v", err)
	}
	if hits != 2 {
		t.Fatalf("token fetched %d times after expiry, want 2", hits)
	}
}

func TestTokenErrorCarriesTheReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ret":"{\"code\":101003,\"msg\":\"invalid client_secret\"}"}`))
	}))
	defer srv.Close()

	c := New("cid", "bad")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()

	_, err := c.Token(context.Background())
	if err == nil {
		t.Fatal("want an error for a rejected client_secret")
	}
	if !strings.Contains(err.Error(), "invalid client_secret") {
		t.Fatalf("error %q does not mention the reason", err)
	}
}

func TestTokenErrorWithAnObjectRet(t *testing.T) {
	// The live API answers http 200 with the failure inside ret, as an object.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ret":{"code":203882498,"msg":"[AppGalleryConnectApiPermissionService]invalid client id"},"products":[]}`))
	}))
	defer srv.Close()

	c := New("wrong", "secret")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()

	_, err := c.Token(context.Background())
	if err == nil {
		t.Fatal("want an error for an unknown client id")
	}
	if !strings.Contains(err.Error(), "invalid client id") {
		t.Fatalf("error %q does not mention the reason", err)
	}
}

func TestRequestsCarryTheAuthHeaders(t *testing.T) {
	var gotAuth, gotClientID string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotClientID = r.Header.Get("client_id")
		_, _ = w.Write([]byte(`{"ret":{"code":0,"msg":"success"}}`))
	})

	if err := c.get(context.Background(), "/api/publish/v2/app-info", nil, nil); err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotClientID != "cid" {
		t.Fatalf("client_id = %q", gotClientID)
	}
}

func TestBothResultEnvelopesBecomeAPIErrors(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		code    int
		message string
	}{
		{"publishing", `{"ret":{"code":204144660,"msg":"It may take 2-5 minutes"}}`, 204144660, "It may take 2-5 minutes"},
		{"reviews", `{"ret":{"rtnCode":401,"rtnDesc":"no permission"}}`, 401, "no permission"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})
			err := c.get(context.Background(), "/api/publish/v2/app-info", nil, nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error %v is not an *APIError", err)
			}
			if apiErr.Code != tc.code || apiErr.Message != tc.message {
				t.Fatalf("got code %d msg %q", apiErr.Code, apiErr.Message)
			}
		})
	}
}

func TestSuccessEnvelopeIsNotAnError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ret":{"code":0,"msg":"success"},"appids":[{"key":"appid","value":"118236677"}]}`))
	})
	if _, err := c.AppIDByPackage(context.Background(), "com.gdebenz.win"); err != nil {
		t.Fatalf("AppIDByPackage: %v", err)
	}
}

func TestHTTPErrorWithoutEnvelopeIsReported(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{}`))
	})
	err := c.get(context.Background(), "/api/publish/v2/app-info", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("want an *APIError with http 403, got %v", err)
	}
}

func TestDomainForRegion(t *testing.T) {
	cases := map[string]string{
		"":                      DomainGlobal,
		"ru":                    DomainRussia,
		"RU":                    DomainRussia,
		"eu":                    DomainEurope,
		"sg":                    DomainAsia,
		"https://example.test/": "https://example.test",
	}
	for in, want := range cases {
		got, err := DomainForRegion(in)
		if err != nil {
			t.Fatalf("DomainForRegion(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("DomainForRegion(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := DomainForRegion("mars"); err == nil {
		t.Fatal("want an error for an unknown region")
	}
}

func TestIsProcessing(t *testing.T) {
	if !IsProcessing(&APIError{Code: 204144660}) {
		t.Fatal("the processing code must be recognised")
	}
	if !IsProcessing(&APIError{Code: 1, Message: "It may take 2-5 minutes"}) {
		t.Fatal("the processing message must be recognised")
	}
	if IsProcessing(&APIError{Code: 401, Message: "no permission"}) {
		t.Fatal("an unrelated error must not look like processing")
	}
	if IsProcessing(errors.New("boom")) {
		t.Fatal("a plain error must not look like processing")
	}
}
