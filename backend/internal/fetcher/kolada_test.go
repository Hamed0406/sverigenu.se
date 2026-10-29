package fetcher

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchKolada wires up a fake Kolada v3 API with a municipality list and a
// single data page, and verifies that FetchKolada produces the right KoladaStat rows.
func TestFetchKolada(t *testing.T) {
	// Build a municipality list with 290+ entries to pass the length guard.
	// We only care about verifying the parsing logic, so we use a minimal set
	// and patch the guard constant via the response count.
	muniListBody := buildMuniList(290)

	dataBody := `{
		"values":[
			{"kpi":"N15504","period":2025,"municipality":"0180",
			 "values":[{"gender":"T","value":245.0,"isdeleted":false},{"gender":"K","value":250.0,"isdeleted":false}]},
			{"kpi":"N15504","period":2025,"municipality":"0180",
			 "values":[{"gender":"T","value":null,"isdeleted":true}]},
			{"kpi":"N03920","period":2025,"municipality":"0480",
			 "values":[{"gender":"T","value":5.2,"isdeleted":false}]},
			{"kpi":"UNKNOWN","period":2025,"municipality":"0180",
			 "values":[{"gender":"T","value":99.9,"isdeleted":false}]}
		],
		"count":4,
		"next_url":null
	}`

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: municipality list.
			w.Write(muniListBody)
			return
		}
		w.Write([]byte(dataBody))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	stats, err := c.FetchKolada(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}

	// Unknown KPIs are always excluded.
	for _, s := range stats {
		if s.KPIID == "UNKNOWN" {
			t.Errorf("unknown KPI should be excluded: %+v", s)
		}
	}

	// There should be at least one N15504 row for 0180.
	// The fetcher emits one KoladaStat per (KPI, municipality, period) entry in
	// the response. If the only gender="T" entry has Value=nil (null JSON), the
	// row is still emitted with a nil Value — filtering happens in the store.
	// The first N15504/0180 row has gender="T" value=245, so Value should be 245.
	var found bool
	for _, s := range stats {
		if s.KPIID == "N15504" && s.MunicipalityID == "0180" && s.Value != nil && *s.Value == 245.0 {
			found = true
			if s.Period != 2025 {
				t.Errorf("N15504/0180 period = %d, want 2025", s.Period)
			}
		}
	}
	if !found {
		t.Error("N15504/0180 with value=245.0 not found in results")
	}
}

func TestFetchKoladaMunicipalityListTooShort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a list with only 10 municipalities — below the 280 threshold.
		w.Write(buildMuniList(10))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.FetchKolada(testCtx(t))
	if err == nil {
		t.Error("expected error for suspiciously short municipality list")
	}
}

// buildMuniList returns a JSON-encoded Kolada municipality list response with n entries.
func buildMuniList(n int) []byte {
	result := `{"values":[`
	for i := 0; i < n; i++ {
		if i > 0 {
			result += ","
		}
		code := 100 + i
		result += `{"id":"` + itoa(code) + `","title":"Kommun ` + itoa(code) + `","type":"K"}`
	}
	result += `],"next_url":null}`
	return []byte(result)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
