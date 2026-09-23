package appgallery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSubmitFullRelease(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(`{"ret":{"code":0,"msg":"success"}}`))
	})
	err := c.Submit(context.Background(), "118236677", SubmitParams{
		Remark:      "Обновили карту и ускорили поиск",
		ReleaseTime: "2026-09-05T10:00:00+0300",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.URL.Path != "/api/publish/v2/app-submit" || got.Method != http.MethodPost {
		t.Fatalf("%s %s", got.Method, got.URL.Path)
	}
	q := got.URL.Query()
	if q.Get("releaseType") != "1" {
		t.Fatalf("releaseType = %q, want 1", q.Get("releaseType"))
	}
	if q.Get("remark") != "Обновили карту и ускорили поиск" {
		t.Fatalf("remark = %q", q.Get("remark"))
	}
	if q.Get("releaseTime") != "2026-09-05T10:00:00+0300" {
		t.Fatalf("releaseTime = %q", q.Get("releaseTime"))
	}
}

func TestSubmitPhasedRelease(t *testing.T) {
	var body PhasedRelease
	var releaseType string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		releaseType = r.URL.Query().Get("releaseType")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"ret":{"code":0}}`))
	})
	phased := &PhasedRelease{
		StartTime:   "2026-09-05T10:00:00+0300",
		EndTime:     "2026-09-12T10:00:00+0300",
		Percent:     "10.00",
		Description: "10% для начала",
	}
	if err := c.Submit(context.Background(), "118236677", SubmitParams{Phased: phased}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if releaseType != "3" {
		t.Fatalf("releaseType = %q, want 3", releaseType)
	}
	if body != *phased {
		t.Fatalf("phased body = %+v", body)
	}
}

func TestWithdraw(t *testing.T) {
	var path, appID string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		appID = r.URL.Query().Get("appId")
		_, _ = w.Write([]byte(`{"ret":{"code":0}}`))
	})
	if err := c.Withdraw(context.Background(), "118236677"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if path != "/api/publish/v1/app-info/withdraw" || appID != "118236677" {
		t.Fatalf("%s?appId=%s", path, appID)
	}
}

func TestValidateRemark(t *testing.T) {
	if err := ValidateRemark(""); err != nil {
		t.Fatalf("an empty note is allowed: %v", err)
	}
	if err := ValidateRemark("коротко"); err == nil {
		t.Fatal("want an error for a note under 10 characters")
	}
	if err := ValidateRemark(string(make([]rune, 301))); err == nil {
		t.Fatal("want an error for a note over 300 characters")
	}
	if err := ValidateRemark("Обновили карту, ускорили поиск"); err != nil {
		t.Fatalf("a normal note must pass: %v", err)
	}
}
