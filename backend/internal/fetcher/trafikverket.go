package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const trafikBase = "https://api.trafikinfo.trafikverket.se/v2/data.json"

// länskod → two-letter county code mapping as defined by Trafikverket.
var lanskodToCounty = map[int]string{
	1: "ab", 3: "c", 4: "d", 5: "e", 6: "f", 7: "g", 8: "h", 9: "i",
	10: "k", 12: "m", 13: "n", 14: "o", 17: "s", 18: "t", 19: "u",
	20: "w", 21: "x", 22: "y", 23: "z", 24: "ac", 25: "bd",
}

// TrafikSituation is a road condition record from Trafikverket (RoadCondition objecttype).
// The fields are mapped to match the trafik_situations table schema:
//   - SiteType  ← ConditionText  (e.g. "Normalt", "Hal vägbana")
//   - Severity  ← ConditionCode as string
//   - Title     ← LocationText   (road segment description)
//   - Description ← ConditionInfo joined
type TrafikSituation struct {
	ID           string     `json:"id"`
	SiteType     string     `json:"site_type"`    // ConditionText: Normalt, Hal vägbana, …
	Severity     string     `json:"severity"`     // ConditionCode as string
	Title        string     `json:"title"`        // LocationText: road segment description
	Description  string     `json:"description"`  // ConditionInfo joined
	RoadNumber   string     `json:"road_number"`
	CountyCodes  []string   `json:"county_codes"` // resolved from CountyNo via länskod map
	Lat          float64    `json:"lat"`
	Lon          float64    `json:"lon"`
	StartTime    time.Time  `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
}

// trafikResponse is the raw JSON envelope from the Trafikverket RoadCondition endpoint.
type trafikResponse struct {
	Response struct {
		Result []struct {
			RoadCondition []trafikSitRaw `json:"RoadCondition"`
		} `json:"RESULT"`
	} `json:"RESPONSE"`
}

type trafikSitRaw struct {
	ID            string   `json:"Id"`
	ConditionCode int      `json:"ConditionCode"` // 1=Normal, 2=Slippery, 3=Snow/Ice, …
	ConditionInfo []string `json:"ConditionInfo"` // human-readable descriptions
	ConditionText string   `json:"ConditionText"` // e.g. "Normalt", "Hal vägbana"
	CountyNo      []int    `json:"CountyNo"`      // länskoder as ints
	LocationText  string   `json:"LocationText"`  // road segment description
	RoadNumber    string   `json:"RoadNumber"`
	StartTime     string   `json:"StartTime"`
	ModifiedTime  string   `json:"ModifiedTime"`
	Deleted       bool     `json:"Deleted"`
	Geometry      struct {
		WGS84 string `json:"WGS84"` // "POINT (lon lat)" or "LINESTRING (…)"
	} `json:"Geometry"`
}

// coordPairRE matches the first coordinate pair in any WKT geometry (POINT or LINESTRING).
var coordPairRE = regexp.MustCompile(`\(\s*([0-9.+\-]+)\s+([0-9.+\-]+)`)

// parseWGS84FirstPoint extracts the first (lon lat) pair from a WKT POINT or LINESTRING.
func parseWGS84FirstPoint(s string) (lat, lon float64) {
	m := coordPairRE.FindStringSubmatch(s)
	if len(m) < 3 {
		return 0, 0
	}
	lon, _ = strconv.ParseFloat(m[1], 64)
	lat, _ = strconv.ParseFloat(m[2], 64)
	return lat, lon
}

func parseTrafikTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	// Trafikverket returns ISO 8601: "2026-06-15T10:30:00.000+02:00"
	layouts := []string{
		"2006-01-02T15:04:05.999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}

// TrafikFlowItem is one lane-level traffic flow measurement from Trafikverket.
type TrafikFlowItem struct {
	SiteID      int       `json:"site_id"`
	CountyCode  string    `json:"county_code"`
	FlowRate    int       `json:"flow_rate"`    // vehicles per hour for this lane
	AvgSpeed    float64   `json:"avg_speed"`    // km/h
	Lane        string    `json:"lane"`
	MeasuredAt  time.Time `json:"measured_at"`
}

// trafikFlowResponse is the raw JSON envelope for TrafficFlow.
type trafikFlowResponse struct {
	Response struct {
		Result []struct {
			TrafficFlow []trafikFlowRaw `json:"TrafficFlow"`
		} `json:"RESULT"`
	} `json:"RESPONSE"`
}

type trafikFlowRaw struct {
	SiteID                           int     `json:"SiteId"`
	MeasurementTime                  string  `json:"MeasurementTime"`
	VehicleFlowRate                  int     `json:"VehicleFlowRate"`
	AverageVehicleSpeed              float64 `json:"AverageVehicleSpeed"`
	CountyNo                         int     `json:"CountyNo"`
	SpecificLane                     string  `json:"SpecificLane"`
	Deleted                          bool    `json:"Deleted"`
}

// FetchTrafikFlow fetches real-time traffic flow measurements from Trafikverket.
// Data covers Stockholm and Västra Götaland sensor networks (~3,300 lane measurements).
func (c *Client) FetchTrafikFlow(ctx context.Context) ([]TrafikFlowItem, error) {
	query := fmt.Sprintf(`<REQUEST>
  <LOGIN authenticationkey="%s"/>
  <QUERY objecttype="TrafficFlow" schemaversion="1.4" limit="5000"></QUERY>
</REQUEST>`, c.trafikKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.trafikURL,
		bytes.NewBufferString(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard; "+c.contactEmail+")")

	slog.Debug("fetcher: POST trafikverket flow", "url", c.trafikURL)
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	slog.Debug("fetcher: trafikverket flow response", "status", resp.StatusCode, "elapsed", time.Since(start).Round(time.Millisecond))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("trafikverket flow returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env trafikFlowResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&env); err != nil {
		return nil, fmt.Errorf("trafikverket flow decode: %w", err)
	}

	var raw []trafikFlowRaw
	if len(env.Response.Result) > 0 {
		raw = env.Response.Result[0].TrafficFlow
	}

	out := make([]TrafikFlowItem, 0, len(raw))
	for _, r := range raw {
		if r.Deleted {
			continue
		}
		cc, ok := lanskodToCounty[r.CountyNo]
		if !ok {
			continue
		}
		var mt time.Time
		if t := parseTrafikTime(r.MeasurementTime); t != nil {
			mt = *t
		}
		out = append(out, TrafikFlowItem{
			SiteID:     r.SiteID,
			CountyCode: cc,
			FlowRate:   r.VehicleFlowRate,
			AvgSpeed:   r.AverageVehicleSpeed,
			Lane:       r.SpecificLane,
			MeasuredAt: mt,
		})
	}
	return out, nil
}

// FetchTrafik fetches road condition records from Trafikverket (RoadCondition objecttype).
// Returns all current (non-deleted) road segments with their condition status.
// In summer most segments report "Normalt"; data is most differentiated in winter.
func (c *Client) FetchTrafik(ctx context.Context) ([]TrafikSituation, error) {
	query := fmt.Sprintf(`<REQUEST>
  <LOGIN authenticationkey="%s"/>
  <QUERY objecttype="RoadCondition" schemaversion="1" limit="2000"></QUERY>
</REQUEST>`, c.trafikKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.trafikURL,
		bytes.NewBufferString(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard; "+c.contactEmail+")")

	slog.Debug("fetcher: POST trafikverket", "url", c.trafikURL)
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	slog.Debug("fetcher: trafikverket response", "status", resp.StatusCode, "elapsed", time.Since(start).Round(time.Millisecond))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("trafikverket returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env trafikResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&env); err != nil {
		return nil, fmt.Errorf("trafikverket decode: %w", err)
	}

	var raw []trafikSitRaw
	if len(env.Response.Result) > 0 {
		raw = env.Response.Result[0].RoadCondition
	}

	out := make([]TrafikSituation, 0, len(raw))
	for _, r := range raw {
		if r.Deleted {
			continue
		}

		// Resolve länskoder to county codes.
		var counties []string
		seen := map[string]bool{}
		for _, code := range r.CountyNo {
			if cc, ok := lanskodToCounty[code]; ok && !seen[cc] {
				counties = append(counties, cc)
				seen[cc] = true
			}
		}

		// WGS84 may be a POINT or LINESTRING — take the first coordinate pair.
		lat, lon := parseWGS84FirstPoint(r.Geometry.WGS84)

		var startT time.Time
		if t := parseTrafikTime(r.StartTime); t != nil {
			startT = *t
		}

		sit := TrafikSituation{
			ID:          r.ID,
			SiteType:    r.ConditionText,
			Severity:    fmt.Sprintf("%d", r.ConditionCode),
			Title:       r.LocationText,
			Description: strings.Join(r.ConditionInfo, ", "),
			RoadNumber:  r.RoadNumber,
			CountyCodes: counties,
			Lat:         lat,
			Lon:         lon,
			StartTime:   startT,
		}
		out = append(out, sit)
	}
	return out, nil
}
