import type { SalesAnalysisItem, SalesAnalysisPeriodResult } from './types';
import type { AnalysisTable, TableRow } from './analysisTable';
import type { Translator } from './i18n';
import type { InsightReason } from './salesInsights';

export const CONTRIBUTION_PREVIEW_LIMIT = 5;
const MONEY_EPS = 0.005;
/** Absolute HKD tolerance when reconciling dimension totals to shared filtered totals. At most half a cent; never a relative mask that could hide dollar-scale error. */
const MONEY_RECONCILE_TOLERANCE = 0.005;

export interface ContributionGroup {
  key: string;
  code: string;
  label: string;
  current: number;
  previous: number;
  delta: number;
  previousLabel?: string;
  renamed?: boolean;
}

export interface ContributionRemainder {
  count: number;
  current: number;
  previous: number;
  delta: number;
}

export interface ContributionRename {
  key: string;
  code: string;
  previousName: string;
  currentName: string;
}

export interface ContributionDimensionView {
  dimension: 'store' | 'category';
  groups: ContributionGroup[];
  gains: ContributionGroup[];
  losses: ContributionGroup[];
  remainder: ContributionRemainder | null;
  totalCurrent: number;
  totalPrevious: number;
  totalDelta: number;
  categoryLevel?: 'category1';
  transfers: ContributionRename[];
}

export interface SalesContributions {
  ready: boolean;
  reason: InsightReason;
  store?: ContributionDimensionView;
  category?: ContributionDimensionView;
}

interface Accumulator {
  key: string;
  code: string;
  currentLabel: string;
  previousLabel: string;
  current: number;
  previous: number;
}

function suppressed(reason: InsightReason): SalesContributions {
  return { ready: false, reason };
}

function finiteAmounts(...values: number[]): boolean {
  return values.every(Number.isFinite);
}

function amountsAgree(left: number, right: number): boolean {
  return finiteAmounts(left, right) && Math.abs(left - right) <= MONEY_RECONCILE_TOLERANCE;
}

function sumField<T>(rows: T[], read: (row: T) => number): number {
  return rows.reduce((total, row) => total + read(row), 0);
}

function addAmount(row: Accumulator, side: 'current' | 'previous', amount: number, label: string): boolean {
  row[side] += amount;
  if (label) {
    if (side === 'current') row.currentLabel = label;
    else row.previousLabel = label;
  }
  return Number.isFinite(row.current) && Number.isFinite(row.previous);
}

function categoryIdentity(item: SalesAnalysisItem): { key: string; code: string; name: string } {
  const code = item.category1Code?.trim() ?? '';
  const name = item.category1?.trim() ?? '';
  if (code) return { key: `code:${code}`, code, name: name || code };
  if (name) return { key: `name:${name}`, code: '', name };
  return { key: 'uncategorized', code: '', name: '' };
}

function remainderOf(groups: ContributionGroup[], shown: Set<string>): ContributionRemainder | null {
  const omitted = groups.filter(group => !shown.has(group.key));
  if (!omitted.length) return null;
  return {
    count: omitted.length,
    current: sumField(omitted, group => group.current),
    previous: sumField(omitted, group => group.previous),
    delta: sumField(omitted, group => group.delta),
  };
}

function preview(groups: ContributionGroup[], dimension: 'store' | 'category', extra: Pick<ContributionDimensionView, 'categoryLevel' | 'transfers'>): ContributionDimensionView {
  const ranked = (direction: 1 | -1) => [...groups]
    .filter(group => direction > 0 ? group.delta > MONEY_EPS : group.delta < -MONEY_EPS)
    .sort((a, b) => direction * (b.delta - a.delta) || a.key.localeCompare(b.key));
  const gains = ranked(1).slice(0, CONTRIBUTION_PREVIEW_LIMIT);
  const losses = ranked(-1).slice(0, CONTRIBUTION_PREVIEW_LIMIT);
  const shown = new Set([...gains, ...losses].map(group => group.key));
  const totalCurrent = sumField(groups, group => group.current);
  const totalPrevious = sumField(groups, group => group.previous);
  return {
    dimension, groups, gains, losses, remainder: remainderOf(groups, shown),
    totalCurrent, totalPrevious, totalDelta: totalCurrent - totalPrevious,
    transfers: extra.transfers, categoryLevel: extra.categoryLevel,
  };
}

function toGroup(row: Accumulator): ContributionGroup {
  const renamed = Boolean(row.currentLabel && row.previousLabel && row.currentLabel !== row.previousLabel);
  return {
    key: row.key, code: row.code, label: row.currentLabel || row.previousLabel || row.code,
    current: row.current, previous: row.previous, delta: row.current - row.previous,
    previousLabel: renamed ? row.previousLabel : undefined, renamed,
  };
}

