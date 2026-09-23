package appgallery

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// APIError is a non-zero result code from the AppGallery Connect API.
//
// The Publishing API answers with {"ret":{"code":N,"msg":"…"}} and the Reviews
// API with {"ret":{"rtnCode":N,"rtnDesc":"…"}}; both land here.
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
	Path       string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("appgallery api: %s (code %d, http %d)", e.Message, e.Code, e.HTTPStatus)
	}
	return fmt.Sprintf("appgallery api: code %d, http %d", e.Code, e.HTTPStatus)
}

// codePackageProcessing is returned by app-submit while AppGallery is still
// processing the package that was just uploaded.
const codePackageProcessing = 204144660

// IsProcessing reports whether the API refused the submission only because the
// uploaded package has not finished processing yet. Such a call is worth a retry.
func IsProcessing(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == codePackageProcessing ||
		strings.Contains(apiErr.Message, "It may take 2-5 minutes")
}

// ret is the result envelope shared by every AppGallery Connect endpoint.
type ret struct {
	Code    *int   `json:"code"`
	Msg     string `json:"msg"`
	RtnCode *int   `json:"rtnCode"`
	RtnDesc string `json:"rtnDesc"`
}

// code returns the result code and whether the envelope carried one at all.
func (r ret) code() (int, bool) {
	switch {
	case r.Code != nil:
		return *r.Code, true
	case r.RtnCode != nil:
		return *r.RtnCode, true
	default:
		return 0, false
	}
}

func (r ret) message() string {
	if r.Msg != "" {
		return r.Msg
	}
	return r.RtnDesc
}

// flexInt is an integer that the API may send as a JSON number or as a string.
type flexInt int64

func (f *flexInt) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Some numeric fields arrive as floats ("4.0"); keep the integer part.
		v, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return fmt.Errorf("not a number: %s", data)
		}
		n = int64(v)
	}
	*f = flexInt(n)
	return nil
}

func (f flexInt) Int() int64 { return int64(f) }

func (f flexInt) MarshalJSON() ([]byte, error) { return json.Marshal(int64(f)) }

// flexBool is a boolean that the API may send as true/false, 0/1 or a string.
type flexBool bool

func (f *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	switch strings.ToLower(s) {
	case "", "null", "false", "0":
		*f = false
	case "true":
		*f = true
	default:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("not a boolean: %s", data)
		}
		*f = n != 0
	}
	return nil
}

func (f flexBool) Bool() bool { return bool(f) }

func (f flexBool) MarshalJSON() ([]byte, error) { return json.Marshal(bool(f)) }

// flexStr is a string that the API may send as a JSON string or as a number.
type flexStr string

func (f *flexStr) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*f = flexStr(str)
		return nil
	}
	*f = flexStr(s)
	return nil
}

func (f flexStr) String() string { return string(f) }

func (f flexStr) MarshalJSON() ([]byte, error) { return json.Marshal(string(f)) }
