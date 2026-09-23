package rustore

import "fmt"

// APIError is a non-OK answer from the RuStore Public API envelope.
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("rustore api: %s (code %s, http %d)", e.Message, e.Code, e.HTTPStatus)
	}
	return fmt.Sprintf("rustore api: code %s, http %d", e.Code, e.HTTPStatus)
}
