// Package cnj validates the numeric CNJ identifier without network access.
package cnj

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var plain = regexp.MustCompile(`^[0-9]{20}$`)
var formatted = regexp.MustCompile(`^[0-9]{7}-[0-9]{2}\.[0-9]{4}\.[1-9]\.[0-9]{2}\.[0-9]{4}$`)

type Number struct {
	Original  string `json:"numero_original"`
	Formatted string `json:"formatado"`
	Digits    string `json:"digitos"`
	Valid     bool   `json:"valido"`
	Year      int    `json:"ano"`
	Segment   int    `json:"segmento"`
	CourtCode int    `json:"tribunal_codigo"`
}

func Parse(input string) (Number, error) {
	s := strings.TrimSpace(input)
	n := Number{Original: s}
	if !plain.MatchString(s) && !formatted.MatchString(s) {
		return n, fmt.Errorf("CNJ deve conter 20 dígitos ou a máscara NNNNNNN-DD.AAAA.J.TR.OOOO")
	}
	d := strings.NewReplacer("-", "", ".", "").Replace(s)
	n.Digits = d
	n.Formatted = fmt.Sprintf("%s-%s.%s.%s.%s.%s", d[:7], d[7:9], d[9:13], d[13:14], d[14:16], d[16:])
	n.Year, _ = strconv.Atoi(d[9:13])
	n.Segment, _ = strconv.Atoi(d[13:14])
	n.CourtCode, _ = strconv.Atoi(d[14:16])
	if n.Year == 0 || n.Segment < 1 || n.Segment > 9 {
		return n, fmt.Errorf("ano ou segmento inválido")
	}
	// Streaming remainder avoids integer overflow and rejects arbitrary text.
	rem := 0
	for _, c := range d[:7] + d[9:] + "00" {
		rem = (rem*10 + int(c-'0')) % 97
	}
	dv, _ := strconv.Atoi(d[7:9])
	n.Valid = dv == 98-rem
	if !n.Valid {
		return n, fmt.Errorf("dígito verificador CNJ inválido")
	}
	return n, nil
}
