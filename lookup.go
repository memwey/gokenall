package utfkenall

import (
	"errors"
	"iter"
	"time"
)

var (
	// ErrNotFound means the code is well formed but this database has no
	// address for it. Note that Japan Post issues codes to large organisations
	// separately, and those are not included here — see the package
	// documentation.
	ErrNotFound = errors.New("utfkenall: zip code not found")

	// ErrInvalidCode means the argument is not a 7 digit zip code.
	ErrInvalidCode = errors.New("utfkenall: not a 7-digit zip code")
)

// Lookup returns the address for a zip code.
//
// The code may be written any of the ways people write them — "1000001",
// "100-0001", "〒100-0001", "１００−０００１" — as long as it holds seven digits.
//
// A few thousand codes cover more than one town; for those Lookup returns the
// first in Japan Post's order, and [LookupAll] returns them all.
func Lookup(code string) (Address, error) {
	zip, ok := parseCode(code)
	if !ok {
		return Address{}, ErrInvalidCode
	}
	db, err := decode()
	if err != nil {
		return Address{}, err
	}
	start, end := db.store.Range(zip)
	if start == end {
		return Address{}, ErrNotFound
	}
	return db.address(db.store.At(start)), nil
}

// LookupAll returns every address registered under a zip code, in the order
// Japan Post publishes them. The result always holds at least one address; a
// code with none is reported as [ErrNotFound].
func LookupAll(code string) ([]Address, error) {
	zip, ok := parseCode(code)
	if !ok {
		return nil, ErrInvalidCode
	}
	db, err := decode()
	if err != nil {
		return nil, err
	}
	start, end := db.store.Range(zip)
	if start == end {
		return nil, ErrNotFound
	}
	out := make([]Address, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, db.address(db.store.At(i)))
	}
	return out, nil
}

// Prefectures returns all 47 prefectures in JIS X0401 code order, so index 0
// is 北海道 and index 46 is 沖縄県.
func Prefectures() []Name {
	db := mustLoad()
	return append([]Name(nil), db.prefectures[:]...)
}

// PublishedPrefectureRomaji returns the prefecture spelling exactly as Japan
// Post publishes it in KEN_ALL_ROME.CSV. The argument is the kanji name, such
// as "東京都". The boolean is false when it is not one of the 47 prefectures.
//
// [Name.Romaji] on values returned by [Prefectures] remains the conventional
// English form intended for addresses, such as "Tokyo" rather than the
// published "TOKYO TO".
func PublishedPrefectureRomaji(kanji string) (string, bool) {
	return mustLoad().store.PublishedPrefectureRomaji(kanji)
}

// All iterates every address in the database, ordered by zip code.
func All() iter.Seq[Address] {
	return func(yield func(Address) bool) {
		db := mustLoad()
		for i := range db.store.Len() {
			if !yield(db.address(db.store.At(i))) {
				return
			}
		}
	}
}

// Len reports how many addresses the database holds. It exceeds the number of
// distinct zip codes, because some codes cover several towns.
func Len() int { return mustLoad().store.Len() }

// DataUpdated reports when Japan Post last published each of the two source
// datasets. The romaji file is revised far less often than the addresses,
// which is why some addresses carry [Address.RomajiEstimated].
func DataUpdated() (addresses, romaji time.Time) { return mustLoad().store.Updated() }

// parseCode extracts the seven digits of a zip code, tolerating the separators
// and full-width digits that turn up in real input.
func parseCode(s string) (uint32, bool) {
	var zip uint32
	var n int
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			r -= '0'
		case r >= '０' && r <= '９':
			r -= '０'
		case isCodeSeparator(r):
			continue
		default:
			return 0, false
		}
		if n == 7 {
			return 0, false
		}
		zip = zip*10 + uint32(r)
		n++
	}
	return zip, n == 7
}

// isCodeSeparator covers the postal mark, spaces, and the assortment of
// hyphens and dashes that get typed or pasted in place of one.
func isCodeSeparator(r rune) bool {
	switch r {
	case '〒', ' ', '\t', '　',
		'-', '‐', '‑', '‒', '–', '—', '―', '−', 'ー', '－', 'ｰ':
		return true
	}
	return false
}
