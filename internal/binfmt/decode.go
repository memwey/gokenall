package binfmt

import (
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"sort"
	"strings"
	"time"
)

// maxPayload bounds the inflated payload. The real one is a few megabytes;
// this only exists so a corrupt or hostile file cannot exhaust memory.
const maxPayload = 1 << 28

// Store is a decoded database. Strings are slices of one shared blob, so
// reading a record costs no allocation.
type Store struct {
	blob   string
	strOff []uint32 // len(strOff) == numStrings+1

	prefs  [PrefectureCount]nameRef
	cities []cityRef

	zips      []uint32 // sorted ascending; parallel to the rec* slices
	recCity   []uint16
	recKanji  []uint32
	recKana   []uint32
	recRomaji []uint32
	recNote   sparseStringColumn
	recNoteKn sparseStringColumn
	recFlags  []uint8

	kenAllUpdated time.Time
	romeUpdated   time.Time
}

type nameRef struct{ kanji, kana, romaji uint32 }

type cityRef struct {
	nameRef
	jis  uint32
	pref uint8
}

// sparseStringColumn stores one presence bit per record and IDs only for the
// records that have a value. rank[word] is the number of set bits before that
// word, so looking up a present value remains O(1).
type sparseStringColumn struct {
	present []uint64
	rank    []uint32
	ids     []uint32
}

func (s sparseStringColumn) id(i int) uint32 {
	word, bit := i>>6, uint(i&63)
	v := s.present[word]
	if v&(uint64(1)<<bit) == 0 {
		return 0
	}
	before := v & ((uint64(1) << bit) - 1)
	return s.ids[int(s.rank[word])+bits.OnesCount64(before)]
}

// Entry is a record with every string resolved.
type Entry struct {
	Prefecture Name
	City       Name
	Town       Name
	Note       string
	NoteKana   string
	Zip        uint32
	JIS        uint32
	Flags      uint8
}

// Decode reads a database written by [Encode].
func Decode(r io.Reader) (*Store, error) {
	var head [HeaderSize]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, fmt.Errorf("binfmt: read header: %w", err)
	}
	if string(head[:len(Magic)]) != Magic {
		return nil, fmt.Errorf("binfmt: bad magic %q, not a utfkenall data file", head[:len(Magic)])
	}
	if v := head[len(Magic)]; v != Version {
		return nil, fmt.Errorf("binfmt: data file is format version %d, this build expects %d (regenerate with `go generate ./...`)", v, Version)
	}

	zr := flate.NewReader(r)
	defer zr.Close()
	payload, err := io.ReadAll(io.LimitReader(zr, maxPayload+1))
	if err != nil {
		return nil, fmt.Errorf("binfmt: decompress payload: %w", err)
	}
	if len(payload) > maxPayload {
		return nil, errors.New("binfmt: payload exceeds the size limit")
	}

	return unmarshal(payload)
}

