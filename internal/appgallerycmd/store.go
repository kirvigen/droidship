package appgallerycmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kirvigen/droidship/internal/appgallery"
	"github.com/kirvigen/droidship/internal/config"
	"github.com/kirvigen/droidship/internal/store"
)

// agTime is the timestamp layout AppGallery Connect expects.
const agTime = "2006-01-02T15:04:05-0700"

// phasedDays is how long the phased window lasts when the unified commands
// start one; the window is AppGallery's, the other stores have none.
const phasedDays = 7

// publishWait is how long the unified commands wait for AppGallery to process a package.
const publishWait = 20 * time.Minute

// Store is the Huawei AppGallery adapter for droidship's unified commands.
type Store struct {
	client *appgallery.Client
	creds  config.AppGalleryCreds
	region string
	ids    map[string]string
}

// NewStore opens AppGallery Connect with the configured API client.
func NewStore() (*Store, error) {
	creds, err := config.AppGallery()
	if err != nil {
		return nil, err
	}
	domain, err := appgallery.DomainForRegion(creds.Region)
	if err != nil {
		return nil, err
	}
	c := appgallery.New(creds.ClientID, creds.ClientSecret)
	c.BaseURL = domain
	region := creds.Region
	if region == "" {
		region = "global"
	}
	return &Store{client: c, creds: creds, region: region, ids: map[string]string{}}, nil
}

// Configured reports whether AppGallery credentials can be found.
func Configured() bool {
	_, err := config.AppGallery()
	return err == nil
}

func (s *Store) Name() string    { return store.AppGallery }
func (s *Store) ReplyLimit() int { return 0 }

// appID maps a package name to the AppGallery app id. A configured app id
// applies to its configured package, or to any package when none is set.
func (s *Store) appID(ctx context.Context, pkg string) (string, error) {
	if s.creds.AppID != "" && (s.creds.Package == "" || s.creds.Package == pkg) {
		return s.creds.AppID, nil
	}
	if id, ok := s.ids[pkg]; ok {
		return id, nil
	}
	id, err := s.client.AppIDByPackage(ctx, pkg)
	if err != nil {
		return "", err
	}
	s.ids[pkg] = id
	return id, nil
}

func (s *Store) Auth(ctx context.Context) (store.AuthInfo, error) {
	if _, err := s.client.Token(ctx); err != nil {
		return store.AuthInfo{}, err
	}
	return store.AuthInfo{Identity: "client " + s.creds.ClientID + " @ " + s.region, Source: s.creds.Source}, nil
}

func (s *Store) Status(ctx context.Context, pkg string) ([]store.Release, error) {
	id, err := s.appID(ctx, pkg)
	if err != nil {
		return nil, err
	}
	info, err := s.client.AppInfo(ctx, id, appgallery.ReleaseFull)
	if err != nil {
		return nil, err
	}
	rows := []store.Release{{Track: "latest", Version: fmt.Sprintf("%s (%s)", info.VersionNumber, info.VersionCode),
		Status: info.StateLabel(), ID: info.VersionID.String()}}
	if info.OnShelfVersionNumber != "" && info.OnShelfVersionCode != info.VersionCode {
		rows = append(rows, store.Release{Track: "live",
			Version: fmt.Sprintf("%s (%s)", info.OnShelfVersionNumber, info.OnShelfVersionCode), Status: "on shelf"})
	}
	return rows, nil
}

func (s *Store) Publish(ctx context.Context, req store.PublishRequest, log io.Writer) (store.PublishResult, error) {
	id, err := s.appID(ctx, req.Package)
	if err != nil {
		return store.PublishResult{}, err
	}
	rel := releaseOpts{}
	if req.GoLive {
		rel = phasedOpts(req.Percent, time.Now())
	} else if req.Percent > 0 {
		fmt.Fprintln(log, "appgallery: --percent applies at release; the build is only uploaded")
	}
	cfg := publishConfig{APK: req.APK, AAB: req.AAB, WhatsNew: req.Notes, Lang: langOr(req.Lang),
		UploadOnly: !req.GoLive, Wait: publishWait, Release: rel}
	if err := runPublish(ctx, s.client, id, cfg, log); err != nil {
		return store.PublishResult{}, err
	}
	if req.GoLive {
		return store.PublishResult{State: "submitted for review, goes live once approved"}, nil
	}
	return store.PublishResult{State: "uploaded, not submitted",
		Next: "droidship release " + req.Package + " --store appgallery"}, nil
}

