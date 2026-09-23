package rustore

import (
	"fmt"
	"net/http"
	"testing"
)

func TestVersions(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/application/com.gdebenz.win/version" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("page") != "0" || q.Get("size") != "5" {
			t.Errorf("query = %v, want page=0 size=5", q)
		}
		if q.Has("ids") {
			t.Errorf("ids must be absent when not requested, query = %v", q)
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":{"content":[
			{"versionId":704372,"appName":"ГдеБЕНЗ","appType":"MAIN","versionName":"1.4.0",
			 "versionCode":13,"versionStatus":"ACTIVE","publishType":"INSTANTLY",
			 "publishDateTime":"2026-08-22T12:34:43.925+00:00","sendDateForModer":"2026-08-21T12:03:06.303+00:00",
			 "partialValue":-1,"whatsNew":"Фиксы","priceValue":0,"paid":false}
		],"pageNumber":0,"pageSize":5,"totalElements":1,"totalPages":1},"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	page, err := c.Versions(t.Context(), "com.gdebenz.win", VersionsOpts{Page: 0, Size: 5})
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if page.TotalElements != 1 || len(page.Content) != 1 {
		t.Fatalf("page = %+v", page)
	}
	v := page.Content[0]
	if v.VersionID != 704372 || v.VersionStatus != "ACTIVE" || v.PartialValue != -1 ||
		v.VersionCode != 13 || v.PublishType != "INSTANTLY" || v.WhatsNew != "Фиксы" {
		t.Errorf("version parsed wrong: %+v", v)
	}
}

func TestVersionsByID(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ids"); got != "704372" {
			t.Errorf("ids = %q, want 704372", got)
		}
		if r.URL.Query().Has("page") || r.URL.Query().Has("size") {
			t.Errorf("page/size must be absent with ids, query = %v", r.URL.Query())
		}
		fmt.Fprint(w, `{"code":"OK","body":{"content":[],"pageNumber":0,"pageSize":20,"totalElements":0,"totalPages":0},"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	if _, err := c.Versions(t.Context(), "com.gdebenz.win", VersionsOpts{ID: 704372}); err != nil {
		t.Fatalf("Versions: %v", err)
	}
}
