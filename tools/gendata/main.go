// Command gendata rebuilds the zip code database that utfkenall embeds.
//
// It downloads the two datasets Japan Post publishes — utf_ken_all.zip for the
// addresses and readings, KEN_ALL_ROME.zip for the official romaji — joins
// them, and writes data/kenall.bin.
//
// It is a maintenance tool, not part of the library: it lives in its own module
// so that its Shift-JIS decoding dependency stays out of every program that
// imports utfkenall.
//
//	go generate ./...                 # from the repository root
//	go run ./tools/gendata -cache tmp # keep the downloads for a second run
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/memwey/utfkenall/internal/binfmt"
)

// verbose is set by -v and only affects the summary written to stderr.
var verbose bool

// dates holds the -ken-date/-rome-date overrides, for when the download pages
// change shape and the scraper can no longer find them.
var dates struct{ kenAll, rome time.Time }

func main() {
	out := flag.String("out", "data/kenall.bin", "path of the data file to write")
	cache := flag.String("cache", "", "directory to keep downloaded archives in, for repeated runs")
	offline := flag.Bool("offline", false, "use only cached archives; fail if any are missing")
	timeout := flag.Duration("timeout", 5*time.Minute, "overall deadline")
	flag.BoolVar(&verbose, "v", false, "list example disagreements between the transliterator and Japan Post")
	flag.Var(dateFlag{&dates.kenAll}, "ken-date", "publication date of utf_ken_all.zip as YYYY-MM-DD, if the page cannot be read")
	flag.Var(dateFlag{&dates.rome}, "rome-date", "publication date of KEN_ALL_ROME.zip as YYYY-MM-DD, if the page cannot be read")
	flag.Parse()

	if err := run(*out, *cache, *offline, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "gendata: %v\n", err)
		os.Exit(1)
	}
}

func run(out, cache string, offline bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	f := &fetcher{
		client:   &http.Client{Timeout: timeout},
		cacheDir: cache,
		offline:  offline,
	}

	logf("downloading")
	kenArchive, err := f.get(ctx, kenAllSource)
	if err != nil {
		return fmt.Errorf("ken_all: %w", err)
	}
	romeArchive, err := f.get(ctx, romeSource)
	if err != nil {
		return fmt.Errorf("rome: %w", err)
	}

	logf("parsing")
	ken, err := parseKenAll(kenArchive.csv)
	if err != nil {
		return err
	}
	rome, err := parseRome(romeArchive.csv)
	if err != nil {
		return err
	}
	logf("  utf_ken_all.csv  %6d rows  (published %s)", len(ken), date(kenArchive.updated))
	logf("  KEN_ALL_ROME.CSV %6d rows  (published %s)", len(rome), date(romeArchive.updated))

	kenDate, err := publicationDate(kenArchive.updated, dates.kenAll, kenAllPublishedFrom, "utf_ken_all.zip", "-ken-date")
	if err != nil {
		return err
	}
	romeDate, err := publicationDate(romeArchive.updated, dates.rome, romePublishedFrom, "KEN_ALL_ROME.zip", "-rome-date")
	if err != nil {
		return err
	}

	logf("building")
	d, st, err := build(ken, rome, kenDate, romeDate)
	if err != nil {
		return err
	}
	report(st)

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// Write and check a temporary file, and only then move it into place. The
	// database already at `out` is the last one known to work, so it must not
	// be replaced by something that has yet to prove it decodes.
	tmp := out + ".tmp"
	encStats, err := write(tmp, d)
	if err != nil {
		os.Remove(tmp)
		return err
	}

	logf("encoding")
	logf("  %d unique strings, %s of text", encStats.UniqueStrings, humanBytes(encStats.BlobBytes))
	logf("  payload %s -> file %s (%.1f%%)", humanBytes(encStats.PayloadBytes), humanBytes(encStats.FileBytes),
		100*float64(encStats.FileBytes)/float64(encStats.PayloadBytes))

	if err := verify(tmp, d); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}

func write(path string, d *binfmt.Dataset) (binfmt.Stats, error) {
	file, err := os.Create(path)
	if err != nil {
		return binfmt.Stats{}, err
	}
	stats, err := binfmt.Encode(file, d)
	if err != nil {
		file.Close()
		return binfmt.Stats{}, err
	}
	return stats, file.Close()
}

// Japan Post began publishing the UTF-8 edition in June 2023; the romaji file
// goes back much further. A date outside these bounds means the page was not
// read correctly, not that the data is old.
var (
	kenAllPublishedFrom = time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	romePublishedFrom   = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
)

