package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// source describes one Japan Post download page.
type source struct {
	// page is the advertised URL. It currently 301s from /zipcode/dl/ to
	// /service/search/zipcode/download/, and the zip links on the page are
	// relative, so they must be resolved against the URL we actually landed on
	// — resolving against the advertised URL yields a 404.
	page string
	// zipName is the file to find among the page's links.
	zipName string
	// fallbackZip is used when the page cannot be scraped. It is a snapshot of
	// where the file lived when this tool was written, not a preferred path.
	fallbackZip string
}

var (
	kenAllSource = source{
		page:        "https://www.post.japanpost.jp/zipcode/dl/utf-zip.html",
		zipName:     "utf_ken_all.zip",
		fallbackZip: "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip",
	}
	romeSource = source{
		page:        "https://www.post.japanpost.jp/zipcode/dl/roman-zip.html",
		zipName:     "KEN_ALL_ROME.zip",
		fallbackZip: "https://www.post.japanpost.jp/service/search/zipcode/download/roman/KEN_ALL_ROME.zip",
	}
)

// The page's own revision date, rendered as `<p class="arrange-r">2026年7月31日更新</p>`.
// The monthly diff table below it says 更新分, so requiring the tag right after
// 更新 keeps us off those rows.
var updatedRe = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日更新\s*<`)

var hrefRe = regexp.MustCompile(`href="([^"]+\.zip)"`)

// fetched is a downloaded dataset plus the date its page advertised.
type fetched struct {
	csv     []byte
	updated time.Time
}

type fetcher struct {
	client   *http.Client
	cacheDir string
	offline  bool
}

func (f *fetcher) get(ctx context.Context, src source) (fetched, error) {
	zipURL, updated, err := f.resolve(ctx, src)
	if err != nil {
		return fetched{}, err
	}

	archive, err := f.body(ctx, zipURL, src.zipName)
	if err != nil {
		return fetched{}, err
	}
	csv, err := unzipSingle(archive)
	if err != nil {
		return fetched{}, fmt.Errorf("%s: %w", src.zipName, err)
	}
	return fetched{csv: csv, updated: updated}, nil
}

// resolve scrapes the download page for the zip link and the revision date,
// falling back to the hardcoded URL if the page layout has moved on.
//
// The result is cached rather than the page itself: the links on the page are
// relative, so a cached page would need the URL it was served from to be usable
// at all, and that is exactly what resolving produces.
func (f *fetcher) resolve(ctx context.Context, src source) (zipURL string, updated time.Time, err error) {
	if b, ok := f.fromCache(src.zipName + ".resolved"); ok {
		if u, t, ok := parseResolved(string(b)); ok {
			return u, t, nil
		}
	}
	if f.offline {
		return "", time.Time{}, fmt.Errorf("offline and %s.resolved is not cached", src.zipName)
	}

	page, finalURL, err := f.page(ctx, src)
	if err != nil {
		logf("  ! %s unreadable (%v); using the hardcoded URL", src.page, err)
		return src.fallbackZip, time.Time{}, nil
	}

	zipURL, updated = scrape(page, finalURL, src.zipName)
	if zipURL == "" {
		logf("  ! no link to %s on %s; using the hardcoded URL", src.zipName, finalURL)
		zipURL = src.fallbackZip
	}
	if updated.IsZero() {
		logf("  ! no revision date found on %s", finalURL)
	}

	f.toCache(src.zipName+".resolved", []byte(formatResolved(zipURL, updated)))
	return zipURL, updated, nil
}

// scrape pulls the download link and the revision date out of a download page.
// Missing pieces come back zero rather than as an error, because either one is
// survivable on its own.
func scrape(page []byte, finalURL, zipName string) (zipURL string, updated time.Time) {
	if base, err := url.Parse(finalURL); err == nil {
		for _, m := range hrefRe.FindAllSubmatch(page, -1) {
			ref, err := url.Parse(string(m[1]))
			if err != nil {
				continue
			}
			abs := base.ResolveReference(ref)
			if path.Base(abs.Path) == zipName {
				zipURL = abs.String()
				break
			}
		}
	}

	if m := updatedRe.FindSubmatch(page); m != nil {
		y, _ := strconv.Atoi(string(m[1]))
		mo, _ := strconv.Atoi(string(m[2]))
		d, _ := strconv.Atoi(string(m[3]))
		updated = time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	}
	return zipURL, updated
}

// The resolution cache is one line of URL and one of date, so that a stale
// entry can be read — or deleted — without a tool.
const resolvedDateLayout = "2006-01-02"

func formatResolved(zipURL string, updated time.Time) string {
	date := ""
	if !updated.IsZero() {
		date = updated.Format(resolvedDateLayout)
	}
	return zipURL + "\n" + date + "\n"
}

func parseResolved(s string) (zipURL string, updated time.Time, ok bool) {
	lines := strings.SplitN(strings.TrimSpace(s), "\n", 2)
	if len(lines) == 0 || lines[0] == "" {
		return "", time.Time{}, false
	}
	if len(lines) == 2 && lines[1] != "" {
		t, err := time.Parse(resolvedDateLayout, strings.TrimSpace(lines[1]))
		if err != nil {
			return "", time.Time{}, false
		}
		updated = t
	}
	return lines[0], updated, true
}

func (f *fetcher) page(ctx context.Context, src source) (body []byte, finalURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.page, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: %s", src.page, resp.Status)
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	// resp.Request.URL is where we ended up after redirects, which is the base
	// the page's relative links resolve against.
	return body, resp.Request.URL.String(), nil
}

func (f *fetcher) body(ctx context.Context, rawURL, cacheName string) ([]byte, error) {
	if b, ok := f.fromCache(cacheName); ok {
		logf("  %s (cached, %s)", cacheName, humanBytes(len(b)))
		return b, nil
	}
	if f.offline {
		return nil, fmt.Errorf("offline and %s is not cached", cacheName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	logf("  %s (%s from %s)", cacheName, humanBytes(len(b)), rawURL)
	f.toCache(cacheName, b)
	return b, nil
}

func (f *fetcher) fromCache(name string) ([]byte, bool) {
	if f.cacheDir == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(f.cacheDir, name))
	return b, err == nil
}

func (f *fetcher) toCache(name string, b []byte) {
	if f.cacheDir == "" {
		return
	}
	if err := os.MkdirAll(f.cacheDir, 0o755); err != nil {
		logf("  ! cache %s: %v", name, err)
		return
	}
	if err := os.WriteFile(filepath.Join(f.cacheDir, name), b, 0o644); err != nil {
		logf("  ! cache %s: %v", name, err)
	}
}

// unzipSingle returns the contents of the one file in the archive. Both Japan
// Post archives hold exactly one CSV; more than one means the format changed.
func unzipSingle(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	if len(zr.File) != 1 {
		names := make([]string, len(zr.File))
		for i, f := range zr.File {
			names[i] = f.Name
		}
		return nil, fmt.Errorf("expected 1 file in the archive, got %d: %v", len(zr.File), names)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", zr.File[0].Name, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
