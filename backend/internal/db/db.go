// Package db handles the database connection and schema migrations.
package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx connection pool and verifies the connection with a ping.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return pool, nil
}

// Migrate applies the schema to the database. All statements use CREATE TABLE
// IF NOT EXISTS so this is safe to call on every startup — it is a no-op when
// the schema is already in place and adds new tables if the code has grown.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	for _, s := range []string{syncLogSchema, accessLogSchema, smhiSchema, nationalPoliceSchema, scbSchema, koladaSchema, riksdagenSchema, folkhalsaSchema, domstolSchema, valSchema, valSeedSchema, airQualitySchema, braSchema, trafikSchema, trafikFlowSchema, riksbankSchema, elprisSchema, arbetsmarknadSchema, skolaSchema, socialstyrelsensSchema, energimixSchema, skattesatsSchema, foretagsoverskottSchema, kapitalinkomstSchema, skattPerPersonSchema, kommunalskattInkomstnivåSchema, riksdagenKalenderSchema, raaFornlamningarSchema} {
		if _, err := pool.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// syncLogSchema records every import run so operators can see when data was
// last refreshed and whether any category failed.
const syncLogSchema = `
CREATE TABLE IF NOT EXISTS sync_log (
    id          BIGSERIAL   PRIMARY KEY,
    category    TEXT        NOT NULL,
    source      TEXT        NOT NULL,
    upserted    INT         NOT NULL DEFAULT 0,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    error_msg   TEXT         -- NULL means the sync succeeded
);
CREATE INDEX IF NOT EXISTS sync_log_category_time_idx ON sync_log (category, started_at DESC);
`

// accessLogSchema stores one row per HTTP request for visitor analytics.
const accessLogSchema = `
CREATE TABLE IF NOT EXISTS access_log (
    id          BIGSERIAL    PRIMARY KEY,
    ip          TEXT         NOT NULL,
    method      TEXT         NOT NULL,
    path        TEXT         NOT NULL,
    status      INT          NOT NULL,
    duration_ms INT          NOT NULL,
    user_agent  TEXT,
    referer     TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS access_log_created_idx ON access_log (created_at DESC);
CREATE INDEX IF NOT EXISTS access_log_ip_idx      ON access_log (ip);
`

// smhiSchema stores the latest observation per SMHI station and parameter.
//
// Design decisions:
//
//   - UNIQUE (station_key, parameter) — the upsert key. parameter 1 = air
//     temperature; the column future-proofs for precipitation (5), wind (4),
//     snow depth (8) and pressure (9) without schema changes.
//
//   - county_code — two-letter code ('ab'..'bd') derived at sync time from the
//     station's coordinates via point-in-polygon; nearest-centroid fallback
//     covers offshore lighthouses and buoys.
//
//   - temperature is nullable — a station can appear in the station set without
//     a reading for the latest hour; keeping the row preserves the station list.
const smhiSchema = `
CREATE TABLE IF NOT EXISTS smhi_observations (
    id          BIGSERIAL        PRIMARY KEY,
    station_key TEXT             NOT NULL,
    parameter   INT              NOT NULL DEFAULT 1,
    name        TEXT             NOT NULL,
    county_code TEXT,
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    height_m    DOUBLE PRECISION,
    temperature NUMERIC(6,2),
    quality     TEXT,
    measured_at TIMESTAMPTZ,
    raw         JSONB            NOT NULL DEFAULT '{}',
    synced_at   TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    UNIQUE (station_key, parameter)
);
CREATE INDEX IF NOT EXISTS smhi_obs_county_idx   ON smhi_observations (county_code);
CREATE INDEX IF NOT EXISTS smhi_obs_measured_idx ON smhi_observations (measured_at DESC);
`

// scbSchema stores per-county annual statistics from SCB PX-Web tables.
// Generic across tables: the upsert key is (table_id, metric, region_code, year).
// county_code is derived at sync time from region_code via geo.CountyBySCBCode.
const scbSchema = `
CREATE TABLE IF NOT EXISTS scb_stats (
    id          BIGSERIAL    PRIMARY KEY,
    table_id    TEXT         NOT NULL,
    metric      TEXT         NOT NULL,
    region_code TEXT         NOT NULL,
    county_code TEXT         NOT NULL,
    year        INT          NOT NULL,
    value       NUMERIC(16,2),
    unit        TEXT         NOT NULL DEFAULT '',
    source      TEXT         NOT NULL DEFAULT 'scb',
    synced_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (table_id, metric, region_code, year)
);
CREATE INDEX IF NOT EXISTS scb_stats_county_idx ON scb_stats (county_code, year DESC);
CREATE INDEX IF NOT EXISTS scb_stats_table_idx  ON scb_stats (table_id, metric, year DESC);
`

