import type { Translator } from './i18n';
import type { SalesAnalysisItem, SalesAnalysisPeriodResult } from './types';
import { periodNeedsItemHydration } from './salesAnalysisItems';
import { relativeChange } from './analysisTableViews';
import type { AnalysisTable, TableRow } from './analysisTable';

function sum(rows: SalesAnalysisItem[]) {
  return rows.reduce((total, item) => ({
    amount: total.amount + item.netSalesAmount,
    quantity: total.quantity + item.netQuantity,
    returns: total.returns + item.returnAmount,
    gross: total.gross + item.saleAmount,
  }), { amount: 0, quantity: 0, returns: 0, gross: 0 });
}

type PeriodBundle = {
  period: SalesAnalysisPeriodResult;
  missing: boolean;
  byStore: Map<string, ReturnType<typeof sum>>;
  knownStores: Set<string>;
  totals: ReturnType<typeof sum> | undefined;
};

function periodBundles(code: string, periods: SalesAnalysisPeriodResult[]): PeriodBundle[] {
  return periods.map((period) => {
    const missing = periodNeedsItemHydration(period);
    const knownStores = new Set((period.stores ?? []).map((store) => store.businessId));
    const byStore = new Map<string, ReturnType<typeof sum>>();
    const target = code.trim();
    const rows = (period.items ?? []).filter((item) => {
      knownStores.add(item.storeId);
      if (item.articleCode.trim() !== target) return false;
      const values = byStore.get(item.storeId) ?? sum([]);
      values.amount += item.netSalesAmount; values.quantity += item.netQuantity;
      values.returns += item.returnAmount; values.gross += item.saleAmount;
      byStore.set(item.storeId, values);
      return true;
    });
    for (const issue of period.issues ?? []) knownStores.delete(issue.storeId);
    return { period, missing, byStore, knownStores,
      totals: missing || (!period.complete && !rows.length) ? undefined : sum(rows),
    };
  });
}

function coverage(period: SalesAnalysisPeriodResult, missing: boolean, t: Translator): string {
  return t(missing ? 'data.detailLoading' : period.complete ? 'data.detailComplete' : 'data.partial');
}

function storeLabels(periods: SalesAnalysisPeriodResult[]): Map<string, string> {
  const labels = new Map<string, string>();
  for (const period of periods) {
    for (const store of period.stores ?? []) labels.set(store.businessId, store.label);
    for (const issue of period.issues ?? []) if (issue.storeId) labels.set(issue.storeId, issue.storeLabel || issue.storeId);
    for (const item of period.items ?? []) if (item.storeId && !labels.has(item.storeId)) labels.set(item.storeId, item.storeLabel || item.storeId);
  }
  return labels;
}

function storeKnown(selected: PeriodBundle | undefined, storeId: string): boolean {
  return Boolean(selected && !selected.missing && selected.knownStores.has(storeId));
}

function storeRowsFor(code: string, periods: SalesAnalysisPeriodResult[], selected: PeriodBundle | undefined, t: Translator): TableRow[] {
  return [...storeLabels(periods)].map(([id, label]) => {
    const known = storeKnown(selected, id);
    const hasProduct = selected?.byStore.has(id);
    const totals = known ? selected?.byStore.get(id) ?? sum([]) : undefined;
    return {
      cells: [
        id,
        label,
        totals?.amount ?? null,
        totals?.quantity ?? null,
        totals?.gross ?? null,
        totals?.returns ?? null,
        t(known ? (hasProduct ? 'data.detailComplete' : 'data.noProductSales') : 'data.detailUnknown'),
      ],
    };
  });
}

export const PRODUCT_PERIOD_VIEW_ORDER = [0, 3, 4, 6, 1, 2, 5, 7];
export const PRODUCT_STORE_VIEW_ORDER = [0, 2, 3, 1, 4, 5, 6];

export function reorderAnalysisTable(table: AnalysisTable, order: number[]): AnalysisTable {
  return {
    ...table,
    columns: order.map((index) => table.columns[index]!),
    rows: table.rows.map((row) => ({
      ...row,
      cells: order.map((index) => row.cells[index] ?? null),
      secondary: row.secondary
        ? Object.fromEntries(order.flatMap((source, view) => {
          const value = row.secondary?.[source];
          return value ? [[view, value]] : [];
        }))
        : undefined,
      product: row.product && order.includes(row.product.column)
        ? { ...row.product, column: order.indexOf(row.product.column) }
        : row.product,
    })),
  };
}

