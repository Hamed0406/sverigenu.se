package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ── fake DB ──────────────────────────────────────────────────────────────────

// fakeDB implements the DB interface for tests.
// Set QueryErr to make Query return an error; set Rows to control what rows
// are returned; set RowData to control QueryRow results.
type fakeDB struct {
	QueryErr error
	Rows     [][]any   // each inner slice is one row of values
	RowData  []any     // values for a single QueryRow
	RowErr   error
}

func (f *fakeDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if f.QueryErr != nil {
		return nil, f.QueryErr
	}
	return &fakeRows{data: f.Rows}, nil
}

func (f *fakeDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &fakeRow{data: f.RowData, err: f.RowErr}
}

// fakeRows implements pgx.Rows backed by a [][]any.
type fakeRows struct {
	data  [][]any
	index int
	err   error
}

func (r *fakeRows) Next() bool {
	if r.index < len(r.data) {
		r.index++
		return true
	}
	return false
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	row := r.data[r.index-1]
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		switch p := d.(type) {
		case *string:
			if v, ok := row[i].(string); ok {
				*p = v
			}
		case *int:
			if v, ok := row[i].(int); ok {
				*p = v
			}
		case *int64:
			if v, ok := row[i].(int64); ok {
				*p = v
			}
		case **float64:
			if row[i] == nil {
				*p = nil
			} else if v, ok := row[i].(float64); ok {
				vv := v
				*p = &vv
			}
		case *float64:
			if v, ok := row[i].(float64); ok {
				*p = v
			}
		case **string:
			if row[i] == nil {
				*p = nil
			} else if v, ok := row[i].(string); ok {
				*p = &v
			}
		}
	}
	return nil
}

func (r *fakeRows) Close()                        {}
func (r *fakeRows) Err() error                    { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (r *fakeRows) RawValues() [][]byte { return nil }
func (r *fakeRows) Conn() *pgx.Conn    { return nil }
func (r *fakeRows) Values() ([]any, error) {
	if r.index == 0 || r.index > len(r.data) {
		return nil, nil
	}
	return r.data[r.index-1], nil
}

// fakeRow implements pgx.Row for QueryRow tests.
type fakeRow struct {
	data []any
	err  error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	fr := &fakeRows{data: [][]any{r.data}}
	fr.Next()
	return fr.Scan(dest...)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newRouter(db DB) http.Handler {
	return NewRouter(db, func() {}, "", "", nil)
}

func doGet(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v — body: %s", err, rr.Body.String())
	}
	return out
}

// ── writeJSON / writeErr ──────────────────────────────────────────────────────

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var out map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["key"] != "value" {
		t.Errorf("got %q, want \"value\"", out["key"])
	}
}

func TestWriteErr(t *testing.T) {
	rr := httptest.NewRecorder()
	writeErr(rr, http.StatusBadRequest, "bad input")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var out map[string]string
	json.NewDecoder(rr.Body).Decode(&out)
	if out["error"] != "bad input" {
		t.Errorf("error = %q, want \"bad input\"", out["error"])
	}
}

// ── health endpoint ────────────────────────────────────────────────────────────

func TestHealthEndpoint(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/health")
	if rr.Code != http.StatusOK {
		t.Errorf("health: status = %d, want 200", rr.Code)
	}
}

// ── robots.txt ────────────────────────────────────────────────────────────────

func TestRobotsEndpoint(t *testing.T) {
	r := newRouter(&fakeDB{})
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("robots.txt: status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "User-agent") {
		t.Error("robots.txt should contain User-agent directive")
	}
}

// ── CORS headers ──────────────────────────────────────────────────────────────

func TestCORSHeaders(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/health")
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS allow-origin header missing or wrong")
	}
}

func TestCORSPreflight(t *testing.T) {
	r := newRouter(&fakeDB{})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/weather", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS preflight: status = %d, want 204", rr.Code)
	}
}

// ── security headers ──────────────────────────────────────────────────────────

