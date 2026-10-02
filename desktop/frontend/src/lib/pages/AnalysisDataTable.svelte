<script lang="ts">
  import { onMount, tick } from 'svelte';
  import type { Translator } from '../i18n';
  import { formatTableCell, type AnalysisTable, type TableSort } from '../analysisTable';
  import { readColumns, tablePositions } from '../tableView';
  export let table: AnalysisTable;
  export let t: Translator;
  export let locale: string;
  export let sort: TableSort | undefined = undefined;
  export let onSort: (id: string, sort: TableSort) => void;
  export let onProduct: (code: string, name: string) => void = () => undefined;
  export let paginated = false;
  export let search = '';
  export let onSearch: (id: string, query: string) => void = () => undefined;
  export let viewKey = '';
  let page = 1,
    top = 0,
    visible: number[] = [],
    signature = '',
    positionKey = '',
    preferenceError = '';
  let scroller: HTMLDivElement;
  let cards = false;
  let mobile = false;
  onMount(() => {
    if (typeof matchMedia !== 'function') return;
    const media = matchMedia('(max-width: 760px)');
    const update = () => {
      mobile = media.matches;
      cards = mobile;
      page = 1;
      top = 0;
    };
    update();
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  });
  $: en = locale === 'en';
  $: nextSignature = `${table.id}:${table.columns.length}`;
  $: if (nextSignature !== signature) {
    signature = nextSignature;
    visible = readColumns(signature, table.columns.length);
  }
  $: nextPositionKey = `${viewKey}:${table.id}`;
  $: if (nextPositionKey !== positionKey) {
    positionKey = nextPositionKey;
    const prior = tablePositions.get(positionKey);
    page = prior?.page ?? 1;
    top = prior?.top ?? 0;
    void tick().then(() => {
      if (scroller) {
        scroller.scrollTop = top;
        scroller.scrollLeft = prior?.left ?? 0;
      }
    });
  }
  $: pageCount = Math.max(1, Math.ceil(table.rows.length / 50));
  $: if (page > pageCount) page = pageCount;
  $: usePagination = paginated || cards;
  $: virtual = !usePagination && table.rows.length > 500;
  $: start = virtual ? Math.max(0, Math.min(table.rows.length - 1, Math.floor(Math.max(0, top - 48) / 64) - 6)) : 0;
  $: rows = usePagination
    ? table.rows.slice((page - 1) * 50, page * 50)
    : virtual
      ? table.rows.slice(start, start + 24)
      : table.rows;
  function position() {
    if (scroller) {
      top = scroller.scrollTop;
      if (tablePositions.size > 100) tablePositions.delete(tablePositions.keys().next().value!);
      tablePositions.set(positionKey, { page, top, left: scroller.scrollLeft });
    }
  }
  function changeSort(column: number) {
    onSort(table.id, {
      column,
      direction: sort?.column === column && sort.direction === 'descending' ? 'ascending' : 'descending',
    });
  }
  function changePresentation() {
    cards = !cards;
    top = 0;
    page = 1;
    if (scroller) scroller.scrollTop = 0;
  }
  function cardFields(identity: number) {
    const fields = visible.filter((index) => index !== identity);
    // Product cards lead with net sales, units and the store instead of three
    // identifiers. Other report tables retain their existing column order.
    if (table.id !== 'products') return fields;
    const priority = [8, 7, 0];
    return [
      ...priority.filter((index) => fields.includes(index)),
      ...fields.filter((index) => !priority.includes(index)),
    ];
  }
  function columns(index: number) {
    if (visible.includes(index) && visible.length === 1) return;
    visible = visible.includes(index) ? visible.filter((i) => i !== index) : [...visible, index].sort((a, b) => a - b);
    try {
      localStorage.setItem(`rta-table-columns-${signature}`, JSON.stringify(visible));
      preferenceError = '';
    } catch {
      preferenceError = en ? 'Column preferences could not be saved.' : '欄位設定無法儲存。';
    }
  }
</script>

