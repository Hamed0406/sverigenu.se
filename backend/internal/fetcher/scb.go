package fetcher

import (
	"context"
	"fmt"
	"strconv"

	"swedennu/internal/geo"
)

const scbBase = "https://api.scb.se/OV0104/v1/doris/sv/ssd"

// SCBStat is one (table, metric, region, year) cell from an SCB PX-Web table.
type SCBStat struct {
	TableID    string  // "befolkning" | "inkomst"
	Metric     string  // ContentsCode, e.g. "BE0101N1", "HE0110K1"
	RegionCode string  // SCB länskod "01".."25"
	Year       int
	Value      float64
	Unit       string
}

// scbTables configures which SCB tables to fetch and how. Dimensions omitted
// from sel are eliminated (summed to totals) by PX-Web when their metadata
// allows it; non-eliminable dimensions MUST appear here (e.g. Alder in the
// income table).
var scbTables = []struct {
	tableID string
	path    string
	sel     []pxSelection
	units   map[string]string // ContentsCode -> unit
}{
	{
		tableID: "befolkning",
		path:    "/START/BE/BE0101/BE0101A/BefolkningNy",
		sel: []pxSelection{
			{Code: "Region", Values: geo.SCBCountyCodes()},
			{Code: "ContentsCode", Values: []string{"BE0101N1"}}, // Folkmängd
			// Civilstand/Alder/Kon eliminable -> totals; Tid omitted -> all years
		},
		units: map[string]string{"BE0101N1": "personer"},
	},
	{
		tableID: "inkomst",
		path:    "/START/HE/HE0110/HE0110A/SamForvInk2",
		sel: []pxSelection{
			{Code: "Region", Values: geo.SCBCountyCodes()},
			{Code: "Alder", Values: []string{"tot20+"}}, // NOT eliminable — must select
			{Code: "ContentsCode", Values: []string{"HE0110K1", "HE0110K2"}}, // medel, median
			// Kon ("1+2") and Inkomstklass ("TOT") eliminated automatically
		},
		units: map[string]string{"HE0110K1": "tkr", "HE0110K2": "tkr"},
	},
	{
		// Bruttoregionprodukt (BRP) per invånare and volume growth per county (ENS2010).
		// NR0105AJ = BRP per invånare, löpande priser (tkr); NR0105AI = volymutveckling (%).
		tableID: "brp",
		path:    "/START/NR/NR0105/NR0105A/NR0105ENS2010T01A",
		sel: []pxSelection{
			{Code: "Region", Values: geo.SCBCountyCodes()},
			{Code: "ContentsCode", Values: []string{"NR0105AJ", "NR0105AI"}},
			// Tid omitted -> all available years (2000–2024)
		},
		units: map[string]string{"NR0105AJ": "tkr/inv", "NR0105AI": "%"},
	},
}

// FetchSCBStats fetches all configured SCB tables and returns one SCBStat per
// non-null (region, metric, year) cell. Two POSTs per call — far under SCB's
// 10-queries-per-10-seconds limit. If more tables are added later, insert a
// ~1.1s sleep between requests.
func (c *Client) FetchSCBStats(ctx context.Context) ([]SCBStat, error) {
	var result []SCBStat
	for _, tbl := range scbTables {
		js, err := c.postPXWeb(ctx, c.scbURL+tbl.path, tbl.sel)
		if err != nil {
			return nil, fmt.Errorf("scb %s: %w", tbl.tableID, err)
		}
		cells := flattenJSONStat2(js)
		if cells == nil {
			return nil, fmt.Errorf("scb %s: malformed json-stat2 cube", tbl.tableID)
		}
		for _, cell := range cells {
			region := cell.Codes["Region"]
			if len(region) != 2 {
				continue // defensive: only county-level codes expected
			}
			year, err := strconv.Atoi(cell.Codes["Tid"])
			if err != nil {
				continue
			}
			metric := cell.Codes["ContentsCode"]
			result = append(result, SCBStat{
				TableID:    tbl.tableID,
				Metric:     metric,
				RegionCode: region,
				Year:       year,
				Value:      cell.Value,
				Unit:       tbl.units[metric],
			})
		}
	}
	return result, nil
}
