package rustore

import (
	"context"
	"strconv"
)

// Version is one application version with its lifecycle status.
type Version struct {
	VersionID        int64  `json:"versionId"`
	AppName          string `json:"appName"`
	AppType          string `json:"appType"`
	VersionName      string `json:"versionName"`
	VersionCode      int64  `json:"versionCode"`
	VersionStatus    string `json:"versionStatus"`
	PublishType      string `json:"publishType"`
	PublishDateTime  string `json:"publishDateTime"`
	SendDateForModer string `json:"sendDateForModer"`
	PartialValue     int    `json:"partialValue"` // -1 means 100%
	WhatsNew         string `json:"whatsNew"`
	Paid             bool   `json:"paid"`
}

// VersionsOpts filters the version list. ID and pagination are mutually
// exclusive in the API, so when ID is set page/size are not sent.
type VersionsOpts struct {
	ID   int64
	Page int
	Size int
}

// VersionsPage is one page of the version list.
type VersionsPage struct {
	Content       []Version `json:"content"`
	PageNumber    int       `json:"pageNumber"`
	PageSize      int       `json:"pageSize"`
	TotalElements int       `json:"totalElements"`
	TotalPages    int       `json:"totalPages"`
}

// Versions returns version statuses for the given package.
func (c *Client) Versions(ctx context.Context, packageName string, opts VersionsOpts) (*VersionsPage, error) {
	query := map[string]string{}
	if opts.ID != 0 {
		query["ids"] = strconv.FormatInt(opts.ID, 10)
	} else {
		query["page"] = strconv.Itoa(opts.Page)
		if opts.Size > 0 {
			query["size"] = strconv.Itoa(opts.Size)
		}
	}
	var page VersionsPage
	if err := c.get(ctx, "/public/v1/application/"+packageName+"/version", query, &page); err != nil {
		return nil, err
	}
	return &page, nil
}
