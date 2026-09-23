package play

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Listing is the store page text for one language.
type Listing struct {
	Language         string `json:"language"`
	Title            string `json:"title"`
	ShortDescription string `json:"shortDescription"`
	FullDescription  string `json:"fullDescription"`
	Video            string `json:"video,omitempty"`
}

// Play's limits on the store page text. Exceeding one is a 400 from the API,
// so we check before spending an upload.
const (
	MaxTitleLen     = 30
	MaxShortDescLen = 80
	MaxFullDescLen  = 4000
)

// Validate reports the first field that breaks Play's length limits.
func (l Listing) Validate() error {
	for _, c := range []struct {
		name  string
		value string
		max   int
	}{
		{"title", l.Title, MaxTitleLen},
		{"shortDescription", l.ShortDescription, MaxShortDescLen},
		{"fullDescription", l.FullDescription, MaxFullDescLen},
	} {
		if n := len([]rune(c.value)); n > c.max {
			return fmt.Errorf("%s is %d characters, Play allows %d", c.name, n, c.max)
		}
	}
	if l.Language == "" {
		return fmt.Errorf("language must not be empty")
	}
	return nil
}

// Details is the app-level contact information.
type Details struct {
	DefaultLanguage string `json:"defaultLanguage,omitempty"`
	ContactEmail    string `json:"contactEmail,omitempty"`
	ContactPhone    string `json:"contactPhone,omitempty"`
	ContactWebsite  string `json:"contactWebsite,omitempty"`
}

// Image is one uploaded graphic asset.
type Image struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// ImageTypes accepted by the Play API for edits.images. Google dropped
// promoGraphic from the AppImageType enum — asking for it is a 400, not an
// empty list, so it must not be in here.
var ImageTypes = []string{
	"phoneScreenshots",
	"sevenInchScreenshots",
	"tenInchScreenshots",
	"tvScreenshots",
	"wearScreenshots",
	"icon",
	"featureGraphic",
	"tvBanner",
}

// ValidImageType reports whether t is one Play understands.
func ValidImageType(t string) bool {
	for _, k := range ImageTypes {
		if k == t {
			return true
		}
	}
	return false
}

// Listings lists the store page text for every language.
func (c *Client) Listings(ctx context.Context, pkg, editID string) ([]Listing, error) {
	var res struct {
		Listings []Listing `json:"listings"`
	}
	if err := c.get(ctx, editPath(pkg, editID)+"/listings", nil, &res); err != nil {
		return nil, err
	}
	return res.Listings, nil
}

