package main

import (
	"testing"
	"time"
)

// Trimmed from the real page. The links are relative and the page 301s to a
// different path, so the base has to be where the redirect landed, not where
// the request was aimed.
const utfZipPage = `<div class="innerwidth">
	<p class="arrange-r">2026年7月31日更新</p>
	<h3 class="lline">最新データのダウンロード（zip形式）</h3>
	<ul class="basic-m sp-v20">
		<li><a class="inline" href="utf/zip/utf_ken_all.zip">最新データのダウンロード</a>（zip形式：2.04Mバイト）</li>
	</ul>
	<table class="data w100p sp-t20">
		<tr>
			<td class="arrange-r">2026年7月31日更新分</td>
			<td><a class="inline" href="utf/zip/utf_add_2607.zip">utf_add_2607.zip</a></td>
			<td><a class="inline" href="utf/zip/utf_del_2607.zip">utf_del_2607.zip</a></td>
		</tr>
	</table>
</div>`

func TestScrape(t *testing.T) {
	const landed = "https://www.post.japanpost.jp/service/search/zipcode/download/utf-zip.html"
	gotURL, gotDate := scrape([]byte(utfZipPage), landed, "utf_ken_all.zip")

	wantURL := "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip"
	if gotURL != wantURL {
		t.Errorf("zip URL = %q, want %q", gotURL, wantURL)
	}
	wantDate := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	if !gotDate.Equal(wantDate) {
		t.Errorf("date = %v, want %v", gotDate, wantDate)
	}
}

// The monthly diff table repeats the same date with 更新分. Picking one of those
// rows would still give the right answer here, so the guard is worth asserting
// on a page whose own date differs from the rows below it.
func TestScrapePrefersThePageDateOverTheDiffTable(t *testing.T) {
	page := `<p class="arrange-r">2025年6月18日更新</p>
		<td class="arrange-r">2026年7月31日更新分</td>
		<a href="roman/KEN_ALL_ROME.zip">x</a>`
	_, got := scrape([]byte(page), "https://www.post.japanpost.jp/service/search/zipcode/download/roman-zip.html", "KEN_ALL_ROME.zip")
	want := time.Date(2025, 6, 18, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("date = %v, want %v", got, want)
	}
}

func TestScrapeMissingPieces(t *testing.T) {
	t.Run("no matching link", func(t *testing.T) {
		url, _ := scrape([]byte(utfZipPage), "https://example.test/p.html", "nothing_like_this.zip")
		if url != "" {
			t.Errorf("zip URL = %q, want empty so the caller falls back", url)
		}
	})
	t.Run("no date", func(t *testing.T) {
		_, date := scrape([]byte(`<a href="utf/zip/utf_ken_all.zip">x</a>`), "https://example.test/p.html", "utf_ken_all.zip")
		if !date.IsZero() {
			t.Errorf("date = %v, want zero", date)
		}
	})
	t.Run("empty page", func(t *testing.T) {
		url, date := scrape(nil, "https://example.test/p.html", "utf_ken_all.zip")
		if url != "" || !date.IsZero() {
			t.Errorf("scrape(nil) = %q, %v; want empty", url, date)
		}
	})
}

func TestResolvedCacheRoundTrip(t *testing.T) {
	tests := []struct {
		url  string
		date time.Time
	}{
		{"https://example.test/a.zip", time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		{"https://example.test/a.zip", time.Time{}},
	}
	for _, tt := range tests {
		gotURL, gotDate, ok := parseResolved(formatResolved(tt.url, tt.date))
		if !ok {
			t.Fatalf("parseResolved rejected what formatResolved wrote for %v", tt.date)
		}
		if gotURL != tt.url || !gotDate.Equal(tt.date) {
			t.Errorf("round-tripped as %q, %v; want %q, %v", gotURL, gotDate, tt.url, tt.date)
		}
	}
}

func TestParseResolvedRejectsJunk(t *testing.T) {
	for _, s := range []string{"", "\n", "  \n  ", "https://example.test/a.zip\nnot-a-date"} {
		if _, _, ok := parseResolved(s); ok {
			t.Errorf("parseResolved(%q) accepted it", s)
		}
	}
}

func TestUnzipSingleRejectsUnexpectedArchives(t *testing.T) {
	if _, err := unzipSingle([]byte("not a zip file at all")); err == nil {
		t.Error("unzipSingle accepted something that is not a zip")
	}
}
