<script lang="ts">
  import { defaultSettings, normalizeSettings } from '../settings';
  import { backend } from '../backend';
  import { errorMessage } from '../i18n';
  import type { Translator } from '../i18n';
  import type { AppSettings, ThemePreference } from '../types';

  export let t: Translator;
  export let settings: AppSettings;
  export let onChange: (settings: AppSettings) => void;
  export let onThemeChange: (theme: ThemePreference) => void;

  let draft: AppSettings = { ...settings };
  let saved = false;
  let browsing = false;
  let mappingError = '';

  $: if (settings.theme !== draft.theme) {
    draft = { ...draft, theme: settings.theme };
  }

  function updateNumber(key: 'maxJobs' | 'accountConcurrency' | 'simulateStoreCount', event: Event) {
    draft = { ...draft, [key]: Number((event.currentTarget as HTMLInputElement | HTMLSelectElement).value) };
    saved = false;
  }

  function save() {
    if (draft.useLocalMapping && !draft.mappingPath.trim()) {
      mappingError = t('settings.mappingRequired');
      return;
    }
    if (draft.useLocalMapping && !/\.(json|csv)$/i.test(draft.mappingPath.trim())) {
      mappingError = t('settings.mappingInvalid');
      return;
    }
    mappingError = '';
    draft = normalizeSettings(draft);
    onChange(draft);
    saved = true;
  }

  function reset() {
    draft = { ...defaultSettings, locale: settings.locale, theme: settings.theme };
    onChange(draft);
    saved = true;
  }

  function changeTheme(theme: ThemePreference) {
    if (draft.theme === theme) return;
    draft = { ...draft, theme };
    onThemeChange(theme);
  }

  async function browseMapping() {
    browsing = true;
    mappingError = '';
    try {
      const path = await backend.openMappingFile();
      if (path) draft = { ...draft, mappingPath: path };
    } catch (error) {
      mappingError = errorMessage(settings.locale, error);
    } finally {
      browsing = false;
    }
  }
</script>

<section class="page settings-page" aria-labelledby="settings-title">
  <div class="page-heading">
    <h1 id="settings-title">{t('settings.title')}</h1>
  </div>

  {#if saved}
    <div class="notice success-notice" role="status">
      <span class="material-symbols-rounded" aria-hidden="true">check_circle</span>
      <span>{t('settings.saved')}</span>
    </div>
  {/if}

  <form class="settings-grid" onsubmit={(event) => { event.preventDefault(); save(); }}>
    <section class="surface-card settings-card" aria-labelledby="appearance-heading">
      <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">palette</span></div>
      <div class="settings-content">
        <h2 id="appearance-heading">{t('settings.appearance')}</h2>
        <div class="appearance-fields">
          <div class="field-group">
            <span class="field-label" id="theme-label">{t('settings.theme')}</span>
            <div class="theme-options" role="radiogroup" aria-labelledby="theme-label">
              {#each [
                { value: 'system', icon: 'desktop_windows', label: 'theme.system' },
                { value: 'light', icon: 'light_mode', label: 'theme.light' },
                { value: 'dark', icon: 'dark_mode', label: 'theme.dark' },
              ] as option}
                <button
                  type="button"
                  class:active={draft.theme === option.value}
                  role="radio"
                  aria-checked={draft.theme === option.value}
                  onclick={() => changeTheme(option.value as ThemePreference)}
                >
                  <span class="material-symbols-rounded" aria-hidden="true">{option.icon}</span>
                  <span>{t(option.label)}</span>
                </button>
              {/each}
            </div>
          </div>
          <div class="field-group compact-field">
            <label for="locale">{t('settings.language')}</label>
            <select
              id="locale"
              value={draft.locale}
              onchange={(event) => {
                const locale = (event.currentTarget as HTMLSelectElement).value === 'en' ? 'en' : 'zh-TW';
                draft = { ...draft, locale };
                onChange({ ...settings, locale });
              }}
            >
              <option value="zh-TW">{t('settings.zhTW')}</option>
              <option value="en">{t('settings.english')}</option>
            </select>
          </div>
        </div>
      </div>
    </section>

    <section class="surface-card settings-card" aria-labelledby="performance-heading">
      <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">speed</span></div>
      <div class="settings-content">
        <h2 id="performance-heading">{t('settings.performance')}</h2>
        <div class="settings-fields">
          <div class="field-group">
            <label for="max-jobs">{t('settings.maxJobs')}</label>
            <input
              id="max-jobs"
              type="number"
              min="1"
              max="2000"
              step="1"
              value={draft.maxJobs}
              oninput={(event) => updateNumber('maxJobs', event)}
            />
          </div>
          <div class="field-group">
            <label for="account-concurrency">{t('settings.concurrency')}</label>
            <select id="account-concurrency" value={draft.accountConcurrency} onchange={(event) => updateNumber('accountConcurrency', event)}>
              <option value="8">8</option>
              <option value="16">16</option>
              <option value="32">32</option>
              <option value="48">48</option>
              <option value="64">64</option>
              <option value="80">80</option>
              <option value="128">128</option>
              <option value="160">160</option>
            </select>
          </div>
        </div>
      </div>
    </section>

    <section class="surface-card settings-card" aria-labelledby="advanced-heading">
      <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">tune</span></div>
      <div class="settings-content">
        <h2 id="advanced-heading">{t('settings.advanced')}</h2>
        <div class="field-group">
          <label for="simulate-stores">{t('settings.simulateStores')}</label>
          <select
            id="simulate-stores"
            value={draft.simulateStoreCount}
            onchange={(event) => updateNumber('simulateStoreCount', event)}
          >
            <option value="0">{t('settings.simulateStoresOff')}</option>
            <option value="16">{t('settings.simulateStores16')}</option>
          </select>
          <small class="field-hint">{t('settings.simulateStoresHint')}</small>
        </div>
        <div class="setting-row">
          <strong>{t('settings.localMapping')}</strong>
          <md-switch
            aria-label={t('settings.localMapping')}
            selected={draft.useLocalMapping}
            onclick={() => { draft = { ...draft, useLocalMapping: !draft.useLocalMapping }; saved = false; }}
          ></md-switch>
        </div>
        {#if draft.useLocalMapping}
          <div class="field-group mapping-field">
            <label for="mapping-path">{t('settings.mappingFile')}</label>
            <div class="path-picker">
              <input
                id="mapping-path"
                value={draft.mappingPath}
                oninput={(event) => { draft = { ...draft, mappingPath: (event.currentTarget as HTMLInputElement).value }; mappingError = ''; saved = false; }}
                aria-describedby={mappingError ? 'mapping-path-error' : undefined}
                aria-invalid={Boolean(mappingError)}
              />
              <md-outlined-button type="button" onclick={browseMapping} disabled={browsing}>
                <span class="material-symbols-rounded" slot="icon">folder_open</span>{t('settings.browse')}
              </md-outlined-button>
            </div>
            {#if mappingError}<small id="mapping-path-error" class="field-error">{mappingError}</small>{/if}
          </div>
        {/if}
      </div>
    </section>

    <div class="form-actions sticky-actions">
      <md-text-button type="button" onclick={reset}>{t('settings.reset')}</md-text-button>
      <md-filled-button type="button" onclick={save}><span class="material-symbols-rounded" slot="icon">save</span>{t('common.save')}</md-filled-button>
    </div>
  </form>
</section>
