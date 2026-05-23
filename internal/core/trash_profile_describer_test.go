package core

import (
	"strings"
	"testing"
)

// HD Bluray + WEB markdown section — captured verbatim from
// docs/Radarr/radarr-setup-quality-profiles.md to exercise the parser
// against real-world TRaSH content (tagline, size, Note, Workflow Logic).
const hdBluRayMD = `### HD Bluray + WEB

If you prefer High-Quality HD Encodes (Bluray-720p/1080p)

- _Size: 6-15 GB for a Bluray-1080p depending on the running time._

{! include-markdown "../../includes/cf/radarr-suggest-attention.md" !}

**The following Custom Formats are required:**

{! include-markdown "../../includes/cf/radarr-cf-hd-bluray-web-scoring.md" !}

Note: The ` + "`Audio Formats`" + ` Custom Formats aren't used in the HD Bluray + WEB profile, as HD Bluray Encodes do not often come with HD audio. If you want HD audio, we would suggest going with a Remux or UHD Encode.

Use the following main settings in your profile.

![HD Bluray + WEB](images/qp-bluray-webdl.png)

??? abstract "Workflow Logic - [Click to show/hide]"

    - When the WEB-1080p is released it will download the WEB-1080p. (streaming services)
    - When the Bluray-1080p is released it will upgrade to the Bluray-1080p.
    - The downloaded media will be upgraded to any of the added Custom Formats until a score of ` + "`10000`" + `.

---

### UHD Bluray + WEB

If you prefer High-Quality UHD Encodes (Bluray-2160p)

- _Size: 20-60 GB for a Bluray-2160p depending on the running time._

**The following Custom Formats are required:**

{! include-markdown "../../includes/cf/radarr-hdr-formats.md" !}

??? abstract "Workflow Logic"

    - When the WEB-2160p is released it will download it.
    - When the Bluray-2160p is released it will upgrade.
`

func TestParseProfileMarkdown_TaglineSizeNoteWorkflow(t *testing.T) {
	sections := parseProfileMarkdown(strings.NewReader(hdBluRayMD))
	hd, ok := sections["HD Bluray + WEB"]
	if !ok {
		t.Fatalf("HD Bluray + WEB section not parsed; got keys %v", keysOf(sections))
	}
	wantTagline := "If you prefer High-Quality HD Encodes (Bluray-720p/1080p)"
	if hd.Tagline != wantTagline {
		t.Errorf("tagline = %q, want %q", hd.Tagline, wantTagline)
	}
	wantSize := "6-15 GB for a Bluray-1080p depending on the running time"
	if hd.SizeText != wantSize {
		t.Errorf("size = %q, want %q", hd.SizeText, wantSize)
	}
	if !strings.Contains(hd.Note, "Audio Formats Custom Formats aren't used") {
		t.Errorf("note missing expected substring; got %q", hd.Note)
	}
	if strings.Contains(hd.Note, "`") {
		t.Errorf("note should have backticks stripped; got %q", hd.Note)
	}
}

func TestParseProfileMarkdown_NoNoteProfile(t *testing.T) {
	sections := parseProfileMarkdown(strings.NewReader(hdBluRayMD))
	uhd, ok := sections["UHD Bluray + WEB"]
	if !ok {
		t.Fatalf("UHD section not parsed")
	}
	if uhd.Note != "" {
		t.Errorf("UHD has no Note in markdown; got %q", uhd.Note)
	}
	if uhd.SizeText != "20-60 GB for a Bluray-2160p depending on the running time" {
		t.Errorf("UHD size text mismatch: %q", uhd.SizeText)
	}
}

