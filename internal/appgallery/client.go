package appgallery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AppGallery Connect API hosts. Which one serves an account depends on the data
// storage location chosen when the developer account was created.
const (
	DomainGlobal = "https://connect-api.cloud.huawei.com"
	DomainRussia = "https://connect-api-drru.cloud.huawei.com"
	DomainEurope = "https://connect-api-dre.cloud.huawei.com"
	DomainAsia   = "https://connect-api-dra.cloud.huawei.com"
)

// Regions maps the region names accepted on the command line to API hosts.
var Regions = map[string]string{
	"global": DomainGlobal,
	"cn":     DomainGlobal,
	"ru":     DomainRussia,
	"eu":     DomainEurope,
	"de":     DomainEurope,
	"sg":     DomainAsia,
	"asia":   DomainAsia,
}

// RegionOrder is the probing order used when the configured region is wrong.
var RegionOrder = []string{"global", "ru", "eu", "sg"}

// DomainForRegion resolves a region name (or a full https:// URL) to an API host.
func DomainForRegion(region string) (string, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return DomainGlobal, nil
	}
	if strings.HasPrefix(region, "https://") || strings.HasPrefix(region, "http://") {
		return strings.TrimRight(region, "/"), nil
	}
	if domain, ok := Regions[strings.ToLower(region)]; ok {
		return domain, nil
	}
	return "", fmt.Errorf("unknown region %q: use global, ru, eu or sg (or a full https:// host)", region)
}

// tokenRefreshMargin re-fetches the access token this long before it expires.
const tokenRefreshMargin = 5 * time.Minute

// Client talks to the AppGallery Connect API on behalf of one API client.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	clientID     string
	clientSecret string
	now          func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New builds a Client from the client_id and client_secret issued by
// AppGallery Connect (Users and permissions -> API key -> Connect API).
func New(clientID, clientSecret string) *Client {
	return &Client{
		BaseURL:      DomainGlobal,
		HTTP:         &http.Client{Timeout: 30 * time.Minute},
		clientID:     clientID,
		clientSecret: clientSecret,
		now:          time.Now,
	}
}

// ClientID returns the API client id these credentials belong to.
func (c *Client) ClientID() string { return c.clientID }

// Token returns a valid access token, re-authenticating when the cached one is
// close to expiry. Huawei tokens live for 48 hours.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.tokenExp) {
		return c.token, nil
	}

	payload, err := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/oauth2/v1/token", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("token response: %w", err)
	}

	var body struct {
		AccessToken string          `json:"access_token"`
		ExpiresIn   int64           `json:"expires_in"`
		Ret         json.RawMessage `json:"ret"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("token response (http %d): %s", resp.StatusCode, snippet(raw))
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("no access token (http %d): %s", resp.StatusCode, tokenFailure(body.Ret, raw))
	}

	ttl := time.Duration(body.ExpiresIn) * time.Second
	if ttl <= tokenRefreshMargin {
		ttl = tokenRefreshMargin + time.Minute
	}
	c.token = body.AccessToken
	c.tokenExp = c.now().Add(ttl - tokenRefreshMargin)
	return c.token, nil
}

// tokenFailure renders the error detail of a failed token response. The field is
// documented as a string holding JSON, but accounts have been seen returning a
// plain object, so both are handled.
func tokenFailure(retField json.RawMessage, raw []byte) string {
	if len(retField) == 0 {
		return snippet(raw)
	}
	var asString string
	if err := json.Unmarshal(retField, &asString); err == nil {
		return asString
	}
	return string(retField)
}

// get performs an authenticated GET and decodes the response into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, "", nil, out)
}

// putJSON performs an authenticated PUT with a JSON body.
func (c *Client) putJSON(ctx context.Context, path string, query url.Values, in, out any) error {
	return c.sendJSON(ctx, http.MethodPut, path, query, in, nil, out)
}

// postJSON performs an authenticated POST with a JSON body.
func (c *Client) postJSON(ctx context.Context, path string, query url.Values, in, out any) error {
	return c.sendJSON(ctx, http.MethodPost, path, query, in, nil, out)
}

func (c *Client) sendJSON(ctx context.Context, method, path string, query url.Values, in any, headers map[string]string, out any) error {
	body := io.Reader(strings.NewReader(""))
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	return c.do(ctx, method, path, query, body, "application/json", headers, out)
}

// do performs one authenticated request and decodes the result envelope. A
// non-zero result code or an HTTP error status becomes an *APIError.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, headers map[string]string, out any) error {
	token, err := c.Token(ctx)
	if err != nil {
		return err
	}

	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("client_id", c.clientID)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}
	return decode(raw, resp.StatusCode, path, out)
}

// decode maps one API answer onto out, turning a non-zero result code into an
// *APIError.
func decode(raw []byte, status int, path string, out any) error {
	var env struct {
		Ret ret `json:"ret"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("%s: unexpected response (http %d): %s", path, status, snippet(raw))
	}
	if code, ok := env.Ret.code(); ok && code != 0 {
		return &APIError{HTTPStatus: status, Code: code, Message: env.Ret.message(), Path: path}
	}
	if status >= 400 {
		return &APIError{HTTPStatus: status, Message: snippet(raw), Path: path}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: response body: %w", path, err)
	}
	return nil
}

func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}
