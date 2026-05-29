package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{
		Coordinator: Coordinator{
			BaseURL: "http://trino:8080",
			User:    "admin",
			Timeout: 10 * time.Second,
		},
		ContentWindowSize:  1024,
		QueryInfoCacheTTL:  5 * time.Minute,
		QueryInfoCacheSize: 100,
	}
}

func TestValidate_OK(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidate_EmptyBaseURL(t *testing.T) {
	for _, url := range []string{"", "   ", "\t"} {
		c := validConfig()
		c.Coordinator.BaseURL = url
		err := c.Validate()
		if err == nil {
			t.Fatalf("expected error for BaseURL=%q", url)
		}
		if !strings.Contains(err.Error(), "coordinator URL is required") {
			t.Errorf("unexpected message: %v", err)
		}
	}
}

func TestValidate_PasswordAndTokenMutuallyExclusive(t *testing.T) {
	c := validConfig()
	c.Coordinator.Password = "pass"
	c.Coordinator.Token = "tok"
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error when both password and token set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestValidate_PasswordAloneOK(t *testing.T) {
	c := validConfig()
	c.Coordinator.Password = "secret"
	if err := c.Validate(); err != nil {
		t.Fatalf("password alone should be valid, got %v", err)
	}
}

func TestValidate_TokenAloneOK(t *testing.T) {
	c := validConfig()
	c.Coordinator.Token = "bearer-token"
	if err := c.Validate(); err != nil {
		t.Fatalf("token alone should be valid, got %v", err)
	}
}

func TestValidate_ContentWindowSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"negative", -1, true},
		{"zero", 0, false},
		{"valid", 1024, false},
		{"max", 64 * 1024, false},
		{"over max", 64*1024 + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			c.ContentWindowSize = tt.size
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ContentWindowSize=%d: err=%v, wantErr=%v", tt.size, err, tt.wantErr)
			}
		})
	}
}

func TestValidate_QueryInfoCacheTTL_Negative(t *testing.T) {
	c := validConfig()
	c.QueryInfoCacheTTL = -1
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for negative TTL")
	}
	if !strings.Contains(err.Error(), "cache TTL") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestValidate_QueryInfoCacheTTL_Zero(t *testing.T) {
	c := validConfig()
	c.QueryInfoCacheTTL = 0
	if err := c.Validate(); err != nil {
		t.Fatalf("TTL=0 should be valid (disables caching), got %v", err)
	}
}

func TestValidate_QueryInfoCacheSize_Zero(t *testing.T) {
	c := validConfig()
	c.QueryInfoCacheSize = 0
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for cache size 0")
	}
	if !strings.Contains(err.Error(), "cache size") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestValidate_QueryInfoCacheSize_Negative(t *testing.T) {
	c := validConfig()
	c.QueryInfoCacheSize = -5
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for negative cache size")
	}
}

func TestValidate_Timeout_Zero(t *testing.T) {
	c := validConfig()
	c.Coordinator.Timeout = 0
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for zero timeout")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestValidate_Timeout_Negative(t *testing.T) {
	c := validConfig()
	c.Coordinator.Timeout = -1 * time.Second
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestValidate_LogFile_Dash(t *testing.T) {
	c := validConfig()
	c.LogFile = "-"
	if err := c.Validate(); err != nil {
		t.Fatalf("LogFile='-' should be valid (stdout), got %v", err)
	}
}

func TestValidate_LogFile_Empty(t *testing.T) {
	c := validConfig()
	c.LogFile = ""
	if err := c.Validate(); err != nil {
		t.Fatalf("empty LogFile should be valid, got %v", err)
	}
}

func TestValidate_LogFile_Exists(t *testing.T) {
	f, err := os.CreateTemp("", "trino-test-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	c := validConfig()
	c.LogFile = f.Name()
	if err := c.Validate(); err != nil {
		t.Fatalf("existing log file should be valid, got %v", err)
	}
}

func TestValidate_LogFile_NotExists(t *testing.T) {
	c := validConfig()
	c.LogFile = "/tmp/trino-insights-test-nonexistent-file-abc123.log"
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for nonexistent log file")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestWithDefaults_SetsAllDefaults(t *testing.T) {
	c := (&Config{}).WithDefaults()

	if c.Coordinator.User != "insights" {
		t.Errorf("User = %q, want %q", c.Coordinator.User, "insights")
	}
	if c.Coordinator.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want %v", c.Coordinator.Timeout, 15*time.Second)
	}
	if c.ContentWindowSize != 16*1024 {
		t.Errorf("ContentWindowSize = %d, want %d", c.ContentWindowSize, 16*1024)
	}
	if c.QueryInfoCacheTTL != 5*time.Minute {
		t.Errorf("QueryInfoCacheTTL = %v, want %v", c.QueryInfoCacheTTL, 5*time.Minute)
	}
	if c.QueryInfoCacheSize != 256 {
		t.Errorf("QueryInfoCacheSize = %d, want %d", c.QueryInfoCacheSize, 256)
	}
}

func TestWithDefaults_PreservesExplicitValues(t *testing.T) {
	c := &Config{
		Coordinator: Coordinator{
			User:    "custom-user",
			Timeout: 30 * time.Second,
		},
		ContentWindowSize:  8192,
		QueryInfoCacheTTL:  10 * time.Minute,
		QueryInfoCacheSize: 512,
	}
	c.WithDefaults()

	if c.Coordinator.User != "custom-user" {
		t.Errorf("User overwritten: %q", c.Coordinator.User)
	}
	if c.Coordinator.Timeout != 30*time.Second {
		t.Errorf("Timeout overwritten: %v", c.Coordinator.Timeout)
	}
	if c.ContentWindowSize != 8192 {
		t.Errorf("ContentWindowSize overwritten: %d", c.ContentWindowSize)
	}
	if c.QueryInfoCacheTTL != 10*time.Minute {
		t.Errorf("QueryInfoCacheTTL overwritten: %v", c.QueryInfoCacheTTL)
	}
	if c.QueryInfoCacheSize != 512 {
		t.Errorf("QueryInfoCacheSize overwritten: %d", c.QueryInfoCacheSize)
	}
}

func TestWithDefaults_ReturnsSelf(t *testing.T) {
	c := &Config{}
	got := c.WithDefaults()
	if got != c {
		t.Error("WithDefaults should return the same pointer for chaining")
	}
}
