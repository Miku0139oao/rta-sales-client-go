<script lang="ts">
  import { onMount, tick } from 'svelte';
  import AccountsPage from './lib/pages/AccountsPage.svelte';
  import ExcelPage from './lib/pages/ExcelPage.svelte';
  import SettingsPage from './lib/pages/SettingsPage.svelte';
  import { translator } from './lib/i18n';
  import { loadSettings, saveSettings } from './lib/settings';
  import { applyTheme, resolveTheme, systemPrefersDark, watchSystemTheme } from './lib/theme';
  import type { AppSettings, Page, ThemePreference } from './lib/types';

  let activePage: Page = 'excel';
  let settings = loadSettings();
  let t = translator(settings.locale);
  let systemDark = systemPrefersDark();
  let resolvedTheme = resolveTheme(settings.theme, systemDark);
  let excelBusy = false;
  let accountsBusy = false;
  let mainContent: HTMLElement;

  $: t = translator(settings.locale);
  $: resolvedTheme = resolveTheme(settings.theme, systemDark);
  $: applyTheme(resolvedTheme);
  $: navigationBusy = activePage === 'excel' ? excelBusy : activePage === 'accounts' ? accountsBusy : false;
  $: if (typeof document !== 'undefined') {
    document.documentElement.lang = settings.locale === 'en' ? 'en' : 'zh-Hant';
    document.title = t('app.name');
  }

  const navigation: Array<{ id: Page; icon: string; label: string }> = [
    { id: 'excel', icon: 'table_view', label: 'nav.excel' },
    { id: 'accounts', icon: 'manage_accounts', label: 'nav.accounts' },
    { id: 'settings', icon: 'settings', label: 'nav.settings' },
  ];

  onMount(() => watchSystemTheme((dark) => {
    systemDark = dark;
  }));

  function updateSettings(next: AppSettings) {
    settings = saveSettings(next);
  }

  function updateThemePreference(theme: ThemePreference) {
    settings = saveSettings({ ...settings, theme });
  }

  function toggleTheme() {
    updateThemePreference(resolvedTheme === 'dark' ? 'light' : 'dark');
  }

  async function navigateTo(page: Page) {
    if (navigationBusy && page !== activePage) return;
    activePage = page;
    await tick();
    document.documentElement.scrollTop = 0;
    document.body.scrollTop = 0;
    mainContent?.focus({ preventScroll: true });
  }
</script>

<div class="app-shell">
  <header class="top-app-bar">
    <a class="brand" href="#main-content" aria-label={t('app.name')}>
      <span class="brand-mark" aria-hidden="true"><span class="material-symbols-rounded">finance_mode</span></span>
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
        {#each navigation as item}
          <button
            type="button"
            class:active={activePage === item.id}
            aria-current={activePage === item.id ? 'page' : undefined}
            disabled={navigationBusy && item.id !== activePage}
            onclick={() => void navigateTo(item.id)}
          >
            <span class="nav-icon material-symbols-rounded" aria-hidden="true">{item.icon}</span>
            <span>{t(item.label)}</span>
          </button>
        {/each}
      </nav>
    </aside>

    <main id="main-content" bind:this={mainContent} tabindex="-1">
      {#if activePage === 'excel'}
        <ExcelPage
          {t}
          {settings}
          onBusyChange={(busy) => (excelBusy = busy)}
          onGoToAccounts={() => { if (!excelBusy) void navigateTo('accounts'); }}
        />
      {:else if activePage === 'accounts'}
        <AccountsPage {t} locale={settings.locale} onBusyChange={(busy) => (accountsBusy = busy)} />
      {:else}
        <SettingsPage {t} {settings} onChange={updateSettings} onThemeChange={updateThemePreference} />
      {/if}
    </main>
  </div>

  <nav class="bottom-navigation" aria-label={t('nav.main')}>
    {#each navigation as item}
      <button
        type="button"
        class:active={activePage === item.id}
        aria-current={activePage === item.id ? 'page' : undefined}
        disabled={navigationBusy && item.id !== activePage}
        onclick={() => void navigateTo(item.id)}
      >
        <span class="material-symbols-rounded" aria-hidden="true">{item.icon}</span>
        <span>{t(item.label)}</span>
      </button>
    {/each}
  </nav>
</div>
