package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"clonarr/internal/arr"
)

// DriftRunner detects Arr-side drift: for each enabled auto-sync rule
// it fetches the current Arr quality profile, builds the target profile
// (what clonarr would push if it synced now), and diffs them field by
// field. Both shapes are the same arr.ArrQualityProfile struct so the
// diff is a direct field walk — no separate comparison engine needed.
//
// Concurrency: runs are serialised via mu so a manual /api/drift/check
// trigger and the scheduled tick can't double-fetch.
type DriftRunner struct {
	app *App
	mu  sync.Mutex
}

// NewDriftRunner constructs a DriftRunner bound to the given App.
func NewDriftRunner(app *App) *DriftRunner {
	return &DriftRunner{app: app}
}

// RunOnce walks every eligible auto-sync rule, fetches each rule's
// current Arr profile, diffs against the BuildArrProfile target, and
// persists the aggregate summary to DriftWatch.LastResult. Returns
// the per-rule results so the caller (e.g. /api/drift/check handler)
// can return them inline.
//
// Eligibility filter (kept narrow — drift is read-only so paused
// rules still count; user might want to know an Arr profile drifted
// even on a rule they manually disabled):
//   - rule.OrphanedAt == "" (soft-tombstoned rules skipped — Arr
//     profile is gone, nothing to compare)
//   - rule.ArrProfileID != 0 (rule never finished its first sync)
//   - instance referenced by rule.InstanceID still exists
//   - TRaSH profile referenced by rule.TrashProfileID still loaded
//
// Per-rule errors (Arr unreachable, build failure) collect into the
// aggregate Errors slice but do not abort the walk — one broken
// instance shouldn't hide drift on the rest.
func (d *DriftRunner) RunOnce(ctx context.Context) ([]DriftResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := d.app.Config.Get()

	// Collect eligible rules + resolve their instances up front so the
	// hot loop below only touches the filtered set.
	type ruleWork struct {
		rule     AutoSyncRule
		instance Instance
	}
	var work []ruleWork
	for _, r := range cfg.AutoSync.Rules {
		if r.OrphanedAt != "" || r.ArrProfileID == 0 {
			continue
		}
		inst, ok := d.app.Config.GetInstance(r.InstanceID)
		if !ok {
			continue
		}
		work = append(work, ruleWork{rule: r, instance: inst})
	}

	// Per-instance Arr snapshot cache: many rules can share an instance,
	// so List* calls only happen once per instance per drift run.
	instCache := make(map[string]*arrSnapshot)
	fetchInst := func(inst Instance) *arrSnapshot {
		if s, ok := instCache[inst.ID]; ok {
			return s
		}
		s := &arrSnapshot{}
		client := arr.NewArrClient(inst.URL, inst.APIKey, d.app.HTTPClient)
		s.profiles, s.err = client.ListProfiles()
		if s.err == nil {
			s.cfs, s.err = client.ListCustomFormats()
		}
		if s.err == nil {
			s.qDefs, s.err = client.ListQualityDefinitions()
		}
		if s.err == nil && inst.Type == "radarr" {
			s.langs, s.err = client.ListLanguages()
		}
		instCache[inst.ID] = s
		return s
	}

	now := time.Now().UTC().Format(time.RFC3339)
	results := make([]DriftResult, 0, len(work))
	var errs []string
	driftCount := 0

	for _, w := range work {
		if ctx.Err() != nil {
			break
		}
		snap := fetchInst(w.instance)
		if snap.err != nil {
			errs = append(errs, fmt.Sprintf("rule %s (%s): %v", w.rule.ID, w.instance.Name, snap.err))
			continue
		}

		// Find the current Arr profile by ID. Missing == soft-orphan
		// territory; the autosync engine handles that flagging path,
		// so drift just skips quietly here.
		var current *arr.ArrQualityProfile
		for i := range snap.profiles {
			if snap.profiles[i].ID == w.rule.ArrProfileID {
				current = &snap.profiles[i]
				break
			}
		}
		if current == nil {
			continue
		}

		appData := d.app.Trash.GetAppData(w.instance.Type)
		if appData == nil {
			errs = append(errs, fmt.Sprintf("rule %s: TRaSH app data unavailable for %s", w.rule.ID, w.instance.Type))
			continue
		}
		var profile *TrashQualityProfile
		for _, p := range appData.Profiles {
			if p.TrashID == w.rule.TrashProfileID {
				profile = p
				break
			}
		}
		if profile == nil {
			// TRaSH profile gone (upstream removed or local data stale)
			// — autosync engine surfaces this separately; drift skips.
			continue
		}

		// Reproduce sync.go's selectedCFs assembly so the target reflects
		// the rule's saved opt-ins / opt-outs vs current TRaSH defaults.
		nameToID := make(map[string]int, len(snap.cfs))
		for _, cf := range snap.cfs {
			nameToID[cf.Name] = cf.ID
		}
		selectedCFs := ComputeTrashDefaults(profile, appData)
		for _, id := range w.rule.SelectedCFs {
			selectedCFs[id] = true
		}
		for _, id := range w.rule.ExcludedCFs {
			delete(selectedCFs, id)
		}
		customCFs := d.app.CustomCFs.List(w.instance.Type)

		target, buildErr := BuildArrProfile(
			profile, appData, snap.qDefs, snap.langs, nameToID,
			selectedCFs, current.Name, w.rule.ScoreOverrides,
			customCFs, w.rule.QualityStructure,
		)
		if buildErr != nil {
			errs = append(errs, fmt.Sprintf("rule %s: build target: %v", w.rule.ID, buildErr))
			continue
		}

		details := diffArrProfile(current, target, snap.cfs)
		result := DriftResult{
			RuleID:        w.rule.ID,
			CheckedAt:     now,
			DriftDetected: len(details) > 0,
			Details:       details,
		}
		if len(details) > 0 {
			result.DriftSummary = summariseDrift(details)
			driftCount++
		}
		results = append(results, result)
	}

	// Persist aggregate to DriftWatch.LastResult so the UI can render
	// "last check: X ago" + the count without keeping a runtime cache.
	if updErr := d.app.Config.Update(func(c *Config) {
		if c.DriftWatch == nil {
			c.DriftWatch = &DriftWatch{}
		}
		c.DriftWatch.LastRun = now
		c.DriftWatch.LastResult = &DriftRunResult{
			DriftsDetected: driftCount,
			Errors:         errs,
		}
	}); updErr != nil {
		return results, fmt.Errorf("persist drift result: %w", updErr)
	}

	return results, nil
}