// publicationDate settles which date to stamp into the database.
//
// A missing one is fatal rather than merely logged: the zero time encodes as a
// perfectly valid date in year 1, so a page that failed to load would otherwise
// leave a database that looks fine and claims to be two millennia old. The
// override flag is the way through when Japan Post redesigns the page again.
func publicationDate(scraped, override time.Time, earliest time.Time, what, flag string) (time.Time, error) {
	if !override.IsZero() {
		if !scraped.IsZero() && !scraped.Equal(override) {
			logf("  ! %s: using %s from %s, not the %s on the page", what, date(override), flag, date(scraped))
		}
		return override, nil
	}
	switch {
	case scraped.IsZero():
		return time.Time{}, fmt.Errorf("no publication date found for %s; the download page may have changed shape — pass %s YYYY-MM-DD to override", what, flag)
	case scraped.Before(earliest):
		return time.Time{}, fmt.Errorf("%s is dated %s, before Japan Post published it at all; pass %s YYYY-MM-DD to override", what, date(scraped), flag)
	case scraped.After(time.Now().AddDate(0, 0, 7)):
		return time.Time{}, fmt.Errorf("%s is dated %s, which is in the future; pass %s YYYY-MM-DD to override", what, date(scraped), flag)
	}
	return scraped, nil
}

// verify reads the file back through the decoder, so a run that reports success
// has actually produced something the library can load.
func verify(path string, d *binfmt.Dataset) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	start := time.Now()
	store, err := binfmt.Decode(file)
	if err != nil {
		return fmt.Errorf("verify %s: %w", path, err)
	}
	elapsed := time.Since(start)

	if store.Len() != len(d.Records) {
		return fmt.Errorf("verify %s: decoded %d records, encoded %d", path, store.Len(), len(d.Records))
	}
	for i, pref := range d.Prefectures {
		got, ok := store.PublishedPrefectureRomaji(pref.Kanji)
		if !ok || got != d.PrefectureSourceRomaji[i] {
			return fmt.Errorf("verify %s: prefecture %q source romaji is %q, %v; encoded %q", path, pref.Kanji, got, ok, d.PrefectureSourceRomaji[i])
		}
	}
	for i := range d.Records {
		got, want := store.At(i), d.Records[i]
		city := d.Cities[want.City]
		if got.Zip != want.Zip || got.Town != want.Town || got.Note != want.Note ||
			got.NoteKana != want.NoteKana || got.Flags != want.Flags ||
			got.JIS != city.JIS || got.City != city.Name || got.Prefecture != d.Prefectures[city.Pref] {
			return fmt.Errorf("verify %s: record %d round-tripped as %+v, encoded %+v", path, i, got, want)
		}
	}

	logf("verified: %d records reload in %s", store.Len(), elapsed.Round(time.Millisecond))
	return nil
}

func report(st buildStats) {
	pct := func(n int) string {
		if st.named() == 0 {
			return "0%"
		}
		return fmt.Sprintf("%.2f%%", 100*float64(n)/float64(st.named()))
	}
	logf("  %d records across %d cities", st.Rows, st.Cities)
	logf("  %d records have no town name (Japan Post placeholders)", st.EmptyTown)
	logf("  romaji of %d named towns:", st.named())
	logf("    %6d  %-7s official, exact match", st.RomajiExact, pct(st.RomajiExact))
	logf("    %6d  %-7s official, matched via the zip code alone", st.RomajiByZip, pct(st.RomajiByZip))
	logf("    %6d  %-7s transliterated from kana", st.RomajiEstimate, pct(st.RomajiEstimate))
	if st.CityEstimate > 0 {
		logf("  ! %d cities had no official romaji and were transliterated", st.CityEstimate)
	}
	if st.HepburnChecked > 0 {
		n := float64(st.HepburnChecked)
		logf("  transliterator vs %d official spellings: %.2f%% identical, %.2f%% same sounds",
			st.HepburnChecked, 100*float64(st.HepburnAgreed)/n, 100*float64(st.HepburnSounded)/n)
		if verbose {
			for _, m := range st.HepburnMismatch {
				logf("    %s", m)
			}
		}
	}
}

// dateFlag parses a YYYY-MM-DD command line date.
type dateFlag struct{ t *time.Time }

func (f dateFlag) String() string {
	if f.t == nil || f.t.IsZero() {
		return ""
	}
	return f.t.Format(dateLayout)
}

func (f dateFlag) Set(s string) error {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("want YYYY-MM-DD: %w", err)
	}
	*f.t = t
	return nil
}

const dateLayout = "2006-01-02"

func date(t time.Time) string {
	if t.IsZero() {
		return "date unknown"
	}
	return t.Format(dateLayout)
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