// koladaSchema stores annual KPIs for all 290 municipalities (gender "T" only).
// county_code is derived from the first two digits of municipality_id (länskod).
const koladaSchema = `
CREATE TABLE IF NOT EXISTS kolada_stats (
    id              BIGSERIAL    PRIMARY KEY,
    kpi_id          TEXT         NOT NULL,
    municipality_id TEXT         NOT NULL,
    municipality    TEXT         NOT NULL,
    county_code     TEXT         NOT NULL,
    period          INT          NOT NULL,
    title           TEXT         NOT NULL,
    unit            TEXT         NOT NULL,
    value           NUMERIC(14,4),
    source          TEXT         NOT NULL DEFAULT 'kolada',
    synced_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (kpi_id, municipality_id, period)
);
CREATE INDEX IF NOT EXISTS kolada_stats_kpi_idx    ON kolada_stats (kpi_id, period DESC);
CREATE INDEX IF NOT EXISTS kolada_stats_county_idx ON kolada_stats (county_code);
`

// domstolSchema stores court statistics from Domstolsverket PX-Web.
// table_id: "courttype" (national case-volume trends) | "processing_tr" (tingsrätt processing times)
// metric:   "filed" | "determined" | "pending" | "proc_crim" | "proc_civil"
const domstolSchema = `
CREATE TABLE IF NOT EXISTS domstol_stats (
    id           BIGSERIAL    PRIMARY KEY,
    table_id     TEXT         NOT NULL,
    court_code   TEXT         NOT NULL,
    court_name   TEXT         NOT NULL,
    county_code  TEXT         NOT NULL DEFAULT '',
    metric       TEXT         NOT NULL,
    year         INT          NOT NULL,
    value        NUMERIC(12,2),
    unit         TEXT         NOT NULL DEFAULT '',
    source       TEXT         NOT NULL DEFAULT 'domstolsverket',
    synced_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (table_id, court_code, metric, year)
);
CREATE INDEX IF NOT EXISTS domstol_table_metric_idx ON domstol_stats (table_id, metric, year DESC);
CREATE INDEX IF NOT EXISTS domstol_county_idx       ON domstol_stats (county_code);
`

// folkhalsaSchema stores public health indicators from Folkhälsomyndigheten.
// metric: "lifeexp" | "smoking" | "alcohol"
// sex:    "total" | "women" | "men"
// period: year string "2025" or range "2021-2024"
const folkhalsaSchema = `
CREATE TABLE IF NOT EXISTS folkhalsa_stats (
    id           BIGSERIAL    PRIMARY KEY,
    metric       TEXT         NOT NULL,
    region_code  TEXT         NOT NULL,
    county_code  TEXT         NOT NULL,
    sex          TEXT         NOT NULL,
    period       TEXT         NOT NULL,
    value        NUMERIC(8,2) NOT NULL,
    unit         TEXT         NOT NULL DEFAULT '',
    source       TEXT         NOT NULL DEFAULT 'fohm',
    synced_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (metric, region_code, sex, period)
);
CREATE INDEX IF NOT EXISTS folkhalsa_metric_idx ON folkhalsa_stats (metric, period DESC);
CREATE INDEX IF NOT EXISTS folkhalsa_county_idx ON folkhalsa_stats (county_code);
`

// riksdagenSchema stores current MPs and aggregated vote results.
const riksdagenSchema = `
CREATE TABLE IF NOT EXISTS riksdagen_members (
    id          TEXT         PRIMARY KEY,  -- intressent_id
    first_name  TEXT         NOT NULL,
    last_name   TEXT         NOT NULL,
    party       TEXT         NOT NULL,
    valkrets    TEXT         NOT NULL,
    gender      TEXT         NOT NULL,
    birth_year  INT,
    photo_url   TEXT,
    status      TEXT         NOT NULL DEFAULT '',
    synced_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS riksdagen_members_party_idx ON riksdagen_members (party);

CREATE TABLE IF NOT EXISTS riksdagen_vote_results (
    id           BIGSERIAL    PRIMARY KEY,
    votering_id  TEXT         NOT NULL,
    rm           TEXT         NOT NULL,
    dok_id       TEXT         NOT NULL,
    beteckning   TEXT         NOT NULL DEFAULT '',
    title        TEXT         NOT NULL DEFAULT '',
    datum        DATE,
    party        TEXT         NOT NULL,
    ja           INT          NOT NULL DEFAULT 0,
    nej          INT          NOT NULL DEFAULT 0,
    avstar       INT          NOT NULL DEFAULT 0,
    franvarande  INT          NOT NULL DEFAULT 0,
    synced_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (votering_id, party)
);
CREATE INDEX IF NOT EXISTS riksdagen_votes_rm_idx ON riksdagen_vote_results (rm, datum DESC);
CREATE INDEX IF NOT EXISTS riksdagen_votes_vid_idx ON riksdagen_vote_results (votering_id);
`

