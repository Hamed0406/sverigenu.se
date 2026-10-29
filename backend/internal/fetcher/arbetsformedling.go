package fetcher

import (
	"context"
	"fmt"
)

const afBase = "https://jobsearch.api.jobtechdev.se"

// afRegions maps Arbetsförmedlingen region codes (matching SCB county codes)
// to our standard two-letter county codes.
var afRegions = []struct {
	regionCode string
	countyCode string
	name       string
}{
	{"01", "ab", "Stockholm"},
	{"03", "c", "Uppsala"},
	{"04", "d", "Södermanland"},
	{"05", "e", "Östergötland"},
	{"06", "f", "Jönköping"},
	{"07", "g", "Kronoberg"},
	{"08", "h", "Kalmar"},
	{"09", "i", "Gotland"},
	{"10", "k", "Blekinge"},
	{"12", "m", "Skåne"},
	{"13", "n", "Halland"},
	{"14", "o", "Västra Götaland"},
	{"17", "s", "Värmland"},
	{"18", "t", "Örebro"},
	{"19", "u", "Västmanland"},
	{"20", "w", "Dalarna"},
	{"21", "x", "Gävleborg"},
	{"22", "y", "Västernorrland"},
	{"23", "z", "Jämtland"},
	{"24", "ac", "Västerbotten"},
	{"25", "bd", "Norrbotten"},
}

// AFJobCount holds the live job ad count for one Swedish county.
type AFJobCount struct {
	RegionCode string
	CountyCode string
	Name       string
	TotalAds   int
}

// AFField holds one occupation field and its national job ad count.
type AFField struct {
	Term      string
	ConceptID string
	Count     int
}

type afSearchResponse struct {
	Total struct {
		Value int `json:"value"`
	} `json:"total"`
	Stats []struct {
		Type   string `json:"type"`
		Values []struct {
			Term      string `json:"term"`
			ConceptID string `json:"concept_id"`
			Count     int    `json:"count"`
		} `json:"values"`
	} `json:"stats"`
}

// FetchAFJobCounts fetches the current live job ad count per Swedish county
// and the national breakdown by occupation field.
func (c *Client) FetchAFJobCounts(ctx context.Context) ([]AFJobCount, []AFField, error) {
	// Fetch occupation field stats nationally in one request.
	var natResp afSearchResponse
	if err := c.getURL(ctx, afBase+"/search?limit=0&stats=occupation-field", &natResp); err != nil {
		return nil, nil, fmt.Errorf("af national stats: %w", err)
	}

	var fields []AFField
	for _, s := range natResp.Stats {
		if s.Type != "occupation-field" {
			continue
		}
		for _, v := range s.Values {
			fields = append(fields, AFField{
				Term:      v.Term,
				ConceptID: v.ConceptID,
				Count:     v.Count,
			})
		}
	}

	// Fetch job count per county region.
	var counts []AFJobCount
	for _, reg := range afRegions {
		url := fmt.Sprintf("%s/search?limit=0&region=%s", afBase, reg.regionCode)
		var resp afSearchResponse
		if err := c.getURL(ctx, url, &resp); err != nil {
			// Skip a region on error rather than failing the whole sync.
			continue
		}
		counts = append(counts, AFJobCount{
			RegionCode: reg.regionCode,
			CountyCode: reg.countyCode,
			Name:       reg.name,
			TotalAds:   resp.Total.Value,
		})
	}

	return counts, fields, nil
}
