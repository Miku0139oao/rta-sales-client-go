<script lang="ts">
  import { defaultSettings, normalizeSettings } from '../settings';
  import { backend } from '../backend';
  import { errorMessage } from '../i18n';
  import type { Translator } from '../i18n';
  import type { AppSettings } from '../types';

  export let t: Translator;
  export let settings: AppSettings;
  export let onChange: (settings: AppSettings) => void;

  let draft: AppSettings = { ...settings };
  let saved = false;
  let browsing = false;
  let mappingError = '';

  function updateNumber(key: 'maxJobs' | 'accountConcurrency', event: Event) {
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
    draft = { ...defaultSettings, locale: settings.locale };
    onChange(draft);
    saved = true;
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
    <section class="surface-card settings-card" aria-labelledby="language-heading">
      <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">translate</span></div>
      <div class="settings-content">
        <h2 id="language-heading">{t('settings.language')}</h2>
        <div class="field-group compact-field">
          <label for="locale">{t('settings.language')}</label>
          <select
            id="locale"
            value={draft.locale}
            onchange={(event) => { draft = { ...draft, locale: (event.currentTarget as HTMLSelectElement).value === 'en' ? 'en' : 'zh-TW' }; saved = false; }}
          >
            <option value="zh-TW">{t('settings.zhTW')}</option>
            <option value="en">{t('settings.english')}</option>
          </select>
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
              <option value="1">1</option>
              <option value="2">2</option>
              <option value="3">3</option>
              <option value="4">4</option>
            </select>
          </div>
        </div>
      </div>
    </section>

    <section class="surface-card settings-card" aria-labelledby="advanced-heading">
      <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">tune</span></div>
      <div class="settings-content">
        <h2 id="advanced-heading">{t('settings.advanced')}</h2>
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
