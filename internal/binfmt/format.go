// Package binfmt implements the compact on-disk format used to ship the
// Japan Post zip code database inside the utfkenall package.
//
// A file is a 5 byte plain header followed by a single DEFLATE stream:
//
//	"UKEN" | version uint8 | flate(payload)
//
// The payload is columnar: every field of the record table is written as its
// own run of varints rather than interleaving whole records. Neighbouring
// values in a column are near-identical, so string IDs are delta encoded before
// DEFLATE sees them. Strings are interned and sorted, then front coded against
// the previous string so their common prefixes are not stored repeatedly.
package binfmt

import "time"

// Magic identifies the file format.
const Magic = "UKEN"

// Version is the format revision. Decode rejects anything else, so bumping it
// forces a regenerated data file rather than a silent misparse.
const Version = 5

// HeaderSize is the number of uncompressed bytes preceding the flate stream.
const HeaderSize = len(Magic) + 1

// stringRestartInterval is how often a front-coded string is written in full
// rather than as a prefix of the one before it.
//
// The decoder expands the table at load, so it only ever reads these in order
// and does not need the restart points. They are here for the decoder that
// would keep the table front coded in memory and rebuild a string on demand by
// replaying from its nearest restart. That decoder was prototyped against the
// real database (251,159 strings) rather than left to the imagination, and the
// trade it offers is:
//
//	string table in memory   4.78 MiB expanded -> 2.06 MiB front coded
//	reading one string       2.4 ns, no allocation -> 385 ns and two
//	load                     about 11 ms shorter
//
// A shorter interval buys the speed back only part way: at 4 the table is
// 2.82 MiB and a read is 77 ns, still allocating, because every read has to
// materialise a string that slicing the blob gives away for free. An address
// resolves eleven strings, or five once cities and prefectures are cached, so
// even the best case turns a 90 ns lookup into several hundred.
//
// That is the wrong trade for a package whose point is fast offline lookup, so
// the option stays unexercised. Keeping the restart points open costs 20 KiB of
// the 1.67 MiB file, which is cheap enough to leave the door on its hinges.
const stringRestartInterval = 64

// The widths the codes are rendered at. A value beyond these would be printed
// modulo its width — 10000001 as "0000001" — so both sides of the format reject
// it rather than emit a plausible wrong answer.
const (
	maxZip = 9999999 // 7 digits
	maxJIS = 99999   // 5 digits
)

// PrefectureCount is the number of Japanese prefectures. The table is fixed
// length, indexed by prefecture code minus one.
const PrefectureCount = 47

// Record flags.
const (
	// FlagRomajiEstimated marks a record whose town romaji was transliterated
	// from katakana because Japan Post's romaji dataset had no entry for it.
	FlagRomajiEstimated uint8 = 1 << 0
)

// Name is a place name written three ways. In [Dataset] the fields hold the
// text itself; internally they are indices into the interned string table.
type Name struct {
	Kanji  string
	Kana   string
	Romaji string
}

// City is one 市区町村 keyed by its JIS X0402 code.
type City struct {
	Name
	// JIS is the 全国地方公共団体コード as a number, e.g. 13101 for 千代田区.
	JIS uint32
	// Pref is a zero-based index into [Dataset.Prefectures].
	Pref uint8
}

// Record is one line of the zip code database.
type Record struct {
	Town Name
	// Note is the annotation stripped from the raw town name, in kanji and in
	// kana, empty for the vast majority of records. For a record whose town is
	// a placeholder rather than a name, it holds that placeholder instead, so
	// nothing Japan Post publishes is discarded.
	Note     string
	NoteKana string
	// Zip is the 7 digit code as a number, e.g. 1000001. Leading zeroes are
	// implied by the fixed width.
	Zip uint32
	// City indexes [Dataset.Cities].
	City  uint16
	Flags uint8
}

// Dataset is the encoder's input: a complete database with plain strings.
type Dataset struct {
	KenAllUpdated time.Time
	RomeUpdated   time.Time
	Prefectures   [PrefectureCount]Name
	Cities        []City
	// Records must be sorted by Zip ascending. Records sharing a zip code stay
	// in the order given, which is the order Japan Post publishes them in.
	Records []Record
}
