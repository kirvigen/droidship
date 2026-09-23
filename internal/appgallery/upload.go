package appgallery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FileType values for app-file-info. 5 is the app package (APK/AAB).
const FileTypePackage = 5

// aab compilation states reported by the aab status endpoint.
const (
	AABCompileProcessing = 1
	AABCompileSuccess    = 2
)

// UploadedFile is a file that lives on Huawei storage and can be attached to an
// app version.
type UploadedFile struct {
	FileName string `json:"fileName"`
	DestURL  string `json:"fileDestUrl"`
	Size     int64  `json:"size"`
}

// UploadPackage uploads an APK or AAB and returns the reference to attach with
// UpdateAppFileInfo.
//
// Huawei offers two upload flows: the newer one hands out a pre-signed object
// storage URL, the older one takes a multipart POST. Accounts differ in which
// one is enabled, so the pre-signed flow is tried first and the multipart flow
// serves as the fallback.
func (c *Client) UploadPackage(ctx context.Context, appID, path string) (UploadedFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return UploadedFile{}, err
	}
	name := filepath.Base(path)
	suffix := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if suffix == "" {
		return UploadedFile{}, fmt.Errorf("cannot tell the file type of %q: expected .apk or .aab", name)
	}

	file, obsErr := c.uploadViaObjectStorage(ctx, appID, path, name, suffix, info.Size())
	if obsErr == nil {
		return file, nil
	}
	file, legacyErr := c.uploadViaMultipart(ctx, appID, path, name, suffix)
	if legacyErr == nil {
		return file, nil
	}
	return UploadedFile{}, fmt.Errorf("upload failed: pre-signed flow: %v; multipart flow: %w", obsErr, legacyErr)
}

