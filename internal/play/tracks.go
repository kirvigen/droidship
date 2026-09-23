package play

import (
	"context"
	"net/url"
)

// Tracks lists every track of the app together with its releases.
func (c *Client) Tracks(ctx context.Context, pkg, editID string) ([]Track, error) {
	var res struct {
		Tracks []Track `json:"tracks"`
	}
	if err := c.get(ctx, editPath(pkg, editID)+"/tracks", nil, &res); err != nil {
		return nil, err
	}
	return res.Tracks, nil
}

// Track fetches one track.
func (c *Client) Track(ctx context.Context, pkg, editID, track string) (*Track, error) {
	var t Track
	if err := c.get(ctx, editPath(pkg, editID)+"/tracks/"+url.PathEscape(track), nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTrack replaces the releases of one track. Play takes the whole track
// as the new truth, so callers pass the full desired release list.
func (c *Client) UpdateTrack(ctx context.Context, pkg, editID string, t Track) (*Track, error) {
	var out Track
	path := editPath(pkg, editID) + "/tracks/" + url.PathEscape(t.Track)
	if err := c.putJSON(ctx, path, nil, t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
