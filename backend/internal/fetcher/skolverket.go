package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const skolverketBase    = "https://api.skolverket.se/skolenhetsregistret/v1"
const skolverketPlanBase = "https://api.skolverket.se/planned-educations"

// parseSv parses a Swedish-formatted decimal string ("11,9" → 11.9).
// Returns 0 and false if the value is blank, ".." or otherwise unparseable.
func parseSv(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == ".." {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	return v, err == nil
}

// SkolaSchools is the aggregated school count for one municipality.
type SkolaSchools struct {
	MunicipalityCode string
	SchoolCount      int
}

// skolenhetList is the raw API response shape.
type skolenhetList struct {
	Skolenheter []struct {
		Kommunkod string `json:"Kommunkod"`
		Status    string `json:"Status"`
	} `json:"Skolenheter"`
}

// FetchSkolaSchools fetches the Skolverket school unit registry and returns
// the count of active schools per municipality (290 municipalities).
func (c *Client) FetchSkolaSchools(ctx context.Context) ([]SkolaSchools, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.skolverketRegistryURL+"/skolenhet", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skolverket: HTTP %d", resp.StatusCode)
	}

	var raw skolenhetList
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("skolverket decode: %w", err)
	}

	counts := make(map[string]int)
	for _, s := range raw.Skolenheter {
		if s.Status == "Aktiv" && s.Kommunkod != "" {
			counts[s.Kommunkod]++
		}
	}

	out := make([]SkolaSchools, 0, len(counts))
	for code, n := range counts {
		out = append(out, SkolaSchools{MunicipalityCode: code, SchoolCount: n})
	}
	return out, nil
}

// ── SALSA ─────────────────────────────────────────────────────────────────────

// SalsaMunicipality holds per-municipality SALSA aggregates derived from
// individual school records. geographicalAreaCode == municipality_code (4 digits).
type SalsaMunicipality struct {
	MunicipalityCode  string
	SchoolCount       int
	AvgActual         *float64
	AvgCalculated     *float64
	AvgDeviation      *float64
	ReqReachedActual  *float64
	ReqReachedDev     *float64
	ParentsEducation  *float64
}

type salsaResponse struct {
	Body struct {
		List []struct {
			SchoolUnitCode           string `json:"schoolUnitCode"`
			GeographicalAreaCode     string `json:"geographicalAreaCode"`
			SalsaActual              struct{ Value string } `json:"salsaAverageGradesIn9thGradeActual"`
			SalsaCalculated          struct{ Value string } `json:"salsaAverageCalculated"`
			SalsaDeviation           struct{ Value string } `json:"salsaAverageGradesIn9thGradeDeviation"`
			ReqReachedActual         struct{ Value string } `json:"salsaRequirementsReachedActual"`
			ReqReachedDeviation      struct{ Value string } `json:"salsaRequirementsReachedDeviation"`
			ParentsEducation         struct{ Value string } `json:"salsaParentsEducation"`
		} `json:"compulsorySchoolUnitSalsaMetricList"`
	} `json:"body"`
}

// FetchSalsaStats fetches SALSA statistics for all ~1500 grundskolor and
// aggregates them by municipality (geographicalAreaCode == 4-digit kommunkod).
func (c *Client) FetchSalsaStats(ctx context.Context) ([]SalsaMunicipality, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.skolverketPlanURL+"/v4/statistics/all-schools/salsa", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skolverket salsa: HTTP %d", resp.StatusCode)
	}

	var raw salsaResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("skolverket salsa decode: %w", err)
	}

	type accumulator struct {
		actual, calc, dev, req, reqDev, parents []float64
		count                                    int
	}
	byMuni := make(map[string]*accumulator)

	for _, s := range raw.Body.List {
		code := s.GeographicalAreaCode
		if code == "" {
			continue
		}
		acc := byMuni[code]
		if acc == nil {
			acc = &accumulator{}
			byMuni[code] = acc
		}
		acc.count++
		if v, ok := parseSv(s.SalsaActual.Value);     ok { acc.actual  = append(acc.actual,  v) }
		if v, ok := parseSv(s.SalsaCalculated.Value); ok { acc.calc    = append(acc.calc,    v) }
		if v, ok := parseSv(s.SalsaDeviation.Value);  ok { acc.dev     = append(acc.dev,     v) }
		if v, ok := parseSv(s.ReqReachedActual.Value);   ok { acc.req  = append(acc.req,     v) }
		if v, ok := parseSv(s.ReqReachedDeviation.Value); ok { acc.reqDev = append(acc.reqDev, v) }
		if v, ok := parseSv(s.ParentsEducation.Value); ok { acc.parents = append(acc.parents, v) }
	}

	avg := func(vals []float64) *float64 {
		if len(vals) == 0 {
			return nil
		}
		var sum float64
		for _, v := range vals {
			sum += v
		}
		r := sum / float64(len(vals))
		return &r
	}

	out := make([]SalsaMunicipality, 0, len(byMuni))
	for code, acc := range byMuni {
		out = append(out, SalsaMunicipality{
			MunicipalityCode: code,
			SchoolCount:      acc.count,
			AvgActual:        avg(acc.actual),
			AvgCalculated:    avg(acc.calc),
			AvgDeviation:     avg(acc.dev),
			ReqReachedActual: avg(acc.req),
			ReqReachedDev:    avg(acc.reqDev),
			ParentsEducation: avg(acc.parents),
		})
	}
	return out, nil
}

