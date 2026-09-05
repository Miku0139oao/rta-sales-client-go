<script lang="ts">
  import { backend } from '../backend';
  import { modal } from '../modal';
  import { analysisTableTSV, workbookSnapshot, type AnalysisTable } from '../analysisTable';
  import type { Translator } from '../i18n';
  export let t: Translator;
  export let tables: AnalysisTable[];
  export let context: string[];
  export let filename: string;
  export let disabled = false;
  export let compact = false;
  export let onBusy: (busy: boolean) => void = () => undefined;
  let busy = false;
  let selectedId = '';
  let notice = '';
  let error = '';
  let manualCopy = '';
  $: if (!tables.some((table) => table.id === selectedId)) selectedId = tables[0]?.id ?? '';
  $: selected = tables.find((table) => table.id === selectedId) ?? tables[0];
  $: if (tables) { notice = ''; error = ''; }
  function closeCopy() { manualCopy = ''; }
  function selectText(node: HTMLTextAreaElement) { queueMicrotask(() => { node.focus(); node.select(); }); }
  async function exportExcel() {
    if (busy || disabled || !tables.some((table) => table.rows.length)) return;
    notice = ''; error = ''; busy = true; onBusy(true);
    try {
      const snapshot = workbookSnapshot(tables, context, filename);
      const path = await backend.exportSalesAnalysisWorkbook(snapshot);
      if (path) notice = t('data.saved', { path });
    } catch (caught) { error = t(caught instanceof Error && caught.message === 'table_limit' ? 'data.tooLarge' : 'data.exportError'); }
    finally { busy = false; onBusy(false); }
  }
  async function copyTable() {
    if (!selected || !selected.rows.length || busy || disabled) return;
    notice = ''; error = '';
    let text: string;
    try { workbookSnapshot([selected], [], 'copy.xlsx'); text = analysisTableTSV(selected); }
    catch { error = t('data.tooLarge'); return; }
    busy = true; onBusy(true);
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard_unavailable');
      await navigator.clipboard.writeText(text);
      notice = t('data.copied', { count: selected.rows.length });
    } catch { manualCopy = text; }
    finally { busy = false; onBusy(false); }
  }
</script>
<div class="data-actions" class:compact>
  <div class="action-row" title={t('data.scopeHint')}>
    <span class="data-hint">{t('data.scopeHint')}</span>
    {#if tables.length > 1}<select aria-label={t('data.copyWhich')} bind:value={selectedId} disabled={busy || disabled}>{#each tables as table}<option value={table.id}>{table.name}</option>{/each}</select>{/if}
    <button type="button" disabled={busy || disabled || !selected?.rows.length} onclick={() => void copyTable()}>{t('data.copy')}</button>
    <button type="button" disabled={busy || disabled || !tables.some((table) => table.rows.length)} onclick={() => void exportExcel()}>{t(busy ? 'common.loading' : 'data.excel')}</button>
  </div>
  {#if notice}<p role="status">{notice}</p>{/if}
  {#if error}<p role="alert">{error}</p>{/if}
</div>
{#if manualCopy}<dialog class="app-dialog" use:modal={{ onClose: closeCopy }} aria-labelledby="manual-copy-title"><div class="dialog-header"><h2 id="manual-copy-title">{t('data.manualCopy')}</h2></div><p>{t('data.manualCopyHint')}</p><textarea use:selectText readonly value={manualCopy} aria-label={t('data.copyContent')} rows="10"></textarea><div class="dialog-actions"><button type="button" onclick={closeCopy}>{t('common.close')}</button></div></dialog>{/if}
<style>
  .data-actions { position: relative; min-width: 0; margin: 0; }
  .action-row { display: flex; gap: 8px; align-items: center; justify-content: flex-end; flex-wrap: wrap; }
  .data-hint { margin-right: auto; color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
  .compact .data-hint { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); }
  button, select { min-height: 36px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 10px; padding: 7px 10px; background: var(--md-sys-color-surface); color: var(--md-sys-color-primary); font: inherit; font-size: 12px; font-weight: 650; }
  select { max-width: min(100%, 240px); }
  button { cursor: pointer; }
  button:disabled { opacity: .45; cursor: default; }
  button:focus-visible, select:focus-visible, textarea:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  p { font-size: 12px; overflow-wrap: anywhere; color: var(--md-sys-color-primary); }
  p[role="alert"] { color: var(--md-sys-color-error); }
  textarea { width: 100%; box-sizing: border-box; font-family: monospace; }
  @media (max-width: 520px) { .data-hint { flex-basis: 100%; } .action-row { justify-content: flex-start; } .compact .action-row { justify-content: flex-end; } }
</style>
