# Changelog

## v1.7.2-beta

### Features
- **Add Extra CFs** — Add any TRaSH CF to a profile via Customize overrides. CFs organized in real TRaSH groups with collapsible headers, toggles, and search. Default scores from profile's score set.
- **Quality overrides redesign** — Dynamic columns, toggle switches, amber override indicator.
- **UI polish** — Column layout for Profile section, toggle switches for override panel, number input spinners removed globally.

### Bug fixes
- **quickSync fallback for importedProfileId** — Pre-v1.7.1 sync history entries now check auto-sync rule as fallback, preventing builder profiles from zeroing on upgrade.
- **Extra CFs persisted** — Restored on resync, included in auto-sync rules and quickSync.
- **Extra CF browser wrong type** — Reset on profile switch to prevent showing radarr CFs for sonarr.
- **Resync loads grouped browser** — extraCFGroups populated after resync (was empty).
- **Reset to TRaSH clears Extra CFs** — Toggle, search, and selections all cleared.

## v1.7.1-beta

### Features
- **Per-CF score overrides on ALL CFs** — Score overrides now work on required CFs and core formatItems, not just optional. Enables overriding scores on CFs like Anime Dual Audio while keeping everything else synced with TRaSH.
- **Create New button** — Duplicate a synced profile as a new Arr profile with different settings. Available on both TRaSH and builder profiles.
- **Builder badge in Sync Rules** — Blue "Builder" tag identifies profiles from Profile Builder.
- **Info banner for builder edits** — Warning when editing builder profiles from Sync Rules that changes affect the profile itself.
- **Sync behavior in create mode** — Add/Scores dropdowns with dynamic descriptions.
- **Edit/Sync/Sync All** — Sync Rules buttons for quick actions with toast result summaries.
- **Custom CF amber grouping** — Custom CFs in dedicated amber-styled category.
- **Toast notifications** — Centered, progress bar, multiline for Sync All breakdown.
- **Profile group sorting** — Standard → Anime → French → German → SQP.

### Bug fixes
- **Builder profile resync zeroed scores** — Resync/quickSync from TRaSH Sync tab fell back to TRaSH base profile instead of imported profile. Now correctly sends importedProfileId.
- **Edit from Sync Rules opened wrong view** — Builder profiles now open in builder editor with correct values.
- **Dry-run/apply reset to TRaSH profile** — After dry-run on imported profiles, code opened TRaSH base profile detail, losing all builder settings.
- **Instance data survives delete+recreate** — Orphan migration now checks instance type to prevent cross-type contamination.
- **Multi-instance support** — Builder sync functions find correct instance from sync history instead of assuming first.
- **API key field appeared empty** — Edit mode shows "Leave empty to keep current key".
- **Stale _resyncReturnSubTab** — Cleared on manual tab switch to prevent stale navigation state.
- **History matching for imported profiles** — Also checks importedProfileId for profiles without trashProfileId.
- **Prowlarr test connection** — Fixed "authentication failed (HTTP 401)" when testing Prowlarr after page refresh.

### Refactoring
- **Generic FileStore[T]** — profileStore 239→14 lines, customCFStore 248→76 lines.
- **Handler helpers** — decodeJSON/requireInstance reduce boilerplate across 10+ handlers.
- **22 unit tests** — sync behavior, field conversion, score resolution, FileStore.

## v1.7.0-beta

### Features
- **Per-CF score overrides** — Override individual CF scores in sync mode. Enable "CF scores" in Customize overrides to edit scores on optional CFs. Overrides persist through auto-sync and resync.
- **Edit/Sync/Sync All buttons** — Sync Rules now has Edit (open profile), Sync (one-click resync), and Sync All (resync all profiles on instance) with toast result summary.
- **Custom CF amber grouping** — Custom CFs displayed in a dedicated amber-styled "Custom" category in CF browser.
- **Sync behavior in create mode** — Add and Scores dropdowns now visible when creating new profiles. Dynamic descriptions explain each option.
- **Profile group sorting** — Standard → Anime → French → German → SQP. New TRaSH groups appear before SQP.
- **Toast notifications** — Centered top, progress bar, auto-dismiss. Used for sync results, cleanup events, and errors.
- **Auto-sync rule on every sync** — Syncing a profile always creates an auto-sync rule (disabled by default). Toggle on/off directly from Sync Rules.
- **Multiple profiles from same TRaSH source** — Same TRaSH profile synced to multiple Arr profiles with different overrides and CF selections.
- **Discord cleanup notifications** — Amber embed when synced profiles are auto-removed because the Arr profile was deleted.
- **Friendly connection errors** — User-friendly messages instead of raw TCP errors in Discord and Settings.
- **Instance data survives delete+recreate** — Sync history and rules preserved when instance is removed and re-added.