func TestSecurityHeaders(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/health")
	headers := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
	}
	for _, h := range headers {
		if rr.Header().Get(h) == "" {
			t.Errorf("security header %q missing", h)
		}
	}
}

// ── sync token protection ─────────────────────────────────────────────────────

func TestSyncEndpointRequiresToken(t *testing.T) {
	router := NewRouter(&fakeDB{}, func() {}, "mysecret", "", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("sync without token: status = %d, want 401", rr.Code)
	}
}

func TestSyncEndpointAcceptsValidToken(t *testing.T) {
	router := NewRouter(&fakeDB{}, func() {}, "mysecret", "", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	req.Header.Set("Authorization", "Bearer mysecret")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("sync with valid token: status = %d, want 202", rr.Code)
	}
}

// ── realIP ────────────────────────────────────────────────────────────────────

func TestRealIP(t *testing.T) {
	cases := []struct {
		xff      string
		xri      string
		remote   string
		wantIP   string
	}{
		{"203.0.113.1, 10.0.0.1", "", "10.0.0.2:1234", "203.0.113.1"},
		{"", "203.0.113.2", "10.0.0.2:1234", "203.0.113.2"},
		{"", "", "10.0.0.3:5678", "10.0.0.3"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.remote
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		if c.xri != "" {
			req.Header.Set("X-Real-IP", c.xri)
		}
		got := realIP(req)
		if got != c.wantIP {
			t.Errorf("realIP with XFF=%q XRI=%q remote=%q = %q, want %q",
				c.xff, c.xri, c.remote, got, c.wantIP)
		}
	}
}

// ── 404 for unknown routes ────────────────────────────────────────────────────

func TestUnknownRouteReturns404(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/api/v1/doesnotexist")
	if rr.Code != http.StatusNotFound {
		t.Errorf("unknown route: status = %d, want 404", rr.Code)
	}
}

// ── DB error path ─────────────────────────────────────────────────────────────

func TestWeatherHandlerDBError(t *testing.T) {
	db := &fakeDB{QueryErr: fmt.Errorf("connection refused")}
	r := newRouter(db)
	rr := doGet(t, r, "/api/v1/weather")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("DB error: status = %d, want 500", rr.Code)
	}
}

func TestKoladaHandlerDBError(t *testing.T) {
	db := &fakeDB{QueryErr: fmt.Errorf("timeout")}
	r := newRouter(db)
	rr := doGet(t, r, "/api/v1/kolada")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("DB error: status = %d, want 500", rr.Code)
	}
}

// ── Empty-result handlers return 200 with empty data ─────────────────────────

func TestWeatherHandlerEmpty(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/api/v1/weather")
	if rr.Code != http.StatusOK {
		t.Errorf("empty weather: status = %d, want 200", rr.Code)
	}
	body := decodeBody(t, rr)
	if body["count"].(float64) != 0 {
		t.Errorf("empty weather count = %v, want 0", body["count"])
	}
}

func TestPoliceEventsHandlerEmpty(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/api/v1/policeevents")
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestSkolaNationalHandlerEmpty(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/api/v1/utbildning/national")
	if rr.Code != http.StatusOK {
		t.Errorf("skolaNational empty: status = %d, want 200", rr.Code)
	}
}

func TestSkolaSalsaHandlerEmpty(t *testing.T) {
	r := newRouter(&fakeDB{})
	rr := doGet(t, r, "/api/v1/utbildning/salsa")
	if rr.Code != http.StatusOK {
		t.Errorf("skolaSalsa empty: status = %d, want 200", rr.Code)
	}
	body := decodeBody(t, rr)
	if body["count"].(float64) != 0 {
		t.Errorf("empty salsa count = %v, want 0", body["count"])
	}
}

// ── sitemap ───────────────────────────────────────────────────────────────────

func TestSitemapContainsPages(t *testing.T) {
	r := newRouter(&fakeDB{})
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("sitemap: status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, page := range []string{"/brott", "/val", "/lan/ab", "/lan/bd"} {
		if !strings.Contains(body, page) {
			t.Errorf("sitemap missing %q", page)
		}
	}
}
