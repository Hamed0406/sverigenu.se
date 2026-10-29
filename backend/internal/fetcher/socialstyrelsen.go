package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"swedennu/internal/geo"
)

// ssBase is the Socialstyrelsen statistics database API base URL.
// No API key required — all data is publicly available.
const ssBase = "https://sdb.socialstyrelsen.se/api/v1/sv"

// maxSSPages caps pagination to avoid runaway fetches.
const maxSSPages = 20

// ssAPIResponse is the envelope returned by every Socialstyrelsen result endpoint.
type ssAPIResponse struct {
	Data      []json.RawMessage `json:"data"`
	NastaSida *string           `json:"nasta_sida"`
}

// ssRegionList is the envelope for /region metadata endpoints.
type ssRegionList []struct {
	ID   int    `json:"id"`
	Kod  string `json:"kod"`
	Text string `json:"text"`
}

// ssDeathDiagnos maps diagnos codes to Swedish display names.
var ssDeathDiagnos = map[string]string{
	"99":      "Samtliga dödsorsaker",
	"C00-D48": "Tumörsjukdomar (cancer)",
	"I00-I99": "Hjärt- och kärlsjukdomar",
	"Alk":     "Alkoholrelaterad dödlighet",
	"Spec1":   "Läkemedels- och narkotikaförgiftningar",
}

// ssBirthVars maps variabel codes to Swedish display names.
var ssBirthVars = map[string]string{
	"V35": "Födda barn",
	"V17": "Alla förlossningar",
	"V18": "Kejsarsnitt",
	"V40": "Apgar < 7 vid 5 min",
	"V41": "Födelsevikt < 2500 g",
	"V10": "Röker i tidig graviditet",
}

// ssBirthRegion maps Socialstyrelsen birth region IDs to county codes.
// Birth region IDs differ from SCB codes so they are hardcoded here.
var ssBirthRegion = map[int]string{
	0: "", 110: "ab", 112: "c", 113: "d", 121: "e", 122: "f",
	123: "g", 124: "h", 126: "i", 127: "k", 128: "m", 142: "n",
	150: "o", 154: "s", 155: "t", 156: "u", 157: "w", 161: "x",
	162: "y", 163: "z", 164: "ac", 165: "bd",
}

// ssBirthRegionName maps birth region IDs to display names.
var ssBirthRegionName = map[int]string{
	0: "Riket", 110: "Stockholm", 112: "Uppsala", 113: "Södermanland",
	121: "Östergötland", 122: "Jönköping", 123: "Kronoberg", 124: "Kalmar",
	126: "Gotland", 127: "Blekinge", 128: "Skåne", 142: "Halland",
	150: "Västra Götaland", 154: "Värmland", 155: "Örebro", 156: "Västmanland",
	157: "Dalarna", 161: "Gävleborg", 162: "Västernorrland", 163: "Jämtland",
	164: "Västerbotten", 165: "Norrbotten",
}

// parseSVFloat parses a Swedish decimal string ("56,3") to *float64.
// Returns nil if the string is empty or cannot be parsed.
func parseSVFloat(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == ".." || s == "-" {
		return nil
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", ".")
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return nil
	}
	return &f
}

// parseSVInt parses a Swedish integer string ("95 626") to *int.
func parseSVInt(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" || s == ".." || s == "-" {
		return nil
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", "")
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err != nil {
		return nil
	}
	return &i
}

// fetchSSPaginated fetches all pages from a Socialstyrelsen result URL,
// calling process for each page's raw data slice.
func (c *Client) fetchSSPaginated(ctx context.Context, firstURL string, process func([]json.RawMessage) error) error {
	url := firstURL
	for page := 0; url != "" && page < maxSSPages; page++ {
		var resp ssAPIResponse
		if err := c.getURL(ctx, url, &resp); err != nil {
			return err
		}
		if err := process(resp.Data); err != nil {
			return err
		}
		if resp.NastaSida != nil && *resp.NastaSida != "" {
			url = *resp.NastaSida
		} else {
			url = ""
		}
	}
	return nil
}

