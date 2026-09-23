package gplaycmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kirvigen/droidship/internal/play"
)

// defaultLang is the release-notes locale for a Russian-first app.
const defaultLang = "ru-RU"

var validStatus = map[string]bool{
	play.StatusDraft:      true,
	play.StatusCompleted:  true,
	play.StatusInProgress: true,
	play.StatusHalted:     true,
}

// fractionalStatus reports whether a status carries a userFraction.
func fractionalStatus(s string) bool {
	return s == play.StatusInProgress || s == play.StatusHalted
}

// uploadConfig is everything the upload command needs.
type uploadConfig struct {
	Package      string
	AAB          string
	Mapping      string
	Track        string
	Status       string
	Name         string
	UserFraction float64
	Priority     int
	WhatsNew     string
	WhatsNewFile string
	Lang         string
	ValidateOnly bool
	NoReview     bool
}

// parseUploadArgs parses `droidship gplay upload <package> [flags]`.
func parseUploadArgs(args []string) (uploadConfig, error) {
	var cfg uploadConfig
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return cfg, fmt.Errorf("package name is required: droidship gplay upload <package> --aab F [flags]")
	}
	cfg.Package = args[0]

	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.StringVar(&cfg.AAB, "aab", "", "path to the .aab to upload")
	fs.StringVar(&cfg.Mapping, "mapping", "", "path to the ProGuard/R8 mapping.txt for this build")
	fs.StringVar(&cfg.Track, "track", play.TrackProduction, "internal | alpha | beta | production")
	fs.StringVar(&cfg.Status, "status", play.StatusDraft, "draft | completed | inProgress | halted")
	fs.StringVar(&cfg.Name, "name", "", "release name shown in the Play Console")
	fs.Float64Var(&cfg.UserFraction, "user-fraction", 0, "staged rollout share, 0<f<1 (inProgress/halted only)")
	fs.IntVar(&cfg.Priority, "priority", 0, "in-app update priority 0..5")
	fs.StringVar(&cfg.WhatsNew, "whats-new", "", "release notes")
	fs.StringVar(&cfg.WhatsNewFile, "whats-new-file", "", "read release notes from a file")
	fs.StringVar(&cfg.Lang, "lang", defaultLang, "release notes language tag")
	fs.BoolVar(&cfg.ValidateOnly, "validate-only", false, "validate the edit and discard it, changing nothing")
	fs.BoolVar(&cfg.NoReview, "no-review", false, "commit without sending the changes to Google review")
	if err := fs.Parse(args[1:]); err != nil {
		return cfg, err
	}

	if cfg.AAB == "" {
		return cfg, fmt.Errorf("--aab is required")
	}
	if _, err := os.Stat(cfg.AAB); err != nil {
		return cfg, fmt.Errorf("file not found: %s", cfg.AAB)
	}
	if cfg.Mapping != "" {
		if _, err := os.Stat(cfg.Mapping); err != nil {
			return cfg, fmt.Errorf("file not found: %s", cfg.Mapping)
		}
	}
	if err := validateRelease(cfg.Track, cfg.Status, cfg.UserFraction, cfg.Priority); err != nil {
		return cfg, err
	}
	if cfg.WhatsNew != "" && cfg.WhatsNewFile != "" {
		return cfg, fmt.Errorf("use either --whats-new or --whats-new-file, not both")
	}
	return cfg, nil
}

// validateRelease checks the flags every release-shaped command shares.
func validateRelease(track, status string, fraction float64, priority int) error {
	if track == "" {
		return fmt.Errorf("--track must not be empty")
	}
	if !validStatus[status] {
		return fmt.Errorf("--status must be one of draft, completed, inProgress, halted")
	}
	if fraction != 0 {
		if !fractionalStatus(status) {
			return fmt.Errorf("--user-fraction only applies to --status inProgress or halted")
		}
		if fraction <= 0 || fraction >= 1 {
			return fmt.Errorf("--user-fraction must be between 0 and 1, exclusive (0.1 = 10%%)")
		}
	}
	if fractionalStatus(status) && fraction == 0 {
		return fmt.Errorf("--status %s needs --user-fraction", status)
	}
	if priority < 0 || priority > 5 {
		return fmt.Errorf("--priority must be between 0 and 5")
	}
	return nil
}

