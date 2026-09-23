package appgallery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempPackage creates a fake package file and returns its path.
func writeTempPackage(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUploadPackageUsesThePreSignedFlow(t *testing.T) {
	const content = "aab-bytes"
	path := writeTempPackage(t, "app-release.aab", content)

	var uploaded string
	var gotQuery, gotHeader string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/publish/v2/upload-url/for-obs":
			gotQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"ret":{"code":0},"urlInfo":{"url":"` + serverURL(r) + `/obs-put","objectId":"obs://pkg/1","method":"PUT","headers":{"x-amz-date":"20260901T000000Z","Content-Type":"application/octet-stream"}}}`))
		case "/obs-put":
			gotHeader = r.Header.Get("x-amz-date")
			body, _ := io.ReadAll(r.Body)
			uploaded = string(body)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	_ = srv

	file, err := c.UploadPackage(context.Background(), "118236677", path)
	if err != nil {
		t.Fatalf("UploadPackage: %v", err)
	}
	if uploaded != content {
		t.Fatalf("uploaded %q, want %q", uploaded, content)
	}
	if gotHeader != "20260901T000000Z" {
		t.Fatalf("pre-signed headers were not replayed, x-amz-date = %q", gotHeader)
	}
	if file.DestURL != "obs://pkg/1" || file.FileName != "app-release.aab" || file.Size != int64(len(content)) {
		t.Fatalf("unexpected file %+v", file)
	}
	for _, want := range []string{"appId=118236677", "suffix=aab", "contentLength=9", "fileName=app-release.aab"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q misses %q", gotQuery, want)
		}
	}
}

func TestUploadPackageFallsBackToMultipart(t *testing.T) {
	const content = "apk-bytes"
	path := writeTempPackage(t, "app-release.apk", content)

	var gotFields map[string]string
	var uploaded string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/publish/v2/upload-url/for-obs":
			// Accounts without the pre-signed flow answer with an error code.
			_, _ = w.Write([]byte(`{"ret":{"code":204144647,"msg":"not supported"}}`))
		case "/api/publish/v2/upload-url":
			_, _ = w.Write([]byte(`{"ret":{"code":0},"uploadUrl":"` + serverURL(r) + `/legacy-put","authCode":"AUTH"}`))
		case "/legacy-put":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm: %v", err)
				return
			}
			gotFields = map[string]string{
				"authCode":  r.FormValue("authCode"),
				"fileCount": r.FormValue("fileCount"),
				"parseType": r.FormValue("parseType"),
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("FormFile: %v", err)
				return
			}
			defer f.Close()
			body, _ := io.ReadAll(f)
			uploaded = string(body)
			_, _ = w.Write([]byte(`{"result":{"resultCode":0,"UploadFileRsp":{"ifSuccess":1,"fileInfoList":[{"fileDestUlr":"https://files/pkg.apk","size":9}]}}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	file, err := c.UploadPackage(context.Background(), "118236677", path)
	if err != nil {
		t.Fatalf("UploadPackage: %v", err)
	}
	if uploaded != content {
		t.Fatalf("uploaded %q, want %q", uploaded, content)
	}
	if gotFields["authCode"] != "AUTH" || gotFields["fileCount"] != "1" || gotFields["parseType"] != "1" {
		t.Fatalf("multipart fields %+v", gotFields)
	}
	if file.DestURL != "https://files/pkg.apk" || file.Size != 9 {
		t.Fatalf("unexpected file %+v", file)
	}
}

func TestUploadPackageReportsBothFailures(t *testing.T) {
	path := writeTempPackage(t, "app.apk", "x")
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ret":{"code":204144647,"msg":"nope"}}`))
	})
	_, err := c.UploadPackage(context.Background(), "1", path)
	if err == nil {
		t.Fatal("want an error when both flows fail")
	}
	if !strings.Contains(err.Error(), "pre-signed") || !strings.Contains(err.Error(), "multipart") {
		t.Fatalf("error %q should name both flows", err)
	}
}

func TestUploadPackageRejectsFilesWithoutExtension(t *testing.T) {
	path := writeTempPackage(t, "package", "x")
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected, got %s", r.URL.Path)
	})
	if _, err := c.UploadPackage(context.Background(), "1", path); err == nil {
		t.Fatal("want an error for a file without a suffix")
	}
}

func TestUpdateAppFileInfo(t *testing.T) {
	var gotBody map[string]any
	var gotQuery string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"ret":{"code":0},"pkgVersion":["1040000300001"]}`))
	})

	versions, err := c.UpdateAppFileInfo(context.Background(), "118236677", ReleaseFull, FileTypePackage, "",
		[]UploadedFile{{FileName: "app.aab", DestURL: "obs://pkg/1", Size: 42}})
	if err != nil {
		t.Fatalf("UpdateAppFileInfo: %v", err)
	}
	if len(versions) != 1 || versions[0] != "1040000300001" {
		t.Fatalf("versions = %v", versions)
	}
	if gotBody["fileType"] != float64(FileTypePackage) {
		t.Fatalf("fileType = %v", gotBody["fileType"])
	}
	files, _ := gotBody["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files = %v", gotBody["files"])
	}
	first, _ := files[0].(map[string]any)
	if first["fileDestUrl"] != "obs://pkg/1" || first["fileName"] != "app.aab" {
		t.Fatalf("file payload = %v", first)
	}
	if _, ok := gotBody["lang"]; ok {
		t.Fatalf("an empty lang must not be sent: %v", gotBody)
	}
	if !strings.Contains(gotQuery, "releaseType=1") {
		t.Fatalf("query %q misses the release type", gotQuery)
	}
}

func TestAABCompileStatus(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("pkgIds"); got != "1040000300001" {
			t.Errorf("pkgIds = %q", got)
		}
		_, _ = w.Write([]byte(`{"ret":{"code":0},"pkgStateList":[{"pkgId":"1040000300001","aabCompileStatus":2}]}`))
	})
	status, err := c.AABCompileStatus(context.Background(), "118236677", []string{"1040000300001"})
	if err != nil {
		t.Fatalf("AABCompileStatus: %v", err)
	}
	if status != AABCompileSuccess {
		t.Fatalf("status = %d, want %d", status, AABCompileSuccess)
	}
}

// serverURL rebuilds the base URL of the test server from an inbound request.
func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
