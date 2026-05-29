package rest

import (
	"encoding/base64"
	"net/http"
	"testing"
)

func TestBasicAuth_Apply_UserAndPassword(t *testing.T) {
	auth := BasicAuth{User: "admin", Password: "secret"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "admin" {
		t.Errorf("X-Trino-User = %q, want %q", got, "admin")
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestBasicAuth_Apply_UserOnly(t *testing.T) {
	auth := BasicAuth{User: "admin"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "admin" {
		t.Errorf("X-Trino-User = %q, want %q", got, "admin")
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:"))
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestBasicAuth_Apply_PasswordOnly(t *testing.T) {
	auth := BasicAuth{Password: "secret"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "" {
		t.Errorf("X-Trino-User should be empty, got %q", got)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(":secret"))
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestBasicAuth_Apply_Empty(t *testing.T) {
	auth := BasicAuth{}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "" {
		t.Errorf("X-Trino-User should be empty, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be empty, got %q", got)
	}
}

func TestBearerAuth_Apply_UserAndToken(t *testing.T) {
	auth := BearerAuth{User: "admin", Token: "my-jwt-token"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "admin" {
		t.Errorf("X-Trino-User = %q, want %q", got, "admin")
	}
	if got := req.Header.Get("Authorization"); got != "Bearer my-jwt-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer my-jwt-token")
	}
}

func TestBearerAuth_Apply_TokenOnly(t *testing.T) {
	auth := BearerAuth{Token: "tok"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "" {
		t.Errorf("X-Trino-User should be empty, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer tok")
	}
}

func TestBearerAuth_Apply_UserOnly(t *testing.T) {
	auth := BearerAuth{User: "admin"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "admin" {
		t.Errorf("X-Trino-User = %q, want %q", got, "admin")
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be empty, got %q", got)
	}
}

func TestBearerAuth_Apply_Empty(t *testing.T) {
	auth := BearerAuth{}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "" {
		t.Errorf("X-Trino-User should be empty, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be empty, got %q", got)
	}
}

func TestNoAuth_Apply_WithUser(t *testing.T) {
	auth := NoAuth{User: "readonly"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "readonly" {
		t.Errorf("X-Trino-User = %q, want %q", got, "readonly")
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be empty for NoAuth, got %q", got)
	}
}

func TestNoAuth_Apply_Empty(t *testing.T) {
	auth := NoAuth{}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.Apply(req)

	if got := req.Header.Get("X-Trino-User"); got != "" {
		t.Errorf("X-Trino-User should be empty, got %q", got)
	}
}

func TestAuthenticatorInterface(t *testing.T) {
	var _ Authenticator = BasicAuth{}
	var _ Authenticator = BearerAuth{}
	var _ Authenticator = NoAuth{}
}
