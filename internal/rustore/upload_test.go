package rustore

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileUploadBody(t *testing.T) {
	p := writeTempFile(t, "app.apk", "binary-bytes")

	contentType, length, body, closeFn, err := newFileUpload(p)
	if err != nil {
		t.Fatalf("newFileUpload: %v", err)
	}
	defer closeFn()

	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if int64(len(raw)) != length {
		t.Errorf("declared length %d != actual %d", length, len(raw))
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q (%v)", contentType, err)
	}
	mr := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("no multipart part: %v", err)
	}
	if part.FormName() != "file" || part.FileName() != "app.apk" {
		t.Errorf("part name=%q filename=%q", part.FormName(), part.FileName())
	}
	got, _ := io.ReadAll(part)
	if string(got) != "binary-bytes" {
		t.Errorf("part content = %q", got)
	}
	if _, err := mr.NextPart(); err != io.EOF {
		t.Errorf("want exactly one part, second NextPart err = %v", err)
	}
}

func TestUploadAPK(t *testing.T) {
	key := testKey(t)
	p := writeTempFile(t, "gdebenz.apk", "apk-content")
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/public/v1/application/com.gdebenz.win/version/243242/apk" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("isMainApk") != "true" {
			t.Errorf("isMainApk = %q, want true", q.Get("isMainApk"))
		}
		if q.Has("servicesType") {
			t.Errorf("servicesType must be omitted when empty, query = %v", q)
		}
		if r.ContentLength <= 0 {
			t.Errorf("ContentLength = %d, want positive (not chunked)", r.ContentLength)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer f.Close()
		if hdr.Filename != "gdebenz.apk" {
			t.Errorf("filename = %q", hdr.Filename)
		}
		got, _ := io.ReadAll(f)
		if string(got) != "apk-content" {
			t.Errorf("uploaded content = %q", got)
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":null,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	if err := c.UploadAPK(t.Context(), "com.gdebenz.win", 243242, p, true, ""); err != nil {
		t.Fatalf("UploadAPK: %v", err)
	}
}

func TestUploadAPKHMS(t *testing.T) {
	key := testKey(t)
	p := writeTempFile(t, "hms.apk", "x")
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("isMainApk") != "false" || q.Get("servicesType") != "HMS" {
			t.Errorf("query = %v, want isMainApk=false servicesType=HMS", q)
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":null,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	if err := c.UploadAPK(t.Context(), "p", 1, p, false, "HMS"); err != nil {
		t.Fatalf("UploadAPK: %v", err)
	}
}

func TestUploadAAB(t *testing.T) {
	key := testKey(t)
	p := writeTempFile(t, "bundle.aab", "aab-content")
	srv, _ := newAuthServer(t, key, "42", 900, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/application/p/version/9/aab" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if len(r.URL.Query()) != 0 {
			t.Errorf("aab upload must have no query, got %v", r.URL.Query())
		}
		fmt.Fprint(w, `{"code":"OK","message":null,"body":null,"timestamp":"x"}`)
	})
	c := newTestClient(key, "42", srv.URL)

	if err := c.UploadAAB(t.Context(), "p", 9, p); err != nil {
		t.Fatalf("UploadAAB: %v", err)
	}
}
