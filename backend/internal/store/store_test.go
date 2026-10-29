package store

import (
	"encoding/json"
	"testing"
	"time"
)

// ── ParseGPS ──────────────────────────────────────────────────────────────────

func TestParseGPS(t *testing.T) {
	cases := []struct {
		in       string
		wantLat  float64
		wantLon  float64
	}{
		{"59.3293,18.0686", 59.3293, 18.0686},
		{"67.8558, 20.2253", 67.8558, 20.2253}, // trailing space
		{" 55.6050 , 13.0038 ", 55.6050, 13.0038},
		{"", 0, 0},
		{"notanumber", 0, 0},
		{"59.3", 0, 0},    // missing second part — only one token
		{"59.3,", 59.3, 0}, // empty second part → ParseFloat("") = 0, lat still parses
	}
	for _, c := range cases {
		lat, lon := ParseGPS(c.in)
		if lat != c.wantLat || lon != c.wantLon {
			t.Errorf("ParseGPS(%q) = (%v, %v), want (%v, %v)",
				c.in, lat, lon, c.wantLat, c.wantLon)
		}
	}
}

// ── ParsePoliceDateTime ───────────────────────────────────────────────────────

func TestParsePoliceDateTime(t *testing.T) {
	cases := []struct {
		in      string
		wantNil bool
		wantHr  int // expected UTC hour after parsing
	}{
		{"2026-05-20 17:11:17 +02:00", false, 15}, // 17:00 CET → 15:00 UTC
		{"2026-01-01 00:00:00 +00:00", false, 0},
		{"", true, 0},
		{"not a date", true, 0},
		{"2026-13-01 00:00:00 +00:00", true, 0}, // invalid month
	}
	for _, c := range cases {
		got := ParsePoliceDateTime(c.in)
		if c.wantNil {
			if got != nil {
				t.Errorf("ParsePoliceDateTime(%q) = %v, want nil", c.in, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("ParsePoliceDateTime(%q) returned nil, want a time", c.in)
			continue
		}
		if got.UTC().Hour() != c.wantHr {
			t.Errorf("ParsePoliceDateTime(%q) UTC hour = %d, want %d",
				c.in, got.UTC().Hour(), c.wantHr)
		}
	}
}

// ── marshal ───────────────────────────────────────────────────────────────────

func TestMarshal(t *testing.T) {
	got := marshal(map[string]int{"a": 1})
	var out map[string]int
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("marshal produced invalid JSON: %v — %s", err, got)
	}
	if out["a"] != 1 {
		t.Errorf("marshal round-trip: got %v, want 1", out["a"])
	}
}

func TestMarshalUnmarshalable(t *testing.T) {
	// Channels cannot be marshalled; the function must return "{}" not panic.
	ch := make(chan int)
	got := marshal(ch)
	if got != "{}" {
		t.Errorf("marshal(channel) = %q, want {}", got)
	}
}

// ── nullableTime ──────────────────────────────────────────────────────────────

func TestNullableTime(t *testing.T) {
	if nullableTime(nil) != nil {
		t.Error("nullableTime(nil) should return nil")
	}
	now := time.Now()
	v := nullableTime(&now)
	if vt, ok := v.(time.Time); !ok || !vt.Equal(now) {
		t.Errorf("nullableTime(&t) = %v, want %v", v, now)
	}
}

// ── nullableFloat64 ───────────────────────────────────────────────────────────

func TestNullableFloat64(t *testing.T) {
	if nullableFloat64(nil) != nil {
		t.Error("nullableFloat64(nil) should return nil")
	}
	val := 3.14
	v := nullableFloat64(&val)
	if vf, ok := v.(float64); !ok || vf != 3.14 {
		t.Errorf("nullableFloat64(&3.14) = %v, want 3.14", v)
	}
}

// ── nullableString ────────────────────────────────────────────────────────────

func TestNullableString(t *testing.T) {
	if nullableString("") != nil {
		t.Error("nullableString(\"\") should return nil")
	}
	v := nullableString("hello")
	if vs, ok := v.(string); !ok || vs != "hello" {
		t.Errorf("nullableString(\"hello\") = %v, want \"hello\"", v)
	}
}