// resolveNotes returns the release notes from the inline flag or the file.
func resolveNotes(inline, path string) (string, error) {
	if path == "" {
		return inline, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read release notes: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// buildRelease assembles the TrackRelease Play will store.
func buildRelease(versionCodes []string, status, name, notes, lang string, fraction float64, priority int) play.TrackRelease {
	r := play.TrackRelease{
		Name:                name,
		VersionCodes:        versionCodes,
		Status:              status,
		InAppUpdatePriority: priority,
	}
	if fractionalStatus(status) {
		r.UserFraction = fraction
	}
	if notes != "" {
		r.ReleaseNotes = []play.ReleaseNote{{Language: lang, Text: notes}}
	}
	return r
}

// runUpload drives the pipeline: edit -> upload -> track -> commit.
func runUpload(ctx context.Context, c *play.Client, cfg uploadConfig, out io.Writer) error {
	notes, err := resolveNotes(cfg.WhatsNew, cfg.WhatsNewFile)
	if err != nil {
		return err
	}

	edit, err := c.CreateEdit(ctx, cfg.Package)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "edit %s opened for %s\n", edit.ID, cfg.Package)

	committed := false
	defer func() {
		if !committed {
			if err := c.DeleteEdit(ctx, cfg.Package, edit.ID); err == nil {
				fmt.Fprintf(out, "edit %s discarded — nothing changed in the Play Console\n", edit.ID)
			}
		}
	}()

	fmt.Fprintf(out, "uploading %s…\n", cfg.AAB)
	bundle, err := c.UploadBundle(ctx, cfg.Package, edit.ID, cfg.AAB, out)
	if err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}
	fmt.Fprintf(out, "uploaded: versionCode %d, sha256 %s\n", bundle.VersionCode, bundle.SHA256)

	if cfg.Mapping != "" {
		fmt.Fprintf(out, "uploading mapping %s…\n", cfg.Mapping)
		if err := c.UploadDeobfuscationFile(ctx, cfg.Package, edit.ID, bundle.VersionCode, "proguard", cfg.Mapping, out); err != nil {
			return fmt.Errorf("upload mapping: %w", err)
		}
		fmt.Fprintln(out, "mapping uploaded")
	}

	release := buildRelease([]string{strconv.Itoa(bundle.VersionCode)}, cfg.Status, cfg.Name, notes, cfg.Lang, cfg.UserFraction, cfg.Priority)
	if _, err := c.UpdateTrack(ctx, cfg.Package, edit.ID, play.Track{Track: cfg.Track, Releases: []play.TrackRelease{release}}); err != nil {
		return fmt.Errorf("update track %s: %w", cfg.Track, err)
	}
	fmt.Fprintf(out, "track %s set to versionCode %d, status %s\n", cfg.Track, bundle.VersionCode, cfg.Status)

	if cfg.ValidateOnly {
		if err := c.ValidateEdit(ctx, cfg.Package, edit.ID); err != nil {
			return fmt.Errorf("validate: %w", err)
		}
		fmt.Fprintln(out, "validation passed — edit discarded (--validate-only), nothing was published")
		return nil
	}

	if err := commitEdit(ctx, c, cfg.Package, edit.ID, !cfg.NoReview, out); err != nil {
		return err
	}
	committed = true
	reportOutcome(out, cfg.Package, cfg.Track, cfg.Status, bundle.VersionCode)
	return nil
}

// commitEdit commits the edit, falling back to a no-review commit when Play
// refuses to auto-submit the changes.
func commitEdit(ctx context.Context, c *play.Client, pkg, editID string, sendForReview bool, out io.Writer) error {
	err := c.CommitEdit(ctx, pkg, editID, sendForReview)
	if err == nil {
		return nil
	}
	var apiErr *play.APIError
	if sendForReview && errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "changesNotSentForReview") {
		fmt.Fprintln(out, "Play declined to auto-submit for review — committing without review instead")
		if retry := c.CommitEdit(ctx, pkg, editID, false); retry != nil {
			return fmt.Errorf("commit (no review): %w", retry)
		}
		return nil
	}
	return fmt.Errorf("commit: %w", err)
}

// reportOutcome spells out what a human must still do, because a draft release
// looks identical to a published one from the terminal.
func reportOutcome(out io.Writer, pkg, track, status string, versionCode int) {
	switch status {
	case play.StatusDraft:
		fmt.Fprintf(out, "\ndone: versionCode %d is a DRAFT release on the %s track of %s.\n", versionCode, track, pkg)
		fmt.Fprintln(out, "Nothing is live yet. Open Play Console → Release → the track → Edit release")
		fmt.Fprintln(out, "→ Review release → Start rollout to finish it.")
	case play.StatusCompleted:
		fmt.Fprintf(out, "\ndone: versionCode %d released to 100%% of the %s track of %s.\n", versionCode, track, pkg)
	default:
		fmt.Fprintf(out, "\ndone: versionCode %d is %s on the %s track of %s.\n", versionCode, status, track, pkg)
	}
}

// releaseConfig backs both `release` (promote a version code) and `rollout`
// (move the share of users an existing release reaches).
type releaseConfig struct {
	Mode         string // "release" or "rollout"
	Package      string
	Track        string
	Status       string
	Name         string
	VersionCode  int
	UserFraction float64
	Priority     int
	WhatsNew     string
	WhatsNewFile string
	Lang         string
	NoReview     bool
}

