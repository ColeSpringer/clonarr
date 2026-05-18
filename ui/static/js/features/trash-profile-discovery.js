// trash-profile-discovery.js — drives the v3 TRaSH Profiles browse tab.
// Renders rich auto-derived descriptions (axes + cf-groups + markdown notes
// + workflow logic) so users can pick a profile without entering the editor.
//
// Data flow: GET /api/trash/{app}/profiles/descriptions returns
// ProfileDescription[] (see internal/core/trash_profile_describer.go for the
// schema). We cache per-app, re-fetch on TRaSH pull-complete.
//
// View modes: 'grid' (default — 3-col auto-fill, all cards expanded) and
// 'list' (vertical compact rows, click-to-expand). Filter chips narrow
// the set by HDR/audio/in-use/etc; search narrows by name + tagline.

export default {
  state: {
    // app → ProfileDescription[]
    trashProfileDescriptions: { radarr: [], sonarr: [] },
    // app → bool (in-flight request)
    tpdLoading: { radarr: false, sonarr: false },
    // 'grid' | 'list' (per-browser localStorage)
    tpdView: localStorage.getItem('clonarr_tpdView') || 'grid',
    // filter chip — 'all' | 'hd' | 'uhd' | 'hdr' | 'lossless' | 'in-use'
    tpdFilter: 'all',
    // category filter — 'all' | <groupName> (Standard / SQP / Anime / …)
    tpdCategoryFilter: 'all',
    tpdSearch: '',
    // trash_id → bool (which cards are expanded in list view)
    tpdOpenIds: {},
  },

  methods: {
    async loadTrashProfileDescriptions(appType) {
      if (!appType) return;
      if (this.tpdLoading[appType]) return;
      this.tpdLoading = { ...this.tpdLoading, [appType]: true };
      try {
        const r = await fetch(`/api/trash/${appType}/profiles/descriptions`);
        if (!r.ok) {
          // 4xx/5xx — clear cached list so empty-state renders
          this.trashProfileDescriptions = { ...this.trashProfileDescriptions, [appType]: [] };
          return;
        }
        const data = await r.json();
        this.trashProfileDescriptions = { ...this.trashProfileDescriptions, [appType]: data || [] };
      } catch (e) {
        console.error('loadTrashProfileDescriptions:', e);
        this.trashProfileDescriptions = { ...this.trashProfileDescriptions, [appType]: [] };
      } finally {
        this.tpdLoading = { ...this.tpdLoading, [appType]: false };
      }
    },

    tpdSetView(view) {
      if (view !== 'grid' && view !== 'list') return;
      this.tpdView = view;
      localStorage.setItem('clonarr_tpdView', view);
    },

    tpdSetFilter(filter) {
      this.tpdFilter = filter;
    },

    tpdSetCategory(cat) {
      this.tpdCategoryFilter = cat;
    },

    // tpdCategoryList returns category names in the same order they
    // appear as section headers in tpdGrouped — by min `group` int
    // ascending, alpha tiebreak. Drives the category-filter pill row
    // so its order matches the on-page grouping (Standard first,
    // SQP next, then Anime / French / German etc).
    tpdCategoryList(appType) {
      const meta = this.trashProfiles[appType] || [];
      const groups = {};
      for (const p of meta) {
        const name = p.groupName || 'Other';
        const gnum = typeof p.group === 'number' ? p.group : Infinity;
        if (!groups[name] || gnum < groups[name].minGroup) {
          groups[name] = { name, minGroup: gnum };
        }
      }
      return Object.values(groups).sort((a, b) => {
        if (a.minGroup !== b.minGroup) return a.minGroup - b.minGroup;
        return a.name.localeCompare(b.name);
      }).map(g => g.name);
    },

    tpdToggleOpen(trashId) {
      this.tpdOpenIds = { ...this.tpdOpenIds, [trashId]: !this.tpdOpenIds[trashId] };
    },

    tpdIsOpen(trashId) {
      return !!this.tpdOpenIds[trashId];
    },

    // True when at least one auto-sync rule references this trash_id on any
    // instance of the same app type. Drives the "in use" badge.
    tpdProfileInUse(appType, trashId) {
      const rules = this.autoSyncRules || [];
      const instIds = new Set(this.instancesOfType(appType).map(i => i.id));
      return rules.some(r => instIds.has(r.instanceId) && r.trashProfileId === trashId);
    },

    // Multi-line tooltip body for the in-use badge — count line + bullet
    // list, same shape whether 1 or N instances so the badge itself stays
    // consistent ("in use" always) and details live in the tooltip.
    // Rendered via white-space: pre-line on the tooltip stylesheet.
    // (Per-section instance picker would make this redundant — open
    // redesign item noted in CLAUDE.md.)
    tpdInUseTooltip(appType, trashId) {
      const rules = this.autoSyncRules || [];
      const insts = this.instancesOfType(appType);
      const byId = new Map(insts.map(i => [i.id, i.name]));
      const names = rules
        .filter(r => r.trashProfileId === trashId && byId.has(r.instanceId))
        // Collapse stray whitespace so an instance name with embedded
        // newlines/tabs can't break the bullet-list layout.
        .map(r => (byId.get(r.instanceId) || '').replace(/\s+/g, ' ').trim());
      if (names.length === 0) return '';
      const header = names.length === 1
        ? 'Used in 1 instance:'
        : `Used in ${names.length} instances:`;
      return header + '\n' + names.map(n => '• ' + n).join('\n');
    },

    // Apply search + category filter + feature filter to the description
    // list. All three combine with AND (a profile must match all active
    // filters to appear). Returns a new array — original state isn't
    // mutated.
    tpdFiltered(appType) {
      const all = this.trashProfileDescriptions[appType] || [];
      const q = (this.tpdSearch || '').toLowerCase().trim();
      const f = this.tpdFilter || 'all';
      const cat = this.tpdCategoryFilter || 'all';
      // Category lookup via the classic trashProfiles metadata — same
      // source tpdGrouped uses, so a profile matches its category-pill
      // exactly when its section heading on the page reads the same.
      const meta = this.trashProfiles[appType] || [];
      const metaById = new Map();
      for (const p of meta) metaById.set(p.trashId, p);
      return all.filter(d => {
        // Search across name + tagline (the two fields visible at-a-glance)
        if (q) {
          const hay = (d.name + ' ' + (d.tagline || '')).toLowerCase();
          if (!hay.includes(q)) return false;
        }
        // Category filter
        if (cat !== 'all') {
          const m = metaById.get(d.trashId);
          const groupName = (m && m.groupName) || 'Other';
          if (groupName !== cat) return false;
        }
        // Feature filter
        switch (f) {
          case 'all': return true;
          case 'hd':       return (d.axes?.resolution || '').includes('1080p') && !(d.axes?.resolution || '').includes('2160p');
          case 'uhd':      return (d.axes?.resolution || '').includes('2160p');
          case 'hdr':      return !!d.axes?.hdr?.scored;
          case 'lossless': return !!d.axes?.audio?.scored;
          case 'in-use':   return this.tpdProfileInUse(appType, d.trashId);
        }
        return true;
      });
    },

    // Group filtered profiles by their groupName (Standard, SQP, Anime, …).
    // Returns [{ name, profiles[] }] sorted by the same rule as the classic
    // groupedProfiles() helper: by min `group` integer ascending (TRaSH's
    // own ordering hint), then alpha for ties. Profiles within each group
    // sorted alphabetically so display order is stable regardless of which
    // order the backend returned them in.
    tpdGrouped(appType) {
      const filtered = this.tpdFiltered(appType);
      // Lookup table from the classic profile list for groupName + group int
      const meta = this.trashProfiles[appType] || [];
      const metaById = new Map();
      for (const p of meta) metaById.set(p.trashId, p);

      const groups = {};
      for (const d of filtered) {
        const m = metaById.get(d.trashId);
        const groupName = (m && m.groupName) || 'Other';
        const groupInt = (m && typeof m.group === 'number') ? m.group : Infinity;
        if (!groups[groupName]) {
          groups[groupName] = { name: groupName, profiles: [], minGroup: Infinity };
        }
        groups[groupName].profiles.push(d);
        if (groupInt < groups[groupName].minGroup) groups[groupName].minGroup = groupInt;
      }
      for (const g of Object.values(groups)) {
        g.profiles.sort((a, b) => a.name.localeCompare(b.name));
      }
      return Object.values(groups).sort((a, b) => {
        if (a.minGroup !== b.minGroup) return a.minGroup - b.minGroup;
        return a.name.localeCompare(b.name);
      });
    },

    // --- Pill-rendering helpers ---
    // Principle: only ASSERT positive features. Don't render pills for
    // missing features ("No HDR", "Lossy audio") — they were called out as
    // visual noise. A card with fewer pills correctly signals fewer
    // differentiators.

    // tpdAudioPillText always returns the user-outcome audio label —
    // "Lossless audio" when [Audio] Audio Formats is scored, "Lossy audio"
    // otherwise. Both are positive framings (asserting what the profile
    // actually gives), not negation. Template uses different pill classes
    // (.aud for lossless = subtle green, neutral for lossy) so they don't
    // visually compete.
    tpdAudioPillText(d) {
      return d.axes?.audio?.scored ? 'Lossless audio' : 'Lossy audio';
    },

    // tpdHDRPillText returns the SHORT HDR pill label — full opt-in
    // enumeration goes in the Highlights bullet list, not on the pill
    // (where long strings break the visual rhythm). Just "HDR" when no
    // variants are available; "HDR · DV available" when at least one
    // Dolby Vision opt-in exists (DV being the most "brag-worthy"
    // optional, more than HDR10+ for most users).
    tpdHDRPillText(d) {
      const hdr = d.axes?.hdr;
      if (!hdr?.scored) return '';
      if (hdr.optIns && hdr.optIns.some(o => o.startsWith('DV'))) {
        return 'HDR · DV available';
      }
      return 'HDR';
    },

    // tpdSourceLabel reduces the raw items[] source list (which may have 6+
    // entries including HDTV / DVD / fallbacks that nobody cares about) to a
    // single canonical label per profile family.
    //
    // Derived from the SOURCES list (not cutoff) — cutoff naming differs
    // between standard ("Remux-1080p"), anime ("Remux 1080p", space), and
    // Sonarr ("WEB 1080p") profiles. The sources list normalises via
    // extractSource() in the backend so the same set of labels appears
    // regardless of which profile family we're rendering. Priority order:
    //   UHD Remux  > Bluray Remux  > UHD Bluray  > Bluray  > WEB-DL only
    // and "+ WEB" suffix added whenever WEB sources are also accepted, so
    // a Remux profile that also accepts WEB-DL reads as "Bluray Remux + WEB"
    // — matches how users think about the profile.
    tpdSourceLabel(d) {
      const srcs = d.axes?.sources || [];
      const set = new Set(srcs);
      const hasWeb = set.has('WEB-DL') || set.has('WEBRip');
      const webSuffix = hasWeb ? ' + WEB' : '';
      if (set.has('UHD Bluray Remux')) return 'UHD Remux' + webSuffix;
      if (set.has('Bluray Remux'))     return 'Bluray Remux' + webSuffix;
      if (set.has('UHD Bluray'))       return 'UHD Bluray' + webSuffix;
      if (set.has('Bluray'))           return 'Bluray' + webSuffix;
      if (hasWeb)                       return 'WEB-DL';
      // Truly unusual profile — just show first 2 sources so we don't lie
      return srcs.slice(0, 2).join(' + ') || 'Mixed sources';
    },

    // tpdResolutionLabel returns just the primary resolution token (1080p /
    // 2160p / 720p). Strips the verbose fallback chain ("720p, 576p, 480p
    // fallback") that exposes raw items[] data nobody cares about for
    // card-level scanning.
    tpdResolutionLabel(d) {
      const raw = d.axes?.resolution || '';
      const m = raw.match(/^(\d+p)/);
      return m ? m[1] : raw;
    },

    // Click-handler for the primary "Use →" CTA on a card. Opens the
    // detail/editor overlay so the user can configure customizations and
    // pick their target instance in Save & Sync's picker. We pass insts[0]
    // as the editor's default working instance (so internal data flow and
    // the sync-modal's picker have a valid starting point), but mark the
    // overlay as browse-mode so the header doesn't display that default
    // as if it were a committed target — the user picks the real instance
    // when they hit Save & Sync. Sync Rule → Edit entries skip this flag
    // (they pass restoreFromRule=true) and continue to show the bound
    // instance as today.
    async tpdUseProfile(appType, trashId) {
      const insts = this.instancesOfType(appType);
      if (insts.length === 0) {
        this.showToast(`Add a ${appType} instance first to use this profile`, 'error', 6000);
        return;
      }
      const profile = (this.trashProfiles[appType] || []).find(p => p.trashId === trashId);
      if (!profile) {
        this.showToast('Profile not found in TRaSH data', 'error', 6000);
        return;
      }
      await this.openProfileDetail(insts[0], profile);
      if (this.profileDetail) {
        this.profileDetail = { ...this.profileDetail, _browseMode: true };
      }
    },
  },
};