// Listing fetches the store page text for one language.
func (c *Client) Listing(ctx context.Context, pkg, editID, lang string) (*Listing, error) {
	var l Listing
	if err := c.get(ctx, editPath(pkg, editID)+"/listings/"+url.PathEscape(lang), nil, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateListing replaces the store page text for one language.
func (c *Client) UpdateListing(ctx context.Context, pkg, editID string, l Listing) (*Listing, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	var out Listing
	path := editPath(pkg, editID) + "/listings/" + url.PathEscape(l.Language)
	if err := c.putJSON(ctx, path, nil, l, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Details fetches the app-level contact information.
func (c *Client) Details(ctx context.Context, pkg, editID string) (*Details, error) {
	var d Details
	if err := c.get(ctx, editPath(pkg, editID)+"/details", nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// PatchDetails changes only the supplied contact fields, leaving the rest as
// they are — a PUT here would blank whatever the caller omitted.
func (c *Client) PatchDetails(ctx context.Context, pkg, editID string, d Details) (*Details, error) {
	var out Details
	if err := c.jsonBody(ctx, "PATCH", editPath(pkg, editID)+"/details", nil, d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Images lists the graphics of one type for one language.
func (c *Client) Images(ctx context.Context, pkg, editID, lang, imageType string) ([]Image, error) {
	if !ValidImageType(imageType) {
		return nil, fmt.Errorf("unknown image type %q", imageType)
	}
	var res struct {
		Images []Image `json:"images"`
	}
	path := editPath(pkg, editID) + "/listings/" + url.PathEscape(lang) + "/" + url.PathEscape(imageType)
	if err := c.get(ctx, path, nil, &res); err != nil {
		return nil, err
	}
	return res.Images, nil
}

// UploadImage adds one graphic. Play appends rather than replaces, so callers
// that want an exact set must DeleteAllImages first.
func (c *Client) UploadImage(ctx context.Context, pkg, editID, lang, imageType, path string, progress io.Writer) (*Image, error) {
	if !ValidImageType(imageType) {
		return nil, fmt.Errorf("unknown image type %q", imageType)
	}
	upath := "/upload" + editPath(pkg, editID) + "/listings/" + url.PathEscape(lang) + "/" + url.PathEscape(imageType)
	q := url.Values{"uploadType": {"media"}}
	var res struct {
		Image Image `json:"image"`
	}
	contentType, err := imageContentType(path)
	if err != nil {
		return nil, err
	}
	if err := c.uploadFile(ctx, upath, q, path, contentType, progress, &res); err != nil {
		return nil, err
	}
	return &res.Image, nil
}

// DeleteImage removes one graphic by id.
func (c *Client) DeleteImage(ctx context.Context, pkg, editID, lang, imageType, imageID string) error {
	path := editPath(pkg, editID) + "/listings/" + url.PathEscape(lang) +
		"/" + url.PathEscape(imageType) + "/" + url.PathEscape(imageID)
	return c.delete(ctx, path, nil)
}

// DeleteAllImages clears every graphic of one type for one language.
func (c *Client) DeleteAllImages(ctx context.Context, pkg, editID, lang, imageType string) error {
	path := editPath(pkg, editID) + "/listings/" + url.PathEscape(lang) + "/" + url.PathEscape(imageType)
	return c.delete(ctx, path, nil)
}

// imageSpec is what Play demands of one kind of graphic. Its own error for a
// bad image is a bare 400, so we check locally and say what is actually wrong.
type imageSpec struct {
	minW, minH int
	maxW, maxH int
	exactW     int // 0 = any within the min/max box
	exactH     int
}

var imageSpecs = map[string]imageSpec{
	"phoneScreenshots":     {minW: 320, minH: 320, maxW: 3840, maxH: 3840},
	"sevenInchScreenshots": {minW: 320, minH: 320, maxW: 3840, maxH: 3840},
	"tenInchScreenshots":   {minW: 320, minH: 320, maxW: 3840, maxH: 3840},
	"tvScreenshots":        {minW: 320, minH: 320, maxW: 3840, maxH: 3840},
	"wearScreenshots":      {minW: 320, minH: 320, maxW: 3840, maxH: 3840},
	"icon":                 {exactW: 512, exactH: 512},
	"featureGraphic":       {exactW: 1024, exactH: 500},
	"tvBanner":             {exactW: 1280, exactH: 720},
}

// imageContentType maps the file to the MIME type Play accepts, rejecting
// anything that is not PNG or JPEG before it costs an upload.
func imageContentType(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png", nil
	case ".jpg", ".jpeg":
		return "image/jpeg", nil
	default:
		return "", fmt.Errorf("%s: Play accepts only PNG and JPEG images", filepath.Base(path))
	}
}

// CheckImage validates the file against Play's rules for that image type.
func CheckImage(path, imageType string) error {
	if _, err := imageContentType(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("%s: cannot read image size: %w", filepath.Base(path), err)
	}
	spec, ok := imageSpecs[imageType]
	if !ok {
		return fmt.Errorf("unknown image type %q", imageType)
	}
	name := filepath.Base(path)
	if spec.exactW > 0 {
		if cfg.Width != spec.exactW || cfg.Height != spec.exactH {
			return fmt.Errorf("%s is %dx%d, %s must be exactly %dx%d",
				name, cfg.Width, cfg.Height, imageType, spec.exactW, spec.exactH)
		}
		return nil
	}
	if cfg.Width < spec.minW || cfg.Height < spec.minH {
		return fmt.Errorf("%s is %dx%d, each side must be at least %dpx", name, cfg.Width, cfg.Height, spec.minW)
	}
	if cfg.Width > spec.maxW || cfg.Height > spec.maxH {
		return fmt.Errorf("%s is %dx%d, no side may exceed %dpx", name, cfg.Width, cfg.Height, spec.maxW)
	}
	// Play rejects a screenshot whose long side is more than twice the short one.
	long, short := cfg.Width, cfg.Height
	if short > long {
		long, short = short, long
	}
	if long > 2*short {
		return fmt.Errorf("%s is %dx%d — the long side may not exceed twice the short one", name, cfg.Width, cfg.Height)
	}
	return nil
}
