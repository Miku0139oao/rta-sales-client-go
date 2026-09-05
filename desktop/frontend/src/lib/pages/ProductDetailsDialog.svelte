<script lang="ts">
  import { modal } from '../modal';
  import type { Translator } from '../i18n';
  import type { SalesAnalysisPeriodResult } from '../types';
  import { exportSortFromView, productDetailHero, productDetailTables, productDetailViewTables } from '../productDetails';
  import { formatTableCell, sortAnalysisTable, type TableSort } from '../analysisTable';
  import { periodNeedsItemHydration } from '../salesAnalysisItems';
  import AnalysisDataTable from './AnalysisDataTable.svelte';
  import AnalysisDataActions from './AnalysisDataActions.svelte';
  export let t: Translator;
  export let locale: string;
  export let code: string;
  export let name: string;
  export let periods: SalesAnalysisPeriodResult[];
  export let initialPeriod = 'current';
  export let context: string[];
  export let failures: Record<string, string>;
  export let loading = false;
  export let pending = false;
  export let onRetry: () => void;
  export let onClose: () => void;
  export let onBusy: (busy: boolean) => void = () => undefined;
  let periodKey = initialPeriod;
  let busy = false;
  let sorts: Record<string, TableSort> = {};
  $: if (!periods.some((period) => period.key === periodKey)) periodKey = periods[0]?.key ?? 'current';
  $: rawTables = productDetailTables(code, periods, periodKey, t);
  $: hero = productDetailHero(code, periods, periodKey, t);
  $: viewTables = productDetailViewTables(rawTables).map((table) => sortAnalysisTable(table, sorts[table.id], locale));
  $: exportTables = rawTables.map((table) => sortAnalysisTable(table, exportSortFromView(table.id, sorts[table.id]), locale));
  $: missing = periods.some(periodNeedsItemHydration);
  $: detailContext = [
    ...context,
    `${t('data.code')}: ${code}`,
    `${t('analysis.article')}: ${name}`,
    t('data.productScope'),
    ...((pending || periods.some((period) => !period.complete)) ? [t('data.partial')] : []),
  ];
  function sort(id: string, value: TableSort) { sorts = { ...sorts, [id]: value }; }
  function metricClass(value: number | null, format: 'percent' | 'money' | 'number') {
    if (format !== 'percent' || value === null || value === 0) return '';
    return value > 0 ? 'positive' : 'negative';
  }
