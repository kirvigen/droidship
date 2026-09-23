package play

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Bundles lists the app bundles already uploaded to this app.
func (c *Client) Bundles(ctx context.Context, pkg, editID string) ([]Bundle, error) {
	var res struct {
		Bundles []Bundle `json:"bundles"`
	}
	if err := c.get(ctx, editPath(pkg, editID)+"/bundles", nil, &res); err != nil {
		return nil, err
	}
	return res.Bundles, nil
}

// UploadBundle uploads an .aab and returns the bundle Play recorded for it.
// Progress is reported to progress, which may be nil.
func (c *Client) UploadBundle(ctx context.Context, pkg, editID, path string, progress io.Writer) (*Bundle, error) {
	upath := "/upload" + editPath(pkg, editID) + "/bundles"
	q := url.Values{"uploadType": {"media"}}

	var b Bundle
	if err := c.uploadFile(ctx, upath, q, path, "application/octet-stream", progress, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// UploadDeobfuscationFile attaches a ProGuard/R8 mapping (or native debug
// symbols) to an uploaded bundle so Play can deobfuscate crash reports.
// fileType is "proguard" or "nativeCode".
func (c *Client) UploadDeobfuscationFile(ctx context.Context, pkg, editID string, versionCode int, fileType, path string, progress io.Writer) error {
	upath := "/upload" + editPath(pkg, editID) + "/bundles/" + strconv.Itoa(versionCode) +
		"/deobfuscationFiles/" + url.PathEscape(fileType)
	q := url.Values{"uploadType": {"media"}}
	return c.uploadFile(ctx, upath, q, path, "application/octet-stream", progress, nil)
}

// uploadFile streams a local file as the raw request body.
func (c *Client) uploadFile(ctx context.Context, path string, query url.Values, filePath, contentType string, progress io.Writer, out any) error {
	token, err := c.Token(ctx)
	if err != nil {
		return err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", filePath, err)
	}

	var body io.Reader = f
	if progress != nil {
		body = &progressReader{r: f, total: info.Size(), out: progress, now: c.now, last: c.now()}
	}

	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = info.Size()

	err = c.send(req, http.MethodPost, path, out)
	if progress != nil {
		fmt.Fprintln(progress)
	}
	return err
}

// progressReader prints an upload progress line at most once a second.
type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	out   io.Writer
	now   func() time.Time
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if now := p.now(); now.Sub(p.last) >= time.Second || (err == io.EOF && p.read == p.total) {
		p.last = now
		pct := 0.0
		if p.total > 0 {
			pct = float64(p.read) / float64(p.total) * 100
		}
		fmt.Fprintf(p.out, "\r  %s / %s (%.0f%%)   ", humanBytes(p.read), humanBytes(p.total), pct)
	}
	return n, err
}

// humanBytes formats a byte count the way a person reads it.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
