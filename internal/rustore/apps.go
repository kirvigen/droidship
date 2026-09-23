package rustore

import "context"

// App is one row of the account's application list.
type App struct {
	AppID           int64  `json:"appId"`
	PackageName     string `json:"packageName"`
	AppName         string `json:"appName"`
	AppStatus       string `json:"appStatus"`
	VersionName     string `json:"versionName"`
	VersionCode     int64  `json:"versionCode"`
	AppVerUpdatedAt string `json:"appVerUpdatedAt"`
	CompanyName     string `json:"companyName"`
	Paid            bool   `json:"paid"`
}

// Apps returns the applications available to the key.
func (c *Client) Apps(ctx context.Context) ([]App, error) {
	var body struct {
		Content []App `json:"content"`
	}
	if err := c.get(ctx, "/public/v1/application", nil, &body); err != nil {
		return nil, err
	}
	return body.Content, nil
}
