package rustore

import (
	"fmt"
	"net/http"
	"testing"
)

func TestApps(t *testing.T) {
	key := testKey(t)
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/application" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"code":"OK","message":"OK","body":{"content":[
			{"appId":478564,"packageName":"com.gdebenz.win","appName":"ГдеБЕНЗ","iconUrl":"u",
			 "appStatus":"PUBLISHED","versionName":"1.4.0","versionCode":13,
			 "companyName":"c","appVerUpdatedAt":"2026-08-22T12:23:55.596998+03:00","paid":false}
		],"continuationToken":null},"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	apps, err := c.Apps(t.Context())
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("len(apps) = %d, want 1", len(apps))
	}
	a := apps[0]
	if a.AppID != 478564 || a.PackageName != "com.gdebenz.win" || a.AppName != "ГдеБЕНЗ" ||
		a.AppStatus != "PUBLISHED" || a.VersionName != "1.4.0" || a.VersionCode != 13 {
		t.Errorf("app parsed wrong: %+v", a)
	}
}
