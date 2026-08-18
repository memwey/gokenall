package main

import (
	"strings"
	"testing"
	"time"
)

func TestPublicationDate(t *testing.T) {
	var (
		zero     time.Time
		good     = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
		ancient  = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
		future   = time.Now().AddDate(1, 0, 0)
		override = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	)

	tests := []struct {
		name     string
		scraped  time.Time
		override time.Time
		want     time.Time
		wantErr  string
	}{
		{
			name:    "a plausible date is taken as is",
			scraped: good,
			want:    good,
		},
		{
			// The zero time encodes as a valid date in year 1, so it must not
			// reach the database: nothing downstream would flag it.
			name:    "a page that could not be read is fatal",
			scraped: zero,
			wantErr: "no publication date",
		},
		{
			name:    "a date before the format existed is fatal",
			scraped: ancient,
			wantErr: "before Japan Post published it",
		},
		{
			name:    "a date in the future is fatal",
			scraped: future,
			wantErr: "in the future",
		},
		{
			name:     "the override wins",
			scraped:  good,
			override: override,
			want:     override,
		},
		{
			name:     "the override rescues a page that could not be read",
			scraped:  zero,
			override: override,
			want:     override,
		},
		{
			// The override is the escape hatch, so it is not second-guessed.
			name:     "the override is not itself range checked",
			scraped:  zero,
			override: ancient,
			want:     ancient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := publicationDate(tt.scraped, tt.override, kenAllPublishedFrom, "utf_ken_all.zip", "-ken-date")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("publicationDate = %v, want an error mentioning %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
				}
				if !strings.Contains(err.Error(), "-ken-date") {
					t.Errorf("error = %v, want it to name the override flag", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("publicationDate: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("publicationDate = %v, want %v", got, tt.want)
			}
		})
	}
}

// A few days of clock skew between here and Japan Post must not fail a build.
func TestPublicationDateToleratesClockSkew(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1)
	if _, err := publicationDate(tomorrow, time.Time{}, kenAllPublishedFrom, "x", "-ken-date"); err != nil {
		t.Errorf("a date one day ahead was rejected: %v", err)
	}
}

func TestDateFlag(t *testing.T) {
	var got time.Time
	f := dateFlag{&got}
	if err := f.Set("2026-07-31"); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("parsed %v, want %v", got, want)
	}
	if s := f.String(); s != "2026-07-31" {
		t.Errorf("String() = %q", s)
	}
	for _, bad := range []string{"", "31/07/2026", "2026-7-31", "yesterday"} {
		if err := f.Set(bad); err == nil {
			t.Errorf("Set(%q) was accepted", bad)
		}
	}
}
