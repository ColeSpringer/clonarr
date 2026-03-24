# Clonarr — Design Notes

Internal design notes and research for the Clonarr project.

---

## How Existing Tools Work

### Recyclarr
- **Language:** C# (.NET)
- **Data fetch:** `git clone` of TRaSH-Guides/Guides repo + recyclarr/config-templates repo
- **Config:** YAML files referencing trash_ids
- **Templates:** Community-maintained YAML in separate repo (recyclarr/config-templates)
- **Profiles:** User manually writes YAML with CF trash_ids and scores
- **Sync:** CLI command, reads YAML → builds API calls → pushes to Radarr/Sonarr
- **No UI** — YAML-only workflow
- **SQP profiles:** Defined as YAML templates, user copies and customizes

### Configarr
- **Language:** TypeScript
- **Data fetch:** Same TRaSH-Guides repo
- **Config:** YAML/JSON files
- **Merge order:** TRaSH-Guides → Local files → Inline config
- **Sync:** Scheduled Docker container, runs every N minutes
- **No UI** — config-driven
- **Extra:** Supports custom CFs beyond TRaSH, experimental Whisparr/Readarr/Lidarr

### Notifiarr
- **Language:** Go
- **Data fetch:** Assumed same TRaSH-Guides repo (or API wrapper)
- **Has UI** — web-based profile builder
- **Paid service** — subscription required
- **Issues:** Slow, sync failures, profile detection problems, missing SQPs

---

## TRaSH Guides Data Structure

### Repository Layout
```
docs/json/
├── radarr/
│   ├── cf/                    # Individual CF JSON files (100+)
│   ├── cf-groups/             # Grouped CF collections
│   ├── quality-size/          # Quality definition presets
│   └── naming/                # Naming scheme templates
├── sonarr/
│   ├── cf/                    # Individual CF JSON files
│   ├── cf-groups/             # Grouped CF collections
│   ├── quality-size/          # Quality definition presets
│   └── naming/                # Naming scheme templates
└── metadata.json              # Resource structure definition
```

### CF Identification
- Every CF has a `trash_id` (UUID) — immutable across versions
- Tools use trash_id to match local CFs against TRaSH definitions
- Radarr/Sonarr CFs don't have trash_id natively — sync tools track mapping

### Scoring System
- `trash_scores` object in each CF JSON
- Context keys: `default`, `sqp-1-1080p`, `sqp-1-2160p`, `anime`, `german`, etc.
- Score ranges: -35000 (reject) to +2500 (top tier)
- Different profiles use different score contexts

### CF Groups
- Bundle related CFs for atomic application
- Referenced by profile definitions
- Can include/exclude specific CFs from a group

### Quality Profiles (SQP)
- Not stored as JSON in TRaSH repo directly
- Defined in guide documentation (markdown)
- Recyclarr templates encode them as YAML
- We need to parse/define these ourselves or use recyclarr templates as reference

---

## Key Technical Decisions to Make

### 1. How to get SQP profile definitions?
**Options:**
a) Parse recyclarr/config-templates YAML → extract CF lists and scores
b) Hardcode known SQP profiles (WEB-1080p, WEB-2160p, Remux, etc.)
c) Let users build profiles from CF list in UI
d) Derive from TRaSH repo `trash_scores` context keys + allow custom

**Decision:** (d) — derive SQPs directly from TRaSH CF data. **Do NOT depend on Recyclarr templates.**

Recyclarr templates are community-maintained and may fall behind or be abandoned.
Instead, scan all CF JSON files and collect `trash_scores` context keys (`default`,
`sqp-1-1080p`, `sqp-1-2160p`, `anime`, etc.). For each context, gather all CFs with
non-zero scores → that IS the profile. The TRaSH repo is the single source of truth.

**Approach:**
1. Clone TRaSH repo, scan `docs/json/{radarr,sonarr}/cf/*.json`
2. For each CF, read `trash_scores` → map context_key → [{trash_id, name, score}]
3. Context keys become available SQP profiles in the UI
4. New contexts added to TRaSH repo are auto-discovered on next pull
5. Users can additionally build custom profiles from the full CF list

### 2. How to track trash_id ↔ Radarr CF mapping?
- When we create a CF in Radarr, Radarr assigns its own numeric ID
- We need to store: trash_id → radarr_cf_id mapping per instance
- On subsequent syncs, use mapping to PUT (update) instead of POST (create)
- Recyclarr does this by matching CF name — fragile but works
- Better: store mapping in our config/database

