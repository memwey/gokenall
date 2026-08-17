package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/memwey/utfkenall/internal/kana"
)

// prefectureEnglish maps each prefecture to the name English writing actually
// uses. KEN_ALL_ROME.CSV would give us "AOMORI KEN" and, for 群馬, the archaic
// "GUMMA"; neither is what anyone addresses an envelope in English with.
var prefectureEnglish = map[string]string{
	"北海道": "Hokkaido", "青森県": "Aomori", "岩手県": "Iwate", "宮城県": "Miyagi",
	"秋田県": "Akita", "山形県": "Yamagata", "福島県": "Fukushima", "茨城県": "Ibaraki",
	"栃木県": "Tochigi", "群馬県": "Gunma", "埼玉県": "Saitama", "千葉県": "Chiba",
	"東京都": "Tokyo", "神奈川県": "Kanagawa", "新潟県": "Niigata", "富山県": "Toyama",
	"石川県": "Ishikawa", "福井県": "Fukui", "山梨県": "Yamanashi", "長野県": "Nagano",
	"岐阜県": "Gifu", "静岡県": "Shizuoka", "愛知県": "Aichi", "三重県": "Mie",
	"滋賀県": "Shiga", "京都府": "Kyoto", "大阪府": "Osaka", "兵庫県": "Hyogo",
	"奈良県": "Nara", "和歌山県": "Wakayama", "鳥取県": "Tottori", "島根県": "Shimane",
	"岡山県": "Okayama", "広島県": "Hiroshima", "山口県": "Yamaguchi", "徳島県": "Tokushima",
	"香川県": "Kagawa", "愛媛県": "Ehime", "高知県": "Kochi", "福岡県": "Fukuoka",
	"佐賀県": "Saga", "長崎県": "Nagasaki", "熊本県": "Kumamoto", "大分県": "Oita",
	"宮崎県": "Miyazaki", "鹿児島県": "Kagoshima", "沖縄県": "Okinawa",
}

func prefectureRomaji(kanji string) (string, error) {
	if v, ok := prefectureEnglish[kanji]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no English name for prefecture %q", kanji)
}

// adminSuffix lists the administrative-unit words KEN_ALL_ROME.CSV writes as
// separate tokens. English convention hyphenates them onto the name:
// "SAPPORO SHI CHUO KU" -> "Sapporo-shi Chuo-ku".
var adminSuffix = map[string]bool{
	"SHI": true, "KU": true, "GUN": true, "CHO": true,
	"MACHI": true, "MURA": true, "SON": true,
}

func prettyCity(s string) string {
	var words []string
	for _, tok := range strings.Fields(s) {
		if adminSuffix[strings.ToUpper(tok)] && len(words) > 0 {
			words[len(words)-1] += "-" + strings.ToLower(tok)
			continue
		}
		words = append(words, capitalize(tok))
	}
	return strings.Join(words, " ")
}

func prettyTown(s string) string { return kana.Titlecase(s) }

// capitalize lower-cases a word and upper-cases its first rune, but only if
// that rune is a letter: "AINOSATO" -> "Ainosato", "1-JO" -> "1-jo",
// "KITA1-JONISHI" -> "Kita1-jonishi".
func capitalize(w string) string {
	rs := []rune(strings.ToLower(w))
	if len(rs) > 0 && unicode.IsLetter(rs[0]) {
		rs[0] = unicode.ToUpper(rs[0])
	}
	return string(rs)
}

// townKey identifies a town across the two datasets. The town component is the
// cleaned name, so the differing parenthetical annotations in the two files do
// not break the match.
type townKey struct {
	zip  string
	pref string
	city string
	town string
}

// romajiIndex is KEN_ALL_ROME.CSV arranged for lookup.
type romajiIndex struct {
	towns map[townKey]string
	// byZip holds the distinct town spellings ROME lists for a zip code, used
	// for the looser second-tier match.
	byZip  map[string][]string
	cities map[[2]string]string
}

func newRomajiIndex(rows []romeRow) *romajiIndex {
	ix := &romajiIndex{
		towns:  make(map[townKey]string, len(rows)),
		byZip:  make(map[string][]string, len(rows)),
		cities: make(map[[2]string]string, 2000),
	}
	for _, r := range rows {
		// The city columns are never long enough to be cut, so they are taken
		// as-is even from a record whose town half is a fragment.
		ix.cities[[2]string{r.Pref, r.City}] = r.CityRomaji

		romaji, romajiWhole := stripRomeNote(r.TownRomaji)
		town, townWhole := stripRomeNoteKanji(r.Town)
		if !romajiWhole || !townWhole || romaji == "" {
			continue
		}

		key := townKey{zip: r.Zip, pref: r.Pref, city: r.City, town: town}
		if _, dup := ix.towns[key]; !dup {
			ix.towns[key] = romaji
		}
		if !contains(ix.byZip[r.Zip], romaji) {
			ix.byZip[r.Zip] = append(ix.byZip[r.Zip], romaji)
		}
	}
	return ix
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (ix *romajiIndex) city(pref, city string) (string, bool) {
	v, ok := ix.cities[[2]string{pref, city}]
	return v, ok
}
