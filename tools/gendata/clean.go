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

// cleanedTown is a town name with Japan Post's editorial notes lifted out.
//
// Nothing the source publishes is dropped along the way: whatever leaves the
// name lands in Note, so the original columns can be put back together.
type cleanedTown struct {
	Kanji string
	Kana  string
	// Note is the annotation that followed the name with its parentheses
	// removed, e.g. "１〜１９丁目". For a placeholder — a record whose town
	// column holds prose rather than a name — it is that prose instead, and
	// Kanji is empty. Empty for the vast majority of records.
	Note string
	// NoteKana is Note's reading. Japan Post sometimes annotates the kanji
	// column and not the kana one, so it can be empty while Note is not.
	NoteKana string
}

// cleanTown rewrites the raw town columns into something usable: placeholders
// move out of the name, and so does a trailing 「（…）」.
//
// Ranges such as 「（１〜１９丁目）」 are deliberately left as a note rather than
// expanded into one record per 丁目. Expanding invents zip code entries that
// Japan Post never published, and callers that want them can read Note.
//
// Annotations that say nothing about the place — 「その他」, 「地階・階層不明」 —
// are kept too. Filtering them here would be the one place this package threw
// source text away, and a caller can ignore a note far more easily than it can
// recover one.
func cleanTown(kanji, kana string) cleanedTown {
	if placeholderRe.MatchString(kanji) {
		return cleanedTown{Note: kanji, NoteKana: kana}
	}

	note := noteRe.FindString(kanji)
	if note == "" {
		return cleanedTown{Kanji: kanji, Kana: kana}
	}

	out := cleanedTown{
		Kanji: strings.TrimSuffix(kanji, note),
		Kana:  noteRe.ReplaceAllString(kana, ""),
	}
	if inner := noteInnerRe.FindStringSubmatch(note); inner != nil {
		out.Note = inner[1]
	}
	if kanaNote := noteRe.FindString(kana); kanaNote != "" {
		if inner := noteInnerRe.FindStringSubmatch(kanaNote); inner != nil {
			out.NoteKana = inner[1]
		}
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

// rejoin puts the town columns back the way Japan Post published them. It is
// the inverse of cleanTown and mirrors utfkenall.Address.RawTown; build checks
// every record against it, so a cleaning rule that quietly ate text fails the
// build rather than shipping.
func rejoin(c cleanedTown) (kanji, kana string) {
	switch {
	case c.Kanji == "":
		return c.Note, c.NoteKana
	case c.Note == "":
		return c.Kanji, c.Kana
	}
	kana = c.Kana
	if c.NoteKana != "" {
		kana += "（" + c.NoteKana + "）"
	}
	return c.Kanji + "（" + c.Note + "）", kana
}
