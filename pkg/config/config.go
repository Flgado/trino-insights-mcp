package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Coordinator struct {
	BaseURL               string
	User                  string
	Password              string
	Token                 string
	InsecureSkipTLSVerify bool
	Timeout               time.Duration
}

type Config struct {
	Coordinator       Coordinator
	Toolsets          []string // nil/empty = use the per-toolset Default:true flag as the source of truth
	Tools             []string // additive opt-ins
	ExcludeTools      []string // hard kill switch
	ReadOnly          bool
	ContentWindowSize int

	// QueryInfoCacheTTL controll how long /v1/query/{id} responses are cached.
	// Their data is immutablle, so the cache is just trading memory for
	// fewer 1-50Mib Json fetches. Live queries (RUNNING/QUEUED/...) are
	// always capped internally at 30s so the agent never sees stale stats.
	// 0 disable caching entirely. CLI default: 5m.
	QueryInfoCacheTTL  time.Duration
	QueryInfoCacheSize int
	LogFile            string

	// Version is set by the CLI from internal/timpc.Version ( the single source of truth)
	// and propagated into log lines, the user_agent header
	// sent to Trino, and the MCP implementation handshake. No defaulted.
	Version string
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Coordinator.BaseURL) == "" {
		return fmt.Errorf("coordinator URL is required (set --coordinator-url or TRINO_INSIGHTS_COORDINATOR_URL)")
	}

	if c.Coordinator.Password != "" && c.Coordinator.Token != "" {
		return fmt.Errorf("password and token are mutually exclusive (set one or the other)")
	}

	if c.ContentWindowSize < 0 || c.ContentWindowSize > 64*1024 {
		return fmt.Errorf("content window size must be between 0 and 64kiB (set --content-window-size)")
	}

	if c.QueryInfoCacheTTL < 0 {
		return fmt.Errorf("query info cache TTL must be greater than 0 (set --queryinfo-cache-ttl)")
	}

	if c.QueryInfoCacheSize <= 0 {
		return fmt.Errorf("query info cache size must be greater than 0 (set --queryinfo-cache-size)")
	}

	if c.Coordinator.Timeout <= 0 {
		return fmt.Errorf("coordinator timeout must be greater than 0 (set --timeout)")
	}

	if c.LogFile != "" && c.LogFile != "-" {
		if _, err := os.Stat(c.LogFile); err != nil {
			return fmt.Errorf("log file %q does not exist (set --log-file)", c.LogFile)
		}
	}

	return nil
}

func (c *Config) WithDefaults() *Config {
	if c.Coordinator.User == "" {
		c.Coordinator.User = "insights"
	}

	if c.Coordinator.Timeout == 0 {
		c.Coordinator.Timeout = 15 * time.Second
	}

	if c.ContentWindowSize == 0 {
		c.ContentWindowSize = 16 * 1024
	}

	if c.QueryInfoCacheTTL == 0 {
		c.QueryInfoCacheTTL = 5 * time.Minute
	}

	if c.QueryInfoCacheSize == 0 {
		c.QueryInfoCacheSize = 256
	}

	return c
}
