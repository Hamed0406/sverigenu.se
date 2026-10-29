package fetcher

import (
	"encoding/json"
	"testing"
)

// TestFlattenJSONStat2 locks in the row-major index arithmetic with a 2×3×2
// cube (Region × Metric × Tid) including a null cell.
func TestFlattenJSONStat2(t *testing.T) {
	raw := `{
		"id": ["Region", "Metric", "Tid"],
		"size": [2, 3, 2],
		"dimension": {
			"Region": {"category": {
				"index": {"01": 0, "03": 1},
				"label": {"01": "Stockholms län", "03": "Uppsala län"}}},
			"Metric": {"category": {
				"index": {"A": 0, "B": 1, "C": 2},
				"label": {"A": "Alpha", "B": "Beta", "C": "Gamma"}}},
			"Tid": {"category": {
				"index": {"2023": 0, "2024": 1},
				"label": {"2023": "2023", "2024": "2024"}}}
		},
		"value": [1, 2, 3, 4, 5, 6, 7, 8, 9, null, 11, 12]
	}`
	var js jsonStat2
	if err := json.Unmarshal([]byte(raw), &js); err != nil {
		t.Fatalf("fixture unmarshal: %v", err)
	}

	cells := flattenJSONStat2(&js)
	if len(cells) != 11 { // 12 cells minus 1 null
		t.Fatalf("got %d cells, want 11", len(cells))
	}

	// value[0]=1 → Region 01, Metric A, Tid 2023
	first := cells[0]
	if first.Codes["Region"] != "01" || first.Codes["Metric"] != "A" || first.Codes["Tid"] != "2023" || first.Value != 1 {
		t.Errorf("cell 0 wrong: %+v", first)
	}
	if first.Labels["Region"] != "Stockholms län" {
		t.Errorf("cell 0 label wrong: %+v", first.Labels)
	}

	// value[11]=12 is at Region 03 (idx/6=1), Metric C ((11/2)%3=2), Tid 2024 (11%2=1).
	last := cells[len(cells)-1]
	if last.Codes["Region"] != "03" || last.Codes["Metric"] != "C" || last.Codes["Tid"] != "2024" || last.Value != 12 {
		t.Errorf("last cell wrong: %+v", last)
	}

	// The null at index 9 (Region 03, Metric B, Tid 2024) must be skipped.
	for _, c := range cells {
		if c.Codes["Region"] == "03" && c.Codes["Metric"] == "B" && c.Codes["Tid"] == "2024" {
			t.Errorf("null cell was emitted: %+v", c)
		}
	}
}

func TestFlattenJSONStat2Malformed(t *testing.T) {
	// Empty index map must return nil, not garbage.
	js := &jsonStat2{ID: []string{"X"}, Size: []int{1}}
	if cells := flattenJSONStat2(js); cells != nil {
		t.Errorf("malformed cube should return nil, got %d cells", len(cells))
	}
}