// valSchema stores riksdag election results per valkrets and year.
// Historical national data (2002–2018) is seeded via valSeedSchema.
// 2022 valkrets data is fetched live from resultat.val.se.
const valSchema = `
CREATE TABLE IF NOT EXISTS val_results (
    id          BIGSERIAL    PRIMARY KEY,
    election_year INT        NOT NULL,
    area_code   TEXT         NOT NULL,  -- "riket" or valkrets slug
    area_name   TEXT         NOT NULL,
    party_code  TEXT         NOT NULL,  -- "S","M","SD","C","V","KD","L","MP","OVR"
    party_name  TEXT         NOT NULL DEFAULT '',
    votes       INT,
    vote_share  NUMERIC(5,2),           -- percent
    mandates    INT,
    source      TEXT         NOT NULL DEFAULT 'valmyndigheten',
    synced_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (election_year, area_code, party_code)
);
CREATE INDEX IF NOT EXISTS val_results_year_idx  ON val_results (election_year, area_code);
CREATE INDEX IF NOT EXISTS val_results_party_idx ON val_results (party_code, election_year);

CREATE TABLE IF NOT EXISTS val_turnout (
    id              BIGSERIAL   PRIMARY KEY,
    election_year   INT         NOT NULL,
    area_code       TEXT        NOT NULL,
    area_name       TEXT        NOT NULL,
    eligible_voters INT,
    votes_cast      INT,
    turnout         NUMERIC(5,2),
    source          TEXT        NOT NULL DEFAULT 'valmyndigheten',
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (election_year, area_code)
);
CREATE INDEX IF NOT EXISTS val_turnout_year_idx ON val_turnout (election_year);
`

