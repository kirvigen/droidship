package appgallery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestUpdatePhasedReleasePercent(t *testing.T) {
	var got map[string]any
	var query string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/publish/v2/phased-release" || r.Method != http.MethodPut {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		query = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ret":{"code":0,"msg":"success"}}`))
	})
	err := c.UpdatePhasedRelease(context.Background(), "42", PhasedUpdate{Percent: 50, EndTime: "2026-09-30T10:00:00+0300"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "appId=42&releaseType=3" {
		t.Fatalf("query %q", query)
	}
	// The start time of a running release cannot change, so it is never sent.
	want := map[string]any{"state": "RELEASE", "phasedReleasePercent": "50.00", "phasedReleaseEndTime": "2026-09-30T10:00:00+0300"}
	if len(got) != len(want) {
		t.Fatalf("body %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("body %v, want %v", got, want)
		}
	}
}

func TestUpdatePhasedReleaseToFull(t *testing.T) {
	var got map[string]any
	var query string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ret":{"code":0,"msg":"success"}}`))
	})
	if err := c.UpdatePhasedRelease(context.Background(), "42", PhasedUpdate{Full: true}); err != nil {
		t.Fatal(err)
	}
	if query != "appId=42&releaseType=1" || len(got) != 0 {
		t.Fatalf("query %q body %v; a full release takes an empty body", query, got)
	}
}

func TestUpdatePhasedReleaseValidatesThePercent(t *testing.T) {
	c := New("cid", "secret")
	for _, p := range []float64{0, 100, -1} {
		if err := c.UpdatePhasedRelease(context.Background(), "42", PhasedUpdate{Percent: p}); err == nil {
			t.Errorf("%g%% should be rejected: use Full for 100%%", p)
		}
	}
}

func TestLanguages(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/publish/v2/app-info" || r.URL.Query().Get("appId") != "42" {
			t.Errorf("%s %s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"ret":{"code":0},"appInfo":{"versionNumber":"1.0"},
			"languages":[{"lang":"ru-RU","appName":"ГдеБЕНЗ","briefInfo":"Цены на топливо","appDesc":"…","newFeatures":"…"}]}`))
	})
	langs, err := c.Languages(context.Background(), "42")
	if err != nil || len(langs) != 1 || langs[0].AppName != "ГдеБЕНЗ" || langs[0].BriefInfo != "Цены на топливо" {
		t.Fatalf("%+v %v", langs, err)
	}
}
