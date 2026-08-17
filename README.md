# utfkenall

[![CI](https://github.com/memwey/utfkenall/actions/workflows/ci.yml/badge.svg)](https://github.com/memwey/utfkenall/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/memwey/utfkenall.svg)](https://pkg.go.dev/github.com/memwey/utfkenall)
[![Apache License](https://img.shields.io/badge/license-Apache-blue.svg)](LICENSE)

Japanese postal codes to place names, offline.

Japan Post's zip code database ships inside the package, so a lookup needs no
network, no data files next to your binary, and no setup. Every place name
comes in three writings: kanji, katakana, and romaji.

```go
import "github.com/memwey/utfkenall"

addr, err := utfkenall.Lookup("100-0001")

addr.Prefecture // {東京都  トウキョウト  Tokyo}
addr.City       // {千代田区 チヨダク     Chiyoda-ku}
addr.Town       // {千代田  チヨダ       Chiyoda}

addr.String()   // 東京都千代田区千代田
addr.English()  // Chiyoda, Chiyoda-ku, Tokyo
addr.JISCode    // 13101
```

```sh
go get github.com/memwey/utfkenall
```

Requires Go 1.24. The library itself has **no dependencies** — only the
standard library.

## API

| | |
|---|---|
| `Lookup(code) (Address, error)` | the address for a zip code |
| `LookupAll(code) ([]Address, error)` | every address under it — a few thousand codes cover more than one town |
| `Prefectures() []Name` | all 47, in JIS X0401 order |
| `PublishedPrefectureRomaji(kanji) (string, bool)` | Japan Post's original prefecture spelling |
| `All() iter.Seq[Address]` | every address, ordered by code |
| `Len() int` | how many there are |
| `DataUpdated() (addresses, romaji time.Time)` | when Japan Post last published each source |
| `Load() error` | decode the database now instead of on first use |
| `Transliterate(kana) string` | the other romaji spelling — see below |

Codes may be written any of the ways people write them — `1000001`,
`100-0001`, `〒100-0001`, `１００−０００１` — as long as they hold seven digits.
Anything else is `ErrInvalidCode`; a well-formed code Japan Post does not
publish is `ErrNotFound`.

### `Town` is sometimes empty, and `Note` says why

Japan Post uses the town column for two things that are not town names. Some
codes cover a municipality as a whole, and get prose where the name would go —
「以下に掲載がない場合」, 「○○市の次に番地がくる場合」, 「○○村一円」. Others carry a
parenthesised annotation: 大通西（１〜１９丁目）.

Both move out of `Town` so that names compare and display cleanly, and both
land in `Note`. `Town` tells you which is which:

| | `Town` | `Note` |
|---|---|---|
| 大通西（１〜１９丁目） | `大通西` | `１〜１９丁目` |
| 以下に掲載がない場合 | *empty* | `以下に掲載がない場合` |
| 千代田 | `千代田` | *empty* |

Ranges are **not** expanded into one record per 丁目. That would invent zip code
entries Japan Post never published; if you want them, `Note` has what you need.

### Nothing is discarded

Uninformative annotations are kept too — 「その他」, 「地階・階層不明」 — because a
caller can ignore a note far more easily than it can recover one. `RawTown`
puts the source columns back together:

```go
a, _ := utfkenall.Lookup("060-0042")
a.Town.Kanji   // 大通西
a.Note.Kanji   // １〜１９丁目
a.RawTown()    // 大通西（１〜１９丁目）, オオドオリニシ（１−１９チョウメ）
```

This is not an approximate reconstruction: the pieces are stored as they were
read, and the generator asserts the round trip on all 124,513 records of every
build. Keeping the annotations and their readings costs about 30 KiB.

### Two romaji spellings

`Name.Romaji` is Japan Post's own spelling, which leaves long vowels unmarked.
That is the ordinary English convention — 東京 is `Tokyo`, not `Tōkyō` — but
applied mechanically it misreads a vowel pair that spans a word boundary. 丸の内
is *maru-no-uchi*, and they publish `Marunochi`.

`Transliterate` gives the other reading of the same katakana, spelling out every
kana. Each convention is right exactly where the other is wrong:

| | `Name.Romaji` | `Transliterate(Name.Kana)` |
|---|---|---|
| 丸の内 マルノウチ | `Marunochi` ✗ | `Marunouchi` ✓ |
| 東京 トウキョウ | `Tokyo` ✓ | `Toukyou` ✗ |
| 中央 チュウオウ | `Chuo` ✓ | `Chuuou` ✗ |

Katakana does not record whether ノウ is one long vowel or two syllables, and
neither does any published dataset, so there is no third field that is simply
correct. The two agree on 45.9% of town names and differ on the rest.

**Display `Name.Romaji`. Use `Transliterate` to match romaji somebody typed** —
a search for `marunouchi` or `toukyou` finds nothing in the published
spellings:

```go
for a := range utfkenall.All() {
    index(a, a.Town.Romaji, utfkenall.Transliterate(a.Town.Kana))
}
```

Nothing extra is stored for this: the katakana is already in the record, so the
second spelling is computed on demand and the data file is unchanged.

## Data

| Source | What it gives | Encoding | Published |
|---|---|---|---|
| [`utf_ken_all.zip`](https://www.post.japanpost.jp/zipcode/dl/utf-zip.html) | addresses and readings | UTF-8, one record per line | monthly |
| [`KEN_ALL_ROME.zip`](https://www.post.japanpost.jp/zipcode/dl/roman-zip.html) | romaji | Shift-JIS | occasionally |

Romaji is Japan Post's own spelling, quirks included — see
[Two romaji spellings](#two-romaji-spellings). It is not an outlier: the Digital
Agency's [Address Base Registry](https://dataset.address-br.digital.go.jp/)
agrees with Japan Post on **99.93%** of the 81,278 towns where both publish a
spelling, and writes `Marunochi` in all twelve cities that have a 丸の内. The 56
disagreements are all over whether 町 reads *-cho* or *-machi*.

The embedded database stores Japan Post's prefecture spelling unchanged. At
read time `Prefecture.Romaji` derives the conventional English form (`Tokyo`,
`Gunma`) from `TOKYO TO` and the archaic `GUMMA KEN`. Call
`addr.PublishedPrefectureRomaji()` or
`PublishedPrefectureRomaji("東京都")` to read the source value directly.

The romaji file is revised far less often than the addresses, so it lacks the
newest codes. Those are transliterated from katakana instead and marked with
`Address.RomajiEstimated`. In the current edition that is **0.67%** of named
towns; the transliterator agrees with 99.15% of the spellings Japan Post has
published, so the estimates are close but not authoritative.

`DataUpdated` reports both publication dates at runtime.

## Cost

Measured on an Apple M1, 124,513 records:

| | |
|---|---|
| embedded database | 1.67 MiB |
| added to a stripped binary | 2.3 MiB |
| `Lookup` | 96 ns, 2 allocations |
| `Transliterate` | 313 ns, 4 allocations |
| first lookup, or `Load()` | 54 ms cold, 31 ms warm |
| resident afterwards | 7.2 MiB |

The database is a front-coded, columnar payload inside one DEFLATE stream,
decoded on first use. String IDs are delta encoded and the mostly-empty note
columns are sparse. Every name is a slice of one shared string after loading,
so reading a record allocates nothing beyond the two formatted codes.

## Updating the database

```sh
go generate ./...
```

That runs `tools/gendata`, which downloads both datasets from Japan Post, joins
them, and rewrites `data/kenall.bin`. It prints what it did:

```
  124513 records across 1892 cities
  1909 records have no town name (Japan Post placeholders)
  romaji of 122604 named towns:
    121432  99.04%  official, exact match
       356  0.29%   official, matched via the zip code alone
       816  0.67%   transliterated from kana
  transliterator vs 121432 official spellings: 73.77% identical, 99.15% same sounds
```

`tools/gendata` is a separate module so that its Shift-JIS decoding dependency
stays out of every program that imports the library. `-cache <dir>` keeps the
downloads for a second run, `-v` lists where the transliterator and Japan Post
disagree.

The manually triggered `update-data` workflow rebuilds the database and opens
a pull request when the data has moved.

## Credits

A rewrite of [oirik/gokenall](https://github.com/oirik/gokenall), which parsed
the Shift-JIS `ken_all.csv` into normalised CSV. Japan Post has since published
a UTF-8, one-record-per-line edition, and this package trades the CSV pipeline
for an embedded database and a lookup API.

## License

[Apache 2.0](LICENSE). The postal data is published by Japan Post; see their
[terms of use](https://www.post.japanpost.jp/zipcode/download.html).
