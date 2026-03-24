# Clonarr — TRaSH Guides Sync with Web UI

Formerly known as Syncarr, then Profilarr. Renamed progression: Profilarr -> Syncarr -> Clonarr.

**GitHub:** https://github.com/ProphetSe7en/clonarr (public)
**Docker image:** `clonarr:latest` (Alpine 3.21, multi-stage build, healthcheck)
**Port:** 6060, PUID/PGID support
**Config:** `/config/clonarr.json` (instances, trash repo settings, sync history)
**Profiles:** `/config/profiles/*.json` (per-file, migrated from old in-config array)
**Data:** `/data/trash-guides/` (git clone of TRaSH-Guides/Guides)

---

## Status

**v1.2.0-beta** tagged and pushed to GitHub. GHCR builds `latest` + `v1.2.0-beta`. Running locally from `clonarr:latest`.

---

## Current State

- Full Go + Alpine.js web app: 17 files, ~10600 lines
- TRaSH repo clone/pull with periodic scheduled pull (configurable interval)
- CF JSON parsing: custom formats, CF groups, quality profiles, quality sizes
- Arr API v3 client: CFs, profiles, quality definitions, languages
- Sync engine: dry-run plan -> apply (CF create/update + profile create/update with scores)
- **Import:** Recyclarr YAML (v7 classic + v8 custom_format_groups with select/exclude/select_all)
- **Export:** Recyclarr YAML (v8 for guide-backed, v7 for custom) + TRaSH JSON (official format) with copy-to-clipboard
- **Per-file profile storage:** `/config/profiles/*.json` with RWMutex, dedup, sanitized filenames
- Web UI: tab navigation (Radarr/Sonarr/Settings), instance management, profile browser, CF groups by category, Arr comparison view, sync modal (create/update), sync history, import/export modals
- Docker image: `clonarr:latest` (Alpine 3.21, multi-stage build, healthcheck)
- Port 6060, PUID/PGID support, config at `/config/clonarr.json`

---

## Architecture

```
TRaSH Guides repo (git clone) -> Go backend parses CF/profile/group JSON
  -> REST API (26+ endpoints) -> Alpine.js SPA
  -> Sync: dry-run plan -> apply (CF create/update + profile create/update)
  -> Import: auto-detect YAML/TRaSH JSON/Clonarr JSON -> per-file JSON
  -> Export: Recyclarr YAML (v8/v7) / TRaSH JSON (copy to clipboard)
  -> Instance backup: download Arr profiles as JSON file
Config: /config/clonarr.json (instances, trash repo settings, sync history)
Profiles: /config/profiles/*.json (per-file, migrated from old in-config array)
Data: /data/trash-guides/ (git clone of TRaSH-Guides/Guides)
```

---

## Key Design Decisions

### Data Source
- **SQP source:** Derive from `trash_scores` context keys in TRaSH CF JSON — NOT from Recyclarr templates
- **TRaSH repo:** `github.com/TRaSH-Guides/Guides` — active, all CF data in `docs/json/{radarr,sonarr}/cf/*.json`

### Tech Stack
- **Go + Alpine.js** (consistent with Constat), port 6060
- **Config file:** `/config/clonarr.json` (instances, trash repo settings, sync history)
- **Data dir:** `/data/trash-guides/` (git clone of TRaSH-Guides/Guides)

### Concurrency Model
- **Snapshot-based concurrent reads (C1):** Pointer swap, old snapshots remain valid
- **Per-instance sync mutex (C5):** Prevents parallel applies creating duplicate CFs
- **Serialized git pulls (C4):** Only one git pull at a time

### CF Matching & Field Format
- **CF matching:** By name (same as Recyclarr) — trash_id not stored in Arr
- **Field format conversion:** TRaSH uses `{"value": X}`, Arr uses `[{"name":"value","value":X}]` — auto-converted

### Profile Handling
- **Profile creation:** Resolves quality items/groups/cutoff/language, adds all unused qualities as disallowed (Arr requires complete list)
- **Per-file JSON profiles** (not SQLite) — human-readable, easy backup/restore
- **Imported profiles with `trashProfileId`** use TRaSH detail API for categorized display (Required/Optional/Streaming groups)

### Import/Export
- **v8 group resolution:** Handler builds trashData -> parser -> resolveCustomFormatGroups()
- **Two export formats:** TRaSH JSON (official `name->trash_id`), Recyclarr YAML (v8 for guide-backed with `custom_format_groups`, v7 for custom with explicit scores)
- **Import auto-detection:** `{` prefix -> JSON path (probe for `trash_id` vs `id`+`appType`), else -> YAML
- **Instance backup:** Selective profile/CF backup + CF-only mode, selective restore with checkboxes
- **Import from Instance in builder:** Applies directly without saving (user saves via Create Profile)