// arrSnapshot is the per-instance fetch cache used inside one drift run.
type arrSnapshot struct {
	profiles []arr.ArrQualityProfile
	cfs      []arr.ArrCF
	qDefs    []arr.ArrQualityDefinition
	langs    []arr.ArrLanguage
	err      error
}

// diffArrProfile walks every field on the Arr profile that drift cares
// about — top-level scalars (upgradeAllowed, cutoff, min/cutoff/upgrade
// scores, language), per-CF scores, and the quality-items structure —
// and emits one DriftDetail per divergence. Output order is stable so
// log diffs and UI rendering don't shuffle between runs.
func diffArrProfile(current, target *arr.ArrQualityProfile, cfs []arr.ArrCF) []DriftDetail {
	var out []DriftDetail

	// Scalar settings.
	if current.UpgradeAllowed != target.UpgradeAllowed {
		out = append(out, DriftDetail{Field: "upgradeAllowed", Current: current.UpgradeAllowed, Target: target.UpgradeAllowed})
	}
	if current.Cutoff != target.Cutoff {
		curName := qualityIDName(current.Cutoff, current.Items)
		tgtName := qualityIDName(target.Cutoff, target.Items)
		out = append(out, DriftDetail{Field: "cutoff", Current: curName, Target: tgtName})
	}
	if current.MinFormatScore != target.MinFormatScore {
		out = append(out, DriftDetail{Field: "minFormatScore", Current: current.MinFormatScore, Target: target.MinFormatScore})
	}
	if current.CutoffFormatScore != target.CutoffFormatScore {
		out = append(out, DriftDetail{Field: "cutoffFormatScore", Current: current.CutoffFormatScore, Target: target.CutoffFormatScore})
	}
	if current.MinUpgradeFormatScore != target.MinUpgradeFormatScore {
		out = append(out, DriftDetail{Field: "minUpgradeFormatScore", Current: current.MinUpgradeFormatScore, Target: target.MinUpgradeFormatScore})
	}
	if !sameLanguage(current.Language, target.Language) {
		out = append(out, DriftDetail{Field: "language", Current: languageName(current.Language), Target: languageName(target.Language)})
	}

	// Per-CF scores. Both slices include every Arr CF (Arr stores 0 for
	// unscored), so a flat score compare keyed by Arr CF ID catches:
	//   - score changed (cur=50, tgt=100 → drift)
	//   - extra-in-arr (cur=50, tgt=0 → user scored a CF clonarr doesn't manage)
	//   - removed-in-arr (cur=0, tgt=50 → user zeroed a CF clonarr expects scored)
	idToName := make(map[int]string, len(cfs))
	for _, cf := range cfs {
		idToName[cf.ID] = cf.Name
	}
	tgtScores := make(map[int]int, len(target.FormatItems))
	for _, fi := range target.FormatItems {
		tgtScores[fi.Format] = fi.Score
	}
	var scoreDiffs []DriftDetail
	for _, ci := range current.FormatItems {
		ts := tgtScores[ci.Format]
		if ci.Score == ts {
			continue
		}
		scoreDiffs = append(scoreDiffs, DriftDetail{
			Field:   "score",
			CFName:  idToName[ci.Format],
			Current: ci.Score,
			Target:  ts,
		})
	}
	sort.Slice(scoreDiffs, func(i, j int) bool { return scoreDiffs[i].CFName < scoreDiffs[j].CFName })
	out = append(out, scoreDiffs...)

	// Quality items structure: compare allowed-set + group structure as
	// flat name → allowed maps. Order changes alone don't count as drift
	// (Arr's items array is order-sensitive for cutoff resolution, but
	// the sync engine already normalises that on push). Reorder-only
	// changes that don't flip any allowed flag are a no-op for the user.
	curAllowed := flattenAllowed(current.Items)
	tgtAllowed := flattenAllowed(target.Items)
	seen := make(map[string]bool, len(curAllowed)+len(tgtAllowed))
	for name, ca := range curAllowed {
		seen[name] = true
		ta, ok := tgtAllowed[name]
		if !ok {
			// Quality exists in current but not target — unusual, would mean
			// Arr added a quality unknown to clonarr's items. Flag as drift.
			out = append(out, DriftDetail{Field: "quality", CFName: name, Current: ca, Target: false})
			continue
		}
		if ca != ta {
			out = append(out, DriftDetail{Field: "quality", CFName: name, Current: ca, Target: ta})
		}
	}
	for name, ta := range tgtAllowed {
		if seen[name] {
			continue
		}
		out = append(out, DriftDetail{Field: "quality", CFName: name, Current: false, Target: ta})
	}

	return out
}

