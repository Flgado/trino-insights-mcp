package queryinfo

import (
	"strconv"
	"strings"
)

// ParseDurationMs parses Trino's human-readable duration strings into milliseconds.
// Formats: "0.00ns", "1.23us", "4.56ms", "7.89s", "1.23m", "0.50h", "1.00d".
// Returns 0 for empty or unparseable strings.
func ParseDurationMs(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	type suffix struct {
		s    string
		mult float64
	}
	suffixes := []suffix{
		{"ns", 1e-6},
		{"us", 1e-3},
		{"ms", 1},
		{"s", 1e3},
		{"m", 6e4},
		{"h", 3.6e6},
		{"d", 8.64e7},
	}

	for _, sfx := range suffixes {
		if strings.HasSuffix(s, sfx.s) {
			numStr := strings.TrimSuffix(s, sfx.s)
			v, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0
			}
			return int64(v * sfx.mult)
		}
	}
	return 0
}

// ParseSizeBytes parses Trino's human-readable size strings into bytes.
// Formats: "0B", "123B", "12.4kB", "56.7MB", "1.2GB", "0.5TB", "0.1PB".
// Returns 0 for empty or unparseable strings.
func ParseSizeBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	type suffix struct {
		s    string
		mult float64
	}
	suffixes := []suffix{
		{"PB", 1e15},
		{"TB", 1e12},
		{"GB", 1e9},
		{"MB", 1e6},
		{"kB", 1e3},
		{"B", 1},
	}

	for _, sfx := range suffixes {
		if strings.HasSuffix(s, sfx.s) {
			numStr := strings.TrimSuffix(s, sfx.s)
			v, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0
			}
			return int64(v * sfx.mult)
		}
	}
	return 0
}
