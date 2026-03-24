# Clonarr Cleanup Tab — Test Plan

## Prerequisites
- Deploy `clonarr:latest` on Unraid
- Rename appdata folder: `syncarr` → `clonarr` (already done)
- Verify container starts and UI loads at port 6060
- Have at least one Radarr/Sonarr instance configured with CFs

## 1. Instance Selection
- [ ] Cleanup tab shows instance dropdown
- [ ] Instances sorted alphabetically (Radarr before Sonarr, then by name)
- [ ] Selecting an instance shows all 5 cleanup cards + keep list card

## 2. Keep List (Persistence)
- [ ] Keep list is empty on first use
- [ ] Type a CF name, press Enter — tag appears as blue pill
- [ ] Type a CF name, click Add — same result
- [ ] Duplicate names (case-insensitive) are rejected silently
- [ ] Click x on a tag — removes it
- [ ] Switch to another instance, switch back — keep list is preserved
- [ ] Restart container — keep list is still there
- [ ] Verify `clonarr.json` has `cleanupKeep` entry with instance ID and names

## 3. Scan: Duplicate CFs
- [ ] Click Scan — returns results or "all clear"
- [ ] If duplicates found: detailed list with name + detail columns
- [ ] Detail shows what makes them duplicates (spec fingerprint match)

## 4. Scan: Reset Unsynced Scores
- [ ] Click Scan — checks scores against synced profiles
- [ ] Shows CFs with non-zero scores that aren't in any synced profile

## 5. Scan: Orphaned Scores
- [ ] Click Scan — finds score entries for CFs that no longer exist
- [ ] Shows profile name + CF name in detail

## 6. Scan: Delete All CFs (Keep Scores)
- [ ] Click Scan — shows summary view (warning icon + count), NOT a full list
- [ ] Count should be total CFs minus any on the keep list
- [ ] Yellow warning box about preserving scores
- [ ] Add a CF to keep list, scan again — count decreases by 1
- [ ] Apply button shows "Apply (N items)"

## 7. Scan: Delete All CFs & Scores
- [ ] Click Scan — shows summary view with count
- [ ] Red warning box about permanent deletion
- [ ] Keep list is respected (excluded from count)

## 8. Apply Flow
- [ ] Click Apply on any scan result — spinner shows
- [ ] On success: green checkmark + result summary (e.g. "deleted: 3")
- [ ] Footer changes to just "Close" button
- [ ] On error: red error message in modal, Close button available
- [ ] After closing success modal, re-scanning should reflect the changes

## 9. Edge Cases
- [ ] Scan with no instance selected — nothing happens
- [ ] Scan on instance with 0 CFs — "all clear" message
- [ ] Keep list with names that don't match any CF — no effect (all CFs still in scan)
- [ ] Delete instance in Settings — keep list for that instance is cleaned up

## 10. UI Polish
- [ ] All cards have correct border colors (default, yellow, red)
- [ ] Scan buttons disable while scanning
- [ ] Apply button disables while applying
- [ ] Modal close (x button, click outside) works
- [ ] Summary view shows for >= 20 items or any delete-all action
- [ ] Detailed table shows for < 20 items on non-delete actions
