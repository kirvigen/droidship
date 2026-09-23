package appgallerycmd

import "github.com/kirvigen/droidship/internal/config"

// credentials are the API client credentials plus the defaults that usually
// travel with them.
type credentials struct {
	ClientID     string
	ClientSecret string
	AppID        string
	Region       string
	Package      string
}

// loadCredentials resolves the credentials through droidship's config: the
// DROIDSHIP_APPGALLERY_, HSTORE_ and HUAWEI_ variables, config.json, then the
// legacy ~/.config/hstore file.
func loadCredentials() (credentials, error) {
	c, err := config.AppGallery()
	if err != nil {
		return credentials{}, err
	}
	return credentials{ClientID: c.ClientID, ClientSecret: c.ClientSecret,
		AppID: c.AppID, Region: c.Region, Package: c.Package}, nil
}
