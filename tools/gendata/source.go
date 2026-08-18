package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// kenAllColumns is the column count of utf_ken_all.csv. A change here means
// Japan Post revised the layout and the rest of this tool needs review.
const kenAllColumns = 15

// kenRow is one line of utf_ken_all.csv, limited to the columns we keep.
// The trailing flag columns describe how Japan Post split the data and carry
// nothing a caller of the library would ask for.
type kenRow struct {
	JIS      string // 全国地方公共団体コード, 5 digits
	Zip      string // 郵便番号, 7 digits
	PrefKana string
	CityKana string
	TownKana string
	Pref     string
	City     string
	Town     string
}

func parseKenAll(b []byte) ([]kenRow, error) {
	r := csv.NewReader(bytes.NewReader(dropBOM(b)))
	r.FieldsPerRecord = kenAllColumns
	r.ReuseRecord = true

	rows := make([]kenRow, 0, 130000)
	for line := 1; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("utf_ken_all.csv line %d: %w", line, err)
		}
		rows = append(rows, kenRow{
			JIS:      strings.TrimSpace(rec[0]),
			Zip:      strings.TrimSpace(rec[2]),
			PrefKana: strings.TrimSpace(rec[3]),
			CityKana: strings.TrimSpace(rec[4]),
			TownKana: strings.TrimSpace(rec[5]),
			Pref:     strings.TrimSpace(rec[6]),
			City:     strings.TrimSpace(rec[7]),
			Town:     strings.TrimSpace(rec[8]),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("utf_ken_all.csv is empty")
	}
	return rows, nil
}

// romeColumns is the column count of KEN_ALL_ROME.CSV.
const romeColumns = 7

// romeRow is one line of KEN_ALL_ROME.CSV. Its kanji columns pad names with
// full-width spaces that utf_ken_all.csv does not have (「札幌市　中央区」 vs
// 「札幌市中央区」), so the joinable forms are stored with spaces removed.
type romeRow struct {
	Zip        string
	Pref       string // spaces removed
	City       string // spaces removed
	Town       string // spaces removed
	PrefRomaji string
	CityRomaji string
	TownRomaji string
}

// KEN_ALL_ROME.CSV is still published in Shift-JIS; only the ken_all data got
// a UTF-8 edition.
func parseRome(b []byte) ([]romeRow, error) {
	dec := transform.NewReader(bytes.NewReader(b), japanese.ShiftJIS.NewDecoder())
	r := csv.NewReader(dec)
	r.FieldsPerRecord = romeColumns
	r.ReuseRecord = true

	rows := make([]romeRow, 0, 130000)
	for line := 1; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("KEN_ALL_ROME.CSV line %d: %w", line, err)
		}
		rows = append(rows, romeRow{
			Zip:        strings.TrimSpace(rec[0]),
			Pref:       squeeze(rec[1]),
			City:       squeeze(rec[2]),
			Town:       squeeze(rec[3]),
			PrefRomaji: strings.TrimSpace(rec[4]),
			CityRomaji: strings.TrimSpace(rec[5]),
			TownRomaji: strings.TrimSpace(rec[6]),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("KEN_ALL_ROME.CSV is empty")
	}
	return rows, nil
}

// squeeze removes every ASCII and full-width space.
func squeeze(s string) string {
	return strings.NewReplacer(" ", "", "\t", "", "　", "").Replace(s)
}

func dropBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
}
