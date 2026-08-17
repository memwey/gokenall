// Package kana transliterates Japanese katakana into Latin script.
//
// It offers the two conventions this repository needs, which differ only in
// what they do with long vowels:
//
//	kana.Romaji("マルノウチ")        // MARUNOUCHI — every kana spelled out
//	kana.PostalRomaji("マルノウチ")  // MARUNOCHI  — long vowels dropped
//
// Neither is right in every case, because katakana does not record whether a
// vowel pair is one long vowel or two syllables. 丸の内 is maru-no-uchi, so
// spelling it out is correct and dropping is not; 東京 is tō-kyō, so dropping
// is correct (TOKYO) and spelling it out is not (TOUKYOU). Japan Post picks
// the dropping rule everywhere, which is why they publish "Marunochi".
//
// Output is upper case, matching the raw Japan Post datasets. Run it through
// [Titlecase] for display.
package kana

import (
	"strings"
	"unicode"
)

// style decides what happens to an unmarked long vowel.
type style int

const (
	// spellOut keeps every kana, so ウ after an o-sound stays a U.
	spellOut style = iota
	// drop discards it, which is what Japan Post does.
	drop
)

// Romaji spells out every kana: マルノウチ becomes MARUNOUCHI and トウキョウ
// becomes TOUKYOU. Because nothing is discarded, the result can be matched
// against romaji that a person typed from the reading.
func Romaji(katakana string) string { return convert(katakana, spellOut) }

// PostalRomaji follows the convention Japan Post uses in KEN_ALL_ROME.CSV and
// the Digital Agency uses in the Address Base Registry: modified Hepburn with
// long vowels left unmarked. マルノウチ becomes MARUNOCHI, トウキョウ becomes
// TOKYO, ホッカイドウ becomes HOKKAIDO.
func PostalRomaji(katakana string) string { return convert(katakana, drop) }

func convert(katakana string, st style) string {
	src := []rune(katakana)
	var b strings.Builder
	b.Grow(len(katakana))

	for i := 0; i < len(src); {
		r := src[i]

		switch {
		case r == 'ー':
			// The long vowel mark carries no vowel of its own, so spelling it
			// out means repeating whatever came before it.
			if st == spellOut {
				if s := b.String(); s != "" && isVowel(s[len(s)-1]) {
					b.WriteByte(s[len(s)-1])
				}
			}
			i++
			continue

		case r == 'ッ':
			// Small tsu doubles the next consonant. Before CH it becomes T,
			// per Hepburn: ハッチョウ -> HATCHO.
			next := ""
			if i+1 < len(src) {
				next, _ = lookup(src[i+1:])
			}
			switch {
			case next == "":
				// Trailing or unmappable; nothing to double.
			case strings.HasPrefix(next, "CH"):
				b.WriteByte('T')
			default:
				b.WriteByte(next[0])
			}
			i++
			continue

		case r >= '０' && r <= '９':
			b.WriteRune(r - '０' + '0')
			// Japan Post separates a number from the mora that follows it:
			// キタ１ジョウ -> KITA1-JO.
			if i+1 < len(src) && isKana(src[i+1]) {
				b.WriteByte('-')
			}
			i++
			continue

		case r >= '0' && r <= '9':
			b.WriteRune(r)
			if i+1 < len(src) && isKana(src[i+1]) {
				b.WriteByte('-')
			}
			i++
			continue
		}

		out, width := lookup(src[i:])
		if out == "" {
			// Anything that is not katakana (punctuation left over from a note,
			// stray ASCII) is passed through if printable and dropped if not.
			if r < 0x80 {
				b.WriteRune(r)
			}
			i++
			continue
		}

		if out == "N" && i+width < len(src) {
			// ン assimilates before a labial: グンマ -> GUMMA.
			if n, _ := lookup(src[i+width:]); n != "" && strings.IndexByte("BMP", n[0]) >= 0 {
				out = "M"
			}
		}
		b.WriteString(out)
		i += width

		if st == drop && i < len(src) {
			// ウ after an o- or u-sound, オ after an o-sound. アア/イイ/エイ are
			// left alone — Japan Post writes NIIGATA, not NIGATA.
			last := out[len(out)-1]
			if (src[i] == 'ウ' && (last == 'O' || last == 'U')) || (src[i] == 'オ' && last == 'O') {
				i++
			}
		}
	}

	return b.String()
}

// Titlecase renders upper-case romaji for display: each space-separated word
// keeps only its first letter capitalised, and a word that starts with a digit
// is left lower case throughout.
//
//	AINOSATO 1-JO -> Ainosato 1-jo
//	KITA1-JONISHI -> Kita1-jonishi
func Titlecase(romaji string) string {
	words := strings.Fields(romaji)
	for i, w := range words {
		words[i] = capitalize(w)
	}
	return strings.Join(words, " ")
}

