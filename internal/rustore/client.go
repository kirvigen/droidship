package rustore

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DefaultBaseURL is the production RuStore Public API host.
const DefaultBaseURL = "https://public-api.rustore.ru"

// tokenRefreshMargin re-fetches the JWE token this long before its ttl ends.
const tokenRefreshMargin = 60 * time.Second

// Client talks to the RuStore Public API. Create it with New or NewWithKey.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	keyID string
	key   *rsa.PrivateKey
	now   func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New builds a Client from a key id and the base64-encoded private key as
// issued by the RuStore Console.
func New(keyID, base64Key string) (*Client, error) {
	key, err := ParsePrivateKey(base64Key)
	if err != nil {
		return nil, err
	}
	return NewWithKey(keyID, key), nil
}

// NewWithKey builds a Client from an already parsed private key.
func NewWithKey(keyID string, key *rsa.PrivateKey) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
		keyID:   keyID,
		key:     key,
		now:     time.Now,
	}
}

// Token returns a valid JWE token, re-authenticating when the cached one is
// close to its ttl.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.tokenExp) {
		return c.token, nil
	}

	ts := formatTimestamp(c.now())
	sig, err := signAuth(c.key, c.keyID, ts)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{
		"keyId":     c.keyID,
		"timestamp": ts,
		"signature": sig,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/public/auth", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()

	body, err := decodeEnvelope(resp)
	if err != nil {
		return "", err
	}
	var auth struct {
		Jwe string `json:"jwe"`
		TTL int    `json:"ttl"`
	}
	if err := json.Unmarshal(body, &auth); err != nil {
		return "", fmt.Errorf("auth response body: %w", err)
	}
	if auth.Jwe == "" {
		return "", fmt.Errorf("auth response has empty jwe token")
	}
	c.token = auth.Jwe
	c.tokenExp = c.now().Add(time.Duration(auth.TTL)*time.Second - tokenRefreshMargin)
	return c.token, nil
}

// get performs an authenticated GET and decodes the envelope body into out.
func (c *Client) get(ctx context.Context, path string, query map[string]string, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, "", 0, out)
}

// postJSON performs an authenticated POST with a JSON body.
func (c *Client) postJSON(ctx context.Context, path string, query map[string]string, in, out any) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	return c.do(ctx, http.MethodPost, path, query, body, "application/json", 0, out)
}

// do performs one authenticated request against the Public API and decodes the
// response envelope. A non-OK envelope code or an HTTP error status becomes an
// *APIError.
func (c *Client) do(ctx context.Context, method, path string, query map[string]string, body io.Reader, contentType string, contentLength int64, out any) error {
	token, err := c.Token(ctx)
	if err != nil {
		return err
	}

	u := c.BaseURL + path
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			q.Set(k, v)
		}
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Public-Token", token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentLength > 0 {
		req.ContentLength = contentLength
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := decodeEnvelope(resp)
	if err != nil {
		return err
	}
	if out != nil && len(respBody) > 0 && !bytes.Equal(respBody, []byte("null")) {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("%s %s: response body: %w", method, path, err)
		}
	}
	return nil
}

// decodeEnvelope parses the {code, message, body} envelope and maps errors.
func decodeEnvelope(resp *http.Response) (json.RawMessage, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var env struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Body    json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		snippet := raw
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("unexpected response (http %d): %s", resp.StatusCode, snippet)
	}
	if resp.StatusCode >= 400 || env.Code != "OK" {
		return nil, &APIError{HTTPStatus: resp.StatusCode, Code: env.Code, Message: env.Message}
	}
	return env.Body, nil
}
