package appgallery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ReleaseType values accepted by the Publishing API.
const (
	ReleaseFull   = 1 // release to everyone once approved
	ReleasePhased = 3 // phased (percentage) release
)

// AppInfo is the subset of the app-info answer that the CLI reports on. The
// untouched payload stays in Raw so `--json` never loses a field.
type AppInfo struct {
	ReleaseState         flexInt `json:"releaseState"`
	VersionNumber        string  `json:"versionNumber"`
	VersionCode          flexStr `json:"versionCode"`
	VersionID            flexStr `json:"versionId"`
	OnShelfVersionNumber string  `json:"onShelfVersionNumber"`
	OnShelfVersionCode   flexStr `json:"onShelfVersionCode"`
	UpdateTime           string  `json:"updateTime"`
	DefaultLang          string  `json:"defaultLang"`
	PrivacyPolicy        string  `json:"privacyPolicy"`
	PublishCountry       string  `json:"publishCountry"`

	Raw json.RawMessage `json:"-"`
}

// releaseStates maps the releaseState codes documented for app-info.
var releaseStates = map[int64]string{
	0:  "released",
	1:  "release rejected",
	2:  "removed",
	3:  "releasing",
	4:  "reviewing",
	5:  "pending update review",
	7:  "draft",
	8:  "update rejected",
	11: "release canceled",
	13: "pre-review failed",
}

// StateLabel renders the releaseState code as text.
func (a AppInfo) StateLabel() string {
	if label, ok := releaseStates[a.ReleaseState.Int()]; ok {
		return label
	}
	return "state " + strconv.FormatInt(a.ReleaseState.Int(), 10)
}

// AppIDByPackage resolves an Android package name to its AppGallery app id.
func (c *Client) AppIDByPackage(ctx context.Context, packageName string) (string, error) {
	var body struct {
		AppIDs []struct {
			Key   string  `json:"key"`
			Value flexStr `json:"value"`
		} `json:"appids"`
	}
	q := url.Values{"packageName": {packageName}}
	if err := c.get(ctx, "/api/publish/v2/appid-list", q, &body); err != nil {
		return "", err
	}
	if len(body.AppIDs) == 0 {
		return "", fmt.Errorf("no app found for package %q (is it created in AppGallery Connect and covered by this API client?)", packageName)
	}
	return body.AppIDs[0].Value.String(), nil
}

// AppInfo fetches the current information about an app version.
func (c *Client) AppInfo(ctx context.Context, appID string, releaseType int) (AppInfo, error) {
	var body struct {
		AppInfo json.RawMessage `json:"appInfo"`
	}
	q := url.Values{"appId": {appID}}
	if releaseType != 0 {
		q.Set("releaseType", strconv.Itoa(releaseType))
	}
	if err := c.get(ctx, "/api/publish/v2/app-info", q, &body); err != nil {
		return AppInfo{}, err
	}
	if len(body.AppInfo) == 0 {
		return AppInfo{}, fmt.Errorf("app-info: empty appInfo for app %s", appID)
	}
	var info AppInfo
	if err := json.Unmarshal(body.AppInfo, &info); err != nil {
		return AppInfo{}, fmt.Errorf("app-info: %w", err)
	}
	info.Raw = body.AppInfo
	return info, nil
}

// LanguageInfo is the localized store listing for one language. Empty fields are
// left untouched in AppGallery Connect.
type LanguageInfo struct {
	Lang        string `json:"lang"`
	AppName     string `json:"appName,omitempty"`
	AppDesc     string `json:"appDesc,omitempty"`
	BriefInfo   string `json:"briefInfo,omitempty"`
	NewFeatures string `json:"newFeatures,omitempty"`
}

// UpdateLanguageInfo writes the localized listing, most often the release notes.
func (c *Client) UpdateLanguageInfo(ctx context.Context, appID string, info LanguageInfo) error {
	if info.Lang == "" {
		return fmt.Errorf("language code is required")
	}
	q := url.Values{"appId": {appID}}
	return c.putJSON(ctx, "/api/publish/v2/app-language-info", q, info, nil)
}
