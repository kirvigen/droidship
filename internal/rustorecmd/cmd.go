package rustorecmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kirvigen/droidship/internal/rustore"
)

// publishConfig is everything the publish command needs.
type publishConfig struct {
	Package      string
	APK          string
	HMSAPK       string
	AAB          string
	WhatsNew     string
	WhatsNewFile string
	ModerInfo    string
	PublishType  string
	PublishDate  string
	Partial      int
	Priority     int
	ReplaceDraft bool
	SkipCommit   bool
}

var validPartial = map[int]bool{0: true, 5: true, 10: true, 25: true, 50: true, 75: true, 100: true}

// parsePublishArgs parses `droidship rustore publish <package> [flags]`.
func parsePublishArgs(args []string) (publishConfig, error) {
	var cfg publishConfig
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return cfg, fmt.Errorf("package name is required: droidship rustore publish <package> [flags]")
	}
	cfg.Package = args[0]

	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.StringVar(&cfg.APK, "apk", "", "path to the main .apk")
	fs.StringVar(&cfg.HMSAPK, "hms-apk", "", "path to an additional .apk with Huawei Mobile Services")
	fs.StringVar(&cfg.AAB, "aab", "", "path to an .aab (requires a signing key in the RuStore Console)")
	fs.StringVar(&cfg.WhatsNew, "whats-new", "", "release notes (up to 5000 chars)")
	fs.StringVar(&cfg.WhatsNewFile, "whats-new-file", "", "read release notes from a file")
	fs.StringVar(&cfg.ModerInfo, "moder-info", "", "comment for the moderator (up to 180 chars)")
	fs.StringVar(&cfg.PublishType, "publish-type", "", "MANUAL | INSTANTLY | DELAYED (store default: INSTANTLY)")
	fs.StringVar(&cfg.PublishDate, "publish-date", "", "publication date-time for DELAYED, e.g. 2026-09-01T10:00:00+03:00")
	fs.IntVar(&cfg.Partial, "partial", 0, "partial rollout percent: 5|10|25|50|75|100")
	fs.IntVar(&cfg.Priority, "priority", 0, "update priority 0..5")
	fs.BoolVar(&cfg.ReplaceDraft, "replace-draft", false, "delete an existing draft and create a new one")
	fs.BoolVar(&cfg.SkipCommit, "skip-commit", false, "create the draft and upload files, but do not submit for moderation")
	if err := fs.Parse(args[1:]); err != nil {
		return cfg, err
	}

	if cfg.HMSAPK != "" && cfg.APK == "" {
		return cfg, fmt.Errorf("--hms-apk can only accompany a main --apk")
	}
	if (cfg.APK == "") == (cfg.AAB == "") {
		return cfg, fmt.Errorf("provide exactly one of --apk or --aab")
	}
	if cfg.WhatsNew != "" && cfg.WhatsNewFile != "" {
		return cfg, fmt.Errorf("use either --whats-new or --whats-new-file, not both")
	}
	if !validPartial[cfg.Partial] {
		return cfg, fmt.Errorf("--partial must be one of 5, 10, 25, 50, 75, 100")
	}
	if cfg.Priority < 0 || cfg.Priority > 5 {
		return cfg, fmt.Errorf("--priority must be between 0 and 5")
	}
	switch cfg.PublishType {
	case "", "MANUAL", "INSTANTLY", "DELAYED":
	default:
		return cfg, fmt.Errorf("--publish-type must be MANUAL, INSTANTLY or DELAYED")
	}
	if cfg.PublishDate != "" && cfg.PublishType != "DELAYED" {
		return cfg, fmt.Errorf("--publish-date only makes sense with --publish-type DELAYED")
	}
	if cfg.PublishType == "DELAYED" && cfg.PublishDate == "" {
		return cfg, fmt.Errorf("--publish-type DELAYED requires --publish-date")
	}
	for _, p := range []string{cfg.APK, cfg.HMSAPK, cfg.AAB} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return cfg, fmt.Errorf("file not found: %s", p)
		}
	}
	return cfg, nil
}

// resolveWhatsNew returns the release notes from the inline flag or the file.
func resolveWhatsNew(cfg publishConfig) (string, error) {
	if cfg.WhatsNewFile == "" {
		return cfg.WhatsNew, nil
	}
	raw, err := os.ReadFile(cfg.WhatsNewFile)
	if err != nil {
		return "", fmt.Errorf("read --whats-new-file: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// runPublish drives the release pipeline: draft -> upload -> commit.
func runPublish(ctx context.Context, c *rustore.Client, cfg publishConfig, out io.Writer) error {
	whatsNew, err := resolveWhatsNew(cfg)
	if err != nil {
		return err
	}
	params := rustore.DraftParams{
		WhatsNew:        whatsNew,
		ModerInfo:       cfg.ModerInfo,
		PublishType:     cfg.PublishType,
		PublishDateTime: cfg.PublishDate,
		PartialValue:    cfg.Partial,
	}

	versionID, err := c.CreateDraft(ctx, cfg.Package, params)
	if err != nil {
		existingID, exists := rustore.ExistingDraftID(err)
		if !exists {
			return fmt.Errorf("create draft: %w", err)
		}
		if !cfg.ReplaceDraft {
			return fmt.Errorf("draft version %d already exists for %s; inspect it with `droidship rustore versions %s --id %d`, delete it with `droidship rustore draft delete %s %d`, or pass --replace-draft",
				existingID, cfg.Package, cfg.Package, existingID, cfg.Package, existingID)
		}
		fmt.Fprintf(out, "draft %d already exists — deleting (--replace-draft)\n", existingID)
		if err := c.DeleteDraft(ctx, cfg.Package, existingID); err != nil {
			return fmt.Errorf("delete existing draft %d: %w", existingID, err)
		}
		versionID, err = c.CreateDraft(ctx, cfg.Package, params)
		if err != nil {
			return fmt.Errorf("create draft after replace: %w", err)
		}
	}
	fmt.Fprintf(out, "draft created: version id %d\n", versionID)

	if cfg.APK != "" {
		fmt.Fprintf(out, "uploading APK %s…\n", cfg.APK)
		if err := c.UploadAPK(ctx, cfg.Package, versionID, cfg.APK, true, ""); err != nil {
			return fmt.Errorf("upload apk: %w", err)
		}
	}
	if cfg.HMSAPK != "" {
		fmt.Fprintf(out, "uploading HMS APK %s…\n", cfg.HMSAPK)
		if err := c.UploadAPK(ctx, cfg.Package, versionID, cfg.HMSAPK, false, "HMS"); err != nil {
			return fmt.Errorf("upload hms apk: %w", err)
		}
	}
	if cfg.AAB != "" {
		fmt.Fprintf(out, "uploading AAB %s…\n", cfg.AAB)
		if err := c.UploadAAB(ctx, cfg.Package, versionID, cfg.AAB); err != nil {
			return fmt.Errorf("upload aab: %w", err)
		}
	}

	if cfg.SkipCommit {
		fmt.Fprintf(out, "done: draft %d uploaded, NOT submitted (--skip-commit). Submit later or delete with `droidship rustore draft delete %s %d`\n",
			versionID, cfg.Package, versionID)
		return nil
	}
	if err := c.Commit(ctx, cfg.Package, versionID, cfg.Priority); err != nil {
		return fmt.Errorf("submit for moderation: %w", err)
	}
	fmt.Fprintf(out, "version %d submitted for moderation\n", versionID)
	return nil
}
