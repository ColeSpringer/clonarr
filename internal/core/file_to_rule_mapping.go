package core

import (
	"strings"
)

// FileChangeKind enumerates the categories of TRaSH-Guides JSON files that
// matter for Profile Sync detection. Unknown/non-data files (README,
// CHANGELOG, includes/, etc.) classify as kindOther so the caller can
// ignore them.
type FileChangeKind string

const (
	FileChangeCF             FileChangeKind = "cf"
	FileChangeCFGroup        FileChangeKind = "cf-group"
	FileChangeQualityProfile FileChangeKind = "quality-profile"
	FileChangeQualitySize    FileChangeKind = "quality-size"
	FileChangeOther          FileChangeKind = "other"
)

// ClassifiedFile is the structured output of ClassifyTrashFilePath: what
// kind of TRaSH object the file represents, the trash_id (= filename
// without .json), and which Arr app-type it belongs to.
type ClassifiedFile struct {
	Kind    FileChangeKind
	TrashID string // filename without .json — matches trash_id in the JSON content
	AppType string // "radarr" | "sonarr" | "" for other
}

// ClassifyTrashFilePath maps a TRaSH-Guides repo path (e.g.
// "docs/json/radarr/cf/abc1234.json") to a structured (kind, trash_id,
// app-type) tuple. Returns Kind=FileChangeOther for paths that don't match
// the data-file conventions; callers should skip those.
//
// Conventions matched (per TRaSH-Guides repo layout):
//   docs/json/<app>/cf/*.json              → CF
//   docs/json/<app>/cf-groups/*.json       → cf-group
//   docs/json/<app>/quality-profiles/*.json → quality-profile
//   docs/json/<app>/quality-size/*.json    → quality-size
//
// trash_id is taken from the filename (without .json extension) — this
// matches the convention that file-name = trash_id in TRaSH's source repo.
func ClassifyTrashFilePath(path string) ClassifiedFile {
	parts := strings.Split(path, "/")
	if len(parts) < 5 || parts[0] != "docs" || parts[1] != "json" {
		return ClassifiedFile{Kind: FileChangeOther}
	}
	app := parts[2]
	if app != "radarr" && app != "sonarr" {
		return ClassifiedFile{Kind: FileChangeOther}
	}
	category := parts[3]
	filename := parts[len(parts)-1]
	if !strings.HasSuffix(filename, ".json") {
		return ClassifiedFile{Kind: FileChangeOther}
	}
	trashID := strings.TrimSuffix(filename, ".json")

	var kind FileChangeKind
	switch category {
	case "cf":
		kind = FileChangeCF
	case "cf-groups":
		kind = FileChangeCFGroup
	case "quality-profiles":
		kind = FileChangeQualityProfile
	case "quality-size":
		kind = FileChangeQualitySize
	default:
		return ClassifiedFile{Kind: FileChangeOther}
	}
	return ClassifiedFile{Kind: kind, TrashID: trashID, AppType: app}
}

// RuleAffectedTrashIDs returns the subset of changedTrashIDs that affect
// the given sync rule, with the rule's ExcludedCFs filter applied.
//
// "Affect" means: the trash_id is in the rule's profile-eligible-CF set —
// i.e. the CF appears in profile.FormatItems OR is a member of a cf-group
// whose quality_profiles.include lists this profile. (Same scope as
// ComputeRuleCustomizations uses for the "is Additional CF" check, so the
// in-UI customizations count and the per-rule notification stay consistent.)
//
// Returns empty slice when no changed trash_ids affect this rule — caller
// then skips notification firing + PendingChanges persistence for it.
func RuleAffectedTrashIDs(rule *AutoSyncRule, profile *TrashQualityProfile, ad *AppData, changedTrashIDs []string) []string {
	if rule == nil || profile == nil || len(changedTrashIDs) == 0 {
		return nil
	}
	eligible := profileEligibleCFSet(profile, ad)
	excluded := make(map[string]bool, len(rule.ExcludedCFs))
	for _, ex := range rule.ExcludedCFs {
		excluded[ex] = true
	}
	out := make([]string, 0, len(changedTrashIDs))
	for _, tid := range changedTrashIDs {
		if !eligible[tid] {
			continue
		}
		if excluded[tid] {
			continue
		}
		out = append(out, tid)
	}
	return out
}

// profileEligibleCFSet mirrors the profileEligibleCFs computation in
// ComputeRuleCustomizations (customizations.go). Extracted here so the
// detection-mapping path doesn't pull in the full customizations-diff
// machinery. Single-source-of-truth would be nice but the function lives
// inside a closure-heavy method body; mechanical re-extraction keeps both
// callers honest until a future refactor.
func profileEligibleCFSet(profile *TrashQualityProfile, ad *AppData) map[string]bool {
	set := make(map[string]bool)
	if profile == nil {
		return set
	}
	for _, tid := range profile.FormatItems {
		set[tid] = true
	}
	if ad == nil {
		return set
	}
	for _, g := range ad.CFGroups {
		if _, ok := g.QualityProfiles.Include[profile.Name]; !ok {
			continue
		}
		for _, cf := range g.CustomFormats {
			set[cf.TrashID] = true
		}
	}
	return set
}
