package queryinfo

import "testing"

func TestParseDurationMs(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"0.00ns", 0},
		{"500.00us", 0},  // 0.5 ms rounds to 0
		{"1000.00us", 1}, // 1 ms
		{"1.23ms", 1},
		{"4.56ms", 4},
		{"100.00ms", 100},
		{"1.00s", 1000},
		{"7.89s", 7890},
		{"1.00m", 60000},
		{"2.50m", 150000},
		{"1.00h", 3600000},
		{"0.50h", 1800000},
		{"1.00d", 86400000},
		{"garbage", 0},
	}

	for _, tt := range tests {
		got := ParseDurationMs(tt.input)
		if got != tt.want {
			t.Errorf("ParseDurationMs(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseSizeBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"0B", 0},
		{"123B", 123},
		{"12.4kB", 12400},
		{"56.7MB", 56700000},
		{"1.2GB", 1200000000},
		{"0.5TB", 500000000000},
		{"0.1PB", 100000000000000},
		{"garbage", 0},
	}

	for _, tt := range tests {
		got := ParseSizeBytes(tt.input)
		if got != tt.want {
			t.Errorf("ParseSizeBytes(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