// parseReleaseArgs parses `droidship gplay release|rollout <package> [flags]`.
func parseReleaseArgs(args []string, mode string) (releaseConfig, error) {
	cfg := releaseConfig{Mode: mode}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return cfg, fmt.Errorf("package name is required: droidship gplay %s <package> [flags]", mode)
	}
	cfg.Package = args[0]

	defaultStatus := play.StatusCompleted
	if mode == "rollout" {
		defaultStatus = play.StatusInProgress
	}

	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	fs.StringVar(&cfg.Track, "track", play.TrackProduction, "internal | alpha | beta | production")
	fs.StringVar(&cfg.Status, "status", defaultStatus, "draft | completed | inProgress | halted")
	fs.StringVar(&cfg.Name, "name", "", "release name shown in the Play Console")
	fs.IntVar(&cfg.VersionCode, "version-code", 0, "version code of an already uploaded bundle")
	fs.Float64Var(&cfg.UserFraction, "user-fraction", 0, "staged rollout share, 0<f<1")
	fs.IntVar(&cfg.Priority, "priority", 0, "in-app update priority 0..5")
	fs.StringVar(&cfg.WhatsNew, "whats-new", "", "release notes")
	fs.StringVar(&cfg.WhatsNewFile, "whats-new-file", "", "read release notes from a file")
	fs.StringVar(&cfg.Lang, "lang", defaultLang, "release notes language tag")
	fs.BoolVar(&cfg.NoReview, "no-review", false, "commit without sending the changes to Google review")
	if err := fs.Parse(args[1:]); err != nil {
		return cfg, err
	}

	if mode == "release" && cfg.VersionCode <= 0 {
		return cfg, fmt.Errorf("--version-code is required: see `droidship gplay bundles %s`", cfg.Package)
	}
	if mode == "rollout" && cfg.UserFraction == 0 {
		return cfg, fmt.Errorf("--user-fraction is required: droidship gplay rollout %s --user-fraction 0.5", cfg.Package)
	}
	if err := validateRelease(cfg.Track, cfg.Status, cfg.UserFraction, cfg.Priority); err != nil {
		return cfg, err
	}
	if cfg.WhatsNew != "" && cfg.WhatsNewFile != "" {
		return cfg, fmt.Errorf("use either --whats-new or --whats-new-file, not both")
	}
	return cfg, nil
}

// runRelease changes an existing track without uploading anything.
func runRelease(ctx context.Context, c *play.Client, cfg releaseConfig, out io.Writer) error {
	notes, err := resolveNotes(cfg.WhatsNew, cfg.WhatsNewFile)
	if err != nil {
		return err
	}

	edit, err := c.CreateEdit(ctx, cfg.Package)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = c.DeleteEdit(ctx, cfg.Package, edit.ID)
		}
	}()

	current, err := c.Track(ctx, cfg.Package, edit.ID, cfg.Track)
	if err != nil {
		return fmt.Errorf("read track %s: %w", cfg.Track, err)
	}

	versionCodes, err := targetVersionCodes(cfg, current)
	if err != nil {
		return err
	}
	if notes == "" {
		notes = existingNotes(current, cfg.Lang)
	}
	name := cfg.Name
	if name == "" {
		name = existingName(current)
	}

	release := buildRelease(versionCodes, cfg.Status, name, notes, cfg.Lang, cfg.UserFraction, cfg.Priority)
	if _, err := c.UpdateTrack(ctx, cfg.Package, edit.ID, play.Track{Track: cfg.Track, Releases: []play.TrackRelease{release}}); err != nil {
		return fmt.Errorf("update track %s: %w", cfg.Track, err)
	}
	if err := commitEdit(ctx, c, cfg.Package, edit.ID, !cfg.NoReview, out); err != nil {
		return err
	}
	committed = true

	fmt.Fprintf(out, "track %s of %s now serves versionCode(s) %s with status %s",
		cfg.Track, cfg.Package, strings.Join(versionCodes, ", "), cfg.Status)
	if fractionalStatus(cfg.Status) {
		fmt.Fprintf(out, " at %.4g%% of users", cfg.UserFraction*100)
	}
	fmt.Fprintln(out)
	return nil
}

// targetVersionCodes picks which build the new track state should serve: the
// one named on the command line, or the one already there for a rollout.
func targetVersionCodes(cfg releaseConfig, current *play.Track) ([]string, error) {
	if cfg.VersionCode > 0 {
		return []string{strconv.Itoa(cfg.VersionCode)}, nil
	}
	for _, r := range current.Releases {
		if len(r.VersionCodes) > 0 {
			return r.VersionCodes, nil
		}
	}
	return nil, fmt.Errorf("track %s has no release to change — upload a build first, or pass --version-code", cfg.Track)
}

// existingNotes keeps the release notes already on the track so a rollout does
// not silently wipe them.
func existingNotes(current *play.Track, lang string) string {
	for _, r := range current.Releases {
		for _, n := range r.ReleaseNotes {
			if n.Language == lang {
				return n.Text
			}
		}
	}
	return ""
}

func existingName(current *play.Track) string {
	for _, r := range current.Releases {
		if r.Name != "" {
			return r.Name
		}
	}
	return ""
}
