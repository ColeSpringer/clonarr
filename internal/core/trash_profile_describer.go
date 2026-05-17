package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ProfileDescription is the full auto-derived description of a TRaSH quality
// profile, computed from three data sources without any per-profile hand-curation:
//
//  1. Profile JSON (quality-profiles/X.json) — items[], cutoff, formatItems
//  2. CF-group JSONs (cf-groups/*.json) — quality_profiles.include maps,
//     default flag tells built-in vs opt-in
//  3. Profile markdown section (docs/<App>/<app>-setup-quality-profiles.md) —
//     tagline ("If you prefer ..."), size ("_Size: X-Y GB..._"), optional
//     Note, optional Workflow Logic block
//
// New profiles TRaSH adds get described automatically on the next pull.
type ProfileDescription struct {
	TrashID       string       `json:"trashId"`
	Name          string       `json:"name"`
	App           string       `json:"app"` // "radarr" | "sonarr"
	Tagline       string       `json:"tagline,omitempty"`
	Axes          ProfileAxes  `json:"axes"`
	BuiltInGroups []CFGroupRef `json:"builtInGroups"`
	OptInGroups   []CFGroupRef `json:"optInGroups"`
	TrashNote     string       `json:"trashNote,omitempty"`
	WorkflowSteps []string     `json:"workflowSteps,omitempty"`
	TrashURL      string       `json:"trashUrl,omitempty"`
}

// ProfileAxes is the 7-row quick-fact summary shown as pills on the profile card.
type ProfileAxes struct {
	Resolution string              `json:"resolution"`
	Sources    []string            `json:"sources"`
	Codec      string              `json:"codec"`
	HDR        ProfileHDRSummary   `json:"hdr"`
	Audio      ProfileAudioSummary `json:"audio"`
	Cutoff     string              `json:"cutoff"`
	AvgSize    string              `json:"avgSize,omitempty"`
}

// ProfileHDRSummary reports whether HDR is scored by default and what HDR
// variant add-ons (DV Boost, HDR10+ Boost, etc.) are available as opt-ins.
type ProfileHDRSummary struct {
	Scored bool     `json:"scored"`
	OptIns []string `json:"optIns,omitempty"` // e.g. ["DV Boost", "HDR10+ Boost"]
}

// ProfileAudioSummary reports whether lossless-audio scoring is enabled by
// default (i.e. [Audio] Audio Formats cf-group has default=true for the profile).
type ProfileAudioSummary struct {
	Scored bool `json:"scored"`
}

// CFGroupRef is a lightweight reference to a TRaSH cf-group that includes the
// profile in its quality_profiles.include map. Used for built-in and opt-in
// lists shown in the profile card's collapsible.
type CFGroupRef struct {
	Name    string `json:"name"`
	TrashID string `json:"trashId"`
}

// ProfileMarkdownSection is the parsed per-profile section of TRaSH's
// docs/<App>/<app>-setup-quality-profiles.md file. All fields are optional —
// not every profile has a Note or a Workflow Logic block.
type ProfileMarkdownSection struct {
	Tagline       string
	SizeText      string
	Note          string
	WorkflowSteps []string
}

// DescribeProfiles returns ProfileDescription for every profile in the app's
// loaded TRaSH data. Combines profile JSONs, cf-groups, and the per-app
// setup-quality-profiles.md sections. Safe to call before data is loaded —
// returns empty slice if no profiles are available yet.
func (ts *TrashStore) DescribeProfiles(app string) ([]ProfileDescription, error) {
	snap := ts.Snapshot()
	if snap == nil {
		return nil, nil
	}
	var appData *AppData
	switch app {
	case "radarr":
		appData = &snap.Radarr
	case "sonarr":
		appData = &snap.Sonarr
	default:
		return nil, fmt.Errorf("unknown app: %s", app)
	}
	if len(appData.Profiles) == 0 {
		return nil, nil
	}
	mdSections, err := LoadProfileMarkdown(ts.dataDir, app)
	if err != nil {
		// Markdown failure is non-fatal — describe what we can from JSON
		mdSections = map[string]ProfileMarkdownSection{}
	}
	out := make([]ProfileDescription, 0, len(appData.Profiles))
	for _, p := range appData.Profiles {
		out = append(out, describeProfile(app, p, appData.CFGroups, mdSections[p.Name]))
	}
	return out, nil
}

