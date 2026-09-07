<script lang="ts">
  import { onMount, tick } from 'svelte';
  import AccountsPage from './lib/pages/AccountsPage.svelte';
  import AnalysisPage from './lib/pages/AnalysisPage.svelte';
  import ExcelPage from './lib/pages/ExcelPage.svelte';
  import ItemCodesPage from './lib/pages/ItemCodesPage.svelte';
  import SettingsPage from './lib/pages/SettingsPage.svelte';
  import UpdateNotice from './lib/UpdateNotice.svelte';
  import { translator } from './lib/i18n';
  import { isWebRuntime } from './lib/runtime';
  import { loadSettings, saveSettings } from './lib/settings';
  import { readWebBannerAck, writeWebBannerAck } from './lib/webBannerAck';
  import { applyTheme, resolveTheme, systemPrefersDark, watchSystemTheme } from './lib/theme';
  import type { AppSettings, Page, ThemePreference } from './lib/types';

  let activePage: Page = 'analysis';
  let settings = loadSettings();
  let t = translator(settings.locale);
  let systemDark = systemPrefersDark();
  let resolvedTheme = resolveTheme(settings.theme, systemDark);
  let excelBusy = false;
  let analysisBusy = false;
  let accountsBusy = false;
  let itemcodesBusy = false;
  let updateBusy = false;
  let settingsDirty = false;
  let mainContent: HTMLElement;
  let webBannerVisible = isWebRuntime() && !readWebBannerAck();
  let analysisCatalogEpoch = 0;
  let excelCatalogEpoch = 0;
  let mounted: Record<Page, boolean> = {
    analysis: true,
    excel: false,
    accounts: false,
    itemcodes: false,
    settings: false,
  };

  $: t = translator(settings.locale);
  $: resolvedTheme = resolveTheme(settings.theme, systemDark);
  $: applyTheme(resolvedTheme);
  $: navigationBusy = updateBusy || (activePage === 'excel' ? excelBusy : activePage === 'analysis' ? analysisBusy : activePage === 'accounts' ? accountsBusy : activePage === 'itemcodes' ? itemcodesBusy : false);
  $: if (typeof document !== 'undefined') {
    document.documentElement.lang = settings.locale === 'en' ? 'en' : 'zh-Hant';
    document.title = t('app.name');
  }

  const navigation: Array<{ id: Page; icon: string; label: string; shortLabel: string }> = [
    { id: 'analysis', icon: 'query_stats', label: 'nav.analysis', shortLabel: 'nav.analysisShort' },
    { id: 'excel', icon: 'table_view', label: 'nav.excel', shortLabel: 'nav.excelShort' },
    { id: 'accounts', icon: 'manage_accounts', label: 'nav.accounts', shortLabel: 'nav.accountsShort' },
    { id: 'itemcodes', icon: 'tag', label: 'nav.itemcodes', shortLabel: 'nav.itemcodesShort' },
    { id: 'settings', icon: 'settings', label: 'nav.settings', shortLabel: 'nav.settingsShort' },
  ];

  onMount(() => watchSystemTheme((dark) => {
    systemDark = dark;
  }));

  function acknowledgeWebBanner() {
    writeWebBannerAck();
    webBannerVisible = false;
  }

  function updateSettings(next: AppSettings) {
    settings = saveSettings(next);
  }

  function updateThemePreference(theme: ThemePreference) {
    settings = saveSettings({ ...settings, theme });
  }

  function toggleTheme() {
    updateThemePreference(resolvedTheme === 'dark' ? 'light' : 'dark');
  }

  function navAriaLabel(item: (typeof navigation)[number]): string {
    if (item.id === 'settings' && settingsDirty) return t('nav.settingsUnsaved');
    return t(item.label);
  }

  function relayMainWheel(event: WheelEvent) {
    if (event.deltaY === 0) return;
    const target = event.target as HTMLElement | null;
    const dialog = document.querySelector('dialog.app-dialog[open]');
    if (dialog) {
      if (scrollableAncestorCanConsume(target, dialog, event.deltaY)) return;
      event.preventDefault();
      return;
    }
    const nested = target?.closest('.pane-scroll, .table-scroll, .facet-options, .store-grid');
    if (nested instanceof HTMLElement && canScrollVertically(nested, event.deltaY)) return;
    const scroller = event.currentTarget as HTMLElement;
    if (!canScrollVertically(scroller, event.deltaY)) return;
    scroller.scrollTop += wheelDeltaY(event);
    event.preventDefault();
  }

  function scrollableAncestorCanConsume(target: HTMLElement | null, boundary: Element, deltaY: number): boolean {
    let current = target;
    while (current && boundary.contains(current)) {
      if (current.matches('.pane-scroll, .table-scroll, .facet-options, .store-grid, .export-dialog-scroll') && canScrollVertically(current, deltaY)) return true;
      if (current === boundary) break;
      current = current.parentElement;
    }
    return false;
  }

  function canScrollVertically(element: HTMLElement, deltaY: number): boolean {
    if (element.scrollHeight <= element.clientHeight + 1) return false;
    const style = getComputedStyle(element);
    if (style.overflowY !== 'auto' && style.overflowY !== 'scroll') return false;
    if (deltaY < 0) return element.scrollTop > 0;
    return element.scrollTop + element.clientHeight < element.scrollHeight - 1;
  }

  function wheelDeltaY(event: WheelEvent): number {
    if (event.deltaMode === 1) return event.deltaY * 16;
    if (event.deltaMode === 2) return event.deltaY * (event.currentTarget as HTMLElement).clientHeight;
    return event.deltaY;
  }

  async function navigateTo(page: Page) {
    if (navigationBusy && page !== activePage) return;
    const leaving = activePage;
    if (page === 'analysis' && (leaving === 'accounts' || leaving === 'itemcodes')) analysisCatalogEpoch += 1;
    if (page === 'excel' && leaving === 'accounts') excelCatalogEpoch += 1;
    mounted = { ...mounted, [page]: true };
    activePage = page;
    await tick();
    if (mainContent) mainContent.scrollTop = 0;
    document.documentElement.scrollTop = 0;
    document.body.scrollTop = 0;
    mainContent?.focus({ preventScroll: true });
  }
