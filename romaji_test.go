package utfkenall_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/memwey/utfkenall"
)

func TestTransliterate(t *testing.T) {
	for _, tt := range []struct{ kana, want string }{
		{"マルノウチ", "Marunouchi"},
		{"トウキョウ", "Toukyou"},
		{"チヨダ", "Chiyoda"},
		{"オオドオリニシ", "Oodoorinishi"},
		{"ミヤノモリ１ジョウ", "Miyanomori1-jou"},
		{"", ""},
	} {
		if got := utfkenall.Transliterate(tt.kana); got != tt.want {
			t.Errorf("Transliterate(%q) = %q, want %q", tt.kana, got, tt.want)
		}
	}
}

// The point of shipping both spellings: the published one cannot be matched
// against romaji typed from the reading, and vice versa.
func TestTransliterateComplementsThePublishedSpelling(t *testing.T) {
	addr, err := utfkenall.Lookup("100-0005")
	if err != nil {
		t.Fatal(err)
	}
	if addr.Town.Kanji != "丸の内" {
		t.Fatalf("expected 丸の内, got %q", addr.Town.Kanji)
	}
	// Japan Post reads マルノウチ as if ノウ were a long o.
	if addr.Town.Romaji != "Marunochi" {
		t.Errorf("Town.Romaji = %q, want Japan Post's Marunochi", addr.Town.Romaji)
	}
	// Spelling out every kana recovers the form people actually write.
	if got := utfkenall.Transliterate(addr.Town.Kana); got != "Marunouchi" {
		t.Errorf("Transliterate(%q) = %q, want Marunouchi", addr.Town.Kana, got)
	}
}

// The two spellings coincide only where the reading holds no long vowel, which
// is a minority of Japanese place names — 町 alone is チョウ. So this measures a
// real split, not a near-duplicate: if Transliterate ever broke, agreement
// would collapse towards zero rather than settle where it does.
func TestTransliterateAgreementWithThePublishedSpelling(t *testing.T) {
	var same, sameIgnoringSpaces, total int
	for a := range utfkenall.All() {
		if a.Town.Kana == "" || a.RomajiEstimated {
			continue
		}
		total++
		got := utfkenall.Transliterate(a.Town.Kana)
		if got == a.Town.Romaji {
			same++
		}
		if strings.ReplaceAll(got, " ", "") == strings.ReplaceAll(a.Town.Romaji, " ", "") {
			sameIgnoringSpaces++
		}
		if got == "" {
			t.Fatalf("Transliterate(%q) is empty for %s", a.Town.Kana, a.Town.Kanji)
		}
	}
	t.Logf("of %d named towns, spelling out matches Japan Post exactly %.1f%% of the time, %.1f%% ignoring spaces",
		total, 100*float64(same)/float64(total), 100*float64(sameIgnoringSpaces)/float64(total))
	if pct := 100 * float64(same) / float64(total); pct < 30 {
		t.Errorf("only %.1f%% agree; the two conventions differ over long vowels alone and should overlap far more", pct)
	}
}

func ExampleTransliterate() {
	addr, err := utfkenall.Lookup("100-0005")
	if err != nil {
		panic(err)
	}
	fmt.Println(addr.Town.Kanji, addr.Town.Kana)
	fmt.Println("published:   ", addr.Town.Romaji)
	fmt.Println("spelled out: ", utfkenall.Transliterate(addr.Town.Kana))
	// Output:
	// 丸の内 マルノウチ
	// published:    Marunochi
	// spelled out:  Marunouchi
}

func BenchmarkTransliterate(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		utfkenall.Transliterate("オオドオリニシ")
	}
}
