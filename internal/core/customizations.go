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
//   - ExcludedCFs: opt-outs from default-on cf-groups
//   - KeepArrCFIDs: Arr-only CF preservation markers
//   - Optional cf-groups: default-off groups the user toggled on

package core

import "strings"

// RuleCustomizations is the per-rule breakdown returned by
// ComputeRuleCustomizations and exposed via the API endpoint.
type RuleCustomizations struct {
	Quality      int `json:"quality"`
	ExtraCFs     int `json:"extraCFs"`
	CustomScores int `json:"customScores"`
	General      int `json:"general"`
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

	// Score-set context. Profiles with a TrashScoreSet override pull from
	// that set; everything else uses "default". Matches sync engine.
	scoreSet := profile.TrashScoreSet
	if scoreSet == "" {
		scoreSet = "default"
	}

	// 1. Split ScoreOverrides into ExtraCFs (added beyond defaults) and
	//    CustomScores (override on default CF where score differs).
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
		if !defaultCFs[tid] {
			out.ExtraCFs++
			continue
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

	out.Total = out.Quality + out.ExtraCFs + out.CustomScores + out.General
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