function aggregateStores(current: SalesAnalysisPeriodResult, previous: SalesAnalysisPeriodResult, include: (item: SalesAnalysisItem) => boolean): ContributionDimensionView | undefined {
  const rows = new Map<string, Accumulator>();
  const remember = (period: SalesAnalysisPeriodResult) => {
    for (const store of period.stores ?? []) {
      const key = store.businessId;
      const existing = rows.get(key) ?? { key, code: key, currentLabel: '', previousLabel: '', current: 0, previous: 0 };
      if (store.label) {
        if (period === current) existing.currentLabel = store.label;
        else existing.previousLabel = store.label;
      }
      rows.set(key, existing);
    }
  };
  remember(current); remember(previous);
  const consume = (period: SalesAnalysisPeriodResult, side: 'current' | 'previous'): boolean => {
    for (const item of period.items ?? []) {
      if (!include(item)) continue;
      if (!Number.isFinite(item.netSalesAmount)) return false;
      const key = item.storeId;
      const row = rows.get(key) ?? { key, code: key, currentLabel: '', previousLabel: '', current: 0, previous: 0 };
      if (!addAmount(row, side, item.netSalesAmount, item.storeLabel || '')) return false;
      rows.set(key, row);
    }
    return true;
  };
  if (!consume(current, 'current') || !consume(previous, 'previous')) return undefined;
  const groups = [...rows.values()].map(toGroup).sort((a, b) => b.delta - a.delta || a.key.localeCompare(b.key));
  return preview(groups, 'store', { transfers: [] });
}

function aggregateCategories(current: SalesAnalysisPeriodResult, previous: SalesAnalysisPeriodResult, include: (item: SalesAnalysisItem) => boolean): ContributionDimensionView | undefined {
  const rows = new Map<string, Accumulator>();
  const consume = (period: SalesAnalysisPeriodResult, side: 'current' | 'previous'): boolean => {
    for (const item of period.items ?? []) {
      if (!include(item)) continue;
      if (!Number.isFinite(item.netSalesAmount)) return false;
      const identity = categoryIdentity(item);
      const row = rows.get(identity.key) ?? { key: identity.key, code: identity.code, currentLabel: '', previousLabel: '', current: 0, previous: 0 };
      if (!addAmount(row, side, item.netSalesAmount, identity.name)) return false;
      rows.set(identity.key, row);
    }
    return true;
  };
  if (!consume(current, 'current') || !consume(previous, 'previous')) return undefined;
  const groups = [...rows.values()].map(toGroup).sort((a, b) => b.delta - a.delta || a.key.localeCompare(b.key));
  const transfers = groups.filter(group => group.renamed && group.previousLabel).map(group => ({
    key: group.key, code: group.code, previousName: group.previousLabel!, currentName: group.label,
  }));
  return preview(groups, 'category', { categoryLevel: 'category1', transfers });
}

function filteredTotals(
  current: SalesAnalysisPeriodResult,
  previous: SalesAnalysisPeriodResult,
  include: (item: SalesAnalysisItem) => boolean,
): { current: number; previous: number; delta: number } | undefined {
  const totalOf = (period: SalesAnalysisPeriodResult): number | undefined => {
    let total = 0;
    for (const item of period.items ?? []) {
      if (!include(item)) continue;
      if (!Number.isFinite(item.netSalesAmount)) return undefined;
      total += item.netSalesAmount;
      if (!Number.isFinite(total)) return undefined;
    }
    return total;
  };
  const currentTotal = totalOf(current);
  const previousTotal = totalOf(previous);
  if (currentTotal === undefined || previousTotal === undefined) return undefined;
  const delta = currentTotal - previousTotal;
  if (!finiteAmounts(currentTotal, previousTotal, delta)) return undefined;
  return { current: currentTotal, previous: previousTotal, delta };
}

function derivedFieldsFinite(view: ContributionDimensionView): boolean {
  if (!finiteAmounts(view.totalCurrent, view.totalPrevious, view.totalDelta)) return false;
  for (const group of view.groups) {
    if (!finiteAmounts(group.current, group.previous, group.delta)) return false;
  }
  if (!view.remainder) return true;
  return finiteAmounts(view.remainder.count, view.remainder.current, view.remainder.previous, view.remainder.delta);
}

