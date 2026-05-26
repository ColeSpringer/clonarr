// Package core — customizations counter.
//
// ComputeRuleCustomizations diffs a saved sync rule against the TRaSH
// profile defaults it targets, returning a count breakdown matching the
// detail view's "Override mode · N changes" header. Pure read-only —
// touches no sync state, never calls Arr.
//
// Counted categories (matches frontend pdOverrideSummary):
//   - Quality: cutoffQuality override + QualityStructure leaf-diff against
//     profile.Items (when QualityStructure is set), OR len(QualityOverrides)
//     when only the legacy flat map is used
//   - ExtraCFs: ScoreOverrides entries whose trashID is NOT in the
//     profile's effective default CF set (FormatItems + default-on cf-groups)
//   - CustomScores: ScoreOverrides entries whose trashID IS in defaults
//     but whose score differs from the CF's TRaSH default for this profile's
//     scoreSet
//   - General: Overrides settings (language, upgradeAllowed, min/cutoff
//     scores) that are non-nil AND differ from profile defaults
//
// Categories deliberately NOT counted in this first version (consistent
// with the detail view's current behaviour; expand both views together
// in a later pass if needed):
//   - KeepArrCFIDs: Arr-only CF preservation markers
//   - Optional cf-groups: default-off groups the user toggled on
//
// ExcludedCFs ARE counted — the lock-icon UI lets the user opt out of
// required CFs, so the rule can carry excludedCFs that materially affect
// what syncs. Without surfacing the count, a rule with N excluded CFs
// reads as "0 customizations" in the Sync Rules pill, which actively
// misleads.

package core

import "strings"

// RuleCustomizations is the per-rule breakdown returned by
// ComputeRuleCustomizations and exposed via the API endpoint.
type RuleCustomizations struct {
	Quality      int `json:"quality"`
	ExtraCFs     int `json:"extraCFs"`
	CustomScores int `json:"customScores"`
	General      int `json:"general"`
	ExcludedCFs  int `json:"excludedCFs"`
	Total        int `json:"total"`
}

