package geo

import (
	_ "embed"
	"encoding/json"
	"log/slog"
)

// lan.geojson is the okfse/sweden-geojson county boundary set (public data).
// Each feature has properties.l_id = the numeric länskod (1..25 with gaps).
//
//go:embed data/lan.geojson
var lanGeoJSON []byte

// lidCounty maps the geojson l_id property to the two-letter county code.
var lidCounty = map[int]string{
	1: "ab", 3: "c", 4: "d", 5: "e", 6: "f", 7: "g", 8: "h", 9: "i",
	10: "k", 12: "m", 13: "n", 14: "o", 17: "s", 18: "t", 19: "u",
	20: "w", 21: "x", 22: "y", 23: "z", 24: "ac", 25: "bd",
}

// countyPolygons holds the parsed boundaries: one entry per county, each a set
// of rings in [lon, lat] order (GeoJSON convention).
var countyPolygons = func() map[string][][][2]float64 {
	var fc struct {
		Features []struct {
			Properties struct {
				LID int `json:"l_id"`
			} `json:"properties"`
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(lanGeoJSON, &fc); err != nil {
		slog.Error("geo: failed to parse embedded lan.geojson", "err", err)
		return nil
	}

	polys := make(map[string][][][2]float64, len(fc.Features))
	for _, f := range fc.Features {
		code, ok := lidCounty[f.Properties.LID]
		if !ok {
			continue
		}
		switch f.Geometry.Type {
		case "Polygon":
			var rings [][][2]float64
			if err := json.Unmarshal(f.Geometry.Coordinates, &rings); err == nil {
				polys[code] = append(polys[code], rings...)
			}
		case "MultiPolygon":
			var multi [][][][2]float64
			if err := json.Unmarshal(f.Geometry.Coordinates, &multi); err == nil {
				for _, rings := range multi {
					polys[code] = append(polys[code], rings...)
				}
			}
		}
	}
	return polys
}()

// countyForPointPIP returns the county whose boundary contains the point, or
// "" when no polygon contains it (offshore). Only outer rings are tested —
// Swedish counties have no holes that another county fills, so treating every
// ring as an outer boundary is correct here.
func countyForPointPIP(lat, lon float64) string {
	for code, rings := range countyPolygons {
		for _, ring := range rings {
			if pointInRing(lon, lat, ring) {
				return code
			}
		}
	}
	return ""
}

// pointInRing implements the ray-casting algorithm: cast a horizontal ray from
// the point and count boundary crossings; an odd count means inside.
// Coordinates are [lon, lat] (x, y) per GeoJSON convention.
func pointInRing(x, y float64, ring [][2]float64) bool {
	inside := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > y) != (yj > y) &&
			x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}
