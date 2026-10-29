package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ValResult holds one party's result for one electoral area and year.
type ValResult struct {
	Year       int
	AreaCode   string  // "riket" or valkrets name slug
	AreaName   string  // "Riket" or "Stockholms kommun"
	PartyCode  string  // "S","M","SD","C","V","KD","L","MP","OVR"
	PartyName  string  // full Swedish name
	Votes      int
	VoteShare  float64 // percent
	Mandates   int     // only set for riket rows
}

// ValTurnout holds turnout for one electoral area and year.
type ValTurnout struct {
	Year            int
	AreaCode        string
	AreaName        string
	EligibleVoters  int
	VotesCast       int
	Turnout         float64 // percent
}

// valkrets names to area code slugs (used for DB upsert key)
var valkretsSlug = map[string]string{
	"Riket":                         "riket",
	"Blekinge län":                  "blekinge",
	"Dalarnas län":                  "dalarna",
	"Gotlands län":                  "gotland",
	"Gävleborgs län":                "gavleborg",
	"Göteborgs kommun":              "goteborg",
	"Hallands län":                  "halland",
	"Jämtlands län":                 "jamtland",
	"Jönköpings län":                "jonkoping",
	"Kalmar län":                    "kalmar",
	"Kronobergs län":                "kronoberg",
	"Malmö kommun":                  "malmo",
	"Norrbottens län":               "norrbotten",
	"Skåne läns norra och östra":    "skane_no",
	"Skåne läns södra":              "skane_s",
	"Skåne läns västra":             "skane_v",
	"Stockholms kommun":             "stockholm_stad",
	"Stockholms län":                "stockholm_lan",
	"Södermanlands län":             "sodermanland",
	"Uppsala län":                   "uppsala",
	"Värmlands län":                 "varmland",
	"Västerbottens län":             "vasterbotten",
	"Västernorrlands län":           "vasternorrland",
	"Västmanlands län":              "vastmanland",
	"Västra Götalands läns norra":   "vgr_norra",
	"Västra Götalands läns södra":   "vgr_sodra",
	"Västra Götalands läns västra":  "vgr_vastra",
	"Västra Götalands läns östra":   "vgr_ostra",
	"Örebro län":                    "orebro",
	"Östergötlands län":             "ostergotland",
}

// mainParties are the parties we track for trend charts.
var mainParties = map[string]bool{
	"S": true, "M": true, "SD": true, "C": true,
	"V": true, "KD": true, "L": true, "MP": true,
}

