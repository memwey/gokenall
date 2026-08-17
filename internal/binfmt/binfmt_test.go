package binfmt

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func sample() *Dataset {
	d := &Dataset{
		KenAllUpdated: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		RomeUpdated:   time.Date(2025, 6, 18, 0, 0, 0, 0, time.UTC),
		Cities: []City{
			{Name: Name{Kanji: "千代田区", Kana: "チヨダク", Romaji: "Chiyoda-ku"}, JIS: 13101, Pref: 12},
			{Name: Name{Kanji: "札幌市中央区", Kana: "サッポロシチュウオウク", Romaji: "Sapporo-shi Chuo-ku"}, JIS: 1101, Pref: 0},
		},
		Records: []Record{
			{Zip: 600042, City: 1, Town: Name{Kanji: "大通西", Kana: "オオドオリニシ", Romaji: "Odorinishi"}, Note: "１〜１９丁目", NoteKana: "１−１９チョウメ"},
			{Zip: 1000001, City: 0, Town: Name{Kanji: "千代田", Kana: "チヨダ", Romaji: "Chiyoda"}},
			{Zip: 1000005, City: 0, Town: Name{Kanji: "丸の内", Kana: "マルノウチ", Romaji: "Marunochi"}, Flags: FlagRomajiEstimated},
			{Zip: 1000005, City: 0, Town: Name{Kanji: "", Kana: "", Romaji: ""}},
		},
	}
	for i := range d.Prefectures {
		d.Prefectures[i] = Name{Kanji: "県", Kana: "ケン", Romaji: "Ken"}
	}
	d.Prefectures[0] = Name{Kanji: "北海道", Kana: "ホッカイドウ", Romaji: "Hokkaido"}
	d.Prefectures[12] = Name{Kanji: "東京都", Kana: "トウキョウト", Romaji: "Tokyo"}
	return d
}

func encodeSample(t *testing.T, d *Dataset) *Store {
	t.Helper()
	var buf bytes.Buffer
	if _, err := Encode(&buf, d); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	s, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return s
}

func TestRoundTrip(t *testing.T) {
	d := sample()
	s := encodeSample(t, d)

	if s.Len() != len(d.Records) {
		t.Fatalf("decoded %d records, encoded %d", s.Len(), len(d.Records))
	}
	ken, rome := s.Updated()
	if !ken.Equal(d.KenAllUpdated) || !rome.Equal(d.RomeUpdated) {
		t.Errorf("dates round-tripped as %v/%v, want %v/%v", ken, rome, d.KenAllUpdated, d.RomeUpdated)
	}

	for i, want := range d.Records {
		got := s.At(i)
		city := d.Cities[want.City]
		switch {
		case got.Zip != want.Zip:
			t.Errorf("record %d zip = %d, want %d", i, got.Zip, want.Zip)
		case got.Town != want.Town:
			t.Errorf("record %d town = %v, want %v", i, got.Town, want.Town)
		case got.Note != want.Note || got.NoteKana != want.NoteKana:
			t.Errorf("record %d note = %q/%q, want %q/%q", i, got.Note, got.NoteKana, want.Note, want.NoteKana)
		case got.Flags != want.Flags:
			t.Errorf("record %d flags = %d, want %d", i, got.Flags, want.Flags)
		case got.JIS != city.JIS:
			t.Errorf("record %d JIS = %d, want %d", i, got.JIS, city.JIS)
		case got.City != city.Name:
			t.Errorf("record %d city = %v, want %v", i, got.City, city.Name)
		case got.Prefecture != d.Prefectures[city.Pref]:
			t.Errorf("record %d prefecture = %v, want %v", i, got.Prefecture, d.Prefectures[city.Pref])
		}
	}

	prefs := s.Prefectures()
	if len(prefs) != PrefectureCount {
		t.Fatalf("got %d prefectures, want %d", len(prefs), PrefectureCount)
	}
	if prefs[12].Romaji != "Tokyo" {
		t.Errorf("prefecture 12 = %v, want Tokyo", prefs[12])
	}
}

func TestRange(t *testing.T) {
	s := encodeSample(t, sample())
	tests := []struct {
		zip              uint32
		wantStart, wantN int
	}{
		{600042, 0, 1},
		{1000001, 1, 1},
		{1000005, 2, 2}, // two towns share the code
		{1, 0, 0},       // before everything
		{9999999, 4, 0}, // after everything
		{700000, 1, 0},  // in a gap
	}
	for _, tt := range tests {
		start, end := s.Range(tt.zip)
		if start != tt.wantStart || end-start != tt.wantN {
			t.Errorf("Range(%d) = [%d,%d), want start %d and %d records", tt.zip, start, end, tt.wantStart, tt.wantN)
		}
	}
}