### Sync Engine
- **Imported/custom profiles can sync:** `resolveScore()` checks cfScoreOverrides before TRaSH TrashScores
- **Score=0 CFs from Arr API:** Cannot distinguish info CFs from unrelated — non-zero only filter for instance import
- **Atomic config updates** via `configStore.Update(fn func(*Config))`
- **Category-based CF groups** parsed from `[Prefix]` in group names

### Security
- **API key masking** in all responses (M9/M10/M11), preserved on update if unchanged
- **Git flag injection prevention (C3):** Branch names validated

---

## Last Worked On (2026-03-24)

### v1.2.0-beta — Sync view refactored to TRaSH groups, sync engine fixes

**Sync/Detail View (major refactor):**
- **TRaSH group-based layout** — Replaced custom category grouping with flat list of TRaSH CF groups (matches Notifiarr's approach)
- **Profile section** — formatItems (CFs not in any group) shown as compact multi-column list (1/2/3 cols based on count)
- **Group toggles** — All groups have toggle to include/exclude from sync. Required CFs within groups shown with 🔒 icon.
- **"All" toggle** — Optional groups with 3+ CFs get bulk toggle (hidden when group is off)
- **Group descriptions** — TRaSH descriptions visible when expanded, bold text in amber for warnings
- **Sync engine fix** — Group toggles now actually affect dry-run/sync (required CFs from disabled groups excluded)
- **`getSelectedCFIds()` rewritten** — Uses Set for O(1) lookups, properly includes required CFs from active groups only

**Cutoff Override:**
- Dropdown with allowed quality items from profile (replaces toggle)
- "TRaSH default" + alternative cutoffs + "Don't sync cutoff" option
- Custom cutoff values now correctly sent to backend (was broken before)

**Profile Builder improvements:**
- formatItems in multi-column compact layout with toggles + scores
- "Add more CFs" with search field (filters live, selected CFs hidden)
- "Clear All" button above selected CFs
- `x-if` for reliable show/hide after template apply

**Code cleanup:**
- Removed dead code: `buildCFCategoryLookup`, `CategorizeResolvedCFs`, `getSelectedOptionalCount`
- `ProfileDetailData()` — new backend function for sync view (flat TRaSH groups + formatItems not in groups)
- Nil slices initialized as empty (JSON `[]` not `null`)
- `initSelectedCFs` — only sets optional CFs individually, required CFs handled by group state
- `getSyncOptionalBreakdown` and `countAllCategoryCFs` migrated to trashGroups with legacy fallback

**CI/CD:**
- GitHub Actions pinned to commit SHAs (supply chain security)
- Removed redundant lowercase image name step
- Updated all actions to latest versions

### Previous (2026-03-23)

### v1.1.0-beta — Profile Builder refactored to TRaSH group system

**Profile Builder (major refactor):**
- **Group-based model** — Replaced per-CF Req/Opt/Opt★ with TRaSH's group system: formatItems (mandatory) + CF groups (optional)
- **Three-state CF pills** — Req (green, required in group), Opt (yellow, optional in group), Fmt (blue, in formatItems). Clickable to change state.
- **Group-level state** — Req/Opt/Fmt pills in group headers to set all CFs at once
- **Separate group cards** — Each TRaSH CF group is its own card (not nested under categories)
- **Golden Rule fix** — Only selected variant enabled (HD or UHD), not both
- **Miscellaneous dropdown** — Enables matching variant group (Standard/SQP)
- **Template loading** — Only default-enabled groups auto-enabled; non-default available but off
- **Fmt auto-disable** — Setting all CFs to Fmt automatically disables the group toggle
- **Sync-accurate save** — FormatItems only contains formatItemCFs + enabled group CFs (~55 CFs, matches TRaSH sync)
- **TRaSH JSON export** — Strict format: formatItems only, no cfGroupIncludes, compact quality items (inline arrays, single-line entries)
- **Group includes export** — Optional checkbox shows `quality_profiles.include` snippets per enabled group
- **Edit/restore** — formatItemCFs, enabledGroups, cfStateOverrides persisted via Go backend

**Backend:**
- `CategorizedCF` — Added `Required` bool and `CFDefault` *bool from TRaSH group data
- `CFPickerGroup` — Added `GroupTrashID` string
- `ImportedProfile` — Added `FormatItemCFs`, `EnabledGroups`, `CfStateOverrides`, `VariantGoldenRule`, `VariantMisc`

**File Naming redesign:**
- Media server tabs (Standard / Plex / Emby / Jellyfin) instead of one long list
- Plex Edition Tags toggle (single-entry mode)
- Instance selector in card above tabs
- Movie File Format before Movie Folder Format
- "Movie:" examples under each pattern
- Combined info boxes (Why naming + IMDb vs TMDb)
- Naming sync renamed "Apply" → "Sync"

**UI improvements:**
- Profile groups with colored borders (SQP blue, Standard green, Anime purple, French yellow, German orange)
- CF category colors for all TRaSH categories (Anime, French, German, Language)
- Profile count badges, CF count badges
- Dropdown sizes unified in Profile Builder
- Profile Name moved into Profile Settings card
- Score Set "Default" capitalized
- Quality Size instance selector redesigned
- Demped pattern colors in File Naming (green `#9ecbaa`, gray instance naming)

**Spec document:** `docs/profile-builder-spec.md` — Complete specification for Profile Builder matching TRaSH's group system

### Previous (2026-03-22)

- **Three-state CF mode** — Req (lilla, locked at sync), Opt ★ (blå, optional default on), Opt (oransje, optional default off). Cycle with pill click.
- **Quality Preset Builder** — Inline quality editor: checkbox list with reorder, Edit Groups mode, cutoff dropdown, start from TRaSH preset or blank
- **CF group variant selectors** — Dropdowns for Golden Rule (HD/UHD) and Miscellaneous (Standard/SQP), auto-set CFs
- **Golden Rule auto-management** — Dropdown sets both CFs (default on/off based on TRaSH), toggles locked in builder, detail shows exclusive pick-one
- **Template-based group filtering** — TRaSH template hides irrelevant CF groups via quality_profiles.include
- **Req/Opt detail view** — Custom profiles show Required (locked) + Golden Rule (pick one) + Optional (toggleable) sections with TRaSH-style colors
- **TRaSH JSON export** — formatItems = required only, strict TRaSH format (no cfGroupIncludes), compact quality items formatting
- **Quality items fix** — Cached from all import paths, confirm on missing, reversed order for TRaSH format
- **Sticky defaults** — Builder remembers last quality preset, golden rule, misc, score set via localStorage
- **Delete All** — Custom confirm modal for bulk delete of imported profiles
- **Duplicate name handling** — Auto-suffix "(2)" with preview, both builder and import
- **Import collision** — Backend auto-renames with suffix, shows notification
- **Custom confirm modals** — Replaces browser confirm() for quality items, duplicate names
- **Category Req/Opt toggle** — Selects all + sets Required, or switches all to Optional
- **formatGroups saved** — CF group membership stored in profiles for Golden Rule detection
- **UI improvements** — Section label colors, pill colors (lilla/blå/oransje), dropdown sizing
- **Quality Size UI** — Auto/Custom mode buttons with tooltips, improved headers
- **Getting Started guide** — 9 screenshots, covers instances, profiles, quality sizes, file naming
- **Single volume mount** — Removed /data, TRaSH repo now in /config/data/
- **Sparse-checkout** — 72 MB → 3 MB for TRaSH repo clone
- **Version footer, app icon, Unraid template**

### Previous (2026-03-17)

- Sync behavior rules, profile settings redesign, CF category unification, TRaSH changelog dropdown, Discord notifications, dynamic languages, UUID fix, cutoff override

---

## Remaining Phases

- **Phase 3:** Multi-instance sync (same profile to multiple instances) — low priority, convenience only
- **Phase 5:** TRaSH update tracking with diff view, selective sync per-CF
- **Phase 7b:** Profile comparison — side-by-side scoring of same release against two profiles
- **Phase 8:** Multi-repo support — pull from multiple guide repos (TRaSH, Recyclarr, Syncarr/Notifiarr 1000+ users, community repos). TRaSH asked about "public profiles" support — confirm what format/location they mean.

Phase priority summary: 1-2 done, 3 (multi-instance), 4 (custom CFs — done), 5 (update diffs), 6 (scoring simulator — done as Phase 7), 7b (profile comparison → now part of sandbox redesign), 8 (multi-repo).

---

## Scoring Sandbox Redesign

Current sandbox shows one tall card per release (~200px each), unusable with more than a few results. TRaSH team tests with 150+ release titles. Redesign planned in 4 phases:

### Phase S1: Compact Table + Batch Fix (Medium, ~210 lines)
- Replace cards with table rows (~36px each) — 5-6x more visible at once
- Columns: Release Title | Quality | Group | Score | Status | X
- Click row to expand CF breakdown, hover score for tooltip
- Keep cards as optional fallback (`viewMode: 'table'|'cards'`)
- Fix batch parse limit (currently 15, chunk requests for 150+ titles)
- No backend changes needed. Rewrite results HTML (lines 1608-1690)

### Phase S2: Column Sorting (Easy, ~70 lines)
- Clickable column headers with arrow indicators
- Sort by: Score (default desc), Status, Quality, Group, Title
- `sortedSandboxResults()` computed getter, remove-button uses `indexOf(res)`
- Depends on S1

### Phase S3: Quick Edit Panel (Hard, ~200 lines)
- 300px collapsible side panel with all profile CFs
- Toggle (on/off) + editable score per CF, Min Score override
- Changes are temporary (sandbox only), live re-scoring with 200ms debounce
- "Reset to Profile" button, optional "Save as Imported Profile"
- Panel clears on profile switch. No backend changes.
- Depends on S1

### Phase S4: Profile Comparison (Hard, ~180 lines)
- Second "Compare with" dropdown, extra columns: Score B + Diff
- Both profiles scored against each result
- Expanded row shows two-column CF breakdown:
  - Green: matches in both profiles
  - Blue: only profile A
  - Orange: only profile B
- Diff column sortable. Profile score cache reused.
- Depends on S1, S2

### Design principles
- Follow Clonarr's own visual style (not Radarr-clone)
- Minimalist: max info per pixel, details on-demand (expand/hover)
- Each phase independently shippable and useful

---

## Next Steps

1. **Profile Builder documentation** — Detailed guide for profile creator with all features (req/opt/opt★, golden rule, quality editor, export)
2. **TRaSH feedback** — VRV/VDL trash_id bug reported. Awaiting feedback on profile builder workflow.
3. **TRaSH update tracking** — Diff view per CF/profil ved oppdateringer, selektiv sync (Phase 5)
4. Debug logging system — toggle in Settings, writes to /config/debug.log (plan ready)

---

## GitHub Workflow

**Repository:** `git@github.com:ProphetSe7en/clonarr.git`
**Git config:** name=ProphetSe7en, email=ProphetSe7en@users.noreply.github.com

**Push process:** Clone -> rsync files (exclude .git, node_modules, config files) -> commit -> push -> delete temp dir. SSH via `git@github.com:ProphetSe7en/clonarr.git`.

**Build workflow:** Build locally (`docker build -t clonarr:latest containers/clonarr/`) -> test on Unraid -> when verified, push to GitHub.

**Rule:** Always build and test locally before pushing to GitHub.

---

## Files

**Source code:** `containers/clonarr/ui/`
- main.go
- config.go
- trash.go
- arr.go
- sync.go
- autosync.go
- handlers.go
- import.go
- profilestore.go
- customcf.go
- prowlarr.go
- static/index.html

**Container:** `containers/clonarr/`
- Dockerfile
- entrypoint.sh

---

## Documentation

All docs are in `containers/clonarr/` unless otherwise noted:

| File | Description |
|------|-------------|
| `README.md` | Main readme |
| `DESIGN_NOTES.md` | Architecture and design notes |
| `docs/DEVELOPER-MODE.md` | Developer mode documentation |
| `docs/AUTO-SYNC-DESIGN.md` | Auto-sync design document |
| `clonarr-test-plan.md` | Backend/API test plan |
| `clonarr-ui-test-plan.md` | UI test plan (120 test cases) |
| `clonarr-cf-create-design.md` | Custom format creation design |
| `clonarr-scoring-sandbox-design.md` | Scoring sandbox design |

---

## Summary Context (from "Where We Are")

Clonarr v0.8.0 is a TRaSH Guides sync tool with web UI (Go + Alpine.js, 17 files, ~10600 lines). Scoring Sandbox was live-tested and debugged: server-side score resolution, search filters (resolution pills + text), cancel search, persistent results (localStorage), individual result removal. Sync engine was hardened: profile-level settings (min scores, cutoff, language, upgrade) now applied during update sync, quality ordering fixed, MinUpgradeFormatScore >= 1, language defaults to Original. Cleanup modals styled properly. 20 commits in the 2026-03-13b session. Next: full end-to-end testing.

Recyclarr v8 opt-in groups need `select:` — Multi-CF groups like [Optional] Miscellaneous silently create 0 CFs without `select:` listing individual trash_ids.
