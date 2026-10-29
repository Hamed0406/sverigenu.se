// Package geo maps coordinates and Swedish place names to the 21 counties (län).
package geo

import "strings"

// County is one of Sweden's 21 counties. Code matches the SVG path ids on the
// landing page (id="lan-ab" → "ab"); SCBCode is the official two-digit länskod
// (note the gaps at 02/11/15/16 — historic counties merged away).
type County struct {
	Code    string  // "ab".."bd" — used in URLs and API responses
	SCBCode string  // "01".."25" — official länskod
	Name    string  // short name, matches data-name in index.html
	LanName string  // official county name as used by polisen.se, e.g. "Stockholms län"
	CLat    float64 // centroid — fallback for offshore points
	CLon    float64
}

// Counties is ordered by SCBCode. The slice (not a map) keeps API responses in
// a stable, official order.
var Counties = []County{
	{"ab", "01", "Stockholm", "Stockholms län", 59.35, 18.15},
	{"c", "03", "Uppsala", "Uppsala län", 60.05, 17.75},
	{"d", "04", "Södermanland", "Södermanlands län", 59.05, 16.75},
	{"e", "05", "Östergötland", "Östergötlands län", 58.45, 15.75},
	{"f", "06", "Jönköping", "Jönköpings län", 57.55, 14.35},
	{"g", "07", "Kronoberg", "Kronobergs län", 56.85, 14.55},
	{"h", "08", "Kalmar", "Kalmar län", 57.05, 16.15},
	{"i", "09", "Gotland", "Gotlands län", 57.45, 18.45},
	{"k", "10", "Blekinge", "Blekinge län", 56.25, 15.25},
	{"m", "12", "Skåne", "Skåne län", 55.85, 13.55},
	{"n", "13", "Halland", "Hallands län", 56.85, 12.85},
	{"o", "14", "Västra Götaland", "Västra Götalands län", 58.25, 12.65},
	{"s", "17", "Värmland", "Värmlands län", 59.85, 13.05},
	{"t", "18", "Örebro", "Örebro län", 59.35, 15.05},
	{"u", "19", "Västmanland", "Västmanlands län", 59.75, 16.25},
	{"w", "20", "Dalarna", "Dalarnas län", 60.85, 14.75},
	{"x", "21", "Gävleborg", "Gävleborgs län", 61.35, 16.35},
	{"y", "22", "Västernorrland", "Västernorrlands län", 63.05, 17.35},
	{"z", "23", "Jämtland", "Jämtlands län", 63.25, 14.35},
	{"ac", "24", "Västerbotten", "Västerbottens län", 64.75, 18.55},
	{"bd", "25", "Norrbotten", "Norrbottens län", 66.85, 20.95},
}

// scbCodeCounty maps the official länskod ("01".."25") to the two-letter code.
var scbCodeCounty = func() map[string]string {
	m := make(map[string]string, len(Counties))
	for _, c := range Counties {
		m[c.SCBCode] = c.Code
	}
	return m
}()

// CountyBySCBCode returns "ab".."bd" for länskod "01".."25", or "" if unknown.
// Also accepts a 4-digit kommunkod prefix sliced to 2 chars by the caller.
func CountyBySCBCode(code string) string {
	return scbCodeCounty[code]
}

// SCBCountyCodes returns the 21 länskoder in official order — used to build
// PX-Web Region selections without hardcoding the list twice.
func SCBCountyCodes() []string {
	out := make([]string, len(Counties))
	for i, c := range Counties {
		out[i] = c.SCBCode
	}
	return out
}

// lanNameCounty maps the official county names (and their "X län"-stripped
// short forms) to the county code, lowercased for case-insensitive lookup.
var lanNameCounty = func() map[string]string {
	m := make(map[string]string, len(Counties)*2)
	for _, c := range Counties {
		m[strings.ToLower(c.LanName)] = c.Code
		m[strings.ToLower(c.Name)] = c.Code
	}
	return m
}()

// CountyForPoliceEvent resolves a polisen.se event to a county code, trying in
// order: exact county-name match ("Norrbottens län"), municipality lookup
// ("Kiruna"), point-in-polygon on the event GPS. Returns "" when nothing matches
// (e.g. nationwide notices with location name "Sverige").
func CountyForPoliceEvent(locationName string, lat, lon float64) string {
	name := strings.TrimSpace(locationName)
	if code, ok := lanNameCounty[strings.ToLower(name)]; ok {
		return code
	}
	if code, ok := municipalityCounty[name]; ok {
		return code
	}
	if lat != 0 || lon != 0 {
		if code := countyForPointPIP(lat, lon); code != "" {
			return code
		}
	}
	return ""
}

// CountyForPoint resolves a coordinate to a county code: point-in-polygon
// first, then nearest county boundary as a fallback for offshore stations
// (lighthouses and buoys fall outside the land polygons).
func CountyForPoint(lat, lon float64) string {
	if code := countyForPointPIP(lat, lon); code != "" {
		return code
	}
	if code := nearestCountyBoundary(lat, lon); code != "" {
		return code
	}
	return nearestCountyCentroid(lat, lon)
}

// nearestCountyBoundary returns the county whose boundary vertex is closest to
// the point. Far more accurate than centroids for offshore points — an island
// lighthouse resolves to the county whose coastline it sits off, not whichever
// county centre happens to be nearer. Only runs on PIP misses (a handful of
// stations per sync), so the exhaustive vertex scan is cheap.
func nearestCountyBoundary(lat, lon float64) string {
	best, bestDist := "", -1.0
	for code, rings := range countyPolygons {
		for _, ring := range rings {
			for _, pt := range ring {
				dLat := lat - pt[1]
				dLon := (lon - pt[0]) * 0.5 // ~cos(60°) — Sweden spans lat 55–69
				d := dLat*dLat + dLon*dLon
				if bestDist < 0 || d < bestDist {
					best, bestDist = code, d
				}
			}
		}
	}
	return best
}

// nearestCountyCentroid is the last-resort fallback, used only if the embedded
// GeoJSON failed to parse and no boundaries are available.
func nearestCountyCentroid(lat, lon float64) string {
	best, bestDist := "", -1.0
	for _, c := range Counties {
		dLat := lat - c.CLat
		dLon := (lon - c.CLon) * 0.5
		d := dLat*dLat + dLon*dLon
		if bestDist < 0 || d < bestDist {
			best, bestDist = c.Code, d
		}
	}
	return best
}