function dimensionReconciles(view: ContributionDimensionView, shared: { current: number; previous: number; delta: number }): boolean {
  if (!derivedFieldsFinite(view)) return false;
  const summedCurrent = sumField(view.groups, group => group.current);
  const summedPrevious = sumField(view.groups, group => group.previous);
  const summedDelta = sumField(view.groups, group => group.delta);
  const previewDelta = sumField([...view.gains, ...view.losses], group => group.delta) + (view.remainder?.delta ?? 0);
  if (!finiteAmounts(summedCurrent, summedPrevious, summedDelta, previewDelta)) return false;
  if (view.remainder && !amountsAgree(view.remainder.delta, view.remainder.current - view.remainder.previous)) return false;
  return amountsAgree(view.totalCurrent, shared.current)
    && amountsAgree(view.totalPrevious, shared.previous)
    && amountsAgree(view.totalDelta, shared.delta)
    && amountsAgree(summedCurrent, shared.current)
    && amountsAgree(summedPrevious, shared.previous)
    && amountsAgree(summedDelta, shared.delta)
    && amountsAgree(view.totalDelta, view.totalCurrent - view.totalPrevious)
    && amountsAgree(summedDelta, view.totalDelta)
    && amountsAgree(previewDelta, shared.delta)
    && amountsAgree(previewDelta, view.totalDelta);
}

export function buildSalesContributions(
  current: SalesAnalysisPeriodResult | undefined,
  previous: SalesAnalysisPeriodResult | undefined,
  include: (item: SalesAnalysisItem) => boolean = () => true,
  reason: InsightReason,
): SalesContributions {
  if (reason !== 'ready') return suppressed(reason);
  if (!current || !previous) return suppressed('invalidData');
  const shared = filteredTotals(current, previous, include);
  if (!shared) return suppressed('invalidData');
  const store = aggregateStores(current, previous, include);
  const category = aggregateCategories(current, previous, include);
  if (!store || !category) return suppressed('invalidData');
  if (!dimensionReconciles(store, shared) || !dimensionReconciles(category, shared)) return suppressed('invalidData');
  if (!amountsAgree(store.totalCurrent, category.totalCurrent)
    || !amountsAgree(store.totalPrevious, category.totalPrevious)
    || !amountsAgree(store.totalDelta, category.totalDelta)
    || !amountsAgree(store.totalDelta, shared.delta)) {
    return suppressed('invalidData');
  }
  return { ready: true, reason: 'ready', store, category };
}

function labelOf(group: ContributionGroup, t: Translator): string {
  return group.label || t('analysis.uncategorized');
}

function dimensionTable(id: string, name: string, view: ContributionDimensionView, t: Translator, kind: 'store' | 'category'): AnalysisTable {
  const columns = kind === 'store'
    ? [
      { label: t('contributions.storeId'), format: 'text' as const },
      { label: t('contributions.storeName'), format: 'text' as const },
      { label: t('analysis.currentPeriod'), format: 'money' as const },
      { label: t('analysis.previousPeriod'), format: 'money' as const },
      { label: t('contributions.delta'), format: 'money' as const },
    ]
    : [
      { label: t('contributions.categoryCode'), format: 'text' as const },
      { label: t('contributions.categoryName'), format: 'text' as const },
      { label: t('analysis.currentPeriod'), format: 'money' as const },
      { label: t('analysis.previousPeriod'), format: 'money' as const },
      { label: t('contributions.delta'), format: 'money' as const },
      { label: t('contributions.transfer'), format: 'text' as const },
    ];
  const rows: TableRow[] = view.groups.map(group => ({
    cells: kind === 'store'
      ? [group.code, labelOf(group, t), group.current, group.previous, group.delta]
      : [
        group.code,
        labelOf(group, t),
        group.current,
        group.previous,
        group.delta,
        group.renamed && group.previousLabel ? t('contributions.transferNote', { previous: group.previousLabel, current: group.label }) : '',
      ],
  }));
  rows.push({
    fixed: true,
    cells: kind === 'store'
      ? [t('contributions.total'), '', view.totalCurrent, view.totalPrevious, view.totalDelta]
      : [t('contributions.total'), '', view.totalCurrent, view.totalPrevious, view.totalDelta, ''],
  });
  return { id, name, columns, rows };
}

export function salesContributionTables(data: SalesContributions, t: Translator): AnalysisTable[] {
  if (!data.ready || !data.store || !data.category) return [];
  return [
    dimensionTable('contrib-stores', t('contributions.exportStores'), data.store, t, 'store'),
    dimensionTable('contrib-categories', t('contributions.exportCategories'), data.category, t, 'category'),
  ];
}
