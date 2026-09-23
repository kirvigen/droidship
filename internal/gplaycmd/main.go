// Package gplaycmd is droidship's Google Play namespace, formerly the gplay
// CLI: upload Android App Bundles, drive release tracks, answer reviews and
// edit the store page.
package gplaycmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kirvigen/droidship/internal/config"
	"github.com/kirvigen/droidship/internal/play"
	"github.com/kirvigen/droidship/internal/version"
)

const usage = `droidship gplay — Google Play Console CLI

Usage:
  droidship gplay auth                                   check the service account key
  droidship gplay tracks <package> [--json]              tracks and their releases
  droidship gplay bundles <package> [--json]             app bundles already uploaded
  droidship gplay upload <package> --aab F
        [--track internal|alpha|beta|production]  (default: production)
        [--status draft|completed|inProgress|halted]  (default: draft)
        [--user-fraction 0.1] [--priority 0..5] [--name S]
        [--whats-new S | --whats-new-file F] [--lang ru-RU]
        [--mapping F] [--validate-only] [--no-review]
  droidship gplay release <package> --version-code N [--track T] [--status S]
        [--user-fraction F] [--whats-new S | --whats-new-file F] [--lang L]
  droidship gplay rollout <package> --user-fraction F [--track T]
  droidship gplay reviews <package> [--stars N] [--unreplied] [--limit N]
        [--translate ru] [--json]
  droidship gplay reply <package> <reviewId> (--text S | --text-file F)

  droidship gplay listing <package> [--lang L] [--json]
  droidship gplay listing set <package> --lang L [--title S] [--short S]
        [--full S | --full-file F] [--video URL] [--dry-run]
  droidship gplay details <package> [--json]
  droidship gplay details set <package> [--email S] [--phone S] [--website S] [--dry-run]
  droidship gplay screenshots <package> [--lang L] [--type T] [--json]
  droidship gplay screenshots upload <package> [--lang L] [--type T] [--replace] FILE...
  droidship gplay screenshots delete <package> [--lang L] [--type T] (--id ID | --all)

  droidship gplay achievements list --app GAME_PROJECT_ID [--locale ru] [--json]
  droidship gplay achievements sync --app ID --file spec.json
        [--dry-run] [--prune] [--icons=false]
  droidship gplay achievements publish --app GAME_PROJECT_ID
  droidship gplay achievements delete --id ACHIEVEMENT_ID
  droidship gplay version

By design upload creates a DRAFT release: the build lands in the Play Console
and a human starts the rollout there. Pass --status completed to publish.

Achievements live in Play Games Services, a separate API: enable it once with
  gcloud services enable gamesconfiguration.googleapis.com --project=<project>
GAME_PROJECT_ID is the numeric id under Play Games Services → Configuration,
not the applicationId. sync writes drafts; publish makes them visible.

Credentials — a Google Cloud service account JSON key, granted access in
Play Console → Users and permissions:
  --key PATH                explicit path
  DROIDSHIP_GPLAY_KEY       path in the environment (GPLAY_SA_JSON works too)
  ~/.config/droidship/config.json   {"gplay": {"key": "PATH"}}
  ~/.config/gplay/          key.json, or the only *.json file in the directory
`

// Run executes the store namespace with the arguments that follow the store
// name, e.g. ["upload", "com.example", "--aab", "app.aab"], and returns the exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if c := args[0]; c == "version" || c == "--version" {
		fmt.Println("droidship", version.Version)
		return 0
	}
	if c := args[0]; c == "help" || c == "-h" || c == "--help" {
		fmt.Print(usage)
		return 0
	}

	keyPath, args, err := extractKeyFlag(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "auth", "tracks", "bundles", "upload", "release", "rollout", "reviews", "reply",
		"listing", "details", "screenshots", "achievements":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	client, err := newClient(keyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	ctx := context.Background()

	switch cmd {
	case "auth":
		err = cmdAuth(ctx, client)
	case "tracks":
		err = cmdTracks(ctx, client, rest)
	case "bundles":
		err = cmdBundles(ctx, client, rest)
	case "upload":
		var cfg uploadConfig
		if cfg, err = parseUploadArgs(rest); err == nil {
			err = runUpload(ctx, client, cfg, os.Stdout)
		}
	case "release":
		var cfg releaseConfig
		if cfg, err = parseReleaseArgs(rest, "release"); err == nil {
			err = runRelease(ctx, client, cfg, os.Stdout)
		}
	case "rollout":
		var cfg releaseConfig
		if cfg, err = parseReleaseArgs(rest, "rollout"); err == nil {
			err = runRelease(ctx, client, cfg, os.Stdout)
		}
	case "reviews":
		err = cmdReviews(ctx, client, rest)
	case "reply":
		err = cmdReply(ctx, client, rest)
	case "listing":
		err = cmdListing(ctx, client, rest)
	case "details":
		err = cmdDetails(ctx, client, rest)
	case "screenshots":
		err = cmdScreenshots(ctx, client, rest)
	case "achievements":
		err = cmdAchievements(ctx, client, rest)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// extractKeyFlag pulls a leading-or-trailing --key out of the argument list so
// every subcommand accepts it without repeating the flag definition.
func extractKeyFlag(args []string) (string, []string, error) {
	var key string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--key" || a == "-key":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--key needs a path")
			}
			key = args[i+1]
			i++
		case strings.HasPrefix(a, "--key="), strings.HasPrefix(a, "-key="):
			key = a[strings.Index(a, "=")+1:]
			if key == "" {
				return "", nil, fmt.Errorf("--key needs a path")
			}
		default:
			rest = append(rest, a)
		}
	}
	return key, rest, nil
}