// Strings appearing in many records must be stored once, since that is the
// whole reason the format has a string table.
func TestStringsAreInterned(t *testing.T) {
	d := sample()
	for i := range 500 {
		d.Records = append(d.Records, Record{
			Zip:  uint32(2000000 + i),
			City: 0,
			Town: Name{Kanji: "千代田", Kana: "チヨダ", Romaji: "Chiyoda"},
		})
	}
	var buf bytes.Buffer
	st, err := Encode(&buf, d)
	if err != nil {
		t.Fatal(err)
	}
	// Roughly: the distinct names above, not 500 copies of them.
	if st.UniqueStrings > 40 {
		t.Errorf("%d unique strings for %d records, expected the repeats to collapse", st.UniqueStrings, len(d.Records))
	}
	if st.BlobBytes > 500 {
		t.Errorf("blob is %d bytes, expected the repeats to collapse", st.BlobBytes)
	}
}

func TestEncodeRejectsUnsortedRecords(t *testing.T) {
	d := sample()
	d.Records[0], d.Records[1] = d.Records[1], d.Records[0]
	_, err := Encode(&bytes.Buffer{}, d)
	if err == nil || !strings.Contains(err.Error(), "not sorted") {
		t.Fatalf("Encode of unsorted records = %v, want a sort complaint", err)
	}
}

func TestEncodeRejectsDanglingIndices(t *testing.T) {
	t.Run("city", func(t *testing.T) {
		d := sample()
		d.Records[0].City = 99
		if _, err := Encode(&bytes.Buffer{}, d); err == nil {
			t.Fatal("Encode accepted a record pointing at a city that does not exist")
		}
	})
	t.Run("prefecture", func(t *testing.T) {
		d := sample()
		d.Cities[0].Pref = PrefectureCount
		if _, err := Encode(&bytes.Buffer{}, d); err == nil {
			t.Fatal("Encode accepted a city pointing at a prefecture that does not exist")
		}
	})
}

func TestDecodeRejectsBadFiles(t *testing.T) {
	var good bytes.Buffer
	if _, err := Encode(&good, sample()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		file []byte
		want string
	}{
		{"empty", nil, "read header"},
		{"short", []byte("UK"), "read header"},
		{"wrong magic", append([]byte("NOPE\x01"), good.Bytes()[HeaderSize:]...), "bad magic"},
		{"wrong version", append([]byte(Magic+"\xff"), good.Bytes()[HeaderSize:]...), "format version"},
		{"truncated payload", good.Bytes()[:HeaderSize+20], ""},
		{"corrupt payload", append(append([]byte{}, good.Bytes()[:HeaderSize]...), 0xde, 0xad, 0xbe, 0xef), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(bytes.NewReader(tt.file))
			if err == nil {
				t.Fatal("Decode accepted the file")
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// Indices are validated once at load time so the lookup path can slice the
// blob without checking. Anything that slipped through would panic on the
// first record that reached it.
func TestCheckRejectsOutOfRangeIndices(t *testing.T) {
	// Two strings, so valid indices are 0 and 1.
	base := func() *Store {
		return &Store{
			blob:      "ab",
			strOff:    []uint32{0, 1, 2},
			cities:    []cityRef{{jis: 13101}},
			zips:      []uint32{1000001},
			recCity:   []uint16{0},
			recKanji:  []uint32{0},
			recKana:   []uint32{0},
			recRomaji: []uint32{0},
			recNote:   []uint32{0},
			recNoteKn: []uint32{0},
			recFlags:  []uint8{0},
		}
	}
	if err := base().check(); err != nil {
		t.Fatalf("a valid store was rejected: %v", err)
	}

	tests := map[string]func(*Store){
		"record string":     func(s *Store) { s.recKanji[0] = 2 },
		"record note":       func(s *Store) { s.recNote[0] = 99 },
		"record note kana":  func(s *Store) { s.recNoteKn[0] = 99 },
		"record city":       func(s *Store) { s.recCity[0] = 7 },
		"city string":       func(s *Store) { s.cities[0].kana = 2 },
		"city prefecture":   func(s *Store) { s.cities[0].pref = PrefectureCount },
		"prefecture string": func(s *Store) { s.prefs[3].romaji = 2 },
	}
	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			s := base()
			corrupt(s)
			if err := s.check(); err == nil {
				t.Error("check accepted an out-of-range index")
			}
		})
	}
}

func BenchmarkDecode(b *testing.B) {
	d := sample()
	var buf bytes.Buffer
	if _, err := Encode(&buf, d); err != nil {
		b.Fatal(err)
	}
	file := buf.Bytes()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(bytes.NewReader(file)); err != nil {
			b.Fatal(err)
		}
	}
}