// uploadViaObjectStorage asks for a pre-signed URL and PUTs the file to it.
func (c *Client) uploadViaObjectStorage(ctx context.Context, appID, path, name, suffix string, size int64) (UploadedFile, error) {
	var body struct {
		URLInfo struct {
			URL      string            `json:"url"`
			ObjectID string            `json:"objectId"`
			Method   string            `json:"method"`
			Headers  map[string]string `json:"headers"`
		} `json:"urlInfo"`
	}
	q := url.Values{
		"appId":         {appID},
		"fileName":      {name},
		"contentLength": {strconv.FormatInt(size, 10)},
		"suffix":        {suffix},
	}
	if err := c.get(ctx, "/api/publish/v2/upload-url/for-obs", q, &body); err != nil {
		return UploadedFile{}, err
	}
	if body.URLInfo.URL == "" || body.URLInfo.ObjectID == "" {
		return UploadedFile{}, fmt.Errorf("upload-url/for-obs returned no url")
	}

	file, err := os.Open(path)
	if err != nil {
		return UploadedFile{}, err
	}
	defer file.Close()

	method := body.URLInfo.Method
	if method == "" {
		method = http.MethodPut
	}
	req, err := http.NewRequestWithContext(ctx, method, body.URLInfo.URL, file)
	if err != nil {
		return UploadedFile{}, err
	}
	req.ContentLength = size
	for k, v := range body.URLInfo.Headers {
		// Go carries the Host header on the request itself, not in the map.
		if strings.EqualFold(k, "Host") {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return UploadedFile{}, fmt.Errorf("upload %s: %w", name, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return UploadedFile{}, fmt.Errorf("upload %s: http %d: %s", name, resp.StatusCode, snippet(raw))
	}
	return UploadedFile{FileName: name, DestURL: body.URLInfo.ObjectID, Size: size}, nil
}

// uploadViaMultipart uses the older upload endpoint that takes a multipart POST
// authorized by a one-shot auth code.
func (c *Client) uploadViaMultipart(ctx context.Context, appID, path, name, suffix string) (UploadedFile, error) {
	var slot struct {
		UploadURL string `json:"uploadUrl"`
		AuthCode  string `json:"authCode"`
	}
	q := url.Values{"appId": {appID}, "suffix": {suffix}}
	if err := c.get(ctx, "/api/publish/v2/upload-url", q, &slot); err != nil {
		return UploadedFile{}, err
	}
	if slot.UploadURL == "" || slot.AuthCode == "" {
		return UploadedFile{}, fmt.Errorf("upload-url returned no upload slot")
	}

	file, err := os.Open(path)
	if err != nil {
		return UploadedFile{}, err
	}
	defer file.Close()

	// Stream the body: an app package is too big to hold in memory twice.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var werr error
		defer func() { pw.CloseWithError(werr) }()
		fields := [][2]string{{"authCode", slot.AuthCode}, {"fileCount", "1"}, {"parseType", "1"}}
		for _, f := range fields {
			if werr = mw.WriteField(f[0], f[1]); werr != nil {
				return
			}
		}
		var part io.Writer
		if part, werr = mw.CreateFormFile("file", name); werr != nil {
			return
		}
		if _, werr = io.Copy(part, file); werr != nil {
			return
		}
		werr = mw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, slot.UploadURL, pr)
	if err != nil {
		return UploadedFile{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return UploadedFile{}, fmt.Errorf("upload %s: %w", name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return UploadedFile{}, fmt.Errorf("upload %s: read response: %w", name, err)
	}
	if resp.StatusCode >= 300 {
		return UploadedFile{}, fmt.Errorf("upload %s: http %d: %s", name, resp.StatusCode, snippet(raw))
	}

	var body struct {
		Result struct {
			ResultCode *int `json:"resultCode"`
			UploadFile struct {
				IfSuccess    int `json:"ifSuccess"`
				FileInfoList []struct {
					// Huawei really does spell it "Ulr" here.
					DestURL flexStr `json:"fileDestUlr"`
					Size    flexInt `json:"size"`
				} `json:"fileInfoList"`
			} `json:"UploadFileRsp"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return UploadedFile{}, fmt.Errorf("upload %s: unexpected response: %s", name, snippet(raw))
	}
	if body.Result.ResultCode != nil && *body.Result.ResultCode != 0 {
		return UploadedFile{}, fmt.Errorf("upload %s: result code %d: %s", name, *body.Result.ResultCode, snippet(raw))
	}
	if body.Result.UploadFile.IfSuccess != 1 || len(body.Result.UploadFile.FileInfoList) == 0 {
		return UploadedFile{}, fmt.Errorf("upload %s: rejected by the file server: %s", name, snippet(raw))
	}
	first := body.Result.UploadFile.FileInfoList[0]
	return UploadedFile{FileName: name, DestURL: first.DestURL.String(), Size: first.Size.Int()}, nil
}

// UpdateAppFileInfo attaches uploaded files to the app version and returns the
// package versions AppGallery assigned to them.
func (c *Client) UpdateAppFileInfo(ctx context.Context, appID string, releaseType, fileType int, lang string, files []UploadedFile) ([]string, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to attach")
	}
	payload := struct {
		FileType int            `json:"fileType"`
		Lang     string         `json:"lang,omitempty"`
		Files    []UploadedFile `json:"files"`
	}{FileType: fileType, Lang: lang, Files: files}

	q := url.Values{"appId": {appID}}
	if releaseType != 0 {
		q.Set("releaseType", strconv.Itoa(releaseType))
	}
	var body struct {
		PkgVersion []flexStr `json:"pkgVersion"`
	}
	if err := c.putJSON(ctx, "/api/publish/v2/app-file-info", q, payload, &body); err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(body.PkgVersion))
	for _, v := range body.PkgVersion {
		versions = append(versions, v.String())
	}
	return versions, nil
}

// AABCompileStatus reports how far AppGallery got compiling an uploaded AAB.
func (c *Client) AABCompileStatus(ctx context.Context, appID string, pkgIDs []string) (int, error) {
	if len(pkgIDs) == 0 {
		return 0, fmt.Errorf("no package ids to check")
	}
	q := url.Values{"appId": {appID}, "pkgIds": {strings.Join(pkgIDs, ",")}}
	var body struct {
		PkgStateList []struct {
			PkgID  flexStr `json:"pkgId"`
			Status flexInt `json:"aabCompileStatus"`
		} `json:"pkgStateList"`
	}
	// The path really is misspelled "complile" in the AppGallery Connect API.
	if err := c.get(ctx, "/api/publish/v2/aab/complile/status", q, &body); err != nil {
		return 0, err
	}
	if len(body.PkgStateList) == 0 {
		return 0, fmt.Errorf("aab status: no state for %s", strings.Join(pkgIDs, ","))
	}
	return int(body.PkgStateList[0].Status.Int()), nil
}
