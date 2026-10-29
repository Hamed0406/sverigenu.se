package fetcher

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestBRALive(t *testing.T) {
	if os.Getenv("BRA_LIVE_TEST") == "" {
		t.Skip("live test; set BRA_LIVE_TEST=1 to run")
	}
	c := New("test@example.com", "")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	rows, err := c.FetchBRACrimes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rows: %d", len(rows))
	years := map[int]int{}
	for _, r := range rows {
		years[r.Year]++
	}
	t.Logf("years: %d (sample 2024 rows: %d)", len(years), years[2024])
	for _, r := range rows {
		if r.Year == 2024 && r.Kommun == "Botkyrka" && r.Slug == "totalt" {
			t.Logf("Botkyrka 2024 totalt: count=%v per100k=%v", *r.Count, *r.Per100k)
		}
	}
}
