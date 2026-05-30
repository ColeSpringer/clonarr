export default {
  state: {},
  methods: {
    // --- Cleanup ---
    cleanupActionLabel(action) {
      const labels = {
        'duplicates': 'Duplicate Custom Formats',
        'delete-cfs-keep-scores': 'Delete All CFs (Keep Scores)',
        'delete-cfs-and-scores': 'Delete All CFs & Scores',
        'reset-unsynced-scores': 'Reset Non-Synced Scores',
        'orphaned-scores': 'Orphaned Scores',
        'unused-by-clonarr': 'Unused Custom Formats (Clonarr-managed)',
        'unused-profiles': 'Unused Quality Profiles',
      };
      return labels[action] || action;
    },

    async cleanupScan(action) {
      if (!this.cleanupInstanceId) return;
      this.cleanupScanning = true;
      try {
        const resp = await fetch('/api/instances/' + this.cleanupInstanceId + '/cleanup/scan', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action, keep: this.cleanupKeepList }),
        });
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({}));
          this.showToast(err.error || 'Scan failed', 'error', 8000);
          return;
        }
        this.cleanupResult = await resp.json();
        this.cleanupFilter = 'all';
        // Per-row selection. Pre-select every item by default so the
        // primary footer button keeps the previous one-click behaviour;
        // user can then uncheck what they want to keep. Exception:
        // unused-by-clonarr pre-selects the "safe" set (non-rename-
        // flagged when rename-flagged items are present) to preserve
        // the previous Delete-safe default. delete-cfs-* don't render
        // a per-item list, so they skip selection entirely.
        this.cleanupSelected = {};
        const selectable = ['unused-by-clonarr','duplicates','reset-unsynced-scores','orphaned-scores','unused-profiles'];
        if (selectable.includes(action) && Array.isArray(this.cleanupResult.items)) {
          const hasRenameFlagged = action === 'unused-by-clonarr' && this.cleanupResult.items.some(i => i.renamingFlag);
          for (const item of this.cleanupResult.items) {
            if (hasRenameFlagged && item.renamingFlag) continue;
            this.cleanupSelected[item.id] = true;
          }
        }
      } catch (e) {
        this.showToast('Scan failed: ' + e.message, 'error', 8000);
      } finally {
        this.cleanupScanning = false;
      }
    },

    // --- Per-row selection helpers (unused-by-clonarr only) ---
    // Reads cleanupResult.items (or filtered subset) and the
    // cleanupSelected map. Filter-tab aware: "Safe" means non-rename-
    // flagged within the currently visible list, "All" means every
    // visible item. The Managed-by-Clonarr tab is read-only and not
    // exposed to selection.
    cleanupVisibleItems() {
      const r = this.cleanupResult;
      if (!r || !Array.isArray(r.items)) return [];
      // unused-by-clonarr's 'managed' tab is read-only; 'rename-flagged'
      // is a subset of items. unused-profiles' 'managed' tab is the
      // in-use list, also read-only. Other actions don't filter.
      if (r.action === 'unused-by-clonarr') {
        if (this.cleanupFilter === 'managed') return [];
        if (this.cleanupFilter === 'rename-flagged') return r.items.filter(i => i.renamingFlag);
        return r.items;
      }
      if (r.action === 'unused-profiles' && this.cleanupFilter === 'managed') return [];
      return r.items;
    },
    cleanupHasSelection() {
      // True iff this action type exposes per-row selection.
      const a = this.cleanupResult?.action;
      return a === 'unused-by-clonarr' || a === 'duplicates' || a === 'reset-unsynced-scores' || a === 'orphaned-scores' || a === 'unused-profiles';
    },
    cleanupActionVerb() {
      const a = this.cleanupResult?.action;
      if (a === 'duplicates') return 'Remove';
      if (a === 'reset-unsynced-scores' || a === 'orphaned-scores') return 'Reset';
      return 'Delete'; // unused-by-clonarr, unused-profiles, delete-*
    },
    cleanupSelCount() {
      return this.cleanupVisibleItems().filter(i => this.cleanupSelected[i.id]).length;
    },
    cleanupSelAllVisible() {
      const items = this.cleanupVisibleItems();
      return items.length > 0 && items.every(i => this.cleanupSelected[i.id]);
    },
    cleanupSelSomeVisible() {
      const items = this.cleanupVisibleItems();
      const n = items.filter(i => this.cleanupSelected[i.id]).length;
      return n > 0 && n < items.length;
    },
    cleanupSelToggle(id) {
      const next = { ...this.cleanupSelected };
      if (next[id]) delete next[id]; else next[id] = true;
      this.cleanupSelected = next;
    },
    cleanupSelAll() {
      const next = { ...this.cleanupSelected };
      for (const item of this.cleanupVisibleItems()) next[item.id] = true;
      this.cleanupSelected = next;
    },
    cleanupSelSafe() {
      const next = { ...this.cleanupSelected };
      for (const item of this.cleanupVisibleItems()) {
        if (item.renamingFlag) delete next[item.id]; else next[item.id] = true;
      }
      this.cleanupSelected = next;
    },
    cleanupSelClear() {
      const next = { ...this.cleanupSelected };
      for (const item of this.cleanupVisibleItems()) delete next[item.id];
      this.cleanupSelected = next;
    },
    cleanupSelToggleAllVisible() {
      if (this.cleanupSelAllVisible()) this.cleanupSelClear(); else this.cleanupSelAll();
    },

    async cleanupApply() {
      if (!this.cleanupResult?.items?.length) return;
      // Selectable actions read from cleanupSelected (toolbar presets +
      // per-row checkboxes). delete-cfs-* don't render an item list and
      // act on the whole scan result.
      const items = this.cleanupHasSelection()
        ? this.cleanupResult.items.filter(i => this.cleanupSelected[i.id])
        : this.cleanupResult.items;
      if (items.length === 0) return;

      // Confirmation message: verb matches the action (Delete / Remove
      // / Reset), and the noun matches the entity (CFs / profiles /
      // scores). Rename-tag warning only applies when unused-by-clonarr
      // selection still includes any rename-flagged items.
      const a = this.cleanupResult.action;
      const verb = this.cleanupActionVerb().toLowerCase();
      const noun = (a === 'unused-profiles') ? `quality profile${items.length === 1 ? '' : 's'}`
                  : (a === 'reset-unsynced-scores' || a === 'orphaned-scores') ? `score entr${items.length === 1 ? 'y' : 'ies'}`
                  : `custom format${items.length === 1 ? '' : 's'}`;
      const renameCount = a === 'unused-by-clonarr' ? items.filter(i => i.renamingFlag).length : 0;
      const includesRenameTags = renameCount > 0;
      const verbCap = verb.charAt(0).toUpperCase() + verb.slice(1);
      let message = `${verbCap} ${items.length} ${noun} from ${this.cleanupResult.instance}?`;
      if (includesRenameTags && renameCount > 0) {
        message += `\n\nIncludes ${renameCount} rename-only CF${renameCount === 1 ? '' : 's'} (score 0 in every profile, only contributing to filenames). Future renames will no longer include their tags. Existing files on disk are unaffected.`;
      }
      // Recovery hint matches the action: deletes can be re-synced from
      // a TRaSH or builder profile; resets just clear scores.
      if (a === 'reset-unsynced-scores' || a === 'orphaned-scores') {
        message += `\n\nThis sets the selected score${items.length === 1 ? '' : 's'} to 0 on the affected profile${items.length === 1 ? '' : 's'}.`;
      } else if (a === 'unused-profiles') {
        message += `\n\nThis cannot be undone. Profiles in use cannot be deleted by Arr; the cleanup will skip any that are still referenced.`;
      } else {
        message += `\n\nThis cannot be undone, but you can re-sync any TRaSH or builder profile to recreate CFs.`;
      }

      const titleNoun = (a === 'unused-profiles') ? 'Quality Profiles'
                      : (a === 'reset-unsynced-scores' || a === 'orphaned-scores') ? 'Scores'
                      : 'Custom Formats';
      const confirmed = await new Promise(resolve => {
        this.confirmModal = {
          show: true,
          title: `${verbCap} ${titleNoun}`,
          message,
          confirmLabel: `${verbCap} ${items.length}`,
          onConfirm: () => resolve(true),
          onCancel: () => resolve(false),
        };
      });
      if (!confirmed) return;

      this.cleanupApplying = true;
      try {
        const ids = items.map(i => i.id);
        const resp = await fetch('/api/instances/' + this.cleanupResult.instanceId + '/cleanup/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: this.cleanupResult.action, ids }),
        });
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({}));
          this.cleanupResult = { ...this.cleanupResult, applied: false, applyError: err.error || 'Apply failed' };
          return;
        }
        const result = await resp.json();
        this.cleanupResult = { ...this.cleanupResult, applied: true, applyResult: result };
      } catch (e) {
        this.cleanupResult = { ...this.cleanupResult, applied: false, applyError: e.message };
      } finally {
        this.cleanupApplying = false;
      }
    },

    async loadCleanupKeep() {
      if (!this.cleanupInstanceId) { this.cleanupKeepList = []; return; }
      try {
        const resp = await fetch('/api/instances/' + this.cleanupInstanceId + '/cleanup/keep');
        if (resp.ok) this.cleanupKeepList = await resp.json();
      } catch (e) { this.cleanupKeepList = []; }
    },
    async saveCleanupKeep() {
      if (!this.cleanupInstanceId) return;
      await fetch('/api/instances/' + this.cleanupInstanceId + '/cleanup/keep', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(this.cleanupKeepList),
      });
    },
    async loadCleanupCFNames() {
      if (!this.cleanupInstanceId) { this.cleanupCFNames = []; return; }
      try {
        const resp = await fetch('/api/instances/' + this.cleanupInstanceId + '/cfs');
        if (resp.ok) {
          const cfs = await resp.json();
          this.cleanupCFNames = (cfs || []).map(cf => cf.name).sort();
        }
      } catch (e) { this.cleanupCFNames = []; }
    },
    addCleanupKeepName(name) {
      if (!name) return;
      if (this.cleanupKeepList.some(n => n.toLowerCase() === name.toLowerCase())) {
        return; // already in list — no-op, keep input + dropdown intact
      }
      this.cleanupKeepList.push(name);
      // Keep the input + dropdown open so the user can click another match
      // from the same query. Refresh suggestions so the just-added one
      // disappears and remaining matches stay visible. Empty query → empty
      // suggestions (dropdown closes naturally).
      const q = this.cleanupKeepInput.trim().toLowerCase();
      if (q) {
        this.cleanupKeepSuggestions = this.cleanupCFNames.filter(n =>
          n.toLowerCase().includes(q) &&
          !this.cleanupKeepList.some(k => k.toLowerCase() === n.toLowerCase())
        ).slice(0, 10);
      } else {
        this.cleanupKeepSuggestions = [];
      }
      this.saveCleanupKeep();
    },
    async addCleanupKeep() {
      const name = this.cleanupKeepInput.trim();
      if (!name) return;
      if (this.cleanupKeepList.some(n => n.toLowerCase() === name.toLowerCase())) {
        this.cleanupKeepInput = '';
        return;
      }
      this.cleanupKeepList.push(name);
      this.cleanupKeepInput = '';
      await this.saveCleanupKeep();
    },
    async addAllMatchingKeep() {
      const query = this.cleanupKeepInput.trim().toLowerCase();
      if (!query) return;
      const matching = this.cleanupCFNames.filter(n =>
        n.toLowerCase().includes(query) && !this.cleanupKeepList.some(k => k.toLowerCase() === n.toLowerCase())
      );
      if (matching.length === 0) return;
      this.cleanupKeepList.push(...matching);
      this.cleanupKeepInput = '';
      this.cleanupKeepSuggestions = [];
      await this.saveCleanupKeep();
      this.showToast(`Added ${matching.length} CFs to Keep List`, 'info', 3000);
    },

    async removeCleanupKeep(idx) {
      this.cleanupKeepList.splice(idx, 1);
      await this.saveCleanupKeep();
    },
  },
};
