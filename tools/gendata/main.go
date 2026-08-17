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

func main() {
	out := flag.String("out", "data/kenall.bin", "path of the data file to write")
	cache := flag.String("cache", "", "directory to keep downloaded archives in, for repeated runs")
	offline := flag.Bool("offline", false, "use only cached archives; fail if any are missing")
	timeout := flag.Duration("timeout", 5*time.Minute, "overall deadline")
	flag.BoolVar(&verbose, "v", false, "list example disagreements between the transliterator and Japan Post")
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

	logf("building")
	d, st, err := build(ken, rome, kenArchive.updated, romeArchive.updated)
	if err != nil {
		return err
	}
	report(st)

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tmp := out + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	encStats, err := binfmt.Encode(file, d)
	if err != nil {
		file.Close()
		os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, out); err != nil {
		return err
	}

	logf("encoding")
	logf("  %d unique strings, %s of text", encStats.UniqueStrings, humanBytes(encStats.BlobBytes))
	logf("  payload %s -> file %s (%.1f%%)", humanBytes(encStats.PayloadBytes), humanBytes(encStats.FileBytes),
		100*float64(encStats.FileBytes)/float64(encStats.PayloadBytes))

	return verify(out, d)
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
	for i := range d.Records {
		got, want := store.At(i), d.Records[i]
		if got.Zip != want.Zip || got.Town != want.Town || got.Note != want.Note || got.Flags != want.Flags {
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

func date(t time.Time) string {
	if t.IsZero() {
		return "date unknown"
	}
	return t.Format("2006-01-02")
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
