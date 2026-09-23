package play

// Edit is a transaction against one application's store listing and releases.
// Nothing an edit touches becomes visible until it is committed.
type Edit struct {
	ID                string `json:"id"`
	ExpiryTimeSeconds string `json:"expiryTimeSeconds"`
}

// Bundle is one uploaded Android App Bundle.
type Bundle struct {
	VersionCode int    `json:"versionCode"`
	SHA1        string `json:"sha1"`
	SHA256      string `json:"sha256"`
}

// ReleaseNote is the "what's new" text for one language.
type ReleaseNote struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}

// TrackRelease is one release inside a track.
type TrackRelease struct {
	Name         string        `json:"name,omitempty"`
	VersionCodes []string      `json:"versionCodes,omitempty"`
	ReleaseNotes []ReleaseNote `json:"releaseNotes,omitempty"`
	Status       string        `json:"status,omitempty"`
	UserFraction float64       `json:"userFraction,omitempty"`

	InAppUpdatePriority int `json:"inAppUpdatePriority,omitempty"`
}

// Track is a release channel: internal, alpha, beta or production.
type Track struct {
	Track    string         `json:"track"`
	Releases []TrackRelease `json:"releases,omitempty"`
}

// Release statuses accepted by the Play Developer API.
const (
	StatusDraft      = "draft"
	StatusInProgress = "inProgress"
	StatusHalted     = "halted"
	StatusCompleted  = "completed"
)

// Well-known track names.
const (
	TrackInternal   = "internal"
	TrackAlpha      = "alpha"
	TrackBeta       = "beta"
	TrackProduction = "production"
)
