export default {
  state: {
    _navSkipPush: false,
  },
  methods: {
    // Legacy tab-switching helpers are kept for older call sites. New
    // navigation should use real anchors from navHref(), with section/app
    // side effects attached to state watchers in init().
    switchTab(tab) {
      this.debugLog('UI', `Tab: ${tab}`);
      this.currentTab = tab;
      localStorage.setItem('clonarr_tab', tab);
      this.profileDetail = null;
      this.syncPlan = null;
      this.syncResult = null;
      // Auto-select maintenance instance for this legacy app tab if only one.
      const typeInsts = this.instances.filter(i => i.type === tab);
      if (typeInsts.length === 1 && this.maintenanceInstanceId !== typeInsts[0].id) {
        this.maintenanceInstanceId = typeInsts[0].id;
        this.cleanupInstanceId = typeInsts[0].id;
        this.loadCleanupKeep();
        this.loadCleanupCFNames();
      }
    },

    switchSection(section) {
      this.debugLog('UI', `Section: ${section}`);
      this.currentSection = section;
      localStorage.setItem('clonarr_section', section);
      this.profileDetail = null;
      this.syncPlan = null;
      this.syncResult = null;
      this.pushNav();
    },

    switchAppType(appType) {
      // Guard unsaved CF Group Builder work: the builder is app-type-scoped,
      // so switching triggers cfgbLoad → cfgbReset which would discard an
      // in-flight edit. Warn via the styled confirm modal (browser's native
      // confirm() was jarring and didn't match the rest of the app).
      const shouldPrompt = this.currentSection === 'advanced'
        && this.advancedTab === 'group-builder'
        && appType !== this.activeAppType
        && typeof this.cfgbIsDirty === 'function' && this.cfgbIsDirty();
      if (shouldPrompt) {
        const label = this.cfgbEditingId
          ? 'changes to "' + (this.cfgbName || '(unnamed)') + '"'
          : 'the unsaved cf-group draft';
        this.confirmModal = {
          show: true,
          title: 'Discard unsaved cf-group work?',
          message: 'Switch to ' + appType + ' and discard ' + label + '?\n\nThe saved copy on disk (if any) is unaffected.',
          confirmLabel: 'Switch to ' + appType,
          onConfirm: () => this._doSwitchAppType(appType),
          onCancel: () => {},
        };
        return;
      }
      this._doSwitchAppType(appType);
    },

    _doSwitchAppType(appType) {
      this.debugLog('UI', `App type: ${appType}`);
      this.activeAppType = appType;
      localStorage.setItem('clonarr_appType', appType);
      this.pushNav();
      this.profileDetail = null;
      this.syncPlan = null;
      this.syncResult = null;
      // Auto-select maintenance instance for this type
      const typeInsts = this.instances.filter(i => i.type === appType);
      if (typeInsts.length === 1) {
        this.maintenanceInstanceId = typeInsts[0].id;
        this.cleanupInstanceId = typeInsts[0].id;
        this.loadCleanupKeep();
        this.loadCleanupCFNames();
      }
      // Reload app-scoped Advanced data. The CF Group Builder
      // pulls CFs, profiles, and saved groups per Radarr/Sonarr — without this
      // the Radarr list keeps showing when the user flips to Sonarr.
      // Scoring Sandbox has the same issue; reload it too for parity.
      if (this.currentSection === 'advanced') {
        if (this.advancedTab === 'group-builder') this.cfgbLoad(appType);
        else if (this.advancedTab === 'scoring') this.loadSandbox(appType);
      }
    },

    // --- v3 sidebar collapse ---
    // localStorage-backed boolean; flipped by the Collapse button in the
    // sidebar header and by Ctrl/Cmd+B keyboard shortcut.
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed;
      localStorage.setItem('clonarr-sidebar-collapsed', this.sidebarCollapsed ? '1' : '0');
    },

    // --- Hash routing (back/forward, bookmarks, copyable nav links) ---
    // Hash format: #appType/section[/subtab] — e.g. #radarr/profiles/compare, #settings/prowlarr, #about
    buildNavHash() {
      const s = this.currentSection;
      if (s === 'settings') return '#settings/' + (this.settingsSection || 'instances');
      if (s === 'about') return '#about';
      const app = this.activeAppType;
      let hash = '#' + app + '/' + s;
      if (s === 'profiles') hash += '/' + (this.getProfileTab(app) || 'trash-sync');
      else if (s === 'advanced') hash += '/' + (this.advancedTab || 'builder');
      return hash;
    },

    // navHref builds the hash that a target section/sub-tab would produce,
    // without mutating any state. Used by nav anchors so right-click → "Open
    // in new tab" and "Copy link address" work, and the browser can show the
    // URL on hover.
    //
    // opts: { appType, profileTab, advancedTab, settingsSection } — each
    // defaults to the current state when omitted.
    navHref(section, opts = {}) {
      if (section === 'settings') {
        return '#settings/' + (opts.settingsSection || this.settingsSection || 'instances');
      }
      if (section === 'about') return '#about';
      const app = opts.appType || this.activeAppType;
      let hash = '#' + app + '/' + section;
      if (section === 'profiles') {
        hash += '/' + (opts.profileTab || this.getProfileTab(app) || 'trash-sync');
      } else if (section === 'advanced') {
        hash += '/' + (opts.advancedTab || this.advancedTab || 'builder');
      }
      return hash;
    },

    // cfgbNeedsConfirm intercepts an app-type anchor click when the CF Group
    // Builder has unsaved work. Returns true (and pops a confirm modal) only
    // for plain left-clicks; modifier-clicks (Ctrl/Cmd/Shift/middle-click) are
    // allowed through so right-click → "Open in new tab" preserves the dirty
    // draft in the original tab.
    cfgbNeedsConfirm($event, appType) {
      if ($event.metaKey || $event.ctrlKey || $event.shiftKey || $event.altKey || $event.button === 1) return false;
      if (this.currentSection !== 'advanced' || this.advancedTab !== 'group-builder') return false;
      if (appType === this.activeAppType) return false;
      if (typeof this.cfgbIsDirty !== 'function' || !this.cfgbIsDirty()) return false;
      const label = this.cfgbEditingId
        ? 'changes to "' + (this.cfgbName || '(unnamed)') + '"'
        : 'the unsaved cf-group draft';
      const targetHref = this.navHref('advanced', { appType, advancedTab: 'group-builder' });
      this.confirmModal = {
        show: true,
        title: 'Discard unsaved cf-group work?',
        message: 'Switch to ' + appType + ' and discard ' + label + '?\n\nThe saved copy on disk (if any) is unaffected.',
        confirmLabel: 'Switch to ' + appType,
        onConfirm: () => { location.hash = targetHref; },
        onCancel: () => {},
      };
      return true;
    },

    pushNav() {
      if (this._navSkipPush) return;
      const hash = this.buildNavHash();
      if (location.hash !== hash) history.pushState(null, '', hash);
    },

    restoreFromHash(hash) {
      if (!hash || hash === '#') return false;
      // Guard against the watch-loop: pushNav writes the hash, the browser
      // fires hashchange, this runs, watchers re-fire pushNav. Early-return
      // when the hash already matches the state we'd produce.
      if (hash === this.buildNavHash()) return true;
      const parts = hash.replace(/^#/, '').split('/');
      const validSections = ['profiles','custom-formats','quality-size','naming','maintenance','advanced','settings','about'];
      const validSettings = ['instances','trash','prowlarr','notifications','display','security','advanced'];
      const validProfileTabs = ['trash-sync','history','compare'];
      const validAdvancedTabs = ['builder','group-builder','scoring','import'];
      this._navSkipPush = true;
      try {
        if (parts[0] === 'settings') {
          this.currentSection = 'settings';
          if (parts[1] && validSettings.includes(parts[1])) this.settingsSection = parts[1];
        } else if (parts[0] === 'about') {
          this.currentSection = 'about';
        } else {
          const appType = parts[0];
          if (appType === 'radarr' || appType === 'sonarr') this.activeAppType = appType;
          if (parts[1] && validSections.includes(parts[1])) this.currentSection = parts[1];
          else return false;
          if (parts[2]) {
            if (this.currentSection === 'profiles' && validProfileTabs.includes(parts[2])) this.setProfileTab(this.activeAppType, parts[2]);
            else if (this.currentSection === 'advanced' && validAdvancedTabs.includes(parts[2])) this.advancedTab = parts[2];
          }
        }
        localStorage.setItem('clonarr_section', this.currentSection);
        localStorage.setItem('clonarr_appType', this.activeAppType);
        return true;
      } finally {
        this._navSkipPush = false;
      }
    },

    getProfileTab(appType) {
      return this.profileTabs[appType] || 'trash-sync';
    },

    setProfileTab(appType, tab) {
      this.profileTabs = { ...this.profileTabs, [appType]: tab };
    },

    getCompareInstanceId(appType) {
      return this.compareInstanceIds[appType] || '';
    },
    setCompareInstanceId(appType, id) {
      this.compareInstanceIds = { ...this.compareInstanceIds, [appType]: id };
    },
    getCompareInstance(appType) {
      const id = this.compareInstanceIds[appType];
      return id ? (this.instances.find(i => i.id === id) || null) : null;
    },

    // Sprint 2 — app-banner helpers. Banner has two display modes:
    // app-scoped (full swatch + breadcrumb) and global (plain section
    // text, no swatch). Settings + About are global; everything else
    // is app-scoped.
    isGlobalSection() {
      return this.currentSection === 'settings' || this.currentSection === 'about';
    },

    // Returns the breadcrumb text shown in the banner middle.
    // App-scoped sections: "Profiles / TRaSH Sync", "Custom Formats",
    //                      "Advanced / Profile Builder", etc.
    // Global sections: "Settings", "About".
    // The "App / " prefix is intentionally omitted — the banner swatch
    // already encodes the active app, repeating it as text is redundant.
    currentBreadcrumb() {
      const sectionLabels = {
        'profiles': 'Profiles',
        'custom-formats': 'Custom Formats',
        'quality-size': 'Quality Definitions',
        'naming': 'File Naming',
        'maintenance': 'Maintenance',
        'advanced': 'Advanced',
        'settings': 'Settings',
        'about': 'About',
      };
      const section = sectionLabels[this.currentSection] || '';
      if (this.isGlobalSection()) return section;

      if (this.currentSection === 'profiles') {
        const tab = this.getProfileTab(this.activeAppType);
        const tabLabel = { 'trash-sync': 'TRaSH Sync', 'history': 'History', 'compare': 'Compare' }[tab] || '';
        return tabLabel ? `${section} / ${tabLabel}` : section;
      }
      if (this.currentSection === 'advanced') {
        const tab = this.advancedTab;
        const tabLabel = { 'builder': 'Profile Builder', 'scoring': 'Scoring Sandbox', 'group-builder': 'CF Group Builder' }[tab] || '';
        return tabLabel ? `${section} / ${tabLabel}` : section;
      }
      return section;
    },

    // Sprint 2 slice 3 — auto-sync banner chip helpers.
    //
    // Aggregates the auto-sync rules belonging to the currently-active app
    // type (Radarr/Sonarr) and exposes a compact "last activity" summary
    // for the banner chip. Hidden on global sections (Settings, About)
    // and when no enabled rules exist for the active app — auto-sync is
    // per-app-instance, so it has no meaning on those pages.
    //
    // States:
    //   ok       — at least one rule has LastSyncTime, none have a current error
    //   failed   — at least one enabled rule has LastSyncError set
    //   paused   — autoSyncSettings.paused === true
    //   never    — rules exist but none have ever synced successfully

    // Rules scoped to the active Radarr/Sonarr app type. Auto-sync is
    // app-wide — the same schedule kicks all enabled rules across every
    // instance of the active app at once — so aggregating by app rather
    // than per-instance matches the schedule's actual behaviour, and
    // keeps the chip stable when section pickers point at different
    // instances.
    _autoSyncRulesForActiveApp() {
      const ids = new Set(this.instancesOfType(this.activeAppType).map(i => i.id));
      return (this.autoSyncRules || []).filter(r => ids.has(r.instanceId));
    },

    autoSyncChipVisible() {
      if (this.isGlobalSection()) return false;
      const rules = this._autoSyncRulesForActiveApp();
      return rules.length > 0;
    },

    autoSyncChipState() {
      if (this.autoSyncSettings && this.autoSyncSettings.paused) return 'paused';
      const rules = this._autoSyncRulesForActiveApp().filter(r => r.enabled);
      if (rules.length === 0) return 'paused';  // visible only when rules exist, so this means all disabled
      if (rules.some(r => r.lastSyncError)) return 'failed';
      if (rules.every(r => !r.lastSyncTime)) return 'never';
      return 'ok';
    },

    autoSyncChipLabel() {
      const state = this.autoSyncChipState();
      if (state === 'paused') return 'Auto-sync paused';
      if (state === 'never') return 'Auto-sync · never run';
      // Find the most-recent LastSyncTime across all enabled rules.
      const rules = this._autoSyncRulesForActiveApp().filter(r => r.enabled && r.lastSyncTime);
      let latest = null;
      for (const r of rules) {
        const t = new Date(r.lastSyncTime).getTime();
        if (!latest || t > latest) latest = t;
      }
      const ago = latest ? this.timeAgo(new Date(latest).toISOString()) : 'never';
      if (state === 'failed') return `Auto-sync failed · ${ago}`;
      return `Auto-sync · ${ago}`;
    },

    // Click → navigate to Profiles → History so the user can see what ran.
    // navHref already returns a leading-# hash; assign directly to
    // location.hash and the existing hashchange listener restores state.
    autoSyncChipClick() {
      window.location.hash = this.navHref('profiles', { profileTab: 'history' });
    },
  },
};
