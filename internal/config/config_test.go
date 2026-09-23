package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clean gives the test an empty home and blanks every credential variable.
func clean(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	vars := []string{"DROIDSHIP_CONFIG", "DROIDSHIP_GPLAY_KEY", "GPLAY_SA_JSON",
		"DROIDSHIP_RUSTORE_KEY_ID", "DROIDSHIP_RUSTORE_PRIVATE_KEY", "RUSTORE_KEY_ID", "RUSTORE_API_KEY",
		"HSTORE_CREDENTIALS", "HSTORE_CONFIG_DIR"}
	for _, prefix := range appGalleryPrefixes {
		for _, name := range []string{"CLIENT_ID", "CLIENT_SECRET", "APP_ID", "REGION", "PACKAGE"} {
			vars = append(vars, prefix+name)
		}
	}
	for _, v := range vars {
		t.Setenv(v, "")
	}
	return home
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGPlayKeyOrder(t *testing.T) {
	home := clean(t)
	legacy := filepath.Join(home, ".config", "gplay", "key.json")
	write(t, legacy, "{}")
	if p, src, err := GPlayKey(""); err != nil || p != legacy || !strings.Contains(src, "gplay") {
		t.Fatalf("legacy dir: %q %q %v", p, src, err)
	}
	write(t, filepath.Join(home, ".config", "droidship", "config.json"), `{"gplay":{"key":"~/keys/play.json"}}`)
	if p, _, _ := GPlayKey(""); p != filepath.Join(home, "keys", "play.json") {
		t.Fatalf("config.json should beat the legacy dir and expand ~, got %q", p)
	}
	t.Setenv("GPLAY_SA_JSON", "/legacy/env.json")
	if p, _, _ := GPlayKey(""); p != "/legacy/env.json" {
		t.Fatalf("legacy env should beat config.json, got %q", p)
	}
	t.Setenv("DROIDSHIP_GPLAY_KEY", "/new/env.json")
	if p, src, _ := GPlayKey(""); p != "/new/env.json" || src != "env DROIDSHIP_GPLAY_KEY" {
		t.Fatalf("DROIDSHIP env should win, got %q from %q", p, src)
	}
	if p, src, _ := GPlayKey("/flag.json"); p != "/flag.json" || src != "flag --key" {
		t.Fatalf("flag should win, got %q from %q", p, src)
	}
}

func TestGPlayKeySeveralFilesIsAnError(t *testing.T) {
	home := clean(t)
	write(t, filepath.Join(home, ".config", "gplay", "a.json"), "{}")
	write(t, filepath.Join(home, ".config", "gplay", "b.json"), "{}")
	if _, _, err := GPlayKey(""); err == nil || !strings.Contains(err.Error(), "several keys") {
		t.Fatalf("want a 'several keys' error, got %v", err)
	}
}

func TestGPlayKeyMissing(t *testing.T) {
	clean(t)
	if _, _, err := GPlayKey(""); err == nil {
		t.Fatal("want an error without any key")
	}
}

func TestDroidshipConfigOverride(t *testing.T) {
	clean(t)
	path := filepath.Join(t.TempDir(), "elsewhere.json")
	write(t, path, `{"gplay":{"key":"/k.json"}}`)
	t.Setenv("DROIDSHIP_CONFIG", path)
	if p, _, err := GPlayKey(""); err != nil || p != "/k.json" {
		t.Fatalf("DROIDSHIP_CONFIG ignored: %q %v", p, err)
	}
}

func TestBrokenConfigIsReported(t *testing.T) {
	home := clean(t)
	write(t, filepath.Join(home, ".config", "droidship", "config.json"), `{not json`)
	if _, err := RuStore(); err == nil || !strings.Contains(err.Error(), "config.json") {
		t.Fatalf("want a parse error naming the file, got %v", err)
	}
}

func TestRuStoreOrder(t *testing.T) {
	home := clean(t)
	if _, err := RuStore(); err == nil {
		t.Fatal("want an error without credentials")
	}
	write(t, filepath.Join(home, ".config", "droidship", "config.json"),
		`{"rustore":{"key_id":"1","private_key":"cfg"}}`)
	if c, _ := RuStore(); c.KeyID != "1" || c.PrivateKey != "cfg" || c.Source != "config.json" {
		t.Fatalf("config.json: %+v", c)
	}
	t.Setenv("RUSTORE_KEY_ID", "2")
	t.Setenv("RUSTORE_API_KEY", "legacy")
	if c, _ := RuStore(); c.KeyID != "2" || c.Source != "env RUSTORE_KEY_ID" {
		t.Fatalf("legacy env: %+v", c)
	}
	t.Setenv("DROIDSHIP_RUSTORE_KEY_ID", "3")
	t.Setenv("DROIDSHIP_RUSTORE_PRIVATE_KEY", "new")
	if c, _ := RuStore(); c.KeyID != "3" || c.PrivateKey != "new" {
		t.Fatalf("DROIDSHIP env: %+v", c)
	}
}

func TestAppGalleryEnvBeatsFileFieldByField(t *testing.T) {
	home := clean(t)
	write(t, filepath.Join(home, ".config", "droidship", "config.json"),
		`{"appgallery":{"client_id":"f-id","client_secret":"f-secret","region":"ru"}}`)
	t.Setenv("HUAWEI_APP_ID", "42")
	c, err := AppGallery()
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientID != "f-id" || c.ClientSecret != "f-secret" || c.Region != "ru" || c.AppID != "42" {
		t.Fatalf("merge: %+v", c)
	}
	t.Setenv("DROIDSHIP_APPGALLERY_CLIENT_ID", "e-id")
	t.Setenv("HSTORE_CLIENT_ID", "legacy-id")
	if c, _ := AppGallery(); c.ClientID != "e-id" {
		t.Fatalf("DROIDSHIP_ should beat HSTORE_: %+v", c)
	}
}

func TestAppGalleryLegacyFile(t *testing.T) {
	home := clean(t)
	write(t, filepath.Join(home, ".config", "hstore", "credentials.json"),
		`{"client_id":"a","client_secret":"b","app_id":"7"}`)
	c, err := AppGallery()
	if err != nil || c.ClientID != "a" || c.AppID != "7" {
		t.Fatalf("legacy file: %+v %v", c, err)
	}
}

func TestAppGalleryMissingSaysWhatToCreate(t *testing.T) {
	clean(t)
	if _, err := AppGallery(); err == nil || !strings.Contains(err.Error(), "DROIDSHIP_APPGALLERY_CLIENT_ID") {
		t.Fatalf("got %v", err)
	}
}

func TestAppGalleryRejectsTheJWTKeyFile(t *testing.T) {
	home := clean(t)
	write(t, filepath.Join(home, ".config", "hstore", "credentials.json"),
		`{"key_id":"x","private_key":"y","sub_account":"z"}`)
	if _, err := AppGallery(); err == nil || !strings.Contains(err.Error(), "private key file") {
		t.Fatalf("want the private-key hint, got %v", err)
	}
}