func (s *Store) Release(ctx context.Context, req store.ReleaseRequest, log io.Writer) error {
	if req.Version != "" {
		return errors.New("AppGallery submits the version currently in the console; drop --version")
	}
	id, err := s.appID(ctx, req.Package)
	if err != nil {
		return err
	}
	rel := phasedOpts(req.Percent, time.Now())
	params, err := rel.params()
	if err != nil {
		return err
	}
	return submitWithRetry(ctx, s.client, id, params, publishWait, log)
}

func (s *Store) Rollout(ctx context.Context, req store.RolloutRequest, log io.Writer) error {
	if req.Version != "" {
		return errors.New("AppGallery has one phased release per app; drop --version")
	}
	id, err := s.appID(ctx, req.Package)
	if err != nil {
		return err
	}
	u := appgallery.PhasedUpdate{Percent: req.Percent, Full: req.Percent >= 100}
	if err := s.client.UpdatePhasedRelease(ctx, id, u); err != nil {
		return err
	}
	if u.Full {
		fmt.Fprintln(log, "appgallery: phased release turned into a full release")
	} else {
		fmt.Fprintf(log, "appgallery: phased release set to %g%%\n", req.Percent)
	}
	return nil
}

func (s *Store) Notes(ctx context.Context, pkg, lang, text string) error {
	id, err := s.appID(ctx, pkg)
	if err != nil {
		return err
	}
	return s.client.UpdateLanguageInfo(ctx, id, appgallery.LanguageInfo{Lang: langOr(lang), NewFeatures: text})
}

func (s *Store) Reviews(ctx context.Context, q store.ReviewsQuery) ([]store.Review, error) {
	id, err := s.appID(ctx, q.Package)
	if err != nil {
		return nil, err
	}
	days := q.Days
	if days <= 0 || days > 180 {
		days = 180 // AppGallery serves at most six months per query
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	params := appgallery.ReviewsParams{AppID: id, Begin: time.Now().AddDate(0, 0, -days), End: time.Now(),
		Countries: []string{"RU"}, Page: 1, Limit: limit}
	if q.Stars > 0 {
		params.Ratings = []int{q.Stars}
	}
	if q.Unanswered {
		params.ReplyStates = []int{0}
	}
	page, err := s.client.Reviews(ctx, params)
	if err != nil {
		return nil, err
	}
	var out []store.Review
	for _, r := range page.Reviews {
		if (q.Stars > 0 && int(r.Rating.Int()) != q.Stars) || (q.Unanswered && r.Answered()) {
			continue
		}
		out = append(out, store.Review{Store: store.AppGallery, ID: r.ReviewID.String(), Stars: int(r.Rating.Int()),
			Text: strings.TrimSpace(r.Content), Time: r.Time(), AppVersion: r.VersionName, Replied: r.Answered()})
	}
	return out, nil
}

func (s *Store) Reply(ctx context.Context, pkg, reviewID, text string) error {
	id, err := s.appID(ctx, pkg)
	if err != nil {
		return err
	}
	return s.client.Reply(ctx, appgallery.ReplyParams{AppID: id, ReviewID: reviewID, Content: text,
		Lang: agLang(defaultLang), CountryCode: "RU"})
}

func (s *Store) Listing(ctx context.Context, pkg, lang string) (store.Listing, error) {
	id, err := s.appID(ctx, pkg)
	if err != nil {
		return store.Listing{}, err
	}
	langs, err := s.client.Languages(ctx, id)
	if err != nil {
		return store.Listing{}, err
	}
	for _, l := range langs {
		if strings.EqualFold(l.Lang, lang) {
			return store.Listing{Lang: l.Lang, Title: l.AppName, Short: l.BriefInfo, Full: l.AppDesc}, nil
		}
	}
	return store.Listing{}, fmt.Errorf("no %s listing in AppGallery", lang)
}

// phasedOpts is a phased window of phasedDays starting now, or a full release
// for 0 and 100 percent.
func phasedOpts(percent float64, now time.Time) releaseOpts {
	if percent <= 0 || percent >= 100 {
		return releaseOpts{}
	}
	return releaseOpts{Phased: percent, PhasedFrom: now.Format(agTime),
		PhasedTo: now.AddDate(0, 0, phasedDays).Format(agTime)}
}

// agLang turns ru-RU into the ru_RU form the comments API wants.
func agLang(lang string) string { return strings.ReplaceAll(lang, "-", "_") }

const defaultLang = "ru-RU"

func langOr(lang string) string {
	if lang == "" {
		return defaultLang
	}
	return lang
}
