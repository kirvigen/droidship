package rustorecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kirvigen/droidship/internal/config"
	"github.com/kirvigen/droidship/internal/rustore"
	"github.com/kirvigen/droidship/internal/store"
)

// pageSize is the page the review endpoints are read with; RuStore accepts 100.
const pageSize = 100

// maxAnswerPages bounds how far back answers are read to mark reviews as replied.
const maxAnswerPages = 10

// Store is the RuStore adapter for droidship's unified commands.
type Store struct {
	client *rustore.Client
	creds  config.RuStoreCreds
}

// NewStore opens RuStore with the configured API key.
func NewStore() (*Store, error) {
	creds, err := config.RuStore()
	if err != nil {
		return nil, err
	}
	c, err := rustore.New(creds.KeyID, creds.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &Store{client: c, creds: creds}, nil
}

// Configured reports whether a RuStore key can be found.
func Configured() bool {
	_, err := config.RuStore()
	return err == nil
}

func (s *Store) Name() string    { return store.RuStore }
func (s *Store) ReplyLimit() int { return rustore.MaxAnswerLen }

func (s *Store) Auth(ctx context.Context) (store.AuthInfo, error) {
	if _, err := s.client.Token(ctx); err != nil {
		return store.AuthInfo{}, err
	}
	return store.AuthInfo{Identity: "key " + s.creds.KeyID, Source: s.creds.Source}, nil
}

func (s *Store) Status(ctx context.Context, pkg string) ([]store.Release, error) {
	page, err := s.client.Versions(ctx, pkg, rustore.VersionsOpts{Size: 5})
	if err != nil {
		return nil, err
	}
	var rows []store.Release
	for _, v := range page.Content {
		pct := float64(v.PartialValue)
		if v.PartialValue == -1 {
			pct = 100
		}
		rows = append(rows, store.Release{Track: strings.ToLower(v.PublishType),
			Version: fmt.Sprintf("%s (%d)", v.VersionName, v.VersionCode),
			Status:  v.VersionStatus, Percent: pct, ID: strconv.FormatInt(v.VersionID, 10)})
	}
	return rows, nil
}

func (s *Store) Publish(ctx context.Context, req store.PublishRequest, log io.Writer) (store.PublishResult, error) {
	pct, err := partial(req.Percent)
	if err != nil {
		return store.PublishResult{}, err
	}
	publishType := "MANUAL"
	if req.GoLive {
		publishType = "INSTANTLY"
	}
	cfg := publishConfig{Package: req.Package, APK: req.APK, AAB: req.AAB,
		WhatsNew: req.Notes, PublishType: publishType, Partial: pct}
	if err := runPublish(ctx, s.client, cfg, log); err != nil {
		return store.PublishResult{}, err
	}
	if req.GoLive {
		return store.PublishResult{State: "sent to moderation, goes live once approved"}, nil
	}
	return store.PublishResult{State: "sent to moderation, released by hand",
		Next: "droidship release " + req.Package + " --store rustore  # once status is READY_FOR_PUBLICATION"}, nil
}

func (s *Store) Release(ctx context.Context, req store.ReleaseRequest, log io.Writer) error {
	id, err := s.pickVersion(ctx, req.Package, req.Version, isReady)
	if err != nil {
		return err
	}
	if req.Percent > 0 && req.Percent < 100 {
		pct, err := partial(req.Percent)
		if err != nil {
			return err
		}
		if err := s.client.UpdatePublishSettings(ctx, req.Package, id, rustore.PublishSettings{PartialValue: pct}); err != nil {
			return err
		}
	}
	if err := s.client.Publish(ctx, req.Package, id); err != nil {
		return err
	}
	fmt.Fprintf(log, "rustore: version %d published\n", id)
	return nil
}

func (s *Store) Rollout(ctx context.Context, req store.RolloutRequest, log io.Writer) error {
	pct, err := partial(req.Percent)
	if err != nil {
		return err
	}
	id, err := s.pickVersion(ctx, req.Package, req.Version, isLive)
	if err != nil {
		return err
	}
	if err := s.client.UpdatePublishSettings(ctx, req.Package, id, rustore.PublishSettings{PartialValue: pct}); err != nil {
		return err
	}
	fmt.Fprintf(log, "rustore: version %d rollout set to %d%%\n", id, pct)
	return nil
}

func (s *Store) Notes(context.Context, string, string, string) error {
	return store.Unsupported(store.RuStore, "notes",
		"RuStore takes release notes only with a new version: droidship publish --notes")
}

func (s *Store) Listing(context.Context, string, string) (store.Listing, error) {
	return store.Listing{}, store.Unsupported(store.RuStore, "listing", "the RuStore API has no store-page read")
}

func (s *Store) Reviews(ctx context.Context, q store.ReviewsQuery) ([]store.Review, error) {
	comments, err := s.client.Comments(ctx, q.Package, 0, pageSize)
	if err != nil {
		return nil, err
	}
	replies := map[string]string{}
	for page := 0; page < maxAnswerPages; page++ {
		answers, err := s.client.Feedbacks(ctx, q.Package, page, pageSize)
		if err != nil {
			return nil, err
		}
		for _, a := range answers {
			if _, seen := replies[a.CommentID]; !seen {
				replies[a.CommentID] = a.Text
			}
		}
		if len(answers) < pageSize {
			break
		}
	}
	var list []rustoreComment
	for _, c := range comments {
		list = append(list, rustoreComment{id: strconv.FormatInt(c.CommentID, 10), author: c.UserName,
			stars: c.AppRating, text: c.CommentText, when: c.Time(), version: c.AppVersionName})
	}
	return joinReviews(list, replies, q), nil
}

func (s *Store) Reply(ctx context.Context, pkg, reviewID, text string) error {
	_, err := s.client.Answer(ctx, pkg, reviewID, text)
	return err
}

// rustoreComment is a review before the join with answers.
type rustoreComment struct {
	id, author, text, version string
	stars                     int
	when                      time.Time
}

// joinReviews attaches answers to reviews and applies the query filters.
func joinReviews(list []rustoreComment, replies map[string]string, q store.ReviewsQuery) []store.Review {
	since := time.Now().AddDate(0, 0, -q.Days)
	var out []store.Review
	for _, c := range list {
		reply, replied := replies[c.id]
		switch {
		case q.Stars > 0 && c.stars != q.Stars,
			q.Unanswered && replied,
			q.Days > 0 && !c.when.IsZero() && c.when.Before(since):
			continue
		}
		out = append(out, store.Review{Store: store.RuStore, ID: c.id, Author: c.author, Stars: c.stars,
			Text: strings.TrimSpace(c.text), Time: c.when, AppVersion: c.version, Replied: replied, Reply: reply})
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out
}

// partial converts a unified percent into a RuStore rollout step; 0 means everyone.
func partial(percent float64) (int, error) {
	p := int(percent)
	if float64(p) != percent || !validPartial[p] {
		return 0, fmt.Errorf("RuStore rolls out in steps of 5, 10, 25, 50, 75 or 100 percent, not %g", percent)
	}
	return p, nil
}

func isReady(status string) bool { return status == "READY_FOR_PUBLICATION" }

func isLive(status string) bool { return status == "ACTIVE" || strings.Contains(status, "PARTIAL") }

// pickVersion returns the explicit version id, or the newest version whose
// status matches.
func (s *Store) pickVersion(ctx context.Context, pkg, version string, match func(string) bool) (int64, error) {
	if version != "" {
		id, err := strconv.ParseInt(version, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("--version for RuStore is a version id, got %q", version)
		}
		return id, nil
	}
	page, err := s.client.Versions(ctx, pkg, rustore.VersionsOpts{Size: 20})
	if err != nil {
		return 0, err
	}
	for _, v := range page.Content {
		if match(v.VersionStatus) {
			return v.VersionID, nil
		}
	}
	return 0, errors.New("no version in the right state: pass --version (see droidship status " + pkg + " --store rustore)")
}