// ComputeRuleCustomizations returns the per-rule customization breakdown.
// Caller is responsible for resolving `profile` from the rule's
// TrashProfileID against the active TRaSH snapshot; passing nil for either
// profile or ad results in an empty (all-zero) breakdown.
//
// `customCFIDs` is the set of user-created custom CF trash IDs (the
// `custom:<id>` registry). Used to filter out dangling references in
// ScoreOverrides — when a user deletes a custom CF, its entry can
// linger on rules until the next successful sync's CleanupDangling
// pass purges it. The detail view hides those orphans; the list pill
// must do the same to keep counts consistent.
// `appType` is "radarr" or "sonarr"; used to gate the Sonarr-doesn't-
// have-language defensive check.
func ComputeRuleCustomizations(rule *AutoSyncRule, profile *TrashQualityProfile, ad *AppData, customCFIDs map[string]bool, appType string) RuleCustomizations {
	var out RuleCustomizations
	if rule == nil || profile == nil {
		return out
	}

	// Effective default CF set — what TRaSH would include in the profile
	// without any user customization. FormatItems + default-on groups.
	defaultCFs := ComputeTrashDefaults(profile, ad)

	// Profile-eligible CF set — every CF that lives in any cf-group
	// whose quality_profiles.include lists this profile, plus
	// formatItems. Used to classify an opt-in as "true Additional"
	// (CF lives in a group OUTSIDE the profile's scope, e.g. HDR
	// Formats for WEB-1080p) vs "profile opt-in" (CF lives in a
	// default-off group that IS in the profile's scope, e.g. Streaming
	// Services UK for WEB-1080p — the group is offered for opt-in by
	// the profile design, so opting in isn't an "Additional CF").
	// Matches frontend pdAllCustomizations's inProfileGroups filter.
	// Without this distinction, opting into a profile-eligible default-
	// off group inflated the ExtraCFs count vs the editor's view.
	profileEligibleCFs := make(map[string]bool)
	for _, tid := range profile.FormatItems {
		profileEligibleCFs[tid] = true
	}
	if ad != nil {
		for _, g := range ad.CFGroups {
			if _, ok := g.QualityProfiles.Include[profile.Name]; !ok {
				continue
			}
			for _, cfEntry := range g.CustomFormats {
				profileEligibleCFs[cfEntry.TrashID] = true
			}
		}
	}

	// Score-set context. Profiles with a TrashScoreSet override pull from
	// that set; everything else uses "default". Matches sync engine.
	scoreSet := profile.TrashScoreSet
	if scoreSet == "" {
		scoreSet = "default"
	}

	// 1a. Pure Additional CF opt-ins from SelectedCFs — CFs the user
	//     opted into via the Additional CF picker WITHOUT also setting
	//     a custom score. These live in rule.SelectedCFs only; the
	//     score-overrides loop below wouldn't see them. Without this
	//     pre-pass, a rule with N opt-ins and no score changes shows
	//     "0 customizations" in the Sync Rules table — actively
	//     misleading. Pair with frontend pdAllCustomizations which
	//     mirrors this logic for the editor-header count.
	counted := make(map[string]bool, len(rule.SelectedCFs))
	for _, tid := range rule.SelectedCFs {
		// Skip if CF lives in any profile-eligible group (including
		// default-off opt-in groups like Streaming Services UK for
		// WEB-1080p). Those are "profile opt-ins", not "Additional".
		// Only count when the CF is truly outside the profile's scope
		// (e.g. HDR Formats for WEB-1080p).
		if profileEligibleCFs[tid] {
			continue
		}
		if strings.HasPrefix(tid, "custom:") {
			if customCFIDs == nil || !customCFIDs[tid] {
				continue
			}
		}
		out.ExtraCFs++
		counted[tid] = true
	}

	// 1b. Split ScoreOverrides into ExtraCFs (added beyond defaults) and
	//     CustomScores (override on default CF where score differs).
	for tid, userScore := range rule.ScoreOverrides {
		// Skip dangling `custom:<id>` orphans — the referenced custom CF
		// has been deleted from the registry but the rule's override
		// hasn't been cleaned yet. Detail view's pdAllCustomizations
		// hides these via its lookup() returning null for custom:-prefix
		// failures; we mirror that hiding here.
		if strings.HasPrefix(tid, "custom:") {
			if customCFIDs == nil || !customCFIDs[tid] {
				continue
			}
		}
		if !profileEligibleCFs[tid] {
			// Truly outside profile scope → Additional. Already counted
			// as a SelectedCFs opt-in above? Don't double-count.
			if !counted[tid] {
				out.ExtraCFs++
			}
			continue
		}
		if !defaultCFs[tid] {
			// CF is profile-eligible but not in TRaSH defaults (default-
			// off opt-in group). Override on it counts as a CustomScore
			// — user changed the recommended score for an opt-in. Falls
			// through to the score-vs-default comparison below.
		}
		// CF is in defaults — compare to its TRaSH default score. If we
		// can't resolve the default (unknown CF or no score for set), be
		// conservative and count the entry as a custom score: presence
		// of an override entry already signals intent.
		if ad == nil {
			out.CustomScores++
			continue
		}
		cf, ok := ad.CustomFormats[tid]
		if !ok {
			out.CustomScores++
			continue
		}
		defScore, hasDef := cf.TrashScores[scoreSet]
		if !hasDef {
			defScore, hasDef = cf.TrashScores["default"]
		}
		if !hasDef || userScore != defScore {
			out.CustomScores++
		}
	}

	// 2. Quality changes.
	//
	//    a) Cutoff override — counts when Overrides.CutoffQuality is set
	//       AND differs from profile.Cutoff.
	if rule.Overrides != nil && rule.Overrides.CutoffQuality != nil {
		if *rule.Overrides.CutoffQuality != profile.Cutoff {
			out.Quality++
		}
	}
	//    b) Quality items diff — leaf-flatten both sides and count
	//       differing or removed entries. Matches pdQualityItemsChangeCount.
	if len(rule.QualityStructure) > 0 {
		orig := flattenQualityLeaves(profile.Items)
		cur := flattenQualityLeaves(rule.QualityStructure)
		for name, allowed := range cur {
			if oa, ok := orig[name]; !ok || oa != allowed {
				out.Quality++
			}
		}
		for name := range orig {
			if _, ok := cur[name]; !ok {
				out.Quality++
			}
		}
	} else if len(rule.QualityOverrides) > 0 {
		out.Quality += len(rule.QualityOverrides)
	}

	// 3. General settings overrides. Each Overrides field is *T (nil =
	//    not overridden). Count when non-nil AND differs from default.
	//    Language is gated to Radarr to match pdGeneralChangeCount's
	//    behaviour — Sonarr profiles don't expose a Language editor so a
	//    Sonarr rule with Language set is almost certainly a hand-edited
	//    config; counting it would be unhelpful.
	if rule.Overrides != nil {
		ov := rule.Overrides
		if appType == "radarr" && ov.Language != nil && *ov.Language != profile.Language {
			out.General++
		}
		if ov.UpgradeAllowed != nil && *ov.UpgradeAllowed != profile.UpgradeAllowed {
			out.General++
		}
		if ov.MinFormatScore != nil && *ov.MinFormatScore != profile.MinFormatScore {
			out.General++
		}
		if ov.MinUpgradeFormatScore != nil && *ov.MinUpgradeFormatScore != profile.MinUpgradeFormatScore {
			out.General++
		}
		if ov.CutoffFormatScore != nil && *ov.CutoffFormatScore != profile.CutoffFormatScore {
			out.General++
		}
	}

	// 4. Excluded CFs — opt-outs from the profile's effective default
	//    set. Only count exclusions that actually subtract something —
	//    if the trashId isn't in defaultCFs, the entry is dead state
	//    (CF moved out of defaults upstream; backend cleans on next
	//    sync). Covers BOTH excluded required CFs (always in defaults
	//    via FormatItems / required-in-active-groups) AND opt-outs
	//    from default-on optional CFs.
	for _, tid := range rule.ExcludedCFs {
		if defaultCFs[tid] {
			out.ExcludedCFs++
		}
	}

	out.Total = out.Quality + out.ExtraCFs + out.CustomScores + out.General + out.ExcludedCFs
	return out
}

// flattenQualityLeaves converts a quality-structure slice into a leaf-
// level {leafName: allowed} map. Group items push their `allowed` down
// to each leaf member, so a group toggled on with three child qualities
// produces three map entries all set to true.
func flattenQualityLeaves(items []QualityItem) map[string]bool {
	out := make(map[string]bool)
	for _, it := range items {
		if len(it.Items) > 0 {
			for _, leaf := range it.Items {
				out[leaf] = it.Allowed
			}
		} else {
			out[it.Name] = it.Allowed
		}
	}
	return out
}