export function exportSortFromView(id: string, sort: { column: number; direction: 'ascending' | 'descending' } | undefined) {
  if (!sort) return sort;
  const order = id === 'product-periods' ? PRODUCT_PERIOD_VIEW_ORDER : id === 'product-stores' ? PRODUCT_STORE_VIEW_ORDER : undefined;
  if (!order) return sort;
  return { column: order[sort.column] ?? sort.column, direction: sort.direction };
}

export function productDetailTables(code: string, periods: SalesAnalysisPeriodResult[], selectedKey: string, t: Translator): AnalysisTable[] {
  const totals = periodBundles(code, periods);
  const previous = totals.find((value) => value.period.key === 'previous');
  const summary: AnalysisTable = {
    id: 'product-periods',
    name: t('data.productPeriods'),
    columns: [
      { label: t('analysis.periods'), format: 'text' },
      { label: t('excel.from'), format: 'text' },
      { label: t('excel.to'), format: 'text' },
      { label: t('analysis.netSales'), format: 'money' },
      { label: t('analysis.netQuantity'), format: 'number' },
      { label: t('analysis.returns'), format: 'money' },
      { label: t('analysis.vsPrevious'), format: 'percent' },
      { label: t('data.coverage'), format: 'text' },
    ],
    rows: totals.map(({ period, missing, totals: values }) => ({
      cells: [
        period.label,
        period.from,
        period.to,
        values?.amount ?? null,
        values?.quantity ?? null,
        values?.returns ?? null,
        period.key === 'current' && period.complete && previous?.period.complete
          ? relativeChange(values?.amount, previous.totals?.amount) ?? null
          : null,
        coverage(period, missing, t),
      ],
    })),
  };
  const selected = totals.find((value) => value.period.key === selectedKey) ?? totals[0];
  return [
    summary,
    {
      id: 'product-stores',
      name: `${t('data.productStores')} · ${selected?.period.label ?? ''}`,
      columns: [
        { label: t('analysis.store'), format: 'text' },
        { label: t('data.storeName'), format: 'text' },
        { label: t('analysis.netSales'), format: 'money' },
        { label: t('analysis.netQuantity'), format: 'number' },
        { label: t('analysis.grossSales'), format: 'money' },
        { label: t('analysis.returns'), format: 'money' },
        { label: t('data.coverage'), format: 'text' },
      ],
      rows: storeRowsFor(code, periods, selected, t),
    },
  ];
}

export function productDetailViewTables(tables: AnalysisTable[]): AnalysisTable[] {
  return tables.map((table) => reorderAnalysisTable(
    table,
    table.id === 'product-stores' ? PRODUCT_STORE_VIEW_ORDER : PRODUCT_PERIOD_VIEW_ORDER,
  ));
}

export type ProductDetailHero = {
  amount: number | null;
  quantity: number | null;
  vsPrevious: number | null;
  currentLabel: string;
  currentFrom: string;
  currentTo: string;
  currentStatus: string;
  selectedLabel: string;
  selectedFrom: string;
  selectedTo: string;
  storesWithSales: number | null;
  storesTotal: number;
};

export function productDetailHero(code: string, periods: SalesAnalysisPeriodResult[], selectedKey: string, t: Translator): ProductDetailHero {
  const totals = periodBundles(code, periods);
  const current = totals.find((value) => value.period.key === 'current') ?? totals[0];
  const previous = totals.find((value) => value.period.key === 'previous');
  const selected = totals.find((value) => value.period.key === selectedKey) ?? totals[0];
  const vsPrevious = current?.period.key === 'current' && current.period.complete && previous?.period.complete
    ? relativeChange(current.totals?.amount, previous.totals?.amount) ?? null
    : null;
  const labels = storeLabels(periods);
  let storesWithSales = 0;
  let storesComplete = Boolean(selected?.period.complete && !selected.missing && labels.size);
  for (const [id] of labels) {
    if (!storeKnown(selected, id)) { storesComplete = false; continue; }
    const values = selected?.byStore.get(id);
    if (values && (values.amount || values.quantity)) storesWithSales += 1;
  }
  return {
    amount: current?.totals?.amount ?? null,
    quantity: current?.totals?.quantity ?? null,
    vsPrevious,
    currentLabel: current?.period.label ?? '',
    currentFrom: current?.period.from ?? '',
    currentTo: current?.period.to ?? '',
    currentStatus: current ? coverage(current.period, current.missing, t) : t('data.detailUnknown'),
    selectedLabel: selected?.period.label ?? '',
    selectedFrom: selected?.period.from ?? '',
    selectedTo: selected?.period.to ?? '',
    storesWithSales: storesComplete ? storesWithSales : null,
    storesTotal: labels.size,
  };
}
