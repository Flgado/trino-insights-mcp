package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type TrinoRESTError struct {
	Op         string
	URL        string
	StatusCode int
	Body       string
	Err        error
}

func NewTrinoRESTError(op, url string, statusCode int, body string, err error) *TrinoRESTError {
	return &TrinoRESTError{Op: op, URL: url, StatusCode: statusCode, Body: body, Err: err}
}

func (e *TrinoRESTError) Error() string {
	if e == nil {
		return "<nil TrinoRestError>"
	}

	switch {
	case e.StatusCode != 0:
		return fmt.Sprintf("trino rest %s failed: %d %s", e.Op, e.StatusCode, http.StatusText(e.StatusCode))
	case e.Err != nil:
		return fmt.Sprintf("trino rest %s failed: %s", e.Op, e.Err)
	default:
		return fmt.Sprintf("trino rest %s failed", e.Op)
	}
}

func (e *TrinoRESTError) Unwrap() error {
	return e.Err
}

func IsNotFound(err error) bool {
	var rerr *TrinoRESTError
	if errors.As(err, &rerr) {
		return rerr.StatusCode == http.StatusNotFound
	}
	return false
}

func IsBadRequest(err error) bool {
	var rerr *TrinoRESTError
	if errors.As(err, &rerr) {
		return rerr.StatusCode == http.StatusBadRequest
	}
	return false
}

func IsUnauthorized(err error) bool {
	var rerr *TrinoRESTError
	if errors.As(err, &rerr) {
		return rerr.StatusCode == http.StatusUnauthorized || rerr.StatusCode == http.StatusForbidden
	}
	return false
}
