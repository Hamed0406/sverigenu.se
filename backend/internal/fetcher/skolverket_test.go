package fetcher

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── parseSv ───────────────────────────────────────────────────────────────────

func TestParseSv(t *testing.T) {
	cases := []struct {
		in      string
		wantVal float64
		wantOK  bool
	}{
		{"11,9", 11.9, true},
		{"228,5", 228.5, true},
		{"0", 0, true},
		{"-3,5", -3.5, true},
		{"", 0, false},
		{"..", 0, false},
		{"   ", 0, false},
		{"n/a", 0, false},
		{"  12,8  ", 12.8, true}, // leading/trailing space
	}
	for _, c := range cases {
		v, ok := parseSv(c.in)
		if ok != c.wantOK {
			t.Errorf("parseSv(%q) ok=%v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if ok && v != c.wantVal {
			t.Errorf("parseSv(%q) = %v, want %v", c.in, v, c.wantVal)
		}
	}
}

// ── FetchSkolaSchools ─────────────────────────────────────────────────────────

func TestFetchSkolaSchools(t *testing.T) {
	body := `{"Skolenheter":[
		{"Kommunkod":"0180","Status":"Aktiv"},
		{"Kommunkod":"0180","Status":"Aktiv"},
		{"Kommunkod":"0480","Status":"Aktiv"},
		{"Kommunkod":"0180","Status":"Nedlagd"},
		{"Kommunkod":"","Status":"Aktiv"}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.skolverketRegistryURL = srv.URL

	schools, err := c.FetchSkolaSchools(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}

	// 2 active for 0180, 1 active for 0480; Nedlagd and empty code are excluded.
	byCode := make(map[string]int)
	for _, s := range schools {
		byCode[s.MunicipalityCode] = s.SchoolCount
	}
	if byCode["0180"] != 2 {
		t.Errorf("0180 count = %d, want 2", byCode["0180"])
	}
	if byCode["0480"] != 1 {
		t.Errorf("0480 count = %d, want 1", byCode["0480"])
	}
	if _, ok := byCode[""]; ok {
		t.Error("empty municipality code should be excluded")
	}
}

func TestFetchSkolaSchoolsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.skolverketRegistryURL = srv.URL
	_, err := c.FetchSkolaSchools(testCtx(t))
	if err == nil {
		t.Error("expected error for non-200 status")
	}
}

// ── FetchSalsaStats ───────────────────────────────────────────────────────────

func TestFetchSalsaStats(t *testing.T) {
	// Two schools in muni 0180, one in 0480. Values use Swedish decimal format.
	body := `{"body":{"compulsorySchoolUnitSalsaMetricList":[
		{"schoolUnitCode":"S1","geographicalAreaCode":"0180",
		 "salsaAverageGradesIn9thGradeActual":{"value":"230,0"},
		 "salsaAverageCalculated":{"value":"210,0"},
		 "salsaAverageGradesIn9thGradeDeviation":{"value":"20,0"},
		 "salsaRequirementsReachedActual":{"value":"75,0"},
		 "salsaRequirementsReachedDeviation":{"value":"5,0"},
		 "salsaParentsEducation":{"value":"2,1"}},
		{"schoolUnitCode":"S2","geographicalAreaCode":"0180",
		 "salsaAverageGradesIn9thGradeActual":{"value":"220,0"},
		 "salsaAverageCalculated":{"value":"200,0"},
		 "salsaAverageGradesIn9thGradeDeviation":{"value":"10,0"},
		 "salsaRequirementsReachedActual":{"value":"65,0"},
		 "salsaRequirementsReachedDeviation":{"value":"3,0"},
		 "salsaParentsEducation":{"value":"1,9"}},
		{"schoolUnitCode":"S3","geographicalAreaCode":"0480",
		 "salsaAverageGradesIn9thGradeActual":{"value":".."},
		 "salsaAverageCalculated":{"value":""},
		 "salsaAverageGradesIn9thGradeDeviation":{"value":"5,0"},
		 "salsaRequirementsReachedActual":{"value":".."},
		 "salsaRequirementsReachedDeviation":{"value":".."},
		 "salsaParentsEducation":{"value":"2,0"}},
		{"schoolUnitCode":"S4","geographicalAreaCode":"",
		 "salsaAverageGradesIn9thGradeActual":{"value":"200,0"},
		 "salsaAverageCalculated":{"value":"190,0"},
		 "salsaAverageGradesIn9thGradeDeviation":{"value":"10,0"},
		 "salsaRequirementsReachedActual":{"value":"60,0"},
		 "salsaRequirementsReachedDeviation":{"value":"2,0"},
		 "salsaParentsEducation":{"value":"2,0"}}
	]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.skolverketPlanURL = srv.URL

	rows, err := c.FetchSalsaStats(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}

	byCode := make(map[string]*SalsaMunicipality)
	for i := range rows {
		byCode[rows[i].MunicipalityCode] = &rows[i]
	}

	// Empty code must be skipped.
	if _, ok := byCode[""]; ok {
		t.Error("empty geographicalAreaCode should be excluded")
	}

	// 0180: two schools averaged.
	m0180, ok := byCode["0180"]
	if !ok {
		t.Fatal("0180 missing from results")
	}
	if m0180.SchoolCount != 2 {
		t.Errorf("0180 SchoolCount = %d, want 2", m0180.SchoolCount)
	}
	wantActual := (230.0 + 220.0) / 2
	if m0180.AvgActual == nil || *m0180.AvgActual != wantActual {
		t.Errorf("0180 AvgActual = %v, want %v", m0180.AvgActual, wantActual)
	}
	wantDev := (20.0 + 10.0) / 2
	if m0180.AvgDeviation == nil || *m0180.AvgDeviation != wantDev {
		t.Errorf("0180 AvgDeviation = %v, want %v", m0180.AvgDeviation, wantDev)
	}

	// 0480: only one school; ".." and "" values should produce nil fields.
	m0480, ok := byCode["0480"]
	if !ok {
		t.Fatal("0480 missing from results")
	}
	if m0480.AvgActual != nil {
		t.Errorf("0480 AvgActual should be nil (value was '..'), got %v", *m0480.AvgActual)
	}
	if m0480.AvgDeviation == nil || *m0480.AvgDeviation != 5.0 {
		t.Errorf("0480 AvgDeviation = %v, want 5.0", m0480.AvgDeviation)
	}
}

// ── FetchNationalGrStats ──────────────────────────────────────────────────────

func TestFetchNationalGrStats(t *testing.T) {
	body := `{"body":{
		"studentsPerTeacherQuota":[{"value":"11,9","timePeriod":"2025/26"}],
		"certifiedTeachersQuota":[{"value":"73,4","timePeriod":"2025/26"}],
		"ratioOfPupilsIn9thGradeWithAllSubjectsPassed":[{"value":"71,9","timePeriod":"2025/26"}],
		"averageGradesMeritRating9thGrade":[{"value":"228,5","timePeriod":"2025/26"}],
		"ratioOfPupils9thGradeEligibleForNationalProgramYR":[{"value":"84,1","timePeriod":"2025/26"}],
		"ratioOfPupils9thGradeEligibleForNationalProgramES":[{"value":"82,8","timePeriod":"2025/26"}],
		"ratioOfPupilsIn6thGradeWithAllSubjectsPassed":[{"value":"70,4","timePeriod":"2024/25"}],
		"averageResultNationalTestsSubjectSVE6thGrade":[{"value":"12,8","timePeriod":"2024/25"}],
		"averageResultNationalTestsSubjectENG6thGrade":[{"value":"15,8","timePeriod":"2024/25"}],
		"averageResultNationalTestsSubjectMA6thGrade":[{"value":"11,6","timePeriod":"2024/25"}],
		"averageResultNationalTestsSubjectSVE9thGrade":[{"value":"13,1","timePeriod":"2024/25"}],
		"averageResultNationalTestsSubjectENG9thGrade":[{"value":"15,8","timePeriod":"2024/25"}],
		"averageResultNationalTestsSubjectMA9thGrade":[{"value":"11,4","timePeriod":"2024/25"}]
	}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.skolverketPlanURL = srv.URL

	stats, err := c.FetchNationalGrStats(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		field string
		got   float64
		want  float64
	}{
		{"StudentsPerTeacher", stats.StudentsPerTeacher, 11.9},
		{"CertifiedTeachersPct", stats.CertifiedTeachersPct, 73.4},
		{"RatioPassed9th", stats.RatioPassed9th, 71.9},
		{"AvgMerit9th", stats.AvgMerit9th, 228.5},
		{"EligibleYRPct", stats.EligibleYRPct, 84.1},
		{"EligibleESPct", stats.EligibleESPct, 82.8},
		{"RatioPassed6th", stats.RatioPassed6th, 70.4},
		{"TestSve6", stats.TestSve6, 12.8},
		{"TestEng6", stats.TestEng6, 15.8},
		{"TestMa6", stats.TestMa6, 11.6},
		{"TestSve9", stats.TestSve9, 13.1},
		{"TestEng9", stats.TestEng9, 15.8},
		{"TestMa9", stats.TestMa9, 11.4},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
	if stats.Period != "2025/26" {
		t.Errorf("Period = %q, want %q", stats.Period, "2025/26")
	}
}

func TestFetchNationalGrStatsEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return valid JSON but with no data in the period fields.
		json.NewEncoder(w).Encode(map[string]any{"body": map[string]any{}})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.skolverketPlanURL = srv.URL

	_, err := c.FetchNationalGrStats(testCtx(t))
	if err == nil {
		t.Error("expected error when response has no period data")
	}
}