<div class="table-tools">
  <label
    >{en ? 'Search this table' : '搜尋此表格'}<input
      type="search"
      value={search}
      oninput={(event) => {
        page = 1;
        top = 0;
        if (scroller) scroller.scrollTop = 0;
        onSearch(table.id, event.currentTarget.value);
      }}
    /></label
  ><span>{table.rows.length} {en ? 'rows' : '筆'}</span>
  <details>
    <summary>{en ? 'Columns' : '欄位'}</summary>
    <div class="column-options">
      {#each table.columns as column, index}<label
          ><input
            type="checkbox"
            checked={visible.includes(index)}
            disabled={visible.includes(index) && visible.length === 1}
            onchange={() => columns(index)}
          />{column.label}</label
        >{/each}<small>{en ? 'Exports retain all columns.' : '匯出保留完整欄位。'}</small>
    </div>
  </details>
</div>
{#if mobile}
  <div class="mobile-table-tools">
    <button type="button" aria-pressed={cards} onclick={changePresentation}
      >{cards ? (en ? 'Switch to full table' : '切換完整表格') : en ? 'Switch to cards' : '切換卡片明細'}</button
    >
    {#if cards}<label
        >{en ? 'Sort by' : '排序欄位'}<select
          value={sort?.column ?? ''}
          onchange={(event) => changeSort(Number(event.currentTarget.value))}
        >
          <option value="" disabled>{en ? 'Select column' : '選擇欄位'}</option>
          {#each table.columns as column, index}<option value={index}>{column.label}</option>{/each}
        </select></label
      >
      {#if sort}<button type="button" onclick={() => changeSort(sort!.column)}
          >{sort.direction === 'ascending' ? (en ? 'Ascending ↑' : '升冪 ↑') : en ? 'Descending ↓' : '降冪 ↓'}</button
        >{/if}
    {:else}<p>{en ? 'Swipe horizontally to see all columns.' : '左右滑動表格可查看全部欄位。'}</p>{/if}
  </div>
{/if}
{#if preferenceError}<p role="status">{preferenceError}</p>{/if}
{#if cards}
  <div class="data-cards" role="region" aria-label={table.name}>
    {#each rows as row}
      {@const identity =
        row.product?.column ??
        Math.max(
          0,
          table.columns.findIndex((column) => column.format === 'text'),
        )}
      {@const fields = cardFields(identity)}
      <article class:weekly-total={row.fixed}>
        <h3>
          {#if row.product?.code}<button
              type="button"
              class="product-link"
              aria-label={t('data.productOpen', { name: row.product.name })}
              onclick={() => onProduct(row.product!.code, row.product!.name)}
              >{formatTableCell(row.cells[identity] ?? null, table.columns[identity]!.format, locale)}</button
            >{:else}{formatTableCell(row.cells[identity] ?? null, table.columns[identity]!.format, locale)}{/if}
        </h3>
        {#if row.secondary?.[identity]}<span class="secondary">{row.secondary[identity]}</span>{/if}
        <dl>
          {#each fields.slice(0, 3) as index}<div>
              <dt>{table.columns[index]!.label}</dt>
              <dd>
                {formatTableCell(
                  row.cells[index] ?? null,
                  table.columns[index]!.format,
                  locale,
                )}{#if row.secondary?.[index]}<span class="secondary">{row.secondary[index]}</span>{/if}
              </dd>
            </div>{/each}
        </dl>
        {#if fields.length > 3}<details class="card-details">
            <summary>{en ? 'More fields' : '更多欄位'}</summary>
            <dl>
              {#each fields.slice(3) as index}<div>
                  <dt>{table.columns[index]!.label}</dt>
                  <dd>
                    {formatTableCell(
                      row.cells[index] ?? null,
                      table.columns[index]!.format,
                      locale,
                    )}{#if row.secondary?.[index]}<span class="secondary">{row.secondary[index]}</span>{/if}
                  </dd>
                </div>{/each}
            </dl>
          </details>{/if}
      </article>
    {:else}<p class="empty-table">{t('analysis.noResults')}</p>{/each}
  </div>
{:else}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users must scroll data tables.) -->
  <div
    class="table-scroll"
    class:virtual
    bind:this={scroller}
    role="region"
    aria-label={table.name}
    tabindex="0"
    onscroll={position}
  >
    <table aria-label={table.name} aria-rowcount={table.rows.length + 1}>
      <thead
        ><tr
          >{#each visible as index}{@const column = table.columns[index]!}<th
              class:numeric={column.format !== 'text'}
              aria-sort={sort?.column === index ? sort.direction : 'none'}
              ><button type="button" onclick={() => changeSort(index)}
                >{column.label}<span aria-hidden="true"
                  >{sort?.column === index ? (sort.direction === 'descending' ? '↓' : '↑') : '↑↓'}</span
                ></button
              ></th
            >{/each}</tr
        ></thead
      >
      <tbody
        >{#if virtual && start > 0}<tr aria-hidden="true" class="spacer"
            ><td colspan={visible.length} style:height={`${start * 64}px`}></td></tr
          >{/if}
        {#each rows as row, rowIndex}<tr
            class:virtual-row={virtual}
            class:weekly-total={row.fixed}
            aria-rowindex={virtual ? start + rowIndex + 2 : paginated ? (page - 1) * 50 + rowIndex + 2 : rowIndex + 2}
            >{#each visible as index}{@const cell = row.cells[index] ?? null}<td
                class:numeric={table.columns[index]?.format !== 'text'}
                class:positive={table.columns[index]?.format === 'percent' && typeof cell === 'number' && cell > 0}
                class:negative={table.columns[index]?.format === 'percent' && typeof cell === 'number' && cell < 0}
              >
                {#if row.product?.column === index && row.product.code}<button
                    class="product-link"
                    type="button"
                    aria-label={t('data.productOpen', { name: row.product.name })}
                    onclick={() => onProduct(row.product!.code, row.product!.name)}
                    >{formatTableCell(cell, table.columns[index]!.format, locale)}</button
                  >{:else}<strong>{formatTableCell(cell, table.columns[index]!.format, locale)}</strong>{/if}
                {#if row.secondary?.[index]}<span class="secondary">{row.secondary[index]}</span>{/if}
              </td>{/each}</tr
          >{:else}<tr><td colspan={visible.length} class="empty-table">{t('analysis.noResults')}</td></tr>{/each}
        {#if virtual && start + rows.length < table.rows.length}<tr aria-hidden="true" class="spacer"
            ><td colspan={visible.length} style:height={`${(table.rows.length - start - rows.length) * 64}px`}></td></tr
          >{/if}
      </tbody>
    </table>
  </div>
{/if}
{#if usePagination && pageCount > 1}<div class="pagination">
    <button
      type="button"
      disabled={page === 1}
      onclick={() => {
        page -= 1;
        position();
      }}>{t('analysis.previous')}</button
    ><strong>{page} / {pageCount}</strong><button
      type="button"
      disabled={page === pageCount}
      onclick={() => {
        page += 1;
        position();
      }}>{t('analysis.next')}</button
    >
  </div>{/if}

<style>
  .mobile-table-tools {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    padding-bottom: 12px;
  }
  .mobile-table-tools label {
    display: grid;
    gap: 4px;
    flex: 1;
    min-width: 140px;
    font-size: 12px;
  }
  .mobile-table-tools button,
  .mobile-table-tools select {
    min-height: 44px;
    padding: 8px 12px;
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: 10px;
    background: var(--md-sys-color-surface);
    color: var(--md-sys-color-primary);
    font: inherit;
  }
  .mobile-table-tools select {
    width: 100%;
    font-size: 16px;
  }
  .mobile-table-tools p {
    margin: 0;
    color: var(--md-sys-color-on-surface-variant);
    font-size: 12px;
  }
  .data-cards {
    display: grid;
    gap: 10px;
    padding-block: 4px;
  }
  .data-cards article {
    min-width: 0;
    padding: 14px;
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: 12px;
  }
  .data-cards h3 {
    margin: 0;
    font-size: 16px;
    overflow-wrap: anywhere;
  }
  .data-cards dl {
    margin: 10px 0 0;
    display: grid;
    gap: 10px;
  }
  .data-cards dl > div {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
    gap: 12px;
    align-items: baseline;
  }
  .data-cards dt {
    font-size: 12px;
    color: var(--md-sys-color-on-surface-variant);
    overflow-wrap: anywhere;
  }
  .data-cards dd {
    margin: 0;
    text-align: right;
    font-size: 14px;
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }
  .card-details {
    margin-top: 10px;
    border-top: 1px solid var(--md-sys-color-outline-variant);
  }
  .card-details summary {
    min-height: 44px;
    color: var(--md-sys-color-primary);
  }
  .table-tools {
    display: flex;
    align-items: end;
    flex-wrap: wrap;
    gap: 14px;
    padding: 12px 0;
    font-size: 12px;
  }
  .table-tools > label {
    display: grid;
    gap: 6px;
    flex: 1;
    min-width: 180px;
    max-width: 360px;
  }
  input[type='search'] {
    font: inherit;
    padding: 9px 12px;
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: 12px;
    background: var(--md-sys-color-surface);
    color: var(--md-sys-color-on-surface);
  }
  details {
    position: relative;
  }
  summary {
    cursor: pointer;
    padding: 10px;
  }
  .column-options {
    position: absolute;
    right: 0;
    z-index: 5;
    display: grid;
    gap: 12px;
    padding: 16px;
    min-width: 220px;
    max-height: 320px;
    overflow: auto;
    background: var(--md-sys-color-surface-container-high);
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: 12px;
    box-shadow: 0 8px 24px #0004;
  }
  .column-options label {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  small {
    color: var(--md-sys-color-on-surface-variant);
  }
  .table-scroll {
    overflow: auto;
    max-width: 100%;
    max-height: 60vh;
    overscroll-behavior: contain;
  }
  .virtual {
    max-height: 640px;
    height: min(60vh, 640px);
  }
  table {
    width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    font-size: 12px;
  }
  th,
  td {
    padding: 12px;
    border-bottom: 1px solid var(--md-sys-color-outline-variant);
    text-align: left;
    vertical-align: middle;
  }
  th {
    position: sticky;
    top: 0;
    z-index: 2;
    background: var(--md-sys-color-surface-container);
    white-space: nowrap;
    color: var(--md-sys-color-on-surface-variant);
  }
  th button {
    display: inline-flex;
    gap: 6px;
    align-items: center;
    color: inherit;
    border: 0;
    background: transparent;
    padding: 4px 0;
    font: inherit;
    font-weight: 650;
    cursor: pointer;
  }
  th[aria-sort='ascending'],
  th[aria-sort='descending'] {
    color: var(--md-sys-color-primary);
  }
  .numeric {
    text-align: right;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
  td strong {
    font-weight: 500;
  }
  .secondary {
    display: block;
    margin-top: 3px;
    font-size: 11px;
    color: var(--md-sys-color-on-surface-variant);
  }
  .product-link {
    border: 0;
    padding: 4px 0;
    background: transparent;
    color: var(--md-sys-color-primary);
    text-align: left;
    font: inherit;
    font-weight: 650;
    cursor: pointer;
  }
  .positive {
    color: var(--md-sys-color-primary);
  }
  .negative {
    color: var(--md-sys-color-error);
  }
  .weekly-total {
    background: var(--md-sys-color-surface-container);
  }
  .weekly-total strong {
    font-weight: 700;
  }
  .virtual-row {
    height: 64px;
  }
  .virtual-row td {
    height: 64px;
    box-sizing: border-box;
    white-space: nowrap;
  }
  .spacer td {
    padding: 0;
    border: 0;
  }
  .pagination {
    display: flex;
    justify-content: center;
    align-items: center;
    gap: 14px;
    padding: 14px;
  }
  .pagination button {
    min-height: 36px;
    padding: 8px 12px;
    background: var(--md-sys-color-surface);
    border: 1px solid var(--md-sys-color-outline-variant);
    color: var(--md-sys-color-primary);
    border-radius: 10px;
    font: inherit;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .empty-table {
    text-align: center;
    padding: 24px;
    color: var(--md-sys-color-on-surface-variant);
  }
  button:focus-visible,
  input:focus-visible,
  summary:focus-visible {
    outline: 2px solid var(--md-sys-color-primary);
    outline-offset: 3px;
  }
  .table-scroll:focus-visible {
    outline: 2px solid var(--md-sys-color-primary);
    outline-offset: -2px;
  }
  @media (max-width: 760px) {
    .table-tools {
      gap: 8px;
      align-items: center;
    }
    .table-tools > label {
      flex-basis: 100%;
      min-width: 0;
      max-width: none;
    }
    input[type='search'] {
      min-height: 44px;
      font-size: 16px;
    }
    .table-tools > details {
      margin-left: auto;
    }
    .column-options {
      max-width: calc(100vw - 64px);
      min-width: 0;
      width: 220px;
    }
    .column-options label,
    .pagination button,
    .product-link,
    th button {
      min-height: 44px;
    }
    .table-scroll {
      max-height: 65dvh;
    }
  }
</style>
