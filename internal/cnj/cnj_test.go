package cnj

import "testing"

func TestKnownNumber(t *testing.T) {
	// Independent fixture: 0000001 + 2023 + 8 + 26 + 0100 + 00 has remainder 25.
	for _, s := range []string{"0000001-73.2023.8.26.0100", "00000017320238260100"} {
		n, err := Parse(s)
		if err != nil || !n.Valid || n.Year != 2023 || n.CourtCode != 26 {
			t.Fatalf("%s: %+v %v", s, n, err)
		}
	}
}

func TestRejectMalformed(t *testing.T) {
	for _, s := range []string{"", "abc00000017320238260100", "0000001-00.2023.8.26.0100", "0000001-62.2023.0.26.0100", "0000001-62.2023.8.2.0100", "00000017320238260100<script>", "１２３"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add("0000001-73.2023.8.26.0100")
	f.Add("x")
	f.Fuzz(func(t *testing.T, s string) {
		n, err := Parse(s)
		if err == nil && (!n.Valid || len(n.Digits) != 20) {
			t.Fatal(n)
		}
	})
}
