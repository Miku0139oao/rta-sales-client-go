<script lang="ts">
  import AccountsPage from './lib/pages/AccountsPage.svelte';
  import ExcelPage from './lib/pages/ExcelPage.svelte';
  import SettingsPage from './lib/pages/SettingsPage.svelte';
  import { isBrowserPreview } from './lib/backend';
  import { translator } from './lib/i18n';
  import { loadSettings, saveSettings } from './lib/settings';
  import type { AppSettings, Page } from './lib/types';

  let activePage: Page = 'excel';
  let settings = loadSettings();
  let t = translator(settings.locale);
  let mockMode = isBrowserPreview();
  let excelBusy = false;
  let accountsBusy = false;

  $: t = translator(settings.locale);
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

  function updateSettings(next: AppSettings) {
    settings = saveSettings(next);
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

    {#if mockMode}
      <span class="mode-badge"><span class="material-symbols-rounded" aria-hidden="true">preview</span>{t('app.mock')}</span>
    {/if}
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
            onclick={() => (activePage = item.id)}
          >
            <span class="nav-icon material-symbols-rounded" aria-hidden="true">{item.icon}</span>
            <span>{t(item.label)}</span>
          </button>
        {/each}
      </nav>
    </aside>

    <main id="main-content" tabindex="-1">
      {#if activePage === 'excel'}
        <ExcelPage
          {t}
          {settings}
          onBusyChange={(busy) => (excelBusy = busy)}
          onGoToAccounts={() => { if (!excelBusy) activePage = 'accounts'; }}
        />
      {:else if activePage === 'accounts'}
        <AccountsPage {t} locale={settings.locale} onBusyChange={(busy) => (accountsBusy = busy)} />
      {:else}
        <SettingsPage {t} {settings} onChange={updateSettings} />
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
        onclick={() => (activePage = item.id)}
      >
        <span class="material-symbols-rounded" aria-hidden="true">{item.icon}</span>
        <span>{t(item.label)}</span>
      </button>
    {/each}
  </nav>
</div>
