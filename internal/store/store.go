// Package store is the contract every store adapter implements, and the types
// the unified verbs pass around.
package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// Names of the supported stores, as typed after --store.
const (
	GPlay      = "gplay"
	RuStore    = "rustore"
	AppGallery = "appgallery"
)

// All lists the stores in display order.
var All = []string{GPlay, RuStore, AppGallery}

// Store is one app store as the unified verbs see it. An adapter returns an
// error wrapping ErrUnsupported for a verb its API cannot perform.
type Store interface {
	Name() string
	Auth(ctx context.Context) (AuthInfo, error)
	Status(ctx context.Context, pkg string) ([]Release, error)
	// Publish uploads a build and stages it. Unless req.GoLive is set, no user
	// sees the build until Release runs. Progress goes to log.
	Publish(ctx context.Context, req PublishRequest, log io.Writer) (PublishResult, error)
	// Plan checks req against the store without changing anything: the
	// credentials reach the app, the build format is accepted, and it says
	// what Publish would do. It backs `publish --dry-run`.
	Plan(ctx context.Context, req PublishRequest) (PublishPlan, error)
	Release(ctx context.Context, req ReleaseRequest, log io.Writer) error
	Rollout(ctx context.Context, req RolloutRequest, log io.Writer) error
	Notes(ctx context.Context, pkg, lang, text string) error
	Reviews(ctx context.Context, q ReviewsQuery) ([]Review, error)
	Reply(ctx context.Context, pkg, reviewID, text string) error
	Listing(ctx context.Context, pkg, lang string) (Listing, error)
	// ReplyLimit is the longest reply in runes the store accepts; 0 if the
	// store enforces it itself.
	ReplyLimit() int
}

// AuthInfo identifies what the credentials authenticate as. It never holds a secret.
type AuthInfo struct {
	Identity string `json:"identity"`
	Source   string `json:"source"`
}

// Release is one row of a status report.
type Release struct {
	Track   string  `json:"track"`
	Version string  `json:"version"`
	Status  string  `json:"status"`
	Percent float64 `json:"percent,omitempty"`
	ID      string  `json:"id,omitempty"`
}

// PublishRequest is a build to upload.
type PublishRequest struct {
	Package string
	AAB     string
	APK     string
	Notes   string
	Lang    string  // BCP-47, e.g. ru-RU
	Percent float64 // 0 means everyone
	GoLive  bool
}

// PublishResult reports where the build landed.
type PublishResult struct {
	State string `json:"state"`
	Next  string `json:"next,omitempty"` // the command that makes it live, when one is needed
}

// PublishPlan is what Publish would do, checked against the live store.
type PublishPlan struct {
	Live   string `json:"live"`           // what users get today
	Action string `json:"action"`         // what publish would do
	Note   string `json:"note,omitempty"` // anything to know before going ahead
}

// ReleaseRequest makes a staged build live. An empty Version means "the staged one".
type ReleaseRequest struct {
	Package string
	Version string
	Percent float64
}

// RolloutRequest changes the share of users a live release reaches.
type RolloutRequest struct {
	Package string
	Version string
	Percent float64
}

// ReviewsQuery narrows a review listing.
type ReviewsQuery struct {
	Package    string
	Stars      int
	Unanswered bool
	Days       int
	Limit      int
}

// Review is one user review, normalised across stores.
type Review struct {
	Store      string    `json:"store"`
	ID         string    `json:"id"`
	Author     string    `json:"author,omitempty"`
	Stars      int       `json:"stars"`
	Text       string    `json:"text"`
	Time       time.Time `json:"time"`
	AppVersion string    `json:"appVersion,omitempty"`
	Replied    bool      `json:"replied"`
	Reply      string    `json:"reply,omitempty"`
}

// Listing is the store page text for one language.
type Listing struct {
	Lang  string `json:"lang"`
	Title string `json:"title"`
	Short string `json:"short,omitempty"`
	Full  string `json:"full,omitempty"`
}

// ErrUnsupported marks a verb the store's API cannot perform.
var ErrUnsupported = errors.New("unsupported")

type unsupportedError struct{ store, verb, why string }

func (e *unsupportedError) Error() string {
	return fmt.Sprintf("%s: unsupported by %s (%s)", e.verb, e.store, e.why)
}

func (e *unsupportedError) Unwrap() error { return ErrUnsupported }

// Unsupported builds the error an adapter returns for a verb it cannot do.
func Unsupported(store, verb, why string) error {
	return &unsupportedError{store: store, verb: verb, why: why}
}

// CheckReply rejects a reply the store would refuse, before any request.
func CheckReply(limit int, text string) error {
	if text == "" {
		return errors.New("reply text is empty")
	}
	if n := len([]rune(text)); limit > 0 && n > limit {
		return fmt.Errorf("reply is %d characters, the store allows %d", n, limit)
	}
	return nil
}
