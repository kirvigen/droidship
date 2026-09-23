package play

import (
	"context"
	"fmt"
	"net/url"
)

// editsPath is the collection of edits for one application.
func editsPath(pkg string) string {
	return "/androidpublisher/v3/applications/" + url.PathEscape(pkg) + "/edits"
}

// editPath is one edit of one application.
func editPath(pkg, editID string) string {
	return editsPath(pkg) + "/" + url.PathEscape(editID)
}

// CreateEdit opens a new edit. Edits expire on their own after about a week,
// so an abandoned one is not fatal — but Delete keeps the app clean.
func (c *Client) CreateEdit(ctx context.Context, pkg string) (*Edit, error) {
	var e Edit
	if err := c.postJSON(ctx, editsPath(pkg), nil, nil, &e); err != nil {
		return nil, err
	}
	if e.ID == "" {
		return nil, fmt.Errorf("create edit: Play returned an edit without an id")
	}
	return &e, nil
}

// ValidateEdit checks the edit without applying it.
func (c *Client) ValidateEdit(ctx context.Context, pkg, editID string) error {
	return c.postJSON(ctx, editPath(pkg, editID)+":validate", nil, nil, nil)
}

// CommitEdit applies the edit. When sendForReview is false the changes are
// saved but not submitted to Google's review queue.
func (c *Client) CommitEdit(ctx context.Context, pkg, editID string, sendForReview bool) error {
	var q url.Values
	if !sendForReview {
		q = url.Values{"changesNotSentForReview": {"true"}}
	}
	return c.postJSON(ctx, editPath(pkg, editID)+":commit", q, nil, nil)
}

// DeleteEdit throws the edit away. Safe to call on an already-dead edit.
func (c *Client) DeleteEdit(ctx context.Context, pkg, editID string) error {
	return c.delete(ctx, editPath(pkg, editID), nil)
}
