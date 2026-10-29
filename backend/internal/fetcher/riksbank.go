package fetcher

import (
	"context"
	"fmt"
	"time"
)

const riksbankBase = "https://api.riksbank.se/swea/v1"

// RiksbankObs is one date→value observation from the Riksbanken API.
type RiksbankObs struct {
	SeriesID string
	Date     time.Time
	Value    float64
}

// riksbankRaw matches the Riksbanken observations JSON: [{date, value}]
type riksbankRaw struct {
	Date  string   `json:"date"`
	Value *float64 `json:"value"`
}

// FetchRiksbank fetches policy rate (SECBREPOEFF), SEK/EUR (SEKEURPMI) and
// SEK/USD (SEKUSDPMI) observations from 2000-01-01 onwards.
func (c *Client) FetchRiksbank(ctx context.Context) ([]RiksbankObs, error) {
	series := []struct {
		id    string
		label string
	}{
		{"SECBREPOEFF", "policy_rate"},
		{"SEKEURPMI", "sek_eur"},
		{"SEKUSDPMI", "sek_usd"},
	}

	var all []RiksbankObs
	for _, s := range series {
		url := fmt.Sprintf("%s/Observations/%s/2000-01-01", riksbankBase, s.id)
		var raw []riksbankRaw
		if err := c.getURL(ctx, url, &raw); err != nil {
			return nil, fmt.Errorf("riksbank %s: %w", s.id, err)
		}
		for _, r := range raw {
			if r.Value == nil {
				continue
			}
			t, err := time.Parse("2006-01-02", r.Date)
			if err != nil {
				continue
			}
			all = append(all, RiksbankObs{
				SeriesID: s.label,
				Date:     t,
				Value:    *r.Value,
			})
		}
	}
	return all, nil
}
