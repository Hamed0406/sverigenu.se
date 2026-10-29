package fetcher

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

const (
	skattPerPersonBase    = "https://skatteverket.entryscape.net/rowstore/dataset/b904f54e-4dae-4d23-a740-a70864dd2ad1"
	kommunalskattNivaBase = "https://skatteverket.entryscape.net/rowstore/dataset/f3fd610c-a575-4283-8831-4f74ebc51ad2"

	skattPerPersonYearStart    = 2013
	skattPerPersonYearEnd      = 2023
	kommunalskattNivaYearStart = 2015
	kommunalskattNivaYearEnd   = 2023
)

// SkattPerPersonRow is median/mean taxes paid per municipality × tax type × year.
type SkattPerPersonRow struct {
	Year         int
	Municipality string
	TypAvSkatt   string
	BeloppSEK    int64
	MedianKr     int
	MedelKr      int
	Antal        int
}

// KommunalskattNivaRow is a tax-type × income bracket × year record.
type KommunalskattNivaRow struct {
	Year          int
	Inkomstniva   string
	Statistikterm string
	BeloppSEK     int64
	Antal         int
}

// FetchSkattPerPerson fetches actual median taxes paid per person per municipality.
// detaljeringsgrad=Typ av skatt returns 5 rows per municipality (one per typ_av_skatt:
// Direkta skatter på arbete, Indirekta skatter på arbete, Skatt på kapital,
// Skatt på konsumtion och insatsvaror, Övriga skatter och avgifter) — all are stored;
// the tax type must be part of the unique key or rows overwrite each other.
func (c *Client) FetchSkattPerPerson(ctx context.Context) ([]SkattPerPersonRow, error) {
	var all []SkattPerPersonRow
	for yr := skattPerPersonYearStart; yr <= skattPerPersonYearEnd; yr++ {
		q := url.Values{}
		q.Set("_limit", "100")
		q.Set("inkomstar", strconv.Itoa(yr))
		q.Set("gruppering", "Kommun")
		q.Set("detaljeringsgrad_skatt", "Typ av skatt")
		pageURL := skattPerPersonBase + "?" + q.Encode()

		for pageURL != "" {
			page, err := c.fetchSkattEntry(ctx, pageURL)
			if err != nil {
				return nil, fmt.Errorf("skatt_per_person %d: %w", yr, err)
			}
			for _, r := range page.Results {
				muni := r.GrupperingsVarde
				if muni == "" || r.TypAvSkatt == "" {
					continue
				}
				belopp, _ := strconv.ParseInt(r.Belopp, 10, 64)
				median, _ := strconv.Atoi(r.Median)
				medel, _ := strconv.Atoi(r.Medel)
				antal, _ := strconv.Atoi(r.Antal)
				year, _ := strconv.Atoi(r.Inkomstar)
				all = append(all, SkattPerPersonRow{
					Year:         year,
					Municipality: muni,
					TypAvSkatt:   r.TypAvSkatt,
					BeloppSEK:    belopp,
					MedianKr:     median,
					MedelKr:      medel,
					Antal:        antal,
				})
			}
			pageURL = page.Next
		}
	}
	return all, nil
}

// wantedTerms are the statistikterm values we store from f3fd610c.
// The dataset has 57 terms per bracket — we keep only the most meaningful.
var wantedTerms = map[string]bool{
	"Kommunal inkomstskatt":                        true,
	"Slutlig skatt":                                true,
	"Summa skatter och avgifter före skattereduktioner": true,
	"Statlig inkomstskatt på förvärvsinkomst":       true,
	"Allmän pensionsavgift på inkomst av anställning": true,
}

// FetchKommunalskattInkomstniva fetches key tax amounts by income bracket.
// Each income bracket × statistikterm pair is one row.
func (c *Client) FetchKommunalskattInkomstniva(ctx context.Context) ([]KommunalskattNivaRow, error) {
	var all []KommunalskattNivaRow
	for yr := kommunalskattNivaYearStart; yr <= kommunalskattNivaYearEnd; yr++ {
		q := url.Values{}
		q.Set("_limit", "100")
		q.Set("inkomstar", strconv.Itoa(yr))
		q.Set("gruppering", "Inkomstnivå")
		pageURL := kommunalskattNivaBase + "?" + q.Encode()

		for pageURL != "" {
			page, err := c.fetchSkattEntry(ctx, pageURL)
			if err != nil {
				return nil, fmt.Errorf("kommunalskatt_niva %d: %w", yr, err)
			}
			for _, r := range page.Results {
				niva := r.GrupperingsVarde
				term := r.Statistikterm
				if niva == "" || !wantedTerms[term] {
					continue
				}
				belopp, _ := strconv.ParseInt(r.Belopp, 10, 64)
				antal, _ := strconv.Atoi(r.Antal)
				year, _ := strconv.Atoi(r.Inkomstar)
				all = append(all, KommunalskattNivaRow{
					Year:          year,
					Inkomstniva:   niva,
					Statistikterm: term,
					BeloppSEK:     belopp,
					Antal:         antal,
				})
			}
			pageURL = page.Next
		}
	}
	return all, nil
}
