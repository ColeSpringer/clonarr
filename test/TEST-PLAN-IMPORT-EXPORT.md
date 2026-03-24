# Syncarr Import/Export Test Plan

## Prerequisites
- `syncarr:latest` deployed on Unraid
- TRaSH repo cloned (Settings > Pull)
- At least one Radarr instance configured

---

## Test 1: TRaSH Profile JSON Import (paste)
1. Copy contents of a real TRaSH profile JSON (e.g., SQP-5 from repo)
2. Go to Profiles > Radarr tab > Import button
3. Paste the JSON into the text field
4. **Verify:** Auto-detected as "TRaSH JSON" (not Syncarr/YAML)
5. **Verify:** Profile appears under Imported Profiles with correct name
6. **Verify:** Open profile — shows TRaSH-backed layout (Required, Optional groups, Streaming Services with sub-categories and colors) — NOT flat "Custom Formats" list
7. **Verify:** Settings bar shows correct values (score set, min score, cutoff, upgrade, language)
8. **Verify:** CF count matches the original TRaSH profile

## Test 2: Syncarr JSON Export + Re-import (round-trip)
1. Open an imported profile (from Test 1) in profile detail
2. Click Export > JSON tab > Copy
3. Delete the profile
4. Import > Paste the copied JSON
5. **Verify:** Auto-detected as "Syncarr JSON"
6. **Verify:** All fields preserved: name, scores, formatGroups, qualities, settings
7. **Verify:** Profile detail view shows same layout as before export
8. **Verify:** CF scores match original (spot-check 3-4 CFs)

## Test 3: TRaSH JSON Export format validation
1. Open any profile with trashProfileId
2. Click Export > TRaSH JSON tab
3. **Verify:** `formatItems` is `{"CF Name": "trash_id"}` — NOT `{"CF Name": {"trash_id": "...", "score": N}}`
4. **Verify:** No `score` fields inside formatItems
5. **Verify:** Top-level fields match TRaSH format: `trash_id`, `name`, `trash_score_set`, `group`, `upgradeAllowed`, `cutoff`, `minFormatScore`, `cutoffFormatScore`, `minUpgradeFormatScore`, `language`, `items`, `formatItems`
6. **Verify:** `items` array has quality groups with `name`, `allowed`, optional `items` sub-array
7. Compare with real TRaSH JSON from repo — structure should be identical

## Test 4: Recyclarr YAML Import (v7 style)
1. Paste a real Recyclarr YAML config (v7 with `custom_formats` + `quality_profiles`)
2. **Verify:** Auto-detected as YAML
3. **Verify:** Profile(s) created with correct CF scores
4. **Verify:** formatGroups populated (CFs categorized, not all under "Custom Formats")

## Test 5: Recyclarr YAML Import (v8 with custom_format_groups)
1. Paste a v8 YAML with `custom_format_groups` section
2. **Verify:** Groups resolved — CFs from groups appear with correct scores
3. **Verify:** `select`/`exclude` filters respected

## Test 6: Recyclarr YAML Export validation
1. Open any imported profile
2. Click Export > Recyclarr YAML tab
3. **Verify:** Valid YAML structure with `radarr:` or `sonarr:` top-level key
4. **Verify:** `quality_profiles` section has name, score_set, upgrade, min_format_score
5. **Verify:** `custom_formats` section groups CFs by score with `trash_ids` + `assign_scores_to`
6. **Verify:** Copy the YAML, re-import it — profile should match original (round-trip)

## Test 7: Profile without trashProfileId (fallback display)
1. Import the test Syncarr export (`test/test-import-syncarr-export.json`) — has no trashProfileId
2. **Verify:** formatGroups resolved from TRaSH data at import time
3. **Verify:** Profile detail shows CFs categorized (HQ Release Groups, Unwanted, etc.) — not all under "Custom Formats"

## Test 8: Error handling
1. Paste invalid content (random text) — should show clear error
2. Paste valid JSON without `trash_id` or `id`+`appType` — should show "unrecognized format" error
3. Paste empty string — should show error

---

## Quick Smoke Test (minimum for deploy validation)
If short on time, run Tests 1, 2, 3, and 7. These cover:
- TRaSH JSON import + display
- Syncarr round-trip (our own format)
- TRaSH export format correctness
- Fallback categorization
