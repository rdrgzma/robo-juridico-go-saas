package calculator

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

func importarXLSX(content []byte) ([]ParcelaItem, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("XLSX inválido")
	}
	if len(zr.File) > 100 {
		return nil, fmt.Errorf("XLSX excede limite de entradas")
	}
	members := map[string][]byte{}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if f.UncompressedSize64 > 8<<20 || total > 16<<20 {
			return nil, fmt.Errorf("XLSX expandido excede limite")
		}
		if !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, (8<<20)+1))
		_ = rc.Close()
		if err != nil || len(b) > 8<<20 {
			return nil, fmt.Errorf("XML inválido ou excessivo")
		}
		if _, ok := members[f.Name]; ok {
			return nil, fmt.Errorf("entrada duplicada")
		}
		members[f.Name] = b
	}
	var shared struct {
		Items []struct {
			Text string `xml:"t"`
			Runs []struct {
				Text string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if b := members["xl/sharedStrings.xml"]; b != nil {
		if err := xml.Unmarshal(b, &shared); err != nil {
			return nil, fmt.Errorf("sharedStrings inválido")
		}
	}
	sheets := []string{}
	for name := range members {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			sheets = append(sheets, name)
		}
	}
	sort.Strings(sheets)
	if len(sheets) == 0 {
		return nil, fmt.Errorf("XLSX sem planilha")
	}
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline struct {
					Text string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(members[sheets[0]], &sheet); err != nil {
		return nil, fmt.Errorf("planilha XML inválida")
	}
	if len(sheet.Rows) > 1001 {
		return nil, fmt.Errorf("mais de 1000 linhas de dados")
	}
	table := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		values := []string{}
		for _, cell := range row.Cells {
			column := 0
			for _, c := range cell.Ref {
				if c < 'A' || c > 'Z' {
					break
				}
				column = column*26 + int(c-'A'+1)
			}
			if column == 0 {
				column = len(values) + 1
			}
			if column > 64 || column <= len(values) {
				return nil, fmt.Errorf("coluna inválida, duplicada ou maior que 64")
			}
			for len(values) < column {
				values = append(values, "")
			}
			value := cell.Value
			if cell.Type == "inlineStr" {
				value = cell.Inline.Text
			}
			if cell.Type == "s" {
				idx, err := strconv.Atoi(cell.Value)
				if err != nil || idx < 0 || idx >= len(shared.Items) {
					return nil, fmt.Errorf("referência sharedString inválida")
				}
				item := shared.Items[idx]
				value = item.Text
				for _, run := range item.Runs {
					value += run.Text
				}
			}
			if len(value) > 8192 {
				return nil, fmt.Errorf("célula excessiva")
			}
			values[column-1] = value
		}
		table = append(table, values)
	}
	return processarLinhasTabela(table)
}