### 3. How to detect existing CFs?
- GET /api/v3/customformat returns all CFs with their specs
- Match by name or by comparing specifications against trash JSON
- Need a "diff" algorithm to show what would change

### 4. Git clone vs raw HTTP?
- **Git clone:** Full repo access, fast subsequent pulls, ~50MB
- **Raw HTTP:** Per-file fetch, no git dependency, more API calls
- **Recommendation:** Git clone with periodic pull (like Recyclarr)

### 5. Radarr vs Sonarr API differences?
- Both use /api/v3 with identical CF structure
- Source enum values differ (WEBDL=7 in Radarr, WEBDL=3 in Sonarr)
- Some CFs are Radarr-only (movie versions) or Sonarr-only (season packs)
- Handle via separate CF directories (already separated in TRaSH repo)

---

## Feature Roadmap

Features are listed in rough priority order. Implemented features are marked with status.

---

### Custom Formats & Profiles — CF Editor
**Priority:** High | **Status:** Not started

Build custom CFs directly in the Clonarr UI. Radarr/Sonarr CFs are JSON with specs
(regex patterns matching release name, quality, source, etc.). The CF editor would let
users define:
- CF name
- Specifications: field (release title, quality, source, etc.) + regex pattern + negate flag
- Scores per profile context

Example use case: Create "NORDiC Release Groups" CF with regex `(?i)\b(NORDiC|NORWEGiAN)\b`,
assign score +1000, sync to Radarr. Uses the same Arr API endpoint as TRaSH CFs — no
difference from the instance's perspective.

Custom CFs are stored in Clonarr's profile storage and can be exported as TRaSH JSON
for sharing with others or contributing back to the community.

Also includes custom quality profiles — build a profile from any combination of TRaSH
CFs, custom CFs, and custom scores. Import/export as Recyclarr YAML or TRaSH JSON.

---

### Naming Scheme Sync
**Priority:** High | **Status:** Not started (data parsing done, API call missing)

TRaSH has recommended naming schemes for files and folders in Radarr/Sonarr. We already
parse this data from the TRaSH repo and display it in the UI (users can copy the values).
Currently users must manually paste into Radarr/Sonarr settings.

Implementation: Arr API has `PUT /api/v3/config/naming` which accepts the full naming
config. Add an "Apply to Instance" button next to each naming field — one click to push
TRaSH's recommended naming to the selected instance.

Fields: Standard Movie/Series format, Movie/Series folder format, Season folder format,
Multi-episode style, Colon replacement.

---

### Instance Backup & Restore
**Priority:** High | **Status:** Profile download works, needs expansion

Full backup/restore of instance configurations with smart CF inclusion.

**Backup modes:**
- **Profiles only:** Select which quality profiles to include in the backup
- **CFs only:** Select individual CFs or "Select All"
- **Profiles + CFs:** Select profiles → Clonarr automatically determines which CFs are
  referenced by those profiles (via scores and CF groups) and includes only those CFs.
  No orphaned CFs in the backup.

**Backup format:** JSON file containing profile definitions + CF definitions + scores.

**Restore workflow:**
1. Upload backup JSON file
2. Select target instance
3. Preview: shows what will be created vs updated (like sync dry-run)
4. Apply with confirmation

Use cases: migrate between instances, recover after accidental cleanup, duplicate
setup across Radarr HD and Radarr 4K.

---

### TRaSH Change Log
**Priority:** Medium | **Status:** Not started

Show what changed in TRaSH Guides data between pulls. Since auto-sync already handles
the actual syncing (users have opted in to TRaSH updates), this is purely informational —
no accept/reject step needed.

**Implementation:** Store a snapshot of TRaSH CF data before each pull. After pull,
diff against the new data and store the changes. Display in a "Change Log" tab or
section (can be enabled in Settings).

**What to show:**
- New CFs added (name, category)
- CFs updated (which specs changed, regex diffs)
- Score changes (old → new per profile context)
- CFs removed (rare but possible)
- Summary: "3 CFs updated, 1 new CF, 2 score changes"

**Not planned:** Selective sync (accept/reject individual changes). Auto-sync means
the user trusts TRaSH. If they need granular control, they should use manual sync
instead of auto-sync.

---

### Multi-Instance Sync
**Priority:** Low | **Status:** Not started

Sync one profile to multiple instances in a single action. Example: push "WEB-2160p"
to both Radarr HD and Radarr 4K at once instead of opening the sync modal twice.