### Refactoring
- **Generic FileStore[T]** — Replaced duplicated CRUD in profileStore (239→14 lines) and customCFStore (248→76 lines).
- **Handler helpers** — `decodeJSON` and `requireInstance` reduce boilerplate across 10+ handlers.
- **22 unit tests** — Coverage for sync behavior, field conversion, score resolution, and FileStore operations.

### Bug fixes
- **Cutoff error on resync** — Cutoff resolved against stale quality items. Now resolved after rebuild.
- **Min Score / overrides not syncing** — Overrides not applied in create mode, not saved in auto-sync rules, not sent when only profile settings changed.
- **Resync didn't restore settings** — Optional CFs, overrides, behavior, target profile, and score overrides now fully restored.
- **SnapshotAppData missing Naming deep-copy** — Shared pointer could cause data corruption on concurrent access.
- **Custom CF field format** — TRaSH `{"value":X}` now converted to Arr array format on write, preventing HTTP 400 errors.
- **Deleted auto-sync rule still running** — Race condition fix with fresh config re-check before execution.
- **Same TRaSH profile overwrote sync history** — Rekeyed from trashProfileId to arrProfileId throughout.
- **Stale sync history after profile deletion** — Auto-cleaned on pull, page load, with Discord notification.
- **Create mode contaminated existing profile** — syncForm.arrProfileId now reset when switching to create mode.
- **Keep List search, File Naming feedback, confirm modals** — Various UI fixes from user reports.
- **Connection errors spammed Discord** — Friendly message, only on startup or new TRaSH changes.
- **API key field appeared empty on edit** — Now shows "Leave empty to keep current key".

## v1.6.1-beta

(Superseded by v1.7.0-beta — not released separately)

## v1.6.0-beta

### Features
- **Quality items sync** — Auto-sync now detects and updates quality item changes (allowed/disallowed qualities). Previously only CFs and scores were synced.
- **Detailed Discord notifications** — Auto-sync notifications now show exactly what changed: CF names created/updated, score changes (old → new), and quality item changes (Enabled → Disabled)
- **Startup auto-repair** — On container start, resets auto-sync commit hashes (ensures all rules re-evaluate) and removes broken rules with arrProfileId=0

### Bug fixes
- **Quality items not applied** — Quality item rebuild was running before the `updated` flag, so changes were never sent to Arr
- **Quality items reversed** — Update mode now correctly reverses item order to match Arr API expectations (same as create mode)
- **Spurious quality notification** — "Quality items updated" no longer shown when nothing actually changed

## v1.5.0-beta

### Features
- **Debug logging** — Enable in Settings to write detailed operations to `/config/debug.log`. Logs sync, compare, auto-sync, and UI actions. Download button for easy sharing when reporting issues.
- **Compare: sync history awareness** — Compare uses Clonarr sync history to accurately identify which score-0 CFs were deliberately synced vs unused defaults. Works best with profiles synced via Clonarr.
- **Auto-sync per-profile toggle** — Enable/disable auto-sync individually for each profile directly from Sync Rules & History. Global toggle removed from Settings.
- **Auto-sync error visibility** — Failed auto-sync rules show error badge with tooltip in Sync Rules

### Improvements
- **Settings: auto-sync clarification** — Description explains that auto-sync triggers on TRaSH pull changes, not on a fixed schedule
- **Settings: active rules moved** — Auto-sync rules managed under Profiles → TRaSH Sync instead of Settings
- **Compare: info note** — Visible warning about score-0 limitations for profiles not synced via Clonarr

### Bug fixes
- **Compare: score-0 CFs** — CFs synced with score 0 via Clonarr now correctly shown as "in use"
- **Sync: case-insensitive BuildArrProfile** — Score assignment no longer fails for mixed-case CF names

## v1.4.0-beta

### Features
- **Profiles tab reorganized** — Three sub-tabs: TRaSH Sync, Profile Builder, and Compare
- **Compare Profiles redesigned** — Uses TRaSH CF groups with per-group status badges, only flags actual errors (wrong scores on active CFs, missing required CFs)
- **Compare: auto-sync from Compare** — Sync fixes and enable auto-sync directly from comparison results
- **Auto-select instance** — When only one instance per type exists, automatically selected across all functions
- **Auto-sync rule auto-update** — Existing auto-sync rules automatically updated with new selections when you re-sync

