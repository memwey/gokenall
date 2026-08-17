package main

import (
	"regexp"
	"strings"
)

// Japan Post uses the town column for three placeholders that are prose, not
// place names: 「以下に掲載がない場合」 covers everything in the city without its
// own code, 「○○市の次に番地がくる場合」 marks addresses with no town at all, and
// 「○○村一円」 means the whole village. All three become an empty town.
//
// The 一円 pattern needs a leading character: 一円 on its own is a real town
// name (滋賀県犬上郡多賀町一円).
var placeholderRe = regexp.MustCompile(`^以下に掲載がない場合$|の次に番地がくる場合$|.+一円$`)

// Trailing parenthesised annotations, full-width in both the kanji and the
// kana column. Every parenthesis in the dataset is a trailing one.
var (
	noteRe      = regexp.MustCompile(`（[^）]*）$`)
	noteInnerRe = regexp.MustCompile(`^（(.*)）$`)
)

// noiseNoteRe matches annotations that say nothing about the place: 「その他」
// (the leftovers of a split town), 「地階・階層不明」 (a building's unnumbered
// floors), and 「…を除く」 exclusion lists.
var noiseNoteRe = regexp.MustCompile(`^その他$|^地階・階層不明$|を除く$`)

// cleanedTown is a town name with Japan Post's editorial notes lifted out.
type cleanedTown struct {
	Kanji string
	Kana  string
	// Note is the annotation that followed the name, e.g. "１〜１９丁目".
	// Empty when there was none or when it carried no information.
	Note string
}

// cleanTown rewrites the raw town columns into something usable: placeholders
// become empty, and a trailing 「（…）」 moves out of the name into Note.
//
// Ranges such as 「（１〜１９丁目）」 are deliberately left as a note rather than
// expanded into one record per 丁目. Expanding invents zip code entries that
// Japan Post never published, and callers that want them can read Note.
func cleanTown(kanji, kana string) cleanedTown {
	if placeholderRe.MatchString(kanji) {
		return cleanedTown{}
	}

	note := noteRe.FindString(kanji)
	if note == "" {
		return cleanedTown{Kanji: kanji, Kana: kana}
	}

	out := cleanedTown{
		Kanji: strings.TrimSuffix(kanji, note),
		Kana:  noteRe.ReplaceAllString(kana, ""),
	}
	if inner := noteInnerRe.FindStringSubmatch(note); inner != nil && !noiseNoteRe.MatchString(inner[1]) {
		out.Note = inner[1]
	}
	return out
}

// KEN_ALL_ROME.CSV is still published in the layout the main dataset left
// behind in 2023: it caps the kanji town column at 17 full-width characters and
// the romaji column at 35, and splits anything longer across several records.
// A record cut mid-name is useless to us, and a record cut inside a trailing
// annotation is fine once the annotation is dropped, so the widths have to be
// known to tell the two apart.
const (
	romeKanjiWidth  = 17
	romeRomajiWidth = 35
)

// romeParenRe matches the trailing annotation in KEN_ALL_ROME.CSV, which uses
// ASCII parentheses — "ODORINISHI(1-19-CHOME)" — including the unclosed form
// left behind when the field was cut short.
var romeParenRe = regexp.MustCompile(`\([^)]*\)?$`)

// romeName strips the annotation from one of ROME's name columns and reports
// whether what remains is the whole name.
//
// It is not when the field ran out of room before any annotation began: there
// is no way to tell how much of the name is missing, so the record has to be
// discarded rather than joined against a name it merely starts with.
func romeName(s string, open rune, limit int, re *regexp.Regexp) (string, bool) {
	truncated := len([]rune(s)) >= limit
	if !strings.ContainsRune(s, open) {
		return strings.TrimSpace(s), !truncated
	}
	return strings.TrimSpace(re.ReplaceAllString(s, "")), true
}

func stripRomeNote(s string) (string, bool) {
	return romeName(s, '(', romeRomajiWidth, romeParenRe)
}

// romeNoteRe is noteRe widened to accept the unclosed full-width parenthesis a
// truncated record ends with.
var romeNoteRe = regexp.MustCompile(`（[^）]*）?$`)

func stripRomeNoteKanji(s string) (string, bool) {
	return romeName(s, '（', romeKanjiWidth, romeNoteRe)
}
