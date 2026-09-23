package play

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Scope is the only OAuth scope the Google Play Developer API needs.
const Scope = "https://www.googleapis.com/auth/androidpublisher"

// defaultTokenURI is used when the key file omits token_uri.
const defaultTokenURI = "https://oauth2.googleapis.com/token"

// assertionTTL is the lifetime we claim for the self-signed JWT. Google caps
// it at one hour.
const assertionTTL = time.Hour

// ServiceAccount is the part of a Google service-account JSON key we need.
type ServiceAccount struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	TokenURI     string `json:"token_uri"`

	key *rsa.PrivateKey
}

// LoadServiceAccount reads and validates a service-account JSON key file.
func LoadServiceAccount(path string) (*ServiceAccount, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service account key: %w", err)
	}
	return ParseServiceAccount(raw)
}

// ParseServiceAccount validates the bytes of a service-account JSON key.
func ParseServiceAccount(raw []byte) (*ServiceAccount, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("service account key is not valid JSON: %w", err)
	}
	if sa.Type != "service_account" {
		return nil, fmt.Errorf(`service account key has type %q, want "service_account" — an OAuth client secret or an API key will not work here`, sa.Type)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("service account key is missing client_email or private_key")
	}
	key, err := parsePrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, err
	}
	sa.key = key
	if sa.TokenURI == "" {
		sa.TokenURI = defaultTokenURI
	}
	return &sa, nil
}

// parsePrivateKey decodes the PEM-encoded PKCS#8 (or PKCS#1) RSA key that
// Google puts in the private_key field.
func parsePrivateKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("private_key is not PEM-encoded")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private_key is %T, want an RSA key", key)
		}
		return rsaKey, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private_key: %w", err)
	}
	return key, nil
}

// assertion builds the signed JWT that is exchanged for an access token.
func (sa *ServiceAccount) assertion(now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"iss":   sa.ClientEmail,
		"scope": Scope,
		"aud":   sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(assertionTTL).Unix(),
	})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	input := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, sa.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign assertion: %w", err)
	}
	return input + "." + enc.EncodeToString(sig), nil
}

// fetchToken exchanges the signed assertion for an OAuth access token.
func (c *Client) fetchToken(ctx context.Context) (string, time.Time, error) {
	now := c.now()
	assertion, err := c.sa.assertion(now)
	if err != nil {
		return "", time.Time{}, err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return "", time.Time{}, err
	}
	if resp.StatusCode >= 400 {
		var oauthErr struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		if oauthErr.Error != "" {
			return "", time.Time{}, fmt.Errorf("token request rejected (http %d): %s: %s — check the system clock and that the key has not been deleted in Google Cloud",
				resp.StatusCode, oauthErr.Error, oauthErr.Description)
		}
		return "", time.Time{}, fmt.Errorf("token request failed (http %d): %s", resp.StatusCode, snippet(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", time.Time{}, fmt.Errorf("token response body: %w", err)
	}
	if tok.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("token response has no access_token")
	}
	return tok.AccessToken, now.Add(time.Duration(tok.ExpiresIn)*time.Second - tokenRefreshMargin), nil
}