// valSeedSchema inserts known national results for 2002–2018.
// Uses INSERT … WHERE NOT EXISTS so it is a no-op on subsequent startups.
const valSeedSchema = `
INSERT INTO val_results (election_year,area_code,area_name,party_code,party_name,votes,vote_share,mandates)
SELECT * FROM (VALUES
  (2022,'riket','Riket','S','Socialdemokraterna',1904640,30.33,107),
  (2022,'riket','Riket','M','Moderaterna',1267957,19.10,68),
  (2022,'riket','Riket','SD','Sverigedemokraterna',1362470,20.54,73),
  (2022,'riket','Riket','C','Centerpartiet',445088,6.71,24),
  (2022,'riket','Riket','V','Vänsterpartiet',447920,6.75,24),
  (2022,'riket','Riket','KD','Kristdemokraterna',354246,5.34,19),
  (2022,'riket','Riket','L','Liberalerna',313184,4.72,16),
  (2022,'riket','Riket','MP','Miljöpartiet',337065,5.08,18),
  (2018,'riket','Riket','S','Socialdemokraterna',1830386,28.26,100),
  (2018,'riket','Riket','M','Moderaterna',1284698,19.84,70),
  (2018,'riket','Riket','SD','Sverigedemokraterna',1135627,17.53,62),
  (2018,'riket','Riket','C','Centerpartiet',557500,8.61,31),
  (2018,'riket','Riket','V','Vänsterpartiet',518454,8.00,28),
  (2018,'riket','Riket','KD','Kristdemokraterna',409478,6.32,22),
  (2018,'riket','Riket','L','Liberalerna',355546,5.49,20),
  (2018,'riket','Riket','MP','Miljöpartiet',285899,4.41,16),
  (2014,'riket','Riket','S','Socialdemokraterna',1932711,31.01,113),
  (2014,'riket','Riket','M','Moderaterna',1453517,23.33,84),
  (2014,'riket','Riket','SD','Sverigedemokraterna',801178,12.86,49),
  (2014,'riket','Riket','C','Centerpartiet',380937,6.11,22),
  (2014,'riket','Riket','V','Vänsterpartiet',356331,5.72,21),
  (2014,'riket','Riket','KD','Kristdemokraterna',284806,4.57,16),
  (2014,'riket','Riket','L','Liberalerna',337773,5.42,19),
  (2014,'riket','Riket','MP','Miljöpartiet',429275,6.89,25),
  (2010,'riket','Riket','S','Socialdemokraterna',1827497,30.66,112),
  (2010,'riket','Riket','M','Moderaterna',1791766,30.06,107),
  (2010,'riket','Riket','SD','Sverigedemokraterna',339610,5.70,20),
  (2010,'riket','Riket','C','Centerpartiet',390804,6.56,23),
  (2010,'riket','Riket','V','Vänsterpartiet',334053,5.60,19),
  (2010,'riket','Riket','KD','Kristdemokraterna',333696,5.60,19),
  (2010,'riket','Riket','L','Liberalerna',420524,7.06,24),
  (2010,'riket','Riket','MP','Miljöpartiet',437435,7.34,25),
  (2006,'riket','Riket','S','Socialdemokraterna',1942625,34.99,130),
  (2006,'riket','Riket','M','Moderaterna',1456666,26.23,97),
  (2006,'riket','Riket','SD','Sverigedemokraterna',162463,2.93,0),
  (2006,'riket','Riket','C','Centerpartiet',437389,7.88,29),
  (2006,'riket','Riket','V','Vänsterpartiet',324722,5.85,22),
  (2006,'riket','Riket','KD','Kristdemokraterna',366005,6.59,24),
  (2006,'riket','Riket','L','Liberalerna',418686,7.54,28),
  (2006,'riket','Riket','MP','Miljöpartiet',291054,5.24,19),
  (2002,'riket','Riket','S','Socialdemokraterna',2115788,39.85,144),
  (2002,'riket','Riket','M','Moderaterna',810391,15.26,55),
  (2002,'riket','Riket','SD','Sverigedemokraterna',76300,1.44,0),
  (2002,'riket','Riket','C','Centerpartiet',328498,6.19,22),
  (2002,'riket','Riket','V','Vänsterpartiet',440371,8.30,30),
  (2002,'riket','Riket','KD','Kristdemokraterna',485221,9.14,33),
  (2002,'riket','Riket','L','Liberalerna',711427,13.39,48),
  (2002,'riket','Riket','MP','Miljöpartiet',246683,4.65,17)
) AS t(election_year,area_code,area_name,party_code,party_name,votes,vote_share,mandates)
WHERE NOT EXISTS (SELECT 1 FROM val_results WHERE area_code='riket' AND election_year=2002);

-- Regionfullmäktige national aggregate 2006–2022
-- 2010–2018 aggregated from historik.val.se per-region data; 2006+2022 from public records.
INSERT INTO val_results (election_year,area_code,area_name,party_code,party_name,votes,vote_share,mandates)
SELECT * FROM (VALUES
  (2022,'riket_rf','Regionval 2022','S','Socialdemokraterna',  1875000::INT,30.0::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','M','Moderaterna',         1196000::INT,19.1::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','SD','Sverigedemokraterna',1241000::INT,19.9::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','C','Centerpartiet',        425000::INT, 6.8::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','V','Vänsterpartiet',       441000::INT, 7.1::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','KD','Kristdemokraterna',   356000::INT, 5.7::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','L','Liberalerna',          322000::INT, 5.2::NUMERIC(5,2),NULL::INT),
  (2022,'riket_rf','Regionval 2022','MP','Miljöpartiet',        315000::INT, 5.1::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','S','Socialdemokraterna',  3701448::INT,30.1::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','M','Moderaterna',         2483434::INT,20.2::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','SD','Sverigedemokraterna',1656440::INT,13.5::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','C','Centerpartiet',       1078150::INT, 8.8::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','V','Vänsterpartiet',      1110086::INT, 9.0::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','KD','Kristdemokraterna',   915358::INT, 7.4::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','L','Liberalerna',          807562::INT, 6.6::NUMERIC(5,2),NULL::INT),
  (2018,'riket_rf','Regionval 2018','MP','Miljöpartiet',        531044::INT, 4.3::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','S','Socialdemokraterna',  4040952::INT,34.4::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','M','Moderaterna',         2642344::INT,22.5::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','SD','Sverigedemokraterna',1123222::INT, 9.6::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','C','Centerpartiet',        775110::INT, 6.6::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','V','Vänsterpartiet',       869984::INT, 7.4::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','KD','Kristdemokraterna',   634540::INT, 5.4::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','L','Liberalerna',          771998::INT, 6.6::NUMERIC(5,2),NULL::INT),
  (2014,'riket_rf','Regionval 2014','MP','Miljöpartiet',        885520::INT, 7.5::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','S','Socialdemokraterna',  3845906::INT,34.3::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','M','Moderaterna',         3176340::INT,28.4::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','SD','Sverigedemokraterna', 532614::INT, 4.7::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','C','Centerpartiet',        717644::INT, 6.4::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','V','Vänsterpartiet',       671776::INT, 6.0::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','KD','Kristdemokraterna',   590186::INT, 5.3::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','L','Liberalerna',          870796::INT, 7.8::NUMERIC(5,2),NULL::INT),
  (2010,'riket_rf','Regionval 2010','MP','Miljöpartiet',        797564::INT, 7.1::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','S','Socialdemokraterna',  NULL::INT,  36.2::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','M','Moderaterna',         NULL::INT,  24.8::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','SD','Sverigedemokraterna',NULL::INT,   2.6::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','C','Centerpartiet',       NULL::INT,   7.5::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','V','Vänsterpartiet',      NULL::INT,   5.4::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','KD','Kristdemokraterna',  NULL::INT,   6.1::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','L','Liberalerna',         NULL::INT,   7.0::NUMERIC(5,2),NULL::INT),
  (2006,'riket_rf','Regionval 2006','MP','Miljöpartiet',       NULL::INT,   4.6::NUMERIC(5,2),NULL::INT)
) AS t(election_year,area_code,area_name,party_code,party_name,votes,vote_share,mandates)
WHERE NOT EXISTS (SELECT 1 FROM val_results WHERE area_code='riket_rf' AND election_year=2018);

INSERT INTO val_turnout (election_year,area_code,area_name,eligible_voters,votes_cast,turnout)
SELECT * FROM (VALUES
  (2022,'riket','Riket',7853741,6628619,84.21),
  (2018,'riket','Riket',7496852,6535271,87.18),
  (2014,'riket','Riket',7290755,6254031,85.81),
  (2010,'riket','Riket',7088072,5986505,84.63),
  (2006,'riket','Riket',6935544,5678960,81.99),
  (2002,'riket','Riket',6731474,5394049,80.11)
) AS t(election_year,area_code,area_name,eligible_voters,votes_cast,turnout)
WHERE NOT EXISTS (SELECT 1 FROM val_turnout WHERE area_code='riket' AND election_year=2002);
`

