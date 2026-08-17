package binfmt

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"slices"
)

// Stats reports what Encode produced, for the generator's summary output.
type Stats struct {
	Records       int
	Cities        int
	UniqueStrings int
	BlobBytes     int
	PayloadBytes  int
	FileBytes     int
}

// Encode writes d to w in the format described in the package doc.
//
// d.Records must be sorted by Zip ascending; Encode returns an error rather
// than writing a file whose binary search would silently misbehave.
func Encode(w io.Writer, d *Dataset) (Stats, error) {
	if err := validate(d); err != nil {
		return Stats{}, err
	}

	tab := intern(d)
	payload := marshal(d, tab)

	var buf bytes.Buffer
	buf.WriteString(Magic)
	buf.WriteByte(Version)
	zw, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return Stats{}, fmt.Errorf("binfmt: new flate writer: %w", err)
	}
	if _, err := zw.Write(payload); err != nil {
		return Stats{}, fmt.Errorf("binfmt: compress payload: %w", err)
	}
	if err := zw.Close(); err != nil {
		return Stats{}, fmt.Errorf("binfmt: flush payload: %w", err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return Stats{}, fmt.Errorf("binfmt: write file: %w", err)
	}

	return Stats{
		Records:       len(d.Records),
		Cities:        len(d.Cities),
		UniqueStrings: len(tab.sorted),
		BlobBytes:     tab.blobLen,
		PayloadBytes:  len(payload),
		FileBytes:     buf.Len(),
	}, nil
}

func validate(d *Dataset) error {
	for i, pref := range d.Prefectures {
		if pref.Romaji == "" {
			return fmt.Errorf("binfmt: prefecture %d has no romaji", i)
		}
	}
	if len(d.Cities) > math.MaxUint16 {
		return fmt.Errorf("binfmt: %d cities exceeds the uint16 record field", len(d.Cities))
	}
	for i, c := range d.Cities {
		if int(c.Pref) >= PrefectureCount {
			return fmt.Errorf("binfmt: city %d (%s) has prefecture index %d", i, c.Kanji, c.Pref)
		}
		if c.JIS > maxJIS {
			return fmt.Errorf("binfmt: city %d (%s) has JIS code %d, which is more than 5 digits", i, c.Kanji, c.JIS)
		}
	}
	for i, r := range d.Records {
		if r.Zip > maxZip {
			return fmt.Errorf("binfmt: record %d has zip code %d, which is more than 7 digits", i, r.Zip)
		}
		if int(r.City) >= len(d.Cities) {
			return fmt.Errorf("binfmt: record %d (zip %07d) has city index %d of %d", i, r.Zip, r.City, len(d.Cities))
		}
		if i > 0 && r.Zip < d.Records[i-1].Zip {
			return fmt.Errorf("binfmt: records not sorted by zip: %07d follows %07d at index %d", r.Zip, d.Records[i-1].Zip, i)
		}
	}
	return nil
}

// stringTable assigns every distinct string an index. Sorting before assigning
// is what makes the blob compress: it puts strings sharing a prefix next to
// each other, and DEFLATE encodes the second one as a back-reference.
type stringTable struct {
	sorted  []string
	index   map[string]uint32
	blobLen int
}

func intern(d *Dataset) *stringTable {
	set := make(map[string]struct{}, 4*len(d.Records))
	// The empty string sorts first, so index 0 always means "absent". Decode
	// and the record loader both rely on that.
	set[""] = struct{}{}

	addName := func(n Name) {
		set[n.Kanji] = struct{}{}
		set[n.Kana] = struct{}{}
		set[n.Romaji] = struct{}{}
	}
	for _, n := range d.Prefectures {
		addName(n)
	}
	for _, c := range d.Cities {
		addName(c.Name)
	}
	for _, r := range d.Records {
		addName(r.Town)
		set[r.Note] = struct{}{}
		set[r.NoteKana] = struct{}{}
	}

	tab := &stringTable{
		sorted: make([]string, 0, len(set)),
		index:  make(map[string]uint32, len(set)),
	}
	for s := range set {
		tab.sorted = append(tab.sorted, s)
	}
	slices.Sort(tab.sorted)
	for i, s := range tab.sorted {
		tab.index[s] = uint32(i)
		tab.blobLen += len(s)
	}
	return tab
}

