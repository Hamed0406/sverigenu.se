package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// raaWFSBase is Riksantikvarieämbetet's public INSPIRE WFS for cultural
// heritage sites (kulturhistoriska lämningar). The point layer holds ~530k
// features with clean kommun/län attributes, and resultType=hits returns a
// count without transferring any feature data.
const raaWFSBase = "https://inspire-raa.metria.se/geoserver/Kulturhistoriska_lamningar/wfs"

// FornlamningCount is the number of registered fornlämningar (point layer)
// in one municipality.
type FornlamningCount struct {
	Kommun string // proper-case municipality name as used by RAÄ/Kolada
	Lan    string // county name from the CQL filter context
	Antal  int
}

var numberMatchedRE = regexp.MustCompile(`numberMatched="(\d+)"`)

// fetchRAACount runs one resultType=hits query and parses numberMatched.
func (c *Client) fetchRAACount(ctx context.Context, cql string) (int, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "GetFeature")
	q.Set("typeNames", "Kulturhistoriska_lamningar:kulturhistoriska_fornlamningar_point")
	q.Set("resultType", "hits")
	if cql != "" {
		q.Set("cql_filter", cql)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raaWFSBase+"?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return 0, fmt.Errorf("raa wfs returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	m := numberMatchedRE.FindSubmatch(body)
	if m == nil {
		return 0, fmt.Errorf("raa wfs: no numberMatched in response")
	}
	var n int
	fmt.Sscanf(string(m[1]), "%d", &n)
	return n, nil
}

// FetchFornlamningar counts registered fornlämningar per municipality.
// municipalities is the proper-case name list (from Kolada) used in the CQL
// filters; RAÄ stores kommun names in the same casing. One hits query per
// municipality with a small delay keeps the load polite (~290 requests, run
// weekly). Municipalities RAÄ does not know return count 0 and are skipped.
func (c *Client) FetchFornlamningar(ctx context.Context, municipalities []string) ([]FornlamningCount, error) {
	var out []FornlamningCount
	for _, m := range municipalities {
		// Kommun names contain no single quotes, so plain quoting is safe.
		n, err := c.fetchRAACount(ctx, "kommun='"+m+"'")
		if err != nil {
			return nil, fmt.Errorf("fornlamningar %s: %w", m, err)
		}
		if n > 0 {
			out = append(out, FornlamningCount{Kommun: m, Antal: n})
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(120 * time.Millisecond):
		}
	}
	return out, nil
}