// qualityIDName resolves a quality-or-group ID to its display name by
// walking the profile's items tree. Used for cutoff diff messages so
// the UI shows "cutoff: HDTV-1080p → Bluray-1080p" instead of bare IDs.
func qualityIDName(id int, items []arr.ArrQualityItem) string {
	for _, it := range items {
		if it.Quality != nil && it.Quality.ID == id {
			return it.Quality.Name
		}
		if it.ID != 0 && it.ID == id && it.Name != "" {
			return it.Name
		}
	}
	return fmt.Sprintf("id=%d", id)
}

func sameLanguage(a, b *arr.ArrLanguage) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ID == b.ID
}

func languageName(l *arr.ArrLanguage) string {
	if l == nil {
		return "(none)"
	}
	if l.Name != "" {
		return l.Name
	}
	return fmt.Sprintf("id=%d", l.ID)
}

// flattenAllowed reduces an items tree to a flat quality-name → allowed
// map. Group rows propagate their allowed flag to each member quality so
// disabling a group reads the same as disabling each of its members
// (Arr enforces that for cutoff resolution anyway).
func flattenAllowed(items []arr.ArrQualityItem) map[string]bool {
	out := make(map[string]bool)
	for _, it := range items {
		if len(it.Items) > 0 {
			for _, m := range it.Items {
				name := ""
				if m.Quality != nil {
					name = m.Quality.Name
				} else if m.Name != "" {
					name = m.Name
				}
				if name == "" {
					continue
				}
				out[name] = it.Allowed
			}
			continue
		}
		name := ""
		if it.Quality != nil {
			name = it.Quality.Name
		} else if it.Name != "" {
			name = it.Name
		}
		if name == "" {
			continue
		}
		out[name] = it.Allowed
	}
	return out
}

// summariseDrift produces one or two short human-readable lines from
// the per-field details for the rule's drift card. Counts per category
// so a profile with many CF-score drifts reads "12 CF scores differ"
// rather than a 12-line wall.
func summariseDrift(details []DriftDetail) []string {
	scoreCount := 0
	qualityCount := 0
	settingsCount := 0
	for _, d := range details {
		switch d.Field {
		case "score":
			scoreCount++
		case "quality":
			qualityCount++
		default:
			settingsCount++
		}
	}
	var out []string
	if scoreCount > 0 {
		noun := "CF score differs"
		if scoreCount != 1 {
			noun = "CF scores differ"
		}
		out = append(out, fmt.Sprintf("%d %s from target", scoreCount, noun))
	}
	if settingsCount > 0 {
		noun := "profile setting"
		if settingsCount != 1 {
			noun = "profile settings"
		}
		out = append(out, fmt.Sprintf("%d %s differ", settingsCount, noun))
	}
	if qualityCount > 0 {
		noun := "quality item"
		if qualityCount != 1 {
			noun = "quality items"
		}
		out = append(out, fmt.Sprintf("%d %s differ", qualityCount, noun))
	}
	return out
}
