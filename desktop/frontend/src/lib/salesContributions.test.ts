import { describe, expect, it } from 'vitest';
import { dataFixture } from '../test/analysisDataFixture';
import { workbookSnapshot } from './analysisTable';
import { buildAnalysisTables } from './analysisTableViews';
import { translator } from './i18n';
import { buildSalesInsights, salesInsightSheets } from './salesInsights';
import { buildSalesContributions, CONTRIBUTION_PREVIEW_LIMIT, salesContributionTables } from './salesContributions';
import type { SalesAnalysisItem, SalesAnalysisPeriodResult } from './types';

const t = translator('zh-TW');

function item(overrides: Partial<SalesAnalysisItem> & Pick<SalesAnalysisItem, 'storeId' | 'articleCode' | 'netSalesAmount'>): SalesAnalysisItem {
  return {
    storeLabel: `${overrides.storeId} Store`, articleName: overrides.articleCode, brandName: 'Brand',
    category1: 'Health', category2: 'Beauty', category2Code: 'A02', category3: 'Skin', category4: 'Face', category5: '',
    transactionCount: 1, saleQuantity: 1, saleAmount: overrides.netSalesAmount, returnQuantity: 0, returnTransactionCount: 0,
    returnAmount: 0, netQuantity: 1, ...overrides,
  };
}

function period(key: 'current' | 'previous', items: SalesAnalysisItem[], storeIds?: string[]): SalesAnalysisPeriodResult {
  const ids = storeIds ?? [...new Set(items.map(row => row.storeId))];
  return {
    key, label: key === 'current' ? '本期' : '上期', from: key === 'current' ? '2026-08-01' : '2026-07-01',
    to: key === 'current' ? '2026-08-31' : '2026-07-31', complete: true, successfulStores: ids.length,
    totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
    stores: ids.map(id => ({ businessId: id, label: `${id} Store`, totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 } })),
    items, itemCount: items.length,
  };
}

function overall(current: SalesAnalysisPeriodResult, previous: SalesAnalysisPeriodResult, include: (item: SalesAnalysisItem) => boolean = () => true): number {
  const sum = (rows: SalesAnalysisItem[]) => rows.reduce((total, row) => include(row) ? total + row.netSalesAmount : total, 0);
  return sum(current.items ?? []) - sum(previous.items ?? []);
}

function reconciled(gains: Array<{ delta: number }>, losses: Array<{ delta: number }>, remainder: { delta: number } | null): number {
  return gains.reduce((sum, group) => sum + group.delta, 0) + losses.reduce((sum, group) => sum + group.delta, 0) + (remainder?.delta ?? 0);
}