// nationalPoliceSchema stores police events for all of Sweden. It is additive:
// each sync appends new events rather than replacing them, so history grows
// beyond the API's 500-event window over time. external_id is the polisen.se
// integer event id and the upsert key. county_code is derived at sync time
// from location_name (county/municipality match) or GPS point-in-polygon.
// airQualitySchema stores the current reading per county per parameter
// from the Open-Meteo CAMS air quality model (one row per county × parameter).
// Parameters: 1=PM10, 2=PM2.5, 5=NO2, 8=O3. All values in µg/m³.
// station_key is "{county_code}_{parameter_id}" (e.g. "ab_2" for Stockholm PM2.5).
const airQualitySchema = `
CREATE TABLE IF NOT EXISTS air_quality (
    id          BIGSERIAL        PRIMARY KEY,
    station_key TEXT             NOT NULL,
    parameter   INT              NOT NULL,
    param_name  TEXT             NOT NULL DEFAULT '',
    name        TEXT             NOT NULL,
    county_code TEXT,
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    height_m    DOUBLE PRECISION,
    value       NUMERIC(8,2),
    quality     TEXT,
    measured_at TIMESTAMPTZ,
    synced_at   TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    UNIQUE (station_key, parameter)
);
CREATE INDEX IF NOT EXISTS air_quality_county_idx  ON air_quality (county_code);
CREATE INDEX IF NOT EXISTS air_quality_param_idx   ON air_quality (parameter);
CREATE INDEX IF NOT EXISTS air_quality_measure_idx ON air_quality (measured_at DESC);
`

// braSchema stores reported crimes (anmälda brott) from BRÅ's Sol statistics
// database, per kommun × crime category × year (1996–). kommun_id is the Sol
// region ID; county_code is resolved from the embedded kommun table in the
// fetcher. per_100k is the rate per 100,000 inhabitants as published by BRÅ
// (kommun population basis). Values can be NULL when BRÅ has no figure.
const braSchema = `
CREATE TABLE IF NOT EXISTS bra_crimes (
    id          BIGSERIAL     PRIMARY KEY,
    year        INT           NOT NULL,
    category_id INT           NOT NULL,
    slug        TEXT          NOT NULL,
    category    TEXT          NOT NULL,
    kommun_id   INT           NOT NULL,
    kommun      TEXT          NOT NULL,
    county_code TEXT          NOT NULL,
    count       INT,
    per_100k    NUMERIC(10,1),
    source      TEXT          NOT NULL DEFAULT 'bra',
    synced_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (year, category_id, kommun_id)
);
CREATE INDEX IF NOT EXISTS bra_crimes_county_idx ON bra_crimes (county_code, slug, year);
CREATE INDEX IF NOT EXISTS bra_crimes_slug_idx   ON bra_crimes (slug, year);
`

