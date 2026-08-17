package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/memwey/utfkenall/internal/kana"
)

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
		words = append(words, kana.Capitalize(tok))
	}
	return strings.Join(words, " ")
}

func prettyTown(s string) string { return kana.Titlecase(s) }

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
	// prefectures keeps the exact spelling from KEN_ALL_ROME.CSV. The public
	// Name.Romaji uses conventional English instead, but the source value must
	// remain available to callers that need fidelity to Japan Post's file.
	prefectures map[string]string
}

func newRomajiIndex(rows []romeRow) (*romajiIndex, error) {
	ix := &romajiIndex{
		towns:       make(map[townKey]string, len(rows)),
		byZip:       make(map[string][]string, len(rows)),
		cities:      make(map[[2]string]string, 2000),
		prefectures: make(map[string]string, 47),
	}
	for _, r := range rows {
		if previous, ok := ix.prefectures[r.Pref]; ok && previous != r.PrefRomaji {
			return nil, fmt.Errorf("prefecture %q has conflicting romaji %q and %q", r.Pref, previous, r.PrefRomaji)
		}
		ix.prefectures[r.Pref] = r.PrefRomaji
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
		if !slices.Contains(ix.byZip[r.Zip], romaji) {
			ix.byZip[r.Zip] = append(ix.byZip[r.Zip], romaji)
		}
	}
	return ix, nil
}

func (ix *romajiIndex) city(pref, city string) (string, bool) {
	v, ok := ix.cities[[2]string{pref, city}]
	return v, ok
}

func (ix *romajiIndex) prefecture(pref string) (string, bool) {
	v, ok := ix.prefectures[pref]
	return v, ok && v != ""
}
