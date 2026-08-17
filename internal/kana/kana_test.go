package kana

import "testing"

func TestPostalRomaji(t *testing.T) {
	tests := []struct {
		name string
		kana string
		want string
	}{
		{"plain", "チヨダ", "CHIYODA"},
		{"sokuon doubles the consonant", "サッポロ", "SAPPORO"},
		{"sokuon before ch becomes t", "ハッチョウ", "HATCHO"},
		{"sokuon before shi", "ミッシリ", "MISSHIRI"},
		{"long o written oo is dropped", "オオドオリニシ", "ODORINISHI"},
		{"long o written ou is dropped", "ホッカイドウ", "HOKKAIDO"},
		{"long u is dropped", "チュウオウ", "CHUO"},
		{"long vowel mark is dropped", "ヨーカイチ", "YOKAICHI"},
		{"ai is not a long vowel", "アイノサト", "AINOSATO"},
		{"ii is not a long vowel", "ニイガタ", "NIIGATA"},
		{"ei is not a long vowel", "ケイセイ", "KEISEI"},
		{"n before b becomes m", "オンベツ", "OMBETSU"},
		{"n before m becomes m", "グンマ", "GUMMA"},
		{"n before p becomes m", "サンポ", "SAMPO"},
		{"n elsewhere stays n", "ヨナグニ", "YONAGUNI"},
		{"digits are separated from the mora after them", "キタ１ジョウニシ", "KITA1-JONISHI"},
		{"multi-digit numbers", "シノロ１０ジョウ", "SHINORO10-JO"},
		{"digits at the end need no separator", "チョウメ２", "CHOME2"},
		{"yoon", "キョウトシシモギョウク", "KYOTOSHISHIMOGYOKU"},
		{"ji and zu", "フジミヅカ", "FUJIMIZUKA"},
		{"wo reads as o", "ヲノゾク", "ONOZOKU"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PostalRomaji(tt.kana); got != tt.want {
				t.Errorf("PostalRomaji(%q) = %q, want %q", tt.kana, got, tt.want)
			}
		})
	}
}

// Romaji differs from PostalRomaji only over long vowels, so these are the
// same names again with nothing discarded.
func TestRomaji(t *testing.T) {
	tests := []struct {
		name string
		kana string
		want string
	}{
		{"the case Japan Post gets wrong", "マルノウチ", "MARUNOUCHI"},
		{"the case Japan Post gets right", "トウキョウ", "TOUKYOU"},
		{"oo stays", "オオドオリニシ", "OODOORINISHI"},
		{"uu stays", "チュウオウ", "CHUUOU"},
		{"long vowel mark repeats the vowel", "ヨーカイチ", "YOOKAICHI"},
		{"nothing to repeat before the mark", "ーカイチ", "KAICHI"},
		{"sokuon is unaffected", "サッポロ", "SAPPORO"},
		{"n assimilation is unaffected", "グンマ", "GUMMA"},
		{"digits are unaffected", "キタ１ジョウニシ", "KITA1-JOUNISHI"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Romaji(tt.kana); got != tt.want {
				t.Errorf("Romaji(%q) = %q, want %q", tt.kana, got, tt.want)
			}
		})
	}
}

// Where a reading holds no long vowel the two conventions cannot differ, which
// is most of the database.
func TestBothStylesAgreeWithoutLongVowels(t *testing.T) {
	for _, kana := range []string{"チヨダ", "ヨナグニ", "アサヒガオカ", "サッポロ", "ニイガタ", "アイノサト"} {
		if a, b := Romaji(kana), PostalRomaji(kana); a != b {
			t.Errorf("%q: Romaji = %q but PostalRomaji = %q", kana, a, b)
		}
	}
}

// Real names, spelled by Japan Post themselves.
func TestPostalRomajiAgainstOfficialSpellings(t *testing.T) {
	tests := []struct{ kana, official string }{
		{"チヨダ", "CHIYODA"},
		{"ヨナグニ", "YONAGUNI"},
		{"ユウラクチョウ", "YURAKUCHO"},
		{"オオテマチ", "OTEMACHI"},
		{"マルノウチ", "MARUNOCHI"},
		{"アサヒガオカ", "ASAHIGAOKA"},
		{"オオドオリヒガシ", "ODORIHIGASHI"},
		{"タイユウサガリニシ", "TAIYUSAGARINISHI"},
		// The Digital Agency's registry spells these the same way.
		{"ニホンバシオオデンマチョウ", "NIHOMBASHIODEMMACHO"},
		{"オオクボ", "OKUBO"},
	}
	for _, tt := range tests {
		if got := PostalRomaji(tt.kana); got != tt.official {
			t.Errorf("PostalRomaji(%q) = %q, Japan Post writes %q", tt.kana, got, tt.official)
		}
	}
}

func TestTitlecase(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"CHIYODA", "Chiyoda"},
		{"ODORINISHI", "Odorinishi"},
		{"MIYANOMORI 1-JO", "Miyanomori 1-jo"},
		{"KITA1-JONISHI", "Kita1-jonishi"},
		{"SHINOROCHO KAMISHINORO", "Shinorocho Kamishinoro"},
		{"", ""},
	} {
		if got := Titlecase(tt.in); got != tt.want {
			t.Errorf("Titlecase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPassesThroughUnknownRunes(t *testing.T) {
	// Anything the transliterator cannot read must not corrupt what it can.
	if got := PostalRomaji("チヨダ-ク"); got != "CHIYODA-KU" {
		t.Errorf("PostalRomaji = %q", got)
	}
	if got := PostalRomaji("チヨダ 　ク"); got != "CHIYODA KU" {
		t.Errorf("dropped the wrong space characters: %q", got)
	}
}

func FuzzRomaji(f *testing.F) {
	for _, s := range []string{"チヨダ", "サッポロ", "オオドオリニシ", "キタ１ジョウ", "", "ッ", "ー", "ン", "ーー"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, kana string) {
		// The only contract is that both styles terminate and stay printable.
		for _, got := range []string{Romaji(kana), PostalRomaji(kana)} {
			for _, r := range got {
				if r == '�' {
					t.Fatalf("%q produced a replacement character: %q", kana, got)
				}
			}
		}
	})
}