// trafikSchema stores active traffic situations from Trafikverket.
// The table is truncated and reloaded every sync since situations are real-time
// and deletions/updates come as new API payloads, not incremental patches.
// external_id is Trafikverket's own UUID for the situation.
const trafikSchema = `
CREATE TABLE IF NOT EXISTS trafik_situations (
    id           BIGSERIAL    PRIMARY KEY,
    external_id  TEXT         NOT NULL UNIQUE,
    site_type    TEXT         NOT NULL DEFAULT '',  -- Olycka, Vägarbete, Trafikmeddelande, …
    severity     TEXT         NOT NULL DEFAULT '',
    title        TEXT         NOT NULL DEFAULT '',
    description  TEXT         NOT NULL DEFAULT '',
    road_number  TEXT         NOT NULL DEFAULT '',
    county_codes TEXT[]       NOT NULL DEFAULT '{}',
    lat          DOUBLE PRECISION,
    lon          DOUBLE PRECISION,
    start_time   TIMESTAMPTZ,
    end_time     TIMESTAMPTZ,
    source       TEXT         NOT NULL DEFAULT 'trafikverket',
    synced_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS trafik_sit_type_idx    ON trafik_situations (site_type);
CREATE INDEX IF NOT EXISTS trafik_sit_county_idx  ON trafik_situations USING GIN (county_codes);
CREATE INDEX IF NOT EXISTS trafik_sit_start_idx   ON trafik_situations (start_time DESC);
`

// trafikFlowSchema stores real-time traffic flow measurements (vehicles/hour + avg speed).
// Truncated and reloaded each sync since measurements are continuously updated by sensors.
// Only Stockholm (ab), Västra Götaland (o), and Skåne (m) sensor networks are available via the free API tier.
const trafikFlowSchema = `
CREATE TABLE IF NOT EXISTS trafik_flow (
    id          BIGSERIAL    PRIMARY KEY,
    site_id     INT          NOT NULL,
    county_code TEXT         NOT NULL,
    flow_rate   INT,                     -- vehicles per hour
    avg_speed   NUMERIC(6,2),            -- km/h
    lane        TEXT         NOT NULL DEFAULT '',
    measured_at TIMESTAMPTZ,
    synced_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS trafik_flow_county_idx ON trafik_flow (county_code);
CREATE INDEX IF NOT EXISTS trafik_flow_site_idx   ON trafik_flow (site_id, measured_at DESC);
`

const nationalPoliceSchema = `
CREATE TABLE IF NOT EXISTS national_police_events (
    id            BIGSERIAL    PRIMARY KEY,
    external_id   BIGINT       NOT NULL UNIQUE,
    source        TEXT         NOT NULL DEFAULT 'polisen',
    event_time    TIMESTAMPTZ,
    name          TEXT,
    summary       TEXT,
    url           TEXT,
    type          TEXT,
    location_name TEXT,
    county_code   TEXT,
    lat           DOUBLE PRECISION,
    lon           DOUBLE PRECISION,
    raw           JSONB        NOT NULL DEFAULT '{}',
    synced_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS national_police_events_time_idx     ON national_police_events (event_time DESC);
CREATE INDEX IF NOT EXISTS national_police_events_type_idx     ON national_police_events (type);
CREATE INDEX IF NOT EXISTS national_police_events_location_idx ON national_police_events (location_name);
CREATE INDEX IF NOT EXISTS national_police_events_county_idx   ON national_police_events (county_code);
`

// riksbankSchema stores Riksbanken time series (policy rate, exchange rates, etc.).
// series_id is Riksbanken's own identifier e.g. "SECBREPOEFF" (effective repo rate).
const riksbankSchema = `
CREATE TABLE IF NOT EXISTS riksbank_obs (
    id        BIGSERIAL    PRIMARY KEY,
    series_id TEXT         NOT NULL,
    date      DATE         NOT NULL,
    value     NUMERIC(12,5) NOT NULL,
    synced_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (series_id, date)
);
CREATE INDEX IF NOT EXISTS riksbank_series_date_idx ON riksbank_obs (series_id, date DESC);
`

// elprisSchema stores 15-minute Nord Pool electricity spot prices for SE1–SE4
// sourced from elprisetjustnu.se. Prices are in SEK and EUR per kWh.
const elprisSchema = `
CREATE TABLE IF NOT EXISTS elpris (
    id          BIGSERIAL    PRIMARY KEY,
    area        TEXT         NOT NULL,
    time_start  TIMESTAMPTZ  NOT NULL,
    sek_per_kwh NUMERIC(8,5) NOT NULL,
    eur_per_kwh NUMERIC(8,5) NOT NULL,
    synced_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (area, time_start)
);
CREATE INDEX IF NOT EXISTS elpris_area_time_idx ON elpris (area, time_start DESC);
`

