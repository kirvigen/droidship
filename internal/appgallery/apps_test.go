package appgallery

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAppIDByPackage(t *testing.T) {
	var gotQuery string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"ret":{"code":0},"appids":[{"key":"appid","value":118236677}]}`))
	})
	id, err := c.AppIDByPackage(context.Background(), "com.gdebenz.win")
	if err != nil {
		t.Fatalf("AppIDByPackage: %v", err)
	}
	// The API has been seen answering with both a string and a number here.
	if id != "118236677" {
		t.Fatalf("app id = %q", id)
	}
	if !strings.Contains(gotQuery, "packageName=com.gdebenz.win") {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestAppIDByPackageWithoutMatch(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ret":{"code":0},"appids":[]}`))
	})
	if _, err := c.AppIDByPackage(context.Background(), "com.absent"); err == nil {
		t.Fatal("want an error when no app matches")
	}
}

func TestAppInfoParsesMixedTypes(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ret":{"code":0},"appInfo":{
			"releaseState":4,
			"versionNumber":"1.4.2",
			"versionCode":"14",
			"onShelfVersionNumber":"1.4.1",
			"updateTime":"2026-09-01 10:00:00",
			"defaultLang":"ru-RU",
			"unknownFutureField":"kept"
		}}`))
	})
	info, err := c.AppInfo(context.Background(), "118236677", ReleaseFull)
	if err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if info.VersionNumber != "1.4.2" || info.VersionCode.String() != "14" {
		t.Fatalf("version = %s (%s)", info.VersionNumber, info.VersionCode)
	}
	if info.StateLabel() != "reviewing" {
		t.Fatalf("state label = %q", info.StateLabel())
	}
	if !strings.Contains(string(info.Raw), "unknownFutureField") {
		t.Fatal("the raw payload must survive for --json")
	}
}

func TestAppInfoUnknownStateFallsBackToTheCode(t *testing.T) {
	info := AppInfo{ReleaseState: 42}
	if info.StateLabel() != "state 42" {
		t.Fatalf("state label = %q", info.StateLabel())
	}
}

func TestUpdateLanguageInfo(t *testing.T) {
	var body string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"ret":{"code":0}}`))
	})
	err := c.UpdateLanguageInfo(context.Background(), "118236677", LanguageInfo{Lang: "ru-RU", NewFeatures: "Починили карту"})
	if err != nil {
		t.Fatalf("UpdateLanguageInfo: %v", err)
	}
	if !strings.Contains(body, `"newFeatures":"Починили карту"`) || !strings.Contains(body, `"lang":"ru-RU"`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, "appName") {
		t.Fatalf("empty fields must be omitted: %s", body)
	}
}

func TestUpdateLanguageInfoRequiresALanguage(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected")
	})
	if err := c.UpdateLanguageInfo(context.Background(), "1", LanguageInfo{NewFeatures: "x"}); err == nil {
		t.Fatal("want an error without a language code")
	}
}
