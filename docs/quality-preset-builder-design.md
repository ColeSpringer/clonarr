# Quality Preset Builder — Design Document

## Problem

Profile builder requires quality items (the right side of a Radarr/Sonarr quality profile) to create usable profiles. Currently, users must select an existing TRaSH preset — but new profiles being developed by TRaSH contributors don't have presets yet, and users building fully custom profiles have no way to define quality items.

## Goal

Allow users to visually build a quality item configuration (ordering, grouping, allowed/disallowed) directly in Clonarr's profile builder, matching the functionality of Radarr/Sonarr's built-in quality editor.

## Location in UI

Inside the profile builder settings panel, replacing/extending the current "Quality Preset" dropdown. Two options:

**Option A: Inline editor** — The quality preset dropdown gets a new option "Custom..." that opens the editor inline below the Quality card. Compact, no modal.

**Option B: Modal editor** — "Edit" button next to the Quality Preset dropdown opens a full modal with the quality editor. More space, clearer separation.

**Recommended: Option A** — inline below the Quality card, consistent with the rest of the builder. The editor is part of the settings panel and collapses when not needed.

## Data Source

Quality definitions come from the Radarr/Sonarr API (`/api/v3/qualitydefinition`). This gives us the complete list of available qualities with their IDs and names. We already fetch this for quality size sync — reuse the same endpoint.

Fallback: if no instance is connected, use a hardcoded default list of Radarr qualities (they rarely change).

## UI Design

### Two Modes (matching Radarr)

#### Mode 1: Quality List (default)

```
Quality Items                                    [Edit Groups]
┌─────────────────────────────────────────────────────────┐
│ [✓] Bluray|WEB-2160p  WEBDL-2160p WEBRip-2160p  [▲][▼] │
│ [✓] Bluray|WEB-1080p  WEBDL-1080p WEBRip-1080p  [▲][▼] │
│ [ ] WEB 720p          WEBDL-720p  WEBRip-720p    [▲][▼] │
│ [ ] Bluray-720p                                  [▲][▼] │
│ [ ] Raw-HD                                       [▲][▼] │
│ [ ] BR-DISK                                      [▲][▼] │
│ [ ] Remux-2160p                                  [▲][▼] │
│ [ ] HDTV-2160p                                   [▲][▼] │
│ [ ] Remux-1080p                                  [▲][▼] │
│ [ ] HDTV-1080p                                   [▲][▼] │
│ [ ] HDTV-720p                                    [▲][▼] │
│ ... (remaining qualities)                               │
└─────────────────────────────────────────────────────────┘
Cutoff: [Bluray|WEB-2160p ▼]
```

- Checkbox = allowed/disallowed
- Group names shown with sub-item badges (like Radarr)
- Arrow up/down buttons to reorder
- Cutoff dropdown shows only allowed items

#### Mode 2: Edit Groups

```
Quality Items                             [Done Editing]
┌─────────────────────────────────────────────────────┐
│ ┌─ Bluray|WEB-2160p [rename]           [▲][▼]      │
│ │  WEBDL-2160p              [remove from group]     │
│ │  WEBRip-2160p             [remove from group]     │
│ │  Bluray-2160p             [remove from group]     │
│ └───────────────────────────────────────────────     │
│ ┌─ Bluray|WEB-1080p [rename]           [▲][▼]      │
│ │  WEBDL-1080p              [remove from group]     │
│ │  WEBRip-1080p             [remove from group]     │
│ │  Bluray-1080p             [remove from group]     │
│ └───────────────────────────────────────────────     │
│                                                     │
│ Bluray-720p          [add to group ▼]  [▲][▼]      │
│ Raw-HD               [add to group ▼]  [▲][▼]      │
│ BR-DISK              [add to group ▼]  [▲][▼]      │
│ ...                                                 │
│                                                     │
│ [+ Create New Group]                                │
└─────────────────────────────────────────────────────┘
```

