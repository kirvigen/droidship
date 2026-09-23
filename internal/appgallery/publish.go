package appgallery

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// PhasedRelease describes a percentage rollout window.
type PhasedRelease struct {
	StartTime   string `json:"phasedReleaseStartTime"`
	EndTime     string `json:"phasedReleaseEndTime"`
	Percent     string `json:"phasedReleasePercent"`
	Description string `json:"phasedReleaseDescription"`
}

// SubmitParams controls how a prepared version is sent for review.
type SubmitParams struct {
	// Remark is the note for the reviewer, 10 to 300 characters.
	Remark string
	// ReleaseTime schedules the release, e.g. 2026-09-05T10:00:00+0300.
	ReleaseTime string
	// Phased turns the submission into a percentage rollout.
	Phased *PhasedRelease
}

// Submit sends the current version of the app for review.
func (c *Client) Submit(ctx context.Context, appID string, p SubmitParams) error {
	releaseType := ReleaseFull
	var body any
	if p.Phased != nil {
		releaseType = ReleasePhased
		body = p.Phased
	}

	q := url.Values{"appId": {appID}, "releaseType": {strconv.Itoa(releaseType)}}
	if p.Remark != "" {
		q.Set("remark", p.Remark)
	}
	if p.ReleaseTime != "" {
		q.Set("releaseTime", p.ReleaseTime)
	}
	return c.postJSON(ctx, "/api/publish/v2/app-submit", q, body, nil)
}

// Withdraw takes the app back from review.
func (c *Client) Withdraw(ctx context.Context, appID string) error {
	q := url.Values{"appId": {appID}}
	return c.postJSON(ctx, "/api/publish/v1/app-info/withdraw", q, nil, nil)
}

// ValidateRemark checks the reviewer note against the limits AppGallery enforces.
func ValidateRemark(remark string) error {
	if remark == "" {
		return nil
	}
	n := len([]rune(remark))
	if n < 10 || n > 300 {
		return fmt.Errorf("the reviewer note must be 10 to 300 characters, got %d", n)
	}
	return nil
}
