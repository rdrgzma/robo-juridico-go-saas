package prazos

import "testing"

func TestCalendarFixtures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request Request
		due     string
	}{
		{"direta", Request{Start: "2026-10-16", Days: 2, Mode: "UTEIS", Notice: "DIRETA"}, "2026-10-20"},
		{"diario", Request{Start: "2026-10-16", Days: 2, Mode: "UTEIS", Notice: "DIARIO"}, "2026-10-21"},
		{"feriado", Request{Start: "2026-10-16", Days: 2, Mode: "UTEIS", Notice: "DIRETA", Holidays: []string{"2026-10-19"}}, "2026-10-21"},
		{"corridos", Request{Start: "2026-10-16", Days: 1, Mode: "CORRIDOS", Notice: "DIRETA"}, "2026-10-19"},
		{"dobro", Request{Start: "2026-10-16", Days: 2, Mode: "UTEIS", Notice: "DIRETA", Double: true}, "2026-10-22"},
		{"recesso", Request{Start: "2026-12-18", Days: 2, Mode: "UTEIS", Notice: "DIRETA", Recess: true}, "2027-01-22"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Calculate(tc.request)
			if err != nil || r.Due != tc.due {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestInvalidCalendarRejected(t *testing.T) {
	for _, r := range []Request{{Start: "2026-02-30", Days: 1, Mode: "UTEIS", Notice: "DIRETA"}, {Start: "2026-10-16", Days: 1, Mode: "TYPO", Notice: "DIRETA"}, {Start: "2026-10-16", Days: 1, Mode: "UTEIS", Notice: "DIRETA", Holidays: []string{"bad"}}} {
		if _, err := Calculate(r); err == nil {
			t.Fatal(r)
		}
	}
}
