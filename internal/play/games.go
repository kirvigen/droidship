package play

// Play Games Services achievements. These live in the Games Configuration API
// (gamesconfiguration.googleapis.com), not in the Play Developer API used by
// the rest of this tool: a different host, a different enablement, the same
// service account and OAuth scope. An achievement is addressed by the numeric
// game project id ("application"), which the Play Console shows under
// Play Games Services → Configuration.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

// Achievement types as the API spells them.
const (
	AchievementStandard    = "STANDARD"
	AchievementIncremental = "INCREMENTAL"
)

// Initial states of an achievement for a player who has not earned it.
const (
	AchievementRevealed = "REVEALED"
	AchievementHidden   = "HIDDEN"
)

// LocalizedString is one translation of a name or a description.
type LocalizedString struct {
	Locale string `json:"locale"`
	Value  string `json:"value"`
}

// LocalizedStringBundle is every translation of one field. The API requires
// the game's primary locale to be present.
type LocalizedStringBundle struct {
	Translations []LocalizedString `json:"translations"`
}

// Get returns the translation for a locale, or the first one as a fallback.
func (b LocalizedStringBundle) Get(locale string) string {
	for _, t := range b.Translations {
		if t.Locale == locale {
			return t.Value
		}
	}
	if len(b.Translations) > 0 {
		return b.Translations[0].Value
	}
	return ""
}

// AchievementDetail is the editable half of an achievement: what the player
// reads, and what it is worth.
type AchievementDetail struct {
	Name        LocalizedStringBundle `json:"name"`
	Description LocalizedStringBundle `json:"description"`
	PointValue  int64                 `json:"pointValue"`
	SortRank    int64                 `json:"sortRank,omitempty"`
	IconURL     string                `json:"iconUrl,omitempty"`
}

// Achievement is one achievement configuration. Draft is what the console
// edits; Published is what players already see, and it only changes when the
// game project is published.
type Achievement struct {
	ID              string             `json:"id,omitempty"`
	Token           string             `json:"token,omitempty"` // assigned by Google, opaque
	AchievementType string             `json:"achievementType"`
	InitialState    string             `json:"initialState"`
	StepsToUnlock   int64              `json:"stepsToUnlock,omitempty"`
	Draft           *AchievementDetail `json:"draft,omitempty"`
	Published       *AchievementDetail `json:"published,omitempty"`
}

type achievementList struct {
	Items         []Achievement `json:"items"`
	NextPageToken string        `json:"nextPageToken"`
}

// Achievements lists every achievement of a game project, following paging.
func (c *Client) Achievements(ctx context.Context, appID string) ([]Achievement, error) {
	var all []Achievement
	page := ""
	for {
		q := url.Values{"maxResults": {"200"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		var out achievementList
		path := "/games/v1configuration/applications/" + url.PathEscape(appID) + "/achievements"
		if err := c.doAt(ctx, c.GamesBaseURL, http.MethodGet, path, q, nil, "", &out); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if out.NextPageToken == "" {
			return all, nil
		}
		page = out.NextPageToken
	}
}

// CreateAchievement adds one achievement to a game project.
func (c *Client) CreateAchievement(ctx context.Context, appID string, a Achievement) (Achievement, error) {
	var out Achievement
	path := "/games/v1configuration/applications/" + url.PathEscape(appID) + "/achievements"
	err := c.jsonBodyAt(ctx, c.GamesBaseURL, http.MethodPost, path, nil, a, &out)
	return out, err
}

// UpdateAchievement replaces an achievement's draft. The token, the type and
// stepsToUnlock cannot be changed after creation.
func (c *Client) UpdateAchievement(ctx context.Context, id string, a Achievement) (Achievement, error) {
	var out Achievement
	path := "/games/v1configuration/achievements/" + url.PathEscape(id)
	err := c.jsonBodyAt(ctx, c.GamesBaseURL, http.MethodPut, path, nil, a, &out)
	return out, err
}

// DeleteAchievement removes an achievement that was never published.
func (c *Client) DeleteAchievement(ctx context.Context, id string) error {
	path := "/games/v1configuration/achievements/" + url.PathEscape(id)
	return c.doAt(ctx, c.GamesBaseURL, http.MethodDelete, path, nil, nil, "", nil)
}

// PublishGameProject pushes the drafts of a game project live. Wraps the
// Play Console's "Review and publish" button.
func (c *Client) PublishGameProject(ctx context.Context, appID string) error {
	path := "/games/v1configuration/applications/" + url.PathEscape(appID) + "/publish"
	return c.doAt(ctx, c.GamesBaseURL, http.MethodPost, path, nil, nil, "", nil)
}

// UploadAchievementIcon replaces the icon of one achievement. Google wants a
// 512×512 PNG or JPEG.
func (c *Client) UploadAchievementIcon(ctx context.Context, achievementID string, png []byte, contentType string) error {
	token, err := c.Token(ctx)
	if err != nil {
		return err
	}
	path := "/upload/games/v1configuration/images/" + url.PathEscape(achievementID) + "/imageType/ACHIEVEMENT_ICON"
	u := c.GamesBaseURL + path + "?uploadType=media"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(png))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(png))
	return c.send(req, http.MethodPost, path, nil)
}

// jsonBodyAt is jsonBody against an explicit host.
func (c *Client) jsonBodyAt(ctx context.Context, base, method, path string, query url.Values, in, out any) error {
	body, contentType, err := jsonPayload(in)
	if err != nil {
		return err
	}
	return c.doAt(ctx, base, method, path, query, body, contentType, out)
}

// NewBundle builds a bundle of translations, primary locale first so the API
// sees it even when a map iterates in another order.
func NewBundle(primary string, values map[string]string) (LocalizedStringBundle, error) {
	if values[primary] == "" {
		return LocalizedStringBundle{}, fmt.Errorf("no text for the primary locale %q", primary)
	}
	b := LocalizedStringBundle{Translations: []LocalizedString{{Locale: primary, Value: values[primary]}}}
	locales := make([]string, 0, len(values))
	for loc := range values {
		if loc != primary && values[loc] != "" {
			locales = append(locales, loc)
		}
	}
	sort.Strings(locales)
	for _, loc := range locales {
		b.Translations = append(b.Translations, LocalizedString{Locale: loc, Value: values[loc]})
	}
	return b, nil
}

// PointsLimit is the total both stores allow across all achievements.
const PointsLimit = 1000

// TotalPoints sums the draft point values, to catch the 1000-point ceiling
// before the API does.
func TotalPoints(list []Achievement) int64 {
	var sum int64
	for _, a := range list {
		if a.Draft != nil {
			sum += a.Draft.PointValue
		}
	}
	return sum
}

// FormatSteps renders stepsToUnlock for a table.
func FormatSteps(a Achievement) string {
	if a.AchievementType != AchievementIncremental {
		return "—"
	}
	return strconv.FormatInt(a.StepsToUnlock, 10)
}
