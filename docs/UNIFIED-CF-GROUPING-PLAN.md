# Unified CF Grouping — Implementation Plan

**Goal:** Profile Builder and Profile Detail must display CFs using the same Category > Group > CF hierarchy with identical colors, sub-groups, exclusive picks, and default badges.

---

## Current State

| Aspect | Builder (`AllCFsCategorized`) | Detail (`ProfileCFCategories`) |
|---|---|---|
| Data shape | Category > flat CF list | Category > Group > CF list |
| CF scope | ALL CFs in the app | Only CFs linked to this profile |
| Score data | All score contexts (`trashScores` map) | Single resolved score |
| Group metadata | None | `shortName`, `trashDescription`, `exclusive`, `defaultEnabled` |
| CF metadata | `isCustom` | `required`, `default` |
| Rendering | Flat rows with toggle + score input | Nested groups with sub-headers, "pick one" badges |

---

## Target State

Both views render CFs as **Category > Group > CF**. Builder gains group sub-structure. Builder-specific features (score override, score set switching, req/opt pills) preserved within the grouped structure.

---

## Step 1: Backend — New Types (trash.go)

```go
type CFPickerGroup struct {
    Name             string          `json:"name"`
    ShortName        string          `json:"shortName"`
    TrashDescription string          `json:"trashDescription"`
    DefaultEnabled   bool            `json:"defaultEnabled"`
    Exclusive        bool            `json:"exclusive"`
    CFs              []CategorizedCF `json:"cfs"`
}

type CFPickerCategoryGrouped struct {
    Category string           `json:"category"`
    Groups   []CFPickerGroup  `json:"groups"`
}

type CFPickerDataGrouped struct {
    Categories []CFPickerCategoryGrouped `json:"categories"`
    ScoreSets  []string                  `json:"scoreSets"`
}
```

## Step 2: Backend — New Function `AllCFsCategorizedGrouped()` (trash.go)

Merges logic from `AllCFsCategorized()` + `ProfileCFCategories()`:
- Iterates ALL CF groups (not filtered to a profile)
- Each CF carries `CategorizedCF` data (all score contexts for score set switching)
- Groups carry metadata: `shortName`, `trashDescription`, `exclusive`, `defaultEnabled`
- CFs not in any group → synthetic "Other" group within their name-fallback category
- Custom CFs → "Custom" group in their assigned category
- Preserves TRaSH's CF order within each group

## Step 3: Backend — Update Handler (handlers.go)

Change `handleAllCFsCategorized` to call `AllCFsCategorizedGrouped()`. Return type changes from `CFPickerData` to `CFPickerDataGrouped`. Endpoint path stays `/api/trash/{app}/all-cfs`.

## Step 4: Frontend — State Changes (index.html)

- Add `pbExpandedGroups: {}` to Alpine state
- `pbCategories` shape changes from `[{category, cfs}]` to `[{category, groups: [{name, shortName, ..., cfs}]}]`
- Update helpers for nested iteration:
  - `pbCatSelectedCount(cat)` — iterate `cat.groups` then `group.cfs`
  - `pbIsCatAllSelected(cat)` — same
  - `pbToggleCategory(cat)` — same
  - `_pbFindCF(trashId)` — iterate `cat.groups` then `group.cfs`
- New helpers:
  - `pbGroupSelectedCount(group)` — count selected CFs in one group
  - `pbIsGroupAllSelected(group)` — check all CFs in group selected
  - `pbToggleGroup(group)` — select/deselect all CFs in group
  - `pbToggleExclusiveCF(trashId, groupCFs)` — for exclusive groups

## Step 5: Frontend — Template Changes (index.html)

Replace flat `x-for="cf in cat.cfs"` with:

```
Category header (collapsible, colored border, select-all)
  [single group] → show CFs directly
  [multiple groups] →
    Group sub-header (collapsible, name, "pick one" badge, select-all)
      Group description (if any)
      CF row (toggle, name, req/opt pill, score input)
```

Each CF row retains builder controls: toggle, name, custom badge, info tooltip, req/opt pill, score input.

## Step 6: Frontend — CSS (index.html)

- Add `.pb-group-header` for group sub-headers (matching `.cf-section-title` from detail)
- Category colors already exist (`cat-audio .pb-cat-header` etc.)
- Reuse `.detail-section-chevron` for group expand/collapse

## Step 7: Cleanup (trash.go)

Remove old `AllCFsCategorized()`, `CFPickerCategory`, `CFPickerData` if no longer referenced.

---

## Features to Preserve

1. Per-CF toggle (`pb.selectedCFs`) — works at CF level, unchanged
2. Score override input (`pb.scoreOverrides`) — works at CF level, unchanged
3. Required/optional pill (`pb.requiredCFs`) — works at CF level, unchanged
4. Select All per category — iterate nested groups
5. Score set switching — uses `cf.trashScores`, unchanged
6. Custom CF injection (`cf.isCustom`) — in "Custom" group
7. Template application — sets by trashId, structure-agnostic
8. Save — iterates `pb.selectedCFs` keys, only `_pbFindCF()` needs update

---

## Risk Areas

1. **`_pbFindCF()` is critical path** — used by save, score display, score override check. Must test thoroughly.
2. **CFs in multiple groups** — deduplicate (first group wins, matching current behavior).
3. **Ungrouped CFs** — synthetic "Other" group within fallback category.
4. **Custom CFs** — need a group to live in.
5. **Exclusive group behavior** — enforce on user click only, not on template load (template may pre-select multiple intentionally).
6. **Template application** — uses detail API data, independent of builder categories. Should not break.
7. **Group default expansion** — groups expand when parent category is expanded.

---

## Also Planned (Same Session)

- **Sync engine Add/Remove/Reset** — per-rule config for how auto-sync handles missing CFs, custom scores, and stopped CFs
- **Profile settings overrides in auto-sync** — save pdOverrides per rule
