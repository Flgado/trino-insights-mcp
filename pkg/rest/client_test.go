package rest

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Flgado/trino-insights-mcp/pkg/errors"
)

func TestNewClient_OK(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "http://trino:8080"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.BaseURL() != "http://trino:8080" {
		t.Errorf("BaseURL() = %q, want %q", c.BaseURL(), "http://trino:8080")
	}
}

func TestNewClient_EmptyBaseURL(t *testing.T) {
	_, err := NewClient(Options{})
	if err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
	if !strings.Contains(err.Error(), "BaseURL is required") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestNewClient_InvalidBaseURL(t *testing.T) {
	_, err := NewClient(Options{BaseURL: "://bad"})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestNewClient_NonHTTPScheme(t *testing.T) {
	_, err := NewClient(Options{BaseURL: "ftp://trino:8080"})
	if err == nil {
		t.Fatal("expected error for non-http scheme")
	}
	if !strings.Contains(err.Error(), "http or https") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestNewClient_HTTPS(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "https://trino:8443"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.BaseURL() != "https://trino:8443" {
		t.Errorf("BaseURL() = %q", c.BaseURL())
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "http://trino:8080"})
	if err != nil {
		t.Fatal(err)
	}
	if c.http.Timeout != 15*time.Second {
		t.Errorf("default timeout = %v, want 15s", c.http.Timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "http://trino:8080", Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.http.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.http.Timeout)
	}
}

func TestNewClient_DefaultUserAgent(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "http://trino:8080"})
	if err != nil {
		t.Fatal(err)
	}
	if c.userAgent != "trino-insights-mcp/0.1" {
		t.Errorf("userAgent = %q, want default", c.userAgent)
	}
}

func TestNewClient_CustomUserAgent(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "http://trino:8080", UserAgent: "test/1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if c.userAgent != "test/1.0" {
		t.Errorf("userAgent = %q, want %q", c.userAgent, "test/1.0")
	}
}

func TestNewClient_NilAuthDefaultsToNoAuth(t *testing.T) {
	c, err := NewClient(Options{BaseURL: "http://trino:8080"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.auth.(NoAuth); !ok {
		t.Errorf("default auth should be NoAuth, got %T", c.auth)
	}
}

func TestNewClient_InsecureSkipTLS(t *testing.T) {
	_, err := NewClient(Options{
		BaseURL:               "https://trino:8443",
		InsecureSkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetJSON_OK(t *testing.T) {
	type resp struct {
		Name string `json:"name"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/query/abc") {
			t.Errorf("path = %q, want suffix /v1/query/abc", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept header = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{Name: "test"})
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	var out resp
	if err := c.GetJSON(context.Background(), "/v1/query/abc", &out); err != nil {
		t.Fatalf("GetJSON error: %v", err)
	}
	if out.Name != "test" {
		t.Errorf("Name = %q, want %q", out.Name, "test")
	}
}

func TestGetJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	err := c.GetJSON(context.Background(), "/v1/query/missing", nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !errors.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestGetJSON_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	err := c.GetJSON(context.Background(), "/v1/info", nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	var rerr *errors.TrinoRESTError
	if !stderrors.As(err, &rerr) {
		t.Fatalf("expected TrinoRESTError, got %T", err)
	}
	if rerr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", rerr.StatusCode)
	}
}

func TestGetJSON_NilOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":"ignored"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	if err := c.GetJSON(context.Background(), "/v1/status", nil); err != nil {
		t.Fatalf("unexpected error when output is nil: %v", err)
	}
}

func TestGetJSON_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	var out map[string]string
	err := c.GetJSON(context.Background(), "/v1/query/abc", &out)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetJSON_AuthHeadersApplied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trino-User") != "testuser" {
			t.Errorf("X-Trino-User = %q, want %q", r.Header.Get("X-Trino-User"), "testuser")
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("Authorization = %q, want Bearer prefix", auth)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{
		BaseURL: srv.URL,
		Auth:    BearerAuth{User: "testuser", Token: "tok123"},
	})
	var out map[string]any
	c.GetJSON(context.Background(), "/v1/info", &out)
}

func TestGetJSON_UserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "custom/2.0" {
			t.Errorf("User-Agent = %q, want %q", r.Header.Get("User-Agent"), "custom/2.0")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL, UserAgent: "custom/2.0"})
	var out map[string]any
	c.GetJSON(context.Background(), "/", &out)
}

func TestPostJSON_OK(t *testing.T) {
	type reqBody struct {
		Query string `json:"query"`
	}
	type respBody struct {
		ID string `json:"id"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		var body reqBody
		json.NewDecoder(r.Body).Decode(&body)
		if body.Query != "SELECT 1" {
			t.Errorf("body.Query = %q, want %q", body.Query, "SELECT 1")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{ID: "q123"})
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	var out respBody
	err := c.PostJSON(context.Background(), "/v1/statement", reqBody{Query: "SELECT 1"}, &out)
	if err != nil {
		t.Fatalf("PostJSON error: %v", err)
	}
	if out.ID != "q123" {
		t.Errorf("ID = %q, want %q", out.ID, "q123")
	}
}

func TestPostJSON_NilBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	var out map[string]any
	if err := c.PostJSON(context.Background(), "/v1/endpoint", nil, &out); err != nil {
		t.Fatalf("unexpected error with nil body: %v", err)
	}
}

func TestPostJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	err := c.PostJSON(context.Background(), "/v1/statement", map[string]string{"q": "bad"}, nil)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !errors.IsBadRequest(err) {
		t.Errorf("expected IsBadRequest, got %v", err)
	}
}

func TestGetJSON_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{BaseURL: srv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.GetJSON(ctx, "/v1/slow", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
