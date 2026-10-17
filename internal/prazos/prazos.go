// Package prazos counts dates using an explicit calendar. It does not infer local rules.
package prazos

import (
	"fmt"
	"time"
)

type Request struct {
	Start    string   `json:"data_evento"`
	Days     int      `json:"dias"`
	Mode     string   `json:"modo"`
	Notice   string   `json:"intimacao"`
	Double   bool     `json:"dobro"`
	Recess   bool     `json:"suspender_recesso"`
	Holidays []string `json:"feriados"`
}

type Day struct {
	Date   string `json:"data"`
	Count  int    `json:"contado"`
	Reason string `json:"motivo"`
}
type Result struct {
	Publication string `json:"publicacao"`
	Due         string `json:"vencimento"`
	Calendar    []Day  `json:"memoria"`
}

func Calculate(r Request) (Result, error) {
	var out Result
	start, err := time.Parse("2006-01-02", r.Start)
	if err != nil || start.Year() < 1900 || start.Year() > 2090 || r.Days < 1 || r.Days > 365 || len(r.Holidays) > 1000 {
		return out, fmt.Errorf("data, dias ou calendário inválido")
	}
	if r.Mode != "UTEIS" && r.Mode != "CORRIDOS" {
		return out, fmt.Errorf("modo deve ser UTEIS ou CORRIDOS")
	}
	if r.Notice != "DIRETA" && r.Notice != "DIARIO" {
		return out, fmt.Errorf("intimação deve ser DIRETA ou DIARIO")
	}
	holidays := map[string]bool{}
	for _, s := range r.Holidays {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return out, fmt.Errorf("feriado inválido")
		}
		holidays[s] = true
	}
	reason := func(t time.Time) string {
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			return "fim de semana"
		}
		if holidays[t.Format("2006-01-02")] {
			return "feriado informado"
		}
		if r.Recess && ((t.Month() == time.December && t.Day() >= 20) || (t.Month() == time.January && t.Day() <= 20)) {
			return "suspensão configurada"
		}
		return ""
	}
	publication := start
	if r.Notice == "DIARIO" {
		publication = publication.AddDate(0, 0, 1)
		for reason(publication) != "" {
			publication = publication.AddDate(0, 0, 1)
		}
	}
	out.Publication = publication.Format("2006-01-02")
	days := r.Days
	if r.Double {
		days *= 2
	}
	count := 0
	for cursor, steps := publication.AddDate(0, 0, 1), 0; steps < 6000; cursor, steps = cursor.AddDate(0, 0, 1), steps+1 {
		why := reason(cursor)
		if count < days && (r.Mode == "CORRIDOS" || why == "") {
			count++
		}
		out.Calendar = append(out.Calendar, Day{cursor.Format("2006-01-02"), count, why})
		if count == days && why == "" {
			out.Due = cursor.Format("2006-01-02")
			return out, nil
		}
	}
	return Result{}, fmt.Errorf("calendário excedeu horizonte de cálculo")
}
