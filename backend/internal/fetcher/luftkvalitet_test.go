package fetcher

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"swedennu/internal/geo"
)

func TestFetchAirQuality(t *testing.T) {
	// Serve a valid Open-Meteo-style response for every request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"current": map[string]any{
				"pm2_5":             3.2,
				"pm10":              5.1,
				"nitrogen_dioxide":  8.4,
				"ozone":             62.0,
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	obs, err := c.FetchAirQuality(testCtx(t))
	if err != nil {
		t.Fatalf("FetchAirQuality: unexpected error: %v", err)
	}

	want := len(geo.Counties) * 4 // 4 parameters per county
	if len(obs) != want {
		t.Errorf("got %d observations, want %d (21 counties × 4 params)", len(obs), want)
	}

	// Spot-check first observation (Stockholm, PM2.5).
	first := obs[0]
	if first.Parameter == 0 {
		t.Error("observation has zero Parameter")
	}
	if first.Value == nil || *first.Value == 0 {
		t.Error("observation has nil or zero value")
	}
	if first.CountyCode == "" {
		t.Error("observation has empty CountyCode")
	}
	if first.StationKey == "" {
		t.Error("observation has empty StationKey")
	}
}

func TestFetchAirQualityAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.FetchAirQuality(testCtx(t))
	if err == nil {
		t.Error("expected error when all county fetches fail, got nil")
	}
}
