package fetcher

import "encoding/json"

// ---- Polisen ----------------------------------------------------------------

// PoliceLocation holds the location fields returned by the polisen.se API.
type PoliceLocation struct {
	Name string `json:"name"` // municipality or county name, e.g. "Kiruna", "Norrbottens län"
	GPS  string `json:"gps"`  // "lat,lon" — parsed by parseGPS in store
}

// PoliceEvent is a single incident from the Swedish Police open data API
// (polisen.se/api/events). The API returns the 500 most recent events;
// history is accumulated by upserting on the integer ID.
type PoliceEvent struct {
	ID       int            `json:"id"`
	DateTime string         `json:"datetime"` // "2026-05-20 17:11:17 +02:00"
	Name     string         `json:"name"`
	Summary  string         `json:"summary"`
	URL      string         `json:"url"` // relative path, e.g. "/aktuellt/handelser/..."
	Type     string         `json:"type"`
	Location PoliceLocation `json:"location"`
}

// ---- SMHI -------------------------------------------------------------------

// smhiStationSet is the top-level envelope of the SMHI metobs "station-set/all"
// endpoint: one entry per station, each carrying the latest-hour readings.
type smhiStationSet struct {
	Station []SMHIStation `json:"station"`
}

// SMHIStation is one weather station with its latest readings.
//
// Quirks of the SMHI API handled here:
//   - key arrives as a JSON number in some responses and a string in others,
//     so json.Number absorbs both.
//   - value[].value (the reading) is a string, e.g. "17.2".
//   - value[].date is epoch milliseconds.
//   - value can be null or empty when the station sent nothing this hour.
type SMHIStation struct {
	Key       json.Number `json:"key"`
	Name      string      `json:"name"`
	Owner     string      `json:"owner"`
	Height    float64     `json:"height"`
	Latitude  float64     `json:"latitude"`
	Longitude float64     `json:"longitude"`
	Value     []SMHIValue `json:"value"`
}

// SMHIValue is a single reading from a station.
type SMHIValue struct {
	Date    int64  `json:"date"`    // epoch milliseconds
	Value   string `json:"value"`   // numeric reading as a string
	Quality string `json:"quality"` // G (green/verified), Y (yellow), R (red)
}

// SMHIObservation is the flattened per-station result handed to the store.
// Temperature is nil when the station sent no reading for the latest hour.
type SMHIObservation struct {
	StationKey  string
	Parameter   int
	Name        string
	CountyCode  string // filled in by the sync job via the geo package
	Lat         float64
	Lon         float64
	HeightM     float64
	Temperature *float64
	Quality     string
	MeasuredAt  *int64 // epoch ms; nil when no reading
	Raw         SMHIStation
}

// ---- SMHI Luftkvalitet -------------------------------------------------------

// ---- Socialstyrelsen ---------------------------------------------------------

// SSDeath is one age-group row from Socialstyrelsen dödsorsaker API.
type SSDeath struct {
	DiagnosID   string
	DiagnosName string
	AgeID       int
	Year        int
	Value       *int
}

// SSDental is one region/year row from Socialstyrelsen tandhälsa API.
// Value is the percentage of residents who visited the dentist last year.
type SSDental struct {
	RegionID   int
	RegionName string
	CountyCode string
	Year       int
	Value      *float64
}

// SSBirth is one region/year/variable row from Socialstyrelsen
// graviditeter/förlossningar/nyfödda API.
type SSBirth struct {
	VariabelID   string
	VariabelName string
	RegionID     int
	RegionName   string
	CountyCode   string
	Year         int
	Value        *int
}

// ---- SMHI Luftkvalitet -------------------------------------------------------

// AirQualityObservation is one per-county, per-parameter air quality reading
// from the Open-Meteo CAMS model. Value is nil only on parse failure.
type AirQualityObservation struct {
	StationKey string
	Parameter  int    // 1=PM10, 2=PM2.5, 5=NO2, 8=O3
	ParamName  string // human label, e.g. "PM2.5"
	Name       string
	CountyCode string // resolved by sync job via geo package
	Lat        float64
	Lon        float64
	HeightM    float64
	Value      *float64
	Quality    string
	MeasuredAt *int64 // epoch ms; nil when no reading
}