// skolaSchema stores school data from Skolverket.
// skola_schools: active school count per municipality from the school unit registry.
// skola_salsa: per-municipality SALSA model aggregates — actual vs. expected grades
//   given pupil background (parents' education, proportion newly arrived, gender mix).
//   Aggregated from ~1,500 individual school records; ~286 municipalities have data.
// skola_national: national grundskola averages — merit score, teacher certification,
//   pass rates, and nationella prov subject scores for åk 6 and åk 9.
const skolaSchema = `
CREATE TABLE IF NOT EXISTS skola_schools (
    id                BIGSERIAL   PRIMARY KEY,
    municipality_code TEXT        NOT NULL UNIQUE,
    school_count      INT         NOT NULL,
    synced_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS skola_salsa (
    id                   BIGSERIAL      PRIMARY KEY,
    municipality_code    TEXT           NOT NULL UNIQUE,
    school_count         INT            NOT NULL DEFAULT 0,
    avg_actual           NUMERIC(6,1),
    avg_calculated       NUMERIC(6,1),
    avg_deviation        NUMERIC(6,1),
    req_reached_actual   NUMERIC(5,1),
    req_reached_dev      NUMERIC(5,1),
    parents_education    NUMERIC(4,2),
    synced_at            TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS skola_national (
    id                              BIGSERIAL      PRIMARY KEY,
    period                          TEXT           NOT NULL UNIQUE,
    students_per_teacher            NUMERIC(5,2),
    certified_teachers_pct          NUMERIC(5,1),
    ratio_passed_9th                NUMERIC(5,1),
    avg_merit_9th                   NUMERIC(6,1),
    eligible_yr_pct                 NUMERIC(5,1),
    eligible_es_pct                 NUMERIC(5,1),
    ratio_passed_6th                NUMERIC(5,1),
    test_sve_6                      NUMERIC(5,2),
    test_eng_6                      NUMERIC(5,2),
    test_ma_6                       NUMERIC(5,2),
    test_sve_9                      NUMERIC(5,2),
    test_eng_9                      NUMERIC(5,2),
    test_ma_9                       NUMERIC(5,2),
    synced_at                       TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS ratio_passed_6th NUMERIC(5,1);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS test_sve_6       NUMERIC(5,2);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS test_eng_6       NUMERIC(5,2);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS test_ma_6        NUMERIC(5,2);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS test_sve_9       NUMERIC(5,2);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS test_eng_9       NUMERIC(5,2);
ALTER TABLE skola_national ADD COLUMN IF NOT EXISTS test_ma_9        NUMERIC(5,2);
`

// arbetsmarknadSchema stores the current Arbetsförmedlingen job ad snapshot.
// Both tables are truncated and reloaded each sync (REPLACE pattern) since the
// API returns a full current picture, not incremental deltas.
// arbetsmarknad_jobs: total active ads per county.
// arbetsmarknad_fields: breakdown by occupation field (yrkesområde) nationally.
const arbetsmarknadSchema = `
CREATE TABLE IF NOT EXISTS arbetsmarknad_jobs (
    id          BIGSERIAL   PRIMARY KEY,
    county_code TEXT        NOT NULL UNIQUE,
    region_code TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    total_ads   INT         NOT NULL,
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS arbetsmarknad_fields (
    id          BIGSERIAL   PRIMARY KEY,
    term        TEXT        NOT NULL UNIQUE,
    concept_id  TEXT        NOT NULL,
    count       INT         NOT NULL,
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// socialstyrelsensSchema holds three datasets from Socialstyrelsen statistics API:
// ss_deaths: national cause-of-death counts by age group, diagnos, and year.
// ss_dental: dental visit rate (%) per region and year.
// ss_births: births/newborn health indicators per region, variable, and year.
const socialstyrelsensSchema = `
CREATE TABLE IF NOT EXISTS ss_deaths (
    id           BIGSERIAL   PRIMARY KEY,
    diagnos_id   TEXT        NOT NULL,
    diagnos_name TEXT        NOT NULL,
    age_id       INT         NOT NULL,
    year         INT         NOT NULL,
    value        INT,
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (diagnos_id, age_id, year)
);

CREATE TABLE IF NOT EXISTS ss_dental (
    id           BIGSERIAL   PRIMARY KEY,
    region_id    INT         NOT NULL,
    region_name  TEXT        NOT NULL,
    county_code  TEXT,
    year         INT         NOT NULL,
    value        NUMERIC(6,2),
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (region_id, year)
);

CREATE TABLE IF NOT EXISTS ss_births (
    id            BIGSERIAL   PRIMARY KEY,
    variabel_id   TEXT        NOT NULL,
    variabel_name TEXT        NOT NULL,
    region_id     INT         NOT NULL,
    region_name   TEXT        NOT NULL,
    county_code   TEXT,
    year          INT         NOT NULL,
    value         INT,
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (variabel_id, region_id, year)
);