// DescribeProfile returns the description for a single profile by trash_id.
// Returns nil + nil error if the profile isn't found (caller should 404).
func (ts *TrashStore) DescribeProfile(app, trashID string) (*ProfileDescription, error) {
	all, err := ts.DescribeProfiles(app)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].TrashID == trashID {
			return &all[i], nil
		}
	}
	return nil, nil
}

// LoadProfileMarkdown reads the per-app setup-quality-profiles.md once and
// returns a map keyed by profile name (the ### header text). Returns an empty
// map if the markdown file is missing (sparse-checkout not yet expanded on
// an existing clone, or pre-pull state). Callers MUST tolerate empty results.
func LoadProfileMarkdown(dataDir, app string) (map[string]ProfileMarkdownSection, error) {
	var path string
	switch app {
	case "radarr":
		path = filepath.Join(dataDir, "docs", "Radarr", "radarr-setup-quality-profiles.md")
	case "sonarr":
		path = filepath.Join(dataDir, "docs", "Sonarr", "sonarr-setup-quality-profiles.md")
	default:
		return nil, fmt.Errorf("unknown app: %s", app)
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// sparse-checkout hasn't fetched the file yet — caller renders
			// just the JSON-derived data and skips tagline/note/workflow
			return map[string]ProfileMarkdownSection{}, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return parseProfileMarkdown(f), nil
}

// parseProfileMarkdown extracts per-profile sections from the
// docs/<App>/<app>-setup-quality-profiles.md content.
//
// Section boundaries: ### <profile name> ... (next ### or ## or end of file)
// Within a section:
//   - Tagline = first non-blank prose line after the header (typically
//     "If you prefer ...")
//   - SizeText = the italic _Size: ..._ line, with surrounding markers stripped
//   - Note = lines starting with "Note:" (single-line; if TRaSH adds
//     multi-line notes later, this captures only the first line — defensible
//     since all current notes are single-line)
//   - WorkflowSteps = bullet items inside the ??? abstract "Workflow Logic"
//     block, until that block's blank-line terminator
func parseProfileMarkdown(r interface{ Read([]byte) (int, error) }) map[string]ProfileMarkdownSection {
	sections := map[string]ProfileMarkdownSection{}
	scanner := bufio.NewScanner(r)
	// markdown lines can be long when they have include-markdown directives;
	// bump the buffer to be safe.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		currentProfile  string
		inWorkflowBlock bool
		workflowIndent  int
		taglineSeen     bool
	)
	sizeRe := regexp.MustCompile(`^[-*]\s*_Size:\s*(.+?)\.?_\s*$`)
	workflowStartRe := regexp.MustCompile(`^(\s*)\?\?\?\s+abstract\s+"Workflow Logic`)
	workflowBulletRe := regexp.MustCompile(`^\s*-\s+(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()
		// New profile section (### Header)
		if strings.HasPrefix(line, "### ") {
			currentProfile = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			sections[currentProfile] = ProfileMarkdownSection{}
			inWorkflowBlock = false
			taglineSeen = false
			continue
		}
		// New top-level section ends the current profile context
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			currentProfile = ""
			inWorkflowBlock = false
			continue
		}
		if currentProfile == "" {
			continue
		}
		sec := sections[currentProfile]

		// Workflow block — capture bullet items inside the ??? abstract block
		if !inWorkflowBlock {
			if m := workflowStartRe.FindStringSubmatch(line); m != nil {
				inWorkflowBlock = true
				workflowIndent = len(m[1])
				continue
			}
		} else {
			// Block ends when we hit a line at <= the opening indent that
			// isn't blank and isn't a continuation bullet/text
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// blank line — could continue (extended ??? block)
				continue
			}
			lineIndent := len(line) - len(strings.TrimLeft(line, " "))
			if lineIndent <= workflowIndent {
				// dedented back to or above the ??? line — block ended
				inWorkflowBlock = false
				// fall through to normal handling
			} else {
				if bm := workflowBulletRe.FindStringSubmatch(line); bm != nil {
					step := strings.TrimSpace(bm[1])
					// Replace markdown backticks with plain text
					step = strings.ReplaceAll(step, "`", "")
					sec.WorkflowSteps = append(sec.WorkflowSteps, step)
					sections[currentProfile] = sec
				}
				continue
			}
		}

		// Tagline — first non-blank prose line after header
		trimmed := strings.TrimSpace(line)
		if !taglineSeen && trimmed != "" && !strings.HasPrefix(trimmed, "{!") &&
			!strings.HasPrefix(trimmed, "**") && !strings.HasPrefix(trimmed, "-") &&
			!strings.HasPrefix(trimmed, "!!!") && !strings.HasPrefix(trimmed, "?") {
			sec.Tagline = trimmed
			taglineSeen = true
			sections[currentProfile] = sec
			continue
		}

		// Size — italic bullet "_Size: X-Y GB..._"
		if m := sizeRe.FindStringSubmatch(line); m != nil {
			sec.SizeText = strings.TrimSpace(m[1])
			sections[currentProfile] = sec
			continue
		}

		// Note — line starting with "Note:"
		if strings.HasPrefix(trimmed, "Note:") {
			note := strings.TrimSpace(strings.TrimPrefix(trimmed, "Note:"))
			// Strip markdown backticks for cleaner display
			note = strings.ReplaceAll(note, "`", "")
			sec.Note = note
			sections[currentProfile] = sec
			continue
		}
	}
	return sections
}

