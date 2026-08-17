package utfkenall_test

import (
	"fmt"
	"testing"

	"github.com/memwey/utfkenall"
)

func TestAddressFormatting(t *testing.T) {
	tests := []struct {
		code        string
		japanese    string
		english     string
		explanation string
	}{
		{
			code:     "100-0001",
			japanese: "東京都千代田区千代田",
			english:  "Chiyoda, Chiyoda-ku, Tokyo",
		},
		{
			code:        "060-0000",
			japanese:    "北海道札幌市中央区",
			english:     "Sapporo-shi Chuo-ku, Hokkaido",
			explanation: "no town, so neither rendering leaves a gap",
		},
	}
	for _, tt := range tests {
		a, err := utfkenall.Lookup(tt.code)
		if err != nil {
			t.Fatalf("%s: %v", tt.code, err)
		}
		if got := a.String(); got != tt.japanese {
			t.Errorf("%s String() = %q, want %q", tt.code, got, tt.japanese)
		}
		if got := a.English(); got != tt.english {
			t.Errorf("%s English() = %q, want %q", tt.code, got, tt.english)
		}
	}
}

func TestPublishedPrefectureRomaji(t *testing.T) {
	tests := []struct {
		code, conventional, published string
	}{
		{"100-0001", "Tokyo", "TOKYO TO"},
		{"370-0000", "Gunma", "GUMMA KEN"},
		{"060-0000", "Hokkaido", "HOKKAIDO"},
	}
	for _, tt := range tests {
		a, err := utfkenall.Lookup(tt.code)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.Prefecture.Romaji; got != tt.conventional {
			t.Errorf("%s conventional romaji = %q, want %q", tt.code, got, tt.conventional)
		}
		if got := a.PublishedPrefectureRomaji(); got != tt.published {
			t.Errorf("%s published romaji = %q, want %q", tt.code, got, tt.published)
		}
	}
	if got, ok := utfkenall.PublishedPrefectureRomaji("東京都"); !ok || got != "TOKYO TO" {
		t.Errorf("PublishedPrefectureRomaji(東京都) = %q, %v", got, ok)
	}
	if got, ok := utfkenall.PublishedPrefectureRomaji("架空県"); ok || got != "" {
		t.Errorf("PublishedPrefectureRomaji(架空県) = %q, %v; want empty, false", got, ok)
	}
}

// The conventional English names used to be a hand-written table of 47, which
// was exhaustively right by construction. They are derived from Japan Post's
// spelling now, so the exhaustive check has to be a test instead: a rule that
// works on Tokyo and Gunma can still be wrong on the other forty-five.
func TestEveryPrefectureDerivesTheConventionalEnglishName(t *testing.T) {
	want := [...]string{
		"Hokkaido", "Aomori", "Iwate", "Miyagi", "Akita", "Yamagata", "Fukushima",
		"Ibaraki", "Tochigi", "Gunma", "Saitama", "Chiba", "Tokyo", "Kanagawa",
		"Niigata", "Toyama", "Ishikawa", "Fukui", "Yamanashi", "Nagano", "Gifu",
		"Shizuoka", "Aichi", "Mie", "Shiga", "Kyoto", "Osaka", "Hyogo", "Nara",
		"Wakayama", "Tottori", "Shimane", "Okayama", "Hiroshima", "Yamaguchi",
		"Tokushima", "Kagawa", "Ehime", "Kochi", "Fukuoka", "Saga", "Nagasaki",
		"Kumamoto", "Oita", "Miyazaki", "Kagoshima", "Okinawa",
	}
	got := utfkenall.Prefectures()
	if len(got) != len(want) {
		t.Fatalf("got %d prefectures, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Romaji != want[i] {
			published, _ := utfkenall.PublishedPrefectureRomaji(p.Kanji)
			t.Errorf("%s (published %q) derived %q, want %q", p.Kanji, published, p.Romaji, want[i])
		}
		if p.Kanji == "" || p.Kana == "" {
			t.Errorf("prefecture %d is incomplete: %#v", i, p)
		}
	}
}

func TestRawTown(t *testing.T) {
	tests := []struct {
		code       string
		kanji      string
		kana       string
		annotation string
	}{
		{
			code:  "100-0001",
			kanji: "千代田", kana: "チヨダ",
			annotation: "no annotation, so the name is the whole column",
		},
		{
			code:  "060-0042",
			kanji: "大通西（１〜１９丁目）", kana: "オオドオリニシ（１−１９チョウメ）",
			annotation: "the annotation goes back where it came from",
		},
		{
			code:  "060-0000",
			kanji: "以下に掲載がない場合", kana: "イカニケイサイガナイバアイ",
			annotation: "the placeholder occupied the whole column",
		},
	}
	for _, tt := range tests {
		t.Run(tt.annotation, func(t *testing.T) {
			a, err := utfkenall.Lookup(tt.code)
			if err != nil {
				t.Fatal(err)
			}
			kanji, kana := a.RawTown()
			if kanji != tt.kanji || kana != tt.kana {
				t.Errorf("RawTown() = %q, %q; want %q, %q", kanji, kana, tt.kanji, tt.kana)
			}
		})
	}
}

// Japan Post never publishes a blank town column, so a blank result would mean
// this package had thrown the original away somewhere.
func TestRawTownIsNeverEmpty(t *testing.T) {
	for a := range utfkenall.All() {
		kanji, kana := a.RawTown()
		if kanji == "" || kana == "" {
			t.Fatalf("%s: RawTown() = %q, %q", a.Code, kanji, kana)
		}
	}
}

func TestNoteIsAPlaceholderExactlyWhenTownIsEmpty(t *testing.T) {
	// RawTown relies on this to tell the two kinds of note apart.
	var placeholders int
	for a := range utfkenall.All() {
		if a.Town.Kanji != "" {
			continue
		}
		placeholders++
		if a.Note.Kanji == "" {
			t.Fatalf("%s has neither a town nor a placeholder", a.Code)
		}
	}
	if placeholders == 0 {
		t.Error("no placeholder records at all, which cannot be right")
	}
	t.Logf("%d of %d records are placeholders", placeholders, utfkenall.Len())
}

func TestNameString(t *testing.T) {
	n := utfkenall.Name{Kanji: "千代田区", Kana: "チヨダク", Romaji: "Chiyoda-ku"}
	if got := n.String(); got != "千代田区" {
		t.Errorf("String() = %q, want the kanji form", got)
	}
	if got := fmt.Sprint(n); got != "千代田区" {
		t.Errorf("fmt.Sprint = %q, want the kanji form", got)
	}
}

func ExampleLookup() {
	addr, err := utfkenall.Lookup("100-0001")
	if err != nil {
		panic(err)
	}
	fmt.Println(addr)
	fmt.Println(addr.English())
	fmt.Println(addr.Town.Kana)
	// Output:
	// 東京都千代田区千代田
	// Chiyoda, Chiyoda-ku, Tokyo
	// チヨダ
}

func ExampleLookupAll() {
	// A handful of codes straddle a prefecture border.
	addrs, err := utfkenall.LookupAll("498-0000")
	if err != nil {
		panic(err)
	}
	for _, a := range addrs {
		fmt.Println(a)
	}
	// Output:
	// 愛知県弥富市
	// 三重県桑名郡木曽岬町
}
