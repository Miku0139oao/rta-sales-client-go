<script lang="ts">
  import type { Translator } from '../i18n';
  import { formatTableCell, type AnalysisTable, type TableSort } from '../analysisTable';
  export let table: AnalysisTable;
  export let t: Translator;
  export let locale: string;
  export let sort: TableSort | undefined = undefined;
  export let onSort: (id: string, sort: TableSort) => void;
  export let onProduct: (code: string, name: string) => void = () => undefined;
  export let paginated = false;
  let page = 1;
  $: if (table.rows) page = 1;
  $: pageCount = Math.max(1, Math.ceil(table.rows.length / 50));
  $: rows = paginated ? table.rows.slice((page - 1) * 50, page * 50) : table.rows;
  function changeSort(column: number) {
    onSort(table.id, { column, direction: sort?.column === column && sort.direction === 'descending' ? 'ascending' : 'descending' });
  }
</script>
<!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users must be able to scroll this overflow region) -->
<div class="table-scroll" role="region" aria-label={table.name} tabindex="0">
  <table aria-label={table.name}>
    <thead><tr>{#each table.columns as column, index}<th class:numeric={column.format !== 'text'} aria-sort={sort?.column === index ? sort.direction : 'none'}><button type="button" onclick={() => changeSort(index)}>{column.label}<span aria-hidden="true">{sort?.column === index ? sort.direction === 'descending' ? '↓' : '↑' : '↑↓'}</span></button></th>{/each}</tr></thead>
    <tbody>{#each rows as row}<tr class:weekly-total={row.fixed}>{#each row.cells as cell, index}<td class:numeric={table.columns[index]?.format !== 'text'} class:positive={table.columns[index]?.format === 'percent' && typeof cell === 'number' && cell > 0} class:negative={table.columns[index]?.format === 'percent' && typeof cell === 'number' && cell < 0}>
      {#if row.product?.column === index && row.product.code}<button class="product-link" type="button" aria-label={t('data.productOpen', { name: row.product.name })} onclick={() => onProduct(row.product!.code, row.product!.name)}>{formatTableCell(cell, table.columns[index]!.format, locale)}</button>
      {:else}<strong>{formatTableCell(cell, table.columns[index]!.format, locale)}</strong>{/if}
      {#if row.secondary?.[index]}<span class="secondary">{row.secondary[index]}</span>{/if}
    </td>{/each}</tr>{:else}<tr><td colspan={table.columns.length} class="empty-table">{t('analysis.noResults')}</td></tr>{/each}</tbody>
  </table>
</div>
{#if paginated && pageCount > 1}<div class="pagination"><button type="button" disabled={page === 1} onclick={() => { page -= 1; }}>{t('analysis.previous')}</button><strong>{page} / {pageCount}</strong><button type="button" disabled={page === pageCount} onclick={() => { page += 1; }}>{t('analysis.next')}</button></div>{/if}
<style>
  .table-scroll { overflow: auto; max-width: 100%; overscroll-behavior: contain; }
  table { width: 100%; min-width: 100%; border-collapse: collapse; font-size: 12px; }
  th, td { padding: 12px; border-bottom: 1px solid var(--md-sys-color-outline-variant); text-align: left; vertical-align: middle; }
  th { white-space: nowrap; color: var(--md-sys-color-on-surface-variant); }
  th button { display: inline-flex; gap: 6px; align-items: center; color: inherit; border: 0; background: transparent; padding: 4px 0; font: inherit; font-weight: 650; cursor: pointer; }
  th[aria-sort="ascending"], th[aria-sort="descending"] { color: var(--md-sys-color-primary); }
  th[aria-sort="none"] button span { opacity: .45; }
  th[aria-sort="ascending"] button span, th[aria-sort="descending"] button span { opacity: 1; }
  .numeric { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
  td strong { font-weight: 500; }
  .secondary { display: block; margin-top: 3px; font-size: 11px; color: var(--md-sys-color-on-surface-variant); }
  .product-link { border: 0; padding: 4px 0; background: transparent; color: var(--md-sys-color-primary); text-align: left; font: inherit; font-weight: 650; line-height: 1.45; cursor: pointer; overflow-wrap: anywhere; }
  button:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .table-scroll:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: -2px; }
  .positive { color: var(--md-sys-color-primary); }
  .negative { color: var(--md-sys-color-error); }
  .product-link:hover { text-decoration: underline; }
  .weekly-total { background: var(--md-sys-color-surface-container); }
  .weekly-total strong { font-weight: 700; }
  .pagination { display: flex; justify-content: center; align-items: center; gap: 14px; padding: 14px; }
  .pagination button { min-height: 36px; padding: 8px 12px; background: var(--md-sys-color-surface); border: 1px solid var(--md-sys-color-outline-variant); color: var(--md-sys-color-primary); border-radius: 10px; font: inherit; font-size: 12px; font-weight: 650; cursor: pointer; }
  button:disabled { opacity: .45; cursor: default; }
  .empty-table { text-align: center; padding: 24px; color: var(--md-sys-color-on-surface-variant); }
</style>
