package indices

import "testing"

func TestCompleteSeriesAndNoSilentFallback(t *testing.T) {
	points := []Point{{"2026-01", "1", "fixture"}, {"2026-02", "-0.5", "fixture"}}
	factor, err := Factor("2026-01", "2026-02", points, true)
	if err != nil || factor.String() != "1.00495" {
		t.Fatal(factor, err)
	}
	factor, err = Factor("2026-01", "2026-02", points, false)
	if err != nil || factor.String() != "1.01" {
		t.Fatal(factor, err)
	}
	if _, err := Factor("2026-01", "2026-03", points, true); err == nil {
		t.Fatal("missing month accepted")
	}
	if _, err := Factor("2026-01", "2026-02", append(points, points[0]), true); err == nil {
		t.Fatal("duplicate month accepted")
	}
	if _, err := Factor("2026-01", "2026-01", []Point{{"2026-01", "1", ""}}, true); err == nil {
		t.Fatal("missing provenance accepted")
	}
}
