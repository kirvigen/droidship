package appgallery

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// PhasedUpdate changes a phased release that is already running.
type PhasedUpdate struct {
	// Percent is the new share of users, above 0 and below 100.
	Percent float64
	// EndTime optionally moves the end of the window, e.g. 2026-09-30T10:00:00+0300.
	EndTime string
	// Full turns the phased release into a release to everyone; the other
	// fields are ignored.
	Full bool
}

// UpdatePhasedRelease changes the percentage of a phased release, or turns it
// into a full release. The start time of a running release cannot change, so
// it is never sent.
func (c *Client) UpdatePhasedRelease(ctx context.Context, appID string, u PhasedUpdate) error {
	if u.Full {
		q := url.Values{"appId": {appID}, "releaseType": {strconv.Itoa(ReleaseFull)}}
		return c.putJSON(ctx, "/api/publish/v2/phased-release", q, struct{}{}, nil)
	}
	if u.Percent <= 0 || u.Percent >= 100 {
		return fmt.Errorf("phased release percent must be above 0 and below 100, got %g (100 means a full release)", u.Percent)
	}
	body := map[string]string{
		"state":                "RELEASE",
		"phasedReleasePercent": fmt.Sprintf("%.2f", u.Percent),
	}
	if u.EndTime != "" {
		body["phasedReleaseEndTime"] = u.EndTime
	}
	q := url.Values{"appId": {appID}, "releaseType": {strconv.Itoa(ReleasePhased)}}
	return c.putJSON(ctx, "/api/publish/v2/phased-release", q, body, nil)
}
