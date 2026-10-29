package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"swedennu/internal/geo"
)

// response is the standard envelope for every list endpoint.
// All API responses follow the shape: {"data": [...], "count": N}
type response struct {
	Data  any `json:"data"`
	Count int `json:"count"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// dbErr logs the real error server-side and returns a generic 500 to the client.
// This prevents internal DB error details (table names, SQL state codes) from
// leaking to callers while still making the root cause visible in logs.
func dbErr(w http.ResponseWriter, handler string, err error) {
	slog.Error("db error", "handler", handler, "err", err)
	writeErr(w, 500, "internal error")
}

// DB is the minimal interface the HTTP handlers require from the database.
// *pgxpool.Pool satisfies this in production.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// handlers holds shared dependencies for all request handlers.
type handlers struct {
	db      DB
	baseURL string // canonical base URL from BASE_URL env var; empty = derived from request
}

// siteBase returns the canonical base URL for use in sitemap/robots.
// If configured via BASE_URL, that value wins. Otherwise the request Host header is used,
// validated to be non-empty.
func (h *handlers) siteBase(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	return proto + "://" + host
}

// ---- Weather -----------------------------------------------------------------

// weather returns the latest air temperature observation per SMHI station.
// Optional ?county=ab filters to one county.
func (h *handlers) weather(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	rows, err := h.db.Query(r.Context(), `
		SELECT station_key, name, COALESCE(county_code,''), lat, lon, height_m,
		       temperature, COALESCE(quality,''), measured_at, synced_at
		FROM smhi_observations
		WHERE parameter = 1 AND ($1 = '' OR county_code = $1)
		ORDER BY name`, county)
	if err != nil {
		dbErr(w, "weather", err)
		return
	}
	defer rows.Close()

	type item struct {
		StationKey  string     `json:"station_key"`
		Name        string     `json:"name"`
		County      string     `json:"county"`
		Lat         float64    `json:"lat"`
		Lon         float64    `json:"lon"`
		HeightM     *float64   `json:"height_m"`
		Temperature *float64   `json:"temperature"`
		Quality     string     `json:"quality,omitempty"`
		MeasuredAt  *time.Time `json:"measured_at"`
		SyncedAt    time.Time  `json:"synced_at"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.StationKey, &it.Name, &it.County, &it.Lat, &it.Lon,
			&it.HeightM, &it.Temperature, &it.Quality, &it.MeasuredAt, &it.SyncedAt); err != nil {
			slog.Error("scan error", "handler", "weather", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "weather", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Police events -------------------------------------------------------------

// policeEvents returns recent national police events, newest first.
// Optional filters: ?county=ab and ?type=Trafikolycka.
func (h *handlers) policeEvents(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	eventType := r.URL.Query().Get("type")
	rows, err := h.db.Query(r.Context(), `
		SELECT external_id, event_time, COALESCE(name,''), COALESCE(summary,''),
		       COALESCE(url,''), COALESCE(type,''), COALESCE(location_name,''),
		       COALESCE(county_code,''), lat, lon, synced_at
		FROM national_police_events
		WHERE ($1 = '' OR county_code = $1)
		  AND ($2 = '' OR type = $2)
		ORDER BY event_time DESC NULLS LAST
		LIMIT 500`, county, eventType)
	if err != nil {
		dbErr(w, "policeevents", err)
		return
	}
	defer rows.Close()

	type item struct {
		ID        int64      `json:"id"`
		EventTime *time.Time `json:"event_time"`
		Name      string     `json:"name"`
		Summary   string     `json:"summary"`
		URL       string     `json:"url"`
		Type      string     `json:"type"`
		Location  string     `json:"location"`
		County    string     `json:"county"`
		Lat       *float64   `json:"lat"`
		Lon       *float64   `json:"lon"`
		SyncedAt  time.Time  `json:"synced_at"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.EventTime, &it.Name, &it.Summary, &it.URL,
			&it.Type, &it.Location, &it.County, &it.Lat, &it.Lon, &it.SyncedAt); err != nil {
			slog.Error("scan error", "handler", "policeevents", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "policeevents", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- SCB: befolkning -------------------------------------------------------------

// befolkning returns the county population time series from SCB.
// Filters: ?county=ab, ?from=2000.
func (h *handlers) befolkning(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	from, _ := strconv.Atoi(r.URL.Query().Get("from"))
	rows, err := h.db.Query(r.Context(), `
		SELECT county_code, region_code, year, value::float8
		FROM scb_stats
		WHERE table_id = 'befolkning' AND metric = 'BE0101N1' AND value IS NOT NULL
		  AND ($1 = '' OR county_code = $1)
		  AND ($2 = 0  OR year >= $2)
		ORDER BY county_code, year`, county, from)
	if err != nil {
		dbErr(w, "befolkning", err)
		return
	}
	defer rows.Close()

	type item struct {
		County     string  `json:"county"`
		RegionCode string  `json:"region_code"`
		Year       int     `json:"year"`
		Population float64 `json:"population"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.RegionCode, &it.Year, &it.Population); err != nil {
			slog.Error("scan error", "handler", "befolkning", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "befolkning", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- SCB: inkomst ----------------------------------------------------------------

// inkomst returns mean/median pre-tax income (tkr, ages 20+) per county per year.
// Filter: ?county=ab.
func (h *handlers) inkomst(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	rows, err := h.db.Query(r.Context(), `
		SELECT county_code, region_code, year, metric, value::float8, unit
		FROM scb_stats
		WHERE table_id = 'inkomst' AND value IS NOT NULL
		  AND ($1 = '' OR county_code = $1)
		ORDER BY county_code, year, metric`, county)
	if err != nil {
		dbErr(w, "inkomst", err)
		return
	}
	defer rows.Close()

	type item struct {
		County     string  `json:"county"`
		RegionCode string  `json:"region_code"`
		Year       int     `json:"year"`
		Metric     string  `json:"metric"` // "mean" | "median"
		Value      float64 `json:"value"`
		Unit       string  `json:"unit"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.RegionCode, &it.Year, &it.Metric, &it.Value, &it.Unit); err != nil {
			slog.Error("scan error", "handler", "inkomst", "err", err)
			continue
		}
		switch it.Metric {
		case "HE0110K1":
			it.Metric = "mean"
		case "HE0110K2":
			it.Metric = "median"
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "inkomst", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// brp returns Bruttoregionprodukt (regional GDP) per county from SCB NR0105.
// Filters: ?county=ab (single county time series), ?from=2010 (start year).
// Metrics: NR0105AJ = BRP per invånare (tkr/inv), NR0105AI = volymutveckling (%).
func (h *handlers) brp(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	from := r.URL.Query().Get("from")
	if from == "" {
		from = "2000"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT county_code, region_code, year, metric, value::float8, unit
		FROM scb_stats
		WHERE table_id = 'brp' AND value IS NOT NULL
		  AND ($1 = '' OR county_code = $1)
		  AND year >= $2::int
		ORDER BY county_code, year, metric`, county, from)
	if err != nil {
		dbErr(w, "brp", err)
		return
	}
	defer rows.Close()

	type item struct {
		County     string  `json:"county"`
		RegionCode string  `json:"region_code"`
		Year       int     `json:"year"`
		Metric     string  `json:"metric"`
		Value      float64 `json:"value"`
		Unit       string  `json:"unit"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.RegionCode, &it.Year, &it.Metric, &it.Value, &it.Unit); err != nil {
			slog.Error("scan error", "handler", "brp", "err", err)
			continue
		}
		switch it.Metric {
		case "NR0105AJ":
			it.Metric = "per_capita"
		case "NR0105AI":
			it.Metric = "growth_pct"
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "brp", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Vindkraft ------------------------------------------------------------------

func (h *handlers) vindkraft(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	from := r.URL.Query().Get("from")
	if from == "" {
		from = "2003"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT county_code, region_code, year, metric, value::float8, unit
		FROM scb_stats
		WHERE table_id = 'vindkraft' AND value IS NOT NULL
		  AND ($1 = '' OR county_code = $1)
		  AND year >= $2::int
		ORDER BY county_code, year, metric`, county, from)
	if err != nil {
		dbErr(w, "vindkraft", err)
		return
	}
	defer rows.Close()

	type item struct {
		County     string  `json:"county"`
		RegionCode string  `json:"region_code"`
		Year       int     `json:"year"`
		Metric     string  `json:"metric"`
		Value      float64 `json:"value"`
		Unit       string  `json:"unit"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.RegionCode, &it.Year, &it.Metric, &it.Value, &it.Unit); err != nil {
			slog.Error("scan error", "handler", "vindkraft", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "vindkraft", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

func (h *handlers) solceller(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	rows, err := h.db.Query(r.Context(), `
		SELECT county_code, region_code, year, metric, value::float8, unit
		FROM scb_stats
		WHERE table_id = 'solceller' AND value IS NOT NULL
		  AND ($1 = '' OR county_code = $1)
		ORDER BY county_code, year, metric`, county)
	if err != nil {
		dbErr(w, "solceller", err)
		return
	}
	defer rows.Close()
	type item struct {
		County     string  `json:"county"`
		RegionCode string  `json:"region_code"`
		Year       int     `json:"year"`
		Metric     string  `json:"metric"`
		Value      float64 `json:"value"`
		Unit       string  `json:"unit"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.RegionCode, &it.Year, &it.Metric, &it.Value, &it.Unit); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

func (h *handlers) skattesats(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	year := r.URL.Query().Get("year")
	if year == "" {
		year = "2025"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT year, municipality, lan_kod, county_code,
		       kommunal_skatt::float8, landstings_skatt::float8,
		       begravnings_avgift::float8, summa_exkl_kyrka::float8
		FROM skattesats
		WHERE ($1 = '' OR county_code = $1)
		  AND ($2 = 'all' OR year = $2::int)
		ORDER BY year DESC, kommunal_skatt DESC`, county, year)
	if err != nil {
		dbErr(w, "skattesats", err)
		return
	}
	defer rows.Close()
	type item struct {
		Year              int     `json:"year"`
		Municipality      string  `json:"municipality"`
		LanKod            string  `json:"lan_kod"`
		County            string  `json:"county"`
		KommunalSkatt     float64 `json:"kommunal_skatt"`
		LandstingsSkatt   float64 `json:"landstings_skatt"`
		BegravningsAvgift float64 `json:"begravnings_avgift"`
		SummaExklKyrka    float64 `json:"summa_exkl_kyrka"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Municipality, &it.LanKod, &it.County,
			&it.KommunalSkatt, &it.LandstingsSkatt, &it.BegravningsAvgift, &it.SummaExklKyrka); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

func (h *handlers) energimix(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	if from == "" {
		from = "1990"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT year, source, twh::float8
		FROM energimix_stats
		WHERE twh IS NOT NULL AND year >= $1::int
		ORDER BY year, source`, from)
	if err != nil {
		dbErr(w, "energimix", err)
		return
	}
	defer rows.Close()
	type item struct {
		Year   int     `json:"year"`
		Source string  `json:"source"`
		TWh    float64 `json:"twh"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Source, &it.TWh); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Kolada ----------------------------------------------------------------------

// kolada returns the latest period per (kpi, municipality).
// Filters: ?county=ab, ?kpi=N03920.
func (h *handlers) kolada(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	kpi := r.URL.Query().Get("kpi")
	rows, err := h.db.Query(r.Context(), `
		SELECT DISTINCT ON (kpi_id, municipality_id)
		       kpi_id, municipality_id, municipality, county_code, period, title, unit, value::float8
		FROM kolada_stats
		WHERE value IS NOT NULL
		  AND ($1 = '' OR county_code = $1)
		  AND ($2 = '' OR kpi_id = $2)
		ORDER BY kpi_id, municipality_id, period DESC`, county, kpi)
	if err != nil {
		dbErr(w, "kolada", err)
		return
	}
	defer rows.Close()

	type item struct {
		KPI          string  `json:"kpi"`
		Municipality string  `json:"municipality"`
		Name         string  `json:"name"`
		County       string  `json:"county"`
		Period       int     `json:"period"`
		Title        string  `json:"title"`
		Unit         string  `json:"unit"`
		Value        float64 `json:"value"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.KPI, &it.Municipality, &it.Name, &it.County, &it.Period, &it.Title, &it.Unit, &it.Value); err != nil {
			slog.Error("scan error", "handler", "kolada", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "kolada", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- County summary ------------------------------------------------------------

// lanSummary returns one row per county (always exactly 21) with the latest
// weather average and recent police event counts. The static geo.Counties
// slice drives the output so counties with no data still appear with nulls.
//
// Note: polisen.se returns only the ~500 most recent events per fetch, so the
// 7-day count undercounts until the database has accumulated a week of syncs.
func (h *handlers) lanSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type tempAgg struct {
		avg      *float64
		stations int
	}
	temps := make(map[string]tempAgg, 21)
	// Only readings from the last 3 hours so offline stations don't skew averages.
	rows, err := h.db.Query(ctx, `
		SELECT county_code, ROUND(AVG(temperature), 1)::float8, COUNT(*)
		FROM smhi_observations
		WHERE parameter = 1 AND temperature IS NOT NULL
		  AND county_code IS NOT NULL
		  AND measured_at > NOW() - INTERVAL '3 hours'
		GROUP BY county_code`)
	if err != nil {
		dbErr(w, "lansummary", err)
		return
	}
	for rows.Next() {
		var code string
		var agg tempAgg
		if err := rows.Scan(&code, &agg.avg, &agg.stations); err != nil {
			slog.Error("scan error", "handler", "lansummary", "err", err)
			continue
		}
		temps[code] = agg
	}
	rows.Close()

	type eventAgg struct {
		count  int
		latest *time.Time
	}
	events := make(map[string]eventAgg, 21)
	rows, err = h.db.Query(ctx, `
		SELECT county_code, COUNT(*), MAX(event_time)
		FROM national_police_events
		WHERE county_code IS NOT NULL
		  AND event_time > NOW() - INTERVAL '7 days'
		GROUP BY county_code`)
	if err != nil {
		dbErr(w, "lansummary", err)
		return
	}
	for rows.Next() {
		var code string
		var agg eventAgg
		if err := rows.Scan(&code, &agg.count, &agg.latest); err != nil {
			slog.Error("scan error", "handler", "lansummary", "err", err)
			continue
		}
		events[code] = agg
	}
	rows.Close()

	type popAgg struct {
		population int64
		year       int
	}
	pops := make(map[string]popAgg, 21)
	rows, err = h.db.Query(ctx, `
		SELECT DISTINCT ON (county_code) county_code, value::bigint, year
		FROM scb_stats
		WHERE table_id = 'befolkning' AND metric = 'BE0101N1' AND value IS NOT NULL
		ORDER BY county_code, year DESC`)
	if err != nil {
		dbErr(w, "lansummary", err)
		return
	}
	for rows.Next() {
		var code string
		var agg popAgg
		if err := rows.Scan(&code, &agg.population, &agg.year); err != nil {
			slog.Error("scan error", "handler", "lansummary", "err", err)
			continue
		}
		pops[code] = agg
	}
	rows.Close()

	type item struct {
		Code            string     `json:"code"`
		Name            string     `json:"name"`
		AvgTempC        *float64   `json:"avg_temp_c"`
		StationCount    int        `json:"station_count"`
		PoliceEvents7d  int        `json:"police_events_7d"`
		LatestEventTime *time.Time `json:"latest_event_time"`
		Population      *int64     `json:"population"`
		PopulationYear  int        `json:"population_year,omitempty"`
	}
	result := make([]item, 0, len(geo.Counties))
	for _, c := range geo.Counties {
		it := item{Code: c.Code, Name: c.Name}
		if t, ok := temps[c.Code]; ok {
			it.AvgTempC = t.avg
			it.StationCount = t.stations
		}
		if e, ok := events[c.Code]; ok {
			it.PoliceEvents7d = e.count
			it.LatestEventTime = e.latest
		}
		if p, ok := pops[c.Code]; ok {
			pop := p.population
			it.Population = &pop
			it.PopulationYear = p.year
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Riksdagen -------------------------------------------------------------------

// riksdagenMembers returns active MPs. Optional ?party=s filter.
func (h *handlers) riksdagenMembers(w http.ResponseWriter, r *http.Request) {
	party := r.URL.Query().Get("party")
	rows, err := h.db.Query(r.Context(), `
		SELECT id, first_name, last_name, party, valkrets, gender, birth_year, photo_url
		FROM riksdagen_members
		WHERE ($1 = '' OR lower(party) = lower($1))
		ORDER BY party, last_name, first_name`, party)
	if err != nil {
		dbErr(w, "riksdagenMembers", err)
		return
	}
	defer rows.Close()

	type item struct {
		ID        string  `json:"id"`
		FirstName string  `json:"first_name"`
		LastName  string  `json:"last_name"`
		Party     string  `json:"party"`
		Valkrets  string  `json:"valkrets"`
		Gender    string  `json:"gender"`
		BirthYear *int    `json:"birth_year,omitempty"`
		PhotoURL  *string `json:"photo_url,omitempty"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.FirstName, &it.LastName, &it.Party, &it.Valkrets, &it.Gender, &it.BirthYear, &it.PhotoURL); err != nil {
			slog.Error("scan error", "handler", "riksdagenMembers", "err", err)
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// riksdagenSeats returns seat counts per party.
func (h *handlers) riksdagenSeats(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `
		SELECT party, COUNT(*) AS seats,
		       COUNT(*) FILTER (WHERE gender = 'kvinna') AS women,
		       COUNT(*) FILTER (WHERE gender = 'man')    AS men
		FROM riksdagen_members
		GROUP BY party
		ORDER BY seats DESC`)
	if err != nil {
		dbErr(w, "riksdagenSeats", err)
		return
	}
	defer rows.Close()

	type item struct {
		Party string `json:"party"`
		Seats int    `json:"seats"`
		Women int    `json:"women"`
		Men   int    `json:"men"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Party, &it.Seats, &it.Women, &it.Men); err != nil {
			slog.Error("scan error", "handler", "riksdagenSeats", "err", err)
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// riksdagenVotes returns recent vote aggregates, grouped by votering_id.
// Optional ?rm=2025/26 filter. Default limit 30 most recent voterings.
func (h *handlers) riksdagenVotes(w http.ResponseWriter, r *http.Request) {
	rm := r.URL.Query().Get("rm")
	rows, err := h.db.Query(r.Context(), `
		SELECT votering_id, rm, dok_id, beteckning, title, datum,
		       party, ja, nej, avstar, franvarande
		FROM riksdagen_vote_results
		WHERE ($1 = '' OR rm = $1)
		  AND votering_id IN (
		      SELECT votering_id FROM riksdagen_vote_results
		      WHERE ($1 = '' OR rm = $1)
		      GROUP BY votering_id, datum
		      ORDER BY MAX(datum) DESC NULLS LAST
		      LIMIT 30
		  )
		ORDER BY datum DESC NULLS LAST, votering_id, party`, rm)
	if err != nil {
		dbErr(w, "riksdagenVotes", err)
		return
	}
	defer rows.Close()

	type partyRow struct {
		Party       string `json:"party"`
		Ja          int    `json:"ja"`
		Nej         int    `json:"nej"`
		Avstar      int    `json:"avstar"`
		Franvarande int    `json:"franvarande"`
	}
	type voteGroup struct {
		VoteringID string     `json:"votering_id"`
		RM         string     `json:"rm"`
		DokID      string     `json:"dok_id"`
		Beteckning string     `json:"beteckning"`
		Title      string     `json:"title"`
		Datum      *string    `json:"datum"`
		Parties    []partyRow `json:"parties"`
	}

	groups := make(map[string]*voteGroup)
	var order []string
	for rows.Next() {
		var vid, rm2, dokID, beteckning, title, party string
		var datumT *time.Time
		var ja, nej, avstar, franvarande int
		if err := rows.Scan(&vid, &rm2, &dokID, &beteckning, &title, &datumT, &party, &ja, &nej, &avstar, &franvarande); err != nil {
			slog.Error("scan error", "handler", "riksdagenVotes", "err", err)
			continue
		}
		var datum *string
		if datumT != nil {
			s := datumT.Format("2006-01-02")
			datum = &s
		}
		if _, ok := groups[vid]; !ok {
			groups[vid] = &voteGroup{VoteringID: vid, RM: rm2, DokID: dokID, Beteckning: beteckning, Title: title, Datum: datum}
			order = append(order, vid)
		}
		groups[vid].Parties = append(groups[vid].Parties, partyRow{Party: party, Ja: ja, Nej: nej, Avstar: avstar, Franvarande: franvarande})
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "riksdagenVotes", "err", err)
	}

	result := make([]voteGroup, 0, len(order))
	for _, vid := range order {
		result = append(result, *groups[vid])
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Domstolsverket -------------------------------------------------------------

// domstolTrends returns yearly case counts per court type.
// Optional ?courttype= filter (e.g. "District Courts").
func (h *handlers) domstolTrends(w http.ResponseWriter, r *http.Request) {
	ct := r.URL.Query().Get("courttype")
	rows, err := h.db.Query(r.Context(), `
		SELECT court_code, court_name, metric, year, value::float8
		FROM domstol_stats
		WHERE table_id = 'courttype' AND value IS NOT NULL
		  AND ($1 = '' OR court_code = $1)
		ORDER BY court_code, metric, year`, ct)
	if err != nil {
		dbErr(w, "domstolTrends", err)
		return
	}
	defer rows.Close()

	type item struct {
		CourtCode string  `json:"court_code"`
		CourtName string  `json:"court_name"`
		Metric    string  `json:"metric"`
		Year      int     `json:"year"`
		Value     float64 `json:"value"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.CourtCode, &it.CourtName, &it.Metric, &it.Year, &it.Value); err != nil {
			slog.Error("scan error", "handler", "domstolTrends", "err", err)
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// domstolProcessing returns processing times (75th percentile, months) per tingsrätt.
// Optional ?year= and ?metric= (proc_crim | proc_civil) filters.
func (h *handlers) domstolProcessing(w http.ResponseWriter, r *http.Request) {
	yearStr := r.URL.Query().Get("year")
	year, _ := strconv.Atoi(yearStr)
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "proc_crim"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT court_code, court_name, county_code, metric, year, value::float8
		FROM domstol_stats
		WHERE table_id = 'processing_tr' AND value IS NOT NULL
		  AND metric = $1
		  AND ($2 = 0 OR year = $2)
		ORDER BY year DESC, value DESC`, metric, year)
	if err != nil {
		dbErr(w, "domstolProcessing", err)
		return
	}
	defer rows.Close()

	type item struct {
		CourtCode  string  `json:"court_code"`
		CourtName  string  `json:"court_name"`
		CountyCode string  `json:"county_code"`
		Metric     string  `json:"metric"`
		Year       int     `json:"year"`
		Value      float64 `json:"value"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.CourtCode, &it.CourtName, &it.CountyCode, &it.Metric, &it.Year, &it.Value); err != nil {
			slog.Error("scan error", "handler", "domstolProcessing", "err", err)
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Folkhälsomyndigheten -------------------------------------------------------

// folkhalsa returns public-health indicators from FOHM.
// Filters: ?metric=lifeexp|smoking|alcohol, ?county=ab, ?sex=total|women|men.
func (h *handlers) folkhalsa(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	county := r.URL.Query().Get("county")
	sex := r.URL.Query().Get("sex")
	rows, err := h.db.Query(r.Context(), `
		SELECT metric, region_code, county_code, sex, period, value::float8, unit
		FROM folkhalsa_stats
		WHERE ($1 = '' OR metric     = $1)
		  AND ($2 = '' OR county_code = $2)
		  AND ($3 = '' OR sex        = $3)
		ORDER BY metric, county_code, sex, period`, metric, county, sex)
	if err != nil {
		dbErr(w, "folkhalsa", err)
		return
	}
	defer rows.Close()

	type item struct {
		Metric  string  `json:"metric"`
		Region  string  `json:"region_code"`
		County  string  `json:"county"`
		Sex     string  `json:"sex"`
		Period  string  `json:"period"`
		Value   float64 `json:"value"`
		Unit    string  `json:"unit"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Metric, &it.Region, &it.County, &it.Sex, &it.Period, &it.Value, &it.Unit); err != nil {
			slog.Error("scan error", "handler", "folkhalsa", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "folkhalsa", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Val (Valmyndigheten) --------------------------------------------------------

// valNational returns national-level party results for all years.
// Optional ?party= filter, ?area= to select area_code (default "riket").
func (h *handlers) valNational(w http.ResponseWriter, r *http.Request) {
	party := r.URL.Query().Get("party")
	area := r.URL.Query().Get("area")
	if area == "" {
		area = "riket"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT election_year, party_code, party_name, votes, vote_share::float8, mandates
		FROM val_results
		WHERE area_code = $1
		  AND ($2 = '' OR party_code = $2)
		ORDER BY election_year, party_code`, area, party)
	if err != nil {
		dbErr(w, "valNational", err)
		return
	}
	defer rows.Close()

	type item struct {
		Year      int     `json:"year"`
		Party     string  `json:"party"`
		PartyName string  `json:"party_name"`
		Votes     *int    `json:"votes"`
		Share     float64 `json:"vote_share"`
		Mandates  *int    `json:"mandates"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Party, &it.PartyName, &it.Votes, &it.Share, &it.Mandates); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// valTurnout returns turnout data.
// Optional ?area= to filter to one area.
func (h *handlers) valTurnout(w http.ResponseWriter, r *http.Request) {
	area := r.URL.Query().Get("area")
	rows, err := h.db.Query(r.Context(), `
		SELECT election_year, area_code, area_name, eligible_voters, votes_cast, turnout::float8
		FROM val_turnout
		WHERE ($1 = '' OR area_code = $1)
		ORDER BY area_code, election_year`, area)
	if err != nil {
		dbErr(w, "valTurnout", err)
		return
	}
	defer rows.Close()

	type item struct {
		Year     int     `json:"year"`
		Area     string  `json:"area"`
		AreaName string  `json:"area_name"`
		Eligible *int    `json:"eligible_voters"`
		Cast     *int    `json:"votes_cast"`
		Turnout  float64 `json:"turnout"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Area, &it.AreaName, &it.Eligible, &it.Cast, &it.Turnout); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// valValkrets returns 2022 results per valkrets. Optional ?party= filter.
func (h *handlers) valValkrets(w http.ResponseWriter, r *http.Request) {
	party := r.URL.Query().Get("party")
	rows, err := h.db.Query(r.Context(), `
		SELECT election_year, area_code, area_name, party_code, votes, vote_share::float8
		FROM val_results
		WHERE election_year = 2022
		  AND area_code <> 'riket'
		  AND ($1 = '' OR party_code = $1)
		ORDER BY area_name, party_code`, party)
	if err != nil {
		dbErr(w, "valValkrets", err)
		return
	}
	defer rows.Close()

	type item struct {
		Year     int     `json:"year"`
		Area     string  `json:"area"`
		AreaName string  `json:"area_name"`
		Party    string  `json:"party"`
		Votes    *int    `json:"votes"`
		Share    float64 `json:"vote_share"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Area, &it.AreaName, &it.Party, &it.Votes, &it.Share); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Luftkvalitet (SMHI air quality) --------------------------------------------

// airQuality returns the latest-hour reading per station and parameter.
// Optional ?county=ab filters by county code.
// Optional ?parameter=2 filters by parameter ID (1=PM10, 2=PM2.5, 5=NO2, 8=O3).
func (h *handlers) airQuality(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	param := r.URL.Query().Get("parameter")
	rows, err := h.db.Query(r.Context(), `
		SELECT station_key, parameter, param_name, name, COALESCE(county_code,''),
		       lat, lon, height_m, value, COALESCE(quality,''), measured_at, synced_at
		FROM air_quality
		WHERE ($1 = '' OR county_code = $1)
		  AND ($2 = '' OR parameter = $2::INT)
		ORDER BY parameter, value DESC NULLS LAST`, county, param)
	if err != nil {
		dbErr(w, "airQuality", err)
		return
	}
	defer rows.Close()

	type item struct {
		StationKey string     `json:"station_key"`
		Parameter  int        `json:"parameter"`
		ParamName  string     `json:"param_name"`
		Name       string     `json:"name"`
		County     string     `json:"county"`
		Lat        float64    `json:"lat"`
		Lon        float64    `json:"lon"`
		HeightM    *float64   `json:"height_m"`
		Value      *float64   `json:"value"`
		Quality    string     `json:"quality,omitempty"`
		MeasuredAt *time.Time `json:"measured_at"`
		SyncedAt   time.Time  `json:"synced_at"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.StationKey, &it.Parameter, &it.ParamName, &it.Name, &it.County,
			&it.Lat, &it.Lon, &it.HeightM, &it.Value, &it.Quality, &it.MeasuredAt, &it.SyncedAt); err != nil {
			slog.Error("scan error", "handler", "airQuality", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "airQuality", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- BRÅ (anmälda brott) -----------------------------------------------------------

// braCrimes returns kommun-level reported-crime statistics from BRÅ.
// Filters: ?county=ab, ?category=misshandel (slug), ?year=2024, ?from=2010.
func (h *handlers) braCrimes(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	category := r.URL.Query().Get("category")
	year := r.URL.Query().Get("year")
	from := r.URL.Query().Get("from")
	rows, err := h.db.Query(r.Context(), `
		SELECT year, slug, category, kommun, county_code, count, per_100k::float8
		FROM bra_crimes
		WHERE ($1 = '' OR county_code = $1)
		  AND ($2 = '' OR slug = $2)
		  AND ($3 = '' OR year = $3::INT)
		  AND ($4 = '' OR year >= $4::INT)
		ORDER BY slug, year, kommun`, county, category, year, from)
	if err != nil {
		dbErr(w, "braCrimes", err)
		return
	}
	defer rows.Close()

	type item struct {
		Year     int      `json:"year"`
		Slug     string   `json:"category"`
		Category string   `json:"category_name"`
		Kommun   string   `json:"kommun"`
		County   string   `json:"county"`
		Count    *int     `json:"count"`
		Per100k  *float64 `json:"per_100k"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Slug, &it.Category, &it.Kommun, &it.County, &it.Count, &it.Per100k); err != nil {
			slog.Error("scan error", "handler", "braCrimes", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "braCrimes", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// braLan returns county-level aggregates of BRÅ reported crimes: the kommun
// counts summed per county and year, with a per-100k rate computed against
// the SCB county population (latest population year at or before the crime
// year, so years beyond the SCB series still get a rate).
// Filters: ?category=totalt (slug; empty = all categories), ?year=, ?from=.
func (h *handlers) braLan(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	year := r.URL.Query().Get("year")
	from := r.URL.Query().Get("from")
	rows, err := h.db.Query(r.Context(), `
		SELECT b.county_code, b.year, b.slug, MAX(b.category) AS category,
		       SUM(b.count) AS total,
		       MAX(p.value) AS population
		FROM bra_crimes b
		LEFT JOIN LATERAL (
			SELECT value FROM scb_stats p
			WHERE p.table_id = 'befolkning' AND p.metric = 'BE0101N1'
			  AND p.county_code = b.county_code AND p.year <= b.year
			  AND p.value IS NOT NULL
			ORDER BY p.year DESC LIMIT 1
		) p ON TRUE
		WHERE ($1 = '' OR b.slug = $1)
		  AND ($2 = '' OR b.year = $2::INT)
		  AND ($3 = '' OR b.year >= $3::INT)
		  AND b.count IS NOT NULL
		GROUP BY b.county_code, b.year, b.slug
		ORDER BY b.year, b.county_code`, category, year, from)
	if err != nil {
		dbErr(w, "braLan", err)
		return
	}
	defer rows.Close()

	type item struct {
		County     string   `json:"county"`
		Year       int      `json:"year"`
		Slug       string   `json:"category"`
		Category   string   `json:"category_name"`
		Total      int      `json:"count"`
		Population *float64 `json:"population"`
		Per100k    *float64 `json:"per_100k"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.Year, &it.Slug, &it.Category, &it.Total, &it.Population); err != nil {
			slog.Error("scan error", "handler", "braLan", "err", err)
			continue
		}
		if it.Population != nil && *it.Population > 0 {
			rate := float64(it.Total) / *it.Population * 100000
			it.Per100k = &rate
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "braLan", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ---- Sync status -----------------------------------------------------------------

func (h *handlers) syncStatus(w http.ResponseWriter, r *http.Request) {
	// DISTINCT ON (category) is a Postgres extension that returns the single most
	// recent row per category without a subquery. ORDER BY must start with the
	// DISTINCT ON key followed by the tie-breaker column.
	rows, err := h.db.Query(r.Context(), `
		SELECT DISTINCT ON (category) category, source, upserted, started_at, finished_at, error_msg
		FROM sync_log
		ORDER BY category, started_at DESC`)
	if err != nil {
		dbErr(w, "syncstatus", err)
		return
	}
	defer rows.Close()

	type entry struct {
		Category   string     `json:"category"`
		Source     string     `json:"source"`
		Upserted   int        `json:"upserted"`
		StartedAt  time.Time  `json:"started_at"`
		FinishedAt *time.Time `json:"finished_at"`
		Error      *string    `json:"error,omitempty"`
	}
	var result []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.Category, &e.Source, &e.Upserted, &e.StartedAt, &e.FinishedAt, &e.Error); err != nil {
			slog.Error("scan error", "handler", "syncstatus", "err", err)
			continue
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "syncstatus", "err", err)
	}
	writeJSON(w, 200, map[string]any{"syncs": result})
}

// ---- Sitemap + robots.txt --------------------------------------------------------

// sitemap returns a machine-readable XML sitemap of every public URL on the site.
// The base URL is built from the incoming Host header + X-Forwarded-Proto so the
// sitemap stays correct on any domain without needing an env variable.
// lastmod is taken from the most recent successful sync so Google knows when data
// changed without having to crawl each page on every visit.
func (h *handlers) sitemap(w http.ResponseWriter, r *http.Request) {
	base := h.siteBase(r)

	// Latest successful sync date for lastmod.
	lastMod := time.Now().Format("2006-01-02")
	rows, err := h.db.Query(r.Context(),
		`SELECT MAX(finished_at) FROM sync_log WHERE error_msg IS NULL`)
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			var t *time.Time
			if rows.Scan(&t) == nil && t != nil {
				lastMod = t.Format("2006-01-02")
			}
		}
	}

	type entry struct {
		loc, freq, pri string
	}
	pages := []entry{
		{base + "/", "daily", "1.0"},
		{base + "/vader", "hourly", "0.9"},
		{base + "/polis", "hourly", "0.9"},
		{base + "/befolkning", "monthly", "0.8"},
		{base + "/inkomst", "monthly", "0.8"},
		{base + "/brott", "weekly", "0.8"},
		{base + "/riksdagen", "daily", "0.8"},
		{base + "/halsa", "monthly", "0.7"},
		{base + "/domstol", "monthly", "0.7"},
		{base + "/val", "yearly", "0.7"},
		{base + "/luftkvalitet", "hourly", "0.6"},
		{base + "/trafik", "hourly", "0.9"},
		{base + "/utbildning", "daily", "0.8"},
		{base + "/ekonomi", "daily", "0.7"},
		{base + "/arbetsmarknad", "daily", "0.7"},
		{base + "/om", "monthly", "0.5"},
	}
	counties := []string{"ab", "c", "d", "e", "f", "g", "h", "i", "k", "m",
		"n", "o", "s", "t", "u", "w", "x", "y", "z", "ac", "bd"}
	for _, c := range counties {
		pages = append(pages, entry{base + "/lan/" + c, "weekly", "0.7"})
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range pages {
		fmt.Fprintf(&sb,
			"  <url>\n    <loc>%s</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>%s</changefreq>\n    <priority>%s</priority>\n  </url>\n",
			p.loc, lastMod, p.freq, p.pri)
	}
	sb.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(200)
	fmt.Fprint(w, sb.String())
}

// robots serves a robots.txt that allows all crawlers and advertises the sitemap.
func (h *handlers) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprintf(w,
		"User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n",
		h.siteBase(r))
}

// ---- Admin access stats -----------------------------------------------------------

// accessStats returns visitor analytics from access_log: today/total counts,
// top paths and IPs over 7 days, and the 50 most recent requests.
func (h *handlers) accessStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var visitorsToday, requestsToday, visitorsTotal, requestsTotal int
	rows, err := h.db.Query(ctx, `
		SELECT
		  COUNT(DISTINCT ip) FILTER (WHERE created_at >= CURRENT_DATE) AS visitors_today,
		  COUNT(*)           FILTER (WHERE created_at >= CURRENT_DATE) AS requests_today,
		  COUNT(DISTINCT ip)                                            AS visitors_total,
		  COUNT(*)                                                      AS requests_total
		FROM access_log`)
	if err != nil {
		dbErr(w, "accessstats", err)
		return
	}
	if rows.Next() {
		rows.Scan(&visitorsToday, &requestsToday, &visitorsTotal, &requestsTotal)
	}
	rows.Close()

	type pathCount struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	}
	var topPaths []pathCount
	rows, err = h.db.Query(ctx, `
		SELECT path, COUNT(*) AS cnt
		FROM access_log
		WHERE created_at >= NOW() - INTERVAL '7 days'
		  AND path NOT LIKE '/health%'
		GROUP BY path ORDER BY cnt DESC LIMIT 10`)
	if err == nil {
		for rows.Next() {
			var p pathCount
			rows.Scan(&p.Path, &p.Count)
			topPaths = append(topPaths, p)
		}
		rows.Close()
	}

	type ipCount struct {
		IP    string `json:"ip"`
		Count int    `json:"count"`
	}
	var topIPs []ipCount
	rows, err = h.db.Query(ctx, `
		SELECT ip, COUNT(*) AS cnt
		FROM access_log
		WHERE created_at >= NOW() - INTERVAL '7 days'
		GROUP BY ip ORDER BY cnt DESC LIMIT 10`)
	if err == nil {
		for rows.Next() {
			var ip ipCount
			rows.Scan(&ip.IP, &ip.Count)
			topIPs = append(topIPs, ip)
		}
		rows.Close()
	}

	type recentEntry struct {
		IP         string    `json:"ip"`
		Method     string    `json:"method"`
		Path       string    `json:"path"`
		Status     int       `json:"status"`
		DurationMs int       `json:"duration_ms"`
		UserAgent  string    `json:"user_agent"`
		Referer    string    `json:"referer"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var recent []recentEntry
	rows, err = h.db.Query(ctx, `
		SELECT ip, method, path, status, duration_ms, COALESCE(user_agent,''), COALESCE(referer,''), created_at
		FROM access_log
		ORDER BY created_at DESC LIMIT 50`)
	if err == nil {
		for rows.Next() {
			var e recentEntry
			rows.Scan(&e.IP, &e.Method, &e.Path, &e.Status, &e.DurationMs, &e.UserAgent, &e.Referer, &e.CreatedAt)
			recent = append(recent, e)
		}
		rows.Close()
	}

	writeJSON(w, 200, map[string]any{
		"visitors_today": visitorsToday,
		"requests_today": requestsToday,
		"visitors_total": visitorsTotal,
		"requests_total": requestsTotal,
		"top_paths":      topPaths,
		"top_ips":        topIPs,
		"recent":         recent,
	})
}

// ---- Trafik ------------------------------------------------------------------

// trafikFlow returns aggregated traffic flow stats (avg speed, total flow) per county.
// Optionally filtered by ?county=ab.
func (h *handlers) trafikFlow(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	rows, err := h.db.Query(r.Context(), `
		SELECT county_code,
		       COUNT(*)                                    AS measurements,
		       ROUND(AVG(avg_speed)::numeric, 1)           AS avg_speed,
		       ROUND(AVG(flow_rate)::numeric, 0)::INT      AS avg_flow_per_lane,
		       MAX(measured_at)                            AS latest_at
		FROM trafik_flow
		WHERE ($1 = '' OR county_code = $1)
		GROUP BY county_code
		ORDER BY measurements DESC NULLS LAST`, county)
	if err != nil {
		dbErr(w, "trafikFlow", err)
		return
	}
	defer rows.Close()

	type item struct {
		County         string     `json:"county"`
		Measurements   int        `json:"measurements"`
		AvgSpeed       *float64   `json:"avg_speed"`
		AvgFlowPerLane *int       `json:"avg_flow_per_lane"`
		LatestAt       *time.Time `json:"latest_at"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.County, &it.Measurements, &it.AvgSpeed, &it.AvgFlowPerLane, &it.LatestAt); err != nil {
			slog.Error("scan error", "handler", "trafikFlow", "err", err)
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// trafik returns active traffic situations from Trafikverket.
// Optional filters: ?county=ab  ?type=Olycka
func (h *handlers) trafik(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	siteType := r.URL.Query().Get("type")
	rows, err := h.db.Query(r.Context(), `
		SELECT external_id, site_type, severity, title, description, road_number,
		       county_codes, lat, lon, start_time, end_time, synced_at
		FROM trafik_situations
		WHERE ($1 = '' OR $1 = ANY(county_codes))
		  AND ($2 = '' OR site_type = $2)
		ORDER BY start_time DESC NULLS LAST
		LIMIT 2000`, county, siteType)
	if err != nil {
		dbErr(w, "trafik", err)
		return
	}
	defer rows.Close()

	type item struct {
		ID          string     `json:"id"`
		SiteType    string     `json:"site_type"`
		Severity    string     `json:"severity"`
		Title       string     `json:"title"`
		Description string     `json:"description"`
		RoadNumber  string     `json:"road_number"`
		CountyCodes []string   `json:"county_codes"`
		Lat         *float64   `json:"lat"`
		Lon         *float64   `json:"lon"`
		StartTime   *time.Time `json:"start_time"`
		EndTime     *time.Time `json:"end_time"`
		SyncedAt    time.Time  `json:"synced_at"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.SiteType, &it.Severity, &it.Title, &it.Description,
			&it.RoadNumber, &it.CountyCodes, &it.Lat, &it.Lon,
			&it.StartTime, &it.EndTime, &it.SyncedAt); err != nil {
			slog.Error("scan error", "handler", "trafik", "err", err)
			continue
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "trafik", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// riksbank returns Riksbanken time-series observations.
// ?series=policy_rate|sek_eur|sek_usd  ?from=2000-01-01
func (h *handlers) riksbank(w http.ResponseWriter, r *http.Request) {
	series := r.URL.Query().Get("series")
	from := r.URL.Query().Get("from")
	if from == "" {
		from = "2000-01-01"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT series_id, date, value
		FROM riksbank_obs
		WHERE ($1 = '' OR series_id = $1)
		  AND date >= $2::date
		ORDER BY series_id, date`,
		series, from)
	if err != nil {
		dbErr(w, "riksbank", err)
		return
	}
	defer rows.Close()
	type item struct {
		SeriesID string  `json:"series_id"`
		Date     string  `json:"date"`
		Value    float64 `json:"value"`
	}
	var result []item
	for rows.Next() {
		var it item
		var d time.Time
		if err := rows.Scan(&it.SeriesID, &d, &it.Value); err != nil {
			continue
		}
		it.Date = d.Format("2006-01-02")
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// elpris returns 15-minute electricity spot prices (Nord Pool quarterly pricing since 2025-10-01).
// ?area=SE1|SE2|SE3|SE4  ?days=2 (default 2)
func (h *handlers) elpris(w http.ResponseWriter, r *http.Request) {
	area := r.URL.Query().Get("area")
	rows, err := h.db.Query(r.Context(), `
		SELECT area, time_start, sek_per_kwh, eur_per_kwh
		FROM elpris
		WHERE ($1 = '' OR area = $1)
		  AND time_start >= NOW() - INTERVAL '2 days'
		ORDER BY area, time_start`,
		area)
	if err != nil {
		dbErr(w, "elpris", err)
		return
	}
	defer rows.Close()
	type item struct {
		Area      string  `json:"area"`
		TimeStart string  `json:"time_start"`
		SEKPerKWh float64 `json:"sek_per_kwh"`
		EURPerKWh float64 `json:"eur_per_kwh"`
	}
	var result []item
	for rows.Next() {
		var it item
		var t time.Time
		if err := rows.Scan(&it.Area, &t, &it.SEKPerKWh, &it.EURPerKWh); err != nil {
			continue
		}
		it.TimeStart = t.UTC().Format(time.RFC3339)
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// arbetsmarknadJobs returns the current live job ad count per county.
func (h *handlers) arbetsmarknadJobs(w http.ResponseWriter, r *http.Request) {
	type row struct {
		CountyCode string `json:"county_code"`
		RegionCode string `json:"region_code"`
		Name       string `json:"name"`
		TotalAds   int    `json:"total_ads"`
		SyncedAt   string `json:"synced_at"`
	}
	rows, err := h.db.Query(r.Context(),
		`SELECT county_code, region_code, name, total_ads, synced_at
		 FROM arbetsmarknad_jobs ORDER BY total_ads DESC`)
	if err != nil {
		dbErr(w, "arbetsmarknad_jobs", err)
		return
	}
	defer rows.Close()
	var result []row
	for rows.Next() {
		var it row
		var t time.Time
		if err := rows.Scan(&it.CountyCode, &it.RegionCode, &it.Name, &it.TotalAds, &t); err != nil {
			continue
		}
		it.SyncedAt = t.UTC().Format(time.RFC3339)
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// arbetsmarknadFields returns the current national breakdown by occupation field.
func (h *handlers) arbetsmarknadFields(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Term      string `json:"term"`
		ConceptID string `json:"concept_id"`
		Count     int    `json:"count"`
	}
	rows, err := h.db.Query(r.Context(),
		`SELECT term, concept_id, count FROM arbetsmarknad_fields ORDER BY count DESC`)
	if err != nil {
		dbErr(w, "arbetsmarknad_fields", err)
		return
	}
	defer rows.Close()
	var result []row
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.Term, &it.ConceptID, &it.Count); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// skolaSchools returns active school counts per municipality, joined with
// Kolada N15504 (meritvärde åk 9) and N15033 (elever per lärare).
// Optional filter: ?county=ab
func (h *handlers) skolaSchools(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	type row struct {
		MunicipalityCode string   `json:"municipality_code"`
		SchoolCount      int      `json:"school_count"`
		MeritValue       *float64 `json:"merit_value"`
		PupilsPerTeacher *float64 `json:"pupils_per_teacher"`
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT
		    s.municipality_code,
		    s.school_count,
		    merit.value::float8,
		    teacher.value::float8
		FROM skola_schools s
		LEFT JOIN LATERAL (
		    SELECT value FROM kolada_stats
		    WHERE municipality_id = s.municipality_code AND kpi_id = 'N15504'
		    ORDER BY period DESC LIMIT 1
		) merit ON true
		LEFT JOIN LATERAL (
		    SELECT value FROM kolada_stats
		    WHERE municipality_id = s.municipality_code AND kpi_id = 'N15033'
		    ORDER BY period DESC LIMIT 1
		) teacher ON true
		WHERE ($1 = '' OR s.municipality_code IN (
		    SELECT municipality_id FROM kolada_stats WHERE county_code = $1
		))
		ORDER BY merit.value DESC NULLS LAST`,
		county)
	if err != nil {
		dbErr(w, "skola_schools", err)
		return
	}
	defer rows.Close()
	var result []row
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.MunicipalityCode, &it.SchoolCount, &it.MeritValue, &it.PupilsPerTeacher); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// skolaSalsa returns per-municipality SALSA aggregates (avg actual grade,
// deviation from expected, requirements-reached %, parents' education).
// Optional filter: ?county=ab
func (h *handlers) skolaSalsa(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	type row struct {
		MunicipalityCode string   `json:"municipality_code"`
		SchoolCount      int      `json:"school_count"`
		AvgActual        *float64 `json:"avg_actual"`
		AvgCalculated    *float64 `json:"avg_calculated"`
		AvgDeviation     *float64 `json:"avg_deviation"`
		ReqReachedActual *float64 `json:"req_reached_actual"`
		ReqReachedDev    *float64 `json:"req_reached_dev"`
		ParentsEducation *float64 `json:"parents_education"`
	}
	salsaRows, err := h.db.Query(r.Context(), `
		SELECT municipality_code, school_count, avg_actual::float8, avg_calculated::float8,
		       avg_deviation::float8, req_reached_actual::float8, req_reached_dev::float8,
		       parents_education::float8
		FROM skola_salsa
		WHERE ($1 = '' OR municipality_code IN (
		    SELECT municipality_id FROM kolada_stats WHERE county_code = $1
		))
		ORDER BY avg_deviation DESC NULLS LAST`,
		county)
	if err != nil {
		dbErr(w, "skola_salsa", err)
		return
	}
	defer salsaRows.Close()
	var result []row
	for salsaRows.Next() {
		var it row
		if err := salsaRows.Scan(
			&it.MunicipalityCode, &it.SchoolCount,
			&it.AvgActual, &it.AvgCalculated, &it.AvgDeviation,
			&it.ReqReachedActual, &it.ReqReachedDev, &it.ParentsEducation,
		); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// skolaNational returns the latest national grundskola statistics row.
func (h *handlers) skolaNational(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Period               string   `json:"period"`
		StudentsPerTeacher   *float64 `json:"students_per_teacher"`
		CertifiedTeachersPct *float64 `json:"certified_teachers_pct"`
		RatioPassed9th       *float64 `json:"ratio_passed_9th"`
		AvgMerit9th          *float64 `json:"avg_merit_9th"`
		EligibleYRPct        *float64 `json:"eligible_yr_pct"`
		EligibleESPct        *float64 `json:"eligible_es_pct"`
		RatioPassed6th       *float64 `json:"ratio_passed_6th"`
		TestSve6             *float64 `json:"test_sve_6"`
		TestEng6             *float64 `json:"test_eng_6"`
		TestMa6              *float64 `json:"test_ma_6"`
		TestSve9             *float64 `json:"test_sve_9"`
		TestEng9             *float64 `json:"test_eng_9"`
		TestMa9              *float64 `json:"test_ma_9"`
	}
	natRows, err := h.db.Query(r.Context(), `
		SELECT period, students_per_teacher::float8, certified_teachers_pct::float8,
		       ratio_passed_9th::float8, avg_merit_9th::float8,
		       eligible_yr_pct::float8, eligible_es_pct::float8,
		       ratio_passed_6th::float8, test_sve_6::float8, test_eng_6::float8, test_ma_6::float8,
		       test_sve_9::float8, test_eng_9::float8, test_ma_9::float8
		FROM skola_national ORDER BY period DESC LIMIT 1`)
	if err != nil {
		dbErr(w, "skola_national", err)
		return
	}
	defer natRows.Close()
	var it row
	if natRows.Next() {
		if err := natRows.Scan(
			&it.Period, &it.StudentsPerTeacher, &it.CertifiedTeachersPct,
			&it.RatioPassed9th, &it.AvgMerit9th, &it.EligibleYRPct, &it.EligibleESPct,
			&it.RatioPassed6th, &it.TestSve6, &it.TestEng6, &it.TestMa6,
			&it.TestSve9, &it.TestEng9, &it.TestMa9,
		); err != nil {
			dbErr(w, "skola_national_scan", err)
			return
		}
	}
	writeJSON(w, 200, it)
}

// ---- Socialstyrelsen ---------------------------------------------------------

// ssDeaths returns national cause-of-death totals aggregated by diagnos and year.
// ?year=2022 filters to a single year. ?diagnos=99 filters to one diagnos_id.
func (h *handlers) ssDeaths(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	diagnos := r.URL.Query().Get("diagnos")
	rows, err := h.db.Query(r.Context(), `
		SELECT diagnos_id, diagnos_name, year, SUM(value)::int AS total
		FROM ss_deaths
		WHERE ($1 = '' OR year::text = $1)
		  AND ($2 = '' OR diagnos_id = $2)
		  AND value IS NOT NULL
		GROUP BY diagnos_id, diagnos_name, year
		ORDER BY year DESC, total DESC`,
		year, diagnos)
	if err != nil {
		dbErr(w, "ss_deaths", err)
		return
	}
	defer rows.Close()
	type row struct {
		DiagnosID   string `json:"diagnos_id"`
		DiagnosName string `json:"diagnos_name"`
		Year        int    `json:"year"`
		Total       *int   `json:"total"`
	}
	var result []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.DiagnosID, &x.DiagnosName, &x.Year, &x.Total); err != nil {
			slog.Error("scan error", "handler", "ssDeaths", "err", err)
			continue
		}
		result = append(result, x)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "ssDeaths", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ssDental returns dental visit rates per region and year.
// ?county=ab filters to a county. ?year=2022 filters to one year.
func (h *handlers) ssDental(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	year := r.URL.Query().Get("year")
	rows, err := h.db.Query(r.Context(), `
		SELECT region_id, region_name, county_code, year, value::float8
		FROM ss_dental
		WHERE ($1 = '' OR county_code = $1)
		  AND ($2 = '' OR year::text = $2)
		ORDER BY year DESC, value DESC NULLS LAST`,
		county, year)
	if err != nil {
		dbErr(w, "ss_dental", err)
		return
	}
	defer rows.Close()
	type row struct {
		RegionID   int      `json:"region_id"`
		RegionName string   `json:"region_name"`
		CountyCode string   `json:"county_code"`
		Year       int      `json:"year"`
		Value      *float64 `json:"value"`
	}
	var result []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.RegionID, &x.RegionName, &x.CountyCode, &x.Year, &x.Value); err != nil {
			slog.Error("scan error", "handler", "ssDental", "err", err)
			continue
		}
		result = append(result, x)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "ssDental", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ssBirths returns births and newborn health indicator rows.
// ?variabel=V35 filters to one variable. ?county=ab filters by county.
func (h *handlers) ssBirths(w http.ResponseWriter, r *http.Request) {
	variabel := r.URL.Query().Get("variabel")
	county := r.URL.Query().Get("county")
	year := r.URL.Query().Get("year")
	rows, err := h.db.Query(r.Context(), `
		SELECT variabel_id, variabel_name, region_id, region_name, county_code, year, value
		FROM ss_births
		WHERE ($1 = '' OR variabel_id = $1)
		  AND ($2 = '' OR county_code = $2)
		  AND ($3 = '' OR year::text = $3)
		ORDER BY year DESC, region_id`,
		variabel, county, year)
	if err != nil {
		dbErr(w, "ss_births", err)
		return
	}
	defer rows.Close()
	type row struct {
		VariabelID   string `json:"variabel_id"`
		VariabelName string `json:"variabel_name"`
		RegionID     int    `json:"region_id"`
		RegionName   string `json:"region_name"`
		CountyCode   string `json:"county_code"`
		Year         int    `json:"year"`
		Value        *int   `json:"value"`
	}
	var result []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.VariabelID, &x.VariabelName, &x.RegionID, &x.RegionName, &x.CountyCode, &x.Year, &x.Value); err != nil {
			slog.Error("scan error", "handler", "ssBirths", "err", err)
			continue
		}
		result = append(result, x)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "handler", "ssBirths", "err", err)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// ── County Rankings & Profile ─────────────────────────────────────────────────

// countyMeta maps 2-letter county codes to display names.
var countyMeta = map[string]string{
	"ab": "Stockholm",  "c": "Uppsala",   "d": "Södermanland",
	"e": "Östergötland","f": "Jönköping",  "g": "Kronoberg",
	"h": "Kalmar",      "i": "Gotland",    "k": "Blekinge",
	"m": "Skåne",       "n": "Halland",    "o": "Västra Götaland",
	"s": "Värmland",    "t": "Örebro",     "u": "Västmanland",
	"w": "Dalarna",     "x": "Gävleborg",  "y": "Västernorrland",
	"z": "Jämtland",    "ac": "Västerbotten", "bd": "Norrbotten",
}

// valkretsToCounty maps area_code slugs in val_results to 2-letter county codes.
var valkretsToCounty = map[string]string{
	"riket": "", // skip
	"stockholm_lan":  "ab", "stockholm_stad": "ab",
	"goteborg":       "o",  "malmo":          "m",
	"uppsala":        "c",  "sodermanland":   "d",
	"ostergotland":   "e",  "jonkoping":      "f",
	"kronoberg":      "g",  "kalmar":         "h",
	"gotland":        "i",  "blekinge":       "k",
	"skane_no":       "m",  "skane_s":        "m",  "skane_v": "m",
	"halland":        "n",
	"vgr_norra":      "o",  "vgr_sodra":      "o",
	"vgr_vastra":     "o",  "vgr_ostra":      "o",
	"varmland":       "s",  "orebro":         "t",
	"vastmanland":    "u",  "dalarna":        "w",
	"gavleborg":      "x",  "vasternorrland": "y",
	"jamtland":       "z",  "vasterbotten":   "ac",
	"norrbotten":     "bd",
}

// partyColors maps party codes to hex colours for charts.
var partyColors = map[string]string{
	"S":   "#E8112D", // Socialdemokraterna
	"M":   "#1B70AE", // Moderaterna
	"SD":  "#DDDD00", // Sverigedemokraterna (dark gold)
	"C":   "#009933", // Centerpartiet
	"V":   "#B5242D", // Vänsterpartiet
	"KD":  "#231977", // Kristdemokraterna
	"L":   "#006AB3", // Liberalerna
	"MP":  "#83C31E", // Miljöpartiet
	"OVR": "#888888", // Övriga
}

type countyMetrics struct {
	IncomeMedian      *float64 `json:"income_median"`
	IncomeRank        int      `json:"income_rank"`
	UnemploymentPct   *float64 `json:"unemployment_pct"`
	UnemploymentRank  int      `json:"unemployment_rank"`
	SchoolGrade       *float64 `json:"school_grade"`
	SchoolGradeRank   int      `json:"school_grade_rank"`
	LifeExpectancy    *float64 `json:"life_expectancy"`
	LifeExpectancyRank int     `json:"life_expectancy_rank"`
	CrimePer100k      *float64 `json:"crime_per_100k"`
	CrimeRank         int      `json:"crime_rank"`
	WelfarePct        *float64 `json:"welfare_pct"`
	WelfareRank       int      `json:"welfare_rank"`
}

type countyPolitics struct {
	DominantParty string             `json:"dominant_party"`
	DominantPct   float64            `json:"dominant_pct"`
	DominantColor string             `json:"dominant_color"`
	Parties       map[string]float64 `json:"parties"`
}

type countyRankingRow struct {
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Metrics  countyMetrics  `json:"metrics"`
	Politics countyPolitics `json:"politics_2022"`
}

// lanRankings returns all 21 counties with socioeconomic metrics and
// 2022 election results, plus computed ranks (1 = best) for each metric.
func (h *handlers) lanRankings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Collect all metrics keyed by county code.
	counties := map[string]*countyRankingRow{}
	for code, name := range countyMeta {
		counties[code] = &countyRankingRow{Code: code, Name: name,
			Politics: countyPolitics{Parties: map[string]float64{}}}
	}

	// ── Income: latest median per county ────────────────────────────────
	rows, err := h.db.Query(ctx, `
		SELECT county_code, value::float8
		FROM scb_stats
		WHERE metric = 'HE0110K1'
		  AND year = (SELECT MAX(year) FROM scb_stats WHERE metric = 'HE0110K1')
		  AND county_code != ''`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var code string; var val float64
			if rows.Scan(&code, &val) == nil {
				if c, ok := counties[code]; ok { c.Metrics.IncomeMedian = &val }
			}
		}
		rows.Close()
	}

	// ── Unemployment: latest per county (avg municipalities) ────────────
	rows2, err := h.db.Query(ctx, `
		SELECT county_code, AVG(value)::float8
		FROM kolada_stats
		WHERE kpi_id = 'N03920'
		  AND period = (SELECT MAX(period) FROM kolada_stats WHERE kpi_id = 'N03920')
		  AND county_code != ''
		GROUP BY county_code`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var code string; var val float64
			if rows2.Scan(&code, &val) == nil {
				if c, ok := counties[code]; ok { c.Metrics.UnemploymentPct = &val }
			}
		}
		rows2.Close()
	}

	// ── School grade: latest meritvärde per county ──────────────────────
	rows3, err := h.db.Query(ctx, `
		SELECT county_code, AVG(value)::float8
		FROM kolada_stats
		WHERE kpi_id = 'N15504'
		  AND period = (SELECT MAX(period) FROM kolada_stats WHERE kpi_id = 'N15504')
		  AND county_code != ''
		GROUP BY county_code`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var code string; var val float64
			if rows3.Scan(&code, &val) == nil {
				if c, ok := counties[code]; ok { c.Metrics.SchoolGrade = &val }
			}
		}
		rows3.Close()
	}

	// ── Life expectancy: latest per county (sex = T) ─────────────────────
	rows4, err := h.db.Query(ctx, `
		SELECT county_code, value::float8
		FROM folkhalsa_stats
		WHERE metric = 'lifeexp' AND sex = 'total'
		  AND period = (SELECT MAX(period) FROM folkhalsa_stats WHERE metric = 'lifeexp')
		  AND county_code != ''`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var code string; var val float64
			if rows4.Scan(&code, &val) == nil {
				if c, ok := counties[code]; ok { c.Metrics.LifeExpectancy = &val }
			}
		}
		rows4.Close()
	}

	// ── Crime per 100k: latest total per county ──────────────────────────
	rows5, err := h.db.Query(ctx, `
		SELECT county_code, AVG(per_100k)::float8
		FROM bra_crimes
		WHERE category = 'Totalt antal brott'
		  AND year = (SELECT MAX(year) FROM bra_crimes WHERE category = 'Totalt antal brott')
		  AND county_code != ''
		GROUP BY county_code`)
	if err == nil {
		defer rows5.Close()
		for rows5.Next() {
			var code string; var val float64
			if rows5.Scan(&code, &val) == nil {
				if c, ok := counties[code]; ok { c.Metrics.CrimePer100k = &val }
			}
		}
		rows5.Close()
	}

	// ── Welfare (ekonomiskt bistånd) per county ───────────────────────────
	rows6, err := h.db.Query(ctx, `
		SELECT county_code, AVG(value)::float8
		FROM kolada_stats
		WHERE kpi_id = 'N31004'
		  AND period = (SELECT MAX(period) FROM kolada_stats WHERE kpi_id = 'N31004')
		  AND county_code != ''
		GROUP BY county_code`)
	if err == nil {
		defer rows6.Close()
		for rows6.Next() {
			var code string; var val float64
			if rows6.Scan(&code, &val) == nil {
				if c, ok := counties[code]; ok { c.Metrics.WelfarePct = &val }
			}
		}
		rows6.Close()
	}

	// ── Politics 2022: aggregate valkretsar → county ─────────────────────
	// Use AVG(vote_share) across the valkretsar that belong to each county.
	// This handles split counties (Skåne→3 valkretsar, VGR→4, Stockholm→2).
	rows7, err := h.db.Query(ctx, `
		SELECT area_code, party_code, vote_share::float8
		FROM val_results
		WHERE election_year = 2022 AND area_code != 'riket'`)
	if err == nil {
		defer rows7.Close()
		type countPct struct{ sum float64; n int }
		totals := map[string]map[string]*countPct{}
		for rows7.Next() {
			var areaCode, partyCode string; var share float64
			if rows7.Scan(&areaCode, &partyCode, &share) != nil { continue }
			countyCode, ok := valkretsToCounty[areaCode]
			if !ok || countyCode == "" { continue }
			if totals[countyCode] == nil { totals[countyCode] = map[string]*countPct{} }
			if totals[countyCode][partyCode] == nil { totals[countyCode][partyCode] = &countPct{} }
			totals[countyCode][partyCode].sum += share
			totals[countyCode][partyCode].n++
		}
		rows7.Close()
		for countyCode, partyMap := range totals {
			c, ok := counties[countyCode]
			if !ok { continue }
			var domParty string; var domPct float64
			for party, cp := range partyMap {
				if cp.n == 0 { continue }
				pct := cp.sum / float64(cp.n)
				c.Politics.Parties[party] = pct
				if pct > domPct { domPct = pct; domParty = party }
			}
			c.Politics.DominantParty = domParty
			c.Politics.DominantPct   = domPct
			if col, ok := partyColors[domParty]; ok { c.Politics.DominantColor = col }
		}
	}

	// ── Compute ranks ────────────────────────────────────────────────────
	list := make([]*countyRankingRow, 0, len(counties))
	for _, c := range counties { list = append(list, c) }

	rankDesc := func(valFn func(*countyRankingRow) *float64, rankSet func(*countyRankingRow, int)) {
		type pair struct{ code string; val float64 }
		var pairs []pair
		for _, c := range list {
			if v := valFn(c); v != nil { pairs = append(pairs, pair{c.Code, *v}) }
		}
		// sort descending (higher = better)
		for i := 0; i < len(pairs); i++ {
			for j := i + 1; j < len(pairs); j++ {
				if pairs[j].val > pairs[i].val { pairs[i], pairs[j] = pairs[j], pairs[i] }
			}
		}
		for rank, p := range pairs {
			if c, ok := counties[p.code]; ok { rankSet(c, rank+1) }
		}
	}
	rankAsc := func(valFn func(*countyRankingRow) *float64, rankSet func(*countyRankingRow, int)) {
		type pair struct{ code string; val float64 }
		var pairs []pair
		for _, c := range list {
			if v := valFn(c); v != nil { pairs = append(pairs, pair{c.Code, *v}) }
		}
		// sort ascending (lower = better)
		for i := 0; i < len(pairs); i++ {
			for j := i + 1; j < len(pairs); j++ {
				if pairs[j].val < pairs[i].val { pairs[i], pairs[j] = pairs[j], pairs[i] }
			}
		}
		for rank, p := range pairs {
			if c, ok := counties[p.code]; ok { rankSet(c, rank+1) }
		}
	}

	// Income: higher = better (rank 1 = highest income)
	rankDesc(func(c *countyRankingRow) *float64 { return c.Metrics.IncomeMedian },
		func(c *countyRankingRow, r int) { c.Metrics.IncomeRank = r })
	// Unemployment: lower = better (rank 1 = lowest unemployment)
	rankAsc(func(c *countyRankingRow) *float64 { return c.Metrics.UnemploymentPct },
		func(c *countyRankingRow, r int) { c.Metrics.UnemploymentRank = r })
	// School grade: higher = better
	rankDesc(func(c *countyRankingRow) *float64 { return c.Metrics.SchoolGrade },
		func(c *countyRankingRow, r int) { c.Metrics.SchoolGradeRank = r })
	// Life expectancy: higher = better
	rankDesc(func(c *countyRankingRow) *float64 { return c.Metrics.LifeExpectancy },
		func(c *countyRankingRow, r int) { c.Metrics.LifeExpectancyRank = r })
	// Crime: lower = better (rank 1 = lowest crime)
	rankAsc(func(c *countyRankingRow) *float64 { return c.Metrics.CrimePer100k },
		func(c *countyRankingRow, r int) { c.Metrics.CrimeRank = r })
	// Welfare: lower = better (rank 1 = fewest welfare recipients)
	rankAsc(func(c *countyRankingRow) *float64 { return c.Metrics.WelfarePct },
		func(c *countyRankingRow, r int) { c.Metrics.WelfareRank = r })

	writeJSON(w, 200, response{Data: list, Count: len(list)})
}

// lanProfile returns a full county profile for ?county=ab including
// historical income, population, and per-year election results.
func (h *handlers) lanProfile(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	if _, ok := countyMeta[county]; !ok {
		writeErr(w, 400, "unknown county code")
		return
	}
	ctx := r.Context()

	type profileResponse struct {
		Code           string                     `json:"code"`
		Name           string                     `json:"name"`
		IncomeHistory  []map[string]interface{}   `json:"income_history"`
		PopHistory     []map[string]interface{}   `json:"pop_history"`
		ElectionHistory []map[string]interface{}  `json:"election_history"`
		CrimeHistory   []map[string]interface{}   `json:"crime_history"`
		HealthLatest   map[string]interface{}      `json:"health_latest"`
	}

	prof := profileResponse{Code: county, Name: countyMeta[county]}

	// ── Income history ───────────────────────────────────────────────────
	rows, err := h.db.Query(ctx, `
		SELECT year, metric, value::float8
		FROM scb_stats
		WHERE county_code = $1 AND table_id = 'inkomst' AND year >= 1995
		ORDER BY year, metric`, county)
	if err == nil {
		defer rows.Close()
		byYear := map[int]map[string]float64{}
		for rows.Next() {
			var year int; var metric string; var val float64
			if rows.Scan(&year, &metric, &val) != nil { continue }
			if byYear[year] == nil { byYear[year] = map[string]float64{} }
			byYear[year][metric] = val
		}
		rows.Close()
		for year := 1995; year <= time.Now().Year(); year++ {
			if m, ok := byYear[year]; ok {
				prof.IncomeHistory = append(prof.IncomeHistory, map[string]interface{}{
					"year": year, "median": m["HE0110K1"], "mean": m["HE0110K2"],
				})
			}
		}
	}

	// ── Population history ───────────────────────────────────────────────
	rows2, err := h.db.Query(ctx, `
		SELECT year, value::float8
		FROM scb_stats
		WHERE county_code = $1 AND table_id = 'befolkning' AND metric = 'BE0101N1' AND year >= 2000
		ORDER BY year`, county)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var year int; var val float64
			if rows2.Scan(&year, &val) != nil { continue }
			prof.PopHistory = append(prof.PopHistory, map[string]interface{}{"year": year, "pop": val})
		}
		rows2.Close()
	}

	// ── Election history: all years per county's valkretsar ──────────────
	// find all area_codes that map to this county
	var areaCodes []string
	for areaCode, cc := range valkretsToCounty {
		if cc == county { areaCodes = append(areaCodes, areaCode) }
	}
	if len(areaCodes) > 0 {
		rows3, err := h.db.Query(ctx, `
			SELECT election_year, party_code, SUM(votes)::float8, AVG(vote_share)::float8
			FROM val_results
			WHERE area_code = ANY($1)
			GROUP BY election_year, party_code
			ORDER BY election_year, party_code`, areaCodes)
		if err == nil {
			defer rows3.Close()
			byYear := map[int]map[string]float64{}
			for rows3.Next() {
				var year int; var party string; var votes, share float64
				if rows3.Scan(&year, &party, &votes, &share) != nil { continue }
				if byYear[year] == nil { byYear[year] = map[string]float64{} }
				byYear[year][party] = share
			}
			rows3.Close()
			for year := 2002; year <= 2022; year += 4 {
				if m, ok := byYear[year]; ok {
					entry := map[string]interface{}{"year": year}
					for party, pct := range m { entry[party] = pct }
					prof.ElectionHistory = append(prof.ElectionHistory, entry)
				}
			}
		}
	}

	// ── Crime history: total + breakdown latest year ─────────────────────
	rows4, err := h.db.Query(ctx, `
		SELECT year, category, SUM(per_100k)::float8
		FROM bra_crimes
		WHERE county_code = $1 AND year >= 2010
		GROUP BY year, category
		ORDER BY year, category`, county)
	if err == nil {
		defer rows4.Close()
		byYear := map[int]map[string]float64{}
		for rows4.Next() {
			var year int; var cat string; var val float64
			if rows4.Scan(&year, &cat, &val) != nil { continue }
			if byYear[year] == nil { byYear[year] = map[string]float64{} }
			byYear[year][cat] = val
		}
		rows4.Close()
		for year := 2010; year <= time.Now().Year(); year++ {
			if m, ok := byYear[year]; ok {
				entry := map[string]interface{}{"year": year}
				for cat, val := range m { entry[cat] = val }
				prof.CrimeHistory = append(prof.CrimeHistory, entry)
			}
		}
	}

	// ── Health: latest all metrics ───────────────────────────────────────
	rows5, err := h.db.Query(ctx, `
		SELECT metric, value::float8
		FROM folkhalsa_stats
		WHERE county_code = $1 AND sex = 'total'
		  AND period = (SELECT MAX(period) FROM folkhalsa_stats WHERE county_code = $1 AND sex = 'total')`,
		county)
	if err == nil {
		defer rows5.Close()
		prof.HealthLatest = map[string]interface{}{}
		for rows5.Next() {
			var metric string; var val float64
			if rows5.Scan(&metric, &val) != nil { continue }
			prof.HealthLatest[metric] = val
		}
		rows5.Close()
	}

	writeJSON(w, 200, prof)
}

// sjukskrivning returns FK F43 (stress) sjukfall data.
// ?county=ab returns that county's monthly series.
// Without ?county returns all counties latest month + yearly national trend.
func (h *handlers) sjukskrivning(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	county := r.URL.Query().Get("county")

	if county != "" {
		// Time series for one county — monthly 2023+, yearly Dec before that
		rows, err := h.db.Query(ctx, `
			SELECT year, month, andel, antal
			FROM fk_sjukfall
			WHERE county_code = $1 AND diagnos = 'F43'
			ORDER BY year, month`, county)
		if err != nil {
			writeErr(w, 500, "db error")
			return
		}
		defer rows.Close()
		type point struct {
			Year  int      `json:"year"`
			Month int      `json:"month"`
			Andel *float64 `json:"andel"`
			Antal *int     `json:"antal"`
		}
		var pts []point
		for rows.Next() {
			var p point
			if rows.Scan(&p.Year, &p.Month, &p.Andel, &p.Antal) == nil {
				pts = append(pts, p)
			}
		}
		writeJSON(w, 200, map[string]any{"county": county, "data": pts})
		return
	}

	// All counties — latest available month (for ranking bar)
	type countyLatest struct {
		Code  string   `json:"code"`
		Andel *float64 `json:"andel"`
		Antal *int     `json:"antal"`
		Year  int      `json:"year"`
		Month int      `json:"month"`
	}
	rows2, err := h.db.Query(ctx, `
		SELECT DISTINCT ON (county_code)
			county_code, andel, antal, year, month
		FROM fk_sjukfall
		WHERE diagnos = 'F43'
		ORDER BY county_code, year DESC, month DESC`)
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	defer rows2.Close()
	var counties []countyLatest
	for rows2.Next() {
		var c countyLatest
		if rows2.Scan(&c.Code, &c.Andel, &c.Antal, &c.Year, &c.Month) == nil {
			counties = append(counties, c)
		}
	}
	rows2.Close()

	// National yearly trend (average across all counties, December)
	rows3, err := h.db.Query(ctx, `
		SELECT year, ROUND(AVG(andel)::numeric,2)
		FROM fk_sjukfall
		WHERE diagnos = 'F43' AND month = 12
		GROUP BY year ORDER BY year`)
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	defer rows3.Close()
	type yearPt struct {
		Year  int      `json:"year"`
		Andel *float64 `json:"andel"`
	}
	var trend []yearPt
	for rows3.Next() {
		var p yearPt
		if rows3.Scan(&p.Year, &p.Andel) == nil {
			trend = append(trend, p)
		}
	}

	writeJSON(w, 200, map[string]any{
		"counties": counties,
		"trend":    trend,
	})
}

func (h *handlers) foretagsoverskott(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	if year == "" {
		year = "2022"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT year, municipality, belopp_sek, antal
		FROM foretagsoverskott
		WHERE ($1 = 'all' OR year = $1::int)
		ORDER BY belopp_sek DESC`, year)
	if err != nil {
		dbErr(w, "foretagsoverskott", err)
		return
	}
	defer rows.Close()
	type item struct {
		Year         int    `json:"year"`
		Municipality string `json:"municipality"`
		BeloppSEK    int64  `json:"belopp_sek"`
		Antal        int    `json:"antal"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Municipality, &it.BeloppSEK, &it.Antal); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

func (h *handlers) kapitalinkomst(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	if year == "" {
		year = "2022"
	}
	term := r.URL.Query().Get("term") // optional filter by statistikterm
	rows, err := h.db.Query(r.Context(), `
		SELECT year, county_name, statistikterm, belopp_sek, antal
		FROM kapitalinkomst
		WHERE ($1 = 'all' OR year = $1::int)
		  AND ($2 = '' OR statistikterm = $2)
		ORDER BY year DESC, belopp_sek DESC`, year, term)
	if err != nil {
		dbErr(w, "kapitalinkomst", err)
		return
	}
	defer rows.Close()
	type item struct {
		Year          int    `json:"year"`
		CountyName    string `json:"county_name"`
		Statistikterm string `json:"statistikterm"`
		BeloppSEK     int64  `json:"belopp_sek"`
		Antal         int    `json:"antal"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.CountyName, &it.Statistikterm, &it.BeloppSEK, &it.Antal); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

func (h *handlers) skattPerPerson(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	if year == "" {
		year = "2022"
	}
	county := r.URL.Query().Get("county")
	// typ selects one typ_av_skatt; default is direct labour taxes (the headline
	// "what people pay" number). typ=all returns every tax type.
	typ := r.URL.Query().Get("typ")
	if typ == "" {
		typ = "Direkta skatter på arbete"
	}
	// Join with skattesats to get county_code for filtering
	rows, err := h.db.Query(r.Context(), `
		SELECT sp.year, sp.municipality, sp.typ_av_skatt, sp.belopp_sek, sp.median_kr, sp.medel_kr, sp.antal,
		       COALESCE(sk.county_code,'') AS county_code
		FROM skatt_per_person sp
		LEFT JOIN skattesats sk ON sk.municipality = sp.municipality AND sk.year = sp.year
		WHERE ($1 = 'all' OR sp.year = $1::int)
		  AND ($2 = '' OR sk.county_code = $2)
		  AND ($3 = 'all' OR sp.typ_av_skatt = $3)
		ORDER BY sp.median_kr DESC`, year, county, typ)
	if err != nil {
		dbErr(w, "skatt_per_person", err)
		return
	}
	defer rows.Close()
	type item struct {
		Year         int    `json:"year"`
		Municipality string `json:"municipality"`
		TypAvSkatt   string `json:"typ_av_skatt"`
		BeloppSEK    int64  `json:"belopp_sek"`
		MedianKr     int    `json:"median_kr"`
		MedelKr      int    `json:"medel_kr"`
		Antal        int    `json:"antal"`
		County       string `json:"county"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Municipality, &it.TypAvSkatt, &it.BeloppSEK, &it.MedianKr, &it.MedelKr, &it.Antal, &it.County); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

func (h *handlers) kommunalskattInkomstniva(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	if year == "" {
		year = "2022"
	}
	term := r.URL.Query().Get("term")
	if term == "" {
		term = "Kommunal inkomstskatt"
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT year, inkomstniva, statistikterm, belopp_sek, antal
		FROM kommunalskatt_inkomstniva
		WHERE ($1 = 'all' OR year = $1::int)
		  AND ($2 = 'all' OR statistikterm = $2)
		ORDER BY year DESC, belopp_sek DESC`, year, term)
	if err != nil {
		dbErr(w, "kommunalskatt_inkomstniva", err)
		return
	}
	defer rows.Close()
	type item struct {
		Year          int    `json:"year"`
		Inkomstniva   string `json:"inkomstniva"`
		Statistikterm string `json:"statistikterm"`
		BeloppSEK     int64  `json:"belopp_sek"`
		Antal         int    `json:"antal"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Year, &it.Inkomstniva, &it.Statistikterm, &it.BeloppSEK, &it.Antal); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// samband returns one row per municipality with metrics joined from five
// sources (Skatteverket, Kolada, BRÅ, SCB via kolada population), for the
// cross-source correlation explorer on /samband. Each metric uses the latest
// available year in its own table; nulls mean the source lacks that municipality.
func (h *handlers) samband(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `
		WITH tax AS (
			SELECT municipality, county_code,
			       (kommunal_skatt + landstings_skatt)::float8 AS v,
			       year
			FROM skattesats
			WHERE year = (SELECT max(year) FROM skattesats)
		),
		k AS (
			SELECT kpi_id, UPPER(municipality) AS m, value::float8 AS v,
			       ROW_NUMBER() OVER (PARTITION BY kpi_id, municipality_id ORDER BY period DESC) AS rn
			FROM kolada_stats
			WHERE kpi_id IN ('N03920','N00989','N00990','N15504','N31004','N01951')
			  AND value IS NOT NULL
		),
		crime AS (
			SELECT UPPER(kommun) AS m, per_100k::float8 AS v
			FROM bra_crimes
			WHERE slug = 'totalt'
			  AND year = (SELECT max(year) FROM bra_crimes WHERE slug='totalt')
		),
		spp AS (
			SELECT UPPER(municipality) AS m, median_kr::float8 AS v
			FROM skatt_per_person
			WHERE typ_av_skatt = 'Direkta skatter på arbete'
			  AND year = (SELECT max(year) FROM skatt_per_person)
		),
		forn AS (
			SELECT UPPER(kommun) AS m, antal::float8 AS v FROM raa_fornlamningar
		)
		SELECT t.municipality, t.county_code, t.v AS skattesats,
		       max(k.v) FILTER (WHERE k.kpi_id='N03920') AS arbetsloshet,
		       max(k.v) FILTER (WHERE k.kpi_id='N00989') AS skuldsatta,
		       max(k.v) FILTER (WHERE k.kpi_id='N00990') AS medianskuld,
		       max(k.v) FILTER (WHERE k.kpi_id='N15504') AS meritvarde,
		       max(k.v) FILTER (WHERE k.kpi_id='N31004') AS bistand,
		       max(k.v) FILTER (WHERE k.kpi_id='N01951') AS befolkning,
		       max(crime.v) AS brott,
		       max(spp.v)   AS median_skatt,
		       max(forn.v)  AS fornlamningar
		FROM tax t
		LEFT JOIN k     ON k.m = t.municipality AND k.rn = 1
		LEFT JOIN crime ON crime.m = t.municipality
		LEFT JOIN spp   ON spp.m = t.municipality
		LEFT JOIN forn  ON forn.m = t.municipality
		GROUP BY t.municipality, t.county_code, t.v
		ORDER BY t.municipality`)
	if err != nil {
		dbErr(w, "samband", err)
		return
	}
	defer rows.Close()
	type item struct {
		Municipality string   `json:"municipality"`
		County       string   `json:"county"`
		Skattesats   float64  `json:"skattesats"`
		Arbetsloshet *float64 `json:"arbetsloshet"`
		Skuldsatta   *float64 `json:"skuldsatta"`
		Medianskuld  *float64 `json:"medianskuld"`
		Meritvarde   *float64 `json:"meritvarde"`
		Bistand      *float64 `json:"bistand"`
		Befolkning   *float64 `json:"befolkning"`
		Brott        *float64 `json:"brott"`
		MedianSkatt  *float64 `json:"median_skatt"`
		Fornlamningar *float64 `json:"fornlamningar"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Municipality, &it.County, &it.Skattesats,
			&it.Arbetsloshet, &it.Skuldsatta, &it.Medianskuld, &it.Meritvarde,
			&it.Bistand, &it.Befolkning, &it.Brott, &it.MedianSkatt, &it.Fornlamningar); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// riksdagenKalender returns upcoming riksdag events (next 4 weeks, no tours).
func (h *handlers) riksdagenKalender(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `
		SELECT dok_id, datum, typ, subtyp, titel, undertitel, organ, plats
		FROM riksdagen_kalender
		ORDER BY datum ASC
		LIMIT 100`)
	if err != nil {
		dbErr(w, "riksdagen_kalender", err)
		return
	}
	defer rows.Close()
	type item struct {
		DokID      string `json:"dok_id"`
		Datum      string `json:"datum"`
		Typ        string `json:"typ"`
		Subtyp     string `json:"subtyp"`
		Titel      string `json:"titel"`
		Undertitel string `json:"undertitel"`
		Organ      string `json:"organ"`
		Plats      string `json:"plats"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.DokID, &it.Datum, &it.Typ, &it.Subtyp, &it.Titel, &it.Undertitel, &it.Organ, &it.Plats); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}

// fornlamningar returns registered cultural heritage sites per municipality
// (RAÄ point layer), with county for filtering and aggregation.
func (h *handlers) fornlamningar(w http.ResponseWriter, r *http.Request) {
	county := r.URL.Query().Get("county")
	rows, err := h.db.Query(r.Context(), `
		SELECT kommun, county_code, antal
		FROM raa_fornlamningar
		WHERE ($1 = '' OR county_code = $1)
		ORDER BY antal DESC`, county)
	if err != nil {
		dbErr(w, "raa_fornlamningar", err)
		return
	}
	defer rows.Close()
	type item struct {
		Kommun string `json:"kommun"`
		County string `json:"county"`
		Antal  int    `json:"antal"`
	}
	var result []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Kommun, &it.County, &it.Antal); err != nil {
			continue
		}
		result = append(result, it)
	}
	writeJSON(w, 200, response{Data: result, Count: len(result)})
}
