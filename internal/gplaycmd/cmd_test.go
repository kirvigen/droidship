package gplaycmd

import (
	"strings"
	"testing"

	"github.com/kirvigen/droidship/internal/play"
)

func TestParseUploadArgsDefaultsToADraftProductionRelease(t *testing.T) {
	aab := writeTempFile(t, "app.aab", "x")
	cfg, err := parseUploadArgs([]string{"com.gdebenz.win", "--aab", aab})
	if err != nil {
		t.Fatalf("parseUploadArgs: %v", err)
	}
	if cfg.Track != play.TrackProduction {
		t.Errorf("Track = %q, want production", cfg.Track)
	}
	if cfg.Status != play.StatusDraft {
		t.Errorf("Status = %q, want draft — uploading must never publish by itself", cfg.Status)
	}
	if cfg.Lang != defaultLang {
		t.Errorf("Lang = %q, want %q", cfg.Lang, defaultLang)
	}
}

func TestParseUploadArgsErrors(t *testing.T) {
	aab := writeTempFile(t, "app.aab", "x")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no package", []string{"--aab", aab}, "package name is required"},
		{"no aab", []string{"com.example"}, "--aab is required"},
		{"missing file", []string{"com.example", "--aab", "/nope/app.aab"}, "file not found"},
		{"bad status", []string{"com.example", "--aab", aab, "--status", "live"}, "--status must be one of"},
		{"fraction on draft", []string{"com.example", "--aab", aab, "--user-fraction", "0.1"}, "only applies to"},
		{"fraction out of range", []string{"com.example", "--aab", aab, "--status", "inProgress", "--user-fraction", "1"}, "between 0 and 1"},
		{"inProgress without fraction", []string{"com.example", "--aab", aab, "--status", "inProgress"}, "needs --user-fraction"},
		{"priority range", []string{"com.example", "--aab", aab, "--priority", "9"}, "--priority must be"},
		{"both notes", []string{"com.example", "--aab", aab, "--whats-new", "a", "--whats-new-file", "b"}, "not both"},
		{"missing mapping", []string{"com.example", "--aab", aab, "--mapping", "/nope/mapping.txt"}, "file not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseUploadArgs(tc.args)
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseUploadArgsAcceptsAStagedRollout(t *testing.T) {
	aab := writeTempFile(t, "app.aab", "x")
	cfg, err := parseUploadArgs([]string{
		"com.example", "--aab", aab, "--status", "inProgress", "--user-fraction", "0.1", "--track", "beta",
	})
	if err != nil {
		t.Fatalf("parseUploadArgs: %v", err)
	}
	if cfg.UserFraction != 0.1 || cfg.Track != "beta" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestParseReleaseArgs(t *testing.T) {
	t.Run("release needs a version code", func(t *testing.T) {
		_, err := parseReleaseArgs([]string{"com.example"}, "release")
		if err == nil || !strings.Contains(err.Error(), "--version-code is required") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("release defaults to completed", func(t *testing.T) {
		cfg, err := parseReleaseArgs([]string{"com.example", "--version-code", "14"}, "release")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Status != play.StatusCompleted || cfg.VersionCode != 14 {
			t.Errorf("cfg = %+v", cfg)
		}
	})
	t.Run("rollout needs a fraction", func(t *testing.T) {
		_, err := parseReleaseArgs([]string{"com.example"}, "rollout")
		if err == nil || !strings.Contains(err.Error(), "--user-fraction is required") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("rollout defaults to inProgress", func(t *testing.T) {
		cfg, err := parseReleaseArgs([]string{"com.example", "--user-fraction", "0.5"}, "rollout")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Status != play.StatusInProgress || cfg.UserFraction != 0.5 {
			t.Errorf("cfg = %+v", cfg)
		}
	})
}

func TestBuildRelease(t *testing.T) {
	t.Run("a draft carries no user fraction", func(t *testing.T) {
		r := buildRelease([]string{"14"}, play.StatusDraft, "", "заметки", "ru-RU", 0.5, 0)
		if r.UserFraction != 0 {
			t.Errorf("UserFraction = %v, want 0 — Play rejects a draft with a fraction", r.UserFraction)
		}
		if len(r.ReleaseNotes) != 1 || r.ReleaseNotes[0].Language != "ru-RU" {
			t.Errorf("ReleaseNotes = %+v", r.ReleaseNotes)
		}
	})
	t.Run("a staged rollout keeps it", func(t *testing.T) {
		r := buildRelease([]string{"14"}, play.StatusInProgress, "1.4.3", "", "ru-RU", 0.25, 3)
		if r.UserFraction != 0.25 {
			t.Errorf("UserFraction = %v", r.UserFraction)
		}
		if r.Name != "1.4.3" || r.InAppUpdatePriority != 3 {
			t.Errorf("release = %+v", r)
		}
		if len(r.ReleaseNotes) != 0 {
			t.Errorf("empty notes should stay absent, got %+v", r.ReleaseNotes)
		}
	})
}

func TestTargetVersionCodes(t *testing.T) {
	current := &play.Track{Track: "production", Releases: []play.TrackRelease{{VersionCodes: []string{"13"}}}}
	t.Run("explicit wins", func(t *testing.T) {
		got, err := targetVersionCodes(releaseConfig{VersionCode: 14}, current)
		if err != nil || len(got) != 1 || got[0] != "14" {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
	t.Run("falls back to what is on the track", func(t *testing.T) {
		got, err := targetVersionCodes(releaseConfig{}, current)
		if err != nil || got[0] != "13" {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
	t.Run("empty track is an error", func(t *testing.T) {
		_, err := targetVersionCodes(releaseConfig{Track: "beta"}, &play.Track{Track: "beta"})
		if err == nil || !strings.Contains(err.Error(), "no release to change") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestExistingNotesSurviveARollout(t *testing.T) {
	current := &play.Track{Releases: []play.TrackRelease{{
		Name:         "1.4.2",
		ReleaseNotes: []play.ReleaseNote{{Language: "ru-RU", Text: "старые заметки"}},
	}}}
	if got := existingNotes(current, "ru-RU"); got != "старые заметки" {
		t.Errorf("existingNotes = %q — a rollout must not wipe the notes", got)
	}
	if got := existingNotes(current, "en-US"); got != "" {
		t.Errorf("existingNotes(en-US) = %q, want empty", got)
	}
	if got := existingName(current); got != "1.4.2" {
		t.Errorf("existingName = %q", got)
	}
}

func TestResolveNotesFromFile(t *testing.T) {
	path := writeTempFile(t, "notes.txt", "  Починили карту\n\n")
	got, err := resolveNotes("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Починили карту" {
		t.Errorf("resolveNotes = %q", got)
	}
}
