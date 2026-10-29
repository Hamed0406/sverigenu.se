package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"swedennu/internal/geo"
)

const skattBase = "https://skatteverket.entryscape.net/rowstore/dataset/c67b320b-ffee-4876-b073-dd9236cd2a99"

// SkattRow is one municipality × year tax-rate record from Skatteverket.
type SkattRow struct {
	Year              int
	Municipality      string
	LanKod            string  // "01"–"25"
	CountyCode        string  // "ab"–"bd"
	KommunalSkatt     float64 // kommunal income tax rate (%)
	LandstingsSkatt   float64 // regional tax rate (%)
	BegravningsAvgift float64 // burial fee (%)
	SummaExklKyrka    float64 // total excl. church fee = kommunal+landsting+begravning
}

type skattAPIRow struct {
	KommunalSkatt   string `json:"kommunal-skatt"`
	ForsamlingsKod  string `json:"församlings-kod"`
	Kommun          string `json:"kommun"`
	BegravningsAvg  string `json:"begravnings-avgift"`
	LandstingsSkatt string `json:"landstings-skatt"`
	Ar              string `json:"år"`
}

type skattPage struct {
	Next    string        `json:"next"`
	Results []skattAPIRow `json:"results"`
}

// FetchSkattesats fetches local tax rates for all Swedish municipalities from Skatteverket.
// The API caps at 100 rows per page so we follow the next-link until exhausted.
// Deduplicates to one row per (municipality, year) — multiple congregations share the same rates.
func (c *Client) FetchSkattesats(ctx context.Context) ([]SkattRow, error) {
	years := []string{"2017", "2018", "2019", "2020", "2021", "2022", "2023", "2024", "2025"}
	seen := map[string]bool{} // "MUNICIPALITY|year"
	var all []SkattRow

	parseF := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		return v
	}

	for _, yr := range years {
		q := url.Values{}
		q.Set("_limit", "100")
		q.Set("år", yr) // url.Values encodes å → %C3%A5 so the filter works correctly
		pageURL := skattBase + "?" + q.Encode()

		for pageURL != "" {
			page, err := c.fetchSkattAPIPage(ctx, pageURL)
			if err != nil {
				return nil, fmt.Errorf("skattesats %s: %w", yr, err)
			}

			for _, r := range page.Results {
				key := strings.ToUpper(r.Kommun) + "|" + r.Ar
				if seen[key] {
					continue
				}
				seen[key] = true

				lanKod := ""
				if len(r.ForsamlingsKod) >= 2 {
					lanKod = strings.TrimSpace(r.ForsamlingsKod[:2])
				}
				if len(lanKod) != 2 {
					continue
				}
				year, err := strconv.Atoi(r.Ar)
				if err != nil {
					continue
				}

				kommunal := parseF(r.KommunalSkatt)
				landsting := parseF(r.LandstingsSkatt)
				begravning := parseF(r.BegravningsAvg)
				all = append(all, SkattRow{
					Year:              year,
					Municipality:      strings.ToUpper(strings.TrimSpace(r.Kommun)),
					LanKod:            lanKod,
					CountyCode:        geo.CountyBySCBCode(lanKod),
					KommunalSkatt:     kommunal,
					LandstingsSkatt:   landsting,
					BegravningsAvgift: begravning,
					// The API's own sum field has a comma in its JSON key
					// ("summa, exkl. kyrkoavgift") which a Go struct tag cannot
					// express, so the total is computed here instead.
					SummaExklKyrka: kommunal + landsting + begravning,
				})
			}
			pageURL = page.Next // empty string when last page
		}
	}
	return all, nil
}

func (c *Client) fetchSkattAPIPage(ctx context.Context, rawURL string) (*skattPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SverigeNu/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var page skattPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return &page, nil
}
