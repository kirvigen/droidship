package gplaycmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kirvigen/droidship/internal/config"
	"github.com/kirvigen/droidship/internal/play"
	"github.com/kirvigen/droidship/internal/store"
)

// Store is the Google Play adapter for droidship's unified commands. It works
// on the production track; the other tracks stay under `droidship gplay`.
type Store struct {
	client *play.Client
	source string
}

// NewStore opens Google Play with the configured service account key.
func NewStore() (*Store, error) {
	path, source, err := config.GPlayKey("")
	if err != nil {
		return nil, err
	}
	c, err := play.New(path)
	if err != nil {
		return nil, err
	}
	return &Store{client: c, source: source}, nil
}

// Configured reports whether a service account key can be found.
func Configured() bool {
	_, _, err := config.GPlayKey("")
	return err == nil
}

func (s *Store) Name() string    { return store.GPlay }
func (s *Store) ReplyLimit() int { return play.MaxReplyLen }

func (s *Store) Auth(ctx context.Context) (store.AuthInfo, error) {
	if _, err := s.client.Token(ctx); err != nil {
		return store.AuthInfo{}, err
	}
	return store.AuthInfo{Identity: s.client.ServiceAccount().ClientEmail, Source: s.source}, nil
}

func (s *Store) Status(ctx context.Context, pkg string) ([]store.Release, error) {
	var tracks []play.Track
	err := withEdit(ctx, s.client, pkg, func(editID string) (err error) {
		tracks, err = s.client.Tracks(ctx, pkg, editID)
		return err
	})
	if err != nil {
		return nil, err
	}
	var rows []store.Release
	for _, t := range tracks {
		for _, r := range t.Releases {
			rows = append(rows, store.Release{
				Track: t.Track, Version: strings.Join(r.VersionCodes, ","),
				Status: r.Status, Percent: r.UserFraction * 100,
			})
		}
	}
	return rows, nil
}

func (s *Store) Publish(ctx context.Context, req store.PublishRequest, log io.Writer) (store.PublishResult, error) {
	if req.AAB == "" {
		return store.PublishResult{}, errors.New("Google Play accepts only an Android App Bundle: pass --aab")
	}
	status, fraction := play.StatusDraft, 0.0
	if req.GoLive {
		status, fraction = liveStatus(req.Percent)
	} else if req.Percent > 0 {
		fmt.Fprintln(log, "gplay: --percent applies at release; the draft is staged without one")
	}
	cfg := uploadConfig{
		Package: req.Package, AAB: req.AAB, Track: play.TrackProduction, Status: status,
		UserFraction: fraction, WhatsNew: req.Notes, Lang: langOr(req.Lang),
	}
	if err := runUpload(ctx, s.client, cfg, log); err != nil {
		return store.PublishResult{}, err
	}
	if req.GoLive {
		return store.PublishResult{State: status + " on production"}, nil
	}
	return store.PublishResult{State: "draft on production",
		Next: "droidship release " + req.Package + " --store gplay"}, nil
}

func (s *Store) Release(ctx context.Context, req store.ReleaseRequest, log io.Writer) error {
	code, err := s.pickVersion(ctx, req.Package, req.Version, play.StatusDraft)
	if err != nil {
		return err
	}
	status, fraction := liveStatus(req.Percent)
	return runRelease(ctx, s.client, releaseConfig{
		Mode: "release", Package: req.Package, Track: play.TrackProduction,
		Status: status, VersionCode: code, UserFraction: fraction, Lang: defaultLang,
	}, log)
}

func (s *Store) Rollout(ctx context.Context, req store.RolloutRequest, log io.Writer) error {
	code, err := s.pickVersion(ctx, req.Package, req.Version, play.StatusInProgress, play.StatusHalted)
	if err != nil {
		return err
	}
	status, fraction := liveStatus(req.Percent)
	return runRelease(ctx, s.client, releaseConfig{
		Mode: "rollout", Package: req.Package, Track: play.TrackProduction,
		Status: status, VersionCode: code, UserFraction: fraction, Lang: defaultLang,
	}, log)
}