func unmarshal(payload []byte) (*Store, error) {
	c := &cursor{buf: payload}
	s := &Store{}

	s.kenAllUpdated = time.Unix(c.varint(), 0).UTC()
	s.romeUpdated = time.Unix(c.varint(), 0).UTC()

	numStrings, err := c.count("strings")
	if err != nil {
		return nil, err
	}
	if numStrings == 0 {
		return nil, errors.New("binfmt: string table has no empty string")
	}
	// This is the expanded size, so front coding can legitimately make it
	// larger than the bytes left in the payload. Bound it independently before
	// using it as an allocation size.
	blobLen := int(c.bounded(maxPayload, "blob bytes"))
	if c.err != nil {
		return nil, c.err
	}
	// Every string but the empty one occupies at least one blob byte, and the
	// table is deduplicated, so the blob bounds how many there can be. Without
	// this a few hundred bytes could ask for a gigabyte of offsets.
	if numStrings > blobLen+1 {
		return nil, fmt.Errorf("binfmt: %d strings cannot fit in a %d byte blob", numStrings, blobLen)
	}
	s.strOff = make([]uint32, numStrings+1)
	var blob strings.Builder
	blob.Grow(blobLen)
	for i := range numStrings {
		s.strOff[i] = uint32(blob.Len())
		prefix := int(c.bounded(uint64(blobLen), "string prefix"))
		if i%stringRestartInterval == 0 && prefix != 0 {
			return nil, fmt.Errorf("binfmt: restart string %d has a %d byte prefix", i, prefix)
		}
		previous := ""
		if i > 0 {
			previous = blob.String()[s.strOff[i-1]:s.strOff[i]]
		}
		if prefix > len(previous) {
			return nil, fmt.Errorf("binfmt: string %d keeps a %d byte prefix from a %d byte predecessor", i, prefix, len(previous))
		}
		suffixLen, err := c.count("string suffix bytes")
		if err != nil {
			return nil, err
		}
		if blob.Len()+prefix+suffixLen > blobLen {
			return nil, fmt.Errorf("binfmt: string %d runs past the end of the %d byte blob", i, blobLen)
		}
		blob.WriteString(previous[:prefix])
		blob.Write(c.bytes(suffixLen))
	}
	s.strOff[numStrings] = uint32(blob.Len())
	if c.err != nil {
		return nil, c.err
	}
	if blob.Len() != blobLen {
		return nil, fmt.Errorf("binfmt: strings expand to %d bytes but the blob is %d bytes", blob.Len(), blobLen)
	}
	s.blob = blob.String()
	if s.str(0) != "" {
		return nil, errors.New("binfmt: string zero is not empty")
	}

	for i := range s.prefs {
		s.prefs[i] = c.nameRef()
	}

	numCities, err := c.count("cities")
	if err != nil {
		return nil, err
	}
	s.cities = make([]cityRef, numCities)
	for i := range s.cities {
		s.cities[i].jis = c.uvarint32()
		s.cities[i].pref = c.byte()
		s.cities[i].nameRef = c.nameRef()
	}

	n, err := c.count("records")
	if err != nil {
		return nil, err
	}
	// The deltas are unsigned and accumulate in a uint64 bounded on every step,
	// so the result is sorted by construction. Range binary searches it, and
	// that is the only thing keeping the search honest.
	s.zips = make([]uint32, n)
	var zip uint64
	for i := range s.zips {
		zip += uint64(c.uvarint32())
		if zip > maxZip {
			return nil, fmt.Errorf("binfmt: record %d has zip code %d, which is not seven digits", i, zip)
		}
		s.zips[i] = uint32(zip)
	}
	s.recCity = deltaColumn[uint16](c, n, math.MaxUint16, "city index")
	maxStringID := uint32(numStrings - 1)
	s.recKanji = deltaColumn[uint32](c, n, maxStringID, "town kanji string")
	s.recKana = deltaColumn[uint32](c, n, maxStringID, "town kana string")
	s.recRomaji = deltaColumn[uint32](c, n, maxStringID, "town romaji string")
	s.recNote = c.sparseStrings(n, maxStringID, "note")
	s.recNoteKn = c.sparseStrings(n, maxStringID, "note kana")
	s.recFlags = make([]uint8, n)
	for i := range s.recFlags {
		s.recFlags[i] = c.byte()
	}

	if c.err != nil {
		return nil, c.err
	}
	if err := s.check(); err != nil {
		return nil, err
	}
	if c.pos != len(c.buf) {
		return nil, fmt.Errorf("binfmt: %d trailing payload bytes", len(c.buf)-c.pos)
	}
	return s, nil
}

