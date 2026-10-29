package geo

import "testing"

func TestCountyForPoint(t *testing.T) {
	cases := []struct {
		name     string
		lat, lon float64
		want     string
	}{
		{"Stockholm city", 59.3293, 18.0686, "ab"},
		{"Kiruna", 67.8558, 20.2253, "bd"},
		{"Gothenburg", 57.7089, 11.9746, "o"},
		{"Malmö", 55.6050, 13.0038, "m"},
		{"Visby", 57.6348, 18.2948, "i"},
		{"Östersund", 63.1792, 14.6357, "z"},
		// Offshore lighthouse south of Öland — must hit the centroid fallback.
		{"Ölands södra grund", 56.07, 16.68, "h"},
	}
	for _, c := range cases {
		if got := CountyForPoint(c.lat, c.lon); got != c.want {
			t.Errorf("%s: CountyForPoint(%v, %v) = %q, want %q", c.name, c.lat, c.lon, got, c.want)
		}
	}
}

func TestCountyForPoliceEvent(t *testing.T) {
	cases := []struct {
		name     string
		loc      string
		lat, lon float64
		want     string
	}{
		{"county name", "Norrbottens län", 0, 0, "bd"},
		{"county name genitive", "Stockholms län", 0, 0, "ab"},
		{"county name non-genitive", "Uppsala län", 0, 0, "c"},
		{"municipality", "Kiruna", 0, 0, "bd"},
		{"municipality diacritics", "Östersund", 0, 0, "z"},
		{"municipality hyphen", "Upplands-Bro", 0, 0, "ab"},
		{"gps fallback", "Okänd ort", 57.7089, 11.9746, "o"},
		{"nationwide", "Sverige", 0, 0, ""},
	}
	for _, c := range cases {
		if got := CountyForPoliceEvent(c.loc, c.lat, c.lon); got != c.want {
			t.Errorf("%s: CountyForPoliceEvent(%q) = %q, want %q", c.name, c.loc, got, c.want)
		}
	}
}

func TestAllMunicipalitiesMapToValidCounty(t *testing.T) {
	valid := make(map[string]bool, len(Counties))
	for _, c := range Counties {
		valid[c.Code] = true
	}
	if len(municipalityCounty) != 290 {
		t.Errorf("municipalityCounty has %d entries, want 290", len(municipalityCounty))
	}
	for name, code := range municipalityCounty {
		if !valid[code] {
			t.Errorf("municipality %q maps to invalid county %q", name, code)
		}
	}
}
