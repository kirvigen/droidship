package play

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListingValidate(t *testing.T) {
	ok := Listing{Language: "ru-RU", Title: "ГдеБЕНЗ", ShortDescription: "коротко", FullDescription: "полно"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid listing rejected: %v", err)
	}
	cases := []struct {
		name string
		l    Listing
		want string
	}{
		{"no language", Listing{Title: "x"}, "language"},
		{"long title", Listing{Language: "ru-RU", Title: strings.Repeat("я", 31)}, "title is 31"},
		{"long short", Listing{Language: "ru-RU", ShortDescription: strings.Repeat("я", 81)}, "shortDescription is 81"},
		{"long full", Listing{Language: "ru-RU", FullDescription: strings.Repeat("я", 4001)}, "fullDescription is 4001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.l.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Limits are counted in runes: a Cyrillic title of 30 characters is legal even
// though it is 60 bytes.
func TestListingLimitsCountRunesNotBytes(t *testing.T) {
	l := Listing{Language: "ru-RU", Title: strings.Repeat("я", 30)}
	if err := l.Validate(); err != nil {
		t.Errorf("30 Cyrillic characters must fit in the title: %v", err)
	}
}

func TestValidImageTypeExcludesPromoGraphic(t *testing.T) {
	// Google removed promoGraphic from AppImageType; asking for it is a 400.
	if ValidImageType("promoGraphic") {
		t.Error("promoGraphic must not be offered — the API rejects it")
	}
	for _, ok := range []string{"phoneScreenshots", "icon", "featureGraphic", "tvBanner"} {
		if !ValidImageType(ok) {
			t.Errorf("%s should be valid", ok)
		}
	}
}

// writePNG makes a PNG of the given size for the image checks.
func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckImage(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name      string
		file      string
		w, h      int
		imageType string
		want      string // "" = must pass
	}{
		{"good screenshot", "ok.png", 1080, 1920, "phoneScreenshots", ""},
		{"too small", "small.png", 200, 400, "phoneScreenshots", "at least 320px"},
		{"too large", "big.png", 4000, 4000, "phoneScreenshots", "exceed 3840px"},
		{"too stretched", "wide.png", 2000, 400, "phoneScreenshots", "twice the short one"},
		{"icon right", "icon.png", 512, 512, "icon", ""},
		{"icon wrong", "icon2.png", 256, 256, "icon", "exactly 512x512"},
		{"feature right", "fg.png", 1024, 500, "featureGraphic", ""},
		{"feature wrong", "fg2.png", 1024, 512, "featureGraphic", "exactly 1024x500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writePNG(t, dir, tc.file, tc.w, tc.h)
			err := CheckImage(p, tc.imageType)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestCheckImageRejectsNonImageFormats(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "shot.webp")
	if err := os.WriteFile(p, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := CheckImage(p, "phoneScreenshots")
	if err == nil || !strings.Contains(err.Error(), "PNG and JPEG") {
		t.Errorf("err = %v, want a format complaint", err)
	}
}

func TestPatchDetailsUsesPATCHAndSendsOnlySetFields(t *testing.T) {
	var method string
	var body map[string]any
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"PATCH /androidpublisher/v3/applications/com.example/edits/e1/details": func(w http.ResponseWriter, r *http.Request) {
			method = r.Method
			_ = json.NewDecoder(r.Body).Decode(&body)
			fmt.Fprint(w, `{"contactEmail":"new@example.com"}`)
		},
	})
	out, err := c.PatchDetails(context.Background(), "com.example", "e1", Details{ContactEmail: "new@example.com"})
	if err != nil {
		t.Fatalf("PatchDetails: %v", err)
	}
	if method != "PATCH" {
		t.Errorf("method = %s, want PATCH — a PUT would blank the other fields", method)
	}
	if _, has := body["contactWebsite"]; has {
		t.Errorf("untouched fields must not be sent, got %v", body)
	}
	if out.ContactEmail != "new@example.com" {
		t.Errorf("out = %+v", out)
	}
}

func TestUpdateListingRefusesOverlongTextBeforeCallingPlay(t *testing.T) {
	called := false
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"PUT /androidpublisher/v3/applications/com.example/edits/e1/listings/ru-RU": func(w http.ResponseWriter, r *http.Request) {
			called = true
			fmt.Fprint(w, `{}`)
		},
	})
	_, err := c.UpdateListing(context.Background(), "com.example", "e1",
		Listing{Language: "ru-RU", Title: strings.Repeat("x", 40)})
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if called {
		t.Error("must not spend a request on text Play will reject")
	}
}

func TestUploadImageSendsTheRightContentType(t *testing.T) {
	dir := t.TempDir()
	p := writePNG(t, dir, "s.png", 1080, 1920)
	var gotType, gotPath string
	c := routes(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /upload/androidpublisher/v3/applications/com.example/edits/e1/listings/ru-RU/phoneScreenshots": func(w http.ResponseWriter, r *http.Request) {
			gotType = r.Header.Get("Content-Type")
			gotPath = r.URL.Path
			fmt.Fprint(w, `{"image":{"id":"img-1","sha256":"abc"}}`)
		},
	})
	img, err := c.UploadImage(context.Background(), "com.example", "e1", "ru-RU", "phoneScreenshots", p, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if img.ID != "img-1" {
		t.Errorf("img = %+v", img)
	}
	if gotType != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", gotType)
	}
	if !strings.Contains(gotPath, "/upload/") {
		t.Errorf("path = %q, want the /upload/ prefix", gotPath)
	}
}
