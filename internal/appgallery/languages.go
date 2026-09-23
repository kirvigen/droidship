package appgallery

import (
	"context"
	"net/url"
)

// Languages returns the localized store listing of every language of the app.
func (c *Client) Languages(ctx context.Context, appID string) ([]LanguageInfo, error) {
	var body struct {
		Languages []LanguageInfo `json:"languages"`
	}
	if err := c.get(ctx, "/api/publish/v2/app-info", url.Values{"appId": {appID}}, &body); err != nil {
		return nil, err
	}
	return body.Languages, nil
}
