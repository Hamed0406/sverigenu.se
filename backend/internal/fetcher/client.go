// Package fetcher talks to all upstream open data APIs used by the dashboard:
// SMHI (weather, air quality), polisen.se (police events), SCB (population,
// income), Kolada (municipal KPIs), BRÅ (crime statistics), Riksdagen (members,
// votes), Domstolsverket (court statistics), Folkhälsomyndigheten (public health),
// Valmyndigheten (election results), Trafikverket (road conditions, flow),
// Riksbanken (policy rates), elprisetjustnu (electricity spot prices),
// Arbetsförmedlingen (job listings), and Skolverket (schools, SALSA, national
// test results). None require authentication except Trafikverket (API key) and
// polisen.se (descriptive User-Agent required by their terms of service).
package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const smhiBase  = "https://opendata-download-metobs.smhi.se/api/version/1.0"
const luftBase  = openMeteoAirBase
const policeBase = "https://polisen.se/api"

// maxResponseBytes caps the size of any upstream response body we will read.
// 32 MB is far above what any current endpoint returns; it prevents memory
// exhaustion if a malfunctioning upstream sends a huge payload.
const maxResponseBytes = 32 << 20 // 32 MB

// Client is the shared HTTP client for all upstream open data APIs.
// Each source has its own base URL field so tests can substitute a fake server.
type Client struct {
	http                  *http.Client
	smhiURL               string // overridable in tests
	luftURL               string // overridable in tests
	policeURL             string // overridable in tests
	scbURL                string // overridable in tests
	koladaURL             string // overridable in tests
	braURL                string // overridable in tests
	trafikURL             string // overridable in tests
	skolverketRegistryURL string // overridable in tests
	skolverketPlanURL     string // overridable in tests
	ssURL                 string // overridable in tests — Socialstyrelsen statistics API
	trafikKey             string // Trafikverket API authentication key
	contactEmail          string // included in the polisen.se User-Agent as required by their terms
}

// New returns a Client pointed at all production API endpoints.
// contactEmail is embedded in the User-Agent sent to polisen.se — their terms
// require a reachable contact address so they can get in touch if needed.
// trafikKey is the Trafikverket open data API key; pass "" to skip trafik syncs.
func New(contactEmail, trafikKey string) *Client {
	return &Client{
		http:                  &http.Client{Timeout: 30 * time.Second},
		smhiURL:               smhiBase,
		luftURL:               luftBase,
		policeURL:             policeBase,
		scbURL:                scbBase,
		koladaURL:             koladaBase,
		braURL:                braBase,
		trafikURL:             trafikBase,
		skolverketRegistryURL: skolverketBase,
		skolverketPlanURL:     skolverketPlanBase,
		ssURL:                 ssBase,
		trafikKey:             trafikKey,
		contactEmail:          contactEmail,
	}
}

// getURL fetches any fully-qualified URL and decodes the JSON response body.
// The response body is capped at maxResponseBytes to prevent memory exhaustion.
func (c *Client) getURL(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	slog.Debug("fetcher: GET", "url", rawURL)
	start := time.Now()
	resp, err := c.http.Do(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		slog.Debug("fetcher: GET failed", "url", rawURL, "elapsed", elapsed, "err", err)
		return err
	}
	defer resp.Body.Close()

	slog.Debug("fetcher: GET response", "url", rawURL, "status", resp.StatusCode, "elapsed", elapsed)
	if resp.StatusCode != http.StatusOK {
		// Drain the body so the TCP connection can be reused.
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return fmt.Errorf("upstream returned %d for %s", resp.StatusCode, rawURL)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out)
}

// FetchSMHITemperatures returns the latest-hour air temperature reading for
// every active SMHI station (~230 currently active) in one request. Stations
// that sent no reading this hour are still returned with Temperature == nil so
// the station list stays complete.
func (c *Client) FetchSMHITemperatures(ctx context.Context) ([]SMHIObservation, error) {
	var set smhiStationSet
	url := c.smhiURL + "/parameter/1/station-set/all/period/latest-hour/data.json"
	if err := c.getURL(ctx, url, &set); err != nil {
		return nil, err
	}

	out := make([]SMHIObservation, 0, len(set.Station))
	for _, st := range set.Station {
		obs := SMHIObservation{
			StationKey: st.Key.String(),
			Parameter:  1,
			Name:       st.Name,
			Lat:        st.Latitude,
			Lon:        st.Longitude,
			HeightM:    st.Height,
			Raw:        st,
		}
		// Take the most recent reading when the station sent any.
		if len(st.Value) > 0 {
			v := st.Value[len(st.Value)-1]
			if t, err := strconv.ParseFloat(v.Value, 64); err == nil {
				obs.Temperature = &t
				obs.Quality = v.Quality
				ms := v.Date
				obs.MeasuredAt = &ms
			}
		}
		out = append(out, obs)
	}
	return out, nil
}

// FetchNationalPoliceEvents returns the ~500 most recent police events across
// all of Sweden (no location filter).
//
// polisen.se terms require a descriptive User-Agent header on every request.
// See: https://polisen.se/om-polisen/om-webbplatsen/oppna-data/regler-for-oppna-data/
func (c *Client) FetchNationalPoliceEvents(ctx context.Context) ([]PoliceEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.policeURL+"/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard; "+c.contactEmail+")")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("polisen.se returned %d", resp.StatusCode)
	}
	var events []PoliceEvent
	return events, json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&events)
}
