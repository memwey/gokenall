package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/memwey/utfkenall/internal/binfmt"
	"github.com/memwey/utfkenall/internal/kana"
)

type buildStats struct {
	Rows           int
	EmptyTown      int
	Cities         int
	RomajiExact    int
	RomajiByZip    int
	RomajiEstimate int
	CityEstimate   int

	// Every officially spelled town is also run through [Hepburn], so each
	// build reports how well the fallback would have done on names where the
	// right answer is known.
	HepburnChecked int
	// HepburnAgreed counts exact agreement. HepburnSounded ignores spaces and
	// hyphens, which is the fairer number: Japan Post breaks words where the
	// kanji has a space (「宮の森　一条」 -> "MIYANOMORI 1-JO") and nothing in the
	// kana column records that.
	HepburnAgreed   int
	HepburnSounded  int
	HepburnMismatch []string
}

func (s buildStats) named() int { return s.Rows - s.EmptyTown }

// hepburnSamples bounds the mismatch examples kept for -v output.
const hepburnSamples = 20

// build turns the two parsed datasets into the structure the encoder writes.
func build(ken []kenRow, rome []romeRow, kenDate, romeDate time.Time) (*binfmt.Dataset, buildStats, error) {
	ix := newRomajiIndex(rome)
	d := &binfmt.Dataset{KenAllUpdated: kenDate, RomeUpdated: romeDate}
	st := buildStats{Rows: len(ken)}

	// Prefectures, indexed by JIS X0401 code minus one.
	var seenPref [binfmt.PrefectureCount]bool
	for _, row := range ken {
		code, err := prefCode(row.JIS)
		if err != nil {
			return nil, st, err
		}
		if seenPref[code] {
			continue
		}
		romaji, err := prefectureRomaji(row.Pref)
		if err != nil {
			return nil, st, err
		}
		d.Prefectures[code] = binfmt.Name{Kanji: row.Pref, Kana: row.PrefKana, Romaji: romaji}
		seenPref[code] = true
	}
	for i, ok := range seenPref {
		if !ok {
			return nil, st, fmt.Errorf("prefecture code %02d is missing from the data", i+1)
		}
	}

	// Cities, in JIS code order so the index is stable across regenerations.
	cityIndex := make(map[string]uint16)
	for _, row := range ken {
		if _, ok := cityIndex[row.JIS]; ok {
			continue
		}
		code, err := prefCode(row.JIS)
		if err != nil {
			return nil, st, err
		}
		jis, err := strconv.ParseUint(row.JIS, 10, 32)
		if err != nil {
			return nil, st, fmt.Errorf("city code %q: %w", row.JIS, err)
		}
		romaji, ok := ix.city(row.Pref, row.City)
		if !ok {
			romaji = kana.PostalRomaji(row.CityKana)
			st.CityEstimate++
		}
		cityIndex[row.JIS] = uint16(len(d.Cities))
		d.Cities = append(d.Cities, binfmt.City{
			Name: binfmt.Name{Kanji: row.City, Kana: row.CityKana, Romaji: prettyCity(romaji)},
			JIS:  uint32(jis),
			Pref: uint8(code),
		})
	}
	slices.SortStableFunc(d.Cities, func(a, b binfmt.City) int { return int(a.JIS) - int(b.JIS) })
	for i, c := range d.Cities {
		cityIndex[fmt.Sprintf("%05d", c.JIS)] = uint16(i)
	}
	st.Cities = len(d.Cities)

	// A zip code whose town Japan Post renders only one way can borrow that
	// spelling even when the exact key does not match, which happens when the
	// two files disagree about how a name is punctuated.
	kenTowns := make(map[string]map[string]bool, len(ken))
	for _, row := range ken {
		t := cleanTown(row.Town, row.TownKana).Kanji
		if t == "" {
			continue
		}
		if kenTowns[row.Zip] == nil {
			kenTowns[row.Zip] = make(map[string]bool, 1)
		}
		kenTowns[row.Zip][t] = true
	}

	d.Records = make([]binfmt.Record, 0, len(ken))
	for _, row := range ken {
		zip, err := strconv.ParseUint(row.Zip, 10, 32)
		if err != nil || len(row.Zip) != 7 {
			return nil, st, fmt.Errorf("zip code %q is not 7 digits", row.Zip)
		}
		town := cleanTown(row.Town, row.TownKana)

		rec := binfmt.Record{
			Zip:  uint32(zip),
			City: cityIndex[row.JIS],
			Note: town.Note,
			Town: binfmt.Name{Kanji: town.Kanji, Kana: town.Kana},
		}
		if town.Kanji == "" {
			st.EmptyTown++
		} else {
			romaji, tier := ix.townRomaji(row, town.Kanji, kenTowns[row.Zip])
			switch tier {
			case tierExact:
				st.RomajiExact++
				st.checkHepburn(town.Kana, romaji, town.Kanji)
			case tierByZip:
				st.RomajiByZip++
			default:
				romaji = kana.PostalRomaji(town.Kana)
				rec.Flags |= binfmt.FlagRomajiEstimated
				st.RomajiEstimate++
			}
			rec.Town.Romaji = prettyTown(romaji)
		}
		d.Records = append(d.Records, rec)
	}

	slices.SortStableFunc(d.Records, func(a, b binfmt.Record) int { return int(a.Zip) - int(b.Zip) })
	return d, st, nil
}

// checkHepburn scores the transliterator against a name Japan Post has already
// spelled for us. Nothing depends on the outcome; it is a running measure of
// how much to trust the fallback on the names they have not.
func (s *buildStats) checkHepburn(reading, official, kanji string) {
	if reading == "" || official == "" {
		return
	}
	s.HepburnChecked++
	got := kana.PostalRomaji(reading)
	if got == official {
		s.HepburnAgreed++
		s.HepburnSounded++
		return
	}
	if letters(got) == letters(official) {
		s.HepburnSounded++
		return
	}
	if len(s.HepburnMismatch) < hepburnSamples {
		s.HepburnMismatch = append(s.HepburnMismatch,
			fmt.Sprintf("%s (%s): official %s, transliterated %s", kanji, reading, official, got))
	}
}

// letters drops everything that is not a letter or a digit, so two spellings
// can be compared on sound alone.
func letters(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type romajiTier int

const (
	tierNone romajiTier = iota
	tierExact
	tierByZip
)

func (ix *romajiIndex) townRomaji(row kenRow, town string, sameZip map[string]bool) (string, romajiTier) {
	key := townKey{zip: row.Zip, pref: row.Pref, city: row.City, town: town}
	if v, ok := ix.towns[key]; ok && v != "" {
		return v, tierExact
	}
	// Only safe when neither file splits the zip code across several towns;
	// otherwise we would hand one town another town's spelling.
	if len(sameZip) == 1 {
		if v := ix.byZip[row.Zip]; len(v) == 1 {
			return v[0], tierByZip
		}
	}
	return "", tierNone
}

func prefCode(jis string) (uint8, error) {
	if len(jis) != 5 {
		return 0, fmt.Errorf("city code %q is not 5 digits", jis)
	}
	n, err := strconv.Atoi(jis[:2])
	if err != nil || n < 1 || n > binfmt.PrefectureCount {
		return 0, fmt.Errorf("city code %q has no valid prefecture prefix", jis)
	}
	return uint8(n - 1), nil
}
