package fetcher

import (
	"context"
	"fmt"
	"time"
)

const elprisBase = "https://www.elprisetjustnu.se/api/v1/prices"

// ElprisObs is one 15-minute electricity price slot for a price area.
// Since 2025-10-01 Sweden uses quarterly (15-min) pricing from Nord Pool.
type ElprisObs struct {
	Area      string    // SE1, SE2, SE3, SE4
	TimeStart time.Time // slot start (UTC)
	SEKPerKWh float64
	EURPerKWh float64
}

type elprisRaw struct {
	SEKPerKWh float64 `json:"SEK_per_kWh"`
	EURPerKWh float64 `json:"EUR_per_kWh"`
	TimeStart string  `json:"time_start"`
}

// FetchElpris fetches today's and yesterday's 15-minute spot prices for all four
// Swedish price areas (SE1–SE4) from elprisetjustnu.se (Nord Pool quarterly pricing).
func (c *Client) FetchElpris(ctx context.Context) ([]ElprisObs, error) {
	areas := []string{"SE1", "SE2", "SE3", "SE4"}
	now := time.Now()
	days := []time.Time{now, now.AddDate(0, 0, -1)}

	var all []ElprisObs
	for _, day := range days {
		ymd := day.Format("2006/01-02")
		for _, area := range areas {
			url := fmt.Sprintf("%s/%s_%s.json", elprisBase, ymd, area)
			var raw []elprisRaw
			if err := c.getURL(ctx, url, &raw); err != nil {
				// yesterday may 404 at start of month — skip silently
				continue
			}
			for _, r := range raw {
				t, err := time.Parse(time.RFC3339, r.TimeStart)
				if err != nil {
					continue
				}
				// Keep all 15-min quarterly slots (Sweden moved to quarterly pricing 2025-10-01)
				all = append(all, ElprisObs{
					Area:      area,
					TimeStart: t.UTC(),
					SEKPerKWh: r.SEKPerKWh,
					EURPerKWh: r.EURPerKWh,
				})
			}
		}
	}
	return all, nil
}
