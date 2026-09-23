package gplaycmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirvigen/droidship/internal/play"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractKeyFlag(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantKey string
		wantRes []string
	}{
		{"absent", []string{"tracks", "com.example"}, "", []string{"tracks", "com.example"}},
		{"separate", []string{"--key", "/k.json", "tracks", "com.example"}, "/k.json", []string{"tracks", "com.example"}},
		{"equals", []string{"tracks", "com.example", "--key=/k.json"}, "/k.json", []string{"tracks", "com.example"}},
		{"single dash", []string{"-key", "/k.json", "auth"}, "/k.json", []string{"auth"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, rest, err := extractKeyFlag(tc.args)
			if err != nil {
				t.Fatalf("extractKeyFlag: %v", err)
			}
			if key != tc.wantKey {
				t.Errorf("key = %q, want %q", key, tc.wantKey)
			}
			if strings.Join(rest, " ") != strings.Join(tc.wantRes, " ") {
				t.Errorf("rest = %v, want %v", rest, tc.wantRes)
			}
		})
	}
	if _, _, err := extractKeyFlag([]string{"tracks", "--key"}); err == nil {
		t.Error("a dangling --key should be an error")
	}
}

func TestResolveKeyPathPrefersTheExplicitFlag(t *testing.T) {
	t.Setenv("GPLAY_SA_JSON", "/from/env.json")
	got, err := resolveKeyPath("/from/flag.json")
	if err != nil || got != "/from/flag.json" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestResolveKeyPathFallsBackToTheEnvironment(t *testing.T) {
	t.Setenv("GPLAY_SA_JSON", "/from/env.json")
	got, err := resolveKeyPath("")
	if err != nil || got != "/from/env.json" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestResolveKeyPathUsesTheOnlyKeyInTheConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GPLAY_SA_JSON", "")
	dir := filepath.Join(home, ".config", "gplay")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	only := filepath.Join(dir, "gdebenz-play-sa.json")
	if err := os.WriteFile(only, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveKeyPath("")
	if err != nil || got != only {
		t.Fatalf("got %q, err %v", got, err)
	}

	// A second key makes the choice ambiguous — say so instead of guessing.
	if err := os.WriteFile(filepath.Join(dir, "other.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveKeyPath(""); err == nil || !strings.Contains(err.Error(), "several keys") {
		t.Errorf("err = %v, want an ambiguity error", err)
	}

	// key.json is the tie-breaker.
	if err := os.WriteFile(filepath.Join(dir, "key.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = resolveKeyPath("")
	if err != nil || filepath.Base(got) != "key.json" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestResolveKeyPathWithoutAnyKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GPLAY_SA_JSON", "")
	if _, err := resolveKeyPath(""); err == nil || !strings.Contains(err.Error(), "no service account key") {
		t.Errorf("err = %v", err)
	}
}

func TestRolloutLabel(t *testing.T) {
	cases := []struct {
		r    play.TrackRelease
		want string
	}{
		{play.TrackRelease{Status: play.StatusCompleted}, "100%"},
		{play.TrackRelease{Status: play.StatusInProgress, UserFraction: 0.1}, "10%"},
		{play.TrackRelease{Status: play.StatusHalted, UserFraction: 0.005}, "0.5%"},
		{play.TrackRelease{Status: play.StatusDraft}, "—"},
	}
	for _, tc := range cases {
		if got := rolloutLabel(tc.r); got != tc.want {
			t.Errorf("rolloutLabel(%+v) = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestRunRejectsUnknownCommands(t *testing.T) {
	if code := Run([]string{"deploy"}); code != 2 {
		t.Errorf("Run(deploy) = %d, want 2", code)
	}
	if code := Run([]string{"version"}); code != 0 {
		t.Errorf("Run(version) = %d, want 0", code)
	}
}
