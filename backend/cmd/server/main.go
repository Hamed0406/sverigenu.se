// Command server is the entry point for the swedennu backend.
//
// On startup it:
//  1. Connects to PostgreSQL and runs schema migrations.
//  2. Fires an initial background sync to populate the database immediately.
//  3. Schedules a periodic sync on the SYNC_INTERVAL (default 15 minutes).
//  4. Starts the HTTP API server on PORT (default 3000).
//
// Graceful shutdown is handled via SIGINT/SIGTERM: the HTTP server gets 10 seconds
// to finish in-flight requests before the process exits.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"swedennu/internal/api"
	"swedennu/internal/config"
	"swedennu/internal/db"
	"swedennu/internal/fetcher"
	"swedennu/internal/geo"
	"swedennu/internal/store"
)

func main() {
	// Bootstrap with a temporary info-level logger so we can log the config load.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg := config.Load()

	// Reconfigure logger now that we know the debug flag.
	logLevel := slog.LevelInfo
	if cfg.Debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	if cfg.Debug {
		slog.Debug("debug logging enabled")
		slog.Debug("config loaded",
			"port", cfg.Port,
			"sync_interval", cfg.SyncInterval,
			"log_dir", cfg.LogDir,
			"sync_token_set", cfg.SyncToken != "",
		)
	}

	// The root context is cancelled when SIGINT or SIGTERM arrives.
	// All long-running goroutines select on ctx.Done() to exit cleanly.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.Info("connecting to database")
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("running migrations")
	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}

	client := fetcher.New(cfg.ContactEmail, cfg.TrafikverketAPIKey)
	st := store.New(pool)

	// syncMu ensures at most one sync runs at a time. If a manual POST /sync
	// arrives while a periodic sync is already in flight, it is silently dropped
	// rather than stacking goroutines that each hold DB connections.
	var syncMu sync.Mutex

	// syncAll fetches every data category and upserts into Postgres.
	// Jobs run sequentially (not concurrently) to avoid hammering upstream APIs.
	// A 10-minute timeout context prevents a slow upstream from blocking forever.
	syncAll := func() {
		if !syncMu.TryLock() {
			slog.Info("sync already in progress, skipping")
			return
		}
		defer syncMu.Unlock()

		slog.Info("sync started")
		syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Minute)
		defer syncCancel()

		type job struct {
			name        string
			source      string
			minInterval time.Duration // skip run if last success was more recent than this; 0 = always run
			run         func() (int, error)
		}

		jobs := []job{
			{name: "luftkvalitet", source: "smhi_luft", minInterval: time.Hour, run: func() (int, error) {
				items, err := client.FetchAirQuality(syncCtx)
				if err != nil {
					return 0, err
				}
				for i := range items {
					items[i].CountyCode = geo.CountyForPoint(items[i].Lat, items[i].Lon)
				}
				return st.UpsertAirQuality(syncCtx, items)
			}},
			{name: "smhi_temperature", source: "smhi", run: func() (int, error) {
				items, err := client.FetchSMHITemperatures(syncCtx)
				if err != nil {
					return 0, err
				}
				// Resolve each station to a county before storing. Cheap:
				// ~550 stations × at most 21 polygon tests per sync.
				for i := range items {
					items[i].CountyCode = geo.CountyForPoint(items[i].Lat, items[i].Lon)
				}
				return st.UpsertSMHIObservations(syncCtx, items)
			}},
			{name: "policeevents_national", source: "polisen", run: func() (int, error) {
				items, err := client.FetchNationalPoliceEvents(syncCtx)
				if err != nil {
					return 0, err
				}
				counties := make([]string, len(items))
				for i, ev := range items {
					lat, lon := store.ParseGPS(ev.Location.GPS)
					counties[i] = geo.CountyForPoliceEvent(ev.Location.Name, lat, lon)
				}
				return st.UpsertNationalPoliceEvents(syncCtx, items, counties)
			}},
			{name: "scb_stats", source: "scb", minInterval: 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchSCBStats(syncCtx)
				if err != nil {
					return 0, err
				}
				counties := make([]string, len(items))
				for i := range items {
					counties[i] = geo.CountyBySCBCode(items[i].RegionCode)
				}
				return st.UpsertSCBStats(syncCtx, items, counties)
			}},
			{name: "vindkraft", source: "energimyndigheten", minInterval: 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchVindkraft(syncCtx)
				if err != nil {
					return 0, err
				}
				counties := make([]string, len(items))
				for i := range items {
					counties[i] = geo.CountyBySCBCode(items[i].RegionCode)
				}
				return st.UpsertSCBStats(syncCtx, items, counties)
			}},
			{name: "solceller", source: "energimyndigheten", minInterval: 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchSolceller(syncCtx)
				if err != nil {
					return 0, err
				}
				counties := make([]string, len(items))
				for i := range items {
					counties[i] = geo.CountyBySCBCode(items[i].RegionCode)
				}
				return st.UpsertSCBStats(syncCtx, items, counties)
			}},
			{name: "energimix", source: "energimyndigheten", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchEnergiMix(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertEnergiMix(syncCtx, rows)
			}},
			{name: "skattesats", source: "skatteverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchSkattesats(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSkattesats(syncCtx, rows)
			}},
			{name: "foretagsoverskott", source: "skatteverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchForetagsoverskott(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertForetagsoverskott(syncCtx, rows)
			}},
			{name: "kapitalinkomst", source: "skatteverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchKapitalinkomst(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertKapitalinkomst(syncCtx, rows)
			}},
			{name: "skatt_per_person", source: "skatteverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchSkattPerPerson(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSkattPerPerson(syncCtx, rows)
			}},
			{name: "kommunalskatt_niva", source: "skatteverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchKommunalskattInkomstniva(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertKommunalskattInkomstniva(syncCtx, rows)
			}},
			{name: "riksdagen_kalender", source: "riksdagen", minInterval: 6 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchRiksdagenKalender(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.ReplaceRiksdagenKalender(syncCtx, rows)
			}},
			{name: "raa_fornlamningar", source: "raa", minInterval: 7 * 24 * time.Hour, run: func() (int, error) {
				munis, err := st.ListKoladaMunicipalities(syncCtx)
				if err != nil {
					return 0, err
				}
				names := make([]string, 0, len(munis))
				for name := range munis {
					names = append(names, name)
				}
				rows, err := client.FetchFornlamningar(syncCtx, names)
				if err != nil {
					return 0, err
				}
				return st.UpsertFornlamningar(syncCtx, rows, munis)
			}},
			{name: "kolada", source: "kolada", minInterval: 12 * time.Hour, run: func() (int, error) {
				items, err := client.FetchKolada(syncCtx)
				if err != nil {
					return 0, err
				}
				counties := make([]string, len(items))
				for i := range items {
					if len(items[i].MunicipalityID) >= 2 {
						counties[i] = geo.CountyBySCBCode(items[i].MunicipalityID[:2]) // länskod prefix
					}
				}
				return st.UpsertKolada(syncCtx, items, counties)
			}},
			{name: "domstol", source: "domstolsverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchDomstolStats(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertDomstolStats(syncCtx, items)
			}},
			{name: "folkhalsa", source: "fohm", minInterval: 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchFolkhalsaStats(syncCtx)
				if err != nil {
					return 0, err
				}
				counties := make([]string, len(items))
				for i := range items {
					counties[i] = geo.CountyBySCBCode(items[i].RegionCode)
				}
				return st.UpsertFolkhalsaStats(syncCtx, items, counties)
			}},
			{name: "riksdagen_members", source: "riksdagen", minInterval: 24 * time.Hour, run: func() (int, error) {
				members, err := client.FetchRiksdagenMembers(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertRiksdagenMembers(syncCtx, members)
			}},
			{name: "val_2022", source: "valmyndigheten", minInterval: 24 * time.Hour, run: func() (int, error) {
				results, turnouts, err := client.FetchVal2022(syncCtx)
				if err != nil {
					return 0, err
				}
				n1, err := st.UpsertValResults(syncCtx, results)
				if err != nil {
					return n1, err
				}
				n2, err := st.UpsertValTurnout(syncCtx, turnouts)
				return n1 + n2, err
			}},
			// Historical per-valkrets results 2006–2022 from SCB ME0104T3.
			// Fetched once per week; the data is static after each election.
			{name: "val_history_scb", source: "scb", minInterval: 7 * 24 * time.Hour, run: func() (int, error) {
				results, err := client.FetchValHistorySCB(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertValResults(syncCtx, results)
			}},
			{name: "riksdagen_votes", source: "riksdagen", minInterval: 24 * time.Hour, run: func() (int, error) {
				// Fetch the two most recent riksmöten — the current session may have
				// few votes early in the year; the previous session fills the gaps.
				var all []fetcher.RiksdagenVoteResult
				for _, rm := range []string{"2025/26", "2024/25"} {
					items, err := client.FetchRiksdagenVotes(syncCtx, rm, 30)
					if err != nil {
						slog.Warn("riksdagen votes fetch error", "rm", rm, "err", err)
						continue
					}
					all = append(all, items...)
				}
				return st.UpsertRiksdagenVotes(syncCtx, all)
			}},
			// Last on purpose: the full 30-year Sol backfill takes minutes, and
			// running it after the fast jobs keeps the 15-minute data fresh.
			{name: "bra_crimes", source: "bra", minInterval: 7 * 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchBRACrimes(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertBRACrimes(syncCtx, items)
			}},
			// Real-time road conditions and traffic flow — refresh every sync cycle.
			{name: "trafik", source: "trafikverket", run: func() (int, error) {
				if cfg.TrafikverketAPIKey == "" {
					return 0, nil
				}
				items, err := client.FetchTrafik(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertTrafik(syncCtx, items)
			}},
			{name: "trafik_flow", source: "trafikverket", run: func() (int, error) {
				if cfg.TrafikverketAPIKey == "" {
					return 0, nil
				}
				items, err := client.FetchTrafikFlow(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertTrafikFlow(syncCtx, items)
			}},
			{name: "riksbank", source: "riksbanken", minInterval: 24 * time.Hour, run: func() (int, error) {
				items, err := client.FetchRiksbank(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertRiksbank(syncCtx, items)
			}},
			{name: "elpris", source: "elprisetjustnu", run: func() (int, error) {
				items, err := client.FetchElpris(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertElpris(syncCtx, items)
			}},
			{name: "arbetsmarknad", source: "arbetsformedlingen", run: func() (int, error) {
				jobs, fields, err := client.FetchAFJobCounts(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.ReplaceArbetsmarknad(syncCtx, jobs, fields)
			}},
			{name: "skolverket_schools", source: "skolverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				schools, err := client.FetchSkolaSchools(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSkolaSchools(syncCtx, schools)
			}},
			{name: "skolverket_salsa", source: "skolverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchSalsaStats(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSalsaStats(syncCtx, rows)
			}},
			{name: "skolverket_national", source: "skolverket", minInterval: 24 * time.Hour, run: func() (int, error) {
				stats, err := client.FetchNationalGrStats(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertNationalGrStats(syncCtx, stats)
			}},
			{name: "ss_deaths", source: "socialstyrelsen", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchSSDeaths(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSSDeaths(syncCtx, rows)
			}},
			{name: "ss_dental", source: "socialstyrelsen", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchSSDental(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSSDental(syncCtx, rows)
			}},
			{name: "ss_births", source: "socialstyrelsen", minInterval: 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchSSBirths(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertSSBirths(syncCtx, rows)
			}},
			{name: "fk_sjukfall", source: "forsakringskassan", minInterval: 30 * 24 * time.Hour, run: func() (int, error) {
				rows, err := client.FetchFKSjukfall(syncCtx)
				if err != nil {
					return 0, err
				}
				return st.UpsertFKSjukfall(syncCtx, rows)
			}},
		}

		for _, j := range jobs {
			// Skip low-frequency jobs if they were synced recently enough.
			if j.minInterval > 0 {
				last, err := st.LastSuccessfulSync(syncCtx, j.name)
				if err == nil && last != nil && time.Since(*last) < j.minInterval {
					slog.Info("sync skipped (within minInterval)",
						"category", j.name,
						"last", last.Round(time.Second),
						"next_in", (j.minInterval - time.Since(*last)).Round(time.Minute),
					)
					continue
				}
			}

			logID, _ := st.LogSyncStart(syncCtx, j.name, j.source)
			slog.Debug("sync job starting", "category", j.name, "source", j.source)
			jobStart := time.Now()
			n, err := j.run()
			elapsed := time.Since(jobStart).Round(time.Millisecond)
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
				slog.Error("sync failed", "category", j.name, "err", err, "elapsed", elapsed)
			} else {
				slog.Info("synced", "category", j.name, "records", n, "elapsed", elapsed)
			}
			st.LogSyncDone(syncCtx, logID, n, errMsg)
		}

		slog.Info("sync complete")
	}

	// Run an initial sync in the background so the server responds to requests
	// immediately rather than waiting for the first ticker tick.
	go syncAll()

	// Periodic sync loop — re-fetches all categories every SYNC_INTERVAL.
	go func() {
		ticker := time.NewTicker(cfg.SyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				syncAll()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Open access log file if LOG_DIR is configured.
	// Entries are written in Combined Log Format (one line per request).
	var accessLogFile *os.File
	if cfg.LogDir != "" {
		if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
			slog.Error("failed to create log directory", "dir", cfg.LogDir, "err", err)
		} else {
			logPath := filepath.Join(cfg.LogDir, "access.log")
			f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				slog.Error("failed to open access log file", "path", logPath, "err", err)
			} else {
				accessLogFile = f
				slog.Info("access log file opened", "path", logPath)
				defer accessLogFile.Close()
			}
		}
	}

	// logAccess writes to both the database and (if configured) the log file.
	logAccess := func(ip, method, path string, status, ms int, ua, referer string) {
		st.LogAccess(ip, method, path, status, ms, ua, referer)
		if accessLogFile != nil {
			// Combined Log Format: IP - - [timestamp] "METHOD path" status ms "referer" "ua"
			fmt.Fprintf(accessLogFile, "%s - - [%s] \"%s %s\" %d %d \"%s\" \"%s\"\n",
				ip,
				time.Now().UTC().Format("02/Jan/2006:15:04:05 +0000"),
				method, path,
				status, ms,
				referer, ua,
			)
		}
	}

	// HTTP server — syncAll is passed as a closure so the POST /sync endpoint
	// can trigger a manual sync without the api package importing fetcher/store.
	router := api.NewRouter(pool, syncAll, cfg.SyncToken, cfg.BaseURL, logAccess)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second, // guards against Slowloris attacks
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Block until the OS sends a shutdown signal.
	<-ctx.Done()
	slog.Info("shutting down")

	// Give in-flight HTTP requests up to 10 seconds to complete.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}
