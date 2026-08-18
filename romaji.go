package utfkenall

import "github.com/memwey/utfkenall/internal/kana"

// Transliterate spells out full-width katakana in Latin script, one kana at a
// time: マルノウチ becomes "Marunouchi" and トウキョウ becomes "Toukyou".
//
// This is a second opinion on [Name.Romaji], not a replacement for it. The two
// differ over long vowels, and katakana does not record which reading is meant:
//
//	                Name.Romaji   Transliterate(Name.Kana)
//	マルノウチ (丸の内)  Marunochi     Marunouchi
//	トウキョウ (東京)    Tokyo         Toukyou
//	チュウオウ (中央)    Chuo          Chuuou
//
// 丸の内 is maru-no-uchi, so Japan Post's Marunochi is wrong and the spelled-out
// form is right; 東京 is tō-kyō, so it is the other way round. Japan Post — and
// the Digital Agency's Address Base Registry, which agrees with them on 99.93%
// of towns — drops long vowels everywhere, and that is what Name.Romaji holds.
//
// Use Name.Romaji to display an address, and Transliterate to match romaji
// somebody typed: a search for "marunouchi" or "toukyou" finds nothing in the
// published spellings.
//
//	for a := range utfkenall.All() {
//		index(a, utfkenall.Transliterate(a.Town.Kana))
//	}
//
// Input that is not katakana is passed through when it is ASCII and dropped
// otherwise, so digits and hyphens inside a reading survive.
func Transliterate(katakana string) string {
	return kana.Titlecase(kana.Romaji(katakana))
}