// FetchSSDeaths fetches national cause-of-death counts from Socialstyrelsen
// for all 20 age groups and 5 key diagnoses. Returns raw age-group rows;
// the handler aggregates them by summing across age groups per diagnos/year.
func (c *Client) FetchSSDeaths(ctx context.Context) ([]SSDeath, error) {
	url := c.ssURL + "/dodsorsaker/resultat/matt/1/diagnos/99,C00-D48,I00-I99,Alk,Spec1" +
		"/alder/1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20/kon/3/region/0?per_sida=5000"

	type row struct {
		DiagnosID string `json:"diagnosId"`
		RegionID  int    `json:"regionId"`
		AlderID   int    `json:"alderId"`
		KonID     int    `json:"konId"`
		MattID    int    `json:"mattId"`
		Ar        int    `json:"ar"`
		Varde     string `json:"varde"`
	}

	var out []SSDeath
	err := c.fetchSSPaginated(ctx, url, func(data []json.RawMessage) error {
		for _, raw := range data {
			var r row
			if err := json.Unmarshal(raw, &r); err != nil {
				continue
			}
			name := ssDeathDiagnos[r.DiagnosID]
			if name == "" {
				name = r.DiagnosID
			}
			out = append(out, SSDeath{
				DiagnosID:   r.DiagnosID,
				DiagnosName: name,
				AgeID:       r.AlderID,
				Year:        r.Ar,
				Value:       parseSVInt(r.Varde),
			})
		}
		return nil
	})
	return out, err
}

// FetchSSDental fetches dental visit rates (% of residents who visited the
// dentist in the last year) per county from Socialstyrelsen tandhälsa API.
func (c *Client) FetchSSDental(ctx context.Context) ([]SSDental, error) {
	// Load region metadata to map region_id → {kod, name}.
	var regions ssRegionList
	if err := c.getURL(ctx, c.ssURL+"/tandhalsa/region", &regions); err != nil {
		return nil, fmt.Errorf("ss dental region list: %w", err)
	}
	regionMeta := make(map[int]struct{ Kod, Name string }, len(regions))
	for _, reg := range regions {
		regionMeta[reg.ID] = struct{ Kod, Name string }{reg.Kod, reg.Text}
	}

	url := c.ssURL + "/tandhalsa/resultat/indikator/1/alder/0/kon/3/matt/2?per_sida=5000"

	type row struct {
		IndikatorID string `json:"indikatorId"`
		RegionID    int    `json:"regionId"`
		AlderID     int    `json:"alderId"`
		KonID       int    `json:"konId"`
		MattID      int    `json:"mattId"`
		Ar          int    `json:"ar"`
		Varde       string `json:"varde"`
	}

	var out []SSDental
	err := c.fetchSSPaginated(ctx, url, func(data []json.RawMessage) error {
		for _, raw := range data {
			var r row
			if err := json.Unmarshal(raw, &r); err != nil {
				continue
			}
			meta := regionMeta[r.RegionID]
			countyCode := ""
			if meta.Kod != "" && meta.Kod != "00" {
				scbKey := meta.Kod
				if len(scbKey) == 4 {
					// Municipality code — take first 2 digits as county prefix.
					scbKey = scbKey[:2]
				}
				countyCode = geo.CountyBySCBCode(scbKey)
			}
			out = append(out, SSDental{
				RegionID:   r.RegionID,
				RegionName: meta.Name,
				CountyCode: countyCode,
				Year:       r.Ar,
				Value:      parseSVFloat(r.Varde),
			})
		}
		return nil
	})
	return out, err
}

// FetchSSBirths fetches births and newborn health indicators per region
// from Socialstyrelsen graviditeter/förlossningar/nyfödda API.
func (c *Client) FetchSSBirths(ctx context.Context) ([]SSBirth, error) {
	url := c.ssURL + "/graviditeterforlossningarochnyfodda/resultat" +
		"/variabel/V35,V17,V18,V40,V41,V10/alder/0/paritet/0?per_sida=5000"

	type row struct {
		VariabelID string `json:"variabelId"`
		RegionID   int    `json:"regionId"`
		AlderID    int    `json:"alderId"`
		ParitetID  int    `json:"paritetId"`
		MattID     int    `json:"mattId"`
		Ar         int    `json:"ar"`
		Varde      string `json:"varde"`
	}

	var out []SSBirth
	err := c.fetchSSPaginated(ctx, url, func(data []json.RawMessage) error {
		for _, raw := range data {
			var r row
			if err := json.Unmarshal(raw, &r); err != nil {
				continue
			}
			name := ssBirthVars[r.VariabelID]
			if name == "" {
				name = r.VariabelID
			}
			countyCode := ssBirthRegion[r.RegionID]
			regionName := ssBirthRegionName[r.RegionID]
			if regionName == "" {
				regionName = fmt.Sprintf("Region %d", r.RegionID)
			}
			out = append(out, SSBirth{
				VariabelID:   r.VariabelID,
				VariabelName: name,
				RegionID:     r.RegionID,
				RegionName:   regionName,
				CountyCode:   countyCode,
				Year:         r.Ar,
				Value:        parseSVInt(r.Varde),
			})
		}
		return nil
	})
	return out, err
}
