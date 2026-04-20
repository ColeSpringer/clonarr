# Changelog

## v2.0.8

### Changed

- **Architecture Improvements**: Complete refactoring of backend packages from the root `ui/` directory to standard Go layout `internal/api/`, `internal/core/`, `internal/auth/`, and `internal/netsec/`.
- **Background Panic Recovery**: Standardized asynchronous routines using `utils.SafeGo` to prevent complete application crashes from unexpected panics.

## v2.0.7

### Fixed

- **Golden Rule (and other exclusive CF groups) can now be disabled at the group level.** Previously, groups that TRaSH marks with a "pick one" exclusivity hint in their description (like `[Required] Golden Rule UHD`) had their group-level toggle hidden in the profile detail / TRaSH-sync view — users had no way to say "I don't want this group at all", only "enable / disable each CF individually". That was inconsistent with how equivalent optional groups (HDR Formats, Optional Movie Versions, Audio Formats) behave, and stricter than what TRaSH's own schema supports (both Golden Rule CFs are `required: false`). The group toggle is now shown for every group including exclusive ones. Behavior:
  - Group ON + not exclusive → all non-required CFs auto-enabled (unchanged).
  - Group ON + exclusive → no CFs auto-enabled; user picks one via pick-one logic.
  - Group OFF → all CFs in the group cleared regardless.
  - The "only enable one" warning still shows when the group is expanded.

## v2.0.6

**⚠️ Breaking change:** Authentication is now enabled by default (Forms + "Disabled for Trusted Networks", matching the Radarr/Sonarr pattern). On first run after upgrade, Clonarr will redirect to `/setup` to create an admin username and password. Existing sessions are invalidated (cookie name changed from `constat_session` to `clonarr_session` as part of branding cleanup). Homepage widgets and external scripts hitting `/api/*` now need the API key (Settings → Security) — send as `X-Api-Key` header.

### Added

- **Authentication (Radarr/Sonarr pattern)** — `/config/auth.json` stores the bcrypt-hashed password + API key. Three modes:
  - `forms` (default): login page + session cookie, 30-day TTL.
  - `basic`: HTTP Basic behind a reverse proxy.
  - `none`: auth disabled (requires password-confirm to enable — catastrophic blast radius).