// describeProfile builds a complete ProfileDescription for one profile by
// combining its JSON, the cf-groups that include it, and (optionally) the
// matching markdown section.
//
// Falls back to JSON-only data when markdown is missing or the section is
// absent — UI gets empty Tagline/Note/WorkflowSteps and still renders the axes.
func describeProfile(
	app string,
	profile *TrashQualityProfile,
	allGroups []*TrashCFGroup,
	mdSec ProfileMarkdownSection,
) ProfileDescription {
	out := ProfileDescription{
		TrashID:  profile.TrashID,
		Name:     profile.Name,
		App:      app,
		Tagline:  cleanTagline(mdSec.Tagline),
		TrashURL: profile.TrashURL,
		Axes: ProfileAxes{
			Resolution: deriveResolution(profile.Items),
			Sources:    deriveSources(profile.Items),
			Codec:      deriveCodec(profile.Items),
			Cutoff:     profile.Cutoff,
			AvgSize:    mdSec.SizeText,
		},
		TrashNote:     mdSec.Note,
		WorkflowSteps: mdSec.WorkflowSteps,
	}

	// Walk cf-groups: any group whose quality_profiles.include map contains
	// this profile's name is associated with the profile. Split by group
	// default=true (built-in when synced) vs default=false (opt-in available).
	var builtIn, optIn []CFGroupRef
	var hdrOptIns []string
	for _, g := range allGroups {
		if _, ok := g.QualityProfiles.Include[profile.Name]; !ok {
			continue
		}
		ref := CFGroupRef{Name: g.Name, TrashID: g.TrashID}
		if strings.ToLower(g.Default) == "true" {
			builtIn = append(builtIn, ref)
			// Audio detection — Audio Formats group included with default=true
			if g.TrashID == audioFormatsTrashID(app) {
				out.Axes.Audio.Scored = true
			}
			// HDR detection — primary HDR Formats group
			if g.TrashID == hdrFormatsTrashID(app) {
				out.Axes.HDR.Scored = true
			}
		} else {
			optIn = append(optIn, ref)
			// HDR variant opt-ins — flag them on the axes so the pill can
			// show "HDR · DV/HDR10+ opt-in" without consulting the cf-group
			// list separately
			if isHDRVariantGroup(g.Name) {
				short := shortHDRVariantName(g.Name)
				if short != "" {
					hdrOptIns = append(hdrOptIns, short)
				}
			}
		}
	}
	sort.Slice(builtIn, func(i, j int) bool { return builtIn[i].Name < builtIn[j].Name })
	sort.Slice(optIn, func(i, j int) bool { return optIn[i].Name < optIn[j].Name })
	sort.Strings(hdrOptIns)
	out.BuiltInGroups = builtIn
	out.OptInGroups = optIn
	out.Axes.HDR.OptIns = hdrOptIns
	return out
}

