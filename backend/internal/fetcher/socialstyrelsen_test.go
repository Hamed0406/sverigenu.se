package fetcher

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ssDeathsFixture is one page of dödsorsaker response — 2 rows across 2 diagnoses.
const ssDeathsFixture = `{
	"data": [
		{"diagnosId":"99","regionId":0,"alderId":1,"konId":3,"mattId":1,"ar":2022,"varde":"95 626"},
		{"diagnosId":"I00-I99","regionId":0,"alderId":2,"konId":3,"mattId":1,"ar":2022,"varde":"32 400"}
	],
	"nasta_sida": null
}`

// ssDentalRegionFixture is the region metadata response.
const ssDentalRegionFixture = `[
	{"id":1,"kod":"01","text":"Stockholm"},
	{"id":3,"kod":"03","text":"Uppsala"},
	{"id":0,"kod":"00","text":"Riket"}
]`

// ssDentalFixture is one page of tandhälsa response.
const ssDentalFixture = `{
	"data": [
		{"indikatorId":"1","regionId":1,"alderId":0,"konId":3,"mattId":2,"ar":2022,"varde":"67,3"},
		{"indikatorId":"1","regionId":3,"alderId":0,"konId":3,"mattId":2,"ar":2022,"varde":"72,1"},
		{"indikatorId":"1","regionId":0,"alderId":0,"konId":3,"mattId":2,"ar":2022,"varde":"65,0"}
	],
	"nasta_sida": null
}`

// ssBirthsFixture is one page of förlossningar response.
const ssBirthsFixture = `{
	"data": [
		{"variabelId":"V35","regionId":0,"alderId":0,"paritetId":0,"mattId":1,"ar":2022,"varde":"114 086"},
		{"variabelId":"V35","regionId":110,"alderId":0,"paritetId":0,"mattId":1,"ar":2022,"varde":"26 513"},
		{"variabelId":"V18","regionId":0,"alderId":0,"paritetId":0,"mattId":1,"ar":2022,"varde":"20 123"}
	],
	"nasta_sida": null
}`

func TestFetchSSDeaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(ssDeathsFixture))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	deaths, err := c.FetchSSDeaths(testCtx(t))
	if err != nil {
		t.Fatalf("FetchSSDeaths error: %v", err)
	}
	if len(deaths) != 2 {
		t.Errorf("want 2 rows, got %d", len(deaths))
	}
	first := deaths[0]
	if first.DiagnosID != "99" {
		t.Errorf("want diagnos_id=99, got %q", first.DiagnosID)
	}
	if first.DiagnosName != "Samtliga dödsorsaker" {
		t.Errorf("unexpected diagnos name: %q", first.DiagnosName)
	}
	if first.Year != 2022 {
		t.Errorf("want year=2022, got %d", first.Year)
	}
	if first.Value == nil || *first.Value != 95626 {
		t.Errorf("want value=95626, got %v", first.Value)
	}
}

func TestFetchSSDeathsParseSVInt(t *testing.T) {
	cases := []struct {
		in   string
		want *int
	}{
		{"95 626", intPtr(95626)},
		{"32 400", intPtr(32400)},
		{"..", nil},
		{"", nil},
		{"-", nil},
	}
	for _, c := range cases {
		got := parseSVInt(c.in)
		if c.want == nil && got != nil {
			t.Errorf("parseSVInt(%q) = %d, want nil", c.in, *got)
		} else if c.want != nil && (got == nil || *got != *c.want) {
			t.Errorf("parseSVInt(%q) = %v, want %d", c.in, got, *c.want)
		}
	}
}

func TestFetchSSDental(t *testing.T) {
	// The dental fetcher makes two requests: /region, then /resultat/...
	// We route based on the path suffix.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/region") {
			w.Write([]byte(ssDentalRegionFixture))
		} else {
			w.Write([]byte(ssDentalFixture))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	rows, err := c.FetchSSDental(testCtx(t))
	if err != nil {
		t.Fatalf("FetchSSDental error: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("want 3 rows, got %d", len(rows))
	}
	// First row is Stockholm (region 1, kod "01" → "ab").
	first := rows[0]
	if first.RegionName != "Stockholm" {
		t.Errorf("want Stockholm, got %q", first.RegionName)
	}
	if first.CountyCode != "ab" {
		t.Errorf("want county_code=ab, got %q", first.CountyCode)
	}
	if first.Value == nil || *first.Value != 67.3 {
		t.Errorf("want value=67.3, got %v", first.Value)
	}
	// Riket row (kod "00") has empty county_code.
	rikset := rows[2]
	if rikset.CountyCode != "" {
		t.Errorf("Riket row should have empty county_code, got %q", rikset.CountyCode)
	}
}

func TestFetchSSBirths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(ssBirthsFixture))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	rows, err := c.FetchSSBirths(testCtx(t))
	if err != nil {
		t.Fatalf("FetchSSBirths error: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("want 3 rows, got %d", len(rows))
	}
	// Riket V35 row.
	first := rows[0]
	if first.VariabelID != "V35" {
		t.Errorf("want V35, got %q", first.VariabelID)
	}
	if first.VariabelName != "Födda barn" {
		t.Errorf("unexpected variabel name: %q", first.VariabelName)
	}
	if first.RegionID != 0 || first.RegionName != "Riket" {
		t.Errorf("want Riket, got id=%d name=%q", first.RegionID, first.RegionName)
	}
	if first.CountyCode != "" {
		t.Errorf("Riket should have empty county_code, got %q", first.CountyCode)
	}
	if first.Value == nil || *first.Value != 114086 {
		t.Errorf("want value=114086, got %v", first.Value)
	}
	// Stockholm V35 row has county_code "ab".
	sthlm := rows[1]
	if sthlm.CountyCode != "ab" {
		t.Errorf("want county_code=ab for region 110, got %q", sthlm.CountyCode)
	}
}

func TestFetchSSBirthsPagination(t *testing.T) {
	page := 0
	var page2URL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page++
		if page == 1 {
			// First page — next_page points to the server itself with ?page=2.
			page2URL = "http://" + r.Host + r.URL.Path + "?page=2"
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"variabelId": "V35", "regionId": 0, "alderId": 0, "paritetId": 0, "mattId": 1, "ar": 2022, "varde": "100"},
				},
				"nasta_sida": page2URL,
			})
		} else {
			// Second page — no next.
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"variabelId": "V35", "regionId": 110, "alderId": 0, "paritetId": 0, "mattId": 1, "ar": 2022, "varde": "20"},
				},
				"nasta_sida": nil,
			})
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	rows, err := c.FetchSSBirths(testCtx(t))
	if err != nil {
		t.Fatalf("FetchSSBirths pagination error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 rows across 2 pages, got %d", len(rows))
	}
}

// intPtr is a test helper for creating *int values inline.
func intPtr(i int) *int { return &i }
