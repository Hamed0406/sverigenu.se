package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const (
	foretagsBase   = "https://skatteverket.entryscape.net/rowstore/dataset/6727f44f-56e8-4115-a8f5-615c1e7cce90"
	kapitalBase    = "https://skatteverket.entryscape.net/rowstore/dataset/186462ce-e2c1-4429-8ef9-401d18f82ce8"
	skattYearStart = 2013
	skattYearEnd   = 2023
)

// ForetagsRow is one municipality × year corporate profit record.
type ForetagsRow struct {
	Year         int
	Municipality string
	BeloppSEK    int64
	Antal        int
}

// KapitalRow is one county × year × statistikterm capital income record.
type KapitalRow struct {
	Year         int
	CountyName   string
	Statistikterm string
	BeloppSEK    int64
	Antal        int
}

type skattEntryRow struct {
	Gruppering       string `json:"gruppering"`
	GrupperingsVarde string `json:"grupperingsvärde"`
	Statistikterm    string `json:"statistikterm"`
	Belopp           string `json:"belopp"`
	Antal            string `json:"antal"`
	Inkomstar        string `json:"inkomstar"`
	Median           string `json:"median"`
	Medel            string `json:"medel"`
	Detaljeringsgrad string `json:"detaljeringsgrad_skatt"`
	TypAvSkatt       string `json:"typ_av_skatt"`
}

type skattEntryPage struct {
	Next    string          `json:"next"`
	Results []skattEntryRow `json:"results"`
}

// FetchForetagsoverskott fetches corporate profit per municipality for all years.
func (c *Client) FetchForetagsoverskott(ctx context.Context) ([]ForetagsRow, error) {
	var all []ForetagsRow
	for yr := skattYearStart; yr <= skattYearEnd; yr++ {
		q := url.Values{}
		q.Set("_limit", "100")
		q.Set("inkomstar", strconv.Itoa(yr))
		q.Set("gruppering", "Kommun")
		pageURL := foretagsBase + "?" + q.Encode()

		for pageURL != "" {
			page, err := c.fetchSkattEntry(ctx, pageURL)
			if err != nil {
				return nil, fmt.Errorf("foretagsoverskott %d: %w", yr, err)
			}
			for _, r := range page.Results {
				belopp, _ := strconv.ParseInt(r.Belopp, 10, 64)
				antal, _ := strconv.Atoi(r.Antal)
				if r.GrupperingsVarde == "" {
					continue
				}
				all = append(all, ForetagsRow{
					Year:         yr,
					Municipality: r.GrupperingsVarde,
					BeloppSEK:    belopp,
					Antal:        antal,
				})
			}
			pageURL = page.Next
		}
	}
	return all, nil
}

// FetchKapitalinkomst fetches capital income per county for all years.
func (c *Client) FetchKapitalinkomst(ctx context.Context) ([]KapitalRow, error) {
	var all []KapitalRow
	for yr := skattYearStart; yr <= skattYearEnd; yr++ {
		q := url.Values{}
		q.Set("_limit", "100")
		q.Set("inkomstar", strconv.Itoa(yr))
		q.Set("gruppering", "Län")
		pageURL := kapitalBase + "?" + q.Encode()

		for pageURL != "" {
			page, err := c.fetchSkattEntry(ctx, pageURL)
			if err != nil {
				return nil, fmt.Errorf("kapitalinkomst %d: %w", yr, err)
			}
			for _, r := range page.Results {
				belopp, _ := strconv.ParseInt(r.Belopp, 10, 64)
				antal, _ := strconv.Atoi(r.Antal)
				if r.GrupperingsVarde == "" || r.Statistikterm == "" {
					continue
				}
				all = append(all, KapitalRow{
					Year:          yr,
					CountyName:    r.GrupperingsVarde,
					Statistikterm: r.Statistikterm,
					BeloppSEK:     belopp,
					Antal:         antal,
				})
			}
			pageURL = page.Next
		}
	}
	return all, nil
}

func (c *Client) fetchSkattEntry(ctx context.Context, rawURL string) (*skattEntryPage, error) {
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
	var page skattEntryPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return &page, nil
}
