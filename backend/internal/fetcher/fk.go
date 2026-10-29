package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// fkLanMap maps FK's county code (01, 03 …) to our 2-letter county slug.
var fkLanMap = map[string]string{
	"01": "ab", "03": "c",  "04": "d",  "05": "e",
	"06": "f",  "07": "g",  "08": "h",  "09": "i",
	"10": "k",  "12": "m",  "13": "n",  "14": "o",
	"17": "s",  "18": "t",  "19": "u",  "20": "w",
	"21": "x",  "22": "y",  "23": "z",  "24": "ac",
	"25": "bd",
}

// FKSjukfall is one row of Försäkringskassan sjukfall data.
type FKSjukfall struct {
	CountyCode string
	LanKod     string
	Year       int
	Month      int
	Diagnos    string
	Andel      *float64 // % of ongoing sick leave cases with this diagnosis
	Antal      *int     // absolute count
}

type fkRow struct {
	Dimensions struct {
		Ar       string `json:"ar"`
		Manad    string `json:"manad"`
		KonKod   string `json:"kon_kod"`
		LanKod   string `json:"lan_kod"`
		KommunKod string `json:"kommun_kod"`
	} `json:"dimensions"`
	Observations struct {
		Andel struct {
			Rojd  bool    `json:"rojd"`
			Value float64 `json:"value"`
		} `json:"andel"`
		Antal struct {
			Rojd  bool    `json:"rojd"`
			Value float64 `json:"value"`
		} `json:"antal"`
	} `json:"observations"`
}

const fkBase = "https://www.forsakringskassan.se/api/sprstatistikrapportera/public/v1"

// FetchFKSjukfall fetches stress-related sick leave (F43) per county.
// Yearly snapshots (December) 2010–2022, then all months 2023–present.
func (c *Client) FetchFKSjukfall(ctx context.Context) ([]FKSjukfall, error) {
	now := time.Now()
	currentYear := now.Year()

	var results []FKSjukfall

	for lanKod, countyCode := range fkLanMap {
		// Yearly Dec snapshots: 2010 → 2022
		for year := 2010; year <= 2022; year++ {
			row, err := c.fetchFKOne(ctx, lanKod, year, 12)
			if err != nil {
				continue // skip missing data silently
			}
			if row != nil {
				row.CountyCode = countyCode
				results = append(results, *row)
			}
		}
		// Monthly 2023 → now
		for year := 2023; year <= currentYear; year++ {
			maxMonth := 12
			if year == currentYear {
				maxMonth = int(now.Month())
				if now.Day() < 15 {
					maxMonth-- // last month may not be published yet
				}
				if maxMonth < 1 {
					maxMonth = 1
				}
			}
			for month := 1; month <= maxMonth; month++ {
				row, err := c.fetchFKOne(ctx, lanKod, year, month)
				if err != nil {
					continue
				}
				if row != nil {
					row.CountyCode = countyCode
					results = append(results, *row)
				}
			}
		}
	}

	return results, nil
}

func (c *Client) fetchFKOne(ctx context.Context, lanKod string, year, month int) (*FKSjukfall, error) {
	// Rate-limit: FK's API is not documented as rate-limited but be polite
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	kommunKod := "ALL_" + fmt.Sprintf("%02s", lanKod)
	url := fmt.Sprintf(
		"%s/sjp-pagaende-sjukfall-diagnos-f43/SJPPagSjukfallDiagnosF43.json?ar=%d&manad=%02d&kon_kod=ALL&lan_kod=%s&kommun_kod=%s",
		fkBase, year, month, lanKod, kommunKod,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "swedennu/1.0 (open data dashboard)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil // 404 = no data for this period
	}

	var rows []fkRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	r := rows[0]
	result := &FKSjukfall{
		LanKod:  lanKod,
		Year:    year,
		Month:   month,
		Diagnos: "F43",
	}

	if !r.Observations.Andel.Rojd {
		v := r.Observations.Andel.Value
		result.Andel = &v
	}
	if !r.Observations.Antal.Rojd {
		v := int(r.Observations.Antal.Value)
		result.Antal = &v
	}

	return result, nil
}
