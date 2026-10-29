package fetcher

import (
	"context"
	"fmt"
)

const domstolBase = "https://pxweb.etjanst.domstol.se/PxWeb/api/v1/sv/DOMstat"

// DomstolStat is one row of court statistics from Domstolsverket.
type DomstolStat struct {
	TableID    string  // "courttype" | "processing_tr"
	CourtCode  string  // API code e.g. "District Courts" or "Stockholm"
	CourtName  string  // Swedish display name e.g. "Tingsrätter" or "Stockholms tingsrätt"
	CountyCode string  // two-letter county code ("ab", "m" …); "" for national aggregates
	Metric     string  // "filed" | "determined" | "pending" | "proc_crim" | "proc_civil"
	Year       int
	Value      float64
	Unit       string
}

// domstolCourtCounty maps Domstolsverket's English tingsrätt codes to county codes.
var domstolCourtCounty = map[string]string{
	"Alingsås":            "o",
	"Attunda":             "ab",
	"Blekinge":            "k",
	"Borås":               "o",
	"Eksjö":               "f",
	"Eskilstuna":          "d",
	"Falu":                "w",
	"Gotland":             "i",
	"Gällivare":           "bd",
	"Gävle":               "x",
	"Gothenburg":          "o",
	"Halmstad":            "n",
	"Haparanda":           "bd",
	"Helsingborg":         "m",
	"Hälsingland":         "x",
	"Hässleholm":          "m",
	"Jönköping":           "f",
	"Kalmar":              "h",
	"Kristianstad":        "m",
	"Linköping":           "e",
	"Luleå":               "bd",
	"Lund":                "m",
	"Lycksele":            "ac",
	"Malmö":               "m",
	"Mora":                "w",
	"Nacka":               "ab",
	"Norrköping":          "e",
	"Norrtälje":           "ab",
	"Nyköping":            "d",
	"Skaraborg":           "o",
	"Skellefteå":          "ac",
	"Solna":               "ab",
	"Stockholm":           "ab",
	"Sundsvall":           "y",
	"Södertälje":          "ab",
	"Södertörn":           "ab",
	"Uddevalla":           "o",
	"Umeå":                "ac",
	"Uppsala":             "c",
	"Varberg":             "n",
	"Vänersborg":          "o",
	"Värmland":            "s",
	"Västmanland":         "u",
	"Växjö":               "g",
	"Ystad":               "m",
	"Ångermanland":        "y",
	"Örebro":              "t",
	"Östersund":           "z",
	"All district courts": "", // national aggregate — no county
}

// domstolVariableMetric maps Domstolsverket Variabel codes to our metric names.
var domstolVariableMetric = map[string]string{
	"Number of filed cases":                      "filed",
	"Number of determined cases":                  "determined",
	"Number of pending cases December 31":         "pending",
}

// domstolCaseTypeMetric maps Verksamhetsmålskategori codes to metric names.
var domstolCaseTypeMetric = map[string]string{
	"Criminal cases excluding priority cases":                       "proc_crim",
	"Civil cases excluding joint petitions for divorce":             "proc_civil",
}

// FetchDomstolStats fetches two datasets from Domstolsverket PX-Web:
//
//   - courttype: inkomna/avgjorda/balanserade mål per domstolsslag, 2002–2025
//   - processing_tr: handläggningstid (75-percentil) per tingsrätt, 2007–2025
func (c *Client) FetchDomstolStats(ctx context.Context) ([]DomstolStat, error) {
	var out []DomstolStat

	// ── Court type case counts (national trends) ───────────────────────────
	courtTypeURL := domstolBase + "/AntalMal/01_Malutveckling_per_domstolsslag.px"
	ctResp, err := c.postPXWeb(ctx, courtTypeURL, []pxSelection{
		{Code: "Domstolsslag (migration separat)", Values: []string{
			"District Courts", "Courts of Appeal", "Supreme Court",
			"Administrative Courts", "Administrative Courts of appeal",
			"Supreme Administrative Court", "Migration Courts", "Migration Court of Appeal",
		}},
		{Code: "Variabel", Values: []string{
			"Number of filed cases",
			"Number of determined cases",
			"Number of pending cases December 31",
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("domstol courttype: %w", err)
	}
	for _, cell := range flattenJSONStat2(ctResp) {
		yearStr := cell.Labels["År"]
		year := 0
		fmt.Sscanf(yearStr, "%d", &year)
		metric := domstolVariableMetric[cell.Codes["Variabel"]]
		if metric == "" {
			continue
		}
		out = append(out, DomstolStat{
			TableID:   "courttype",
			CourtCode: cell.Codes["Domstolsslag (migration separat)"],
			CourtName: cell.Labels["Domstolsslag (migration separat)"],
			Metric:    metric,
			Year:      year,
			Value:     cell.Value,
			Unit:      "mål",
		})
	}

	// ── Tingsrätt processing times ─────────────────────────────────────────
	procURL := domstolBase + "/Verksamhetsmal/01_Verksamhetsmal_TR.px"
	procResp, err := c.postPXWeb(ctx, procURL, []pxSelection{
		{Code: "Verksamhetsmålskategori", Values: []string{
			"Criminal cases excluding priority cases",
			"Civil cases excluding joint petitions for divorce",
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("domstol processing_tr: %w", err)
	}
	for _, cell := range flattenJSONStat2(procResp) {
		yearStr := cell.Labels["År"]
		year := 0
		fmt.Sscanf(yearStr, "%d", &year)
		metric := domstolCaseTypeMetric[cell.Codes["Verksamhetsmålskategori"]]
		if metric == "" {
			continue
		}
		code := cell.Codes["Domstol"]
		county := domstolCourtCounty[code]
		out = append(out, DomstolStat{
			TableID:    "processing_tr",
			CourtCode:  code,
			CourtName:  cell.Labels["Domstol"],
			CountyCode: county,
			Metric:     metric,
			Year:       year,
			Value:      cell.Value,
			Unit:       "månader (p75)",
		})
	}

	return out, nil
}
