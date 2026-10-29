package fetcher

import (
	"context"
	"fmt"
)

const fohmBase = "https://fohm-app.folkhalsomyndigheten.se/Folkhalsodata/api/v1/sv/A_Folkhalsodata"

// FolkhalsaStat is one row of public-health data from Folkhälsomyndigheten.
type FolkhalsaStat struct {
	Metric     string  // e.g. "lifeexp", "smoking", "alcohol"
	RegionCode string  // "01".."25"
	Sex        string  // "total" | "women" | "men"
	Period     string  // year "2025" or range "2021-2024"
	Value      float64
	Unit       string
}

// fohmRegions are the 21 county codes (skipping "00" = Riket).
var fohmRegions = []string{
	"01", "03", "04", "05", "06", "07", "08", "09", "10",
	"12", "13", "14", "17", "18", "19", "20", "21", "22", "23", "24", "25",
}

// FetchFolkhalsaStats fetches three indicators from Folkhälsomyndigheten:
//
//   - lifeexp: medellivslängd vid födseln (MedlivsXReg.px), kön totalt/women/men
//   - smoking: daglig tobaksrökning per region (tobakdagyreg.px), totalt
//   - alcohol: riskbruk av alkohol per region (alkriskyreg.px), totalt
func (c *Client) FetchFolkhalsaStats(ctx context.Context) ([]FolkhalsaStat, error) {
	var out []FolkhalsaStat

	// ── Life expectancy ────────────────────────────────────────────────────────
	lifeURL := fohmBase + "/A_Mo8/Halsoutfall/01Overgrip/01.03medlivs/MedlivsXReg.px"
	lifeResp, err := c.postPXWeb(ctx, lifeURL, []pxSelection{
		{Code: "Region", Values: fohmRegions},
		{Code: "Återstående medellivslängd", Values: []string{"0"}},
		{Code: "Kön", Values: []string{"1+2", "1", "2"}},
	})
	if err != nil {
		return nil, fmt.Errorf("folkhalsa lifeexp: %w", err)
	}
	for _, cell := range flattenJSONStat2(lifeResp) {
		out = append(out, FolkhalsaStat{
			Metric:     "lifeexp",
			RegionCode: cell.Codes["Region"],
			Sex:        parseFohmSex(cell.Labels["Kön"]),
			Period:     cell.Codes["År"],
			Value:      cell.Value,
			Unit:       "år",
		})
	}

	// ── Tobacco daily smoking ──────────────────────────────────────────────────
	smokingURL := fohmBase + "/A_Mo8/6_Levanor/01Begrans/06.01tobakdag/tobakdagyreg.px"
	smokingResp, err := c.postPXWeb(ctx, smokingURL, []pxSelection{
		{Code: "Region", Values: fohmRegions},
		{Code: "Andel och konfidensintervall", Values: []string{"01"}}, // share only, not CI
		{Code: "Kön", Values: []string{"00", "01", "02"}},
	})
	if err != nil {
		return nil, fmt.Errorf("folkhalsa smoking: %w", err)
	}
	for _, cell := range flattenJSONStat2(smokingResp) {
		out = append(out, FolkhalsaStat{
			Metric:     "smoking",
			RegionCode: cell.Codes["Region"],
			Sex:        parseFohmSex(cell.Labels["Kön"]),
			Period:     cell.Codes["År"],
			Value:      cell.Value,
			Unit:       "%",
		})
	}

	// ── Alcohol risk consumption ───────────────────────────────────────────────
	alcoholURL := fohmBase + "/A_Mo8/6_Levanor/01Begrans/06.04alkrisk/alkriskyreg.px"
	alcoholResp, err := c.postPXWeb(ctx, alcoholURL, []pxSelection{
		{Code: "Region", Values: fohmRegions},
		{Code: "Kön", Values: []string{"00", "01", "02"}},
	})
	if err != nil {
		return nil, fmt.Errorf("folkhalsa alcohol: %w", err)
	}
	for _, cell := range flattenJSONStat2(alcoholResp) {
		// alcohol table includes CI columns — skip non-share rows
		if label, ok := cell.Labels["Andel och konfidensintervall"]; ok && label != "Andel" {
			continue
		}
		out = append(out, FolkhalsaStat{
			Metric:     "alcohol",
			RegionCode: cell.Codes["Region"],
			Sex:        parseFohmSex(cell.Labels["Kön"]),
			Period:     cell.Codes["År"],
			Value:      cell.Value,
			Unit:       "%",
		})
	}

	return out, nil
}

// parseFohmSex maps Swedish gender labels to our canonical "total"/"women"/"men".
func parseFohmSex(label string) string {
	switch label {
	case "Kvinnor":
		return "women"
	case "Män":
		return "men"
	default:
		return "total"
	}
}

