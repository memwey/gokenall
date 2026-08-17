package main

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func TestParseKenAll(t *testing.T) {
	const csv = `01101,"060  ","0600000","ホッカイドウ","サッポロシチュウオウク","イカニケイサイガナイバアイ","北海道","札幌市中央区","以下に掲載がない場合",0,0,0,0,0,0
13101,"100  ","1000001","トウキョウト","チヨダク","チヨダ","東京都","千代田区","千代田",0,0,0,0,0,0
`
	rows, err := parseKenAll([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	want := kenRow{
		JIS: "13101", Zip: "1000001",
		PrefKana: "トウキョウト", CityKana: "チヨダク", TownKana: "チヨダ",
		Pref: "東京都", City: "千代田区", Town: "千代田",
	}
	if rows[1] != want {
		t.Errorf("row 1 = %#v, want %#v", rows[1], want)
	}
	// The old zip code column is padded with spaces; every field is trimmed.
	if strings.ContainsAny(rows[0].Zip, " ") {
		t.Errorf("zip %q was not trimmed", rows[0].Zip)
	}
}

func TestParseKenAllRejectsBadInput(t *testing.T) {
	tests := map[string]string{
		"a shorter layout than Japan Post publishes": `13101,"100  ","1000001","トウキョウト"` + "\n",
		"an empty file": "",
	}
	for name, csv := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseKenAll([]byte(csv)); err == nil {
				t.Error("parseKenAll accepted it")
			}
		})
	}
}

func TestParseRome(t *testing.T) {
	const csv = `"0600000","北海道","札幌市　中央区","以下に掲載がない場合","HOKKAIDO","SAPPORO SHI CHUO KU","IKANIKEISAIGANAIBAAI"
"0640951","北海道","札幌市　中央区","宮の森　一条","HOKKAIDO","SAPPORO SHI CHUO KU","MIYANOMORI 1-JO"
`
	rows, err := parseRome(toShiftJIS(t, csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Japan Post pads the kanji columns with full-width spaces that
	// utf_ken_all.csv does not have; joining depends on them being gone.
	want := romeRow{
		Zip: "0640951", Pref: "北海道", City: "札幌市中央区", Town: "宮の森一条",
		PrefRomaji: "HOKKAIDO", CityRomaji: "SAPPORO SHI CHUO KU", TownRomaji: "MIYANOMORI 1-JO",
	}
	if rows[1] != want {
		t.Errorf("row 1 = %#v, want %#v", rows[1], want)
	}
}

func TestParseRomeRejectsUTF8(t *testing.T) {
	// A UTF-8 file would decode as mojibake rather than fail outright, so the
	// point here is only that valid Shift-JIS is what the parser is built for.
	rows, err := parseRome([]byte(`"0600000","北海道","札幌市","x","A","B","C"` + "\n"))
	if err != nil {
		return
	}
	if rows[0].Pref == "北海道" {
		t.Error("parseRome read UTF-8 as if it were Shift-JIS and got lucky; the decoder is not being applied")
	}
}

func TestSqueeze(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"札幌市　中央区", "札幌市中央区"},
		{"宮の森　一条", "宮の森一条"},
		{"千代田区", "千代田区"},
		{" a\tb ", "ab"},
	} {
		if got := squeeze(tt.in); got != tt.want {
			t.Errorf("squeeze(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDropBOM(t *testing.T) {
	if got := string(dropBOM([]byte("\xef\xbb\xbfabc"))); got != "abc" {
		t.Errorf("dropBOM = %q", got)
	}
	if got := string(dropBOM([]byte("abc"))); got != "abc" {
		t.Errorf("dropBOM = %q", got)
	}
}

func toShiftJIS(t *testing.T, s string) []byte {
	t.Helper()
	b, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("encode to Shift-JIS: %v", err)
	}
	return b
}