func (s *Store) Notes(ctx context.Context, pkg, lang, text string) error {
	edit, err := s.client.CreateEdit(ctx, pkg)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.client.DeleteEdit(ctx, pkg, edit.ID)
		}
	}()
	track, err := s.client.Track(ctx, pkg, edit.ID, play.TrackProduction)
	if err != nil {
		return err
	}
	if len(track.Releases) == 0 {
		return errors.New("production has no release to attach notes to")
	}
	r := &track.Releases[0]
	var notes []play.ReleaseNote
	for _, n := range r.ReleaseNotes {
		if n.Language != lang {
			notes = append(notes, n)
		}
	}
	r.ReleaseNotes = append(notes, play.ReleaseNote{Language: lang, Text: text})
	if _, err := s.client.UpdateTrack(ctx, pkg, edit.ID, *track); err != nil {
		return err
	}
	if err := commitEdit(ctx, s.client, pkg, edit.ID, true, io.Discard); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) Reviews(ctx context.Context, q store.ReviewsQuery) ([]store.Review, error) {
	list, err := collectReviews(ctx, s.client, q.Package, "", q.Limit, q.Stars, q.Unanswered)
	if err != nil {
		return nil, err
	}
	since := time.Now().AddDate(0, 0, -q.Days)
	var out []store.Review
	for _, r := range list {
		u := r.User()
		if u == nil {
			continue
		}
		when := u.LastModified.Time()
		if q.Days > 0 && when.Before(since) {
			continue
		}
		rev := store.Review{Store: store.GPlay, ID: r.ReviewID, Author: r.AuthorName, Stars: r.Stars(),
			Text: strings.TrimSpace(u.Text), Time: when, AppVersion: u.AppVersionName}
		if reply := r.Reply(); reply != nil {
			rev.Replied, rev.Reply = true, reply.Text
		}
		out = append(out, rev)
	}
	return out, nil
}

func (s *Store) Reply(ctx context.Context, pkg, reviewID, text string) error {
	_, err := s.client.ReplyToReview(ctx, pkg, reviewID, text)
	return err
}

func (s *Store) Listing(ctx context.Context, pkg, lang string) (store.Listing, error) {
	var l *play.Listing
	err := withEdit(ctx, s.client, pkg, func(editID string) (err error) {
		l, err = s.client.Listing(ctx, pkg, editID, lang)
		return err
	})
	if err != nil {
		return store.Listing{}, err
	}
	return store.Listing{Lang: l.Language, Title: l.Title, Short: l.ShortDescription, Full: l.FullDescription}, nil
}

// liveStatus turns a unified percent into a Play status and user fraction.
func liveStatus(percent float64) (string, float64) {
	if percent > 0 && percent < 100 {
		return play.StatusInProgress, percent / 100
	}
	return play.StatusCompleted, 0
}

// pickVersion resolves the version code to act on: the explicit one, or the
// newest release on production in one of the given statuses.
func (s *Store) pickVersion(ctx context.Context, pkg, version string, statuses ...string) (int, error) {
	if version != "" {
		code, err := strconv.Atoi(version)
		if err != nil {
			return 0, fmt.Errorf("--version for Google Play is a version code, got %q", version)
		}
		return code, nil
	}
	var track *play.Track
	err := withEdit(ctx, s.client, pkg, func(editID string) (err error) {
		track, err = s.client.Track(ctx, pkg, editID, play.TrackProduction)
		return err
	})
	if err != nil {
		return 0, err
	}
	for _, r := range track.Releases {
		for _, st := range statuses {
			if r.Status == st && len(r.VersionCodes) > 0 {
				return strconv.Atoi(r.VersionCodes[len(r.VersionCodes)-1])
			}
		}
	}
	return 0, fmt.Errorf("no %s release on production: pass --version (see droidship status %s --store gplay)",
		strings.Join(statuses, "/"), pkg)
}

func langOr(lang string) string {
	if lang == "" {
		return defaultLang
	}
	return lang
}

func (s *Store) Plan(ctx context.Context, req store.PublishRequest) (store.PublishPlan, error) {
	if req.AAB == "" {
		return store.PublishPlan{}, errors.New("Google Play accepts only an Android App Bundle: pass --aab")
	}
	var track *play.Track
	err := withEdit(ctx, s.client, req.Package, func(editID string) (err error) {
		track, err = s.client.Track(ctx, req.Package, editID, play.TrackProduction)
		return err
	})
	if err != nil {
		return store.PublishPlan{}, err
	}
	return planFromTrack(track, req), nil
}

// planFromTrack describes what Publish would do to the production track.
func planFromTrack(track *play.Track, req store.PublishRequest) store.PublishPlan {
	p := store.PublishPlan{Live: "nothing on production"}
	for _, r := range track.Releases {
		codes := strings.Join(r.VersionCodes, ",")
		switch r.Status {
		case play.StatusDraft:
			p.Note = "draft " + codes + " is already staged; publishing replaces it"
		case play.StatusCompleted, play.StatusInProgress, play.StatusHalted:
			if p.Live == "nothing on production" {
				p.Live = codes
				if r.Status != play.StatusCompleted {
					p.Live += fmt.Sprintf(" at %g%%", r.UserFraction*100)
				}
			}
		}
	}
	file := filepath.Base(req.AAB)
	p.Action = "upload " + file + " → draft on production"
	if req.GoLive {
		who := "everyone"
		if req.Percent > 0 && req.Percent < 100 {
			who = fmt.Sprintf("%g%%", req.Percent)
		}
		p.Action = "upload " + file + " → roll out to " + who + " on production"
	}
	return p
}
