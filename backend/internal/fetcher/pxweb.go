package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// This file implements a generic PX-Web client: POST a query, decode the
// json-stat2 response, and flatten it into cells with full dimension
// coordinates. PX-Web powers SCB, Domstolsverket and Folkhälsomyndigheten,
// so everything here is source-agnostic.
//
// json-stat2 notes:
//   - The spec allows category.index to be a JSON array; PX-Web always emits
//     the object form modelled here. flattenJSONStat2 fails loudly (returns
//     nil) if an index map decodes empty rather than emit garbage.
//   - The spec allows value to be a sparse object; PX-Web emits a dense
//     array, which is what the struct expects.

// jsonStat2 is the response envelope of a PX-Web query with
// "response":{"format":"json-stat2"}. Value is row-major over the dimensions
// listed in ID with per-dimension lengths in Size.
type jsonStat2 struct {
	ID        []string `json:"id"`
	Size      []int    `json:"size"`
	Dimension map[string]struct {
		Category struct {
			Index map[string]int    `json:"index"`
			Label map[string]string `json:"label"`
		} `json:"category"`
	} `json:"dimension"`
	Value []interface{} `json:"value"`
}

// pxSelection is one {"code", "selection":{"filter":"item","values"}} entry
// in a PX-Web query body.
type pxSelection struct {
	Code   string
	Values []string
}

// postPXWeb POSTs a PX-Web item-filter query to tableURL and decodes the
// json-stat2 response. Dimensions not mentioned in sel are eliminated by
// PX-Web (summed to totals) when their metadata allows it.
func (c *Client) postPXWeb(ctx context.Context, tableURL string, sel []pxSelection) (*jsonStat2, error) {
	query := make([]map[string]any, len(sel))
	for i, s := range sel {
		query[i] = map[string]any{
			"code": s.Code,
			"selection": map[string]any{
				"filter": "item",
				"values": s.Values,
			},
		}
	}
	body, err := json.Marshal(map[string]any{
		"query":    query,
		"response": map[string]string{"format": "json-stat2"},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tableURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard; "+c.contactEmail+")")

	slog.Debug("fetcher: POST pxweb", "url", tableURL)
	start := time.Now()
	resp, err := c.http.Do(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		slog.Debug("fetcher: POST pxweb failed", "url", tableURL, "elapsed", elapsed, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	slog.Debug("fetcher: POST pxweb response", "url", tableURL, "status", resp.StatusCode, "elapsed", elapsed)
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return nil, fmt.Errorf("pxweb returned %d for %s", resp.StatusCode, tableURL)
	}

	var js jsonStat2
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&js); err != nil {
		return nil, fmt.Errorf("pxweb decode %s: %w", tableURL, err)
	}
	return &js, nil
}

// PXCell is one non-null cell of a flattened json-stat2 cube, carrying the
// full coordinate of the cell in both code and label form.
type PXCell struct {
	Codes  map[string]string // dimension id -> category code,  e.g. "Region" -> "01"
	Labels map[string]string // dimension id -> category label, e.g. "Region" -> "Stockholms län"
	Value  float64
}

// flattenJSONStat2 walks js.Value in row-major order (per the json-stat2
// spec) and emits one PXCell per non-null numeric value. The dimension order
// comes entirely from js.ID/js.Size — nothing is hardcoded, so the same code
// handles any PX-Web table shape. Returns nil if the cube is malformed.
func flattenJSONStat2(js *jsonStat2) []PXCell {
	n := len(js.ID)
	if n == 0 || n != len(js.Size) {
		slog.Error("pxweb: malformed cube", "ids", len(js.ID), "sizes", len(js.Size))
		return nil
	}

	// Ordered code and label slices per dimension.
	codes := make([][]string, n)
	labels := make([][]string, n)
	for i, dim := range js.ID {
		cat, ok := js.Dimension[dim]
		if !ok || len(cat.Category.Index) == 0 {
			slog.Error("pxweb: dimension missing or empty index", "dim", dim)
			return nil
		}
		codes[i] = orderedCodes(cat.Category.Index)
		labels[i] = orderedLabels(cat.Category.Index, cat.Category.Label)
		if len(codes[i]) != js.Size[i] {
			slog.Error("pxweb: size mismatch", "dim", dim, "index", len(codes[i]), "size", js.Size[i])
			return nil
		}
	}

	// Row-major strides, computed right to left.
	stride := make([]int, n)
	stride[n-1] = 1
	for i := n - 2; i >= 0; i-- {
		stride[i] = stride[i+1] * js.Size[i+1]
	}

	cells := make([]PXCell, 0, len(js.Value))
	for idx, raw := range js.Value {
		if raw == nil {
			continue
		}
		v, ok := raw.(float64)
		if !ok {
			continue
		}
		cell := PXCell{
			Codes:  make(map[string]string, n),
			Labels: make(map[string]string, n),
			Value:  v,
		}
		for i, dim := range js.ID {
			pos := (idx / stride[i]) % js.Size[i]
			cell.Codes[dim] = codes[i][pos]
			cell.Labels[dim] = labels[i][pos]
		}
		cells = append(cells, cell)
	}
	return cells
}

// orderedCodes returns the category codes ordered by their index position.
func orderedCodes(index map[string]int) []string {
	ordered := make([]string, len(index))
	for code, i := range index {
		if i < len(ordered) {
			ordered[i] = code
		}
	}
	return ordered
}

// orderedLabels returns the category labels ordered by their index position.
func orderedLabels(index map[string]int, label map[string]string) []string {
	ordered := make([]string, len(index))
	for code, i := range index {
		if i < len(ordered) {
			ordered[i] = label[code]
		}
	}
	return ordered
}
