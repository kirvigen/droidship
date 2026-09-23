package rustore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// DraftParams is the body of "create a draft release". Empty fields are
// omitted and inherited by RuStore from the active version.
type DraftParams struct {
	AppName          string `json:"appName,omitempty"`
	ShortDescription string `json:"shortDescription,omitempty"`
	FullDescription  string `json:"fullDescription,omitempty"`
	WhatsNew         string `json:"whatsNew,omitempty"`
	ModerInfo        string `json:"moderInfo,omitempty"`
	PublishType      string `json:"publishType,omitempty"` // MANUAL | INSTANTLY | DELAYED
	PublishDateTime  string `json:"publishDateTime,omitempty"`
	PartialValue     int    `json:"partialValue,omitempty"` // 5|10|25|50|75|100
}

// CreateDraft creates a draft version and returns its versionId. The API
// answers either with a bare number or {"versionId": N} — both are handled.
func (c *Client) CreateDraft(ctx context.Context, packageName string, params DraftParams) (int64, error) {
	var raw json.RawMessage
	if err := c.postJSON(ctx, "/public/v1/application/"+packageName+"/version", nil, params, &raw); err != nil {
		return 0, err
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err == nil {
		return id, nil
	}
	var obj struct {
		VersionID int64 `json:"versionId"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.VersionID != 0 {
		return obj.VersionID, nil
	}
	return 0, fmt.Errorf("create draft: unexpected response body %s", raw)
}

var draftIDRe = regexp.MustCompile(`(\d+)`)

// ExistingDraftID extracts the version id of an already existing draft from a
// "draft already exists" API error, if present.
func ExistingDraftID(err error) (int64, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return 0, false
	}
	m := draftIDRe.FindString(apiErr.Message)
	if m == "" {
		return 0, false
	}
	id, convErr := strconv.ParseInt(m, 10, 64)
	if convErr != nil {
		return 0, false
	}
	return id, true
}

// Commit submits the draft version for moderation.
func (c *Client) Commit(ctx context.Context, packageName string, versionID int64, priority int) error {
	path := fmt.Sprintf("/public/v1/application/%s/version/%d/commit", packageName, versionID)
	return c.postJSON(ctx, path, map[string]string{"priorityUpdate": strconv.Itoa(priority)}, nil, nil)
}

// Publish publishes a moderated version with the MANUAL publication type.
func (c *Client) Publish(ctx context.Context, packageName string, versionID int64) error {
	path := fmt.Sprintf("/public/v1/application/%s/version/%d/publish", packageName, versionID)
	return c.postJSON(ctx, path, nil, nil, nil)
}

// DeleteDraft deletes a not-yet-published draft version.
func (c *Client) DeleteDraft(ctx context.Context, packageName string, versionID int64) error {
	path := fmt.Sprintf("/public/v1/application/%s/version/%d", packageName, versionID)
	return c.do(ctx, "DELETE", path, nil, nil, "", 0, nil)
}

// PublishSettings changes the publication type, delayed date or partial
// rollout percentage of a version. Empty fields are omitted.
type PublishSettings struct {
	PublishType     string `json:"publishType,omitempty"`
	PublishDateTime string `json:"publishDateTime,omitempty"`
	PartialValue    int    `json:"partialValue,omitempty"`
}

// UpdatePublishSettings applies PublishSettings to a version. The partial
// rollout percentage can only grow.
func (c *Client) UpdatePublishSettings(ctx context.Context, packageName string, versionID int64, s PublishSettings) error {
	path := fmt.Sprintf("/public/v1/application/%s/version/%d/publish-settings", packageName, versionID)
	return c.postJSON(ctx, path, nil, s, nil)
}
