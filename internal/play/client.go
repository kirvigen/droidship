package play

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

// DefaultBaseURL is the Google Play Developer API host.
const DefaultBaseURL = "https://androidpublisher.googleapis.com"

// DefaultGamesBaseURL is the host of the Play Games Services Publishing API,
// a separate service with its own enablement but the same OAuth scope.
const DefaultGamesBaseURL = "https://www.googleapis.com"

// tokenRefreshMargin re-fetches the access token this long before it expires.
const tokenRefreshMargin = 60 * time.Second

// Client talks to the Google Play Developer API v3 on behalf of a service
// account. Create it with New or NewWithServiceAccount.
type Client struct {
	BaseURL      string
	GamesBaseURL string
	TokenURL     string
	HTTP         *http.Client

	sa  *ServiceAccount
	now func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New builds a Client from a service-account JSON key file.
func New(keyPath string) (*Client, error) {
	sa, err := LoadServiceAccount(keyPath)
	if err != nil {
		return nil, err
	}
	return NewWithServiceAccount(sa), nil
}

// NewWithServiceAccount builds a Client from an already parsed key.
func NewWithServiceAccount(sa *ServiceAccount) *Client {
	return &Client{
		BaseURL:      DefaultBaseURL,
		GamesBaseURL: DefaultGamesBaseURL,
		TokenURL:     sa.TokenURI,
		// Bundles are tens of megabytes over a proxy — be generous.
		HTTP: &http.Client{Timeout: 30 * time.Minute},
		sa:   sa,
		now:  time.Now,
	}
}

// ServiceAccount exposes the identity the client authenticates as.
func (c *Client) ServiceAccount() *ServiceAccount { return c.sa }

// Token returns a valid access token, re-authenticating when the cached one is
// close to expiry.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.tokenExp) {
		return c.token, nil
	}
	token, exp, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	c.token, c.tokenExp = token, exp
	return c.token, nil
}

// APIError is a non-2xx answer from the Play Developer API.
type APIError struct {
	HTTPStatus int
	Status     string // e.g. PERMISSION_DENIED
	Message    string
	Method     string
	Path       string
}

func (e *APIError) Error() string {
	head := fmt.Sprintf("play api %s %s: http %d", e.Method, e.Path, e.HTTPStatus)
	if e.Status != "" {
		head += " " + e.Status
	}
	if e.Message != "" {
		head += ": " + e.Message
	}
	if hint := e.Hint(); hint != "" {
		head += "\n\n" + hint
	}
	return head
}

// Hint turns the API's terse errors into the concrete next step, because every
// one of these has exactly one cause in practice.
func (e *APIError) Hint() string {
	msg := strings.ToLower(e.Message)
	switch {
	case e.HTTPStatus == 401 && strings.Contains(msg, "insufficient permissions"),
		e.HTTPStatus == 403 && strings.Contains(msg, "does not have permission"):
		return "The service account is authenticated but has no access to this app.\n" +
			"Play Console → Users and permissions → Invite new users → paste the service\n" +
			"account email, add the app, grant the release permissions, then Invite user.\n" +
			"Google needs a few minutes to propagate a fresh grant."
	case e.HTTPStatus == 403 && strings.Contains(msg, "has not been used"),
		e.HTTPStatus == 403 && strings.Contains(msg, "is disabled"):
		return "The Google Play Android Developer API is not enabled in the key's project.\n" +
			"Enable it: gcloud services enable androidpublisher.googleapis.com --project=<project>"
	case e.HTTPStatus == 404:
		return "No such package (or the service account cannot see it). Check the applicationId\n" +
			"and that the app exists in this Play Console developer account."
	case e.HTTPStatus == 400 && strings.Contains(msg, "version code"):
		return "Play rejects a versionCode that was already uploaded. Bump versionCode and rebuild."
	case e.HTTPStatus == 409:
		return "Another edit is already open for this app. Wait for it to expire (7 days) or\n" +
			"finish it in the Play Console."
	}
	return ""
}

// get performs an authenticated GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, "", out)
}

// postJSON performs an authenticated POST with an optional JSON body.
func (c *Client) postJSON(ctx context.Context, path string, query url.Values, in, out any) error {
	return c.jsonBody(ctx, http.MethodPost, path, query, in, out)
}

// putJSON performs an authenticated PUT with a JSON body.
func (c *Client) putJSON(ctx context.Context, path string, query url.Values, in, out any) error {
	return c.jsonBody(ctx, http.MethodPut, path, query, in, out)
}

func (c *Client) jsonBody(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body io.Reader
	contentType := ""
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
		contentType = "application/json"
	}
	return c.do(ctx, method, path, query, body, contentType, out)
}

// delete performs an authenticated DELETE.
func (c *Client) delete(ctx context.Context, path string, query url.Values) error {
	return c.do(ctx, http.MethodDelete, path, query, nil, "", nil)
}

// do performs one authenticated request against the Play Developer API.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, out any) error {
	return c.doAt(ctx, c.BaseURL, method, path, query, body, contentType, out)
}

// doAt performs one authenticated request against an explicit host, so the
// Games Configuration API can share the client, its token and its errors.
func (c *Client) doAt(ctx context.Context, base, method, path string, query url.Values, body io.Reader, contentType string, out any) error {
	token, err := c.Token(ctx)
	if err != nil {
		return err
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.send(req, method, path, out)
}

// send executes a prepared request and decodes its response.
func (c *Client) send(req *http.Request, method, path string, out any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return newAPIError(resp.StatusCode, raw, method, path)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s %s: response body: %w", method, path, err)
		}
	}
	return nil
}

// newAPIError maps Google's {"error":{...}} envelope onto *APIError.
func newAPIError(status int, raw []byte, method, path string) error {
	var env struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Error.Message == "" {
		return &APIError{HTTPStatus: status, Message: snippet(raw), Method: method, Path: path}
	}
	return &APIError{
		HTTPStatus: status,
		Status:     env.Error.Status,
		Message:    env.Error.Message,
		Method:     method,
		Path:       path,
	}
}

func readBody(resp *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return raw, nil
}

func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// jsonPayload marshals a request body, returning nil for a bodiless call.
func jsonPayload(in any) (io.Reader, string, error) {
	if in == nil {
		return nil, "", nil
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(payload), "application/json", nil
}
