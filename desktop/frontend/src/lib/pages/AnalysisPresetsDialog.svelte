<script lang="ts">
  import { modal } from '../modal';
  import type { Translator } from '../i18n';
  import type { ManCodeGroup } from '../types';
  import {
    loadAnalysisPresets, putAnalysisPreset, saveAnalysisPresets, resolvePresetQuery, setAnalysisPresetPinned,
    MAX_ANALYSIS_PRESETS, PRESET_NAME_MAX, PRESET_CATEGORIES, PresetError,
    type AnalysisPreset, type AnalysisPresetDraft, type PresetMonthMode,
  } from '../analysisPresets';

  export let t: Translator;
  export let locale = 'zh-TW';
  export let draft: AnalysisPresetDraft | undefined;
  export let groups: ManCodeGroup[];
  export let onClose: () => void;
  export let onApply: (preset: AnalysisPreset) => Promise<string | undefined>;

  let presets: AnalysisPreset[] = [];
  let selectedId = '';
  let newName = '';
  let renameName = '';
  let monthMode: PresetMonthMode = 'fixed';
  let action: '' | 'rename' | 'delete' | 'update' = '';
  let busy = false;
  let loadFailed = false;
  let error = '';
  let notice = '';
  $: selected = presets.find((preset) => preset.id === selectedId);
  $: currentDraft = draft && {
    ...draft,
    query: { ...draft.query, monthMode, ...(monthMode !== 'fixed' ? { periodMode: 'month' as const, weekCompare: false } : {}) },
  };

  function reload() {
    try { presets = loadAnalysisPresets(); selectedId = presets[0]?.id ?? ''; loadFailed = false; error = ''; }
    catch { loadFailed = true; error = t('presets.readError'); }
  }
  reload();

  function periodLabel(value: AnalysisPresetDraft): string {
    const query = resolvePresetQuery(value.query);
    if (query.monthMode !== 'fixed') return `${t(`presets.month.${query.monthMode}`)} · ${query.month}`;
    return query.periodMode === 'month' ? query.month : `${query.from} — ${query.to}`;
  }
  function filterLabels(value: AnalysisPresetDraft): string[] {
    return [
      ...(value.filters.search ? [`${t('analysis.search')}: ${value.filters.search}`] : []),
      ...(value.filters.groupId ? [groups.find((group) => group.id === value.filters.groupId)?.name ?? t('presets.missingGroupLabel')] : []),
      ...PRESET_CATEGORIES.flatMap((key) => value.filters.categories[key]),
    ];
  }
  function showError(caught: unknown) {
    error = t(caught instanceof PresetError ? `presets.error.${caught.code}` : 'presets.writeError');
  }
  function saveNew() {
    if (busy || loadFailed || !currentDraft) return;
    error = ''; notice = '';
    try {
      presets = putAnalysisPreset(presets, currentDraft, newName);
      selectedId = presets[presets.length - 1]!.id;
      newName = ''; action = ''; notice = t('presets.saved');
    } catch (caught) { showError(caught); }
  }
  function begin(next: typeof action) {
    action = next; renameName = selected?.name ?? ''; error = ''; notice = '';
  }
  function confirmAction() {
    if (busy || loadFailed || !selected) return;
    error = ''; notice = '';
    try {
      if (action === 'delete') {
        presets = saveAnalysisPresets(presets.filter((preset) => preset.id !== selected!.id));
        selectedId = presets[0]?.id ?? '';
        notice = t('presets.deleted');
      } else if (action === 'rename') {
        presets = putAnalysisPreset(presets, selected, renameName, selected.id);
        notice = t('presets.renamed');
      } else if (action === 'update' && currentDraft) {
        presets = putAnalysisPreset(presets, currentDraft, selected.name, selected.id);
        notice = t('presets.updated');
      }
      action = '';
    } catch (caught) { showError(caught); }
  }
  function togglePin() {
    if (!selected || busy || loadFailed) return;
    error = ''; notice = '';
    try { presets = setAnalysisPresetPinned(presets, selected.id, !selected.pinned); }
    catch (caught) { showError(caught); }
  }
  async function apply() {
    if (!selected || busy || loadFailed) return;
    busy = true; error = ''; notice = '';
    try {
      const problem = await onApply(selected);
      if (problem) error = problem;
      else onClose();
    } catch { error = t('presets.applyError'); }
    finally { busy = false; }
  }
</script>