</script>

{#snippet navButtons(short: boolean)}
  {#each navigation as item}
    <button
      type="button"
      class:active={activePage === item.id}
      class:dirty={item.id === 'settings' && settingsDirty}
      aria-current={activePage === item.id ? 'page' : undefined}
      aria-label={navAriaLabel(item)}
      title={navigationBusy && item.id !== activePage ? t('nav.busy') : undefined}
      disabled={navigationBusy && item.id !== activePage}
      onclick={() => void navigateTo(item.id)}
    >
      <span class="nav-icon material-symbols-rounded" aria-hidden="true">{item.icon}</span>
      <span>{t(short ? item.shortLabel : item.label)}</span>
      {#if item.id === 'settings' && settingsDirty}<span class="nav-dirty-dot" aria-hidden="true"></span>{/if}
    </button>
  {/each}
{/snippet}

<div class="app-shell">
  <header class="top-app-bar">
    <a class="brand" href="#main-content" aria-label={t('app.name')}>
      <span class="brand-mark" aria-hidden="true">
        <svg class="brand-glyph" viewBox="0 0 24 24" aria-hidden="true">
          <rect x="4.2" y="13.4" width="4" height="6.4" rx="1.3" />
          <rect x="10" y="9.6" width="4" height="10.2" rx="1.3" />
          <rect x="15.8" y="5.6" width="4" height="14.2" rx="1.3" />
        </svg>
      </span>
      <span class="brand-copy">
        <strong>{t('app.name')}</strong>
      </span>
    </a>

    <md-icon-button
      class="theme-toggle"
      role="button"
      tabindex="0"
      aria-label={resolvedTheme === 'dark' ? t('theme.switchToLight') : t('theme.switchToDark')}
      title={resolvedTheme === 'dark' ? t('theme.switchToLight') : t('theme.switchToDark')}
      onclick={toggleTheme}
    >
      <span class="material-symbols-rounded" aria-hidden="true">{resolvedTheme === 'dark' ? 'light_mode' : 'dark_mode'}</span>
    </md-icon-button>
  </header>

  <div class="shell-body">
    <aside class="navigation-rail">
      <nav aria-label={t('nav.main')}>
        {@render navButtons(false)}
      </nav>
    </aside>

    <main id="main-content" bind:this={mainContent} tabindex="-1" onwheel={relayMainWheel}>
      <UpdateNotice {settings} details={activePage === 'settings'} busy={excelBusy || analysisBusy || accountsBusy || itemcodesBusy} onChange={updateSettings} onBusyChange={(busy) => (updateBusy = busy)} />
      {#if webBannerVisible}
        <div class="notice warning-notice web-preview-notice" role="status">
          <span class="material-symbols-rounded" aria-hidden="true">language</span>
          <div class="web-banner-copy">
            <strong>{t('web.bannerTitle')}</strong>
            <p>{t('web.bannerBody')}</p>
            <ul class="web-notice-list">
              <li>{t('web.noticeStore')}</li>
              <li>{t('web.noticeRecord')}</li>
              <li>{t('web.noticeSession')}</li>
              <li>{t('web.noticeLog')}</li>
            </ul>
          </div>
          <md-outlined-button type="button" onclick={acknowledgeWebBanner}>{t('web.bannerAck')}</md-outlined-button>
        </div>
      {/if}
      {#if mounted.excel}
        <div class="page-host" hidden={activePage !== 'excel'} inert={activePage !== 'excel' ? true : undefined}>
          <ExcelPage
            {t}
            {settings}
            catalogEpoch={excelCatalogEpoch}
            onBusyChange={(busy) => (excelBusy = busy)}
            onGoToAccounts={() => { if (!excelBusy) void navigateTo('accounts'); }}
          />
        </div>
      {/if}
      {#if mounted.analysis}
        <div class="page-host" hidden={activePage !== 'analysis'} inert={activePage !== 'analysis' ? true : undefined}>
          <AnalysisPage
            {t}
            {settings}
            catalogEpoch={analysisCatalogEpoch}
            onBusyChange={(busy) => (analysisBusy = busy)}
            onSettingsChange={updateSettings}
            onGoToAccounts={() => { if (!analysisBusy) void navigateTo('accounts'); }}
          />
        </div>
      {/if}
      {#if mounted.accounts}
        <div class="page-host" hidden={activePage !== 'accounts'} inert={activePage !== 'accounts' ? true : undefined}>
          <AccountsPage {t} locale={settings.locale} onBusyChange={(busy) => (accountsBusy = busy)} />
        </div>
      {/if}
      {#if mounted.itemcodes}
        <div class="page-host" hidden={activePage !== 'itemcodes'} inert={activePage !== 'itemcodes' ? true : undefined}>
          <ItemCodesPage {t} locale={settings.locale} onBusyChange={(busy) => (itemcodesBusy = busy)} />
        </div>
      {/if}
      {#if mounted.settings}
        <div class="page-host" hidden={activePage !== 'settings'} inert={activePage !== 'settings' ? true : undefined}>
          <SettingsPage {t} {settings} onChange={updateSettings} onThemeChange={updateThemePreference} onDirtyChange={(dirty) => (settingsDirty = dirty)} />
        </div>
      {/if}
    </main>
  </div>

  <nav class="bottom-navigation" aria-label={t('nav.main')}>
    {@render navButtons(true)}
  </nav>
</div>