describe('store and category contribution decomposition', () => {
  it('reconciles store and category deltas separately to the filtered overall change', () => {
    const [current, previous] = dataFixture().periods!;
    const insights = buildSalesInsights(current, previous);
    const contrib = insights.contributions!;
    expect(contrib.ready).toBe(true);
    expect(contrib.store!.totalDelta).toBe(70);
    expect(contrib.category!.totalDelta).toBe(70);
    expect(contrib.store!.totalDelta).toBe(overall(current!, previous!));
    expect(contrib.store!.totalDelta).toBe(contrib.category!.totalDelta);
    expect(contrib.store!.groups.find(group => group.key === '107')).toMatchObject({ current: 120, previous: 60, delta: 60 });
    expect(contrib.store!.groups.find(group => group.key === '109')).toMatchObject({ current: 0, previous: 0, delta: 0 });
    expect(contrib.category!.groups).toHaveLength(1);
    expect(contrib.category!.categoryLevel).toBe('category1');
  });

  it('keeps mixed gains and losses as amounts when they cancel to a zero total', () => {
    const stores = ['107', '108', '109'];
    const current = period('current', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 100 }), item({ storeId: '108', articleCode: 'B', netSalesAmount: 0 })], stores);
    const previous = period('previous', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 0 }), item({ storeId: '108', articleCode: 'B', netSalesAmount: 100 })], stores);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.ready).toBe(true);
    expect(contrib.store!.totalDelta).toBe(0);
    expect(contrib.store!.gains[0]).toMatchObject({ key: '107', delta: 100 });
    expect(contrib.store!.losses[0]).toMatchObject({ key: '108', delta: -100 });
    expect(contrib.store!.remainder).toMatchObject({ count: 1, delta: 0 });
    expect(reconciled(contrib.store!.gains, contrib.store!.losses, contrib.store!.remainder)).toBe(0);
    expect(contrib.store!.groups.every(group => !('percent' in group))).toBe(true);
  });

  it('treats empty filtered results as a zero overall change without inventing groups from excluded rows', () => {
    const [current, previous] = dataFixture().periods!;
    const include = () => false;
    const contrib = buildSalesInsights(current, previous, include).contributions!;
    expect(contrib.ready).toBe(true);
    expect(contrib.store!.totalDelta).toBe(0);
    expect(contrib.category!.groups).toHaveLength(0);
    expect(contrib.store!.gains).toHaveLength(0);
    expect(contrib.store!.losses).toHaveLength(0);
    expect(contrib.category!.totalDelta).toBe(overall(current!, previous!, include));
  });

  it('identifies same-name categories by code and discloses a renamed code as a transfer', () => {
    const stores = ['107'];
    const current = period('current', [
      item({ storeId: '107', articleCode: 'A', netSalesAmount: 40, category1: 'Health', category1Code: 'A' }),
      item({ storeId: '107', articleCode: 'B', netSalesAmount: 25, category1: 'Health', category1Code: 'B' }),
    ], stores);
    const previous = period('previous', [
      item({ storeId: '107', articleCode: 'A', netSalesAmount: 10, category1: 'Wellness', category1Code: 'A' }),
      item({ storeId: '107', articleCode: 'B', netSalesAmount: 5, category1: 'Health', category1Code: 'B' }),
    ], stores);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.category!.groups).toHaveLength(2);
    expect(contrib.category!.groups.find(group => group.code === 'A')).toMatchObject({ label: 'Health', previousLabel: 'Wellness', renamed: true, delta: 30 });
    expect(contrib.category!.groups.find(group => group.code === 'B')).toMatchObject({ label: 'Health', renamed: false, delta: 20 });
    expect(contrib.category!.transfers).toEqual([expect.objectContaining({ code: 'A', previousName: 'Wellness', currentName: 'Health' })]);
    expect(contrib.category!.totalDelta).toBe(50);
  });

  it('attributes category changes as a decrease in one group and an increase in another', () => {
    const stores = ['107'];
    const current = period('current', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 80, category1: 'Skin', category1Code: 'S' })], stores);
    const previous = period('previous', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 80, category1: 'Health', category1Code: 'H' })], stores);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.category!.groups.find(group => group.code === 'S')).toMatchObject({ current: 80, previous: 0, delta: 80 });
    expect(contrib.category!.groups.find(group => group.code === 'H')).toMatchObject({ current: 0, previous: 80, delta: -80 });
    expect(contrib.category!.totalDelta).toBe(0);
    expect(contrib.store!.totalDelta).toBe(0);
  });

  it('falls back to category name when no code exists', () => {
    const stores = ['107'];
    const current = period('current', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 12, category1: 'Nameless' })], stores);
    const previous = period('previous', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 2, category1: 'Nameless' })], stores);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.category!.groups).toEqual([expect.objectContaining({ key: 'name:Nameless', code: '', label: 'Nameless', delta: 10 })]);
  });

  it('shows a bounded gain/loss subset and a remainder that still lists every omitted group in evidence', () => {
    const storeIds = Array.from({ length: 8 }, (_, index) => `s${index}`);
    const current = period('current', storeIds.map((id, index) => item({ storeId: id, articleCode: id, netSalesAmount: 10 + index })), storeIds);
    const previous = period('previous', storeIds.map(id => item({ storeId: id, articleCode: id, netSalesAmount: 1 })), storeIds);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.store!.gains).toHaveLength(CONTRIBUTION_PREVIEW_LIMIT);
    expect(contrib.store!.groups).toHaveLength(8);
    expect(contrib.store!.remainder).toMatchObject({ count: 3 });
    expect(reconciled(contrib.store!.gains, contrib.store!.losses, contrib.store!.remainder)).toBe(contrib.store!.totalDelta);
    const sheets = salesContributionTables(contrib, t);
    expect(sheets[0]!.rows.filter(row => !row.fixed)).toHaveLength(8);
    expect(sheets[0]!.rows.at(-1)).toMatchObject({ fixed: true, cells: expect.arrayContaining([contrib.store!.totalDelta]) });
  });

  it('suppresses decomposition when insights eligibility is not ready', () => {
    const [current, previous] = dataFixture().periods!;
    expect(buildSalesContributions(current, previous, () => true, 'previousMissing')).toMatchObject({ ready: false, reason: 'previousMissing' });
    expect(buildSalesContributions(current, previous, () => true, 'storesDiffer').store).toBeUndefined();
    current!.items![0]!.netSalesAmount = Number.NaN;
    expect(buildSalesContributions(current, previous, () => true, 'ready')).toMatchObject({ ready: false, reason: 'invalidData' });
  });

  it('does not mutate source periods while aggregating 200k synthetic rows', () => {
    const [current, previous] = dataFixture().periods!;
    current!.items = Array.from({ length: 200000 }, (_, index) => item({
      storeId: index % 2 ? '108' : '107', articleCode: `code-${index % 50}`, netSalesAmount: 1,
      category1: index % 2 ? 'Care' : 'Health', category1Code: index % 2 ? 'C' : 'H',
    }));
    current!.itemCount = current!.items.length;
    previous!.items = [item({ storeId: '107', articleCode: 'seed', netSalesAmount: 0, category1: 'Health', category1Code: 'H' })];
    previous!.itemCount = 1;
    const first = current!.items![0]!, previousFirst = previous!.items![0]!;
    const before = { amount: first.netSalesAmount, code: first.articleCode, count: current!.items!.length, previous: previousFirst.netSalesAmount };
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.ready).toBe(true);
    expect(contrib.store!.totalDelta).toBe(200000);
    expect(contrib.category!.totalDelta).toBe(200000);
    expect(contrib.store!.totalDelta).toBe(contrib.category!.totalDelta);
    expect(first.netSalesAmount).toBe(before.amount);
    expect(first.articleCode).toBe(before.code);
    expect(current!.items).toHaveLength(before.count);
    expect(previousFirst.netSalesAmount).toBe(before.previous);
  });

  it('exports every group as numbers, including formula-like names, and mounts sheets on the overview snapshot', () => {
    const stores = ['107', '108'];
    const current = period('current', [
      item({ storeId: '107', articleCode: 'A', netSalesAmount: 9, category1: '=SUM(1)', category1Code: 'X' }),
      item({ storeId: '108', articleCode: 'B', netSalesAmount: 3, category1: 'Safe', category1Code: 'Y' }),
    ], stores);
    const previous = period('previous', [
      item({ storeId: '107', articleCode: 'A', netSalesAmount: 1, category1: '=SUM(1)', category1Code: 'X' }),
      item({ storeId: '108', articleCode: 'B', netSalesAmount: 1, category1: 'Safe', category1Code: 'Y' }),
    ], stores);
    const insights = buildSalesInsights(current, previous);
    const sheets = salesInsightSheets(insights, t);
    expect(sheets.map(sheet => sheet.id)).toEqual(['insights', 'contrib-stores', 'contrib-categories']);
    const snapshot = workbookSnapshot(sheets, ['synthetic-local'], 'contrib.xlsx');
    const storeSheet = snapshot.sheets.find(sheet => sheet.name === '門店差額拆解')!;
    const categorySheet = snapshot.sheets.find(sheet => sheet.name === '分類差額拆解')!;
    expect(storeSheet.rows.filter(row => typeof row[2] === 'number')).toHaveLength(storeSheet.rows.length);
    expect(storeSheet.rows.some(row => row[0] === '107' && row[4] === 8)).toBe(true);
    expect(storeSheet.rows.some(row => row[0] === '108' && row[4] === 2)).toBe(true);
    expect(categorySheet.rows.some(row => row[1] === '=SUM(1)' && row[4] === 8)).toBe(true);
    const overview = buildAnalysisTables({
      items: [], performance: [], categories: [], stores: [], periods: [], weekAligned: true,
      topSales: [], topQuantity: [], salesGroups: [], quantityGroups: [], focus: [], insights: sheets,
    }, t, 'zh-TW', {}).overview;
    expect(overview.map(table => table.id)).toEqual(expect.arrayContaining(['insights', 'contrib-stores', 'contrib-categories']));
    const single = buildAnalysisTables({
      items: [], performance: [], categories: [], stores: [], periods: [], weekAligned: true,
      topSales: [], topQuantity: [], salesGroups: [], quantityGroups: [], focus: [], insights: sheets[0],
    }, t, 'zh-TW', {}).overview;
    expect(single.filter(table => table.id === 'insights')).toHaveLength(1);
    expect(single.some(table => table.id === 'contrib-stores')).toBe(false);
  });

  it('suppresses infinite group deltas even when product eligibility and overall totals stay finite', () => {
    const stores = ['S1', 'S2'];
    const current = period('current', [
      item({ storeId: 'S1', articleCode: 'P1', netSalesAmount: 1e308, category1: 'C1', category1Code: 'C1' }),
      item({ storeId: 'S2', articleCode: 'P2', netSalesAmount: -1e308, category1: 'C2', category1Code: 'C2' }),
    ], stores);
    const previous = period('previous', [
      item({ storeId: 'S1', articleCode: 'P3', netSalesAmount: -1e308, category1: 'C1', category1Code: 'C1' }),
      item({ storeId: 'S2', articleCode: 'P4', netSalesAmount: 1e308, category1: 'C2', category1Code: 'C2' }),
    ], stores);
    const insights = buildSalesInsights(current, previous);
    expect(insights.reason).toBe('ready');
    expect(insights.entries.length).toBeGreaterThan(0);
    expect(insights.contributions).toMatchObject({ ready: false, reason: 'invalidData' });
    expect(salesInsightSheets(insights, t).some(sheet => sheet.id.startsWith('contrib-'))).toBe(false);
  });

  it('suppresses unreconciled dimension totals from 1e16, 1, -1e16 ordering', () => {
    const stores = ['S1', 'S2'];
    const current = period('current', [
      item({ storeId: 'S1', articleCode: 'P1', netSalesAmount: 1e16, category1: 'C1', category1Code: 'C1' }),
      item({ storeId: 'S1', articleCode: 'P2', netSalesAmount: 1, category1: 'C2', category1Code: 'C2' }),
      item({ storeId: 'S2', articleCode: 'P3', netSalesAmount: -1e16, category1: 'C1', category1Code: 'C1' }),
    ], stores);
    const previous = period('previous', [], stores);
    const insights = buildSalesInsights(current, previous);
    expect(insights.reason).toBe('ready');
    expect(insights.contributions).toMatchObject({ ready: false, reason: 'invalidData' });
  });

  it('keeps a remainder when every group is below the preview threshold', () => {
    const stores = ['S1', 'S2', 'S3'];
    const current = period('current', stores.map((id, index) => item({
      storeId: id, articleCode: `C${index}`, netSalesAmount: 1.004, category1: `Cat${index}`, category1Code: `C${index}`,
    })), stores);
    const previous = period('previous', stores.map((id, index) => item({
      storeId: id, articleCode: `C${index}`, netSalesAmount: 1, category1: `Cat${index}`, category1Code: `C${index}`,
    })), stores);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.ready).toBe(true);
    expect(contrib.store!.gains).toHaveLength(0);
    expect(contrib.store!.losses).toHaveLength(0);
    expect(contrib.store!.remainder).toMatchObject({ count: 3 });
    expect(contrib.store!.remainder!.delta).toBeCloseTo(0.012, 10);
    expect(contrib.category!.remainder).toMatchObject({ count: 3 });
    expect(reconciled(contrib.store!.gains, contrib.store!.losses, contrib.store!.remainder)).toBeCloseTo(contrib.store!.totalDelta, 10);
  });

  it('keeps every renamed category row in export evidence', () => {
    const stores = ['107'];
    const current = period('current', Array.from({ length: 100 }, (_, index) => item({
      storeId: '107', articleCode: `P${index}`, netSalesAmount: 1, category1: `Now${index}`, category1Code: `C${index}`,
    })), stores);
    const previous = period('previous', Array.from({ length: 100 }, (_, index) => item({
      storeId: '107', articleCode: `P${index}`, netSalesAmount: 1, category1: `Was${index}`, category1Code: `C${index}`,
    })), stores);
    const contrib = buildSalesContributions(current, previous, () => true, 'ready');
    expect(contrib.category!.transfers).toHaveLength(100);
    const categorySheet = salesContributionTables(contrib, t)[1]!;
    expect(categorySheet.rows.filter(row => !row.fixed)).toHaveLength(100);
    expect(categorySheet.rows.filter(row => String(row.cells[5]).includes('上期名稱'))).toHaveLength(100);
  });
});
