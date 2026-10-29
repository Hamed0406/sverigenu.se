// Package store persists fetched data into PostgreSQL. Every Upsert method
// follows the same pattern: INSERT … ON CONFLICT DO UPDATE keyed on the
// upstream identifier, so re-syncing refreshes rows instead of duplicating.
package store

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"swedennu/internal/fetcher"
)

// DB is the subset of pgxpool.Pool the store needs; narrowed for testability.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	db DB
}

func New(db DB) *Store {
	return &Store{db: db}
}

// marshal renders v as JSON for jsonb columns; "{}" on failure.
func marshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// nullableTime converts a *time.Time to an interface{} that pgx maps to NULL when nil.
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// nullableFloat64 converts a *float64 to an interface{} that pgx maps to NULL when nil.
func nullableFloat64(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// nullableString maps "" to NULL.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---- Access log ------------------------------------------------------------

// LogAccess inserts one row into access_log for visitor analytics.
// It is intentionally fire-and-forget: errors are silently dropped so a
// logging failure never disrupts the actual HTTP response.
func (s *Store) LogAccess(ip, method, path string, status, durationMs int, userAgent, referer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.db.Exec(ctx, //nolint:errcheck
		`INSERT INTO access_log (ip, method, path, status, duration_ms, user_agent, referer)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ip, method, path, status, durationMs, userAgent, referer,
	)
}

// ---- Sync log --------------------------------------------------------------

// LogSyncStart inserts a new in-progress row and returns its ID so
// LogSyncDone can close it out after the fetch completes.
func (s *Store) LogSyncStart(ctx context.Context, category, source string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO sync_log (category, source) VALUES ($1, $2) RETURNING id`,
		category, source,
	).Scan(&id)
	return id, err
}

// LogSyncDone stamps the finished_at time, records how many rows were upserted,
// and stores any error message. A blank errMsg means success.
func (s *Store) LogSyncDone(ctx context.Context, id int64, upserted int, errMsg string) {
	var errVal any
	if errMsg != "" {
		errVal = errMsg
	}
	s.db.Exec(ctx, //nolint:errcheck
		`UPDATE sync_log SET finished_at=NOW(), upserted=$2, error_msg=$3 WHERE id=$1`,
		id, upserted, errVal,
	)
}

// LastSuccessfulSync returns the finished_at time of the most recent sync run
// for the given category that completed without error, or nil if none exists.
func (s *Store) LastSuccessfulSync(ctx context.Context, category string) (*time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(ctx,
		`SELECT finished_at FROM sync_log
		 WHERE category=$1 AND error_msg IS NULL AND finished_at IS NOT NULL
		 ORDER BY finished_at DESC LIMIT 1`,
		category,
	).Scan(&t)
	if err != nil {
		return nil, err // includes pgx.ErrNoRows — caller treats nil as "never synced"
	}
	return &t, nil
}

// ---- SMHI observations -------------------------------------------------------

// UpsertSMHIObservations inserts or updates the latest reading per station.
// The upsert key is (station_key, parameter). Temperature and measured_at are
// nullable — stations can appear in the station set without a current reading.
func (s *Store) UpsertSMHIObservations(ctx context.Context, items []fetcher.SMHIObservation) (int, error) {
	count := 0
	for _, item := range items {
		var measuredAt any
		if item.MeasuredAt != nil {
			measuredAt = time.UnixMilli(*item.MeasuredAt)
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO smhi_observations
				(station_key, parameter, name, county_code, lat, lon, height_m, temperature, quality, measured_at, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
			ON CONFLICT (station_key, parameter) DO UPDATE SET
				name        = EXCLUDED.name,
				county_code = EXCLUDED.county_code,
				lat         = EXCLUDED.lat,
				lon         = EXCLUDED.lon,
				height_m    = EXCLUDED.height_m,
				temperature = EXCLUDED.temperature,
				quality     = EXCLUDED.quality,
				measured_at = EXCLUDED.measured_at,
				raw         = EXCLUDED.raw,
				synced_at   = NOW()`,
			item.StationKey, item.Parameter, item.Name, nullableString(item.CountyCode),
			item.Lat, item.Lon, item.HeightM, nullableFloat64(item.Temperature),
			nullableString(item.Quality), measuredAt, marshal(item.Raw),
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ---- SCB stats -----------------------------------------------------------------

// UpsertSCBStats stores per-county annual statistics from SCB. countyCodes is
// aligned with items (resolved by the sync job via geo.CountyBySCBCode).
// ~2,600 rows per sync.
func (s *Store) UpsertSCBStats(ctx context.Context, items []fetcher.SCBStat, countyCodes []string) (int, error) {
	count := 0
	for i, item := range items {
		county := ""
		if i < len(countyCodes) {
			county = countyCodes[i]
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO scb_stats (table_id, metric, region_code, county_code, year, value, unit, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'scb')
			ON CONFLICT (table_id, metric, region_code, year) DO UPDATE SET
				county_code = EXCLUDED.county_code,
				value       = EXCLUDED.value,
				unit        = EXCLUDED.unit,
				synced_at   = NOW()`,
			item.TableID, item.Metric, item.RegionCode, county, item.Year, item.Value, item.Unit,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ---- Kolada stats ----------------------------------------------------------------

// UpsertKolada stores municipal KPI values; the upsert key is
// (kpi_id, municipality_id, period). countyCodes is aligned with items.
// ~27K single-row Execs per sync (~30-60s) — acceptable at the 12h cadence;
// switch to pgx.Batch or CopyFrom if it ever becomes a problem.
func (s *Store) UpsertKolada(ctx context.Context, items []fetcher.KoladaStat, countyCodes []string) (int, error) {
	count := 0
	for i, item := range items {
		county := ""
		if i < len(countyCodes) {
			county = countyCodes[i]
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO kolada_stats (kpi_id, municipality_id, municipality, county_code, period, title, unit, value, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'kolada')
			ON CONFLICT (kpi_id, municipality_id, period) DO UPDATE SET
				municipality = EXCLUDED.municipality,
				county_code  = EXCLUDED.county_code,
				title        = EXCLUDED.title,
				unit         = EXCLUDED.unit,
				value        = EXCLUDED.value,
				synced_at    = NOW()`,
			item.KPIID, item.MunicipalityID, item.Municipality, county, item.Period,
			item.Title, item.Unit, nullableFloat64(item.Value),
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ---- Police events -----------------------------------------------------------

// UpsertNationalPoliceEvents stores police events for all of Sweden.
// countyCode is resolved by the caller (sync job) via the geo package; it is
// passed per item through the CountyCodes slice aligned with items.
func (s *Store) UpsertNationalPoliceEvents(ctx context.Context, items []fetcher.PoliceEvent, countyCodes []string) (int, error) {
	count := 0
	for i, item := range items {
		lat, lon := ParseGPS(item.Location.GPS)
		eventTime := ParsePoliceDateTime(item.DateTime)
		var county any
		if i < len(countyCodes) && countyCodes[i] != "" {
			county = countyCodes[i]
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO national_police_events
				(external_id, source, event_time, name, summary, url, type, location_name, county_code, lat, lon, raw)
			VALUES ($1, 'polisen', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
			ON CONFLICT (external_id) DO UPDATE SET
				event_time    = EXCLUDED.event_time,
				name          = EXCLUDED.name,
				summary       = EXCLUDED.summary,
				url           = EXCLUDED.url,
				type          = EXCLUDED.type,
				location_name = EXCLUDED.location_name,
				county_code   = EXCLUDED.county_code,
				lat           = EXCLUDED.lat,
				lon           = EXCLUDED.lon,
				raw           = EXCLUDED.raw,
				synced_at     = NOW()`,
			item.ID, nullableTime(eventTime), item.Name, item.Summary,
			item.URL, item.Type, item.Location.Name, county, lat, lon, marshal(item),
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ParseGPS splits a "lat,lon" string into float64 values. Returns 0,0 if malformed.
// Exported because the sync job in main needs coordinates for county resolution
// before handing events to the store.
func ParseGPS(gps string) (lat, lon float64) {
	parts := strings.SplitN(gps, ",", 2)
	if len(parts) != 2 {
		return
	}
	lat, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return
}

// ---- Riksdagen ---------------------------------------------------------------

// UpsertRiksdagenMembers replaces all current MP rows.
func (s *Store) UpsertRiksdagenMembers(ctx context.Context, items []fetcher.RiksdagenMember) (int, error) {
	count := 0
	for _, m := range items {
		var birthYear any
		if m.BirthYear > 0 {
			birthYear = m.BirthYear
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO riksdagen_members (id, first_name, last_name, party, valkrets, gender, birth_year, photo_url, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				first_name = EXCLUDED.first_name,
				last_name  = EXCLUDED.last_name,
				party      = EXCLUDED.party,
				valkrets   = EXCLUDED.valkrets,
				gender     = EXCLUDED.gender,
				birth_year = EXCLUDED.birth_year,
				photo_url  = EXCLUDED.photo_url,
				status     = EXCLUDED.status,
				synced_at  = NOW()`,
			m.ID, m.FirstName, m.LastName, m.Party, m.Valkrets,
			m.Gender, birthYear, nullableString(m.PhotoURL), m.Status,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpsertRiksdagenVotes stores aggregated vote results.
func (s *Store) UpsertRiksdagenVotes(ctx context.Context, items []fetcher.RiksdagenVoteResult) (int, error) {
	count := 0
	for _, v := range items {
		var datum any
		if v.Datum != "" {
			datum = v.Datum
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO riksdagen_vote_results
				(votering_id, rm, dok_id, beteckning, title, datum, party, ja, nej, avstar, franvarande)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (votering_id, party) DO UPDATE SET
				rm          = EXCLUDED.rm,
				dok_id      = EXCLUDED.dok_id,
				beteckning  = EXCLUDED.beteckning,
				title       = EXCLUDED.title,
				datum       = EXCLUDED.datum,
				ja          = EXCLUDED.ja,
				nej         = EXCLUDED.nej,
				avstar      = EXCLUDED.avstar,
				franvarande = EXCLUDED.franvarande,
				synced_at   = NOW()`,
			v.VoteringID, v.RM, v.DokID, v.Beteckning, v.Title,
			datum, v.Party, v.Ja, v.Nej, v.Avstar, v.Franvarande,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ---- Domstolsverket ---------------------------------------------------------

// UpsertDomstolStats stores court case-volume and processing-time data.
func (s *Store) UpsertDomstolStats(ctx context.Context, items []fetcher.DomstolStat) (int, error) {
	count := 0
	for _, item := range items {
		var val any
		if item.Value != 0 {
			val = item.Value
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO domstol_stats
				(table_id, court_code, court_name, county_code, metric, year, value, unit, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'domstolsverket')
			ON CONFLICT (table_id, court_code, metric, year) DO UPDATE SET
				court_name  = EXCLUDED.court_name,
				county_code = EXCLUDED.county_code,
				value       = EXCLUDED.value,
				unit        = EXCLUDED.unit,
				synced_at   = NOW()`,
			item.TableID, item.CourtCode, item.CourtName, item.CountyCode,
			item.Metric, item.Year, val, item.Unit,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ---- Folkhälsomyndigheten ---------------------------------------------------

// UpsertFolkhalsaStats stores public-health indicators from FOHM.
// countyCodes is aligned with items (resolved by the sync job via geo.CountyBySCBCode).
func (s *Store) UpsertFolkhalsaStats(ctx context.Context, items []fetcher.FolkhalsaStat, countyCodes []string) (int, error) {
	count := 0
	for i, item := range items {
		county := ""
		if i < len(countyCodes) {
			county = countyCodes[i]
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO folkhalsa_stats (metric, region_code, county_code, sex, period, value, unit, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'fohm')
			ON CONFLICT (metric, region_code, sex, period) DO UPDATE SET
				county_code = EXCLUDED.county_code,
				value       = EXCLUDED.value,
				unit        = EXCLUDED.unit,
				synced_at   = NOW()`,
			item.Metric, item.RegionCode, county, item.Sex, item.Period, item.Value, item.Unit,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpsertValResults upserts riksdag election party results.
func (s *Store) UpsertValResults(ctx context.Context, items []fetcher.ValResult) (int, error) {
	count := 0
	for _, item := range items {
		_, err := s.db.Exec(ctx, `
			INSERT INTO val_results (election_year, area_code, area_name, party_code, party_name, votes, vote_share, mandates, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'valmyndigheten')
			ON CONFLICT (election_year, area_code, party_code) DO UPDATE SET
				area_name  = EXCLUDED.area_name,
				party_name = EXCLUDED.party_name,
				votes      = EXCLUDED.votes,
				vote_share = EXCLUDED.vote_share,
				mandates   = EXCLUDED.mandates,
				synced_at  = NOW()`,
			item.Year, item.AreaCode, item.AreaName, item.PartyCode, item.PartyName,
			item.Votes, item.VoteShare, item.Mandates,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpsertValTurnout upserts election turnout per area.
func (s *Store) UpsertValTurnout(ctx context.Context, items []fetcher.ValTurnout) (int, error) {
	count := 0
	for _, item := range items {
		_, err := s.db.Exec(ctx, `
			INSERT INTO val_turnout (election_year, area_code, area_name, eligible_voters, votes_cast, turnout, source)
			VALUES ($1, $2, $3, $4, $5, $6, 'valmyndigheten')
			ON CONFLICT (election_year, area_code) DO UPDATE SET
				area_name       = EXCLUDED.area_name,
				eligible_voters = EXCLUDED.eligible_voters,
				votes_cast      = EXCLUDED.votes_cast,
				turnout         = EXCLUDED.turnout,
				synced_at       = NOW()`,
			item.Year, item.AreaCode, item.AreaName,
			item.EligibleVoters, item.VotesCast, item.Turnout,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ---- Air quality -------------------------------------------------------------

// UpsertAirQuality inserts or updates the latest reading per station and parameter.
func (s *Store) UpsertAirQuality(ctx context.Context, items []fetcher.AirQualityObservation) (int, error) {
	count := 0
	for _, item := range items {
		var measuredAt any
		if item.MeasuredAt != nil {
			measuredAt = time.UnixMilli(*item.MeasuredAt)
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO air_quality
				(station_key, parameter, param_name, name, county_code, lat, lon, height_m, value, quality, measured_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (station_key, parameter) DO UPDATE SET
				param_name  = EXCLUDED.param_name,
				name        = EXCLUDED.name,
				county_code = EXCLUDED.county_code,
				lat         = EXCLUDED.lat,
				lon         = EXCLUDED.lon,
				height_m    = EXCLUDED.height_m,
				value       = EXCLUDED.value,
				quality     = EXCLUDED.quality,
				measured_at = EXCLUDED.measured_at,
				synced_at   = NOW()`,
			item.StationKey, item.Parameter, item.ParamName, item.Name,
			nullableString(item.CountyCode), item.Lat, item.Lon, item.HeightM,
			nullableFloat64(item.Value), nullableString(item.Quality), measuredAt,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpsertBRACrimes stores reported-crime statistics scraped from BRÅ's Sol
// database (~87K rows on a full 30-year backfill; the job runs weekly so the
// row-by-row upsert cost is acceptable, same trade-off as UpsertKolada).
func (s *Store) UpsertBRACrimes(ctx context.Context, items []fetcher.BRACrime) (int, error) {
	count := 0
	for _, item := range items {
		var cnt, per any
		if item.Count != nil {
			cnt = *item.Count
		}
		if item.Per100k != nil {
			per = *item.Per100k
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO bra_crimes
				(year, category_id, slug, category, kommun_id, kommun, county_code, count, per_100k, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'bra')
			ON CONFLICT (year, category_id, kommun_id) DO UPDATE SET
				slug        = EXCLUDED.slug,
				category    = EXCLUDED.category,
				kommun      = EXCLUDED.kommun,
				county_code = EXCLUDED.county_code,
				count       = EXCLUDED.count,
				per_100k    = EXCLUDED.per_100k,
				synced_at   = NOW()`,
			item.Year, item.CategoryID, item.Slug, item.Category,
			item.KommunID, item.Kommun, item.CountyCode, cnt, per,
		)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ParsePoliceDateTime parses the police API's datetime format: "2026-05-20 17:11:17 +02:00"
func ParsePoliceDateTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02 15:04:05 -07:00", s)
	if err != nil {
		return nil
	}
	return &t
}

// ---- Trafikverket ------------------------------------------------------------

// UpsertTrafik replaces all traffic situations with the freshly fetched set.
// Since situations are real-time (active events only), we truncate then insert
// rather than upsert, so stale closed situations disappear automatically.
func (s *Store) UpsertTrafik(ctx context.Context, items []fetcher.TrafikSituation) (int, error) {
	if _, err := s.db.Exec(ctx, `TRUNCATE TABLE trafik_situations`); err != nil {
		return 0, err
	}
	for _, it := range items {
		var endTime any
		if it.EndTime != nil {
			endTime = *it.EndTime
		}
		var lat, lon any
		if it.Lat != 0 || it.Lon != 0 {
			lat = it.Lat
			lon = it.Lon
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO trafik_situations
			  (external_id, site_type, severity, title, description, road_number,
			   county_codes, lat, lon, start_time, end_time)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (external_id) DO UPDATE SET
			  site_type=EXCLUDED.site_type, severity=EXCLUDED.severity,
			  title=EXCLUDED.title, description=EXCLUDED.description,
			  road_number=EXCLUDED.road_number, county_codes=EXCLUDED.county_codes,
			  lat=EXCLUDED.lat, lon=EXCLUDED.lon,
			  start_time=EXCLUDED.start_time, end_time=EXCLUDED.end_time,
			  synced_at=NOW()`,
			it.ID, it.SiteType, it.Severity, it.Title, it.Description, it.RoadNumber,
			it.CountyCodes, lat, lon, it.StartTime, endTime,
		)
		if err != nil {
			return 0, err
		}
	}
	return len(items), nil
}

// UpsertTrafikFlow replaces all traffic flow measurements with the latest fetch.
// Measurements are per-lane and update every minute; truncate+insert keeps the table current.
func (s *Store) UpsertTrafikFlow(ctx context.Context, items []fetcher.TrafikFlowItem) (int, error) {
	if _, err := s.db.Exec(ctx, `TRUNCATE TABLE trafik_flow`); err != nil {
		return 0, err
	}
	for _, it := range items {
		var measuredAt any
		if !it.MeasuredAt.IsZero() {
			measuredAt = it.MeasuredAt
		}
		_, err := s.db.Exec(ctx, `
			INSERT INTO trafik_flow (site_id, county_code, flow_rate, avg_speed, lane, measured_at)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			it.SiteID, it.CountyCode, it.FlowRate, it.AvgSpeed, it.Lane, measuredAt,
		)
		if err != nil {
			return 0, err
		}
	}
	return len(items), nil
}

// UpsertRiksbank inserts or updates Riksbanken observations (policy rate, exchange rates).
func (s *Store) UpsertRiksbank(ctx context.Context, items []fetcher.RiksbankObs) (int, error) {
	n := 0
	for _, it := range items {
		_, err := s.db.Exec(ctx, `
			INSERT INTO riksbank_obs (series_id, date, value)
			VALUES ($1,$2,$3)
			ON CONFLICT (series_id, date) DO UPDATE SET value=$3, synced_at=NOW()`,
			it.SeriesID, it.Date, it.Value,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertElpris inserts or updates 15-minute electricity spot prices for SE1–SE4 from elprisetjustnu.se.
func (s *Store) UpsertElpris(ctx context.Context, items []fetcher.ElprisObs) (int, error) {
	n := 0
	for _, it := range items {
		_, err := s.db.Exec(ctx, `
			INSERT INTO elpris (area, time_start, sek_per_kwh, eur_per_kwh)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (area, time_start) DO UPDATE SET sek_per_kwh=$3, eur_per_kwh=$4, synced_at=NOW()`,
			it.Area, it.TimeStart, it.SEKPerKWh, it.EURPerKWh,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertSkolaSchools upserts active school counts per municipality from the Skolverket registry.
func (s *Store) UpsertSkolaSchools(ctx context.Context, schools []fetcher.SkolaSchools) (int, error) {
	n := 0
	for _, r := range schools {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO skola_schools (municipality_code, school_count)
			 VALUES ($1, $2)
			 ON CONFLICT (municipality_code) DO UPDATE
			   SET school_count = EXCLUDED.school_count, synced_at = NOW()`,
			r.MunicipalityCode, r.SchoolCount,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertSalsaStats upserts per-municipality SALSA aggregates from Skolverket planned-educations API.
func (s *Store) UpsertSalsaStats(ctx context.Context, rows []fetcher.SalsaMunicipality) (int, error) {
	n := 0
	for _, r := range rows {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO skola_salsa
			   (municipality_code, school_count, avg_actual, avg_calculated, avg_deviation,
			    req_reached_actual, req_reached_dev, parents_education)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			 ON CONFLICT (municipality_code) DO UPDATE SET
			   school_count       = EXCLUDED.school_count,
			   avg_actual         = EXCLUDED.avg_actual,
			   avg_calculated     = EXCLUDED.avg_calculated,
			   avg_deviation      = EXCLUDED.avg_deviation,
			   req_reached_actual = EXCLUDED.req_reached_actual,
			   req_reached_dev    = EXCLUDED.req_reached_dev,
			   parents_education  = EXCLUDED.parents_education,
			   synced_at          = NOW()`,
			r.MunicipalityCode, r.SchoolCount,
			r.AvgActual, r.AvgCalculated, r.AvgDeviation,
			r.ReqReachedActual, r.ReqReachedDev, r.ParentsEducation,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertNationalGrStats upserts one row per period for national grundskola averages.
func (s *Store) UpsertNationalGrStats(ctx context.Context, r *fetcher.NationalGrStats) (int, error) {
	if _, err := s.db.Exec(ctx,
		`INSERT INTO skola_national
		   (period, students_per_teacher, certified_teachers_pct, ratio_passed_9th,
		    avg_merit_9th, eligible_yr_pct, eligible_es_pct,
		    ratio_passed_6th, test_sve_6, test_eng_6, test_ma_6,
		    test_sve_9, test_eng_9, test_ma_9)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 ON CONFLICT (period) DO UPDATE SET
		   students_per_teacher   = EXCLUDED.students_per_teacher,
		   certified_teachers_pct = EXCLUDED.certified_teachers_pct,
		   ratio_passed_9th       = EXCLUDED.ratio_passed_9th,
		   avg_merit_9th          = EXCLUDED.avg_merit_9th,
		   eligible_yr_pct        = EXCLUDED.eligible_yr_pct,
		   eligible_es_pct        = EXCLUDED.eligible_es_pct,
		   ratio_passed_6th       = EXCLUDED.ratio_passed_6th,
		   test_sve_6             = EXCLUDED.test_sve_6,
		   test_eng_6             = EXCLUDED.test_eng_6,
		   test_ma_6              = EXCLUDED.test_ma_6,
		   test_sve_9             = EXCLUDED.test_sve_9,
		   test_eng_9             = EXCLUDED.test_eng_9,
		   test_ma_9              = EXCLUDED.test_ma_9,
		   synced_at              = NOW()`,
		r.Period, r.StudentsPerTeacher, r.CertifiedTeachersPct, r.RatioPassed9th,
		r.AvgMerit9th, r.EligibleYRPct, r.EligibleESPct,
		r.RatioPassed6th, r.TestSve6, r.TestEng6, r.TestMa6,
		r.TestSve9, r.TestEng9, r.TestMa9,
	); err != nil {
		return 0, err
	}
	return 1, nil
}

// ---- Socialstyrelsen ---------------------------------------------------------

// UpsertSSDeaths writes cause-of-death rows from Socialstyrelsen.
// Keyed on (diagnos_id, age_id, year) — safe to call repeatedly.
func (s *Store) UpsertSSDeaths(ctx context.Context, rows []fetcher.SSDeath) (int, error) {
	n := 0
	for _, r := range rows {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO ss_deaths (diagnos_id, diagnos_name, age_id, year, value)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (diagnos_id, age_id, year) DO UPDATE
			  SET diagnos_name = EXCLUDED.diagnos_name,
			      value        = EXCLUDED.value,
			      synced_at    = NOW()`,
			r.DiagnosID, r.DiagnosName, r.AgeID, r.Year, r.Value,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertSSDental writes dental visit rate rows from Socialstyrelsen.
// Keyed on (region_id, year).
func (s *Store) UpsertSSDental(ctx context.Context, rows []fetcher.SSDental) (int, error) {
	n := 0
	for _, r := range rows {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO ss_dental (region_id, region_name, county_code, year, value)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (region_id, year) DO UPDATE
			  SET region_name = EXCLUDED.region_name,
			      county_code = EXCLUDED.county_code,
			      value       = EXCLUDED.value,
			      synced_at   = NOW()`,
			r.RegionID, r.RegionName, r.CountyCode, r.Year, r.Value,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertSSBirths writes births/newborn indicator rows from Socialstyrelsen.
// Keyed on (variabel_id, region_id, year).
func (s *Store) UpsertSSBirths(ctx context.Context, rows []fetcher.SSBirth) (int, error) {
	n := 0
	for _, r := range rows {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO ss_births (variabel_id, variabel_name, region_id, region_name, county_code, year, value)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (variabel_id, region_id, year) DO UPDATE
			  SET variabel_name = EXCLUDED.variabel_name,
			      region_name   = EXCLUDED.region_name,
			      county_code   = EXCLUDED.county_code,
			      value         = EXCLUDED.value,
			      synced_at     = NOW()`,
			r.VariabelID, r.VariabelName, r.RegionID, r.RegionName, r.CountyCode, r.Year, r.Value,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ReplaceArbetsmarknad refreshes the arbetsmarknad snapshot tables (TRUNCATE + INSERT).
// Follows the same pattern as trafik — no explicit transaction, brief empty window is acceptable.
func (s *Store) ReplaceArbetsmarknad(ctx context.Context, jobs []fetcher.AFJobCount, fields []fetcher.AFField) (int, error) {
	if _, err := s.db.Exec(ctx, `TRUNCATE TABLE arbetsmarknad_jobs, arbetsmarknad_fields`); err != nil {
		return 0, err
	}
	n := 0
	for _, j := range jobs {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO arbetsmarknad_jobs (county_code, region_code, name, total_ads) VALUES ($1,$2,$3,$4)`,
			j.CountyCode, j.RegionCode, j.Name, j.TotalAds,
		); err != nil {
			return n, err
		}
		n++
	}
	for _, f := range fields {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO arbetsmarknad_fields (term, concept_id, count) VALUES ($1,$2,$3)`,
			f.Term, f.ConceptID, f.Count,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertFKSjukfall writes Försäkringskassan F43 stress sjukfall rows.
func (s *Store) UpsertFKSjukfall(ctx context.Context, rows []fetcher.FKSjukfall) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO fk_sjukfall (county_code, lan_kod, year, month, diagnos, andel, antal, synced_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
			ON CONFLICT (county_code, year, month, diagnos)
			DO UPDATE SET andel=EXCLUDED.andel, antal=EXCLUDED.antal, synced_at=NOW()`,
			r.CountyCode, r.LanKod, r.Year, r.Month, r.Diagnos, r.Andel, r.Antal,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertEnergiMix writes national electricity production by source rows.
func (s *Store) UpsertEnergiMix(ctx context.Context, rows []fetcher.EnergiMixRow) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO energimix_stats (year, source, twh, synced_at)
			VALUES ($1,$2,$3,NOW())
			ON CONFLICT (year, source)
			DO UPDATE SET twh=EXCLUDED.twh, synced_at=NOW()`,
			r.Year, r.Source, r.TWh,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertSkattesats writes municipality tax-rate rows from Skatteverket.
func (s *Store) UpsertSkattesats(ctx context.Context, rows []fetcher.SkattRow) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO skattesats (year, municipality, lan_kod, county_code, kommunal_skatt, landstings_skatt, begravnings_avgift, summa_exkl_kyrka, synced_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
			ON CONFLICT (year, municipality)
			DO UPDATE SET kommunal_skatt=EXCLUDED.kommunal_skatt, landstings_skatt=EXCLUDED.landstings_skatt,
			              begravnings_avgift=EXCLUDED.begravnings_avgift, summa_exkl_kyrka=EXCLUDED.summa_exkl_kyrka, synced_at=NOW()`,
			r.Year, r.Municipality, r.LanKod, r.CountyCode, r.KommunalSkatt, r.LandstingsSkatt, r.BegravningsAvgift, r.SummaExklKyrka,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertForetagsoverskott writes corporate profit per municipality rows.
func (s *Store) UpsertForetagsoverskott(ctx context.Context, rows []fetcher.ForetagsRow) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO foretagsoverskott (year, municipality, belopp_sek, antal, synced_at)
			VALUES ($1,$2,$3,$4,NOW())
			ON CONFLICT (year, municipality)
			DO UPDATE SET belopp_sek=EXCLUDED.belopp_sek, antal=EXCLUDED.antal, synced_at=NOW()`,
			r.Year, r.Municipality, r.BeloppSEK, r.Antal,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertKapitalinkomst writes capital income per county rows.
func (s *Store) UpsertKapitalinkomst(ctx context.Context, rows []fetcher.KapitalRow) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO kapitalinkomst (year, county_name, statistikterm, belopp_sek, antal, synced_at)
			VALUES ($1,$2,$3,$4,$5,NOW())
			ON CONFLICT (year, county_name, statistikterm)
			DO UPDATE SET belopp_sek=EXCLUDED.belopp_sek, antal=EXCLUDED.antal, synced_at=NOW()`,
			r.Year, r.CountyName, r.Statistikterm, r.BeloppSEK, r.Antal,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertSkattPerPerson writes actual taxes paid (median/mean) per municipality.
func (s *Store) UpsertSkattPerPerson(ctx context.Context, rows []fetcher.SkattPerPersonRow) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO skatt_per_person (year, municipality, typ_av_skatt, belopp_sek, median_kr, medel_kr, antal, synced_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
			ON CONFLICT (year, municipality, typ_av_skatt)
			DO UPDATE SET belopp_sek=EXCLUDED.belopp_sek, median_kr=EXCLUDED.median_kr,
			              medel_kr=EXCLUDED.medel_kr, antal=EXCLUDED.antal, synced_at=NOW()`,
			r.Year, r.Municipality, r.TypAvSkatt, r.BeloppSEK, r.MedianKr, r.MedelKr, r.Antal,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UpsertKommunalskattInkomstniva writes tax amounts by income bracket and statistikterm.
func (s *Store) UpsertKommunalskattInkomstniva(ctx context.Context, rows []fetcher.KommunalskattNivaRow) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO kommunalskatt_inkomstniva (year, inkomstniva, statistikterm, belopp_sek, antal, synced_at)
			VALUES ($1,$2,$3,$4,$5,NOW())
			ON CONFLICT (year, inkomstniva, statistikterm)
			DO UPDATE SET belopp_sek=EXCLUDED.belopp_sek, antal=EXCLUDED.antal, synced_at=NOW()`,
			r.Year, r.Inkomstniva, r.Statistikterm, r.BeloppSEK, r.Antal,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ReplaceRiksdagenKalender swaps in the current upcoming-events snapshot.
// TRUNCATE+INSERT: past events fall out of the 4-week fetch window and must
// disappear from the table too.
func (s *Store) ReplaceRiksdagenKalender(ctx context.Context, rows []fetcher.RiksdagenEvent) (int, error) {
	if _, err := s.db.Exec(ctx, `TRUNCATE riksdagen_kalender`); err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO riksdagen_kalender (dok_id, datum, typ, subtyp, titel, undertitel, organ, plats, synced_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
			ON CONFLICT (dok_id) DO NOTHING`,
			r.DokID, r.Datum, r.Typ, r.Subtyp, r.Titel, r.Undertitel, r.Organ, r.Plats,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ListKoladaMunicipalities returns each municipality's proper-case name and
// county code from the Kolada table — the canonical 290-name list used to
// drive per-municipality queries against other sources (RAÄ WFS).
func (s *Store) ListKoladaMunicipalities(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT municipality, county_code FROM kolada_stats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, cc string
		if err := rows.Scan(&name, &cc); err != nil {
			return nil, err
		}
		out[name] = cc
	}
	return out, rows.Err()
}

// UpsertFornlamningar writes fornlämning counts per municipality.
func (s *Store) UpsertFornlamningar(ctx context.Context, rows []fetcher.FornlamningCount, countyByKommun map[string]string) (int, error) {
	n := 0
	for _, r := range rows {
		_, err := s.db.Exec(ctx, `
			INSERT INTO raa_fornlamningar (kommun, county_code, antal, synced_at)
			VALUES ($1,$2,$3,NOW())
			ON CONFLICT (kommun)
			DO UPDATE SET county_code=EXCLUDED.county_code, antal=EXCLUDED.antal, synced_at=NOW()`,
			r.Kommun, countyByKommun[r.Kommun], r.Antal,
		)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
