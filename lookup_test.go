package utfkenall_test

import (
	"errors"
	"testing"
	"time"

	"github.com/memwey/utfkenall"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name string
		code string
		want utfkenall.Address
	}{
		{
			name: "town, prefecture and city all named",
			code: "100-0001",
			want: utfkenall.Address{
				Code:       "1000001",
				JISCode:    "13101",
				Prefecture: utfkenall.Name{Kanji: "東京都", Kana: "トウキョウト", Romaji: "Tokyo"},
				City:       utfkenall.Name{Kanji: "千代田区", Kana: "チヨダク", Romaji: "Chiyoda-ku"},
				Town:       utfkenall.Name{Kanji: "千代田", Kana: "チヨダ", Romaji: "Chiyoda"},
			},
		},
		{
			// 「以下に掲載がない場合」 is a placeholder, not a town.
			name: "code covering a whole municipality",
			code: "060-0000",
			want: utfkenall.Address{
				Code:       "0600000",
				JISCode:    "01101",
				Prefecture: utfkenall.Name{Kanji: "北海道", Kana: "ホッカイドウ", Romaji: "Hokkaido"},
				City:       utfkenall.Name{Kanji: "札幌市中央区", Kana: "サッポロシチュウオウク", Romaji: "Sapporo-shi Chuo-ku"},
				Note:       utfkenall.Annotation{Kanji: "以下に掲載がない場合", Kana: "イカニケイサイガナイバアイ"},
			},
		},
		{
			name: "annotated town keeps the annotation out of the name",
			code: "060-0042",
			want: utfkenall.Address{
				Code:       "0600042",
				JISCode:    "01101",
				Prefecture: utfkenall.Name{Kanji: "北海道", Kana: "ホッカイドウ", Romaji: "Hokkaido"},
				City:       utfkenall.Name{Kanji: "札幌市中央区", Kana: "サッポロシチュウオウク", Romaji: "Sapporo-shi Chuo-ku"},
				Town:       utfkenall.Name{Kanji: "大通西", Kana: "オオドオリニシ", Romaji: "Odorinishi"},
				Note:       utfkenall.Annotation{Kanji: "１〜１９丁目", Kana: "１−１９チョウメ"},
			},
		},
		{
			name: "westernmost municipality in the country",
			code: "907-1801",
			want: utfkenall.Address{
				Code:       "9071801",
				JISCode:    "47382",
				Prefecture: utfkenall.Name{Kanji: "沖縄県", Kana: "オキナワケン", Romaji: "Okinawa"},
				City:       utfkenall.Name{Kanji: "八重山郡与那国町", Kana: "ヤエヤマグンヨナグニチョウ", Romaji: "Yaeyama-gun Yonaguni-cho"},
				Town:       utfkenall.Name{Kanji: "与那国", Kana: "ヨナグニ", Romaji: "Yonaguni"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utfkenall.Lookup(tt.code)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", tt.code, err)
			}
			if got != tt.want {
				t.Errorf("Lookup(%q)\n got %#v\nwant %#v", tt.code, got, tt.want)
			}
		})
	}
}

func TestLookupAcceptsWrittenForms(t *testing.T) {
	want, err := utfkenall.Lookup("1000001")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		"100-0001", "100 0001", "〒100-0001", "〒1000001",
		"１００−０００１", "１０００００１", "100–0001", "100ー0001", " 100-0001 ",
	} {
		got, err := utfkenall.Lookup(code)
		if err != nil {
			t.Errorf("Lookup(%q): %v", code, err)
			continue
		}
		if got != want {
			t.Errorf("Lookup(%q) = %v, want %v", code, got, want)
		}
	}
}

func TestLookupRejectsBadInput(t *testing.T) {
	for _, code := range []string{
		"", "100000", "10000012", "abcdefg", "100-000x", "１００-０００",
		"〒", "----", "1000001x", "0x1000001",
	} {
		if _, err := utfkenall.Lookup(code); !errors.Is(err, utfkenall.ErrInvalidCode) {
			t.Errorf("Lookup(%q) error = %v, want ErrInvalidCode", code, err)
		}
	}
}

