package appgallerycmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kirvigen/droidship/internal/appgallery"
	"github.com/kirvigen/droidship/internal/config"
	"github.com/kirvigen/droidship/internal/store"
)

func TestPhasedWindow(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	r := phasedOpts(25, now)
	if r.Phased != 25 || r.PhasedFrom != "2026-09-23T10:00:00+0300" || r.PhasedTo != "2026-09-30T10:00:00+0300" {
		t.Fatalf("%+v", r)
	}
	if _, err := r.params(); err != nil {
		t.Fatalf("the window must pass the legacy validation: %v", err)
	}
	for _, p := range []float64{0, 100} {
		if full := phasedOpts(p, now); full.Phased != 0 || full.PhasedFrom != "" {
			t.Fatalf("%g%% means a full release: %+v", p, full)
		}
	}
}

func TestLangForReply(t *testing.T) {
	if agLang("ru-RU") != "ru_RU" {
		t.Fatal("AppGallery replies take ru_RU")
	}
}

func TestAppIDFromConfigOnlyForItsPackage(t *testing.T) {
	s := &Store{creds: config.AppGalleryCreds{AppID: "42", Package: "com.a"}, ids: map[string]string{"com.b": "7"}}
	if id, err := s.appID(context.Background(), "com.a"); err != nil || id != "42" {
		t.Fatalf("configured package: %q %v", id, err)
	}
	if id, err := s.appID(context.Background(), "com.b"); err != nil || id != "7" {
		t.Fatalf("another package must be looked up, not given the configured id: %q %v", id, err)
	}
	s = &Store{creds: config.AppGalleryCreds{AppID: "42"}, ids: map[string]string{}}
	if id, _ := s.appID(context.Background(), "com.anything"); id != "42" {
		t.Fatalf("an app id without a package applies to any package, as in hstore: %q", id)
	}
}

func TestPlanFromInfo(t *testing.T) {
	info := appgallery.AppInfo{VersionNumber: "1.4.16", VersionCode: "29", OnShelfVersionNumber: "1.4.11", OnShelfVersionCode: "23", ReleaseState: 5}
	p := planFromInfo(info, store.PublishRequest{AAB: "/b/app.aab"})
	if p.Live != "1.4.11 (23)" || !strings.Contains(p.Action, "app.aab → attach, not submitted") || !strings.Contains(p.Note, "1.4.16 (29) is "+info.StateLabel()) {
		t.Fatalf("%+v", p)
	}
	p = planFromInfo(appgallery.AppInfo{}, store.PublishRequest{APK: "app.apk", GoLive: true, Percent: 20})
	if p.Live != "nothing on shelf" || !strings.Contains(p.Action, "20% over 7 days") || p.Note != "" {
		t.Fatalf("%+v", p)
	}
}
