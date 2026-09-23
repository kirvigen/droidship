// Package config resolves credentials for every store droidship talks to.
//
// Per store, the first match wins: a flag, a DROIDSHIP_* variable, the
// variable the old standalone tool read, ~/.config/droidship/config.json (or
// $DROIDSHIP_CONFIG), then the old tool's own config file. Source strings say
// where a value came from and never contain the value.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is the shape of ~/.config/droidship/config.json.
type File struct {
	GPlay *struct {
		Key string `json:"key"`
	} `json:"gplay,omitempty"`
	RuStore *struct {
		KeyID      string `json:"key_id"`
		PrivateKey string `json:"private_key"`
	} `json:"rustore,omitempty"`
	AppGallery *AppGalleryCreds `json:"appgallery,omitempty"`
}

// Path is where droidship reads its config file.
func Path() string {
	if p := os.Getenv("DROIDSHIP_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(home(), ".config", "droidship", "config.json")
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// load reads the droidship config file; a missing file is an empty config.
func load() (File, error) {
	var f File
	raw, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("%s: %w", Path(), err)
	}
	return f, nil
}

// GPlayKey finds the Google Play service account key file.
func GPlayKey(flagValue string) (path, source string, err error) {
	if flagValue != "" {
		return flagValue, "flag --key", nil
	}
	for _, v := range []string{"DROIDSHIP_GPLAY_KEY", "GPLAY_SA_JSON"} {
		if p := os.Getenv(v); p != "" {
			return p, "env " + v, nil
		}
	}
	f, err := load()
	if err != nil {
		return "", "", err
	}
	if f.GPlay != nil && f.GPlay.Key != "" {
		return expand(f.GPlay.Key), "config.json", nil
	}
	dir := filepath.Join(home(), ".config", "gplay")
	if def := filepath.Join(dir, "key.json"); isFile(def) {
		return def, "~/.config/gplay/key.json", nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	switch len(matches) {
	case 1:
		return matches[0], "~/.config/gplay/" + filepath.Base(matches[0]), nil
	case 0:
		return "", "", fmt.Errorf("no service account key found: pass --key PATH, set DROIDSHIP_GPLAY_KEY, or add \"gplay\": {\"key\": PATH} to %s", Path())
	default:
		sort.Strings(matches)
		return "", "", fmt.Errorf("several keys in %s — pass --key or set DROIDSHIP_GPLAY_KEY:\n  %s", dir, strings.Join(matches, "\n  "))
	}
}

// RuStoreCreds is a RuStore API key.
type RuStoreCreds struct {
	KeyID, PrivateKey, Source string
}

// RuStore finds the RuStore API key.
func RuStore() (RuStoreCreds, error) {
	pairs := [][2]string{
		{"DROIDSHIP_RUSTORE_KEY_ID", "DROIDSHIP_RUSTORE_PRIVATE_KEY"},
		{"RUSTORE_KEY_ID", "RUSTORE_API_KEY"},
	}
	for _, p := range pairs {
		if id, key := os.Getenv(p[0]), os.Getenv(p[1]); id != "" && key != "" {
			return RuStoreCreds{KeyID: id, PrivateKey: key, Source: "env " + p[0]}, nil
		}
	}
	f, err := load()
	if err != nil {
		return RuStoreCreds{}, err
	}
	if f.RuStore != nil && f.RuStore.KeyID != "" && f.RuStore.PrivateKey != "" {
		return RuStoreCreds{KeyID: f.RuStore.KeyID, PrivateKey: f.RuStore.PrivateKey, Source: "config.json"}, nil
	}
	return RuStoreCreds{}, fmt.Errorf("no RuStore key: set DROIDSHIP_RUSTORE_KEY_ID and DROIDSHIP_RUSTORE_PRIVATE_KEY (RuStore Console → Company/Developer → API RuStore), or add \"rustore\" to %s", Path())
}

// AppGalleryCreds is an AppGallery Connect API client plus the defaults that
// usually travel with it.
type AppGalleryCreds struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AppID        string `json:"app_id,omitempty"`
	Region       string `json:"region,omitempty"`
	Package      string `json:"package,omitempty"`
	Source       string `json:"-"`
}

// appGalleryPrefixes are the accepted names for the same setting, strongest first.
var appGalleryPrefixes = []string{"DROIDSHIP_APPGALLERY_", "HSTORE_", "HUAWEI_"}

// AppGallery resolves the AppGallery Connect credentials. Environment values
// override the file field by field.
func AppGallery() (AppGalleryCreds, error) {
	env := AppGalleryCreds{
		ClientID:     agEnv("CLIENT_ID"),
		ClientSecret: agEnv("CLIENT_SECRET"),
		AppID:        agEnv("APP_ID"),
		Region:       agEnv("REGION"),
		Package:      agEnv("PACKAGE"),
		Source:       "env " + agEnvName("CLIENT_ID"),
	}
	if env.ClientID != "" && env.ClientSecret != "" {
		return env, nil
	}
	file, err := appGalleryFromFiles()
	if err != nil {
		return AppGalleryCreds{}, err
	}
	for _, kv := range []struct {
		dst *string
		v   string
	}{
		{&file.ClientID, env.ClientID}, {&file.ClientSecret, env.ClientSecret},
		{&file.AppID, env.AppID}, {&file.Region, env.Region}, {&file.Package, env.Package},
	} {
		if kv.v != "" {
			*kv.dst = kv.v
		}
	}
	if file.ClientID == "" || file.ClientSecret == "" {
		return AppGalleryCreds{}, fmt.Errorf("%s has no client_id/client_secret", file.Source)
	}
	return file, nil
}

// agEnv reads one AppGallery setting under any accepted prefix.
func agEnv(name string) string {
	for _, prefix := range appGalleryPrefixes {
		if v := os.Getenv(prefix + name); v != "" {
			return v
		}
	}
	return ""
}

// agEnvName is the variable agEnv read a setting from.
func agEnvName(name string) string {
	for _, prefix := range appGalleryPrefixes {
		if os.Getenv(prefix+name) != "" {
			return prefix + name
		}
	}
	return ""
}

// appGalleryFromFiles reads the droidship config, then the legacy hstore file.
func appGalleryFromFiles() (AppGalleryCreds, error) {
	f, err := load()
	if err != nil {
		return AppGalleryCreds{}, err
	}
	if f.AppGallery != nil && f.AppGallery.ClientID != "" {
		c := *f.AppGallery
		c.Source = "config.json"
		return c, nil
	}
	path, err := hstoreCredentialsPath()
	if err != nil {
		return AppGalleryCreds{}, err
	}
	c, err := readHStoreCredentials(path)
	if err != nil {
		return AppGalleryCreds{}, err
	}
	c.Source = path
	return c, nil
}

// hstoreConfigDir is where the hstore tool kept its credentials file.
func hstoreConfigDir() string {
	if dir := os.Getenv("HSTORE_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(home(), ".config", "hstore")
}

// hstoreCredentialsPath finds the legacy credentials file, explaining what to
// create when there is none.
func hstoreCredentialsPath() (string, error) {
	if path := os.Getenv("HSTORE_CREDENTIALS"); path != "" {
		return path, nil
	}
	dir := hstoreConfigDir()
	named := filepath.Join(dir, "credentials.json")
	if _, err := os.Stat(named); err == nil {
		return named, nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf(`no AppGallery credentials found.

Set DROIDSHIP_APPGALLERY_CLIENT_ID and DROIDSHIP_APPGALLERY_CLIENT_SECRET, or add
an "appgallery" section to %s (the old %s works too):

  {"appgallery": {"client_id": "…", "client_secret": "…", "app_id": "118236677", "region": "global"}}

The pair comes from AppGallery Connect -> Users and permissions -> API key ->
Connect API -> Create (set Project to N/A, otherwise the API answers 403).`, Path(), named)
	default:
		return "", fmt.Errorf("%s holds several .json files; point HSTORE_CREDENTIALS at the right one", dir)
	}
}

// readHStoreCredentials parses a credentials file, recognising the private key
// file AppGallery Connect hands out for the other authentication scheme.
func readHStoreCredentials(path string) (AppGalleryCreds, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AppGalleryCreds{}, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return AppGalleryCreds{}, fmt.Errorf("%s: %w", path, err)
	}
	if _, isKeyFile := probe["private_key"]; isKeyFile {
		return AppGalleryCreds{}, fmt.Errorf(`%s is a private key file (key_id/private_key/sub_account) for the JWT scheme, not the Connect API pair.

droidship needs a client_id and client_secret from AppGallery Connect ->
Users and permissions -> API key -> Connect API. They are shown on screen once,
there is no file to download`, path)
	}
	var c AppGalleryCreds
	if err := json.Unmarshal(raw, &c); err != nil {
		return AppGalleryCreds{}, fmt.Errorf("%s: %w", path, err)
	}
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.ClientSecret = strings.TrimSpace(c.ClientSecret)
	return c, nil
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home(), p[2:])
	}
	return p
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