// check validates every index once at load time so that the hot lookup path
// can slice the blob without bounds reasoning of its own.
func (s *Store) check() error {
	maxStr := uint32(len(s.strOff) - 1)
	for i, id := range s.recKanji {
		if id >= maxStr || s.recKana[i] >= maxStr || s.recRomaji[i] >= maxStr {
			return fmt.Errorf("binfmt: record %d references a string out of range", i)
		}
		if int(s.recCity[i]) >= len(s.cities) {
			return fmt.Errorf("binfmt: record %d references city %d of %d", i, s.recCity[i], len(s.cities))
		}
	}
	for i, c := range s.cities {
		if c.kanji >= maxStr || c.kana >= maxStr || c.romaji >= maxStr {
			return fmt.Errorf("binfmt: city %d references a string out of range", i)
		}
		if int(c.pref) >= PrefectureCount {
			return fmt.Errorf("binfmt: city %d references prefecture %d", i, c.pref)
		}
	}
	for i, p := range s.prefs {
		if p.kanji >= maxStr || p.kana >= maxStr || p.romaji >= maxStr {
			return fmt.Errorf("binfmt: prefecture %d references a string out of range", i)
		}
	}
	return nil
}

func (s *Store) str(id uint32) string { return s.blob[s.strOff[id]:s.strOff[id+1]] }

func (s *Store) name(r nameRef) Name {
	return Name{Kanji: s.str(r.kanji), Kana: s.str(r.kana), Romaji: s.str(r.romaji)}
}

// Len reports the number of records.
func (s *Store) Len() int { return len(s.zips) }

// Updated reports the publication dates of the two source datasets.
func (s *Store) Updated() (kenAll, rome time.Time) { return s.kenAllUpdated, s.romeUpdated }

// Prefectures returns all 47 prefectures in JIS X0401 code order.
func (s *Store) Prefectures() []Name {
	out := make([]Name, PrefectureCount)
	for i, p := range s.prefs {
		out[i] = s.name(p)
	}
	return out
}

// PublishedPrefectureRomaji returns the exact prefecture spelling from
// KEN_ALL_ROME.CSV for a kanji prefecture name.
func (s *Store) PublishedPrefectureRomaji(kanji string) (string, bool) {
	for _, p := range s.prefs {
		if s.str(p.kanji) == kanji {
			return s.str(p.romaji), true
		}
	}
	return "", false
}

// At returns the i'th record. It panics if i is out of range.
func (s *Store) At(i int) Entry {
	city := s.cities[s.recCity[i]]
	return Entry{
		Prefecture: s.name(s.prefs[city.pref]),
		City:       s.name(city.nameRef),
		Town: Name{
			Kanji:  s.str(s.recKanji[i]),
			Kana:   s.str(s.recKana[i]),
			Romaji: s.str(s.recRomaji[i]),
		},
		Note:     s.str(s.recNote.id(i)),
		NoteKana: s.str(s.recNoteKn.id(i)),
		Zip:      s.zips[i],
		JIS:      city.jis,
		Flags:    s.recFlags[i],
	}
}

// Range returns the half-open index range of records carrying zip. The range
// is empty when the code is not in the database.
func (s *Store) Range(zip uint32) (start, end int) {
	start = sort.Search(len(s.zips), func(i int) bool { return s.zips[i] >= zip })
	end = start
	for end < len(s.zips) && s.zips[end] == zip {
		end++
	}
	return start, end
}

// cursor walks the payload, latching the first error so callers can parse
// straight through and check once at the end.
type cursor struct {
	buf []byte
	pos int
	err error
}

func (c *cursor) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf("binfmt: "+format, args...)
	}
}

func (c *cursor) byte() uint8 {
	if c.err != nil {
		return 0
	}
	if c.pos >= len(c.buf) {
		c.fail("payload truncated at offset %d", c.pos)
		return 0
	}
	b := c.buf[c.pos]
	c.pos++
	return b
}

func (c *cursor) bytes(n int) []byte {
	if c.err != nil {
		return nil
	}
	if n < 0 || c.pos+n > len(c.buf) {
		c.fail("payload truncated reading %d bytes at offset %d", n, c.pos)
		return nil
	}
	b := c.buf[c.pos : c.pos+n]
	c.pos += n
	return b
}

