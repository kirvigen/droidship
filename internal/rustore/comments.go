package rustore

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// MaxAnswerLen is the longest developer answer RuStore accepts.
const MaxAnswerLen = 500

// Comment is one user review.
type Comment struct {
	CommentID      int64  `json:"commentId"`
	UserName       string `json:"userName"`
	AppRating      int    `json:"appRating"`
	CommentStatus  string `json:"commentStatus"`
	CommentText    string `json:"commentText"`
	CommentDateISO string `json:"commentDateIso"`
	AppVersionName string `json:"appVersionName"`
	Edited         bool   `json:"edited"`
}

// Time is when the review was written.
func (c Comment) Time() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, c.CommentDateISO)
	return t
}

// Feedback is a developer answer to a review. RuStore returns its ids as strings.
type Feedback struct {
	ID        string `json:"id"`
	CommentID string `json:"commentId"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	Date      string `json:"date"`
}

// Comments returns one page of reviews, newest first.
func (c *Client) Comments(ctx context.Context, pkg string, page, size int) ([]Comment, error) {
	var out []Comment
	err := c.get(ctx, "/public/v1/application/"+url.PathEscape(pkg)+"/comment", pageQuery(page, size), &out)
	return out, err
}

// Feedbacks returns one page of developer answers.
func (c *Client) Feedbacks(ctx context.Context, pkg string, page, size int) ([]Feedback, error) {
	var out []Feedback
	err := c.get(ctx, "/public/v1/application/"+url.PathEscape(pkg)+"/feedback", pageQuery(page, size), &out)
	return out, err
}

// Answer publishes a developer answer to a review and returns the answer id.
func (c *Client) Answer(ctx context.Context, pkg, commentID, text string) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	err := c.postJSON(ctx, "/public/v1/application/"+url.PathEscape(pkg)+"/feedback",
		map[string]string{"commentId": commentID}, map[string]string{"message": text}, &out)
	return out.ID, err
}

func pageQuery(page, size int) map[string]string {
	return map[string]string{"page": strconv.Itoa(page), "size": strconv.Itoa(size)}
}