### Improvements
- **Compare: smart verification** — Optional CFs with score 0 are not flagged as errors, exclusive groups (Golden Rule, SDR) verified correctly
- **Compare: "Extra in Arr"** — CFs not in the TRaSH profile shown with removal option
- **Sync Rules & History** — Visible in TRaSH Sync tab with auto-sync badges and re-sync/remove buttons
- **Profile Builder** — Moved to dedicated tab with description and prominent Create/Import buttons
- **Consistent status display** — All instance selectors show Connected/Failed/Not tested uniformly
- **Descriptions** — Added tab descriptions for TRaSH Sync, Profile Builder, and Compare

### Bug fixes
- **Compare: HTML rendering** — TRaSH descriptions now render HTML correctly (was showing raw tags)
- **Compare: category colors** — Group cards show colored left borders matching TRaSH categories
- **Maintenance cleaned up** — Only Cleanup and Backup/Restore remain (Compare moved to Profiles)

## v1.3.0-beta

### Features
- **TRaSH JSON export sort order** — Matches TRaSH convention (grouped CFs by score, Tiers, Repack, Unwanted, Resolution)
- **Case-insensitive CF matching** — Handles name mismatches like HULU/Hulu across sync, compare, and single-CF operations
- **Builder: formatItems group display** — CFs in formatItems shown in their TRaSH group with Fmt state (e.g. Audio in SQP-3 Audio)
- **Variant dropdowns with templates** — Golden Rule and Misc variants auto-detected and visible when loading templates

### Bug fixes
- **syncSingleCF updates CF specs** — Not just score, also corrects name and specifications
- **pdHasOverrides tautology** — Copy-paste error causing override banner to always show
- **SelectedCFs deep copy** — Fixed concurrency bug in config store
- **Resync restore** — Correctly sets deselected CFs to false (not just selected to true)
- **Resync loads sync history** — Synced Profiles section now appears immediately in Maintenance

## v1.2.0-beta

### Features
- **Sync view refactored to TRaSH groups** — Replaced custom category grouping with TRaSH CF groups (matches Notifiarr's approach)
- **Group toggles** — Include/exclude groups from sync, required CFs shown with lock icon
- **"All" toggle** — Bulk toggle for optional groups with 3+ CFs
- **Group descriptions** — TRaSH descriptions visible when expanded, bold amber warnings
- **Cutoff override dropdown** — Select from allowed quality items, TRaSH default, or "Don't sync cutoff"
- **Profile Builder: "Add more CFs"** — Search field with live filtering and "Clear All" button
- **Instance connection status** — Quality Size, File Naming, Maintenance tabs show actual connection status
- **Tab persistence** — Last selected tab saved to localStorage
- **Resync from Maintenance** — Opens profile detail with previously synced optional CFs restored from sync history

### Bug fixes
- **Sync engine fix** — Group toggles now actually affect dry-run/sync (required CFs from disabled groups properly excluded)
- **Custom cutoff values** — Now correctly sent to backend (was broken before)
- **CI hardening** — GitHub Actions pinned to commit SHAs, removed redundant lowercase step

## v1.1.0-beta

### Features
- **Profile Builder refactored to TRaSH group system** — Group-based model replacing per-CF Req/Opt/Opt★ categories
- **Three-state CF pills** — Req (green), Opt (yellow), Fmt (blue) with click-to-cycle
- **Group-level state controls** — Set all CFs in a group at once via header pills
- **Golden Rule fix** — Only selected variant enabled (HD or UHD), not both
- **TRaSH JSON export** — Strict format matching TRaSH sync expectations
- **Group includes export** — Optional checkbox shows `quality_profiles.include` snippets
- **File Naming redesign** — Media server tabs (Standard/Plex/Emby/Jellyfin), instance selector, combined info boxes
- **Profile Builder spec** — Complete specification document for the group system

## v1.0.0-beta

### Features
- **Profile sync** — Sync quality profiles from TRaSH Guides to Radarr/Sonarr instances
- **Profile Builder** — Create custom quality profiles with CF selection and scoring
- **Quality Size sync** — Sync quality size limits from TRaSH Guides
- **File Naming sync** — Apply TRaSH recommended naming conventions
- **Multi-instance support** — Manage multiple Radarr/Sonarr instances
- **Custom CFs** — Create and manage custom format definitions
- **Maintenance tab** — View synced profiles, resync, and manage sync history
- **API key security** — Keys masked in all API responses, git flag injection prevention
- **Docker-native** — Go + Alpine.js, port 6060, Alpine-based
