package appgallerycmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kirvigen/droidship/internal/appgallery"
)

// sleep is a variable so tests can run the waiting loops instantly.
var sleep = time.Sleep

// pollInterval is how often the AAB compilation status is re-checked.
const pollInterval = 30 * time.Second

// globalOpts are the flags every command understands.
type globalOpts struct {
	AppID   string
	Package string
	Region  string
}

// bind registers the shared flags on a command's flag set.
func (g *globalOpts) bind(fs *flag.FlagSet) {
	fs.StringVar(&g.AppID, "app-id", "", "AppGallery app id (default $HUAWEI_APP_ID)")
	fs.StringVar(&g.Package, "package", "", "package name to resolve into an app id (default $HUAWEI_PACKAGE)")
	fs.StringVar(&g.Region, "region", "", "API region: global | ru | eu | sg (default $HUAWEI_REGION)")
}

// newFlagSet builds a flag set that already carries the shared flags.
func newFlagSet(name string) (*flag.FlagSet, *globalOpts) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	g := &globalOpts{}
	g.bind(fs)
	return fs, g
}

func cmdAuth(ctx context.Context, args []string) error {
	fs, g := newFlagSet("auth")
	probe := fs.Bool("probe", false, "try every region and report which one accepts the credentials")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *probe {
		return probeRegions(ctx, g, os.Stdout)
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	if _, err := s.client.Token(ctx); err != nil {
		return fmt.Errorf("%w\n(if the account is not in this region, try --probe)", err)
	}
	fmt.Printf("auth OK (%s)\n", s.client.BaseURL)
	return nil
}

// probeRegions reports which API hosts accept these credentials. An account
// lives in exactly one region and the others answer with an error.
func probeRegions(ctx context.Context, g *globalOpts, out io.Writer) error {
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	var ok int
	for _, name := range appgallery.RegionOrder {
		domain := appgallery.Regions[name]
		probe := appgallery.New(creds.ClientID, creds.ClientSecret)
		probe.BaseURL = domain
		if _, err := probe.Token(ctx); err != nil {
			fmt.Fprintf(out, "%-7s %s — %v\n", name, domain, err)
			continue
		}
		ok++
		fmt.Fprintf(out, "%-7s %s — OK\n", name, domain)
	}
	if ok == 0 {
		return fmt.Errorf("no region accepted these credentials")
	}
	return nil
}

func cmdApps(ctx context.Context, args []string) error {
	fs, g := newFlagSet("apps")
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("package name is required: droidship appgallery apps <package>")
	}
	pkg := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	id, err := s.client.AppIDByPackage(ctx, pkg)
	if err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func cmdInfo(ctx context.Context, args []string) error {
	fs, g := newFlagSet("info")
	asJSON := fs.Bool("json", false, "print the raw appInfo payload")
	phased := fs.Bool("phased", false, "ask about the phased release instead of the full one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	releaseType := appgallery.ReleaseFull
	if *phased {
		releaseType = appgallery.ReleasePhased
	}
	info, err := s.client.AppInfo(ctx, appID, releaseType)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(info.Raw)
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintf(w, "app id\t%s\n", appID)
	fmt.Fprintf(w, "state\t%s\n", info.StateLabel())
	fmt.Fprintf(w, "version\t%s (%s)\n", info.VersionNumber, info.VersionCode)
	if info.OnShelfVersionNumber != "" {
		fmt.Fprintf(w, "on shelf\t%s (%s)\n", info.OnShelfVersionNumber, info.OnShelfVersionCode)
	}
	if info.UpdateTime != "" {
		fmt.Fprintf(w, "updated\t%s\n", info.UpdateTime)
	}
	if info.DefaultLang != "" {
		fmt.Fprintf(w, "default lang\t%s\n", info.DefaultLang)
	}
	return w.Flush()
}

// releaseOpts are the submission flags shared by publish and submit.
type releaseOpts struct {
	Remark      string
	ReleaseTime string
	Phased      float64
	PhasedFrom  string
	PhasedTo    string
	PhasedNote  string
}

func (r *releaseOpts) bind(fs *flag.FlagSet) {
	fs.StringVar(&r.Remark, "remark", "", "note for the reviewer, 10 to 300 characters")
	fs.StringVar(&r.ReleaseTime, "release-time", "", "schedule the release, e.g. 2026-09-05T10:00:00+0300")
	fs.Float64Var(&r.Phased, "phased", 0, "phased rollout percent, e.g. 10")
	fs.StringVar(&r.PhasedFrom, "phased-from", "", "phased rollout start time")
	fs.StringVar(&r.PhasedTo, "phased-to", "", "phased rollout end time")
	fs.StringVar(&r.PhasedNote, "phased-note", "", "phased rollout description")
}

// params validates the flags and turns them into API parameters.
func (r *releaseOpts) params() (appgallery.SubmitParams, error) {
	var p appgallery.SubmitParams
	if err := appgallery.ValidateRemark(r.Remark); err != nil {
		return p, err
	}
	p.Remark = r.Remark
	p.ReleaseTime = r.ReleaseTime

	if r.Phased == 0 {
		if r.PhasedFrom != "" || r.PhasedTo != "" || r.PhasedNote != "" {
			return p, fmt.Errorf("--phased-from/--phased-to/--phased-note need --phased PCT")
		}
		return p, nil
	}
	if r.Phased <= 0 || r.Phased > 100 {
		return p, fmt.Errorf("--phased must be between 0 and 100")
	}
	if r.PhasedFrom == "" || r.PhasedTo == "" {
		return p, fmt.Errorf("--phased needs --phased-from and --phased-to")
	}
	if r.ReleaseTime != "" {
		return p, fmt.Errorf("--release-time and --phased cannot be combined: a phased rollout carries its own window")
	}
	note := r.PhasedNote
	if note == "" {
		note = fmt.Sprintf("Rollout to %.2f%% from %s to %s", r.Phased, r.PhasedFrom, r.PhasedTo)
	}
	p.Phased = &appgallery.PhasedRelease{
		StartTime:   r.PhasedFrom,
		EndTime:     r.PhasedTo,
		Percent:     fmt.Sprintf("%.2f", r.Phased),
		Description: note,
	}
	return p, nil
}

// publishConfig is everything the publish command needs.
type publishConfig struct {
	APK          string
	AAB          string
	WhatsNew     string
	WhatsNewFile string
	Lang         string
	UploadOnly   bool
	Wait         time.Duration
	Release      releaseOpts
}

// parsePublishArgs parses `droidship appgallery publish [flags]`.
func parsePublishArgs(args []string) (publishConfig, *globalOpts, error) {
	var cfg publishConfig
	fs, g := newFlagSet("publish")
	fs.StringVar(&cfg.APK, "apk", "", "path to the .apk to publish")
	fs.StringVar(&cfg.AAB, "aab", "", "path to the .aab to publish")
	fs.StringVar(&cfg.WhatsNew, "whats-new", "", "release notes for --lang")
	fs.StringVar(&cfg.WhatsNewFile, "whats-new-file", "", "read the release notes from a file")
	fs.StringVar(&cfg.Lang, "lang", "ru-RU", "language of the release notes, e.g. ru-RU")
	fs.BoolVar(&cfg.UploadOnly, "upload-only", false, "upload and attach the package, but do not submit for review")
	fs.DurationVar(&cfg.Wait, "wait", 20*time.Minute, "how long to wait for AppGallery to process the package")
	cfg.Release.bind(fs)
	if err := fs.Parse(args); err != nil {
		return cfg, g, err
	}

	if (cfg.APK == "") == (cfg.AAB == "") {
		return cfg, g, fmt.Errorf("provide exactly one of --apk or --aab")
	}
	if cfg.WhatsNew != "" && cfg.WhatsNewFile != "" {
		return cfg, g, fmt.Errorf("use either --whats-new or --whats-new-file, not both")
	}
	path := cfg.APK
	if path == "" {
		path = cfg.AAB
	}
	if _, err := os.Stat(path); err != nil {
		return cfg, g, fmt.Errorf("file not found: %s", path)
	}
	if _, err := cfg.Release.params(); err != nil {
		return cfg, g, err
	}
	return cfg, g, nil
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

func cmdPublish(ctx context.Context, args []string) error {
	cfg, g, err := parsePublishArgs(args)
	if err != nil {
		return err
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	return runPublish(ctx, s.client, appID, cfg, os.Stdout)
}

// runPublish drives the release pipeline: upload -> attach -> notes -> submit.
func runPublish(ctx context.Context, c *appgallery.Client, appID string, cfg publishConfig, out io.Writer) error {
	params, err := cfg.Release.params()
	if err != nil {
		return err
	}
	whatsNew, err := resolveWhatsNew(cfg)
	if err != nil {
		return err
	}
	path := cfg.APK
	isAAB := false
	if path == "" {
		path, isAAB = cfg.AAB, true
	}

	fmt.Fprintf(out, "app %s\n", appID)
	if info, err := os.Stat(path); err == nil {
		fmt.Fprintf(out, "uploading %s (%s)…\n", info.Name(), humanSize(info.Size()))
	}
	file, err := c.UploadPackage(ctx, appID, path)
	if err != nil {
		return err
	}

	releaseType := appgallery.ReleaseFull
	if params.Phased != nil {
		releaseType = appgallery.ReleasePhased
	}
	pkgVersions, err := c.UpdateAppFileInfo(ctx, appID, releaseType, appgallery.FileTypePackage, "", []appgallery.UploadedFile{file})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "attached %s", file.FileName)
	if len(pkgVersions) > 0 {
		fmt.Fprintf(out, " (package version %s)", strings.Join(pkgVersions, ", "))
	}
	fmt.Fprintln(out)

	if isAAB && len(pkgVersions) > 0 {
		if err := waitForAAB(ctx, c, appID, pkgVersions, cfg.Wait, out); err != nil {
			return err
		}
	}

	if whatsNew != "" {
		if err := c.UpdateLanguageInfo(ctx, appID, appgallery.LanguageInfo{Lang: cfg.Lang, NewFeatures: whatsNew}); err != nil {
			return err
		}
		fmt.Fprintf(out, "release notes set for %s\n", cfg.Lang)
	}

	if cfg.UploadOnly {
		fmt.Fprintln(out, "not submitted (--upload-only)")
		return nil
	}
	return submitWithRetry(ctx, c, appID, params, cfg.Wait, out)
}

// waitForAAB blocks until AppGallery finishes compiling the uploaded bundle.
func waitForAAB(ctx context.Context, c *appgallery.Client, appID string, pkgVersions []string, timeout time.Duration, out io.Writer) error {
	deadline := time.Now().Add(timeout)
	for {
		status, err := c.AABCompileStatus(ctx, appID, pkgVersions)
		if err != nil {
			return err
		}
		switch status {
		case appgallery.AABCompileSuccess:
			fmt.Fprintln(out, "aab compiled")
			return nil
		case appgallery.AABCompileProcessing:
			if time.Now().After(deadline) {
				return fmt.Errorf("the aab is still compiling after %s; check AppGallery Connect or raise --wait", timeout)
			}
			fmt.Fprintf(out, "aab is compiling, waiting %s…\n", pollInterval)
			sleep(pollInterval)
		default:
			return fmt.Errorf("aab compilation failed (status %d)", status)
		}
	}
}

// submitWithRetry sends the version for review, retrying while AppGallery is
// still processing the freshly uploaded package.
func submitWithRetry(ctx context.Context, c *appgallery.Client, appID string, params appgallery.SubmitParams, timeout time.Duration, out io.Writer) error {
	deadline := time.Now().Add(timeout)
	for {
		err := c.Submit(ctx, appID, params)
		if err == nil {
			if params.Phased != nil {
				fmt.Fprintf(out, "submitted for review, phased rollout %s%%\n", params.Phased.Percent)
			} else {
				fmt.Fprintln(out, "submitted for review")
			}
			return nil
		}
		if !appgallery.IsProcessing(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w (still processing after %s; raise --wait or submit later with `droidship appgallery submit`)", err, timeout)
		}
		fmt.Fprintf(out, "package is still processing, retrying in %s…\n", pollInterval)
		sleep(pollInterval)
	}
}

func cmdSubmit(ctx context.Context, args []string) error {
	fs, g := newFlagSet("submit")
	var rel releaseOpts
	rel.bind(fs)
	wait := fs.Duration("wait", 20*time.Minute, "how long to wait while the package is still processing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	params, err := rel.params()
	if err != nil {
		return err
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	return submitWithRetry(ctx, s.client, appID, params, *wait, os.Stdout)
}

func cmdWithdraw(ctx context.Context, args []string) error {
	fs, g := newFlagSet("withdraw")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	if err := s.client.Withdraw(ctx, appID); err != nil {
		return err
	}
	fmt.Printf("app %s withdrawn from review\n", appID)
	return nil
}

func cmdNotes(ctx context.Context, args []string) error {
	fs, g := newFlagSet("notes")
	lang := fs.String("lang", "ru-RU", "language of the listing, e.g. ru-RU")
	whatsNew := fs.String("whats-new", "", "release notes")
	whatsNewFile := fs.String("whats-new-file", "", "read the release notes from a file")
	appName := fs.String("app-name", "", "app name in the store listing")
	briefInfo := fs.String("brief", "", "short description")
	appDesc := fs.String("description", "", "full description")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *whatsNew != "" && *whatsNewFile != "" {
		return fmt.Errorf("use either --whats-new or --whats-new-file, not both")
	}
	notes, err := resolveWhatsNew(publishConfig{WhatsNew: *whatsNew, WhatsNewFile: *whatsNewFile})
	if err != nil {
		return err
	}
	if notes == "" && *appName == "" && *briefInfo == "" && *appDesc == "" {
		return fmt.Errorf("nothing to update: pass --whats-new, --whats-new-file, --app-name, --brief or --description")
	}
	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	info := appgallery.LanguageInfo{
		Lang:        *lang,
		AppName:     *appName,
		AppDesc:     *appDesc,
		BriefInfo:   *briefInfo,
		NewFeatures: notes,
	}
	if err := s.client.UpdateLanguageInfo(ctx, appID, info); err != nil {
		return err
	}
	fmt.Printf("listing updated for %s\n", *lang)
	return nil
}

func cmdReviews(ctx context.Context, args []string) error {
	fs, g := newFlagSet("reviews")
	days := fs.Int("days", 7, "how many days back to read")
	since := fs.String("since", "", "start of the window, 2026-08-01 or 2026-08-01T10:00:00+03:00")
	until := fs.String("until", "", "end of the window (default: now)")
	countries := fs.String("country", "RU", "comma separated country codes")
	ratings := fs.String("rating", "", "comma separated ratings, e.g. 1,2")
	unanswered := fs.Bool("unanswered", false, "only comments without a developer reply")
	page := fs.Int("page", 1, "page number")
	limit := fs.Int("limit", 20, "comments per page, max 100")
	asJSON := fs.Bool("json", false, "print the raw comments")
	if err := fs.Parse(args); err != nil {
		return err
	}

	begin, end, err := timeWindow(*since, *until, *days)
	if err != nil {
		return err
	}
	params := appgallery.ReviewsParams{
		Begin:     begin,
		End:       end,
		Countries: splitList(*countries),
		Page:      *page,
		Limit:     *limit,
	}
	if *ratings != "" {
		values, err := parseInts(*ratings)
		if err != nil {
			return fmt.Errorf("--rating: %w", err)
		}
		params.Ratings = values
	}
	if *unanswered {
		params.ReplyStates = []int{0}
	}

	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	params.AppID = appID

	res, err := s.client.Reviews(ctx, params)
	if err != nil {
		return err
	}
	if *asJSON {
		if res.Reviews == nil {
			return printJSON([]appgallery.Review{})
		}
		return printJSON(res.Reviews)
	}
	if len(res.Reviews) == 0 {
		fmt.Println("no comments in this window")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tRATING\tVERSION\tCOUNTRY\tREPLIED\tID\tCOMMENT")
	for _, r := range res.Reviews {
		replied := "no"
		if r.Answered() {
			replied = "yes"
		}
		date := ""
		if t := r.Time(); !t.IsZero() {
			date = t.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			date, r.Rating.Int(), r.VersionName, r.CountryCode, replied, r.ReviewID, oneLine(r.Content, 60))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("%d of %d comments", len(res.Reviews), res.Total)
	if res.HasNext {
		fmt.Printf(", more on page %d", *page+1)
	}
	fmt.Println()
	return nil
}

func cmdReply(ctx context.Context, args []string) error {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return fmt.Errorf("usage: droidship appgallery reply <reviewId> <text> [flags]")
	}
	reviewID, text := args[0], args[1]

	fs, g := newFlagSet("reply")
	lang := fs.String("lang", "ru_RU", "language of the comment, e.g. ru_RU")
	country := fs.String("country", "RU", "country code of the comment, e.g. RU")
	toReply := fs.String("to-reply-id", "", "answer a specific user reply in the thread")
	updateReply := fs.String("update-reply-id", "", "edit a reply that is already published")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}

	s, err := newSession(g)
	if err != nil {
		return err
	}
	appID, err := s.appID(ctx)
	if err != nil {
		return err
	}
	err = s.client.Reply(ctx, appgallery.ReplyParams{
		AppID:         appID,
		ReviewID:      reviewID,
		Content:       text,
		Lang:          *lang,
		CountryCode:   *country,
		ToReplyID:     *toReply,
		UpdateReplyID: *updateReply,
	})
	if err != nil {
		return err
	}
	fmt.Printf("replied to %s\n", reviewID)
	return nil
}

// timeWindow resolves --since/--until/--days into a concrete interval.
func timeWindow(since, until string, days int) (time.Time, time.Time, error) {
	end := time.Now()
	if until != "" {
		parsed, err := parseTime(until)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--until: %w", err)
		}
		end = parsed
	}
	begin := end.AddDate(0, 0, -days)
	if since != "" {
		parsed, err := parseTime(since)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--since: %w", err)
		}
		begin = parsed
	}
	if !begin.Before(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("the start of the window must come before its end")
	}
	// AppGallery refuses windows longer than six months.
	if end.Sub(begin) > 183*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("the window may not exceed six months")
	}
	return begin, end, nil
}

// parseTime accepts a date or a full timestamp, in local time when no zone is given.
func parseTime(value string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read the time %q: use 2026-08-01 or 2026-08-01T10:00:00+03:00", value)
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseInts(value string) ([]int, error) {
	var out []int
	for _, part := range splitList(value) {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", part)
		}
		out = append(out, n)
	}
	return out, nil
}