func TestExtractResolution(t *testing.T) {
	cases := map[string]string{
		"Bluray-1080p":   "1080p",
		"Bluray-2160p":   "2160p",
		"WEB 1080p":      "1080p",
		"WEB 2160p":      "2160p",
		"Remux-1080p":    "1080p",
		"Bluray-720p":    "720p",
		"Raw-HD":         "",
		"Unknown":        "",
		"BR-DISK":        "",
	}
	for in, want := range cases {
		if got := extractResolution(in); got != want {
			t.Errorf("extractResolution(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractSource(t *testing.T) {
	cases := map[string]string{
		"Bluray-1080p":       "Bluray",
		"Bluray-2160p":       "UHD Bluray",
		"Bluray-1080p Remux": "Bluray Remux",
		"Bluray-2160p Remux": "UHD Bluray Remux",
		"Remux-1080p":        "Bluray Remux",
		"Remux-2160p":        "UHD Bluray Remux",
		"WEBDL-1080p":        "WEB-DL",
		"WEBRip-1080p":       "WEBRip",
		"HDTV-1080p":         "HDTV",
		"WEB 1080p":          "", // grouping item — children carry the source
	}
	for in, want := range cases {
		if got := extractSource(in); got != want {
			t.Errorf("extractSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveResolutionWithFallback(t *testing.T) {
	// HD Bluray + WEB items: Bluray-1080p, WEB 1080p (allowed), Bluray-720p allowed
	items := []QualityItem{
		{Name: "Bluray-1080p", Allowed: true},
		{Name: "WEB 1080p", Allowed: true, Items: []string{"WEBDL-1080p"}},
		{Name: "Bluray-720p", Allowed: true},
		{Name: "Bluray-2160p", Allowed: false},
	}
	got := deriveResolution(items)
	want := "1080p (720p fallback)"
	if got != want {
		t.Errorf("deriveResolution = %q, want %q", got, want)
	}
}

func TestDeriveResolutionStrict2160p(t *testing.T) {
	// UHD Bluray + WEB items: only 2160p allowed
	items := []QualityItem{
		{Name: "Bluray-2160p", Allowed: true},
		{Name: "WEB 2160p", Allowed: true, Items: []string{"WEBDL-2160p"}},
		{Name: "Bluray-1080p", Allowed: false},
	}
	got := deriveResolution(items)
	if got != "2160p" {
		t.Errorf("deriveResolution = %q, want '2160p'", got)
	}
}

func TestDeriveCodec(t *testing.T) {
	hdItems := []QualityItem{{Name: "Bluray-1080p", Allowed: true}}
	if c := deriveCodec(hdItems); c != "x264" {
		t.Errorf("HD codec = %q, want x264", c)
	}
	uhdItems := []QualityItem{{Name: "Bluray-2160p", Allowed: true}}
	if c := deriveCodec(uhdItems); c != "x265" {
		t.Errorf("UHD codec = %q, want x265", c)
	}
}

func TestDescribeProfile_AudioScoredFromCFGroupInclude(t *testing.T) {
	// Simulate Remux + WEB 2160p: profile JSON has only tier scoring, but
	// audio-formats cf-group includes the profile with default=true.
	profile := &TrashQualityProfile{
		TrashID: "fd161a61e3ab826d3a22d53f935696dd",
		Name:    "Remux + WEB 2160p",
		Cutoff:  "Remux-2160p",
		Items: []QualityItem{
			{Name: "Remux-2160p", Allowed: true},
			{Name: "WEB 2160p", Allowed: true, Items: []string{"WEBDL-2160p"}},
		},
	}
	audioGroup := &TrashCFGroup{
		Name:    "[Audio] Audio Formats",
		TrashID: "9d5acd8f1da78dfbae788182f7605200",
		Default: "true",
	}
	audioGroup.QualityProfiles.Include = map[string]string{
		"Remux + WEB 2160p": profile.TrashID,
	}
	hdrGroup := &TrashCFGroup{
		Name:    "[HDR Formats] HDR",
		TrashID: "ef20e67b95a381fb3bc6d1f06ea24f46",
		Default: "true",
	}
	hdrGroup.QualityProfiles.Include = map[string]string{
		"Remux + WEB 2160p": profile.TrashID,
	}
	dvBoost := &TrashCFGroup{
		Name:    "[HDR Formats] DV Boost",
		TrashID: "1616617ab3a14397a2b2321bcbda44d1",
		Default: "false",
	}
	dvBoost.QualityProfiles.Include = map[string]string{
		"Remux + WEB 2160p": profile.TrashID,
	}

	desc := describeProfile("radarr", profile,
		[]*TrashCFGroup{audioGroup, hdrGroup, dvBoost},
		ProfileMarkdownSection{Tagline: "For 2160p Remuxes (Remux-2160p)"})

	if !desc.Axes.Audio.Scored {
		t.Error("Audio should be Scored when audio-formats cf-group is default=true for this profile")
	}
	if !desc.Axes.HDR.Scored {
		t.Error("HDR should be Scored when HDR cf-group is default=true")
	}
	if len(desc.Axes.HDR.OptIns) != 1 || desc.Axes.HDR.OptIns[0] != "DV Boost" {
		t.Errorf("HDR OptIns = %v, want [DV Boost]", desc.Axes.HDR.OptIns)
	}
	if desc.Axes.Codec != "x265" {
		t.Errorf("Codec = %q, want x265 for 2160p", desc.Axes.Codec)
	}
}

func TestDescribeProfile_HDProfileNoAudioNoHDR(t *testing.T) {
	// HD Bluray + WEB: profile JSON has tier scoring, no audio or HDR
	// cf-groups have it in include map (audio docs explicitly note this).
	profile := &TrashQualityProfile{
		TrashID: "d1d67249d3890e49bc12e275d989a7e9",
		Name:    "HD Bluray + WEB",
		Cutoff:  "Bluray-1080p",
		Items: []QualityItem{
			{Name: "Bluray-1080p", Allowed: true},
			{Name: "WEB 1080p", Allowed: true, Items: []string{"WEBDL-1080p"}},
			{Name: "Bluray-720p", Allowed: true},
		},
	}
	// Audio cf-group exists but DOESN'T include HD Bluray + WEB
	audioGroup := &TrashCFGroup{
		Name:    "[Audio] Audio Formats",
		TrashID: "9d5acd8f1da78dfbae788182f7605200",
		Default: "true",
	}
	audioGroup.QualityProfiles.Include = map[string]string{
		// Note: HD Bluray + WEB NOT in this include map (matches reality)
		"Remux + WEB 1080p": "other-id",
	}

	desc := describeProfile("radarr", profile,
		[]*TrashCFGroup{audioGroup},
		ProfileMarkdownSection{})

	if desc.Axes.Audio.Scored {
		t.Error("Audio should NOT be scored when audio cf-group doesn't include this profile")
	}
	if desc.Axes.HDR.Scored {
		t.Error("HDR should NOT be scored when no HDR cf-group is present")
	}
	if desc.Axes.Codec != "x264" {
		t.Errorf("Codec = %q, want x264 for HD profile", desc.Axes.Codec)
	}
	if !strings.Contains(desc.Axes.Resolution, "1080p") {
		t.Errorf("Resolution = %q, want 1080p reference", desc.Axes.Resolution)
	}
}

func TestCleanTagline(t *testing.T) {
	cases := map[string]string{
		"If you prefer High-Quality HD Encodes (Bluray-720p/1080p)": "High-Quality HD Encodes (Bluray-720p/1080p)",
		"If you want HD audio":   "HD audio",
		"Streaming 1080p TV":     "Streaming 1080p TV", // no prefix — leave alone
		"":                       "",
	}
	for in, want := range cases {
		if got := cleanTagline(in); got != want {
			t.Errorf("cleanTagline(%q) = %q, want %q", in, got, want)
		}
	}
}

func keysOf(m map[string]ProfileMarkdownSection) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
