package errors

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestNewTrinoRESTError(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := NewTrinoRESTError("GET", "http://trino:8080/v1/query", 500, `{"error":"boom"}`, inner)

	if e.Op != "GET" {
		t.Errorf("Op = %q, want %q", e.Op, "GET")
	}
	if e.URL != "http://trino:8080/v1/query" {
		t.Errorf("URL = %q, want %q", e.URL, "http://trino:8080/v1/query")
	}
	if e.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", e.StatusCode)
	}
	if e.Body != `{"error":"boom"}` {
		t.Errorf("Body = %q, want %q", e.Body, `{"error":"boom"}`)
	}
	if e.Err != inner {
		t.Errorf("Err = %v, want %v", e.Err, inner)
	}
}

func TestTrinoRESTError_Error_WithStatusCode(t *testing.T) {
	e := &TrinoRESTError{Op: "GET", StatusCode: http.StatusNotFound}
	got := e.Error()
	want := "trino rest GET failed: 404 Not Found"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestTrinoRESTError_Error_WithInnerError(t *testing.T) {
	inner := fmt.Errorf("dial tcp: connection refused")
	e := &TrinoRESTError{Op: "POST", Err: inner}
	got := e.Error()
	want := "trino rest POST failed: dial tcp: connection refused"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestTrinoRESTError_Error_NoStatusNoErr(t *testing.T) {
	e := &TrinoRESTError{Op: "DELETE"}
	got := e.Error()
	want := "trino rest DELETE failed"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestTrinoRESTError_Error_Nil(t *testing.T) {
	var e *TrinoRESTError
	got := e.Error()
	want := "<nil TrinoRestError>"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestTrinoRESTError_Error_StatusCodeTakesPrecedence(t *testing.T) {
	inner := fmt.Errorf("some error")
	e := &TrinoRESTError{Op: "GET", StatusCode: 503, Err: inner}
	got := e.Error()
	want := "trino rest GET failed: 503 Service Unavailable"
	if got != want {
		t.Errorf("Error() = %q, want %q (status code should take precedence over inner error)", got, want)
	}
}

func TestTrinoRESTError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("root cause")
	e := &TrinoRESTError{Op: "GET", Err: inner}
	if e.Unwrap() != inner {
		t.Errorf("Unwrap() = %v, want %v", e.Unwrap(), inner)
	}
}

func TestTrinoRESTError_Unwrap_Nil(t *testing.T) {
	e := &TrinoRESTError{Op: "GET"}
	if e.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil", e.Unwrap())
	}
}

func TestTrinoRESTError_ImplementsErrorInterface(t *testing.T) {
	var _ error = &TrinoRESTError{}
}

func TestTrinoRESTError_ErrorsAs(t *testing.T) {
	inner := NewTrinoRESTError("GET", "/v1/query", 404, "", nil)
	wrapped := fmt.Errorf("wrapped: %w", inner)

	var target *TrinoRESTError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find TrinoRESTError in chain")
	}
	if target.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", target.StatusCode)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"404 error", NewTrinoRESTError("GET", "/", http.StatusNotFound, "", nil), true},
		{"wrapped 404", fmt.Errorf("wrapped: %w", NewTrinoRESTError("GET", "/", http.StatusNotFound, "", nil)), true},
		{"500 error", NewTrinoRESTError("GET", "/", http.StatusInternalServerError, "", nil), false},
		{"non-REST error", fmt.Errorf("plain error"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"400 error", NewTrinoRESTError("POST", "/", http.StatusBadRequest, "", nil), true},
		{"wrapped 400", fmt.Errorf("wrapped: %w", NewTrinoRESTError("POST", "/", http.StatusBadRequest, "", nil)), true},
		{"500 error", NewTrinoRESTError("POST", "/", http.StatusInternalServerError, "", nil), false},
		{"non-REST error", fmt.Errorf("plain error"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBadRequest(tt.err); got != tt.want {
				t.Errorf("IsBadRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUnauthorized(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"401 error", NewTrinoRESTError("GET", "/", http.StatusUnauthorized, "", nil), true},
		{"403 error", NewTrinoRESTError("GET", "/", http.StatusForbidden, "", nil), true},
		{"wrapped 401", fmt.Errorf("wrapped: %w", NewTrinoRESTError("GET", "/", http.StatusUnauthorized, "", nil)), true},
		{"wrapped 403", fmt.Errorf("wrapped: %w", NewTrinoRESTError("GET", "/", http.StatusForbidden, "", nil)), true},
		{"404 error", NewTrinoRESTError("GET", "/", http.StatusNotFound, "", nil), false},
		{"non-REST error", fmt.Errorf("plain error"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnauthorized(tt.err); got != tt.want {
				t.Errorf("IsUnauthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}
