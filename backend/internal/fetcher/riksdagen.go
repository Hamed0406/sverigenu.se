package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const riksdagenBase = "https://data.riksdagen.se"

// RiksdagenMember is one active MP from the personlista endpoint.
type RiksdagenMember struct {
	ID        string
	FirstName string
	LastName  string
	Party     string
	Valkrets  string
	Gender    string
	BirthYear int
	PhotoURL  string
	Status    string
}

// RiksdagenVoteResult is the aggregated tally for one party in one vote.
type RiksdagenVoteResult struct {
	VoteringID  string
	RM          string
	DokID       string
	Beteckning  string
	Title       string
	Datum       string // "YYYY-MM-DD"
	Party       string
	Ja          int
	Nej         int
	Avstar      int
	Franvarande int
}

// ---- members -------------------------------------------------------------------

type personlistaResp struct {
	Personlista struct {
		Person []rawPerson `json:"person"`
	} `json:"personlista"`
}

type rawPerson struct {
	IntressентID string `json:"intressent_id"`
	Tilltalsnamn string `json:"tilltalsnamn"`
	Efternamn    string `json:"efternamn"`
	Parti        string `json:"parti"`
	Valkrets     string `json:"valkrets"`
	Kon          string `json:"kon"`
	FoddAr       string `json:"fodd_ar"`
	Status       string `json:"status"`
	BildURL192   string `json:"bild_url_192"`
}

// FetchRiksdagenMembers returns all currently active (tjänstgörande) MPs.
func (c *Client) FetchRiksdagenMembers(ctx context.Context) ([]RiksdagenMember, error) {
	var resp personlistaResp
	u := riksdagenBase + "/personlista/?rdlstatus=tjanst&utformat=json&antal=500"
	if err := c.getURL(ctx, u, &resp); err != nil {
		return nil, fmt.Errorf("riksdagen members: %w", err)
	}

	var out []RiksdagenMember
	for _, p := range resp.Personlista.Person {
		if p.Status != "Tjänstgörande riksdagsledamot" {
			continue
		}
		yr := 0
		fmt.Sscanf(p.FoddAr, "%d", &yr)
		out = append(out, RiksdagenMember{
			ID:        p.IntressентID,
			FirstName: p.Tilltalsnamn,
			LastName:  p.Efternamn,
			Party:     p.Parti,
			Valkrets:  p.Valkrets,
			Gender:    p.Kon,
			BirthYear: yr,
			PhotoURL:  p.BildURL192,
			Status:    p.Status,
		})
	}
	return out, nil
}

// ---- votes ---------------------------------------------------------------------

// voteringlistaResp wraps the outer envelope. The votering field can be an
// array or a single object depending on result count, so we decode it as
// raw JSON and normalise below.
type voteringlistaResp struct {
	Voteringlista struct {
		Votering json.RawMessage `json:"votering"`
	} `json:"voteringlista"`
}

type rawVote struct {
	VoteringID  string `json:"votering_id"`
	RM          string `json:"rm"`
	DokID       string `json:"dok_id"`
	Beteckning  string `json:"beteckning"`
	Parti       string `json:"parti"`
	Rost        string `json:"rost"`
	Systemdatum string `json:"systemdatum"`
}

type partyCounts struct {
	Ja, Nej, Avstar, Franvarande int
}

type voteAggrKey struct {
	VoteringID string
	Party      string
}

type voteAggrMeta struct {
	RM, DokID, Beteckning, Datum string
}

// FetchRiksdagenVotes fetches and aggregates votes for the given riksmöte.
// It returns one RiksdagenVoteResult per (votering_id, party).
// maxVoterings limits how many distinct votes to process (each ~350 rows).
func (c *Client) FetchRiksdagenVotes(ctx context.Context, rm string, maxVoterings int) ([]RiksdagenVoteResult, error) {
	var resp voteringlistaResp
	rmEsc := url.QueryEscape(rm)
	u := fmt.Sprintf("%s/voteringlista/?rm=%s&sz=10000&utformat=json", riksdagenBase, rmEsc)
	if err := c.getURL(ctx, u, &resp); err != nil {
		return nil, fmt.Errorf("riksdagen votes %s: %w", rm, err)
	}

	// Normalise array-or-single-object into a slice.
	var rawVotes []rawVote
	raw := resp.Voteringlista.Votering
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &rawVotes); err != nil {
			return nil, fmt.Errorf("riksdagen votes array decode: %w", err)
		}
	} else {
		var single rawVote
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, fmt.Errorf("riksdagen votes single decode: %w", err)
		}
		rawVotes = []rawVote{single}
	}

	counts := make(map[voteAggrKey]*partyCounts)
	meta := make(map[string]*voteAggrMeta)
	var metaOrder []string // preserves insertion order

	for _, v := range rawVotes {
		if v.VoteringID == "" {
			continue
		}
		if _, seen := meta[v.VoteringID]; !seen {
			if maxVoterings > 0 && len(meta) >= maxVoterings {
				continue
			}
			datum := ""
			if len(v.Systemdatum) >= 10 {
				datum = v.Systemdatum[:10]
			}
			meta[v.VoteringID] = &voteAggrMeta{RM: v.RM, DokID: v.DokID, Beteckning: v.Beteckning, Datum: datum}
			metaOrder = append(metaOrder, v.VoteringID)
		}
		if _, ok := meta[v.VoteringID]; !ok {
			continue // over maxVoterings
		}
		key := voteAggrKey{VoteringID: v.VoteringID, Party: v.Parti}
		if counts[key] == nil {
			counts[key] = &partyCounts{}
		}
		switch v.Rost {
		case "Ja":
			counts[key].Ja++
		case "Nej":
			counts[key].Nej++
		case "Avstår":
			counts[key].Avstar++
		case "Frånvarande":
			counts[key].Franvarande++
		}
	}

	// Fetch titles for each unique dok_id (one request per votering, ~20 total).
	titles := make(map[string]string)
	for _, vid := range metaOrder {
		m := meta[vid]
		if m.DokID == "" || titles[m.DokID] != "" {
			continue
		}
		title, _ := c.fetchDokumentTitle(ctx, m.DokID)
		titles[m.DokID] = title
	}

	var out []RiksdagenVoteResult
	for key, pc := range counts {
		m := meta[key.VoteringID]
		if m == nil {
			continue
		}
		out = append(out, RiksdagenVoteResult{
			VoteringID:  key.VoteringID,
			RM:          m.RM,
			DokID:       m.DokID,
			Beteckning:  m.Beteckning,
			Title:       titles[m.DokID],
			Datum:       m.Datum,
			Party:       key.Party,
			Ja:          pc.Ja,
			Nej:         pc.Nej,
			Avstar:      pc.Avstar,
			Franvarande: pc.Franvarande,
		})
	}
	return out, nil
}

// fetchDokumentTitle fetches the title for a given dok_id.
// Errors are silently swallowed — a missing title degrades to empty string.
func (c *Client) fetchDokumentTitle(ctx context.Context, dokID string) (string, error) {
	u := fmt.Sprintf("%s/dokument/%s.json", riksdagenBase, url.PathEscape(dokID))
	var resp struct {
		Dokumentstatus struct {
			Dokument struct {
				Titel string `json:"titel"`
			} `json:"dokument"`
		} `json:"dokumentstatus"`
	}
	if err := c.getURL(ctx, u, &resp); err != nil {
		return "", err
	}
	return resp.Dokumentstatus.Dokument.Titel, nil
}
