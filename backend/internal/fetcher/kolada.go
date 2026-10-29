package fetcher

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const koladaBase = "https://api.kolada.se/v3"

// koladaBatchSize is the hard v3 limit on path-parameter id lists:
// "Number of members must be between 1 and 25".
const koladaBatchSize = 25

// maxKoladaPages caps next_url pagination per batch. Measured responses for
// 25 municipalities × 3 KPIs arrive in a single page; the cap is defensive.
const maxKoladaPages = 10

// koladaKPIs is the set of municipal KPIs fetched for all 290 municipalities.
var koladaKPIs = []struct {
	ID    string
	Title string
	Unit  string
}{
	{"N01951", "Befolkning totalt", "invånare"},
	{"N03920", "Arbetslöshet 18–64 år", "%"},
	{"N15033", "Elever per lärare grundskola", "elever/lärare"},
	{"N15504", "Meritvärde åk 9 lägeskommun", "poäng"},
	{"N45905", "Energianvändning totalt", "MWh/inv"},
	{"N45925", "Förnybar elproduktion andel", "%"},
	{"N00989", "Skuldsatta hos Kronofogden", "%"},
	{"N00990", "Medianskuld Kronofogden", "kr"},
	{"N31004", "Utbetalt ekonomiskt bistånd", "kr/inv"},
}

// KoladaResponse is the top-level envelope returned by the Kolada v3 API.
type KoladaResponse struct {
	Values  []KoladaEntry `json:"values"`
	Count   int           `json:"count"`
	NextURL *string       `json:"next_url"`
}

// KoladaEntry is one (kpi × municipality × period) combination. The nested
// Values slice contains one element per gender: "T" (total), "K" (female), "M" (male).
type KoladaEntry struct {
	KPI          string        `json:"kpi"`
	Period       int           `json:"period"`
	Municipality string        `json:"municipality"`
	Values       []KoladaValue `json:"values"`
}

// KoladaValue holds a single gender-disaggregated measurement. Value is a
// pointer because some periods have no data and the API returns null.
type KoladaValue struct {
	Gender    string   `json:"gender"`
	Value     *float64 `json:"value"`
	Status    string   `json:"status"`
	IsDeleted bool     `json:"isdeleted"`
}

// koladaMunicipalityList is the envelope of GET /v3/municipality.
type koladaMunicipalityList struct {
	Values []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Type  string `json:"type"` // "K" municipality, "L" region
	} `json:"values"`
	NextURL *string `json:"next_url"`
}

// KoladaStat is one (KPI, municipality, period) row stored in the DB.
// All available periods are returned (not just the latest) so the frontend
// can draw multi-year trend charts. Only the gender="T" (total) value is kept.
type KoladaStat struct {
	KPIID          string
	MunicipalityID string // 4-digit kommunkod, e.g. "0180"
	Municipality   string
	Period         int
	Value          *float64
	Title          string
	Unit           string
}

// FetchKolada fetches the configured KPIs for ALL 290 municipalities.
//
// The v3 API quirks this works around (verified against the live API):
//   - /data/municipality/all/... silently returns zero rows — "all" is dead.
//   - Path id lists are capped at 25 members (HTTP 422 beyond that), so the
//     290 municipality ids are fetched in batches of 25.
func (c *Client) FetchKolada(ctx context.Context) ([]KoladaStat, error) {
	// Step 1: the official municipality list (also contains regions, type "L").
	munis, err := c.fetchKoladaMunicipalities(ctx)
	if err != nil {
		return nil, err
	}
	if len(munis) < 280 {
		return nil, fmt.Errorf("kolada municipality list suspiciously short: %d entries", len(munis))
	}

	ids := make([]string, 0, len(munis))
	for id := range munis {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	kpiIDs := make([]string, len(koladaKPIs))
	metaByID := make(map[string]struct{ Title, Unit string }, len(koladaKPIs))
	for i, k := range koladaKPIs {
		kpiIDs[i] = k.ID
		metaByID[k.ID] = struct{ Title, Unit string }{k.Title, k.Unit}
	}
	kpiPath := strings.Join(kpiIDs, ",")

	// Step 2: batched data fetch with per-batch pagination.
	var result []KoladaStat
	for start := 0; start < len(ids); start += koladaBatchSize {
		end := min(start+koladaBatchSize, len(ids))
		batch := strings.Join(ids[start:end], ",")
		nextURL := c.koladaURL + "/data/municipality/" + batch + "/kpi/" + kpiPath

		for page := 0; nextURL != "" && page < maxKoladaPages; page++ {
			var resp KoladaResponse
			if err := c.getURL(ctx, nextURL, &resp); err != nil {
				return nil, fmt.Errorf("kolada batch %d: %w", start/koladaBatchSize, err)
			}
			for _, entry := range resp.Values {
				meta, ok := metaByID[entry.KPI]
				if !ok {
					continue
				}
				stat := KoladaStat{
					KPIID:          entry.KPI,
					MunicipalityID: entry.Municipality,
					Municipality:   munis[entry.Municipality],
					Period:         entry.Period,
					Title:          meta.Title,
					Unit:           meta.Unit,
				}
				for _, v := range entry.Values {
					if v.Gender == "T" && !v.IsDeleted {
						stat.Value = v.Value
						break
					}
				}
				result = append(result, stat)
			}
			if resp.NextURL != nil && *resp.NextURL != "" {
				nextURL = *resp.NextURL
			} else {
				nextURL = ""
			}
		}
	}
	return result, nil
}

// fetchKoladaMunicipalities returns id -> name for all type "K" entries
// (municipalities; the list also contains Riket "0000" and 21 regions, type "L").
func (c *Client) fetchKoladaMunicipalities(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, 300)
	nextURL := c.koladaURL + "/municipality?per_page=500"
	for page := 0; nextURL != "" && page < maxKoladaPages; page++ {
		var resp koladaMunicipalityList
		if err := c.getURL(ctx, nextURL, &resp); err != nil {
			return nil, fmt.Errorf("kolada municipality list: %w", err)
		}
		for _, m := range resp.Values {
			if m.Type == "K" {
				out[m.ID] = m.Title
			}
		}
		if resp.NextURL != nil && *resp.NextURL != "" {
			nextURL = *resp.NextURL
		} else {
			nextURL = ""
		}
	}
	return out, nil
}
