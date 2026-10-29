package fetcher

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// EnergiMixRow is one (year, source, TWh) cell of national electricity production.
type EnergiMixRow struct {
	Year   int
	Source string  // "vattenkraft" | "vindkraft" | "solkraft" | "kärnkraft" | "biokraft"
	TWh    float64
}

const energiBase = "https://pxexternal.energimyndigheten.se/api/v1/sv/Energimyndighetens_statistikdatabas/Officiell_energistatistik"

// metricMap translates Energimyndigheten's Kategori index codes to human names.
var vindMetric = map[string]struct {
	name string
	unit string
}{
	"0": {"antal_verk", "st"},
	"1": {"installerad_effekt_mw", "MW"},
	"2": {"produktion_gwh", "GWh"},
}

// FetchVindkraft fetches wind power statistics per county from Energimyndigheten.
// Returns SCBStat slices (table_id="vindkraft") that can be stored via UpsertSCBStats.
func (c *Client) FetchVindkraft(ctx context.Context) ([]SCBStat, error) {
	url := energiBase + "/Vindkraftsstatistik/EN0105_3.px"

	js, err := c.postPXWeb(ctx, url, []pxSelection{
		// Fetch all three categories: antal verk, installerad effekt, elproduktion
		{Code: "Kategori", Values: []string{"0", "1", "2"}},
	})
	if err != nil {
		return nil, fmt.Errorf("vindkraft: %w", err)
	}
	cells := flattenJSONStat2(js)
	if cells == nil {
		return nil, fmt.Errorf("vindkraft: malformed json-stat2 cube")
	}

	var result []SCBStat
	for _, cell := range cells {
		// County label is "01 Stockholms län" — extract the leading länskod.
		lanLabel := cell.Labels["Län"]
		parts := strings.Fields(lanLabel)
		if len(parts) < 1 {
			continue
		}
		regionCode := parts[0] // "01", "03" … "25"
		if len(regionCode) != 2 {
			continue
		}

		year, err := strconv.Atoi(cell.Labels["År"])
		if err != nil {
			continue
		}

		kategori := cell.Codes["Kategori"]
		m, ok := vindMetric[kategori]
		if !ok {
			continue
		}

		result = append(result, SCBStat{
			TableID:    "vindkraft",
			Metric:     m.name,
			RegionCode: regionCode,
			Year:       year,
			Value:      cell.Value,
			Unit:       m.unit,
		})
	}
	return result, nil
}
// FetchSolceller fetches solar panel installations per county from Energimyndigheten.
// Returns SCBStat slices (table_id="solceller") for UpsertSCBStats.
func (c *Client) FetchSolceller(ctx context.Context) ([]SCBStat, error) {
	url := energiBase + "/Natanslutna_solcellsanlaggningar/EN0123_1.px"

	js, err := c.postPXWeb(ctx, url, []pxSelection{
		{Code: "Effektklass", Values: []string{"3"}},        // Totalt (all size classes)
		{Code: "Kategori", Values: []string{"0", "1"}},      // antal + installerad MW
	})
	if err != nil {
		return nil, fmt.Errorf("solceller: %w", err)
	}
	cells := flattenJSONStat2(js)
	if cells == nil {
		return nil, fmt.Errorf("solceller: malformed json-stat2 cube")
	}

	solMetric := map[string]struct{ name, unit string }{
		"0": {"antal_anlaggningar", "st"},
		"1": {"installerad_effekt_mw", "MW"},
	}

	var result []SCBStat
	for _, cell := range cells {
		// Region label is "01 Stockholms län" — skip national row "00 Riket".
		lanLabel := cell.Labels["Region"]
		parts := strings.Fields(lanLabel)
		if len(parts) < 1 {
			continue
		}
		regionCode := parts[0]
		if len(regionCode) != 2 || regionCode == "00" {
			continue // skip national aggregate and municipality codes
		}

		year, err := strconv.Atoi(cell.Labels["År"])
		if err != nil {
			continue
		}

		kategori := cell.Codes["Kategori"]
		m, ok := solMetric[kategori]
		if !ok {
			continue
		}

		result = append(result, SCBStat{
			TableID:    "solceller",
			Metric:     m.name,
			RegionCode: regionCode,
			Year:       year,
			Value:      cell.Value,
			Unit:       m.unit,
		})
	}
	return result, nil
}

// kraftslagNames maps Kraftslag category index codes to normalised source names.
var kraftslagNames = map[string]string{
	"0": "vattenkraft",
	"1": "vindkraft",
	"2": "solkraft",
	"3": "kärnkraft",
}

// FetchEnergiMix fetches national electricity production (TWh) by source 1970–present.
// Uses EN0202_25 from Energimyndigheten (dimension order: ContentsCode × År × Kraftslag).
func (c *Client) FetchEnergiMix(ctx context.Context) ([]EnergiMixRow, error) {
	url := energiBase + "/Arlig_energibalans/El_och_fjarrvarmeproduktion/EN0202_25.px"

	js, err := c.postPXWeb(ctx, url, []pxSelection{
		{Code: "Kraftslag", Values: []string{"0", "1", "2", "3"}}, // vattten/vind/sol/kärn
	})
	if err != nil {
		return nil, fmt.Errorf("energimix: %w", err)
	}
	cells := flattenJSONStat2(js)
	if cells == nil {
		return nil, fmt.Errorf("energimix: malformed json-stat2 cube")
	}

	var result []EnergiMixRow
	for _, cell := range cells {
		year, err := strconv.Atoi(cell.Labels["År"])
		if err != nil {
			continue
		}
		ksCode := cell.Codes["Kraftslag"]
		source, ok := kraftslagNames[ksCode]
		if !ok {
			continue
		}
		result = append(result, EnergiMixRow{
			Year:   year,
			Source: source,
			TWh:    cell.Value,
		})
	}
	return result, nil
}