// --- Derivation helpers (JSON → axes) ---

// deriveResolution returns a human-readable "1080p (720p Bluray fallback)" style
// summary from the allowed quality items in the profile.
func deriveResolution(items []QualityItem) string {
	// Walk allowed items in order; the first one is the cutoff target. We
	// classify by resolution (extracted from names like "Bluray-1080p",
	// "WEB 2160p"). Items appear top-down by preference, so the first
	// resolution mentioned is the target; lower resolutions are fallbacks.
	resOrder := []string{}
	seen := map[string]bool{}
	for _, it := range items {
		if !it.Allowed {
			continue
		}
		res := extractResolution(it.Name)
		if res == "" {
			continue
		}
		if !seen[res] {
			seen[res] = true
			resOrder = append(resOrder, res)
		}
	}
	if len(resOrder) == 0 {
		return "unknown"
	}
	if len(resOrder) == 1 {
		return resOrder[0]
	}
	// Multiple resolutions — main + fallback list
	return resOrder[0] + " (" + strings.Join(resOrder[1:], ", ") + " fallback)"
}

// deriveSources returns the deduplicated list of release types accepted by the
// profile (Bluray, Bluray Remux, WEB-DL, WEBRip, etc.).
func deriveSources(items []QualityItem) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, it := range items {
		if !it.Allowed {
			continue
		}
		// Flatten nested items (WEB 1080p contains WEBRip-1080p + WEBDL-1080p)
		names := []string{it.Name}
		names = append(names, it.Items...)
		for _, n := range names {
			src := extractSource(n)
			if src == "" || seen[src] {
				continue
			}
			seen[src] = true
			out = append(out, src)
		}
	}
	return out
}

// deriveCodec returns the dominant codec for the resolution range of the profile.
// HD = x264 (industry standard for Bluray/WEB-DL HD). UHD = x265/HEVC (industry
// standard for 4K Bluray/WEB-DL). Mixed profiles default to the highest-resolution
// codec since that's what the cutoff drives toward.
func deriveCodec(items []QualityItem) string {
	hasUHD := false
	for _, it := range items {
		if !it.Allowed {
			continue
		}
		if strings.Contains(it.Name, "2160p") || strings.Contains(strings.ToLower(it.Name), "uhd") {
			hasUHD = true
			break
		}
	}
	if hasUHD {
		return "x265"
	}
	return "x264"
}

// extractResolution pulls a normalized resolution token from a quality name.
// "Bluray-1080p" → "1080p"; "WEB 2160p" → "2160p"; "Remux-1080p" → "1080p".
// Returns "" for non-resolution items (Raw-HD, Unknown, etc.).
func extractResolution(name string) string {
	for _, res := range []string{"2160p", "1080p", "720p", "576p", "480p"} {
		if strings.Contains(name, res) {
			return res
		}
	}
	return ""
}

