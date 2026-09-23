package appgallery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// reviewsPath serves both the comment list (GET) and the developer reply (POST).
const reviewsPath = "/api/reviews/v1/manage/dev/reviews"

// Review is one user comment. Fields AppGallery may add over time stay in Raw.
type Review struct {
	ReviewID      flexStr `json:"reviewId"`
	Content       string  `json:"content"`
	Rating        flexInt `json:"rating"`
	Dateline      flexInt `json:"dateline"`
	Lang          string  `json:"lang"`
	CountryCode   string  `json:"countryCode"`
	VersionName   string  `json:"versionName"`
	PhoneName     string  `json:"phoneName"`
	DevReplyState flexInt `json:"devReplyState"`

	Raw json.RawMessage `json:"-"`
}

// Time returns when the comment was posted; dateline is in milliseconds.
func (r Review) Time() time.Time {
	ms := r.Dateline.Int()
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// Answered reports whether the developer has already replied.
func (r Review) Answered() bool {
	// 0: not replied, 1: replied, 3: replied again, 6: user replied back.
	switch r.DevReplyState.Int() {
	case 1, 3:
		return true
	default:
		return false
	}
}

// ReviewsParams filters the comment list. AppID, Begin, End and Countries are
// required by the API; the window may not exceed six months.
type ReviewsParams struct {
	AppID     string
	Begin     time.Time
	End       time.Time
	Countries []string
	Ratings   []int
	Langs     []string
	// ReplyStates filters by devReplyState (0 not replied, 1 replied,
	// 3 replied again, 6 user replied back).
	ReplyStates []int
	Page        int
	Limit       int
}

// ReviewsPage is one page of comments.
type ReviewsPage struct {
	Reviews []Review
	HasNext bool
	Total   int64
}

// Reviews fetches one page of user comments.
func (c *Client) Reviews(ctx context.Context, p ReviewsParams) (ReviewsPage, error) {
	if p.AppID == "" {
		return ReviewsPage{}, fmt.Errorf("app id is required")
	}
	if p.Begin.IsZero() || p.End.IsZero() {
		return ReviewsPage{}, fmt.Errorf("both ends of the time window are required")
	}
	if len(p.Countries) == 0 {
		return ReviewsPage{}, fmt.Errorf("at least one country code is required")
	}

	q := url.Values{
		"appId":     {p.AppID},
		"beginTime": {strconv.FormatInt(p.Begin.UnixMilli(), 10)},
		"endTime":   {strconv.FormatInt(p.End.UnixMilli(), 10)},
		"countries": {strings.Join(p.Countries, ",")},
	}
	if len(p.Ratings) > 0 {
		q.Set("ratings", joinInts(p.Ratings))
	}
	if len(p.Langs) > 0 {
		q.Set("langs", strings.Join(p.Langs, ","))
	}
	if len(p.ReplyStates) > 0 {
		q.Set("devReplyStates", joinInts(p.ReplyStates))
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	var body struct {
		Data struct {
			ReviewList []json.RawMessage `json:"reviewList"`
			HasNext    flexBool          `json:"hasNext"`
			Total      flexInt           `json:"total"`
		} `json:"data"`
	}
	if err := c.get(ctx, reviewsPath, q, &body); err != nil {
		return ReviewsPage{}, err
	}

	page := ReviewsPage{HasNext: body.Data.HasNext.Bool(), Total: body.Data.Total.Int()}
	for _, raw := range body.Data.ReviewList {
		var r Review
		if err := json.Unmarshal(raw, &r); err != nil {
			return ReviewsPage{}, fmt.Errorf("reviews: %w", err)
		}
		r.Raw = raw
		page.Reviews = append(page.Reviews, r)
	}
	return page, nil
}

// ReplyParams is a developer answer to one comment.
type ReplyParams struct {
	AppID       string `json:"appId"`
	ReviewID    string `json:"reviewId"`
	Content     string `json:"devReplyContent"`
	Lang        string `json:"lang"`
	CountryCode string `json:"countryCode"`
	// ToReplyID answers a specific user reply inside the thread.
	ToReplyID string `json:"toReplyId,omitempty"`
	// UpdateReplyID edits an answer that was posted earlier.
	UpdateReplyID string `json:"updateReplyId,omitempty"`
}

// Reply posts a developer answer to a comment. A comment can be answered once;
// use UpdateReplyID to change an answer that is already published.
func (c *Client) Reply(ctx context.Context, p ReplyParams) error {
	switch {
	case p.AppID == "":
		return fmt.Errorf("app id is required")
	case p.ReviewID == "":
		return fmt.Errorf("review id is required")
	case strings.TrimSpace(p.Content) == "":
		return fmt.Errorf("the reply text is empty")
	case p.Lang == "":
		return fmt.Errorf("language code is required, e.g. ru_RU")
	case p.CountryCode == "":
		return fmt.Errorf("country code is required, e.g. RU")
	case p.ToReplyID != "" && p.UpdateReplyID != "":
		return fmt.Errorf("toReplyId and updateReplyId cannot be used together")
	}
	headers := map[string]string{"requestId": newRequestID()}
	return c.sendJSON(ctx, "POST", reviewsPath, nil, p, headers, nil)
}

// newRequestID returns the unique request id the Reviews API expects.
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf[:])
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