<dialog use:modal={{ busy, onClose }} class="app-dialog presets-dialog" aria-modal="true" aria-labelledby="presets-title" aria-describedby="presets-help">
  <div class="dialog-header">
    <h2 id="presets-title">{t('presets.title')}</h2>
    <button type="button" class="close-button" disabled={busy} aria-label={t('common.close')} onclick={onClose}><span class="material-symbols-rounded" aria-hidden="true">close</span></button>
  </div>
  <div class="presets-body pane-scroll">
    <p id="presets-help" class="muted">{t('presets.help')}</p>
    {#if error}<div class="notice error-notice" role="alert">{error}{#if loadFailed}<button type="button" onclick={reload}>{t('presets.retryRead')}</button>{/if}</div>{/if}
    {#if notice}<div class="notice success-notice" role="status">{notice}</div>{/if}

    <section class="preset-section" aria-labelledby="saved-presets-label">
      <div class="section-title"><h3 id="saved-presets-label">{t('presets.savedList')}</h3><span>{presets.length} / {MAX_ANALYSIS_PRESETS}</span></div>
      {#if presets.length}
        <label for="preset-selected">{t('presets.choose')}</label>
        <select id="preset-selected" data-autofocus bind:value={selectedId} disabled={busy || loadFailed} onchange={() => { action = ''; error = ''; notice = ''; }}>
          {#each presets as preset (preset.id)}<option value={preset.id}>{preset.name}</option>{/each}
        </select>
        {#if selected}
          <div class="preset-summary">
            <strong>{selected.query.profileName || selected.query.profileId}</strong>
            <span>{periodLabel(selected)}{selected.query.periodMode === 'range' && selected.query.weekCompare ? ` · ${t('analysis.weekMode')}` : ''}</span>
            <span>{t('analysis.selectedStores', { count: selected.query.storeIds.length })} · {selected.query.storeIds.join(', ')}</span>
            {#if selected.lastUsedAt !== undefined}<span>{t('presets.lastUsed', { date: new Date(selected.lastUsedAt).toLocaleString(locale) })}</span>{/if}
          </div>
          {#if filterLabels(selected).length}
            <details><summary>{t('presets.filters', { count: filterLabels(selected).length })}</summary><ul>{#each filterLabels(selected) as label}<li>{label}</li>{/each}</ul></details>
          {:else}<p class="muted">{t('presets.noFilters')}</p>{/if}
          <div class="preset-actions">
            <button type="button" class="primary" disabled={busy || loadFailed} onclick={() => void apply()}>{t(busy ? 'presets.applying' : 'presets.apply')}</button>
            <button type="button" disabled={busy || loadFailed} aria-pressed={Boolean(selected.pinned)} onclick={togglePin}>{t(selected.pinned ? 'presets.unpin' : 'presets.pin')}</button>
            <button type="button" disabled={busy || loadFailed} onclick={() => begin('rename')}>{t('presets.rename')}</button>
            <button type="button" disabled={busy || loadFailed || !currentDraft} onclick={() => begin('update')}>{t('presets.update')}</button>
            <button type="button" class="danger" disabled={busy || loadFailed} onclick={() => begin('delete')}>{t('presets.delete')}</button>
          </div>
          {#if action}
            <form class="preset-confirm" onsubmit={(event) => { event.preventDefault(); confirmAction(); }}>
              {#if action === 'rename'}
                <label for="preset-rename">{t('presets.newName')}</label>
                <input id="preset-rename" bind:value={renameName} maxlength={PRESET_NAME_MAX} disabled={busy} required />
              {:else}<p>{t(action === 'delete' ? 'presets.confirmDelete' : 'presets.confirmUpdate', { name: selected.name })}</p>{/if}
              <div class="preset-actions"><button type="submit" disabled={busy}>{t('presets.confirm')}</button><button type="button" disabled={busy} onclick={() => { action = ''; error = ''; }}>{t('common.cancel')}</button></div>
            </form>
          {/if}
        {/if}
      {:else}<p class="muted">{t('presets.empty')}</p>{/if}
    </section>

    <section class="preset-section" aria-labelledby="save-preset-label">
      <h3 id="save-preset-label">{t('presets.saveCurrent')}</h3>
      {#if currentDraft}
        <p class="muted">{currentDraft.query.profileName} · {periodLabel(currentDraft)} · {t('analysis.selectedStores', { count: currentDraft.query.storeIds.length })} · {t('presets.filters', { count: filterLabels(currentDraft).length })}</p>
        <form onsubmit={(event) => { event.preventDefault(); saveNew(); }}>
          <label for="preset-month-mode">{t('presets.dateRule')}</label>
          <select id="preset-month-mode" bind:value={monthMode} disabled={busy || loadFailed}>
            <option value="fixed">{t('presets.month.fixed')}</option><option value="current">{t('presets.month.current')}</option><option value="previous">{t('presets.month.previous')}</option>
          </select>
          <label for="preset-name">{t('presets.name')}</label>
          <div class="preset-name-row"><input id="preset-name" bind:value={newName} maxlength={PRESET_NAME_MAX} placeholder={t('presets.namePlaceholder')} disabled={busy || loadFailed} required /><button type="submit" disabled={busy || loadFailed || presets.length >= MAX_ANALYSIS_PRESETS}>{t('presets.saveNew')}</button></div>
        </form>
        {#if presets.length >= MAX_ANALYSIS_PRESETS}<p class="muted">{t('presets.error.limit')}</p>{/if}
      {:else}<p class="muted">{t('presets.cannotSave')}</p>{/if}
    </section>
    <p class="preset-storage-hint">{t('presets.localOnly')}</p>
  </div>
  <div class="dialog-actions"><button type="button" disabled={busy} onclick={onClose}>{t('common.close')}</button></div>
</dialog>

<style>
  .presets-dialog { width: min(680px, calc(100vw - 32px)); max-height: min(90dvh, 850px); padding: 0; overflow: hidden; }
  .presets-dialog[open] { display: flex; flex-direction: column; }
  .dialog-header, .dialog-actions { flex-shrink: 0; padding: 16px 22px; margin: 0; }
  .dialog-header { align-items: center; }
  .dialog-actions { border-top: 1px solid var(--md-sys-color-outline-variant); }
  .presets-dialog form { display: block; }
  .dialog-header h2 { margin: 0; font-size: 20px; }
  .presets-body { min-height: 0; overflow-y: auto; padding: 0 22px 16px; overscroll-behavior: contain; }
  .preset-section { padding: 16px 0; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
  .section-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  h3 { margin: 0 0 12px; font-size: 15px; }
  .section-title span, .muted, .preset-storage-hint { color: var(--md-sys-color-on-surface-variant); font-size: 12px; line-height: 1.7; }
  .muted { margin: 6px 0 12px; }
  .preset-storage-hint { margin-bottom: 0; }
  label { display: block; font-size: 12px; margin: 12px 0 6px; font-weight: 600; }
  input, select { width: 100%; min-width: 0; min-height: 42px; padding: 9px 12px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 10px; color: var(--md-sys-color-on-surface); background: var(--md-sys-color-surface); font: inherit; }
  button { min-height: 36px; padding: 7px 10px; border-radius: 10px; border: 1px solid var(--md-sys-color-outline-variant); background: var(--md-sys-color-surface); color: var(--md-sys-color-primary); font: inherit; font-size: 12px; font-weight: 650; cursor: pointer; }
  button.primary { background: var(--md-sys-color-primary); color: var(--md-sys-color-on-primary); border-color: transparent; }
  button.danger { color: var(--md-sys-color-error); }
  button:disabled { opacity: .5; cursor: default; }
  button:focus-visible, input:focus-visible, select:focus-visible, summary:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .close-button { display: grid; place-items: center; padding: 6px; border: 0; }
  .preset-summary { display: grid; gap: 4px; margin: 12px 0; font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; }
  .preset-summary strong { font-size: 14px; }
  .preset-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
  .preset-name-row { display: grid; grid-template-columns: 1fr auto; gap: 8px; }
  .preset-confirm { margin-top: 12px; padding: 12px; background: var(--md-sys-color-surface-container); border-radius: 12px; font-size: 13px; }
  .preset-confirm p { margin: 0; overflow-wrap: anywhere; }
  summary { font-size: 12px; cursor: pointer; }
  ul { padding-left: 20px; font-size: 12px; overflow-wrap: anywhere; }
  .notice { margin: 12px 0; font-size: 12px; overflow-wrap: anywhere; }
  @media (max-width: 520px) { .dialog-header, .dialog-actions { padding: 14px; } .presets-body { padding: 0 14px 14px; } .preset-actions button { flex: 1 1 auto; } }
</style>
