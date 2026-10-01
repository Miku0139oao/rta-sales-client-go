<script lang="ts">
  import { onMount } from 'svelte';
  import { modal } from '../modal';
  import { compareHistory, listHistory, deleteHistory, saveHistory, type HistoryReport } from '../reportHistory';
  import { downloadText } from '../webDownloads';
  export let locale: string;
  export let current: HistoryReport | undefined;
  export let onClose: () => void;
  let reports: HistoryReport[] = [];
  let left = '__current',
    right = '';
  let busy = false,
    error = '',
    notice = '',
    deleting = '';
  $: en = locale === 'en';
  $: a = left === '__current' ? current : reports.find((report) => report.id === left);
  $: b = reports.find((report) => report.id === right);
  $: comparison = a && b ? compareHistory(a, b) : undefined;
  const money = (value: number) => new Intl.NumberFormat(locale, { style: 'currency', currency: 'HKD' }).format(value);
  async function reload() {
    reports = await listHistory();
    if (!current && !reports.some((report) => report.id === left)) left = reports[0]?.id ?? '';
    if (!reports.some((report) => report.id === right)) right = reports.find((report) => report.id !== left)?.id ?? '';
  }
  async function action(task: () => Promise<void>) {
    if (busy) return;
    busy = true;
    error = '';
    notice = '';
    try {
      await task();
      await reload();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      busy = false;
    }
  }
  async function save() {
    if (!current) return;
    await action(async () => {
      await saveHistory(current!);
      notice = en ? 'Report saved locally' : '報表已儲存於本機';
    });
  }
  onMount(() => {
    void action(async () => {});
  });
</script>