func (t *stringTable) id(s string) uint32 { return t.index[s] }

func marshal(d *Dataset, tab *stringTable) []byte {
	// Rough guess: the blob plus a handful of varints per record.
	buf := make([]byte, 0, tab.blobLen+16*len(d.Records))

	buf = binary.AppendVarint(buf, d.KenAllUpdated.Unix())
	buf = binary.AppendVarint(buf, d.RomeUpdated.Unix())

	buf = binary.AppendUvarint(buf, uint64(len(tab.sorted)))
	buf = binary.AppendUvarint(buf, uint64(tab.blobLen))
	var previous string
	for i, s := range tab.sorted {
		prefix := 0
		if i%stringRestartInterval != 0 {
			prefix = commonPrefixLen(previous, s)
		}
		suffix := s[prefix:]
		buf = binary.AppendUvarint(buf, uint64(prefix))
		buf = binary.AppendUvarint(buf, uint64(len(suffix)))
		buf = append(buf, suffix...)
		previous = s
	}

	for _, n := range d.Prefectures {
		buf = binary.AppendUvarint(buf, uint64(tab.id(n.Kanji)))
		buf = binary.AppendUvarint(buf, uint64(tab.id(n.Kana)))
		buf = binary.AppendUvarint(buf, uint64(tab.id(n.Romaji)))
	}

	buf = binary.AppendUvarint(buf, uint64(len(d.Cities)))
	for _, c := range d.Cities {
		buf = binary.AppendUvarint(buf, uint64(c.JIS))
		buf = append(buf, c.Pref)
		buf = binary.AppendUvarint(buf, uint64(tab.id(c.Kanji)))
		buf = binary.AppendUvarint(buf, uint64(tab.id(c.Kana)))
		buf = binary.AppendUvarint(buf, uint64(tab.id(c.Romaji)))
	}

	// Records, one column at a time.
	buf = binary.AppendUvarint(buf, uint64(len(d.Records)))
	var prev uint32
	for _, r := range d.Records {
		buf = binary.AppendUvarint(buf, uint64(r.Zip-prev))
		prev = r.Zip
	}
	buf = appendDeltaColumn(buf, d.Records, func(r Record) uint32 { return uint32(r.City) })
	buf = appendDeltaColumn(buf, d.Records, func(r Record) uint32 { return tab.id(r.Town.Kanji) })
	buf = appendDeltaColumn(buf, d.Records, func(r Record) uint32 { return tab.id(r.Town.Kana) })
	buf = appendDeltaColumn(buf, d.Records, func(r Record) uint32 { return tab.id(r.Town.Romaji) })
	buf = appendSparseColumn(buf, d.Records, func(r Record) uint32 { return tab.id(r.Note) })
	buf = appendSparseColumn(buf, d.Records, func(r Record) uint32 { return tab.id(r.NoteKana) })
	for _, r := range d.Records {
		buf = append(buf, r.Flags)
	}

	return buf
}

func commonPrefixLen(a, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// appendDeltaColumn writes signed differences between adjacent values. Records
// are grouped geographically, so their city and town string IDs tend to stay
// close; small differences take fewer bytes and leave longer repeated runs for
// DEFLATE than writing the absolute IDs.
func appendDeltaColumn(buf []byte, records []Record, value func(Record) uint32) []byte {
	var previous int64
	for _, r := range records {
		v := int64(value(r))
		buf = binary.AppendVarint(buf, v-previous)
		previous = v
	}
	return buf
}

// appendSparseColumn writes only non-empty values. Notes occur on fewer than
// one record in fifteen, so a dense column spends almost all of its bytes on
// zeros even before it becomes a much larger []uint32 at load time. Gaps are
// measured from the record after the previous value, making every gap
// non-negative and keeping adjacent notes cheap.
func appendSparseColumn(buf []byte, records []Record, value func(Record) uint32) []byte {
	count := 0
	for _, r := range records {
		if value(r) != 0 {
			count++
		}
	}
	buf = binary.AppendUvarint(buf, uint64(count))
	previous := -1
	for i, r := range records {
		id := value(r)
		if id == 0 {
			continue
		}
		buf = binary.AppendUvarint(buf, uint64(i-previous-1))
		buf = binary.AppendUvarint(buf, uint64(id))
		previous = i
	}
	return buf
}
