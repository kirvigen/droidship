// Package appgallerycmd is droidship's Huawei AppGallery namespace, formerly the
// hstore CLI: publish Android app releases to AppGallery and answer reviews.
package appgallerycmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kirvigen/droidship/internal/appgallery"
	"github.com/kirvigen/droidship/internal/version"
)

const usage = `droidship appgallery — Huawei AppGallery Connect CLI

Usage:
  droidship appgallery auth [--probe]                        check the credentials
  droidship appgallery apps <package>                        resolve a package name to an app id
  droidship appgallery info [--phased] [--json]              current version and release state
  droidship appgallery publish (--apk F | --aab F)
         [--whats-new S | --whats-new-file F] [--lang ru-RU]
         [--remark S] [--release-time T]
         [--phased PCT --phased-from T --phased-to T [--phased-note S]]
         [--upload-only] [--wait D]
  droidship appgallery submit [--remark S] [--release-time T]
         [--phased PCT --phased-from T --phased-to T [--phased-note S]]
  droidship appgallery withdraw                              take the app back from review
  droidship appgallery notes --lang ru-RU (--whats-new S | --whats-new-file F)
  droidship appgallery reviews [--days N | --since D [--until D]] [--country RU]
         [--rating 1,2] [--unanswered] [--page N] [--limit N] [--json]
  droidship appgallery reply <reviewId> <text> [--lang ru_RU] [--country RU]
         [--to-reply-id ID | --update-reply-id ID]
  droidship appgallery version

Every command also takes --app-id, --package and --region.

Credentials (AppGallery Connect -> Users and permissions -> API key -> Connect API).
Every variable also answers to the DROIDSHIP_APPGALLERY_ and HUAWEI_ prefixes:
  HSTORE_CLIENT_ID      client id of the API client
  HSTORE_CLIENT_SECRET  client secret of the API client
  HSTORE_APP_ID         app id, e.g. 118236677 (or pass --app-id)
  HSTORE_PACKAGE        package name, resolved to an app id (or pass --package)
  HSTORE_REGION         global (default) | ru | eu | sg, or a full https:// host

Instead of the environment, droidship reads ~/.config/droidship/config.json
(or the old ~/.config/hstore/credentials.json):
  {"appgallery": {"client_id": "…", "client_secret": "…", "app_id": "118236677", "region": "global"}}

Times are AppGallery timestamps, e.g. 2026-09-05T10:00:00+0300.
`

// Run executes the store namespace with the arguments that follow the store
// name, e.g. ["upload", "com.example", "--aab", "app.aab"], and returns the exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "--version":
		fmt.Println("droidship", version.Version)
		return 0
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	case "auth", "apps", "info", "publish", "submit", "withdraw", "notes", "reviews", "reply":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	ctx := context.Background()
	err := dispatch(ctx, cmd, rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func dispatch(ctx context.Context, cmd string, args []string) error {
	switch cmd {
	case "auth":
		return cmdAuth(ctx, args)
	case "apps":
		return cmdApps(ctx, args)
	case "info":
		return cmdInfo(ctx, args)
	case "publish":
		return cmdPublish(ctx, args)
	case "submit":
		return cmdSubmit(ctx, args)
	case "withdraw":
		return cmdWithdraw(ctx, args)
	case "notes":
		return cmdNotes(ctx, args)
	case "reviews":
		return cmdReviews(ctx, args)
	case "reply":
		return cmdReply(ctx, args)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

// session carries the resolved credentials and a client built from them.
type session struct {
	client *appgallery.Client
	creds  credentials
	opts   *globalOpts
}

// newSession resolves the credentials and points a client at the right region.
func newSession(g *globalOpts) (*session, error) {
	creds, err := loadCredentials()
	if err != nil {
		return nil, err
	}
	region := g.Region
	if region == "" {
		region = creds.Region
	}
	domain, err := appgallery.DomainForRegion(region)
	if err != nil {
		return nil, err
	}
	c := appgallery.New(creds.ClientID, creds.ClientSecret)
	c.BaseURL = domain
	return &session{client: c, creds: creds, opts: g}, nil
}

// appID picks the app id from the flags, the credentials, or by looking up the
// package name.
func (s *session) appID(ctx context.Context) (string, error) {
	if s.opts.AppID != "" {
		return s.opts.AppID, nil
	}
	if s.creds.AppID != "" {
		return s.creds.AppID, nil
	}
	pkg := s.opts.Package
	if pkg == "" {
		pkg = s.creds.Package
	}
	if pkg == "" {
		return "", fmt.Errorf("pass --app-id or --package (or set HSTORE_APP_ID / HSTORE_PACKAGE, or app_id in the credentials file)")
	}
	return s.client.AppIDByPackage(ctx, pkg)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// oneLine flattens text to a single line no longer than max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// humanSize renders a file size the way a release engineer reads it.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
