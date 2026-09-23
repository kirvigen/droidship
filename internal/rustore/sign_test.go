package rustore

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"testing"
	"time"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParsePrivateKeyPKCS8(t *testing.T) {
	k := testKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePrivateKey(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	if got.N.Cmp(k.N) != 0 {
		t.Error("parsed key modulus differs from original")
	}
}

func TestParsePrivateKeyPKCS1(t *testing.T) {
	k := testKey(t)
	der := x509.MarshalPKCS1PrivateKey(k)
	got, err := ParsePrivateKey(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	if got.N.Cmp(k.N) != 0 {
		t.Error("parsed key modulus differs from original")
	}
}

func TestParsePrivateKeyTrimsWhitespace(t *testing.T) {
	k := testKey(t)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	b64 := "  " + base64.StdEncoding.EncodeToString(der) + "\n"
	if _, err := ParsePrivateKey(b64); err != nil {
		t.Fatalf("ParsePrivateKey with surrounding whitespace: %v", err)
	}
}

func TestParsePrivateKeyErrors(t *testing.T) {
	if _, err := ParsePrivateKey("not-base64!!!"); err == nil {
		t.Error("want error for invalid base64")
	}
	if _, err := ParsePrivateKey(base64.StdEncoding.EncodeToString([]byte("garbage"))); err == nil {
		t.Error("want error for invalid DER")
	}
}

func TestFormatTimestamp(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	ts := time.Date(2026, 8, 29, 15, 4, 5, 123_000_000, msk)
	got := formatTimestamp(ts)
	want := "2026-08-29T15:04:05.123+03:00"
	if got != want {
		t.Errorf("formatTimestamp = %q, want %q", got, want)
	}
}

func TestSignAuthVerifies(t *testing.T) {
	k := testKey(t)
	const keyID = "1234567"
	ts := "2026-08-29T15:04:05.123+03:00"

	sig, err := signAuth(k, keyID, ts)
	if err != nil {
		t.Fatalf("signAuth: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("signature is not valid base64: %v", err)
	}
	digest := sha512.Sum512([]byte(keyID + ts))
	if err := rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA512, digest[:], raw); err != nil {
		t.Errorf("signature does not verify over keyID+timestamp: %v", err)
	}
}
