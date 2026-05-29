package queryinfo

import (
	"context"
	"sync"
	"time"
)

type CachingFetcher struct {
	Inner          Fetcher
	TTL            time.Duration
	NonTerminalTTL time.Duration
	Cap            int
	mu             sync.Mutex
	cache          map[string]cacheEntry
}

type cacheEntry struct {
	qi      *QueryInfo
	expires time.Time
}

const nonTerminalTTLCap = 30 * time.Second

func NewCachedFetcher(inner Fetcher, ttl time.Duration, maxEntries int) *CachingFetcher {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	if maxEntries <= 0 {
		maxEntries = 256
	}

	nonTerm := ttl

	if nonTerm > nonTerminalTTLCap {
		nonTerm = nonTerminalTTLCap
	}

	return &CachingFetcher{
		Inner:          inner,
		TTL:            ttl,
		NonTerminalTTL: nonTerm,
		Cap:            maxEntries,
		cache:          make(map[string]cacheEntry, maxEntries),
	}
}

func (c *CachingFetcher) Fetch(ctx context.Context, queryID string) (*QueryInfo, error) {
	if queryID == "" {
		return nil, errMissingQueryID
	}

	now := time.Now()
	c.mu.Lock()
	if e, ok := c.cache[queryID]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.qi, nil
	}
	c.mu.Unlock()

	qi, err := c.Inner.Fetch(ctx, queryID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= c.Cap {
		c.evictOldestLocked(now)
	}

	c.cache[queryID] = cacheEntry{qi: qi, expires: now.Add(c.ttlFor(qi))}
	return qi, nil
}

func (c *CachingFetcher) ttlFor(qi *QueryInfo) time.Duration {
	if qi != nil && isTerminalState(qi.State) {
		return c.TTL
	}

	return c.NonTerminalTTL
}

func isTerminalState(state string) bool {
	switch state {
	case "FINISHED", "FAILED", "CANCELED":
		return true
	default:
		return false
	}
}

func (c *CachingFetcher) Stats() (entries, capacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache), c.Cap
}

func (c *CachingFetcher) evictOldestLocked(now time.Time) {
	for k, e := range c.cache {
		if now.After(e.expires) {
			delete(c.cache, k)
		}
	}
	if len(c.cache) < c.Cap {
		return
	}

	// still full, evict th one clossest to expiry
	var oldestK string
	var oldestT time.Time
	for k, e := range c.cache {
		if oldestK == "" || e.expires.Before(oldestT) {
			oldestK = k
			oldestT = e.expires
		}
	}
	if oldestK != "" {
		delete(c.cache, oldestK)
	}
}
