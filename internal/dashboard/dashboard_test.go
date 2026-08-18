package dashboard

import (
	"testing"
	"time"
)

func TestResolveRangeParams_ValidCustom(t *testing.T) {
	// 3-day range (2026-08-01 00:00:00 UTC to 2026-08-03 23:59:59.999 UTC) -> interval = "day"
	fromMs, toMs, interval, norm, fromDate, toDate := resolveRangeParams("custom", "2026-08-01", "2026-08-03")
	if norm != "custom" {
		t.Errorf("expected norm = 'custom', got %q", norm)
	}
	if fromDate != "2026-08-01" {
		t.Errorf("expected fromDate = '2026-08-01', got %q", fromDate)
	}
	if toDate != "2026-08-03" {
		t.Errorf("expected toDate = '2026-08-03', got %q", toDate)
	}
	if interval != "day" {
		t.Errorf("expected interval = 'day', got %q", interval)
	}

	expectedFromMs := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	expectedToMs := time.Date(2026, 8, 3, 23, 59, 59, 999000000, time.UTC).UnixMilli()
	if fromMs != expectedFromMs {
		t.Errorf("expected fromMs = %d, got %d", expectedFromMs, fromMs)
	}
	if toMs != expectedToMs {
		t.Errorf("expected toMs = %d, got %d", expectedToMs, toMs)
	}

	// 2-day range (<= 48h) -> interval = "hour"
	_, _, interval2, norm2, fromDate2, toDate2 := resolveRangeParams("custom", "2026-08-01", "2026-08-02")
	if norm2 != "custom" || fromDate2 != "2026-08-01" || toDate2 != "2026-08-02" {
		t.Errorf("expected custom range '2026-08-01' to '2026-08-02', got norm=%q from=%q to=%q", norm2, fromDate2, toDate2)
	}
	if interval2 != "hour" {
		t.Errorf("expected interval = 'hour' for 2-day range, got %q", interval2)
	}
}

func TestResolveRangeParams_InvalidCustom(t *testing.T) {
	testCases := []struct {
		name, from, to string
	}{
		{"empty strings", "", ""},
		{"invalid from", "not-a-date", "2026-08-03"},
		{"invalid to", "2026-08-01", "not-a-date"},
		{"inverted dates", "2026-08-05", "2026-08-01"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, interval, norm, _, _ := resolveRangeParams("custom", tc.from, tc.to)
			// Invalid custom parameters should fallback to 7d range
			if interval != "day" {
				t.Errorf("expected fallback interval = 'day', got %q", interval)
			}
			if norm != "custom" && norm != "7d" {
				t.Errorf("expected fallback norm to be 'custom' or '7d', got %q", norm)
			}
		})
	}
}

func TestResolveRangeParams_Presets(t *testing.T) {
	tests := []struct {
		key          string
		wantNorm     string
		wantInterval string
	}{
		{"today", "today", "hour"},
		{"24h", "24h", "hour"},
		{"7d", "7d", "day"},
		{"30d", "30d", "day"},
		{"90d", "90d", "day"},
		{"unknown", "7d", "day"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			_, _, interval, norm, fromDate, toDate := resolveRangeParams(tt.key, "", "")
			if norm != tt.wantNorm {
				t.Errorf("expected norm = %q, got %q", tt.wantNorm, norm)
			}
			if interval != tt.wantInterval {
				t.Errorf("expected interval = %q, got %q", tt.wantInterval, interval)
			}
			if fromDate != "" || toDate != "" {
				t.Errorf("expected empty from/to date strings for preset, got from=%q to=%q", fromDate, toDate)
			}
		})
	}
}

func TestRangeOptions(t *testing.T) {
	opts := rangeOptions("custom")
	foundCustom := false
	for _, o := range opts {
		if o.Key == "custom" {
			foundCustom = true
			if !o.Selected {
				t.Errorf("expected 'custom' option to be selected")
			}
		}
	}
	if !foundCustom {
		t.Errorf("expected rangeOptions to include 'custom'")
	}
}
