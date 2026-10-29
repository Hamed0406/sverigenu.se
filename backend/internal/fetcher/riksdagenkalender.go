package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// RiksdagenEvent is one upcoming calendar entry (committee meeting, debate,
// vote, hearing) from the riksdagen dokumentlista kalender feed.
type RiksdagenEvent struct {
	DokID     string
	Datum     string // "YYYY-MM-DD HH:MM:SS" start time
	Typ       string // utskottsmöte, sammanträde, debatt, ...
	Subtyp    string
	Titel     string
	Undertitel string
	Organ     string
	Plats     string
}

type kalenderResp struct {
	Dokumentlista struct {
		Dokument json.RawMessage `json:"dokument"`
	} `json:"dokumentlista"`
}

type kalenderDoc struct {
	DokID      string `json:"dok_id"`
	Datum      string `json:"datum"`
	Typ        string `json:"typ"`
	Subtyp     string `json:"subtyp"`
	Titel      string `json:"titel"`
	Undertitel string `json:"undertitel"`
	Organ      string `json:"organ"`
	Plats      string `json:"plats"`
}

// FetchRiksdagenKalender returns upcoming riksdag events for the next four
// weeks. Public guided tours (typ=kalakt) are excluded — they are visitor
// activities, not parliamentary work.
func (c *Client) FetchRiksdagenKalender(ctx context.Context) ([]RiksdagenEvent, error) {
	now := time.Now()
	q := url.Values{}
	q.Set("avd", "kalender")
	q.Set("from", now.Format("2006-01-02"))
	q.Set("tom", now.AddDate(0, 0, 28).Format("2006-01-02"))
	q.Set("sortorder", "asc")
	q.Set("sort", "datum")
	q.Set("utformat", "json")
	q.Set("sz", "200")

	var resp kalenderResp
	if err := c.getURL(ctx, riksdagenBase+"/dokumentlista/?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("riksdagen kalender: %w", err)
	}

	// dokument is an array normally but a bare object on single-hit responses.
	var docs []kalenderDoc
	if len(resp.Dokumentlista.Dokument) > 0 {
		if err := json.Unmarshal(resp.Dokumentlista.Dokument, &docs); err != nil {
			var one kalenderDoc
			if err2 := json.Unmarshal(resp.Dokumentlista.Dokument, &one); err2 != nil {
				return nil, fmt.Errorf("riksdagen kalender: decode dokument: %w", err)
			}
			docs = []kalenderDoc{one}
		}
	}

	var out []RiksdagenEvent
	for _, d := range docs {
		if d.DokID == "" || d.Typ == "kalakt" {
			continue
		}
		out = append(out, RiksdagenEvent{
			DokID:      d.DokID,
			Datum:      d.Datum,
			Typ:        d.Typ,
			Subtyp:     d.Subtyp,
			Titel:      d.Titel,
			Undertitel: d.Undertitel,
			Organ:      d.Organ,
			Plats:      d.Plats,
		})
	}
	return out, nil
}