// ── National GR statistics ────────────────────────────────────────────────────

// NationalGrStats holds the latest national averages for grundskola.
type NationalGrStats struct {
	Period               string
	StudentsPerTeacher   float64
	CertifiedTeachersPct float64
	RatioPassed9th       float64
	AvgMerit9th          float64
	EligibleYRPct        float64
	EligibleESPct        float64
	RatioPassed6th       float64
	TestSve6             float64
	TestEng6             float64
	TestMa6              float64
	TestSve9             float64
	TestEng9             float64
	TestMa9              float64
}

type nationalGrResponse struct {
	Body struct {
		StudentsPerTeacherQuota                            []struct{ Value, TimePeriod string } `json:"studentsPerTeacherQuota"`
		CertifiedTeachersQuota                             []struct{ Value, TimePeriod string } `json:"certifiedTeachersQuota"`
		RatioOfPupilsIn9thGradeWithAllSubjectsPassed       []struct{ Value, TimePeriod string } `json:"ratioOfPupilsIn9thGradeWithAllSubjectsPassed"`
		AverageGradesMeritRating9thGrade                   []struct{ Value, TimePeriod string } `json:"averageGradesMeritRating9thGrade"`
		RatioOfPupils9thGradeEligibleForNationalProgramYR  []struct{ Value, TimePeriod string } `json:"ratioOfPupils9thGradeEligibleForNationalProgramYR"`
		RatioOfPupils9thGradeEligibleForNationalProgramES  []struct{ Value, TimePeriod string } `json:"ratioOfPupils9thGradeEligibleForNationalProgramES"`
		RatioOfPupilsIn6thGradeWithAllSubjectsPassed       []struct{ Value, TimePeriod string } `json:"ratioOfPupilsIn6thGradeWithAllSubjectsPassed"`
		TestSve6 []struct{ Value, TimePeriod string } `json:"averageResultNationalTestsSubjectSVE6thGrade"`
		TestEng6 []struct{ Value, TimePeriod string } `json:"averageResultNationalTestsSubjectENG6thGrade"`
		TestMa6  []struct{ Value, TimePeriod string } `json:"averageResultNationalTestsSubjectMA6thGrade"`
		TestSve9 []struct{ Value, TimePeriod string } `json:"averageResultNationalTestsSubjectSVE9thGrade"`
		TestEng9 []struct{ Value, TimePeriod string } `json:"averageResultNationalTestsSubjectENG9thGrade"`
		TestMa9  []struct{ Value, TimePeriod string } `json:"averageResultNationalTestsSubjectMA9thGrade"`
	} `json:"body"`
}

// FetchNationalGrStats fetches current national averages for grundskola.
func (c *Client) FetchNationalGrStats(ctx context.Context) (*NationalGrStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.skolverketPlanURL+"/v4/statistics/national-values/gr", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "swedennu/1.0 (national open data dashboard)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skolverket national-gr: HTTP %d", resp.StatusCode)
	}

	var raw nationalGrResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("skolverket national-gr decode: %w", err)
	}

	first := func(rows []struct{ Value, TimePeriod string }) (float64, string) {
		for _, r := range rows {
			if v, ok := parseSv(r.Value); ok {
				return v, r.TimePeriod
			}
		}
		return 0, ""
	}

	spt, period := first(raw.Body.StudentsPerTeacherQuota)
	cert, _      := first(raw.Body.CertifiedTeachersQuota)
	pass9, _     := first(raw.Body.RatioOfPupilsIn9thGradeWithAllSubjectsPassed)
	merit, _     := first(raw.Body.AverageGradesMeritRating9thGrade)
	yr, _        := first(raw.Body.RatioOfPupils9thGradeEligibleForNationalProgramYR)
	es, _        := first(raw.Body.RatioOfPupils9thGradeEligibleForNationalProgramES)
	pass6, _     := first(raw.Body.RatioOfPupilsIn6thGradeWithAllSubjectsPassed)
	sve6, _      := first(raw.Body.TestSve6)
	eng6, _      := first(raw.Body.TestEng6)
	ma6, _       := first(raw.Body.TestMa6)
	sve9, _      := first(raw.Body.TestSve9)
	eng9, _      := first(raw.Body.TestEng9)
	ma9, _       := first(raw.Body.TestMa9)

	if period == "" {
		return nil, fmt.Errorf("skolverket national-gr: no data in response")
	}

	return &NationalGrStats{
		Period:               period,
		StudentsPerTeacher:   spt,
		CertifiedTeachersPct: cert,
		RatioPassed9th:       pass9,
		AvgMerit9th:          merit,
		EligibleYRPct:        yr,
		EligibleESPct:        es,
		RatioPassed6th:       pass6,
		TestSve6:             sve6,
		TestEng6:             eng6,
		TestMa6:              ma6,
		TestSve9:             sve9,
		TestEng9:             eng9,
		TestMa9:              ma9,
	}, nil
}