// FetchVal2022 fetches the final 2022 riksdag results from resultat.val.se.
// Returns per-valkrets party results and turnout data.
func (c *Client) FetchVal2022(ctx context.Context) ([]ValResult, []ValTurnout, error) {
	url := "https://resultat.val.se/data/resultat/val2022/RD_S.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "swedennu/1.0 ("+c.contactEmail+")")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("val2022: HTTP %d", resp.StatusCode)
	}

	var raw struct {
		Namn               string  `json:"namn"`
		AntalRostberattigade string `json:"antalRostberattigade"`
		Valdeltagande      string  `json:"valdeltagande"`
		PartiMandat        []struct {
			Partiforkortning string `json:"partiforkortning"`
			Partibeteckning  string `json:"partibeteckning"`
			AntalMandat      int    `json:"antalMandat"`
		} `json:"partiMandat"`
		RosterPaverkaMandat struct {
			AntalRoster  int `json:"antalRoster"`
			Partiroster  []struct {
				Partiforkortning string  `json:"partiforkortning"`
				Partibeteckning  string  `json:"partibeteckning"`
				AntalRoster      int     `json:"antalRoster"`
				AndelRoster      float64 `json:"andelRoster"`
			} `json:"partiroster"`
		} `json:"rosterPaverkaMandat"`
		Valkretsar []struct {
			Namn                  string `json:"namn"`
			AntalRostberattigade  string `json:"antalRostberattigade"`
			AntalRaknadeRoster    string `json:"antalRaknadeRoster"`
			RosterPaverkaMandat   struct {
				AntalRoster int `json:"antalRoster"`
				Partiroster []struct {
					Partiforkortning string  `json:"partiforkortning"`
					Partibeteckning  string  `json:"partibeteckning"`
					AntalRoster      int     `json:"antalRoster"`
					AndelRoster      float64 `json:"andelRoster"`
				} `json:"partiroster"`
			} `json:"rosterPaverkaMandat"`
		} `json:"valkretsar"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, nil, fmt.Errorf("val2022 decode: %w", err)
	}

	var results []ValResult
	var turnouts []ValTurnout

	// Build mandate lookup from partiMandat
	mandates := map[string]int{}
	for _, pm := range raw.PartiMandat {
		mandates[pm.Partiforkortning] = pm.AntalMandat
	}

	// National totals (riket)
	total2022 := raw.RosterPaverkaMandat.AntalRoster
	if total2022 == 0 {
		total2022 = 1
	}
	for _, p := range raw.RosterPaverkaMandat.Partiroster {
		code := p.Partiforkortning
		if !mainParties[code] {
			code = "OVR"
		}
		results = append(results, ValResult{
			Year:      2022,
			AreaCode:  "riket",
			AreaName:  "Riket",
			PartyCode: code,
			PartyName: p.Partibeteckning,
			Votes:     p.AntalRoster,
			VoteShare: p.AndelRoster,
			Mandates:  mandates[p.Partiforkortning],
		})
	}

	// Per-valkrets
	for _, vk := range raw.Valkretsar {
		slug := valkretsSlug[vk.Namn]
		if slug == "" {
			slug = vk.Namn
		}
		for _, p := range vk.RosterPaverkaMandat.Partiroster {
			code := p.Partiforkortning
			if !mainParties[code] {
				code = "OVR"
			}
			results = append(results, ValResult{
				Year:      2022,
				AreaCode:  slug,
				AreaName:  vk.Namn,
				PartyCode: code,
				PartyName: p.Partibeteckning,
				Votes:     p.AntalRoster,
				VoteShare: p.AndelRoster,
			})
		}

		// Turnout per valkrets
		eligible := parseIntStr(vk.AntalRostberattigade)
		cast := parseIntStr(vk.AntalRaknadeRoster)
		if vk.RosterPaverkaMandat.AntalRoster > 0 {
			cast = vk.RosterPaverkaMandat.AntalRoster
		}
		var turnoutPct float64
		if eligible > 0 {
			turnoutPct = float64(cast) / float64(eligible) * 100
		}
		turnouts = append(turnouts, ValTurnout{
			Year:           2022,
			AreaCode:       slug,
			AreaName:       vk.Namn,
			EligibleVoters: eligible,
			VotesCast:      cast,
			Turnout:        turnoutPct,
		})
	}

	// National turnout
	eligible := parseIntStr(raw.AntalRostberattigade)
	turnouts = append(turnouts, ValTurnout{
		Year:           2022,
		AreaCode:       "riket",
		AreaName:       "Riket",
		EligibleVoters: eligible,
		VotesCast:      total2022,
		Turnout:        84.21,
	})

	return results, turnouts, nil
}

// scbVRToSlug maps SCB valkrets codes (VR2…VR29) to the area_code slugs
// used in val_results. Matches the slugs from valkretsSlug where possible.
var scbVRToSlug = map[string]string{
	"VR00": "riket",
	"VR2":  "stockholm_lan",
	"VR3":  "uppsala",
	"VR4":  "sodermanland",
	"VR5":  "ostergotland",
	"VR6":  "jonkoping",
	"VR7":  "kronoberg",
	"VR8":  "kalmar",
	"VR9":  "gotland",
	"VR10": "blekinge",
	"VR12": "skane_v",
	"VR13": "skane_s",
	"VR14": "skane_no",
	"VR15": "halland",
	"VR17": "vgr_vastra",
	"VR18": "vgr_norra",
	"VR19": "vgr_sodra",
	"VR20": "vgr_ostra",
	"VR21": "varmland",
	"VR22": "orebro",
	"VR23": "vastmanland",
	"VR24": "dalarna",
	"VR25": "gavleborg",
	"VR26": "vasternorrland",
	"VR27": "jamtland",
	"VR28": "vasterbotten",
	"VR29": "norrbotten",
}

// scbVRYears are the riksdag election years available in SCB ME0104T3.
var scbVRYears = []string{"2006", "2010", "2014", "2018", "2022"}

// scbPartyNorm normalises historical party codes to current ones.
// FP became L (Liberalerna) in 2015.
var scbPartyNorm = map[string]string{
	"FP":     "L",
	"ÖVRIGA": "OVR",
}

// FetchValHistorySCB fetches per-valkrets riksdag election results for
// 2006–2022 from SCB ME0104T3 (Riksdagsval – valresultat efter region och parti).
// This enriches the val_results table with historical county-level trends.
func (c *Client) FetchValHistorySCB(ctx context.Context) ([]ValResult, error) {
	const tableURL = "https://api.scb.se/OV0104/v1/doris/sv/ssd/ME/ME0104/ME0104C/ME0104T3"

	vrCodes := make([]string, 0, len(scbVRToSlug))
	for code := range scbVRToSlug {
		if code != "VR00" { // skip riket — we already have it
			vrCodes = append(vrCodes, code)
		}
	}

	js, err := c.postPXWeb(ctx, tableURL, []pxSelection{
		{Code: "Region", Values: vrCodes},
		{Code: "Partimm", Values: []string{"M", "C", "FP", "KD", "MP", "S", "V", "SD", "ÖVRIGA"}},
		{Code: "ContentsCode", Values: []string{"ME0104B7"}}, // andel röster (vote share %)
		{Code: "Tid", Values: scbVRYears},
	})
	if err != nil {
		return nil, fmt.Errorf("val history SCB: %w", err)
	}

	cells := flattenJSONStat2(js)
	if cells == nil {
		return nil, fmt.Errorf("val history SCB: failed to flatten response")
	}

	var results []ValResult
	for _, cell := range cells {
		regionCode := cell.Codes["Region"]
		slug, ok := scbVRToSlug[regionCode]
		if !ok {
			continue
		}
		rawParty := cell.Codes["Partimm"]
		party := rawParty
		if norm, found := scbPartyNorm[rawParty]; found {
			party = norm
		}
		yearStr := cell.Codes["Tid"]
		year := 0
		for _, c := range yearStr {
			if c >= '0' && c <= '9' {
				year = year*10 + int(c-'0')
			}
		}
		if year == 0 {
			continue
		}
		results = append(results, ValResult{
			Year:      year,
			AreaCode:  slug,
			AreaName:  cell.Labels["Region"],
			PartyCode: party,
			PartyName: cell.Labels["Partimm"],
			VoteShare: cell.Value,
		})
	}
	return results, nil
}

// parseIntStr parses a Swedish-formatted integer string like "7 853 741" or "7853741".
func parseIntStr(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n
}
