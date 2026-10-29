package fetcher

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchSMHITemperatures(t *testing.T) {
	body := `{"station":[
		{"key":"1234","name":"TestStation","latitude":59.3,"longitude":18.0,"height":10.0,
		 "value":[{"date":1700000000000,"value":"7.5","quality":"G"}]},
		{"key":"5678","name":"NoReading","latitude":60.0,"longitude":15.0,"height":5.0,
		 "value":[]}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	obs, err := c.FetchSMHITemperatures(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("want 2 observations, got %d", len(obs))
	}

	// First station: has a reading.
	if obs[0].Temperature == nil {
		t.Error("station 1234 should have a temperature reading")
	} else if *obs[0].Temperature != 7.5 {
		t.Errorf("station 1234 temp = %v, want 7.5", *obs[0].Temperature)
	}
	if obs[0].Quality != "G" {
		t.Errorf("quality = %q, want G", obs[0].Quality)
	}

	// Second station: no reading.
	if obs[1].Temperature != nil {
		t.Errorf("station 5678 should have nil temperature, got %v", *obs[1].Temperature)
	}
}

func TestFetchSMHITemperaturesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.FetchSMHITemperatures(testCtx(t))
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestFetchSMHITemperaturesInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.FetchSMHITemperatures(testCtx(t))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchNationalPoliceEvents(t *testing.T) {
	body := `[
		{"id":1,"datetime":"2026-01-01 10:00:00 +01:00","name":"Inbrott","summary":"...","url":"/ev/1","type":"Inbrott","location":{"name":"Stockholm","gps":"59.3,18.0"}},
		{"id":2,"datetime":"2026-01-02 11:00:00 +01:00","name":"Brand","summary":"...","url":"/ev/2","type":"Brand","location":{"name":"Malmö","gps":"55.6,13.0"}}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify User-Agent is set as required by polisen.se terms.
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent header should not be empty for polisen.se")
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	events, err := c.FetchNationalPoliceEvents(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	if events[0].ID != 1 {
		t.Errorf("first event ID = %d, want 1", events[0].ID)
	}
	if events[1].Type != "Brand" {
		t.Errorf("second event type = %q, want Brand", events[1].Type)
	}
}

func TestFetchNationalPoliceEventsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.FetchNationalPoliceEvents(testCtx(t))
	if err == nil {
		t.Error("expected error for 429 response")
	}
}
