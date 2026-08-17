package binfmt

import (
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
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
	recNote   []uint32
	recNoteKn []uint32
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
	blobLen, err := c.count("blob bytes")
	if err != nil {
		return nil, err
	}
	s.blob = string(c.bytes(blobLen))
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
	// Accumulating in uint64 and bounding against the blob on every step is
	// what makes str safe to slice without checking. Narrowing to uint32 here
	// would let a crafted length wrap the running offset back into range,
	// leaving the table non-monotonic and pointing outside the blob.
	var off uint64
	for i := range numStrings {
		s.strOff[i] = uint32(off)
		off += uint64(c.uvarint32())
		if off > uint64(blobLen) {
			return nil, fmt.Errorf("binfmt: string %d runs %d bytes past the end of the %d byte blob", i, off-uint64(blobLen), blobLen)
		}
	}
	s.strOff[numStrings] = uint32(off)
	if c.err != nil {
		return nil, c.err
	}
	if off != uint64(blobLen) {
		return nil, fmt.Errorf("binfmt: string lengths sum to %d but the blob is %d bytes", off, blobLen)
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
	s.recCity = make([]uint16, n)
	for i := range s.recCity {
		s.recCity[i] = uint16(c.bounded(math.MaxUint16, "city index"))
	}
	s.recKanji = c.uint32s(n)
	s.recKana = c.uint32s(n)
	s.recRomaji = c.uint32s(n)
	s.recNote = c.uint32s(n)
	s.recNoteKn = c.uint32s(n)
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
	return s, nil
}

// check validates every index once at load time so that the hot lookup path
// can slice the blob without bounds reasoning of its own.
func (s *Store) check() error {
	maxStr := uint32(len(s.strOff) - 1)
	for i, id := range s.recKanji {
		if id >= maxStr || s.recKana[i] >= maxStr || s.recRomaji[i] >= maxStr || s.recNote[i] >= maxStr || s.recNoteKn[i] >= maxStr {
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
		Note:     s.str(s.recNote[i]),
		NoteKana: s.str(s.recNoteKn[i]),
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

func (c *cursor) uint32s(n int) []uint32 {
	out := make([]uint32, n)
	for i := range out {
		out[i] = c.uvarint32()
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
