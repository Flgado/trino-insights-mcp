package queryinfo

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type fakeFetcher struct {
	calls int64
	qi    *QueryInfo
	err   error
}

func (f *fakeFetcher) Fetch(_ context.Context, _ string) (*QueryInfo, error) {
	atomic.AddInt64(&f.calls, 1)
	return f.qi, f.err
}

func TestNewCachedFetcher_Defaults(t *testing.T) {
	inner := &fakeFetcher{}
	c := NewCachedFetcher(inner, 0, 0)

	if c.TTL != 5*time.Minute {
		t.Errorf("TTL = %v, want 5m", c.TTL)
	}
	if c.Cap != 256 {
		t.Errorf("Cap = %d, want 256", c.Cap)
	}
	if c.NonTerminalTTL != 30*time.Second {
		t.Errorf("NonTerminalTTL = %v, want 30s", c.NonTerminalTTL)
	}
}

func TestNewCachedFetcher_CustomValues(t *testing.T) {
	c := NewCachedFetcher(&fakeFetcher{}, 10*time.Minute, 100)
	if c.TTL != 10*time.Minute {
		t.Errorf("TTL = %v, want 10m", c.TTL)
	}
	if c.Cap != 100 {
		t.Errorf("Cap = %d, want 100", c.Cap)
	}
}

func TestNewCachedFetcher_NonTerminalTTLCapped(t *testing.T) {
	c := NewCachedFetcher(&fakeFetcher{}, 10*time.Minute, 10)
	if c.NonTerminalTTL != 30*time.Second {
		t.Errorf("NonTerminalTTL = %v, want 30s cap", c.NonTerminalTTL)
	}
}

func TestNewCachedFetcher_ShortTTLNotCapped(t *testing.T) {
	c := NewCachedFetcher(&fakeFetcher{}, 10*time.Second, 10)
	if c.NonTerminalTTL != 10*time.Second {
		t.Errorf("NonTerminalTTL = %v, want 10s (not capped)", c.NonTerminalTTL)
	}
}

func TestCachingFetcher_Fetch_CachesResult(t *testing.T) {
	inner := &fakeFetcher{qi: &QueryInfo{QueryID: "q1", State: "FINISHED"}}
	c := NewCachedFetcher(inner, 5*time.Minute, 10)

	qi1, err := c.Fetch(context.Background(), "q1")
	if err != nil {
		t.Fatal(err)
	}
	qi2, err := c.Fetch(context.Background(), "q1")
	if err != nil {
		t.Fatal(err)
	}

	if qi1 != qi2 {
		t.Error("second fetch should return cached result")
	}
	if inner.calls != 1 {
		t.Errorf("inner.calls = %d, want 1 (cached)", inner.calls)
	}
}

func TestCachingFetcher_Fetch_EmptyQueryID(t *testing.T) {
	c := NewCachedFetcher(&fakeFetcher{}, 5*time.Minute, 10)
	_, err := c.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query ID")
	}
}

func TestCachingFetcher_Fetch_InnerError(t *testing.T) {
	inner := &fakeFetcher{err: fmt.Errorf("connection refused")}
	c := NewCachedFetcher(inner, 5*time.Minute, 10)

	_, err := c.Fetch(context.Background(), "q1")
	if err == nil {
		t.Fatal("expected error from inner fetcher")
	}
}

func TestCachingFetcher_Fetch_InnerError_NotCached(t *testing.T) {
	inner := &fakeFetcher{err: fmt.Errorf("timeout")}
	c := NewCachedFetcher(inner, 5*time.Minute, 10)

	c.Fetch(context.Background(), "q1")
	c.Fetch(context.Background(), "q1")

	if inner.calls != 2 {
		t.Errorf("errors should not be cached, inner.calls = %d, want 2", inner.calls)
	}
}

func TestCachingFetcher_Stats(t *testing.T) {
	inner := &fakeFetcher{qi: &QueryInfo{QueryID: "q1", State: "FINISHED"}}
	c := NewCachedFetcher(inner, 5*time.Minute, 10)

	entries, cap := c.Stats()
	if entries != 0 || cap != 10 {
		t.Errorf("Stats() = (%d, %d), want (0, 10)", entries, cap)
	}

	c.Fetch(context.Background(), "q1")
	entries, cap = c.Stats()
	if entries != 1 || cap != 10 {
		t.Errorf("Stats() = (%d, %d), want (1, 10)", entries, cap)
	}
}

func TestCachingFetcher_Eviction_WhenFull(t *testing.T) {
	inner := &fakeFetcher{qi: &QueryInfo{State: "FINISHED"}}
	c := NewCachedFetcher(inner, 5*time.Minute, 2)

	c.Fetch(context.Background(), "q1")
	c.Fetch(context.Background(), "q2")
	c.Fetch(context.Background(), "q3")

	entries, _ := c.Stats()
	if entries > 2 {
		t.Errorf("entries = %d, should not exceed cap of 2", entries)
	}
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"FINISHED", true},
		{"FAILED", true},
		{"CANCELED", true},
		{"RUNNING", false},
		{"QUEUED", false},
		{"PLANNING", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := isTerminalState(tt.state); got != tt.want {
				t.Errorf("isTerminalState(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
