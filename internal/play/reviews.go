package play

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// MaxReplyLen is the hard limit Google Play puts on a developer reply.
const MaxReplyLen = 350

// Timestamp is Play's seconds/nanos pair.
type Timestamp struct {
	Seconds string `json:"seconds"`
	Nanos   int    `json:"nanos"`
}

// Time converts the timestamp to a Go time, falling back to the zero time.
func (t Timestamp) Time() time.Time {
	secs, err := strconv.ParseInt(t.Seconds, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(secs, int64(t.Nanos))
}

// UserComment is what a user wrote.
type UserComment struct {
	Text             string    `json:"text"`
	LastModified     Timestamp `json:"lastModified"`
	StarRating       int       `json:"starRating"`
	ReviewerLanguage string    `json:"reviewerLanguage"`
	Device           string    `json:"device"`
	AndroidOSVersion int       `json:"androidOsVersion"`
	AppVersionCode   int       `json:"appVersionCode"`
	AppVersionName   string    `json:"appVersionName"`
	ThumbsUpCount    int       `json:"thumbsUpCount"`
	ThumbsDownCount  int       `json:"thumbsDownCount"`
}

// DeveloperComment is the reply already published for a review.
type DeveloperComment struct {
	Text         string    `json:"text"`
	LastModified Timestamp `json:"lastModified"`
}

// Comment is one entry of a review thread: either the user's or ours.
type Comment struct {
	UserComment      *UserComment      `json:"userComment,omitempty"`
	DeveloperComment *DeveloperComment `json:"developerComment,omitempty"`
}

// Review is one review thread.
type Review struct {
	ReviewID   string    `json:"reviewId"`
	AuthorName string    `json:"authorName"`
	Comments   []Comment `json:"comments"`
}

// User returns the user's comment, or nil for a malformed review.
func (r Review) User() *UserComment {
	for _, c := range r.Comments {
		if c.UserComment != nil {
			return c.UserComment
		}
	}
	return nil
}

// Reply returns the developer reply already published, or nil.
func (r Review) Reply() *DeveloperComment {
	for _, c := range r.Comments {
		if c.DeveloperComment != nil {
			return c.DeveloperComment
		}
	}
	return nil
}

// Stars is the rating, or 0 when the review carries no user comment.
func (r Review) Stars() int {
	if u := r.User(); u != nil {
		return u.StarRating
	}
	return 0
}

// ReviewsOpts narrows a review listing.
type ReviewsOpts struct {
	MaxResults          int
	Token               string // page token from a previous call
	TranslationLanguage string // ask Play to translate reviews into this language
}

// Reviews lists reviews. Play only returns reviews created or edited in the
// last week — there is no way to page further back through this API.
func (c *Client) Reviews(ctx context.Context, pkg string, opts ReviewsOpts) ([]Review, string, error) {
	q := url.Values{}
	if opts.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(opts.MaxResults))
	}
	if opts.Token != "" {
		q.Set("token", opts.Token)
	}
	if opts.TranslationLanguage != "" {
		q.Set("translationLanguage", opts.TranslationLanguage)
	}
	var res struct {
		Reviews         []Review `json:"reviews"`
		TokenPagination struct {
			NextPageToken string `json:"nextPageToken"`
		} `json:"tokenPagination"`
	}
	path := "/androidpublisher/v3/applications/" + url.PathEscape(pkg) + "/reviews"
	if err := c.get(ctx, path, q, &res); err != nil {
		return nil, "", err
	}
	return res.Reviews, res.TokenPagination.NextPageToken, nil
}

// Review fetches a single review thread.
func (c *Client) Review(ctx context.Context, pkg, reviewID string) (*Review, error) {
	var r Review
	path := "/androidpublisher/v3/applications/" + url.PathEscape(pkg) + "/reviews/" + url.PathEscape(reviewID)
	if err := c.get(ctx, path, nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ReplyToReview publishes a public developer reply. A review carries at most
// one reply — replying again replaces the previous text.
func (c *Client) ReplyToReview(ctx context.Context, pkg, reviewID, text string) (*DeveloperComment, error) {
	if n := len([]rune(text)); n > MaxReplyLen {
		return nil, fmt.Errorf("reply is %d characters, Play allows %d", n, MaxReplyLen)
	}
	if text == "" {
		return nil, fmt.Errorf("reply text must not be empty")
	}
	var res struct {
		Result DeveloperComment `json:"result"`
	}
	path := "/androidpublisher/v3/applications/" + url.PathEscape(pkg) + "/reviews/" + url.PathEscape(reviewID) + ":reply"
	if err := c.postJSON(ctx, path, nil, map[string]string{"replyText": text}, &res); err != nil {
		return nil, err
	}
	return &res.Result, nil
}