// resolveKeyPath finds the service account key: explicit flag, environment,
// the droidship config file, then the legacy ~/.config/gplay directory.
func resolveKeyPath(explicit string) (string, error) {
	path, _, err := config.GPlayKey(explicit)
	return path, err
}

func newClient(explicitKey string) (*play.Client, error) {
	path, err := resolveKeyPath(explicitKey)
	if err != nil {
		return nil, err
	}
	return play.New(path)
}

func cmdAuth(ctx context.Context, c *play.Client) error {
	if _, err := c.Token(ctx); err != nil {
		return err
	}
	sa := c.ServiceAccount()
	fmt.Println("auth OK")
	fmt.Println("  service account:", sa.ClientEmail)
	fmt.Println("  cloud project  :", sa.ProjectID)
	fmt.Println("  client id      :", sa.ClientID)
	fmt.Println()
	fmt.Println("A token only proves the key is valid. Access to an app is granted separately")
	fmt.Println("in Play Console → Users and permissions; check it with `droidship gplay tracks <package>`.")
	return nil
}

// withEdit opens an edit, runs fn, and always throws the edit away. Read-only
// commands need an edit because the Play API exposes no other way to look.
func withEdit(ctx context.Context, c *play.Client, pkg string, fn func(editID string) error) error {
	edit, err := c.CreateEdit(ctx, pkg)
	if err != nil {
		return err
	}
	defer func() { _ = c.DeleteEdit(ctx, pkg, edit.ID) }()
	return fn(edit.ID)
}

func cmdTracks(ctx context.Context, c *play.Client, args []string) error {
	pkg, fs, err := packageAndFlags(args, "tracks")
	if err != nil {
		return err
	}
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var tracks []play.Track
	if err := withEdit(ctx, c, pkg, func(editID string) error {
		tracks, err = c.Tracks(ctx, pkg, editID)
		return err
	}); err != nil {
		return err
	}
	if *asJSON {
		return printJSON(tracks)
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TRACK\tSTATUS\tVERSION CODES\tROLLOUT\tNAME")
	for _, t := range tracks {
		if len(t.Releases) == 0 {
			fmt.Fprintf(w, "%s\t—\t—\t—\t—\n", t.Track)
			continue
		}
		for _, r := range t.Releases {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				t.Track, dash(r.Status), dash(strings.Join(r.VersionCodes, ", ")),
				rolloutLabel(r), dash(oneLine(r.Name, 30)))
		}
	}
	return w.Flush()
}

func cmdBundles(ctx context.Context, c *play.Client, args []string) error {
	pkg, fs, err := packageAndFlags(args, "bundles")
	if err != nil {
		return err
	}
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var bundles []play.Bundle
	if err := withEdit(ctx, c, pkg, func(editID string) error {
		bundles, err = c.Bundles(ctx, pkg, editID)
		return err
	}); err != nil {
		return err
	}
	if *asJSON {
		return printJSON(bundles)
	}
	sort.Slice(bundles, func(i, j int) bool { return bundles[i].VersionCode > bundles[j].VersionCode })
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION CODE\tSHA256")
	for _, b := range bundles {
		fmt.Fprintf(w, "%d\t%s\n", b.VersionCode, b.SHA256)
	}
	return w.Flush()
}

// packageAndFlags validates the leading <package> argument shared by commands.
func packageAndFlags(args []string, cmd string) (string, *flag.FlagSet, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, fmt.Errorf("package name is required: droidship gplay %s <package> [flags]", cmd)
	}
	return args[0], flag.NewFlagSet(cmd, flag.ContinueOnError), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// rolloutLabel renders the share of users a release is live for.
func rolloutLabel(r play.TrackRelease) string {
	switch r.Status {
	case play.StatusInProgress, play.StatusHalted:
		if r.UserFraction > 0 {
			return fmt.Sprintf("%.4g%%", r.UserFraction*100)
		}
		return "—"
	case play.StatusCompleted:
		return "100%"
	}
	return "—"
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
