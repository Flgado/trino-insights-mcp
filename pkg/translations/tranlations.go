package translations

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type HelperFunc func(key, fallback string) string

func NewHelper() (HelperFunc, func()) {
	overrides := loadEnvOvrrides()
	var mu sync.Mutex
	seen := map[string]string{}

	helper := func(key, fallback string) string {
		mu.Lock()
		defer mu.Unlock()
		if v, ok := overrides[key]; ok {
			seen[key] = v
			return v
		}
		seen[key] = fallback
		return fallback
	}

	dump := func() {
		mu.Lock()
		defer mu.Unlock()
		data, err := json.MarshalIndent(seen, "", "  ")
		if err != nil {
			return
		}
		_ = os.WriteFile("translations.json", data, 0o644)
	}
	return helper, dump
}

const envPrefix = "TRINO_INSIGHTS_DESC_"

func loadEnvOvrrides() map[string]string {
	m := map[string]string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, envPrefix) {
			continue
		}
		eq := strings.IndexByte(e, '=')
		if eq <= len(envPrefix) {
			continue
		}
		key := e[len(envPrefix):eq]
		val := e[eq+1:]
		m[key] = val
	}
	return m
}