Lower priority because auto-sync already handles this — each instance gets its own
rule and syncs independently. This would mainly be a convenience for initial setup.

---

### Developer Mode — CF Editor & Scoring Simulator
**Priority:** Low | **Status:** Not started (devMode flag exists in config)

For TRaSH team members and advanced profile builders. Two main features:

**CF Editor (developer context):**
- Edit existing TRaSH CF definitions (specs, regex, scores) in a sandbox
- Test changes without pushing to a live Radarr/Sonarr instance
- Export as TRaSH JSON for GitHub PRs to contribute back to TRaSH Guides
- Create/edit SQPs and export as structured data

**Scoring Simulator:**
- Enter a release name (e.g. `Movie.2024.2160p.MA.WEB-DL.DV-FLUX`)
- See which CFs match, individual scores, and total score
- "What if" mode: toggle CFs on/off and see impact instantly
- Test case library: save release names, track score changes over time
- Regression testing: detect when a CF edit changes results for saved test cases
- Replicates Radarr/Sonarr's internal scoring logic (spec matching → score sum)

---

## Scoring Simulator Design

The simulator would replicate how Radarr/Sonarr scores releases internally:

### How Radarr Scores Releases
1. For each release candidate, Radarr runs all CF specifications against the release name
2. If all required specs match (AND logic) + at least one non-required (OR logic) → CF matches
3. The CF's score (from the quality profile) is added to the total
4. Release with highest total score wins (among those meeting quality cutoff)

### Simulator Features
```
┌─ Test Release ─────────────────────────────────────────┐
│ Movie.2024.2160p.MA.WEB-DL.TrueHD.Atmos.7.1.DV-FLUX  │
└────────────────────────────────────────────────────────┘

Matching CFs:                          Score
├── [✓] WEB Tier 01 (FLUX)           +1700
├── [✓] TrueHD Atmos                 + 500
├── [✓] DV HDR10+                    + 400
├── [✓] MA WEB-DL                    + 200
└── Total:                            +2800

Non-matching:
├── [✗] UHD Bluray Tier 01           (not bluray)
├── [✗] x265                         (no x265 tag)
└── ...
```

### Test Case Library
- Predefined test cases covering common release patterns
- User can add custom filenames
- History preserved — shows score delta when CFs/scores change
- "What if" mode — toggle a CF on/off and see impact on all test cases

---

## Potential Challenges

1. **SQP definition source** — Derive from `trash_scores` context keys in CF JSON files.
   No dependency on Recyclarr templates (may become outdated/unmaintained).
2. **CF name collisions** — User may have manually created CFs with same names
3. **Partial sync** — User wants some CFs from TRaSH, some custom, some from Recyclarr
4. **API rate limits** — Pushing 50+ CFs + profile updates in rapid succession
5. **Version tracking** — Detecting when TRaSH updates a CF and showing diff
6. **Sonarr v4 vs v3** — API differences between versions
7. **Scoring simulation accuracy** — Must exactly match Radarr/Sonarr's internal logic
8. **Multi-language CFs** — German, French etc. have their own CF sets and scoring contexts

---

## Name Alternatives Considered

- Clonarr — clone/sync-focused, *arr naming convention (chosen)
- Formatarr — CF-focused, but narrow
- Guidarr — guide-focused
- TRaSHarr — too close to TRaSH brand

**Final name:** Clonarr

---

## Planned Export Enhancements

Current state: Export works on individual imported/custom profiles only.

### Priority 1: Export on TRaSH profiles
- When viewing a TRaSH profile detail (with selected optional CFs), allow export as Recyclarr YAML/JSON
- Build an ImportedProfile-equivalent object from TRaSH profile data + selected optional CFs
- The Export button already exists in profile detail view — just needs to populate `exportSource`

### Priority 2: Export All for imported profiles
- One button per app (Radarr/Sonarr) that exports all imported/custom profiles as a single Recyclarr YAML
- Shared CFs are deduped, different scores grouped per profile
- Useful for backup and migration between instances

### Priority 3: Export All per TRaSH profile group
- Button on each TRaSH group header (e.g. "Export All SQP profiles")
- Generates one Recyclarr YAML with all profiles from that group
- More complex due to score dedup across profiles with different score sets

### Not planned: Custom profile categories
- Custom folders/categories for organizing imported profiles adds UI complexity for little benefit
- With 2-5 profiles per app the flat list is fine
- Revisit if Phase 3 (multi-instance sync) creates a need for grouping by purpose