</script>
<dialog use:modal={{ busy, onClose }} class="app-dialog product-dialog" aria-modal="true" aria-labelledby="product-detail-title">
  <div class="dialog-header">
    <div>
      <h2 id="product-detail-title">{name}</h2>
      <p>{code} · {t('data.productDetails')}</p>
    </div>
    <button type="button" disabled={busy} aria-label={t('common.close')} onclick={onClose}>×</button>
  </div>
  <div class="product-body pane-scroll">
    {#if pending}<p class="detail-hint" role="status">{t('analysis.supplementingHint')}</p>{/if}
    {#if Object.keys(failures).length}
      <div class="detail-error" role="alert">
        <p>{t('analysis.itemsRecoveryHint')}</p>
        {#each periods.filter((period) => failures[period.key]) as period}<p>{period.label}: {failures[period.key]}</p>{/each}
        <button type="button" disabled={loading || busy} onclick={onRetry}>{t('analysis.retryItems')}</button>
      </div>
    {:else if missing}
      <p role="status">{t('analysis.loadingItems')}</p>
    {/if}
    <section class="product-hero" aria-label={t('data.productSnapshot')}>
      <p class="hero-period">{hero.currentLabel} · {hero.currentFrom} — {hero.currentTo} · {hero.currentStatus}</p>
      <dl class="hero-metrics">
        <div class="hero-primary">
          <dt>{t('analysis.netSales')}</dt>
          <dd>{formatTableCell(hero.amount, 'money', locale)}</dd>
        </div>
        <div>
          <dt>{t('analysis.netQuantity')}</dt>
          <dd>{formatTableCell(hero.quantity, 'number', locale)}</dd>
        </div>
        <div>
          <dt>{t('analysis.vsPrevious')}</dt>
          <dd class={metricClass(hero.vsPrevious, 'percent')}>{formatTableCell(hero.vsPrevious, 'percent', locale)}</dd>
        </div>
      </dl>
      <p class="hero-stores">
        {hero.storesWithSales === null ? t('data.storesWithSalesUnknown') : t('data.storesWithSales', { sold: hero.storesWithSales, total: hero.storesTotal })}
        · {hero.selectedLabel}
      </p>
    </section>
    <p class="detail-hint">{t('data.productScope')}</p>
    <AnalysisDataActions {t} tables={exportTables} context={detailContext} filename={`RTA-product-${code.replace(/[^a-zA-Z0-9_-]/g, '_').slice(0, 60)}.xlsx`} compact disabled={missing} onBusy={(value) => { busy = value; onBusy(value); }} />
    <section>
      <h3>{t('data.productPeriods')}</h3>
      <AnalysisDataTable table={viewTables[0]!} {t} {locale} sort={sorts['product-periods']} onSort={sort} />
    </section>
    <section>
      <div class="store-heading">
        <h3>{t('data.productStores')}</h3>
        <select aria-label={t('data.detailPeriod')} bind:value={periodKey}>
          {#each periods as period}<option value={period.key}>{period.label} · {period.from} — {period.to}</option>{/each}
        </select>
      </div>
      <AnalysisDataTable table={viewTables[1]!} {t} {locale} sort={sorts['product-stores']} onSort={sort} />
    </section>
  </div>
  <div class="dialog-actions"><button type="button" disabled={busy} onclick={onClose}>{t('common.close')}</button></div>
</dialog>
<style>
  .product-dialog { width: min(1140px, calc(100vw - 32px)); max-height: 90dvh; padding: 0; overflow: hidden; }
  .product-dialog[open] { display: flex; flex-direction: column; }
  .dialog-header, .dialog-actions { padding: 16px 22px; margin: 0; flex-shrink: 0; }
  .dialog-header h2 { overflow-wrap: anywhere; }
  .dialog-header p { margin: 6px 0 0; font-size: 12px; color: var(--md-sys-color-on-surface-variant); }
  .product-body { padding: 0 22px 16px; overflow: auto; min-height: 0; overscroll-behavior: contain; }
  section { border: 1px solid var(--md-sys-color-outline-variant); border-radius: 12px; margin: 14px 0; overflow: hidden; }
  h3 { font-size: 14px; margin: 14px; }
  .product-hero { margin: 0 0 12px; padding: 14px 16px 12px; background: var(--md-sys-color-surface-container-low); }
  .hero-period, .hero-stores { margin: 0; color: var(--md-sys-color-on-surface-variant); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
  .hero-metrics { display: grid; grid-template-columns: minmax(0, 1.3fr) repeat(2, minmax(0, 1fr)); gap: 10px 16px; margin: 10px 0; }
  .hero-metrics > div { min-width: 0; }
  .hero-metrics dt { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-weight: 650; }
  .hero-metrics dd { margin: 4px 0 0; color: var(--app-summary-value, var(--md-sys-color-on-surface)); font-size: 22px; font-weight: 750; font-variant-numeric: tabular-nums; letter-spacing: -.03em; line-height: 1.2; overflow-wrap: anywhere; }
  .hero-primary dd { color: var(--md-sys-color-primary); }
  .hero-metrics .positive { color: var(--md-sys-color-primary); }
  .hero-metrics .negative { color: var(--md-sys-color-error); }
  .store-heading { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding-right: 14px; }
  .store-heading h3 { margin-right: auto; }
  select { max-width: 100%; min-width: 0; }
  button, select { min-height: 38px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 10px; background: var(--md-sys-color-surface); color: var(--md-sys-color-primary); padding: 8px 12px; font: inherit; font-size: 12px; }
  button { cursor: pointer; }
  button:disabled { opacity: .5; }
  button:focus-visible, select:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .detail-hint, .detail-error { font-size: 12px; line-height: 1.6; color: var(--md-sys-color-on-surface-variant); }
  .detail-error { padding: 12px; border: 1px solid var(--md-sys-color-error); border-radius: 12px; }
  .dialog-actions { border-top: 1px solid var(--md-sys-color-outline-variant); }
  @media (max-width: 720px) {
    .hero-metrics { grid-template-columns: 1fr 1fr; }
    .hero-primary { grid-column: 1 / -1; }
    .hero-metrics dd { font-size: 20px; }
  }
  @media (max-width: 520px) {
    .dialog-header, .dialog-actions { padding: 14px; }
    .product-body { padding: 0 14px 14px; }
    .product-hero { padding: 12px; }
    .hero-metrics { gap: 8px 12px; margin: 8px 0; }
    .store-heading { padding: 0 12px 12px; }
    .store-heading h3 { margin-left: 0; }
    select { width: 100%; }
  }
</style>
