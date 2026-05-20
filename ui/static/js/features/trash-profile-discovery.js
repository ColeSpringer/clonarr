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
    // Multi-select feature filters — array of any combination of
    // 'hd' | 'uhd' | 'hdr' | 'lossless' | 'in-use'. Empty means "All".
    // Within the group selections are OR'd (HDR + Lossless → either),
    // and the feature group AND's with the category group.
    tpdFeatureFilters: [],
    // Multi-select category filters — array of group names from
    // tpdCategoryList (Standard / SQP / Anime / …). Empty means "All".
    // Selections within the group are OR'd.
    tpdCategoryFilters: [],
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

    // Toggle a feature pill in or out of the active set. Clicking an
    // active pill removes it; clicking the "All" pill clears the entire
    // set. Same shape for the two groups, kept as separate helpers so
    // template binding is symmetric with isActive checks.
    tpdToggleFeature(filter) {
      const i = this.tpdFeatureFilters.indexOf(filter);
      if (i >= 0) this.tpdFeatureFilters.splice(i, 1);
      else this.tpdFeatureFilters.push(filter);
    },
    tpdClearFeatures() {
      this.tpdFeatureFilters = [];
    },
    tpdIsFeatureActive(filter) {
      return this.tpdFeatureFilters.includes(filter);
    },

    tpdToggleCategory(cat) {
      const i = this.tpdCategoryFilters.indexOf(cat);
      if (i >= 0) this.tpdCategoryFilters.splice(i, 1);
      else this.tpdCategoryFilters.push(cat);
    },
    tpdClearCategories() {
      this.tpdCategoryFilters = [];
    },
    tpdIsCategoryActive(cat) {
      return this.tpdCategoryFilters.includes(cat);
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

    // True when at least one profile detail body is currently expanded.
    // Drives the Expand-all / Collapse-all toggle label in list view —
    // the same button flips between the two actions based on this state.
    tpdAnyOpen() {
      return Object.values(this.tpdOpenIds).some(v => v);
    },

    // Open every profile in the current filtered set. Operates only on
    // tpdFiltered so a filter narrows the bulk-expand scope (e.g. filter
    // to UHD then Expand all → just those expand, not all 30).
    tpdExpandAll(appType) {
      const next = { ...this.tpdOpenIds };
      for (const d of this.tpdFiltered(appType)) next[d.trashId] = true;
      this.tpdOpenIds = next;
    },

    // Reset all open states. Cheap one-liner; preserved as a named helper
    // so the template stays declarative.
    tpdCollapseAll() {
      this.tpdOpenIds = {};
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
      const cats = this.tpdCategoryFilters || [];
      const feats = this.tpdFeatureFilters || [];
      // Category lookup via the classic trashProfiles metadata — same
      // source tpdGrouped uses, so a profile matches its category-pill
      // exactly when its section heading on the page reads the same.
      const meta = this.trashProfiles[appType] || [];
      const metaById = new Map();
      for (const p of meta) metaById.set(p.trashId, p);

      const matchesFeature = (d, f) => {
        switch (f) {
          case 'hd':       return (d.axes?.resolution || '').includes('1080p') && !(d.axes?.resolution || '').includes('2160p');
          case 'uhd':      return (d.axes?.resolution || '').includes('2160p');
          case 'hdr':      return !!d.axes?.hdr?.scored;
          case 'lossless': return !!d.axes?.audio?.scored;
          case 'in-use':   return this.tpdProfileInUse(appType, d.trashId);
        }
        return false;
      };

      return all.filter(d => {
        if (q) {
          // Build a wider haystack so words from the description match too.
          // Includes name + tagline + every Highlights bullet + axes prose
          // (resolution, sources list, HDR opt-ins, average size) + the
          // disclaimer prose. Matches user intent: searching "atmos" finds
          // the lossless-audio bullet; "remux" matches the source label
          // even when not in the profile name.
          const hayParts = [
            d.name,
            d.tagline || '',
            (d.highlights || []).join(' '),
            d.axes?.resolution || '',
            (d.axes?.sources || []).join(' '),
            (d.axes?.hdr?.optIns || []).join(' '),
            d.axes?.avgSize || '',
            d.disclaimer?.before || '',
            d.disclaimer?.linkText || '',
            d.disclaimer?.after || '',
          ];
          const hay = hayParts.join(' ').toLowerCase();
          if (!hay.includes(q)) return false;
        }
        // Category — empty array means "All". Multiple selections OR'd
        // (Standard + SQP → either group qualifies).
        if (cats.length > 0) {
          const m = metaById.get(d.trashId);
          const groupName = (m && m.groupName) || 'Other';
          if (!cats.includes(groupName)) return false;
        }
        // Features — empty array means "All". Multiple selections OR'd
        // (HDR + Lossless → either feature qualifies). To require all
        // selected features at once, swap .some → .every; OR was chosen
        // because most chip-grids in this app behave additively.
        if (feats.length > 0) {
          if (!feats.some(f => matchesFeature(d, f))) return false;
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

    // Returns the auto-derived ProfileDescription for the profile currently
    // open in the editor overlay, or null. Reused by the Sync Preview's
    // profile-info panel so the editor opens with the same rich axes /
    // highlights / disclaimer copy as the TPD profile card.
    spProfileDescription() {
      const appType = this.profileDetail?.instance?.type;
      const trashId = this.profileDetail?.profile?.trashId;
      if (!appType || !trashId) return null;
      const list = this.trashProfileDescriptions?.[appType] || [];
      return list.find(d => d.trashId === trashId) || null;
    },

    // Parses TRaSH cf-group names like "[Streaming Services] General" and
    // returns a sectioned structure for the Sync Preview sub-nav:
    //   [{ section: 'STREAMING SERVICES', items: [{ ...group, _shortName: 'General' }, ...] }, ...]
    // Groups without a bracket prefix fall into "OTHER". Section order is
    // preserved from the input list (TRaSH sorts groups by its own `group`
    // field, so categories come out in the same order as TRaSH renders
    // them on its website).
    spGroupBySection(groups) {
      const re = /^\[([^\]]+)\]\s*(.*)$/;
      const sections = new Map();
      for (const g of (groups || [])) {
        const m = (g.name || '').match(re);
        const section = m ? m[1].trim().toUpperCase() : 'OTHER';
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        if (!sections.has(section)) sections.set(section, []);
        sections.get(section).push({ ...g, _shortName: shortName });
      }
      return Array.from(sections, ([section, items]) => ({ section, items }));
    },

    // Effective active-CF count for a group, mirroring the action-row
    // counter logic but resolving group-state via selectedOptionalCFs +
    // group.defaultEnabled fallback. Used by the sub-nav to render
    // "X / total" so the user can scan which groups are in use without
    // clicking through each one.
    //
    // Rules (same as the action-row counter):
    //  - Group with required and/or default-on CFs (hasToggle): when group
    //    is OFF, count is 0; when ON, count = required + effectively-on
    //    optional CFs.
    //  - Group without auto-includes (opt-in only — no required, no
    //    default-on): group-toggle is irrelevant; count purely by per-CF
    //    effective state.
    spGroupActiveCount(g) {
      const cfs = g.cfs || [];
      const hasToggle = cfs.some(cf => cf.required)
        || (!g.exclusive && cfs.some(cf => cf.default));
      const sel = this.selectedOptionalCFs || {};
      const grpKey = '__grp_' + g.name;
      const grpOn = sel[grpKey] !== undefined ? sel[grpKey] : g.defaultEnabled;
      const cfOn = (cf) => sel[cf.trashId] === undefined ? !!cf.default : !!sel[cf.trashId];
      if (!hasToggle) {
        return cfs.filter(cf => cfOn(cf)).length;
      }
      if (!grpOn) return 0;
      return cfs.filter(cf => cf.required || cfOn(cf)).length;
    },

    // === Sync Preview "Profile overview" tab helpers ===
    // Read-only summary of what's currently configured on the profile —
    // built so the user can see the whole spec at a glance without
    // navigating into each group. Edit-affordances layer on top later
    // once the Customize button wires basics-editing.

    // The 6 basics fields with effective value + whether the user has
    // overridden them. pdOverrides legacy semantics: `.enabled === false`
    // means "override is active". Returns one row per field.
    spOverviewBasics() {
      const pd = this.profileDetail?.detail?.profile;
      const ov = this.pdOverrides || {};
      if (!pd) return [];
      const isMod = (f) => ov[f]?.enabled === false;
      return [
        {
          label: 'Language',
          value: ov.language?.value || pd.language?.name || 'Original',
          modified: isMod('language'),
          defaultValue: pd.language?.name || 'Original',
        },
        {
          label: 'Min score',
          value: ov.minFormatScore?.value ?? pd.minFormatScore ?? 0,
          modified: isMod('minFormatScore'),
          defaultValue: pd.minFormatScore ?? 0,
        },
        {
          label: 'Min upgrade',
          value: ov.minUpgradeFormatScore?.value ?? pd.minUpgradeFormatScore ?? 1,
          modified: isMod('minUpgradeFormatScore'),
          defaultValue: pd.minUpgradeFormatScore ?? 1,
        },
        {
          label: 'Cutoff score',
          value: ov.cutoffFormatScore?.value ?? pd.cutoffFormatScore ?? 10000,
          modified: isMod('cutoffFormatScore'),
          defaultValue: pd.cutoffFormatScore ?? 10000,
        },
        {
          label: 'Upgrades allowed',
          value: (ov.upgradeAllowed?.value ?? pd.upgradeAllowed) ? 'On' : 'Off',
          modified: isMod('upgradeAllowed'),
          defaultValue: pd.upgradeAllowed ? 'On' : 'Off',
        },
        {
          label: 'Cutoff quality',
          value: this.pdOverrides?.cutoffQuality || pd.cutoff || '—',
          // pdInitOverrides seeds cutoffQuality with the profile's default
          // (profiles.js:3523 / :3417) so it's always non-empty. A truthy
          // check would falsely flag it as modified — compare against the
          // default like profiles.js:1601 does in its canonical "did it
          // actually change?" check.
          modified: !!this.pdOverrides?.cutoffQuality
                 && this.pdOverrides.cutoffQuality !== pd.cutoff,
          defaultValue: pd.cutoff || '—',
        },
      ];
    },

    // CFs grouped by source (Required first, then by in-profile group,
    // then by Additional CF group). Each section carries a `kind` flag
    // so sub-nav filter views can pick a subset:
    //   'required'   — formatItemNames (profile-level required CFs)
    //   'in-profile' — TRaSH groups inside profile.quality_profiles.include
    //   'additional' — extraCFGroups (opt-in pool outside profile)
    // Each CF carries `_isRequired` + a `_score` object with effective +
    // original + isOverridden, so the row can render strikethrough on
    // the original when the user has overridden a score (customizations
    // shown inline, not in a separate card).
    // De-dupes by trashId in case the same CF lives in multiple groups.
    spOverviewEnabledCFs() {
      if (!this.profileDetail) return [];
      const sel = this.selectedOptionalCFs || {};
      const overrides = this.cfScoreOverrides || {};
      const scoreInfo = (cf) => {
        const orig = cf.score;
        const o = overrides[cf.trashId];
        if (o !== undefined && o !== orig) {
          return { effective: o, original: orig, isOverridden: true };
        }
        return { effective: orig, original: orig, isOverridden: false };
      };
      const seen = new Set();
      const sections = [];
      const pushSection = (label, category, cfs, kind) => {
        const filtered = cfs.filter(cf => {
          if (seen.has(cf.trashId)) return false;
          seen.add(cf.trashId);
          return true;
        });
        if (filtered.length > 0) sections.push({ source: label, sourceCategory: category, cfs: filtered, kind });
      };

      // 1. Required CFs at profile level — formatItemNames
      pushSection(
        'Required CFs',
        'required',
        (this.profileDetail.detail?.formatItemNames || []).map(fi => ({
          ...fi, _isRequired: true, _score: scoreInfo(fi),
        })),
        'required'
      );

      // 2. In-profile trashGroups
      for (const g of (this.profileDetail.detail?.trashGroups || [])) {
        const hasToggle = g.cfs.some(cf => cf.required)
          || (!g.exclusive && g.cfs.some(cf => cf.default));
        const grpKey = '__grp_' + g.name;
        const grpOn = sel[grpKey] !== undefined ? sel[grpKey] : g.defaultEnabled;
        if (hasToggle && !grpOn) continue;
        const m = (g.name || '').match(/^\[([^\]]+)\]\s*(.*)$/);
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        const enabled = [];
        for (const cf of g.cfs) {
          let isOn;
          if (cf.required) {
            isOn = !hasToggle || grpOn;
          } else {
            const cfOn = sel[cf.trashId] === undefined ? !!cf.default : !!sel[cf.trashId];
            isOn = (!hasToggle || grpOn) && cfOn;
          }
          if (isOn) enabled.push({ ...cf, _isRequired: !!cf.required, _score: scoreInfo(cf) });
        }
        pushSection(shortName, g.category, enabled, 'in-profile');
      }

      // 3. Additional CF groups (opt-in pool outside the profile)
      for (const g of (this.extraCFGroups || [])) {
        const m = (g.name || '').match(/^\[([^\]]+)\]\s*(.*)$/);
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        const enabled = g.cfs
          .filter(cf => !!sel[cf.trashId])
          .map(cf => ({ ...cf, _isRequired: false, _score: scoreInfo(cf) }));
        pushSection(shortName, g.category, enabled, 'additional');
      }

      return sections;
    },

    // Diffs view — everything that diverges from the TRaSH default for
    // this profile. Four buckets in priority order:
    //   1. modifiedBasics — pdOverrides where .enabled === false (legacy
    //      inverse semantics) + cutoffQuality if set.
    //   2. scoreOverrides — CFs with cfScoreOverrides[trashId] !== cf.score.
    //   3. additionalCFs — CFs from extraCFGroups the user has opted into
    //      (outside the profile's default scope entirely).
    //   4. excludedCFs — default-on CFs in still-active groups that the
    //      user has explicitly turned off (selectedOptionalCFs[id] === false).
    //
    // Intentionally NOT included: optional CFs the user has activated
    // within in-profile groups. Those live inside the profile's own
    // structure and aren't considered "external" deviations — they're
    // visible via the "Optional CF" sub-nav view.
    //
    // Each bucket returns rich objects so the template can render the
    // diff with original-vs-current side by side. Computed lazily on
    // each access (Alpine reactivity tracks the dependencies).
    spOverviewDiffs() {
      const out = { modifiedBasics: [], scoreOverrides: [], additionalCFs: [], excludedCFs: [] };
      if (!this.profileDetail) return out;
      const sel = this.selectedOptionalCFs || {};
      const overrides = this.cfScoreOverrides || {};
      const pd = this.profileDetail?.detail?.profile;

      // 1. Modified basics
      const ov = this.pdOverrides || {};
      const basicSpec = {
        language: { label: 'Language', display: (v) => v },
        minFormatScore: { label: 'Min score', display: (v) => v },
        minUpgradeFormatScore: { label: 'Min upgrade', display: (v) => v },
        cutoffFormatScore: { label: 'Cutoff score', display: (v) => v },
        upgradeAllowed: { label: 'Upgrades allowed', display: (v) => v ? 'On' : 'Off' },
      };
      for (const [field, spec] of Object.entries(basicSpec)) {
        if (ov[field]?.enabled === false) {
          out.modifiedBasics.push({
            label: spec.label,
            current: spec.display(ov[field].value),
            original: spec.display(field === 'language' ? (pd?.language?.name || 'Original') : pd?.[field]),
          });
        }
      }
      // Same pattern as profiles.js:1601 — only flag as modified when
      // the override actually differs from the profile default.
      if (this.pdOverrides?.cutoffQuality && this.pdOverrides.cutoffQuality !== pd?.cutoff) {
        out.modifiedBasics.push({
          label: 'Cutoff quality',
          current: this.pdOverrides.cutoffQuality,
          original: pd?.cutoff || '—',
        });
      }

      // 2. Score overrides — walk every CF source and pick out diffs
      const seenScoreOv = new Set();
      const collectScoreOv = (cf, fromGroup) => {
        if (seenScoreOv.has(cf.trashId)) return;
        const o = overrides[cf.trashId];
        if (o !== undefined && o !== cf.score) {
          seenScoreOv.add(cf.trashId);
          out.scoreOverrides.push({ name: cf.name, current: o, original: cf.score, fromGroup });
        }
      };
      for (const cf of (this.profileDetail.detail?.formatItemNames || [])) collectScoreOv(cf, 'Required CFs');
      for (const g of (this.profileDetail.detail?.trashGroups || [])) {
        const m = (g.name || '').match(/^\[([^\]]+)\]\s*(.*)$/);
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        for (const cf of g.cfs) collectScoreOv(cf, shortName);
      }
      for (const g of (this.extraCFGroups || [])) {
        const m = (g.name || '').match(/^\[([^\]]+)\]\s*(.*)$/);
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        for (const cf of g.cfs) collectScoreOv(cf, shortName + ' (Additional)');
      }

      // 3. Additional CFs activated — CFs from extraCFGroups the user
      // turned on (these are entirely outside the profile's default
      // scope, so any opt-in counts as a deviation).
      for (const g of (this.extraCFGroups || [])) {
        const m = (g.name || '').match(/^\[([^\]]+)\]\s*(.*)$/);
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        for (const cf of g.cfs) {
          if (!!sel[cf.trashId]) {
            out.additionalCFs.push({
              name: cf.name,
              score: overrides[cf.trashId] !== undefined ? overrides[cf.trashId] : cf.score,
              fromGroup: shortName,
              sourceCategory: g.category,
            });
          }
        }
      }

      // 4. Excluded default-on CFs — only counted when the group is
      // still active (an inactive group's CFs aren't "excluded" — the
      // whole group is off).
      for (const g of (this.profileDetail.detail?.trashGroups || [])) {
        const grpKey = '__grp_' + g.name;
        const grpOn = sel[grpKey] !== undefined ? sel[grpKey] : g.defaultEnabled;
        if (!grpOn) continue;
        const m = (g.name || '').match(/^\[([^\]]+)\]\s*(.*)$/);
        const shortName = m ? (m[2].trim() || g.name) : g.name;
        for (const cf of g.cfs) {
          if (cf.required) continue;
          if (cf.default && sel[cf.trashId] === false) {
            out.excludedCFs.push({
              name: cf.name,
              score: cf.score,
              fromGroup: shortName,
              sourceCategory: g.category,
            });
          }
        }
      }

      return out;
    },

    // Total diff count for the sub-nav badge.
    spOverviewDiffCount() {
      const d = this.spOverviewDiffs();
      return d.modifiedBasics.length + d.scoreOverrides.length + d.additionalCFs.length + d.excludedCFs.length;
    },

    // Allowed qualities in their qualityStructure order. Falls back to
    // profile.items if qualityStructure isn't populated (initial load).
    spOverviewQualities() {
      const items = (this.qualityStructure && this.qualityStructure.length)
        ? this.qualityStructure
        : (this.profileDetail?.detail?.profile?.items || []);
      const out = [];
      for (const item of items) {
        if (!item.allowed) continue;
        // Quality items can be either single qualities (item.quality.name)
        // or groups (item.name + item.items[]). For overview-rendering we
        // surface the top-level label.
        out.push(item.name || item.quality?.name || '');
      }
      return out.filter(n => n);
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