func (c *cursor) uvarint() uint64 {
	if c.err != nil {
		return 0
	}
	v, n := binary.Uvarint(c.buf[c.pos:])
	if n <= 0 {
		c.fail("malformed uvarint at offset %d", c.pos)
		return 0
	}
	c.pos += n
	return v
}

func (c *cursor) varint() int64 {
	if c.err != nil {
		return 0
	}
	v, n := binary.Varint(c.buf[c.pos:])
	if n <= 0 {
		c.fail("malformed varint at offset %d", c.pos)
		return 0
	}
	c.pos += n
	return v
}

// count reads a length prefix and refuses one that the rest of the payload
// could not possibly hold, before it is used to size an allocation. Every
// element costs at least one more byte downstream — a length varint, a field
// varint, a blob byte — so the bytes left are an upper bound on all of them.
func (c *cursor) count(what string) (int, error) {
	v := c.uvarint()
	if c.err != nil {
		return 0, c.err
	}
	if remaining := len(c.buf) - c.pos; v > uint64(remaining) {
		return 0, fmt.Errorf("binfmt: %s count is %d with only %d bytes left", what, v, remaining)
	}
	return int(v), nil
}

// bounded reads a uvarint that has to fit somewhere narrower than 64 bits.
// Truncating instead would turn an out-of-range value into an in-range one and
// slip past the checks downstream.
func (c *cursor) bounded(max uint64, what string) uint64 {
	v := c.uvarint()
	if c.err == nil && v > max {
		c.fail("%s is %d, which does not fit in the field", what, v)
		return 0
	}
	return v
}

func (c *cursor) uvarint32() uint32 { return uint32(c.bounded(math.MaxUint32, "value")) }

// deltaColumn expands one signed-delta column while checking both sides of
// the target integer range before addition. A malformed negative delta must
// not wrap around to a plausible positive string or city index.
func deltaColumn[T ~uint16 | ~uint32](c *cursor, n int, max uint32, what string) []T {
	out := make([]T, n)
	var previous int64
	for i := range out {
		delta := c.varint()
		if c.err != nil {
			return out
		}
		if delta < -previous || delta > int64(max)-previous {
			c.fail("%s delta at record %d leaves the field range", what, i)
			return out
		}
		previous += delta
		out[i] = T(previous)
	}
	return out
}

func (c *cursor) sparseStrings(records int, maxID uint32, what string) sparseStringColumn {
	count, err := c.count(what + " values")
	if err != nil {
		return sparseStringColumn{}
	}
	if count > records {
		c.fail("%s has %d values for %d records", what, count, records)
		return sparseStringColumn{}
	}
	out := sparseStringColumn{
		present: make([]uint64, (records+63)/64),
		rank:    make([]uint32, (records+63)/64),
		ids:     make([]uint32, count),
	}
	previous := -1
	for j := range count {
		gap := c.bounded(uint64(records), what+" record gap")
		i := previous + 1 + int(gap)
		if c.err != nil {
			return out
		}
		if i >= records {
			c.fail("%s value %d points at record %d of %d", what, j, i, records)
			return out
		}
		id := c.uvarint32()
		if c.err != nil {
			return out
		}
		if id == 0 || id > maxID {
			c.fail("%s value %d references string %d of %d", what, j, id, maxID+1)
			return out
		}
		out.present[i>>6] |= uint64(1) << uint(i&63)
		out.ids[j] = id
		previous = i
	}
	var rank uint32
	for i, word := range out.present {
		out.rank[i] = rank
		rank += uint32(bits.OnesCount64(word))
	}
	return out
}

func (c *cursor) nameRef() nameRef {
	return nameRef{
		kanji:  c.uvarint32(),
		kana:   c.uvarint32(),
		romaji: c.uvarint32(),
	}
}