func capitalize(w string) string {
	rs := []rune(strings.ToLower(w))
	if len(rs) > 0 && unicode.IsLetter(rs[0]) {
		rs[0] = unicode.ToUpper(rs[0])
	}
	return string(rs)
}

func isVowel(b byte) bool { return strings.IndexByte("AIUEO", b) >= 0 }

func isKana(r rune) bool { return r >= 0x30A1 && r <= 0x30FA }

// lookup returns the romaji for the mora starting at src and how many runes it
// consumed, preferring the two-rune digraph when there is one.
func lookup(src []rune) (romaji string, width int) {
	if len(src) >= 2 {
		if v, ok := digraphs[string(src[:2])]; ok {
			return v, 2
		}
	}
	if v, ok := monographs[src[0]]; ok {
		return v, 1
	}
	return "", 0
}

var monographs = map[rune]string{
	'ア': "A", 'イ': "I", 'ウ': "U", 'エ': "E", 'オ': "O",
	'カ': "KA", 'キ': "KI", 'ク': "KU", 'ケ': "KE", 'コ': "KO",
	'サ': "SA", 'シ': "SHI", 'ス': "SU", 'セ': "SE", 'ソ': "SO",
	'タ': "TA", 'チ': "CHI", 'ツ': "TSU", 'テ': "TE", 'ト': "TO",
	'ナ': "NA", 'ニ': "NI", 'ヌ': "NU", 'ネ': "NE", 'ノ': "NO",
	'ハ': "HA", 'ヒ': "HI", 'フ': "FU", 'ヘ': "HE", 'ホ': "HO",
	'マ': "MA", 'ミ': "MI", 'ム': "MU", 'メ': "ME", 'モ': "MO",
	'ヤ': "YA", 'ユ': "YU", 'ヨ': "YO",
	'ラ': "RA", 'リ': "RI", 'ル': "RU", 'レ': "RE", 'ロ': "RO",
	'ワ': "WA", 'ヰ': "I", 'ヱ': "E", 'ヲ': "O", 'ン': "N",
	'ガ': "GA", 'ギ': "GI", 'グ': "GU", 'ゲ': "GE", 'ゴ': "GO",
	'ザ': "ZA", 'ジ': "JI", 'ズ': "ZU", 'ゼ': "ZE", 'ゾ': "ZO",
	'ダ': "DA", 'ヂ': "JI", 'ヅ': "ZU", 'デ': "DE", 'ド': "DO",
	'バ': "BA", 'ビ': "BI", 'ブ': "BU", 'ベ': "BE", 'ボ': "BO",
	'パ': "PA", 'ピ': "PI", 'プ': "PU", 'ペ': "PE", 'ポ': "PO",
	'ヴ': "VU",
	'ャ': "YA", 'ュ': "YU", 'ョ': "YO",
	'ァ': "A", 'ィ': "I", 'ゥ': "U", 'ェ': "E", 'ォ': "O",
}

var digraphs = map[string]string{
	"キャ": "KYA", "キュ": "KYU", "キョ": "KYO",
	"シャ": "SHA", "シュ": "SHU", "ショ": "SHO", "シェ": "SHE",
	"チャ": "CHA", "チュ": "CHU", "チョ": "CHO", "チェ": "CHE",
	"ニャ": "NYA", "ニュ": "NYU", "ニョ": "NYO",
	"ヒャ": "HYA", "ヒュ": "HYU", "ヒョ": "HYO",
	"ミャ": "MYA", "ミュ": "MYU", "ミョ": "MYO",
	"リャ": "RYA", "リュ": "RYU", "リョ": "RYO",
	"ギャ": "GYA", "ギュ": "GYU", "ギョ": "GYO",
	"ジャ": "JA", "ジュ": "JU", "ジョ": "JO", "ジェ": "JE",
	"ヂャ": "JA", "ヂュ": "JU", "ヂョ": "JO",
	"ビャ": "BYA", "ビュ": "BYU", "ビョ": "BYO",
	"ピャ": "PYA", "ピュ": "PYU", "ピョ": "PYO",
	"ファ": "FA", "フィ": "FI", "フェ": "FE", "フォ": "FO",
	"ウィ": "WI", "ウェ": "WE", "ウォ": "WO",
	"ヴァ": "VA", "ヴィ": "VI", "ヴェ": "VE", "ヴォ": "VO",
	"ティ": "TI", "ディ": "DI", "トゥ": "TU", "ドゥ": "DU",
	"ツァ": "TSA", "ツィ": "TSI", "ツェ": "TSE", "ツォ": "TSO",
}
