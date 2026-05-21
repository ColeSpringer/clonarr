package core

import "testing"

// Tests for ExcludedCFs counting (Phase 2c, 2026-05-21). The struct's
// other fields (Quality, ExtraCFs, CustomScores, General) are covered
// implicitly via the autosync flow tests; this file targets the new
// bucket in isolation.

func makeBaseFixture() (*TrashQualityProfile, *AppData) {
	profile := &TrashQualityProfile{
		TrashID: "profile-1",
		Name:    "Test Profile",
		// FormatItems is name → trash_id; required CFs the user can
		// opt out of via Phase 2c sit here.
		FormatItems: map[string]string{
			"Required A": "req-a",
			"Required B": "req-b",
		},
	}
	defTrue := true
	defFalse := false
	groupOn := &TrashCFGroup{
		Name:    "[Unwanted] Default On",
		TrashID: "grp-on",
		Default: "true",
		CustomFormats: []CFGroupEntry{
			{Name: "Group Required", TrashID: "grp-req", Required: true},
			{Name: "Group Default", TrashID: "grp-def", Default: &defTrue},
			{Name: "Group Optional", TrashID: "grp-opt", Default: &defFalse},
		},
	}
	groupOn.QualityProfiles.Include = map[string]string{"Test Profile": "profile-1"}
	ad := &AppData{
		CustomFormats: map[string]*TrashCF{},
		CFGroups:      []*TrashCFGroup{groupOn},
	}
	return profile, ad
}

func TestComputeRuleCustomizations_ExcludedCFs_RequiredFromFormatItems(t *testing.T) {
	profile, ad := makeBaseFixture()
	rule := &AutoSyncRule{
		ID:          "r1",
		ExcludedCFs: []string{"req-a"},
	}
	out := ComputeRuleCustomizations(rule, profile, ad, nil, "radarr")
	if out.ExcludedCFs != 1 {
		t.Errorf("ExcludedCFs = %d, want 1 (excluded required FormatItem)", out.ExcludedCFs)
	}
	if out.Total != 1 {
		t.Errorf("Total = %d, want 1", out.Total)
	}
}

func TestComputeRuleCustomizations_ExcludedCFs_RequiredFromGroup(t *testing.T) {
	profile, ad := makeBaseFixture()
	rule := &AutoSyncRule{ExcludedCFs: []string{"grp-req"}}
	out := ComputeRuleCustomizations(rule, profile, ad, nil, "radarr")
	if out.ExcludedCFs != 1 {
		t.Errorf("ExcludedCFs = %d, want 1 (excluded group-required)", out.ExcludedCFs)
	}
}

func TestComputeRuleCustomizations_ExcludedCFs_DefaultOnOptional(t *testing.T) {
	profile, ad := makeBaseFixture()
	rule := &AutoSyncRule{ExcludedCFs: []string{"grp-def"}}
	out := ComputeRuleCustomizations(rule, profile, ad, nil, "radarr")
	if out.ExcludedCFs != 1 {
		t.Errorf("ExcludedCFs = %d, want 1 (excluded default-on CF)", out.ExcludedCFs)
	}
}

func TestComputeRuleCustomizations_ExcludedCFs_NotInDefaults(t *testing.T) {
	profile, ad := makeBaseFixture()
	// Excluding a CF that isn't in defaults is dead state — don't count.
	rule := &AutoSyncRule{ExcludedCFs: []string{"grp-opt", "ghost-cf"}}
	out := ComputeRuleCustomizations(rule, profile, ad, nil, "radarr")
	if out.ExcludedCFs != 0 {
		t.Errorf("ExcludedCFs = %d, want 0 (none in defaults)", out.ExcludedCFs)
	}
}

func TestComputeRuleCustomizations_ExcludedCFs_Mixed(t *testing.T) {
	profile, ad := makeBaseFixture()
	rule := &AutoSyncRule{
		ExcludedCFs: []string{"req-a", "req-b", "grp-req", "grp-def", "ghost-cf"},
	}
	out := ComputeRuleCustomizations(rule, profile, ad, nil, "radarr")
	// req-a + req-b (FormatItems) + grp-req (group-required) + grp-def
	// (default-on optional) → 4. ghost-cf isn't in defaults → not counted.
	if out.ExcludedCFs != 4 {
		t.Errorf("ExcludedCFs = %d, want 4", out.ExcludedCFs)
	}
	if out.Total != 4 {
		t.Errorf("Total = %d, want 4 (no other customizations)", out.Total)
	}
}

func TestComputeRuleCustomizations_ExcludedCFs_AddsToTotal(t *testing.T) {
	profile, ad := makeBaseFixture()
	min := 100
	rule := &AutoSyncRule{
		ExcludedCFs: []string{"req-a"},
		Overrides:   &SyncOverrides{MinFormatScore: &min},
	}
	out := ComputeRuleCustomizations(rule, profile, ad, nil, "radarr")
	if out.General != 1 {
		t.Errorf("General = %d, want 1", out.General)
	}
	if out.ExcludedCFs != 1 {
		t.Errorf("ExcludedCFs = %d, want 1", out.ExcludedCFs)
	}
	if out.Total != 2 {
		t.Errorf("Total = %d, want 2 (General + ExcludedCFs)", out.Total)
	}
}
