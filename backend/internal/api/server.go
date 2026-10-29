// Package api wires together the HTTP router and all request handlers.
package api

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds and returns the complete HTTP handler tree.
//
// db must satisfy the DB interface; *pgxpool.Pool does in production.
// triggerSync is a closure injected from main so the POST /sync endpoint can
// kick off a background sync without this package importing the fetcher or store.
// syncToken is the bearer token for POST /sync; empty string means unprotected (dev only).
// logAccess is called once per request to record visitor data; pass nil to disable.
func NewRouter(db DB, triggerSync func(), syncToken string, baseURL string, logAccess func(ip, method, path string, status, ms int, ua, referer string)) http.Handler {
	h := &handlers{db: db, baseURL: baseURL}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer) // turn panics into 500s instead of crashing the server
	r.Use(securityHeaders)
	r.Use(cors)
	if logAccess != nil {
		r.Use(accessLogMiddleware(logAccess))
	}

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	r.Get("/sitemap.xml", h.sitemap)
	r.Get("/robots.txt", h.robots)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/weather", h.weather)
		r.Get("/policeevents", h.policeEvents)
		r.Get("/befolkning", h.befolkning)
		r.Get("/inkomst", h.inkomst)
		r.Get("/kolada", h.kolada)
		r.Get("/brp", h.brp)
		r.Get("/vindkraft", h.vindkraft)
		r.Get("/solceller", h.solceller)
		r.Get("/energimix", h.energimix)
		r.Get("/skattesats", h.skattesats)
		r.Get("/foretagsoverskott", h.foretagsoverskott)
		r.Get("/kapitalinkomst", h.kapitalinkomst)
		r.Get("/skatt/per-person", h.skattPerPerson)
		r.Get("/skatt/inkomstniva", h.kommunalskattInkomstniva)
		r.Get("/samband", h.samband)
		r.Get("/riksdagen/kalender", h.riksdagenKalender)
		r.Get("/fornlamningar", h.fornlamningar)
		r.Get("/lan/summary", h.lanSummary)
		r.Get("/lan/rankings", h.lanRankings)
		r.Get("/lan/profile", h.lanProfile)
		r.Get("/sjukskrivning", h.sjukskrivning)
		r.Get("/domstol/trends", h.domstolTrends)
		r.Get("/domstol/processing", h.domstolProcessing)
		r.Get("/folkhalsa", h.folkhalsa)
		r.Get("/val/national", h.valNational)
		r.Get("/val/turnout", h.valTurnout)
		r.Get("/val/valkrets", h.valValkrets)
		r.Get("/luftkvalitet", h.airQuality)
		r.Get("/brott", h.braCrimes)
		r.Get("/brott/lan", h.braLan)
		r.Get("/trafik", h.trafik)
		r.Get("/trafik/flow", h.trafikFlow)
		r.Get("/riksbank", h.riksbank)
		r.Get("/elpris", h.elpris)
		r.Get("/arbetsmarknad/jobs", h.arbetsmarknadJobs)
		r.Get("/arbetsmarknad/fields", h.arbetsmarknadFields)
		r.Get("/utbildning/schools", h.skolaSchools)
		r.Get("/utbildning/salsa", h.skolaSalsa)
		r.Get("/utbildning/national", h.skolaNational)
		r.Get("/halsa/dodsorsaker", h.ssDeaths)
		r.Get("/halsa/tandhalsa", h.ssDental)
		r.Get("/befolkning/forlossningar", h.ssBirths)
		r.Get("/riksdagen/members", h.riksdagenMembers)
		r.Get("/riksdagen/seats", h.riksdagenSeats)
		r.Get("/riksdagen/votes", h.riksdagenVotes)
		r.Get("/sync/status", h.syncStatus)
		// POST /sync fires a non-blocking background sync and returns 202 immediately.
		// Protected by a static bearer token (SYNC_TOKEN env var) to prevent anyone
		// on the internet from triggering unlimited sync goroutines.
		r.Post("/sync", syncHandler(triggerSync, syncToken))
		// Admin stats — protected by the same SYNC_TOKEN bearer auth.
		r.Get("/admin/stats", adminStatsHandler(h, syncToken))
	})

	return r
}

// wrappedWriter captures the HTTP status code written by a handler so the
// access log middleware can record it after the response is sent.
type wrappedWriter struct {
	http.ResponseWriter
	status int
}

func (ww *wrappedWriter) WriteHeader(code int) {
	ww.status = code
	ww.ResponseWriter.WriteHeader(code)
}

// accessLogMiddleware records one row per request into access_log via logAccess.
// Health checks and OPTIONS preflight requests are skipped to reduce noise.
func accessLogMiddleware(logAccess func(ip, method, path string, status, ms int, ua, referer string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			ww := &wrappedWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			go logAccess(
				realIP(r),
				r.Method,
				r.URL.Path,
				ww.status,
				int(time.Since(start).Milliseconds()),
				r.Header.Get("User-Agent"),
				r.Header.Get("Referer"),
			)
		})
	}
}

// realIP extracts the true client IP. X-Forwarded-For / X-Real-IP are only
// trusted when the direct TCP connection comes from a private network address
// (the Docker bridge), preventing IP spoofing from internet-facing callers.
func realIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	if isTrustedProxy(remoteHost) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx != -1 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
	return remoteHost
}

// isTrustedProxy reports whether ip is in a private network range used by
// Docker bridge networks or localhost — the only sources allowed to set
// X-Forwarded-For headers.
func isTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(parsed) {
			return true
		}
	}
	return false
}

// syncHandler returns an http.HandlerFunc that triggers a background sync.
// If token is set, the request must supply it as "Bearer <token>" in the
// Authorization header. If token is empty the endpoint is unprotected
// (acceptable for local dev; not recommended in production).
func syncHandler(triggerSync func(), token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			if r.Header.Get("Authorization") != "Bearer "+token {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		go triggerSync()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync started"})
	}
}

// adminStatsHandler returns visitor and request statistics from access_log.
// Protected by the same SYNC_TOKEN bearer auth used by POST /sync.
func adminStatsHandler(h *handlers, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			if r.Header.Get("Authorization") != "Bearer "+token {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		h.accessStats(w, r)
	}
}

// securityHeaders adds conservative security headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src http: https:; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

// cors is a permissive CORS middleware for GET requests.
// All served data is intentionally public, so GET * is fine.
// POST is excluded from Allow-Methods so browsers will not send cross-origin
// sync requests without an explicit preflight that our server will reject.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
