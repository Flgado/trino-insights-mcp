package queryinfo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/rest"
)

func TestRestFetcher_Fetch_OK(t *testing.T) {
	qi := QueryInfo{QueryID: "abc123", State: "FINISHED"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/query/abc123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qi)
	}))
	defer srv.Close()

	client, _ := rest.NewClient(rest.Options{BaseURL: srv.URL})
	fetcher := &RestFetcher{Client: client}

	result, err := fetcher.Fetch(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryID != "abc123" {
		t.Errorf("QueryID = %q, want %q", result.QueryID, "abc123")
	}
	if result.State != "FINISHED" {
		t.Errorf("State = %q, want %q", result.State, "FINISHED")
	}
}

func TestRestFetcher_Fetch_EmptyQueryID(t *testing.T) {
	client, _ := rest.NewClient(rest.Options{BaseURL: "http://localhost:1"})
	fetcher := &RestFetcher{Client: client}

	_, err := fetcher.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query ID")
	}
	if err.Error() != "query ID is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRestFetcher_Fetch_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client, _ := rest.NewClient(rest.Options{BaseURL: srv.URL})
	fetcher := &RestFetcher{Client: client}

	_, err := fetcher.Fetch(context.Background(), "expired_query")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error should mention expiration, got: %v", err)
	}
}

func TestRestFetcher_Fetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client, _ := rest.NewClient(rest.Options{BaseURL: srv.URL})
	fetcher := &RestFetcher{Client: client}

	_, err := fetcher.Fetch(context.Background(), "query123")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestSentinelError(t *testing.T) {
	var err error = errMissingQueryID
	if err.Error() != "query ID is required" {
		t.Errorf("Error() = %q", err.Error())
	}
}
