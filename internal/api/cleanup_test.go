package api

import (
	"clonarr/internal/arr"
	"testing"
)

// TestKeepSetFromNames covers the shared helper every cleanup scan +
// handleCleanupApply uses to build their keep-list lookup. Case-
// sensitivity + whitespace handling are part of the contract — the
// sync engine matches CF names case-sensitively (see sync.go
// existingByName), so the keep set must too.
func TestKeepSetFromNames(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want map[string]bool
	}{
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty input returns nil",
			in:   []string{},
			want: nil,
		},
		{
			name: "single entry",
			in:   []string{"FLUX"},
			want: map[string]bool{"FLUX": true},
		},
		{
			name: "trims whitespace per entry",
			in:   []string{"  FLUX  ", "\tSiC\n"},
			want: map[string]bool{"FLUX": true, "SiC": true},
		},
		{
			name: "blank + whitespace-only entries are skipped",
			in:   []string{"FLUX", "", "   ", "\t"},
			want: map[string]bool{"FLUX": true},
		},
		{
			name: "case-sensitive — pcok and PCOK are different keys",
			in:   []string{"PCOK", "pcok"},
			want: map[string]bool{"PCOK": true, "pcok": true},
		},
		{
			name: "duplicates collapse to single entry",
			in:   []string{"FLUX", "FLUX", "FLUX"},
			want: map[string]bool{"FLUX": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := keepSetFromNames(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got=%v want=%v)", len(got), len(tc.want), got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("key %q: got %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

// TestNamingFormatUsesCustomFormats covers the helper used by
// scanUnusedByClonarr to decide whether the scan result should expose
// the "rename-flag is functional" info box on the frontend.
func TestNamingFormatUsesCustomFormats(t *testing.T) {
	cases := []struct {
		name     string
		instType string
		naming   arr.ArrNamingConfig
		want     bool
	}{
		{
			name:     "radarr default-trash format with token",
			instType: "radarr",
			naming: arr.ArrNamingConfig{
				"standardMovieFormat": "{Movie CleanTitle} ({Release Year}) {[Custom Formats]}{[Quality Full]}",
			},
			want: true,
		},
		{
			name:     "sonarr default-trash format with token",
			instType: "sonarr",
			naming: arr.ArrNamingConfig{
				"standardEpisodeFormat": "{Series TitleYear} - S{season:00}E{episode:00} {[Custom Formats]}",
			},
			want: true,
		},
		{
			name:     "radarr stripped format without token",
			instType: "radarr",
			naming: arr.ArrNamingConfig{
				"standardMovieFormat": "{Movie CleanTitle} ({Release Year}) {Quality Full}",
			},
			want: false,
		},
		{
			name:     "sonarr without token",
			instType: "sonarr",
			naming: arr.ArrNamingConfig{
				"standardEpisodeFormat": "{Series Title} - {Episode Title}",
			},
			want: false,
		},
		{
			name:     "missing key returns false",
			instType: "radarr",
			naming:   arr.ArrNamingConfig{},
			want:     false,
		},
		{
			name:     "non-string value returns false",
			instType: "radarr",
			naming: arr.ArrNamingConfig{
				"standardMovieFormat": 12345,
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := namingFormatUsesCustomFormats(tc.naming, tc.instType)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