// extractSource normalizes a quality-item or sub-item name to a canonical
// source label. "Bluray-1080p" → "Bluray"; "WEBDL-1080p" → "WEB-DL";
// "WEBRip-1080p" → "WEBRip"; "Remux-2160p" → "UHD Bluray Remux"; etc.
// Returns "" for quality-grouping items (e.g. "WEB 1080p" itself, since its
// children carry the actual source).
func extractSource(name string) string {
	switch {
	case strings.Contains(name, "Bluray-2160p Remux"), strings.HasPrefix(name, "Remux-2160p"):
		return "UHD Bluray Remux"
	case strings.Contains(name, "Bluray-1080p Remux"), strings.HasPrefix(name, "Remux-1080p"):
		return "Bluray Remux"
	case strings.HasPrefix(name, "Bluray-2160p"):
		return "UHD Bluray"
	case strings.HasPrefix(name, "Bluray-"):
		return "Bluray"
	case strings.HasPrefix(name, "WEBDL-"):
		return "WEB-DL"
	case strings.HasPrefix(name, "WEBRip-"):
		return "WEBRip"
	case strings.HasPrefix(name, "HDTV-"):
		return "HDTV"
	case strings.HasPrefix(name, "DVD"):
		return "DVD"
	}
	return ""
}

// cleanTagline strips redundant prefixes from TRaSH's markdown tagline.
// Most start with "If you prefer " which adds no info on a profile card —
// the user is on the profile's card, of course they're considering it.
func cleanTagline(raw string) string {
	if raw == "" {
		return ""
	}
	for _, prefix := range []string{
		"If you prefer ",
		"If you want ",
	} {
		if strings.HasPrefix(raw, prefix) {
			rest := strings.TrimPrefix(raw, prefix)
			// Capitalize first letter, keep rest verbatim
			if rest == "" {
				return raw
			}
			return strings.ToUpper(rest[:1]) + rest[1:]
		}
	}
	return raw
}

// --- HDR / Audio cf-group ID lookup ---

// audioFormatsTrashID returns the trash_id of the primary Audio Formats
// cf-group for the given app. Used by describeProfile to flag the Audio axis
// as "scored" when this group is included with default=true. IDs are stable
// (set at CF-group creation, never changed by TRaSH).
func audioFormatsTrashID(app string) string {
	switch app {
	case "radarr":
		return "9d5acd8f1da78dfbae788182f7605200"
	case "sonarr":
		return "e9a1944a254e6f8a9da63083f7ae15cb"
	}
	return ""
}

// hdrFormatsTrashID returns the trash_id of the primary HDR Formats cf-group
// for the given app. Same purpose as audioFormatsTrashID for HDR scoring.
func hdrFormatsTrashID(app string) string {
	switch app {
	case "radarr":
		return "ef20e67b95a381fb3bc6d1f06ea24f46"
	case "sonarr":
		return "7e1724c5da59e7474803ad25be98f6a3"
	}
	return ""
}

// isHDRVariantGroup reports whether a cf-group name is one of the HDR variant
// opt-ins (DV Boost, DV w/o HDR fallback, HDR10+ Boost, SDR, etc.) — used so
// the HDR axis pill can summarise "DV/HDR10+ opt-in" without dumping the
// whole group list onto the pill.
func isHDRVariantGroup(name string) bool {
	if !strings.HasPrefix(name, "[HDR Formats]") {
		return false
	}
	// Exclude the primary "HDR" group itself; we only want the variants
	return !strings.HasSuffix(strings.TrimSpace(name), " HDR")
}

// shortHDRVariantName extracts a short pill-label from a cf-group name like
// "[HDR Formats] DV Boost" → "DV Boost". Returns "" for the SDR negative-prefer
// group since users rarely care about it on the axis pill.
func shortHDRVariantName(name string) string {
	short := strings.TrimSpace(strings.TrimPrefix(name, "[HDR Formats]"))
	if short == "SDR" {
		return "" // omit from axis pill — too niche
	}
	return short
}
