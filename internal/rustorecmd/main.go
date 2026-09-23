// Package rustorecmd is droidship's RuStore namespace, formerly the rstore CLI:
// publish Android app releases to RuStore.
package rustorecmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/kirvigen/droidship/internal/config"
	"github.com/kirvigen/droidship/internal/rustore"
	"github.com/kirvigen/droidship/internal/version"
)

const usage = `droidship rustore — RuStore Console CLI

Usage:
  droidship rustore auth                                  check the API key
  droidship rustore apps [--json]                         list applications
  droidship rustore versions <package> [--id N] [--page N] [--size N] [--json]
  droidship rustore publish <package> (--apk F [--hms-apk F] | --aab F)
         [--whats-new S | --whats-new-file F] [--moder-info S]
         [--publish-type MANUAL|INSTANTLY|DELAYED] [--publish-date T]
         [--partial 5|10|25|50|75|100] [--priority 0..5]
         [--replace-draft] [--skip-commit]
  droidship rustore release <package> <versionId>         publish a moderated MANUAL version
  droidship rustore rollout <package> <versionId> <pct>   raise the partial rollout percent
  droidship rustore draft delete <package> <versionId>    delete a draft
  droidship rustore version

Credentials (RuStore Console → Company/Developer → API RuStore):
  DROIDSHIP_RUSTORE_KEY_ID       key id                      (or RUSTORE_KEY_ID)
  DROIDSHIP_RUSTORE_PRIVATE_KEY  private key, base64 as issued (or RUSTORE_API_KEY)
or "rustore": {"key_id": "…", "private_key": "…"} in ~/.config/droidship/config.json.
`

// Run executes the store namespace with the arguments that follow the store
// name, e.g. ["upload", "com.example", "--aab", "app.aab"], and returns the exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	if cmd == "version" || cmd == "--version" {
		fmt.Println("droidship", version.String())
		return 0
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return 0
	}

	switch cmd {
	case "auth", "apps", "versions", "publish", "release", "rollout", "draft":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	client, err := newClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	ctx := context.Background()

	switch cmd {
	case "auth":
		err = cmdAuth(ctx, client)
	case "apps":
		err = cmdApps(ctx, client, rest)
	case "versions":
		err = cmdVersions(ctx, client, rest)
	case "publish":
		var cfg publishConfig
		cfg, err = parsePublishArgs(rest)
		if err == nil {
			err = runPublish(ctx, client, cfg, os.Stdout)
		}
	case "release":
		err = cmdRelease(ctx, client, rest)
	case "rollout":
		err = cmdRollout(ctx, client, rest)
	case "draft":
		err = cmdDraft(ctx, client, rest)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func newClientFromEnv() (*rustore.Client, error) {
	c, err := config.RuStore()
	if err != nil {
		return nil, err
	}
	return rustore.New(c.KeyID, c.PrivateKey)
}

func cmdAuth(ctx context.Context, c *rustore.Client) error {
	if _, err := c.Token(ctx); err != nil {
		return err
	}
	fmt.Println("auth OK")
	return nil
}

func cmdApps(ctx context.Context, c *rustore.Client, args []string) error {
	fs := flag.NewFlagSet("apps", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apps, err := c.Apps(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(apps)
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PACKAGE\tNAME\tSTATUS\tVERSION\tUPDATED")
	for _, a := range apps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s (%d)\t%s\n",
			a.PackageName, a.AppName, a.AppStatus, a.VersionName, a.VersionCode, shortDate(a.AppVerUpdatedAt))
	}
	return w.Flush()
}

func cmdVersions(ctx context.Context, c *rustore.Client, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("package name is required: droidship rustore versions <package> [flags]")
	}
	pkg := args[0]
	fs := flag.NewFlagSet("versions", flag.ContinueOnError)
	id := fs.Int64("id", 0, "fetch one version by id")
	page := fs.Int("page", 0, "page number (from 0)")
	size := fs.Int("size", 20, "versions per page (max 100)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	res, err := c.Versions(ctx, pkg, rustore.VersionsOpts{ID: *id, Page: *page, Size: *size})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(res)
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tVERSION\tCODE\tSTATUS\tTYPE\tPARTIAL\tSENT\tWHATS NEW")
	for _, v := range res.Content {
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			v.VersionID, v.VersionName, v.VersionCode, v.VersionStatus, v.PublishType,
			partialLabel(v.PartialValue), shortDate(v.SendDateForModer), oneLine(v.WhatsNew, 40))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if res.TotalPages > 1 {
		fmt.Printf("page %d/%d, total %d versions\n", res.PageNumber+1, res.TotalPages, res.TotalElements)
	}
	return nil
}

func cmdRelease(ctx context.Context, c *rustore.Client, args []string) error {
	pkg, id, err := packageAndID(args, "release")
	if err != nil {
		return err
	}
	if err := c.Publish(ctx, pkg, id); err != nil {
		return err
	}
	fmt.Printf("version %d published\n", id)
	return nil
}

func cmdRollout(ctx context.Context, c *rustore.Client, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: droidship rustore rollout <package> <versionId> <percent>")
	}
	pkg, id, err := packageAndID(args[:2], "rollout")
	if err != nil {
		return err
	}
	pct, err := strconv.Atoi(args[2])
	if err != nil || !validPartial[pct] || pct == 0 {
		return fmt.Errorf("percent must be one of 5, 10, 25, 50, 75, 100")
	}
	if err := c.UpdatePublishSettings(ctx, pkg, id, rustore.PublishSettings{PartialValue: pct}); err != nil {
		return err
	}
	fmt.Printf("version %d rollout set to %d%%\n", id, pct)
	return nil
}

func cmdDraft(ctx context.Context, c *rustore.Client, args []string) error {
	if len(args) < 1 || args[0] != "delete" {
		return fmt.Errorf("usage: droidship rustore draft delete <package> <versionId>")
	}
	pkg, id, err := packageAndID(args[1:], "draft delete")
	if err != nil {
		return err
	}
	if err := c.DeleteDraft(ctx, pkg, id); err != nil {
		return err
	}
	fmt.Printf("draft %d deleted\n", id)
	return nil
}

func packageAndID(args []string, cmd string) (string, int64, error) {
	if len(args) != 2 {
		return "", 0, fmt.Errorf("usage: droidship rustore %s <package> <versionId>", cmd)
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("versionId must be a number, got %q", args[1])
	}
	return args[0], id, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func partialLabel(v int) string {
	if v == -1 {
		return "100%"
	}
	return strconv.Itoa(v) + "%"
}

func shortDate(ts string) string {
	if len(ts) >= 16 {
		return strings.Replace(ts[:16], "T", " ", 1)
	}
	return ts
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
