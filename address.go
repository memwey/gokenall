package utfkenall

import (
	"strings"

	"github.com/memwey/utfkenall/internal/binfmt"
)

// Name is a place name in the three writings Japan Post publishes.
type Name struct {
	// Kanji is the everyday written form, e.g. 千代田区.
	Kanji string
	// Kana is the reading in full-width katakana, e.g. チヨダク.
	Kana string
	// Romaji is the Latin-script form, e.g. Chiyoda-ku.
	Romaji string
}

// String returns the kanji form.
func (n Name) String() string { return n.Kanji }

// Annotation is a remark Japan Post attaches to a record rather than a place
// name of its own, so it has a reading but no romaji.
type Annotation struct {
	// Kanji is the remark as published, e.g. １〜１９丁目.
	Kanji string
	// Kana is its reading. Japan Post sometimes annotates only the kanji
	// column, so this can be empty while Kanji is not.
	Kana string
}

// String returns the kanji form.
func (n Annotation) String() string { return n.Kanji }

// Address is one entry of the zip code database: a prefecture, a municipality,
// and — unless Japan Post files the code under the municipality as a whole — a
// town.
type Address struct {
	// Code is the 7 digit zip code without a hyphen, e.g. "1000001".
	Code string
	// JISCode is the 5 digit 全国地方公共団体コード of City (JIS X0402),
	// e.g. "13101".
	JISCode string

	Prefecture Name
	City       Name
	// Town is empty for codes that cover a municipality rather than a named
	// town: Japan Post files those under 「以下に掲載がない場合」 and similar
	// placeholders, which this package reports as no town at all.
	Town Name

	// Note is what Japan Post filed alongside — or instead of — the town name,
	// with any parentheses removed. It is empty for the vast majority of
	// addresses. Town says which of the two it is:
	//
	//	Town named    an annotation that followed it, "１〜１９丁目"
	//	              for 大通西（１〜１９丁目）
	//	Town empty    the placeholder filed in place of a name,
	//	              "以下に掲載がない場合"
	//
	// The annotation is kept out of Town so that names compare and display
	// cleanly, but it is never discarded: [Address.RawTown] puts the original
	// columns back together.
	Note Annotation

	// RomajiEstimated reports that Town.Romaji was transliterated from Kana
	// rather than taken from Japan Post's romaji dataset, which is republished
	// less often than the addresses and so lacks the newest codes.
	RomajiEstimated bool
}

// String returns the address in Japanese, largest unit first:
// "東京都千代田区千代田".
func (a Address) String() string {
	return a.Prefecture.Kanji + a.City.Kanji + a.Town.Kanji
}

// English returns the address in Latin script, smallest unit first, the way an
// envelope bound for Japan is addressed: "Chiyoda, Chiyoda-ku, Tokyo".
func (a Address) English() string {
	parts := make([]string, 0, 3)
	for _, s := range []string{a.Town.Romaji, a.City.Romaji, a.Prefecture.Romaji} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// PublishedPrefectureRomaji returns the prefecture spelling exactly as Japan
// Post publishes it in KEN_ALL_ROME.CSV. It differs from
// a.Prefecture.Romaji, which is the conventional English form used by
// [Address.English]: Tokyo is published as "TOKYO TO", and Gunma as
// "GUMMA KEN".
func (a Address) PublishedPrefectureRomaji() string {
	romaji, _ := PublishedPrefectureRomaji(a.Prefecture.Kanji)
	return romaji
}

// RawTown returns the town columns exactly as Japan Post publishes them,
// before this package moves placeholders and annotations out of the name:
//
//	Lookup("060-0042") // Town 大通西, Note １〜１９丁目
//	                   // RawTown 大通西（１〜１９丁目）
//	Lookup("060-0000") // Town empty, Note 以下に掲載がない場合
//	                   // RawTown 以下に掲載がない場合
//
// Nothing is reconstructed approximately: the parts are stored as they were
// read, so this is the original text.
func (a Address) RawTown() (kanji, kana string) {
	switch {
	case a.Town.Kanji == "":
		// A placeholder occupied the whole column.
		return a.Note.Kanji, a.Note.Kana
	case a.Note.Kanji == "":
		return a.Town.Kanji, a.Town.Kana
	}
	kanji = a.Town.Kanji + "（" + a.Note.Kanji + "）"
	kana = a.Town.Kana
	if a.Note.Kana != "" {
		kana += "（" + a.Note.Kana + "）"
	}
	return kanji, kana
}

func newAddress(e binfmt.Entry) Address {
	return Address{
		Code:            digits(e.Zip, 7),
		JISCode:         digits(e.JIS, 5),
		Prefecture:      name(e.Prefecture),
		City:            name(e.City),
		Town:            name(e.Town),
		Note:            Annotation{Kanji: e.Note, Kana: e.NoteKana},
		RomajiEstimated: e.Flags&binfmt.FlagRomajiEstimated != 0,
	}
}

func name(n binfmt.Name) Name {
	return Name{Kanji: n.Kanji, Kana: n.Kana, Romaji: n.Romaji}
}

// digits renders v zero-padded to width, which beats fmt for codes formatted
// on every record of a full scan.
func digits(v uint32, width int) string {
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf)
}