func TestLookupNotFound(t *testing.T) {
	// Well formed, but Japan Post publishes nothing for it.
	for _, code := range []string{"0000000", "9999999"} {
		if _, err := utfkenall.Lookup(code); !errors.Is(err, utfkenall.ErrNotFound) {
			t.Errorf("Lookup(%q) error = %v, want ErrNotFound", code, err)
		}
	}
}

// Japan Post publishes organisation codes as a separate dataset, which this
// package does not carry. That is a boundary worth stating rather than a gap
// to discover: somebody looking up a company gets ErrNotFound, not a wrong
// address.
func TestOrganisationCodesAreNotHere(t *testing.T) {
	for _, tt := range []struct{ code, who string }{
		{"100-8111", "宮内庁"},
		{"163-8001", "東京都庁"},
	} {
		if _, err := utfkenall.Lookup(tt.code); !errors.Is(err, utfkenall.ErrNotFound) {
			t.Errorf("Lookup(%q), the %s code, gave %v; want ErrNotFound", tt.code, tt.who, err)
		}
	}
}

// Well-known addresses, checkable by anyone against Japan Post's own search
// page. Every other golden value in this package was read out of the database
// it is meant to be testing; these did not come from there.
func TestLandmarks(t *testing.T) {
	for _, tt := range []struct{ code, want string }{
		{"100-0001", "東京都千代田区千代田"},
		{"530-0001", "大阪府大阪市北区梅田"},
		{"460-0001", "愛知県名古屋市中区三の丸"},
		{"060-0001", "北海道札幌市中央区北一条西"},
		{"900-0001", "沖縄県那覇市港町"},
		{"907-1801", "沖縄県八重山郡与那国町与那国"},
	} {
		a, err := utfkenall.Lookup(tt.code)
		if err != nil {
			t.Errorf("Lookup(%q): %v", tt.code, err)
			continue
		}
		if got := a.String(); got != tt.want {
			t.Errorf("Lookup(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestLookupAll(t *testing.T) {
	// 498-0000 straddles a prefecture border: Japan Post files it under both
	// 愛知県弥富市 and 三重県桑名郡木曽岬町.
	got, err := utfkenall.LookupAll("498-0000")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("LookupAll(498-0000) returned %d addresses, want 2", len(got))
	}
	if got[0].Prefecture.Kanji != "愛知県" || got[1].Prefecture.Kanji != "三重県" {
		t.Errorf("got prefectures %q and %q", got[0].Prefecture.Kanji, got[1].Prefecture.Kanji)
	}

	// Lookup returns the first of them.
	first, err := utfkenall.Lookup("498-0000")
	if err != nil {
		t.Fatal(err)
	}
	if first != got[0] {
		t.Errorf("Lookup = %v, want the first of LookupAll, %v", first, got[0])
	}
}

func TestLookupAllReturnsEveryTown(t *testing.T) {
	// The most crowded code in the database.
	got, err := utfkenall.LookupAll("452-0961")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("LookupAll(452-0961) returned %d addresses, want many", len(got))
	}
	seen := make(map[string]bool, len(got))
	for _, a := range got {
		if a.Code != "4520961" {
			t.Errorf("got code %q among the results", a.Code)
		}
		if seen[a.Town.Kanji] {
			t.Errorf("town %q appears twice", a.Town.Kanji)
		}
		seen[a.Town.Kanji] = true
	}
}

func TestAllIsSortedAndComplete(t *testing.T) {
	var n int
	var prev string
	for a := range utfkenall.All() {
		if a.Code < prev {
			t.Fatalf("All() went backwards: %q after %q", a.Code, prev)
		}
		prev = a.Code
		n++
	}
	if n != utfkenall.Len() {
		t.Errorf("All() yielded %d addresses, Len() reports %d", n, utfkenall.Len())
	}
	if n < 100000 {
		t.Errorf("only %d addresses in the database, expected well over 100000", n)
	}
}

func TestAllStopsEarly(t *testing.T) {
	var n int
	for range utfkenall.All() {
		n++
		if n == 3 {
			break
		}
	}
	if n != 3 {
		t.Errorf("iterated %d times after breaking at 3", n)
	}
}

func TestEveryAddressIsWellFormed(t *testing.T) {
	prefs := make(map[string]bool, 47)
	for _, p := range utfkenall.Prefectures() {
		prefs[p.Kanji] = true
	}
	var estimated int
	for a := range utfkenall.All() {
		switch {
		case len(a.Code) != 7:
			t.Fatalf("%#v: code is not 7 digits", a)
		case len(a.JISCode) != 5:
			t.Fatalf("%#v: JIS code is not 5 digits", a)
		case !prefs[a.Prefecture.Kanji]:
			t.Fatalf("%#v: unknown prefecture", a)
		case a.City.Kanji == "" || a.City.Kana == "" || a.City.Romaji == "":
			t.Fatalf("%#v: incomplete city name", a)
		}
		// A town is either fully named or fully absent.
		named := a.Town.Kanji != ""
		if named != (a.Town.Kana != "") || named != (a.Town.Romaji != "") {
			t.Fatalf("%#v: town is partly named", a)
		}
		if a.RomajiEstimated {
			estimated++
		}
	}
	// The romaji dataset lags, so a few estimates are expected — but if the
	// join ever breaks, this is where it shows.
	if pct := 100 * float64(estimated) / float64(utfkenall.Len()); pct > 2 {
		t.Errorf("%.2f%% of addresses have transliterated romaji, expected well under 2%%", pct)
	}
}

func TestPrefectures(t *testing.T) {
	got := utfkenall.Prefectures()
	if len(got) != 47 {
		t.Fatalf("got %d prefectures, want 47", len(got))
	}
	for _, want := range []struct {
		index int
		utfkenall.Name
	}{
		{0, utfkenall.Name{Kanji: "北海道", Kana: "ホッカイドウ", Romaji: "Hokkaido"}},
		{12, utfkenall.Name{Kanji: "東京都", Kana: "トウキョウト", Romaji: "Tokyo"}},
		{46, utfkenall.Name{Kanji: "沖縄県", Kana: "オキナワケン", Romaji: "Okinawa"}},
	} {
		if got[want.index] != want.Name {
			t.Errorf("Prefectures()[%d] = %v, want %v", want.index, got[want.index], want.Name)
		}
	}
}

func TestDataUpdated(t *testing.T) {
	addresses, romaji := utfkenall.DataUpdated()
	if addresses.IsZero() || romaji.IsZero() {
		t.Fatalf("DataUpdated() = %v, %v; neither should be zero", addresses, romaji)
	}
	oldest := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	if addresses.Before(oldest) {
		t.Errorf("address data is dated %v, older than the UTF-8 format itself", addresses)
	}
	if romaji.After(addresses) {
		t.Errorf("romaji data (%v) is newer than the address data (%v), which Japan Post does not do", romaji, addresses)
	}
}

func TestLoad(t *testing.T) {
	if err := utfkenall.Load(); err != nil {
		t.Fatalf("Load() = %v", err)
	}
}

func FuzzLookup(f *testing.F) {
	for _, s := range []string{"1000001", "100-0001", "〒100-0001", "１００−０００１", "", "abc", "00000000"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, code string) {
		got, err := utfkenall.Lookup(code)
		switch {
		case err == nil:
			if len(got.Code) != 7 {
				t.Fatalf("Lookup(%q) succeeded with code %q", code, got.Code)
			}
		case errors.Is(err, utfkenall.ErrInvalidCode), errors.Is(err, utfkenall.ErrNotFound):
		default:
			t.Fatalf("Lookup(%q) = %v, an unexpected error", code, err)
		}
	})
}

func BenchmarkLookup(b *testing.B) {
	if err := utfkenall.Load(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := utfkenall.Lookup("100-0001"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAll(b *testing.B) {
	if err := utfkenall.Load(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		for a := range utfkenall.All() {
			_ = a
		}
	}
}
