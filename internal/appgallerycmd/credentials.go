package appgallerycmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// credentials are the API client credentials plus the defaults that usually
// travel with them.
type credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AppID        string `json:"app_id,omitempty"`
	Region       string `json:"region,omitempty"`
	Package      string `json:"package,omitempty"`
}

// envPrefixes are the accepted names for the same setting: HSTORE_CLIENT_ID and
// HUAWEI_CLIENT_ID both work.
var envPrefixes = []string{"HSTORE_", "HUAWEI_"}

// lookupEnv reads one setting under either prefix.
func lookupEnv(name string) string {
	for _, prefix := range envPrefixes {
		if value := os.Getenv(prefix + name); value != "" {
			return value
		}
	}
	return ""
}

// configDir is where hstore looks for a credentials file.
func configDir() string {
	if dir := os.Getenv("HSTORE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/hstore"
	}
	return filepath.Join(home, ".config", "hstore")
}

// loadCredentials resolves the credentials from, in order: the environment, the
// file named by HSTORE_CREDENTIALS, credentials.json in the config directory, or
// the single *.json file there.
func loadCredentials() (credentials, error) {
	env := credentials{
		ClientID:     lookupEnv("CLIENT_ID"),
		ClientSecret: lookupEnv("CLIENT_SECRET"),
		AppID:        lookupEnv("APP_ID"),
		Region:       lookupEnv("REGION"),
		Package:      lookupEnv("PACKAGE"),
	}
	if env.ClientID != "" && env.ClientSecret != "" {
		return env, nil
	}

	path, err := credentialsPath()
	if err != nil {
		return credentials{}, err
	}
	fromFile, err := readCredentials(path)
	if err != nil {
		return credentials{}, err
	}
	// The environment still wins over the file, field by field.
	if env.ClientID != "" {
		fromFile.ClientID = env.ClientID
	}
	if env.ClientSecret != "" {
		fromFile.ClientSecret = env.ClientSecret
	}
	if env.AppID != "" {
		fromFile.AppID = env.AppID
	}
	if env.Region != "" {
		fromFile.Region = env.Region
	}
	if env.Package != "" {
		fromFile.Package = env.Package
	}
	if fromFile.ClientID == "" || fromFile.ClientSecret == "" {
		return credentials{}, fmt.Errorf("%s has no client_id/client_secret", path)
	}
	return fromFile, nil
}

// credentialsPath finds the credentials file, explaining what to create when
// there is none.
func credentialsPath() (string, error) {
	if path := os.Getenv("HSTORE_CREDENTIALS"); path != "" {
		return path, nil
	}
	dir := configDir()
	named := filepath.Join(dir, "credentials.json")
	if _, err := os.Stat(named); err == nil {
		return named, nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf(`no credentials found.

Set HSTORE_CLIENT_ID and HSTORE_CLIENT_SECRET, or write %s:

  {"client_id": "…", "client_secret": "…", "app_id": "118236677", "region": "global"}

The pair comes from AppGallery Connect -> Users and permissions -> API key ->
Connect API -> Create (set Project to N/A, otherwise the API answers 403).`, named)
	default:
		return "", fmt.Errorf("%s holds several .json files; point HSTORE_CREDENTIALS at the right one", dir)
	}
}

// readCredentials parses a credentials file, recognising the private key file
// AppGallery Connect hands out for the other authentication scheme.
func readCredentials(path string) (credentials, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return credentials{}, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return credentials{}, fmt.Errorf("%s: %w", path, err)
	}
	if _, isKeyFile := probe["private_key"]; isKeyFile {
		return credentials{}, fmt.Errorf(`%s is a private key file (key_id/private_key/sub_account), not the Connect API pair.

droidship needs a client_id and client_secret from AppGallery Connect ->
Users and permissions -> API key -> Connect API. They are shown on screen once,
there is no file to download`, path)
	}
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return credentials{}, fmt.Errorf("%s: %w", path, err)
	}
	creds.ClientID = strings.TrimSpace(creds.ClientID)
	creds.ClientSecret = strings.TrimSpace(creds.ClientSecret)
	return creds, nil
}
