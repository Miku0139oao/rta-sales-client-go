import type { SalesAnalysisItem, SalesAnalysisPeriodResult } from './types';
import type { AnalysisTable } from './analysisTable';
import type { Translator } from './i18n';
import { periodNeedsItemHydration } from './salesAnalysisItems';
import { buildSalesContributions, salesContributionTables, type SalesContributions } from './salesContributions';

export type InsightKind = 'decline' | 'growth' | 'returns' | 'leader';
export interface SalesInsight {
  kind: InsightKind; code: string; name: string;
  current: number; previous: number | null; difference: number | null; percent: number | null;
  quantity: number; refunds: number;
}
export type InsightReason = 'ready' | 'currentMissing' | 'currentPartial' | 'previousMissing' | 'previousPartial' | 'storesDiffer' | 'invalidData';
export interface SalesInsights {
  entries: SalesInsight[]; reason: InsightReason; current?: SalesAnalysisPeriodResult; previous?: SalesAnalysisPeriodResult;
  contributions?: SalesContributions;
}
interface Product { code: string; name: string; amount: number; quantity: number; refunds: number }
function aggregate(items: SalesAnalysisItem[], include: (item: SalesAnalysisItem) => boolean): Map<string, Product> | undefined {
  const products = new Map<string, Product>();
  for (const item of items) {
    if (!include(item)) continue;
    if (![item.netSalesAmount, item.netQuantity, item.returnAmount].every(Number.isFinite)) return undefined;
    const code = item.articleCode?.trim();
    if (!code) continue;
    const product = products.get(code) ?? { code, name: item.articleName || code, amount: 0, quantity: 0, refunds: 0 };
    product.amount += item.netSalesAmount; product.quantity += item.netQuantity; product.refunds += item.returnAmount;
    if (![product.amount, product.quantity, product.refunds].every(Number.isFinite)) return undefined;
    products.set(code, product);
  }
  return products;
}
function storeIds(period: SalesAnalysisPeriodResult): Set<string> { return new Set((period.stores ?? []).map(store => store.businessId)); }
function coverageReady(period: SalesAnalysisPeriodResult): boolean {
  const ids = storeIds(period);
  return period.complete && !(period.issues?.length) && ids.size > 0 && ids.size === period.successfulStores
    && (period.items ?? []).every(item => ids.has(item.storeId));
}
export function buildSalesInsights(current: SalesAnalysisPeriodResult | undefined, previous: SalesAnalysisPeriodResult | undefined, include: (item: SalesAnalysisItem) => boolean = () => true): SalesInsights {
  const output: SalesInsights = { entries: [], reason: 'ready', current, previous };
  const finish = (result: SalesInsights): SalesInsights => ({ ...result, contributions: buildSalesContributions(result.current, result.previous, include, result.reason) });
  if (!current || periodNeedsItemHydration(current)) return finish({ ...output, reason: 'currentMissing' });
  if (!coverageReady(current)) return finish({ ...output, reason: 'currentPartial' });
  const now = aggregate(current.items ?? [], include);
  if (!now) return finish({ ...output, reason: 'invalidData' });
  let before: Map<string, Product> | undefined;
  if (!previous || periodNeedsItemHydration(previous)) output.reason = 'previousMissing';
  else if (!coverageReady(previous)) output.reason = 'previousPartial';
  else {
    const a = storeIds(current), b = storeIds(previous);
    if (a.size !== b.size || [...a].some(id => !b.has(id))) output.reason = 'storesDiffer';
    else { before = aggregate(previous.items ?? [], include); if (!before) output.reason = 'invalidData'; }
  }
  let decline: SalesInsight | undefined, growth: SalesInsight | undefined, returns: SalesInsight | undefined, leader: SalesInsight | undefined;
  const codes = new Set(now.keys());
  if (before) for (const code of before.keys()) codes.add(code);
  for (const code of codes) {
    const a = now.get(code), b = before?.get(code);
    const difference = before ? (a?.amount ?? 0) - (b?.amount ?? 0) : null;
    if (difference !== null && !Number.isFinite(difference)) return finish({ ...output, entries: [], reason: 'invalidData' });
    const rate = difference !== null && b?.amount ? difference / Math.abs(b.amount) : null;
    if (rate !== null && !Number.isFinite(rate)) return finish({ ...output, entries: [], reason: 'invalidData' });
    const value: SalesInsight = { kind: 'leader', code, name: a?.name ?? b?.name ?? code, current: a?.amount ?? 0, previous: before ? b?.amount ?? 0 : null,
      difference, percent: rate !== null && Number.isFinite(rate) ? rate : null, quantity: a?.quantity ?? 0, refunds: Math.abs(a?.refunds ?? 0) };
    if (difference !== null && Number.isFinite(difference) && difference < -0.005 && (!decline || difference < decline.difference! || (difference === decline.difference && code < decline.code))) decline = { ...value, kind: 'decline' };
    if (difference !== null && Number.isFinite(difference) && difference > 0.005 && (!growth || difference > growth.difference! || (difference === growth.difference && code < growth.code))) growth = { ...value, kind: 'growth' };
    if (value.refunds > 0 && (!returns || value.refunds > returns.refunds || (value.refunds === returns.refunds && code < returns.code))) returns = { ...value, kind: 'returns' };
    if (value.current > 0 && (!leader || value.current > leader.current || (value.current === leader.current && code < leader.code))) leader = value;
  }
  output.entries = [decline, growth, returns].filter((entry): entry is SalesInsight => Boolean(entry));
  if (output.entries.length < 3 && leader && !output.entries.some(entry => entry.code === leader.code)) output.entries.push(leader);
  return finish(output);
}
export function salesInsightsTable(data: SalesInsights, t: Translator): AnalysisTable {
  return { id: 'insights', name: t('insights.title'), columns: [
    { label: t('insights.type'), format: 'text' }, { label: t('data.code'), format: 'text' }, { label: t('analysis.article'), format: 'text' },
    { label: t('analysis.currentPeriod'), format: 'money' }, { label: t('analysis.previousPeriod'), format: 'money' },
    { label: t('analysis.variance'), format: 'money' }, { label: t('analysis.vsPrevious'), format: 'percent' },
    { label: t('analysis.returns'), format: 'money' }, { label: t('analysis.netQuantity'), format: 'number' },
  ], rows: data.entries.map(entry => ({ cells: [t(`insights.${entry.kind}`), entry.code, entry.name, entry.current, entry.previous, entry.difference, entry.percent, entry.refunds, entry.quantity] })) };
}
export function salesInsightSheets(data: SalesInsights, t: Translator): AnalysisTable[] {
  const sheets: AnalysisTable[] = [];
  if (data.entries.length) sheets.push(salesInsightsTable(data, t));
  if (data.contributions) sheets.push(...salesContributionTables(data.contributions, t));
  return sheets;
}
