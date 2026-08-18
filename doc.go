// Package utfkenall resolves Japanese postal codes to place names, offline.
//
// Japan Post's zip code database ships inside the package as a compact binary
// blob, so a lookup needs no network, no data files alongside the binary, and
// no setup:
//
//	addr, err := utfkenall.Lookup("100-0001")
//	// addr.Prefecture.Kanji  東京都
//	// addr.City.Kana         チヨダク
//	// addr.Town.Romaji       Chiyoda
//
// Every place name comes in three writings — kanji, full-width katakana, and
// romaji — held together by [Name].
//
// # Two romaji spellings
//
// [Name.Romaji] normally derives from Japan Post's spelling, which leaves long
// vowels unmarked. That is the ordinary English convention (東京 is Tokyo, not
// Tōkyō), but applied mechanically it misreads a vowel pair that spans a word
// boundary: 丸の内 is maru-no-uchi, and they publish Marunochi. The database
// stores Japan Post's prefecture spelling unchanged, then the public address
// derives a conventional English name for display; use
// [Address.PublishedPrefectureRomaji] or [PublishedPrefectureRomaji] to read
// the stored source spelling.
//
// [Transliterate] gives the other reading of the same katakana, spelling out
// every kana — Marunouchi, but also Toukyou. Neither convention is right in
// every case, because katakana does not record which is meant. Display
// Name.Romaji; use Transliterate to match romaji somebody typed.
//
// # Nothing is discarded
//
// Japan Post files some codes under a municipality rather than a town, with
// prose where the name would go, and annotates some town names in parentheses.
// This package moves both out of [Address.Town] so that names compare and
// display cleanly — but it keeps them, in [Address.Note], and
// [Address.RawTown] puts the original columns back together. The generator
// checks that round trip on every record of every build.
//
// # What is not here
//
// Japan Post issues a second kind of code to organisations that receive enough
// mail to warrant one — 事業所個別郵便番号, about 22,000 of them, published as a
// separate dataset. 100-8111 is the Imperial Household Agency and 163-8001 is
// the Tokyo Metropolitan Government; neither is in this package, and looking
// one up gives [ErrNotFound]. Only the 住所の郵便番号 that name a place are here.
//
// # Data
//
// Addresses and readings come from Japan Post's 郵便番号データ (1レコード1行、
// UTF-8形式), romaji from their 郵便番号データ（ローマ字）. The two are published on
// different schedules and the romaji file lags, so a small fraction of towns
// carry a transliterated spelling instead of an official one; those are marked
// with [Address.RomajiEstimated]. [DataUpdated] reports the publication date of
// each source. Run `go generate ./...` to rebuild the embedded database.
//
// The database is decoded lazily on first use, which takes a few tens of
// milliseconds and a few megabytes. Call [Load] to do it at a moment of your
// choosing instead.
package utfkenall