- **Authentication Required** — `enabled` (every request needs auth) or `disabled_for_local_addresses` (default — LAN bypasses).
- **Trusted Networks** — user-configurable CIDR list of what counts as "local". Empty = Radarr-parity defaults (10/8, 172.16/12, 192.168/16, link-local, IPv6 ULA, loopback). Narrow the list (`192.168.0.0/24`, `192.168.0.5/32`) for tighter control.
- **Trusted Proxies** — required when Clonarr sits behind a reverse proxy (SWAG, Authelia, etc.) so `X-Forwarded-For` is trusted.
- **Env-var override for trust-boundary config** — set `TRUSTED_NETWORKS` and/or `TRUSTED_PROXIES` in the Unraid template or `docker-compose.yml` to pin the values at host level. When set, the UI shows the field as locked and rejects edits — the trust boundary can only be changed by editing the template and restarting.
- **API key** — auto-generated on first setup, rotatable from the Security panel. Send as `X-Api-Key: <key>` header (preferred) or `?apikey=<key>` query param (legacy — leaks to access logs and browser history). For Homepage widgets, scripts, Uptime Kuma.
- **Change password** — from the Security panel. Requires current password. Invalidates all other sessions.
- **CSRF protection** — double-submit cookie pattern on all state-mutating requests. Transparent to browser users; scripts using the API key bypass (verified key required, not just presence).
- **Security headers** — `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`. Radarr-parity scope.
- **SSRF-safe notification client** — Discord and Pushover (both always external) now use a blocklisted HTTP client that refuses RFC1918/loopback/link-local/ULA/NAT64/CGN/doc-range targets with per-request IP revalidation (defeats DNS rebinding). Gotify stays on a plain client (LAN targets are legitimate for self-hosted Gotify).
- **Webhook and notification secret masking** — Discord webhook URLs, Gotify token, Pushover user key + app token, and Arr instance API keys are masked in API responses. Empty-on-unchanged-edit preserves the stored value on save (so editing unrelated fields doesn't clobber secrets).

### Fixed

- **T64 — live-reload no longer clobbers env-locked trust-boundary fields.** Previously any unrelated config save (session TTL, auth mode) could silently empty the env-derived trusted-networks slice. Now guarded at every call site.
- **T65 — `UpdateConfig` preserves all deployment-level fields.** Previously only `AuthFilePath` was preserved; `SessionsFilePath`, `MaxSessions`, and env-lock state could be silently dropped by a future caller building config from scratch. Defense-in-depth: also force-restores locked values from the internal state.
- **T66 — data races eliminated from `Middleware` / `TrustedProxies()` / `IsRequestFromTrustedProxy()`.** Config snapshot taken via `RLock` at the top; all downstream reads use the local value. Passes `go test -race`.

### Changed

- **Cookie rename** — `constat_csrf` → `clonarr_csrf`, `constat_session` → `clonarr_session`. Avoids browser-scope collision when both apps sit behind the same parent domain. Existing sessions won't survive the upgrade.
- **Basic realm** — `WWW-Authenticate: Basic realm="Clonarr"` (was `"Constat"` from initial port).
- **Setup page footer** — GitHub link points to `prophetse7en/clonarr` (was `/constat`).

### Security

- First-run forces the `/setup` wizard — no default credentials.
- bcrypt cost 12; password verify is timing-equalized (prevents user-enumeration via response timing).
- Session persistence via atomic write to `/config/sessions.json` (survives container restart).
- CIDR min-mask enforced (`/8` IPv4, `/16` IPv6) to reject mis-typed host bits masking as subnets.
- See `docs/security-implementation-baseline.md` in the repo for the full trap catalogue (T1–T66) behind the implementation.

### Notes for upgraders

- First boot redirects to `/setup`. Choose a strong password (≥10 chars, 2+ of upper/lower/digit/symbol).
- If you access Clonarr from the same LAN the host is on, the default "Disabled for Trusted Networks" mode will skip login for you — no change in day-to-day UX.
- Homepage / Uptime Kuma: use the API key from Security panel, send as `X-Api-Key` header.
- Lost your password: stop the container, delete `/config/auth.json` (credentials only — no profile data), restart. The setup wizard will run again.

## v2.0.5

### Fixed

- **Extra CFs showed hex IDs instead of names in Overridden Scores** — Score overrides on Extra CFs (CFs added to a profile but not part of the base TRaSH profile) displayed their trash ID (e.g. `82cc7396f79a`) instead of the CF name after Save & Sync. The display helpers only looked at CFs belonging to the base profile; they now fall back to the Extra CFs list so the correct name and default score are shown. Sort order in the panel also now uses real names. Same fix covers both TRaSH Extra CFs and user-created custom CFs added as extras.

## v2.0.4

### Fixed

- **Quality Definitions null values** — Sonarr/Radarr "Unlimited" (null) for preferred/max size showed as 0.0. Now uses `*float64` to distinguish null from explicit zero.
- **Sync All score oscillation** — Ring-buffer entries with different selectedCFs caused scores to flip-flop on every Sync All. Now deduplicates to latest entry per profile.
- **CF Editor dropdowns lost on edit** — Language, Resolution, and other select-type specs showed raw numeric values instead of dropdown. Three-part fix: schema matching, string coercion, and programmatic option population (replaces `<template x-for>` inside `<select>`).
- **Cutoff dropdown showing deleted group** — When quality structure override removed the TRaSH default cutoff group, dropdown showed the deleted name. Now auto-picks first allowed quality. Also fixed same `x-for`-in-`select` timing bug.
- **Language dropdown in Edit view** — Same programmatic population fix applied.
- **Custom CF filenames** — Regression from path traversal fix: files saved as `custom:hex.json` instead of readable names. Now uses sanitized CF name. Auto-migrates on startup.
- **GitHub #10** — Unknown quality names (group names without sub-items, cross-type names) now skipped with log warning instead of failing entire sync.
- **pprof debug endpoints removed** — `/debug/pprof/*` endpoints removed from release builds.

### Improved

- **Score Override UX** — Summary panel shows all overridden CFs when toggle is active, editable inline with per-CF ↻ reset button. Override count badge per CF group header.
- **Toggle labels** — "Override" → "Hide Overrides" when active (General, Quality, CF Scores, Extra CFs).
- **Extra CFs layout** — Fixed-width columns (toggle | name 180px | score 65px), sorted A→Z.
- **Keep List redesign** — Side-by-side layout: search + Add/Add all on left, 3-column CF list on right. Batch "Add all (N)" matching, "Remove all" button.
- **Sync Rules default sort** — A→Z by Arr Profile name instead of ring-buffer insertion order.
- **Per-webhook Discord test** — Sync and Updates webhooks each have independent Test buttons.

## v2.0.3

### Added

- **Docker Hub mirror** — Image now published to both GHCR (`ghcr.io/prophetse7en/clonarr`) and Docker Hub (`prophetse7en/clonarr`). Use Docker Hub if your platform can't pull from GHCR (e.g. Synology DSM with older Docker).
- **Per-webhook Discord test buttons** — Sync webhook and Updates webhook each have their own Test button.

## v2.0.2

### Added

- **Pushover notifications** — Third notification provider alongside Discord and Gotify. Collapsible provider sections with status indicators and test buttons. Discord can now be toggled on/off. (Community contribution by @xFlawless11x, PR #12)

### Fixed

- **GHCR pull fails on older Docker clients (Synology, DSM)** — Multi-arch builds produced OCI image indexes that older Docker versions can't parse. Added `provenance: false` to CI workflow to force Docker manifest list v2 format.

## v2.0.1

### Fixed

- **Dry-run Apply button shows wrong instance** — When selecting a non-default instance in the sync modal's Target Instance dropdown, the dry-run results banner showed "Apply to [default instance]" instead of the selected instance. Now uses `syncPlan.instanceName` from the backend.

## v2.0.0

### Compare — Redesigned

- **Table layout** for Required CFs and CF Groups — current vs TRaSH values side-by-side with checkboxes per row
- **Profile Settings table** — compares Language, Upgrade Allowed, Min/Cutoff/Upgrade scores against TRaSH defaults
- **Filter chips** — All / Only diffs / Wrong score / Missing / Extra / Matching to focus on what matters
- **Golden Rule picker** — auto-selects HD or UHD variant based on what's in use, with cascade logic (inUse → default+required → default → first)
- **Per-card Sync selected** — sync changes per section (Required CFs, each CF Group, Settings) instead of all-or-nothing
- **Toggle all** link per card header for quick select/deselect
- **Score override badges** — blue "OR" badge when a score difference is intentional (from your sync rule overrides)
- **Score-0 extras via sync history** — CFs added via "Add Extra CFs" with score=0 now correctly appear in Compare instead of being silently dropped
- **Exclusive group radio behavior** — "pick one" groups work correctly with proper counting

### Sync History & Rollback — New

- **History tab** between TRaSH Sync and Compare — dedicated change log for all synced profiles
- **Ring-buffer storage** — last 10 change events per profile (no-change syncs only update the timestamp)
- **CF set-diff tracking** — catches all CF changes including score-0 CFs from group enable/disable
- **Detailed change log** — CFs added/removed, scores before→after, quality items toggled, settings changed
- **Sortable columns** — TRaSH Profile, Arr Profile, Last Changed, Events
- **Rollback** — restore a profile to any previous state with one click. Confirmation shows what will be reversed. Auto-disables auto-sync to prevent overwrite
- **Auto-refresh** — History tab updates in real-time after sync operations

### Profile Detail — Redesigned

- **General + Quality cards** with blue/purple stripe design and per-section override toggles
- **Inline Quality Items editor** — expands inside the Quality card (same as Builder) with drag-and-drop grouping
- **Quality card spans full width** when editor is open (prevents CSS column overflow)
- **Override summary bar** — shows active overrides with per-section and "Reset all" controls

### Profile Builder — Redesigned

- **Init card with tabs** — TRaSH template / Instance profile (replaces cluttered "Start from" row)
- **General + Quality cards** matching the Edit view's visual language
- **Golden Rule + Miscellaneous variants** as sub-section inside Quality card
- **Collapsible Advanced Mode** behind devMode flag
- **Shared Quality Items editor** — Builder and Edit view share the same drag-drop editor code (parameterized with target='edit'|'builder')
- **Import from instance improved** — consults sync history for score-0 extras, resolves custom CFs, surfaces all CFs in Required CFs section
- **Button label** — "Editing Items" → "Done" (describes action, not state)

### Settings — Redesigned

- **Sidebar + content panel** layout matching vpn-gateway and PurgeBot
- Six sections: Instances, TRaSH Guides, Prowlarr, Notifications, Display, Advanced
- **Prowlarr gets its own section** (split from Advanced) with custom search categories per app type
- Green left-border active indicator, centered layout (1100px max-width)

### Scoring Sandbox — Improved

- **Custom Prowlarr search categories** — configurable Radarr/Sonarr category IDs for indexers that don't cascade root IDs
- **Numeric release group fallback** — trailing numeric groups like `-126811` now parsed correctly when Arr returns empty
- **Per-row selection + filter** — checkbox per row, "Filter to selected" toggle, "Reset filter"
- **Drag reorder** — manual sorting with drag handles (disabled during filter to prevent confusion)
- **Copy-box modal** — shareable plain-text summary per release (title, parsed metadata, matched CFs, scores)
- **Language CFs stripped** — "Wrong Language" and "Language: *" CFs excluded from scoring (Parse API can't evaluate without TMDB context)
- **Stable drag keys** — `_sid` identity tracking prevents DOM glitches during reorder

### Browser Navigation — New

- **Back/forward works** — `pushState` on every section/tab change, `popstate` listener restores state
- **URL hash routing** — e.g. `#radarr/profiles/compare`, `#settings/prowlarr`, `#sonarr/advanced/scoring`
- **Hash validation** — invalid hashes fall back to defaults (no blank page)
- **Initial entry seeded** — `replaceState` ensures the first Back click has somewhere to go

### Other Improvements

- **Sonarr language** — language field hidden everywhere for Sonarr (removed in Sonarr v4, not in TRaSH Sonarr profiles)
- **Sortable Sync Rules columns** — TRaSH Profile and Arr Profile headers clickable to sort A→Z / Z→A
- **Sync Rules renamed** from "Sync Rules & History" (History has its own tab now)

### Fixed

- **GitHub #10** — "WEB 2160p not found in definitions" when syncing. Quality names not in definitions are now skipped with a log warning instead of failing the entire sync
- **XSS sanitization** — all `x-html` bindings now wrapped in `sanitizeHTML()` (3 were missing)
- **Path traversal** in custom CF create endpoint
- **Shared quality editor state leak** — `qualityStructureEditMode` no longer leaks between Builder and Edit view
- **`pb.qualityItems` identity tracking** — `$watch` auto-assigns stable `_id` on every reassignment
- **Sonarr Language "Unknown" diff** — no longer shows false Language diff in Compare for Sonarr profiles
- **`alert()` → toast** — all browser alerts replaced with toast notifications

### Security

- All `x-html` bindings sanitized via `sanitizeHTML()`
- `GetLatestSyncEntry` returns defensive copy (not pointer into config slice)
- Path traversal prevention in custom CF file operations
- API key masking on all config responses

## v1.9.0

### Added

- **Clone profile** — Clone button on sync history row creates a copy of a synced profile with a new name, including all overrides, quality structure, and behavior settings.
- **Inline rename** — Click the Arr profile name in sync history to rename it directly. Changes are applied to the Arr instance and local sync history. Duplicate name detection prevents accidental overwrites.
- **Dry-run settings/quality preview** — Dry-run now shows settings changes (min score, cutoff, language, upgrade until) and quality item changes (enabled/disabled) — same detail level as the apply result.
- **Arr profile name in Edit header** — When editing a synced profile, the header shows which Arr profile it syncs to (e.g. "Sonarr → WEB-2160p").

### Fixed

- **"Delete CFs & Scores" cleanup now respects Keep List** — Score reset previously zeroed ALL scores across every profile, even for CFs in the Keep List. Now only scores for the actually deleted CFs are reset.
- **Safer cleanup order** — "Delete CFs & Scores" now deletes CFs first, then resets scores. If CF deletion fails partway through, orphaned scores are harmless. Previously scores were zeroed first, which was unrecoverable if CF deletion then failed.
- **Button text invisible in several modals** — Pull, Preview, Apply, Download Backup, and Create/Update Profile buttons appeared as empty green/colored rectangles. Caused by `<template x-if>` inside `<button>`, which browsers handle inconsistently. Replaced with `<span x-show>` across 9 buttons.
- **Cleanup descriptions clarified** — "Delete All CFs" and "Delete All CFs & Scores" descriptions now state "(respects Keep List)" so the relationship with the Keep List above is clear.
- **Auto-sync checkbox in sync modal** — "Auto-sync this profile" checkbox couldn't be unticked after Save & Sync. The binding checked if a rule *existed* rather than if it was *enabled*.
- **Auto-sync rule not updated on profile change** — Changing target Arr profile in sync modal dropdown didn't update the auto-sync rule reference, causing stale checkbox state.
- **CF score overrides lost after Done** — Static score display always showed TRaSH default after closing the override panel. Now shows overridden values in yellow.
- **Alpine errors on quality structure** — `item.items.length` crashed on non-group items (undefined), cascading into reactive state corruption that affected CF score overrides.
- **Custom CF false "update" on every sync** — Custom CFs with numeric field values (e.g. resolution "2160") were always reported as changed because the stored string didn't match Arr's integer. Values are now normalized before comparison.
- **Profile Builder label clarity** — "Create New Profile" → "New Profile", "Import" → "Import JSON", builder "Import" row → "Start from" to distinguish file import from Arr instance import.
- **Extra CFs score-0 visibility** — CFs with score 0 stayed visible in "Other" after being added to extras because `!0` is `true` in JavaScript. Fixed with explicit `undefined` check.

### Improved

- **Extra CFs Added list** — Multi-column layout (2 cols >10, 3 cols >20) matching the Other list, preventing long single-column scrolling.

### Changed

- **Icon buttons** — Sync history action buttons (Edit, Sync, Clone, Remove) replaced with compact icons + tooltips for a cleaner layout.

## v1.8.8

### Fixed

- **Custom CF storage — eliminate cross-app name collisions** — Imported custom formats with identical names in Radarr and Sonarr (e.g. `!LQ`) no longer get a `(2)` suffix. CFs are now stored in app-scoped directories (`/config/custom/json/{radarr,sonarr}/cf/`). Existing installations migrate automatically on startup — old files are moved to the correct subdirectory and collision suffixes are stripped.
- **CF editor Type dropdown empty on first open** — The "Type" dropdown in the Custom Format editor showed "Select type..." instead of the actual type (e.g. Source, Release Group) when opening a CF for the first time. Root cause: `<template x-for>` inside `<select>` is invalid HTML and the browser silently removes it. Replaced with programmatic option creation via `x-effect`.
- **Export TRaSH JSON broken over HTTP** — The "Export TRaSH JSON" button in the CF editor silently failed on non-HTTPS connections (e.g. LAN access). Replaced with a proper export modal showing formatted JSON with a Copy button, matching the profile builder export style.

## v1.8.7

### Fixed

- **Custom Format editor — context dropdown showed wrong app types** — When editing a user-created CF, the "Trash Scores → Context" dropdown listed all contexts regardless of app type. A Sonarr CF's dropdown showed Radarr-only SQP tiers (`sqp-1-1080p`, `sqp-2`, etc.) and `anime-radarr`. The list is now derived dynamically from the actual TRaSH-Guides CF JSONs on disk via a new `/api/trash/{app}/score-contexts` endpoint, so Sonarr CFs only show Sonarr contexts (including `anime-sonarr`) and Radarr CFs only show Radarr contexts (with all SQP tiers). New contexts added by TRaSH upstream are picked up automatically without code changes.

### Improved

- **Sync Profile modal — clearer dropdown labels and descriptions** — All three dropdowns (Add / Scores / Reset) had labels and descriptions that either implied the wrong behavior or hid important details. Rewritten against the actual `BuildSyncPlan` / `ExecuteSyncPlan` logic so each option states exactly what it does:
  - **Scores:** "Enforce TRaSH scores" / "Allow custom scores" suggested TRaSH defaults override everything and that "custom scores" meant Clonarr-side overrides. Both misleading — Clonarr score overrides apply in *both* modes, and the real distinction is how Clonarr handles manual edits made directly in Arr's UI. Renamed to "Overwrite all scores in Arr" / "Preserve manual edits in Arr" with descriptions that spell out the behavior precisely.
  - **Add:** "Automatically add new formats" didn't mention that this mode respects manual CF removals in Arr (the actual reason to pick it over "add missing"). Renamed to "Respect manual removals — only add new ones" and the description now explains the `lastSyncedSet` comparison and the first-sync edge case.
  - **Reset:** "Reset unsynced scores to 0" didn't clarify that only non-zero scores are touched, or what "unsynced" means. Renamed to "Zero out orphaned scores" and the description spells out that it targets CFs in the target Arr profile that are no longer part of this sync.
  No logic change — pure text and label rewrite.
- **File Naming tab — verbatim TRaSH-Guides text** — All descriptions on the File Naming tab now quote TRaSH-Guides directly instead of paraphrasing. Clonarr is a TRaSH sync tool; it should use the wording the guide maintainers have crafted. Replaced the "Why use a naming scheme?" and "IMDb vs TVDb / TMDb" info cards, per-scheme descriptions (Original Title, P2P/Scene), section descriptions for Movie File/Folder Format, Episode/Series/Season Folder Format, and the Plex "Edition Tags" warning with their TRaSH-Guides source text. Source file paths documented in the UI markup.

## v1.8.6

### Added

- **Quality Group editor in TRaSH sync overrides** — Edit quality groups directly from the Customize Overrides dialog without opening Profile Builder. Drag-and-drop to reorder, drop on a row to merge, click a group name to rename. Create / rename / merge / ungroup / delete / reorder groups inline.
- **Multi-arch GHCR builds** — `linux/amd64` + `linux/arm64` (Apple Silicon support).

### Fixed

- **Memory leak** — Every API call created a new `http.Client` with its own connection pool, accumulating ~2-3 MiB/hour of unreclaimable transport state. Replaced with two shared clients (one for Arr/Prowlarr API, one for notifications). Also fixed event slice reslicing to release old backing arrays.
- **Five sync diff blindspots** — Sync previously missed Radarr-side changes that kept the same set of allowed qualities: reorder items, reorder groups, extracting a quality from a group, cutoff change, and upgradeUntil change. The diff was set-based and silently ignored ordering and structure. Replaced with a structure-aware fingerprint that captures ordering, group structure, and allowed-state. Covers Auto-Sync, manual Sync, and Sync All.
- **Sync result banner hiding change details** — After Save & Sync, the profile detail banner only showed `cfsCreated` / `cfsUpdated` / `scoresUpdated` counts. Quality flips, cutoff changes, and per-CF changes were in the backend response but never rendered. Banner now lists the full details.
- **Imported profile toast hiding change details** — Same blindspot in the `startApply` toast path. Now renders the full details list like `Sync` / `Sync All` already did.
- **Quality structure override loss on auto-sync** — Enabled structure overrides now survive every sync regardless of upstream TRaSH quality/CF/score changes.
- **Cutoff handling with structure override** — Cutoff dropdown reads from the override structure when set (so renamed/created groups appear). "Reset to TRaSH" properly clears the structure override.

## v1.8.5

### Fixed

- **Zombie process leak** — `git auto-gc` was detaching as an orphan subprocess and getting reparented to the Go binary running as PID 1, which the Go runtime does not reap. Accumulated ~79 zombies in 6 hours under normal load. Fix: `tini` as PID 1 in the Dockerfile (`ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]`), plus `git config gc.auto=0` on the TRaSH data dir in `ui/trash.go` (both the fresh-clone and migration code paths). Verified zero zombies after 3+ hours in production.

## v1.8.4

### Fixed

- **CF tooltip showing raw markdown** — Descriptions with Wikipedia links (e.g. streaming service CFs) now display as clean text instead of raw markdown syntax

## v1.8.3

### Fixed

- **Browser autofill popup on Settings** — URL and token fields no longer trigger browser password save/fill dialogs

## v1.8.2

### Improved

- **Sync Rules column headers** — TRaSH Profile, Arr Profile, Auto-Sync, Details, and Actions columns with consistent alignment across all rows
- **Arr Profile ID** — Profile ID shown next to Arr profile name (e.g. `ID 23`) for easy identification
- **Builder Synced Profiles** — Same column layout as TRaSH sync (Your Profile, Arr Profile, Details, Actions)
- **Text readability** — All secondary text lightened from `#484f58` to `#6e7681` across all tabs (quality sizes, scoring sandbox, settings, compare, builder)
- **Healthcheck suggestion UI** — Suggestion box hidden when no Extra Parameters command is available (e.g. distroless images)

### Fixed

- **conflicts.json parser** — Updated to match the TRaSH Guides PR #2681 schema where trash_ids are object keys, not fields. Ready for when the PR merges.

## v1.8.1

First stable release — all previous beta versions consolidated.

### Features
- **Gotify push notifications** — Configurable Gotify support for all notification types (auto-sync, cleanup, repo updates, changelog). Per-level priority toggles (Critical/Warning/Info) with customizable priority values.
- **Second Discord webhook** — Separate webhook for TRaSH Guides updates (repo changes, weekly changelog), keeping sync notifications on the main webhook.
- **Settings reorganized** — Collapsible accordion sections: Instances, Notifications, Auto-Sync, Advanced. Cleaner layout as settings grew.

### Bug fixes
- **Gotify fires independently of Discord** — Notifications no longer require a Discord webhook to be set. Gotify and Discord send independently.
- **Priority value 0 preserved** — Gotify priority value of 0 (silent) now persists correctly through restarts instead of being reset to defaults.

## v1.8.0-beta

### Features
- **Auto-sync GUI toasts** — When scheduled or manual pull triggers auto-sync, toast notifications show detailed results (CF names, score changes, quality items) with staggered 3s delay between multiple profiles.
- **Detailed sync toasts** — quickSync, Sync All, and toggle auto-sync now show specific changes (e.g. "Repack/Proper: 5 → 6") instead of just counts.
- **Sync All respects auto-sync** — Only syncs profiles with auto-sync enabled. Shows warning if no profiles qualify.
- **Scheduled pull diff toast** — Scheduled pulls show "TRaSH updated: ..." toast in GUI automatically.
- **Instance version display** — Settings shows "Connected · vX.Y.Z" for Radarr, Sonarr, and Prowlarr consistently.
- **Prowlarr auto-test** — Prowlarr tested on init and every 60s alongside Radarr/Sonarr.

### UI improvements
- **Sync rules layout** — Fixed min-widths for profile names, arrow, Arr name, and auto-sync toggle for vertical alignment across all rules.
- **Larger arrow** — Profile → Arr arrow more visible (15px, lighter color, centered margins).
- **Settings layout** — Instance URL inline after name, version on same line as Connected.

## v1.7.9-beta

### Features
- **Compare overhaul** — Compare tab now shows profile settings (min score, cutoff, language, upgrade allowed, quality items) alongside CF comparison. All sections in collapsible cards with summary badges and status icons.
- **Settings sync from Compare** — Checkboxes on each setting/quality diff: checked syncs to TRaSH value, unchecked keeps current value as override. Overrides passed to sync modal automatically.
- **Override and custom CF badges on sync rules** — TRaSH Sync tab shows separate pills: blue "X custom CFs" for user-created formats, amber "X overrides" for score/quality/settings overrides. Tooltips explain each.
- **Auto-sync immediate run** — Enabling auto-sync toggle now runs sync immediately instead of waiting for next TRaSH pull.
- **Pull toast notification** — Manual pull shows toast with result: "TRaSH data up to date" or diff summary.
- **conflicts.json support** — Auto-deselect conflicting CFs when TRaSH merges conflicts.json. Activates automatically on pull.

### Bug fixes
- **Optional exclusive groups (SDR)** — Can now deselect all toggles. Golden Rule still requires at least one active.
- **Sync All Fixes** — Confirm dialog with profile names. Correct profile pre-selection via resyncTargetArrProfileId.
- **Required CFs counts** — Compare badges now show section-specific counts (not global totals that included grouped CFs).
- **Auto-sync hidden in Compare sync** — Sync modal from Compare hides auto-sync toggle.
- **Select option type mismatch** — Fixed String vs number comparison for Arr profile dropdown pre-selection.
- **Shallow clone diff detection** — Pull diff works reliably with shallow clones (fetch uses `--deepen=1`).

### Internal
- Prepared conflicts.json parsing (ConflictsData structs, API endpoint, frontend loading). Zero-downtime activation when TRaSH merges PR #2681.

## v1.7.7-beta

### Bug fixes
- **Profile Builder buttons missing** — `_resyncReturnSubTab` and `_resyncNavigating` were not declared in Alpine data, causing console errors and hiding Create/Save/Sync buttons entirely.
- **Top action bar in Profile Builder** — Save/Sync buttons now shown at top of builder (not just in sticky bottom bar), matching user expectation.
- **Auto-sync hidden for builder profiles** — Sync modal no longer shows auto-sync toggle for builder profiles (manual sync only, prevents TRaSH/builder conflicts).

## v1.7.6-beta

### Features
- **Git diff Discord notifications** — "TRaSH Guides Updated" now shows actual file changes (Added/Updated/Removed per CF, profile, group) via git diff instead of stale updates.txt entries.
- **Separate weekly changelog notification** — "TRaSH Weekly Changelog" Discord notification sent only when TRaSH updates their changelog (amber embed, distinct from per-pull blue notifications).
- **Latest Update in GUI dropdown** — Changelog dropdown now shows last pull's actual changes at the top, with timestamp and commit range. TRaSH Changelog (updates.txt) shown below.
- **Next pull countdown** — Header bar shows time until next scheduled pull (auto-updates every 30s).
- **Arr profile name in Discord** — Auto-sync Discord notifications show Arr profile name when different from TRaSH profile name.
- **CF tab uses TRaSH groups** — Custom Formats tab now uses actual TRaSH CF group files as categories instead of hardcoded fake categorization. Each group file is its own collapsible section with color-coded borders.
- **Multi-column CF lists** — CF lists with 10+ items use 2 columns, 30+ use 3 columns for compact display.

### Bug fixes
- **CF description duplicate name** — TRaSH markdown descriptions started with a bold title line repeating the CF name. Now stripped automatically.
- **Pull remote URL sync** — Changing repo URL in settings now updates the git remote before fetching. Previously the old remote was used until re-clone.
- **Quality override flip-flop** — Quality overrides (user-toggled resolutions) are now applied before comparing with Arr state, preventing false Enabled/Disabled changes on every sync.
- **Discord "no changes" spam** — Auto-sync no longer sends Discord notifications for profiles that are already in sync.
- **Discord bullet point formatting** — Fixed indented bullet points rendering incorrectly in Discord embeds.
- **Manual pull sends Discord notification** — Manual pull button now triggers "TRaSH Guides Updated" notification (previously only scheduled pulls did).
- **timeAgo auto-updates** — Sync timestamps in UI now update automatically every 30s without manual refresh.
- **Sync history auto-reload** — Frontend detects when scheduled pull completes and reloads sync data automatically.
- **Last diff persisted to disk** — Latest Update diff survives container restarts.
- **Unique category colors** — Fixed duplicate colors for Streaming Services, Optional, Resolution, and HQ Release Groups categories.
- **Improved text contrast** — Fixed dark-on-dark text for commit hash, changelog counts, and PR links in UI.
- **Dockerfile version** — Updated from 1.7.2-beta to 1.7.6-beta.

## v1.7.5-beta

### Bug fixes
- **Builder/TRaSH sync rule separation** — Auto-sync disabled for builder profiles (manual sync only). Prevents builder rules from overwriting TRaSH sync history on pull.
- **Auto-sync rule updated on source change** — Syncing a TRaSH profile to an Arr profile with a builder rule now converts the rule permanently. No merge-back possible.
- **Confirm dialog on source change** — Warning shown when syncing overwrites a rule of different type (Builder→TRaSH or TRaSH→Builder).
- **Startup cleanup safety** — Cleanup skips instances returning 0 profiles (race condition when Arr is still starting).
- **Reset Non-Synced Scores** — Now includes extra CFs, custom CFs, and all CFs from sync history. Previously only checked standard TRaSH profile CFs, causing user-synced CFs to be falsely flagged.

## v1.7.4-beta

### Features
- **Instance health check every 60s** — Connection status now updates automatically within a minute when instances go up or down (was 5 minutes).
- **Comprehensive debug logging** — Cleanup, auto-sync, TRaSH pull, and sync errors now all logged to debug.log for easier troubleshooting.
- **Profile Builder description** — Clarified as "For advanced users" with amber warning, pointing users to TRaSH Sync tab.

### Bug fixes
- **Sync errors shown as "no changes"** — Backend returns `{"error":"..."}` but frontend only checked `result.errors` (array). Connection failures now correctly show red error toast.
- **Error badge persists through toggle** — Toggling auto-sync no longer clears the error badge. Error clears only when a sync succeeds.
- **Sync All/quickSync sets error badge** — Manual sync failures now set lastSyncError on auto-sync rules, not just auto-sync failures.
- **Sync All toast type** — All failures = red, some = amber, none = blue (was always amber or blue).

## v1.7.3-beta

### Features
- **Builder sync rules in Builder tab** — Builder synced profiles now shown in Profile Builder tab instead of TRaSH Sync, with distinct tooltips and "Sync All" per tab.
- **Discord notifications for settings changes** — Auto-sync notifications now show profile settings changes (Min Score, Cutoff Score, etc.) and zeroed scores with CF names.

### Bug fixes
- **Create-mode cutoff override preserved** — Cutoff override no longer replaced by first allowed quality when user's chosen cutoff is still valid.
- **Update-mode settings-only changes detected** — HasChanges() now always executes for updates, catching min score and cutoff changes that were previously skipped.
- **Cutoff read-only display shows override** — After Done, cutoff override now shown in amber instead of always showing TRaSH default.

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
- **CF name casing auto-corrected** — CFs with wrong casing (e.g. HULU vs Hulu) are now updated to match TRaSH's canonical name on sync.
- **Orphaned scores case-insensitive** — Maintenance Reset Non-Synced Scores no longer flags CFs with different casing as out of sync.
- **Tooltip links clickable** — SQP description tooltips now have styled, clickable links. Tooltip stays visible when hovering over it.
- **CF info icon more readable** — Info icon and trash ID in builder now use lighter color for better visibility.

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