CREATE TABLE IF NOT EXISTS fk_sjukfall (
    id           BIGSERIAL   PRIMARY KEY,
    county_code  TEXT        NOT NULL,
    lan_kod      TEXT        NOT NULL,
    year         INT         NOT NULL,
    month        INT         NOT NULL,
    diagnos      TEXT        NOT NULL DEFAULT 'F43',
    andel        NUMERIC(5,2),
    antal        INT,
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (county_code, year, month, diagnos)
);
CREATE INDEX IF NOT EXISTS fk_sjukfall_county_idx ON fk_sjukfall (county_code);
`

const energimixSchema = `
CREATE TABLE IF NOT EXISTS energimix_stats (
    id        BIGSERIAL   PRIMARY KEY,
    year      INT         NOT NULL,
    source    TEXT        NOT NULL,
    twh       NUMERIC(10,3),
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(year, source)
);
CREATE INDEX IF NOT EXISTS energimix_year_idx ON energimix_stats (year);
`

const skattesatsSchema = `
CREATE TABLE IF NOT EXISTS skattesats (
    id                BIGSERIAL    PRIMARY KEY,
    year              INT          NOT NULL,
    municipality      TEXT         NOT NULL,
    lan_kod           TEXT         NOT NULL,
    county_code       TEXT         NOT NULL,
    kommunal_skatt    NUMERIC(6,3) NOT NULL,
    landstings_skatt  NUMERIC(6,3) NOT NULL,
    begravnings_avgift NUMERIC(6,3) NOT NULL,
    summa_exkl_kyrka  NUMERIC(6,3) NOT NULL,
    synced_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(year, municipality)
);
CREATE INDEX IF NOT EXISTS skattesats_county_idx ON skattesats (county_code, year DESC);
CREATE INDEX IF NOT EXISTS skattesats_year_idx   ON skattesats (year);
`

const foretagsoverskottSchema = `
CREATE TABLE IF NOT EXISTS foretagsoverskott (
    id           BIGSERIAL PRIMARY KEY,
    year         INT       NOT NULL,
    municipality TEXT      NOT NULL,
    belopp_sek   BIGINT    NOT NULL,
    antal        INT       NOT NULL,
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(year, municipality)
);
CREATE INDEX IF NOT EXISTS foretagsoverskott_year_idx ON foretagsoverskott (year);
`

const kapitalinkomstSchema = `
CREATE TABLE IF NOT EXISTS kapitalinkomst (
    id            BIGSERIAL PRIMARY KEY,
    year          INT       NOT NULL,
    county_name   TEXT      NOT NULL,
    statistikterm TEXT      NOT NULL,
    belopp_sek    BIGINT    NOT NULL,
    antal         INT       NOT NULL,
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(year, county_name, statistikterm)
);
CREATE INDEX IF NOT EXISTS kapitalinkomst_year_idx ON kapitalinkomst (year);
`

const skattPerPersonSchema = `
CREATE TABLE IF NOT EXISTS skatt_per_person (
    id           BIGSERIAL PRIMARY KEY,
    year         INT       NOT NULL,
    municipality TEXT      NOT NULL,
    typ_av_skatt TEXT      NOT NULL,
    belopp_sek   BIGINT    NOT NULL,
    median_kr    INT       NOT NULL,
    medel_kr     INT       NOT NULL,
    antal        INT       NOT NULL,
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(year, municipality, typ_av_skatt)
);
CREATE INDEX IF NOT EXISTS skatt_per_person_year_idx ON skatt_per_person (year);
`

const kommunalskattInkomstnivåSchema = `
CREATE TABLE IF NOT EXISTS kommunalskatt_inkomstniva (
    id            BIGSERIAL PRIMARY KEY,
    year          INT       NOT NULL,
    inkomstniva   TEXT      NOT NULL,
    statistikterm TEXT      NOT NULL,
    belopp_sek    BIGINT    NOT NULL,
    antal         INT       NOT NULL,
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(year, inkomstniva, statistikterm)
);
CREATE INDEX IF NOT EXISTS ki_niva_year_idx ON kommunalskatt_inkomstniva (year);
`

const riksdagenKalenderSchema = `
CREATE TABLE IF NOT EXISTS riksdagen_kalender (
    id         BIGSERIAL PRIMARY KEY,
    dok_id     TEXT      NOT NULL UNIQUE,
    datum      TEXT      NOT NULL,
    typ        TEXT      NOT NULL,
    subtyp     TEXT      NOT NULL DEFAULT '',
    titel      TEXT      NOT NULL,
    undertitel TEXT      NOT NULL DEFAULT '',
    organ      TEXT      NOT NULL DEFAULT '',
    plats      TEXT      NOT NULL DEFAULT '',
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS riksdagen_kalender_datum_idx ON riksdagen_kalender (datum);
`

const raaFornlamningarSchema = `
CREATE TABLE IF NOT EXISTS raa_fornlamningar (
    id          BIGSERIAL PRIMARY KEY,
    kommun      TEXT      NOT NULL UNIQUE,
    county_code TEXT      NOT NULL,
    antal       INT       NOT NULL,
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS raa_fornlamningar_county_idx ON raa_fornlamningar (county_code);
`
