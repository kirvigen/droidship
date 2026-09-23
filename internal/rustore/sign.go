package rustore

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// ParsePrivateKey decodes a base64-encoded DER private key as issued by the
// RuStore Console (PKCS#8, one line), with a PKCS#1 fallback.
func ParsePrivateKey(b64 string) (*rsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("private key is not valid base64: %w", err)
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("private key is not PKCS#8 or PKCS#1 DER")
}

// formatTimestamp renders the auth timestamp the way RuStore expects:
// ISO-8601 with milliseconds and a numeric UTC offset.
func formatTimestamp(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000-07:00")
}

// signAuth produces the auth signature: base64(RSA-PKCS1v15-SHA512(keyID+timestamp)).
func signAuth(key *rsa.PrivateKey, keyID, timestamp string) (string, error) {
	digest := sha512.Sum512([]byte(keyID + timestamp))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA512, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign auth request: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}