<dialog class="app-dialog history-dialog" use:modal={{ busy, onClose }} aria-labelledby="history-title">
  <div class="dialog-header">
    <h2 id="history-title">{en ? 'Report history and comparison' : '歷史報表與比較'}</h2>
    <button type="button" disabled={busy} onclick={onClose}>{en ? 'Close' : '關閉'}</button>
  </div>
  <div class="body">
    <p>
      {en
        ? 'Saved summaries stay on this device. Maximum 20 reports. Clearing browser data removes web history.'
        : '摘要僅儲存於目前裝置，上限 20 份；清除瀏覽器資料會刪除網頁版歷史。'}
    </p>
    {#if error}<div class="notice error-notice" role="alert">
        {error}<button type="button" onclick={() => void action(async () => {})}>{en ? 'Retry' : '重試'}</button>
      </div>{/if}
    {#if notice}<p role="status">{notice}</p>{/if}
    <button
      type="button"
      disabled={busy || !current || reports.some((report) => report.id === current?.id)}
      onclick={() => void save()}>{en ? 'Save current finished report' : '儲存目前完整報表'}</button
    >
    {#if !current}<p>
        {en
          ? 'Finish a query to save its report. Existing reports can still be compared.'
          : '完成查詢後即可儲存；仍可比較已存報表。'}
      </p>{/if}
    <div class="selection">
      <label
        >{en ? 'Report A' : '報表 A'}<select bind:value={left} disabled={busy}
          >{#if current}<option value="__current"
              >{en ? 'Current report' : '目前報表'} · {current.from} — {current.to}</option
            >{/if}{#each reports as report}<option value={report.id}
              >{report.name} · {new Date(report.savedAt).toLocaleString(locale)}</option
            >{/each}</select
        ></label
      ><label
        >{en ? 'Report B' : '報表 B'}<select bind:value={right} disabled={busy}
          ><option value="">{en ? 'Choose a report' : '選擇報表'}</option>{#each reports as report}<option
              value={report.id}>{report.name} · {new Date(report.savedAt).toLocaleString(locale)}</option
            >{/each}</select
        ></label
      >
    </div>
    {#if a && b}
      <p>{a.from} — {a.to} / {b.from} — {b.to}</p>
      <p>{a.scopeLabel}</p>
      {#if comparison?.comparable}
        <div class="metrics">
          <span>A <strong>{money(a.totals.netSalesAmount)}</strong></span><span
            >B <strong>{money(b.totals.netSalesAmount)}</strong></span
          ><span>{en ? 'Difference' : '差額'} <strong>{money(comparison.difference!)}</strong></span><span
            >{en ? 'Change' : '變化'}
            <strong
              >{comparison.percent === undefined
                ? '—'
                : new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 2 }).format(
                    comparison.percent,
                  )}</strong
            ></span
          >
        </div>
        <p>
          {en
            ? 'A minus B; percentage uses absolute B. A zero baseline has no percentage.'
            : '差額為 A 減 B，百分比以 B 的絕對值為基準；基準為零時不顯示百分比。'}
        </p>
        <div class="table-scroll">
          <table>
            <thead
              ><tr><th>{en ? 'Store' : '門店'}</th><th>A</th><th>B</th><th>{en ? 'Difference' : '差額'}</th></tr></thead
            ><tbody
              >{#each a.stores as store}{@const prior = b.stores.find((row) => row.businessId === store.businessId)}<tr
                  ><td>{store.label}</td><td>{money(store.totals.netSalesAmount)}</td><td
                    >{prior ? money(prior.totals.netSalesAmount) : '—'}</td
                  ><td>{prior ? money(store.totals.netSalesAmount - prior.totals.netSalesAmount) : '—'}</td></tr
                >{/each}</tbody
            >
          </table>
        </div>
      {:else}<div class="notice warning-notice">
          {en
            ? 'Store/account/filter scope differs or data is incomplete. Totals cannot be compared.'
            : '帳號、門店或篩選範圍不同，或資料不完整，因此不計算比較差額。'}
        </div>{/if}
    {/if}
    <h3>{en ? 'Saved reports' : '已儲存報表'} ({reports.length}/20)</h3>
    {#each reports as report}<div class="record">
        <div>
          <strong>{report.name}</strong><small
            >{report.scopeLabel} · {new Date(report.savedAt).toLocaleString(locale)}</small
          >
        </div>
        <button
          type="button"
          onclick={() => downloadText(`RTA-history-${report.from}-${report.id}.json`, JSON.stringify(report, null, 2))}
          >{en ? 'Export backup' : '匯出備份'}</button
        ><button type="button" disabled={busy} onclick={() => (deleting = report.id)}>{en ? 'Delete' : '刪除'}</button
        >{#if deleting === report.id}<span>{en ? 'Delete this saved report?' : '刪除此份歷史報表？'}</span><button
            type="button"
            disabled={busy}
            onclick={() =>
              void action(async () => {
                await deleteHistory(report.id);
                deleting = '';
              })}>{en ? 'Confirm deletion' : '確認刪除'}</button
          ><button type="button" onclick={() => (deleting = '')}>{en ? 'Cancel' : '取消'}</button>{/if}
      </div>{:else}<p>{en ? 'No saved reports yet.' : '尚未儲存報表。'}</p>{/each}
  </div>
</dialog>

<style>
  .history-dialog {
    width: min(960px, calc(100vw - 32px));
  }
  .body {
    padding: 20px;
    max-height: 75vh;
    overflow: auto;
  }
  button,
  select {
    font: inherit;
    color: var(--md-sys-color-on-surface);
    background: var(--md-sys-color-surface-container);
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: 12px;
    padding: 9px 12px;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .selection,
  .metrics {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
    margin: 20px 0;
  }
  label {
    display: grid;
    gap: 8px;
    flex: 1 1 240px;
    min-width: 0;
    max-width: 100%;
  }
  select {
    width: 100%;
    min-width: 0;
    max-width: 100%;
  }
  .metrics span {
    display: grid;
    gap: 8px;
    flex: 1;
  }
  .record {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    border-top: 1px solid var(--md-sys-color-outline-variant);
    padding: 14px 0;
  }
  .record div {
    flex: 1 1 200px;
    min-width: 0;
  }
  small {
    display: block;
    margin-top: 6px;
    overflow-wrap: anywhere;
  }
  .table-scroll {
    overflow: auto;
  }
  table {
    border-collapse: collapse;
    width: 100%;
  }
  th,
  td {
    padding: 12px;
    border-bottom: 1px solid var(--md-sys-color-outline-variant);
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  th:first-child,
  td:first-child {
    text-align: left;
  }
  button:focus-visible,
  select:focus-visible {
    outline: 2px solid var(--md-sys-color-primary);
    outline-offset: 3px;
  }
</style>
