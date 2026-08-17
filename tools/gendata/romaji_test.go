package main

import "testing"

func TestPrettyCity(t *testing.T) {
	tests := []struct{ in, want string }{
		{"CHIYODA KU", "Chiyoda-ku"},
		{"SAPPORO SHI CHUO KU", "Sapporo-shi Chuo-ku"},
		{"YAEYAMA GUN YONAGUNI CHO", "Yaeyama-gun Yonaguni-cho"},
		{"KUWANA GUN KISOSAKI CHO", "Kuwana-gun Kisosaki-cho"},
		{"NISHITAMA GUN HINOHARA MURA", "Nishitama-gun Hinohara-mura"},
		{"SHIMAJIRI GUN AGUNI SON", "Shimajiri-gun Aguni-son"},
		{"YOKOTE SHI", "Yokote-shi"},
		{"HACHIJO MACHI", "Hachijo-machi"},
	}
	for _, tt := range tests {
		if got := prettyCity(tt.in); got != tt.want {
			t.Errorf("prettyCity(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPrettyTown(t *testing.T) {
	tests := []struct{ in, want string }{
		{"CHIYODA", "Chiyoda"},
		{"ODORINISHI", "Odorinishi"},
		{"MIYANOMORI 1-JO", "Miyanomori 1-jo"},
		{"KITA1-JONISHI", "Kita1-jonishi"},
		{"SHINOROCHO KAMISHINORO", "Shinorocho Kamishinoro"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := prettyTown(tt.in); got != tt.want {
			t.Errorf("prettyTown(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRomajiIndexSkipsTruncatedRecords(t *testing.T) {
	rows := []romeRow{
		{Zip: "1000001", Pref: "東京都", City: "千代田区", Town: "千代田",
			PrefRomaji: "TOKYO TO", CityRomaji: "CHIYODA KU", TownRomaji: "CHIYODA"},
		// A fragment: 17 kanji with no annotation, 35 romaji with no "(".
		{Zip: "0788201", Pref: "北海道", City: "旭川市", Town: "東旭川町東桜岡（３０〜４９９番地",
			PrefRomaji: "HOKKAIDO", CityRomaji: "ASAHIKAWA SHI",
			TownRomaji: "HIGASHIASAHIKAWACHO HIGASHISAKURAOK"},
	}
	ix, err := newRomajiIndex(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := ix.prefecture("東京都"); !ok || got != "TOKYO TO" {
		t.Errorf("published prefecture romaji = %q, %v; want TOKYO TO", got, ok)
	}

	if got := ix.towns[townKey{"1000001", "東京都", "千代田区", "千代田"}]; got != "CHIYODA" {
		t.Errorf("complete record indexed as %q, want CHIYODA", got)
	}
	if _, ok := ix.towns[townKey{"0788201", "北海道", "旭川市", "東旭川町東桜岡"}]; ok {
		t.Error("a truncated romaji spelling was indexed")
	}
	if len(ix.byZip["0788201"]) != 0 {
		t.Errorf("truncated record reached byZip: %q", ix.byZip["0788201"])
	}
	// The city columns are short enough never to be cut, so they are kept even
	// from a record whose town half is a fragment.
	if got, ok := ix.city("北海道", "旭川市"); !ok || got != "ASAHIKAWA SHI" {
		t.Errorf("city romaji = %q, %v; want ASAHIKAWA SHI", got, ok)
	}
}

func TestRomajiIndexRejectsConflictingPrefectureSpellings(t *testing.T) {
	rows := []romeRow{
		{Pref: "群馬県", PrefRomaji: "GUMMA KEN"},
		{Pref: "群馬県", PrefRomaji: "GUNMA KEN"},
	}
	if _, err := newRomajiIndex(rows); err == nil {
		t.Fatal("newRomajiIndex accepted conflicting source spellings")
	}
}
