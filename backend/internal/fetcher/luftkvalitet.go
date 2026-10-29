package fetcher

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"swedennu/internal/geo"
)

// openMeteoAirBase is the Open-Meteo air quality endpoint. No API key required.
const openMeteoAirBase = "https://air-quality-api.open-meteo.com/v1/air-quality"

type openMeteoAirResp struct {
	Current struct {
		Pm25            float64 `json:"pm2_5"`
		Pm10            float64 `json:"pm10"`
		NitrogenDioxide float64 `json:"nitrogen_dioxide"`
		Ozone           float64 `json:"ozone"`
	} `json:"current"`
}

// FetchAirQuality fetches current PM2.5, PM10, NO₂ and O₃ for all 21 Swedish
// counties from the Open-Meteo air quality API (CAMS European model).
// Each county's centroid is used as the query coordinate.
// Parameter mapping: 1=PM10, 2=PM2.5, 5=NO2, 8=O3 (matches the old SMHI codes
// so the DB schema, handler, and frontend are unchanged).
func (c *Client) FetchAirQuality(ctx context.Context) ([]AirQualityObservation, error) {
	nowMs := time.Now().UnixMilli()
	var out []AirQualityObservation
	failed := 0

	for _, county := range geo.Counties {
		url := fmt.Sprintf(
			"%s?latitude=%.4f&longitude=%.4f&current=pm2_5,pm10,nitrogen_dioxide,ozone&timezone=Europe%%2FStockholm",
			c.luftURL, county.CLat, county.CLon,
		)
		var resp openMeteoAirResp
		if err := c.getURL(ctx, url, &resp); err != nil {
			slog.Warn("open-meteo air quality fetch failed", "county", county.Code, "err", err)
			failed++
			continue
		}

		type paramEntry struct {
			id    int
			name  string
			value float64
		}
		params := []paramEntry{
			{2, "PM2.5", resp.Current.Pm25},
			{1, "PM10", resp.Current.Pm10},
			{5, "NO2", resp.Current.NitrogenDioxide},
			{8, "O3", resp.Current.Ozone},
		}
		nowMsCopy := nowMs
		for _, p := range params {
			v := p.value
			out = append(out, AirQualityObservation{
				StationKey: county.Code + "_" + strconv.Itoa(p.id),
				Parameter:  p.id,
				ParamName:  p.name,
				Name:       county.LanName,
				CountyCode: county.Code,
				Lat:        county.CLat,
				Lon:        county.CLon,
				Value:      &v,
				Quality:    "ok",
				MeasuredAt: &nowMsCopy,
			})
		}
	}
	if failed == len(geo.Counties) {
		return nil, fmt.Errorf("all %d counties failed to fetch air quality from Open-Meteo", len(geo.Counties))
	}
	return out, nil
}