- Groups are expandable with sub-items visible
- Each sub-item has "remove from group" button
- Ungrouped items have "add to group" dropdown
- "Create New Group" button at bottom
- Group names are editable inline
- Arrow up/down for reordering (both groups and individual items)

### Starting Point

The editor needs initial data. Three options in a dropdown at the top:

```
Start from: [TRaSH Preset ▼] [Apply]
            ├── Blank (all qualities, none allowed)
            ├── Current instance profile...
            ├── ── SQP-4 (MA Hybrid)
            ├── ── HD Bluray + WEB
            ├── TRaSH — SQP
            │   ├── SQP-1 (1080p)
            │   ├── SQP-2
            │   └── ...
            ├── TRaSH — Standard
            │   ├── HD Bluray + WEB
            │   └── ...
            └── TRaSH — Remux
                ├── Remux + WEB 1080p
                └── ...
```

## Data Flow

```
Quality Definitions (from Arr API)
  → Quality Builder UI (user reorders, groups, toggles)
  → pb.qualityItems[] (same format as TRaSH items)
  → saveCustomProfile() uses pb.qualityItems
  → generateTrashJSON() reads qualities from saved profile
```

No backend changes needed. The `items` array format is already defined:
```json
[
  { "name": "Bluray|WEB-2160p", "allowed": true, "items": ["WEBDL-2160p", "WEBRip-2160p", "Bluray-2160p"] },
  { "name": "Remux-1080p", "allowed": false },
  ...
]
```

## Implementation Plan

### Phase 1: Basic Quality Editor (core functionality)

**New API endpoint:**
- `GET /api/instances/{id}/quality-definitions` — returns available qualities from Arr API (name + ID list)
- Fallback: hardcoded default list if no instance connected

**Frontend — Quality List mode:**
- New section in profile builder below Quality card (or replacing it)
- Checkbox list of all qualities in order
- Arrow up/down buttons for reordering
- Groups shown with badge sub-items (read-only in this mode)
- Cutoff dropdown (filtered to allowed items only)
- "Start from" dropdown to load a TRaSH preset or blank

**State:**
- `pb.qualityItems[]` — already exists, reused
- `pb.qualityEditMode` — 'list' or 'groups'
- `pb.qualityDefs[]` — available quality definitions from API

**Estimated scope:** ~200 lines HTML, ~150 lines JS, ~20 lines Go

### Phase 2: Edit Groups mode

**Frontend — Edit Groups mode:**
- Toggle between list/groups mode via button
- Expandable groups showing sub-items
- "Remove from group" button per sub-item
- "Add to group" dropdown per ungrouped item
- "Create New Group" with name input
- Inline rename for group names
- Reorder within groups

**Estimated scope:** ~150 lines HTML, ~100 lines JS

### Phase 3: Instance import

- "Load from instance" option in "Start from" dropdown
- Fetches current quality profile items from connected Radarr/Sonarr
- Populates the editor with existing configuration

**Estimated scope:** ~30 lines JS (reuses existing instance profile fetch)

## Edge Cases

1. **No instance connected:** Use hardcoded quality list (Radarr defaults). Groups can still be created manually.
2. **Sonarr vs Radarr:** Quality names differ slightly. Load definitions from the correct instance type.
3. **Empty editor:** "Blank" start gives all qualities in default order, none allowed. User must check at least one.
4. **Switching presets:** Warn if user has made manual changes before loading a new preset.
5. **Missing qualities:** If a TRaSH preset references a quality not in the instance definitions, skip it with warning.

## Files to Modify

| File | Changes |
|------|---------|
| `ui/static/index.html` | Quality builder UI (HTML + JS) |
| `ui/handlers.go` | New endpoint for quality definitions |
| `ui/arr.go` | Quality definitions API call (may already exist for quality sizes) |

## Not in Scope

- Drag-and-drop (requires external library, arrow buttons are sufficient)
- Saving quality presets separately (they live as part of the profile)
- Syncing quality items independently of a full profile sync
