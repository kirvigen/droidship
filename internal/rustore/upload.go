package rustore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
)

// newFileUpload builds a streaming multipart/form-data body with a single
// "file" part. The Content-Length is computed up front (head + file + tail),
// so the request is not sent chunked. closeFn releases the underlying file.
func newFileUpload(path string) (contentType string, contentLength int64, body io.Reader, closeFn func() error, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return "", 0, nil, nil, err
	}

	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	if _, err := mw.CreateFormFile("file", filepath.Base(path)); err != nil {
		f.Close()
		return "", 0, nil, nil, err
	}
	tail := fmt.Sprintf("\r\n--%s--\r\n", mw.Boundary())

	contentLength = int64(head.Len()) + st.Size() + int64(len(tail))
	body = io.MultiReader(&head, f, bytes.NewReader([]byte(tail)))
	return mw.FormDataContentType(), contentLength, body, f.Close, nil
}

// UploadAPK uploads an .apk into a draft version. isMain marks the main APK
// (required by the API); servicesType is e.g. "HMS" and omitted when empty.
func (c *Client) UploadAPK(ctx context.Context, packageName string, versionID int64, path string, isMain bool, servicesType string) error {
	query := map[string]string{"isMainApk": strconv.FormatBool(isMain)}
	if servicesType != "" {
		query["servicesType"] = servicesType
	}
	urlPath := fmt.Sprintf("/public/v1/application/%s/version/%d/apk", packageName, versionID)
	return c.uploadFile(ctx, urlPath, query, path)
}

// UploadAAB uploads an .aab into a draft version. Requires the signing key to
// be set up in the RuStore Console.
func (c *Client) UploadAAB(ctx context.Context, packageName string, versionID int64, path string) error {
	urlPath := fmt.Sprintf("/public/v1/application/%s/version/%d/aab", packageName, versionID)
	return c.uploadFile(ctx, urlPath, nil, path)
}

func (c *Client) uploadFile(ctx context.Context, urlPath string, query map[string]string, filePath string) error {
	contentType, length, body, closeFn, err := newFileUpload(filePath)
	if err != nil {
		return err
	}
	defer closeFn()
	return c.do(ctx, "POST", urlPath, query, body, contentType, length, nil)
}
