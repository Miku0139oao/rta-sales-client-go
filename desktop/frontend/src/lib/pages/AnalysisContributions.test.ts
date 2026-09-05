import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { dataFixture } from '../../test/analysisDataFixture';
import { translator } from '../i18n';
import { buildSalesInsights } from '../salesInsights';
import { buildSalesContributions, CONTRIBUTION_PREVIEW_LIMIT } from '../salesContributions';
import type { SalesAnalysisItem, SalesAnalysisPeriodResult } from '../types';
import AnalysisContributions from './AnalysisContributions.svelte';
import AnalysisInsights from './AnalysisInsights.svelte';

const t = translator('zh-TW');

function item(overrides: Partial<SalesAnalysisItem> & Pick<SalesAnalysisItem, 'storeId' | 'articleCode' | 'netSalesAmount'>): SalesAnalysisItem {
  return {
    storeLabel: `${overrides.storeId} Store`, articleName: overrides.articleCode, brandName: 'Brand',
    category1: 'Health', category2: 'Beauty', category2Code: 'A02', category3: 'Skin', category4: 'Face', category5: '',
    transactionCount: 1, saleQuantity: 1, saleAmount: overrides.netSalesAmount, returnQuantity: 0, returnTransactionCount: 0,
    returnAmount: 0, netQuantity: 1, ...overrides,
  };
}

function period(key: 'current' | 'previous', items: SalesAnalysisItem[], storeIds: string[]): SalesAnalysisPeriodResult {
  return {
    key, label: key === 'current' ? '本期' : '上期', from: '2026-08-01', to: '2026-08-31', complete: true, successfulStores: storeIds.length,
    totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
    stores: storeIds.map(id => ({ businessId: id, label: `${id} Store`, totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 } })),
    items, itemCount: items.length,
  };
}

afterEach(() => cleanup());

describe('analysis contribution discovery', () => {
  it('lists bounded store and category amounts with a reconciling remainder and no combined total', () => {
    const storeIds = Array.from({ length: 7 }, (_, index) => `s${index}`);
    const current = period('current', [
      ...storeIds.map((id, index) => item({ storeId: id, articleCode: id, netSalesAmount: 20 + index, category1: 'Care', category1Code: 'C' })),
      item({ storeId: 's0', articleCode: 'loss', netSalesAmount: 1, category1: 'Drop', category1Code: 'D' }),
    ], storeIds);
    const previous = period('previous', [
      ...storeIds.map(id => item({ storeId: id, articleCode: id, netSalesAmount: 2, category1: 'Care', category1Code: 'C' })),
      item({ storeId: 's0', articleCode: 'loss', netSalesAmount: 40, category1: 'Drop', category1Code: 'D' }),
    ], storeIds);
    const data = buildSalesContributions(current, previous, () => true, 'ready');
    render(AnalysisContributions, { props: { t, locale: 'zh-TW', data } });
    expect(screen.getByRole('heading', { name: '依門店拆解' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '依最上層分類拆解' })).toBeInTheDocument();
    expect(screen.getByText(/分類層級為最上層「商品分類」/)).toBeInTheDocument();
    expect(screen.getByText(/兩個拆解各自對回總差額，不可相加/)).toBeInTheDocument();
    expect(screen.getByText(/其餘 1 組/)).toBeInTheDocument();
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  });

  it('stays hidden when comparison eligibility withholds decomposition', () => {
    const [current, previous] = dataFixture().periods!;
    const data = buildSalesContributions(current, previous, () => true, 'storesDiffer');
    render(AnalysisContributions, { props: { t, locale: 'zh-TW', data } });
    expect(screen.queryByRole('heading', { name: '依門店拆解' })).not.toBeInTheDocument();
  });

  it('discloses a category code rename inside the existing calculation-basis details', async () => {
    const stores = ['107'];
    const current = period('current', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 8, category1: 'Health', category1Code: 'A' })], stores);
    const previous = period('previous', [item({ storeId: '107', articleCode: 'A', netSalesAmount: 2, category1: 'Wellness', category1Code: 'A' })], stores);
    const data = buildSalesInsights(current, previous);
    render(AnalysisInsights, { props: { t, locale: 'zh-TW', data, onProduct() {} } });
    await fireEvent.click(screen.getByText('計算依據與範圍'));
    expect(screen.getByRole('heading', { name: '依門店拆解' })).toBeInTheDocument();
    expect(screen.getByText(/上期名稱：Wellness；本期名稱：Health/)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '淨銷售成長最多' })).toBeInTheDocument();
  });

  it('keeps same-name groups distinguishable by store id and category code', () => {
    const stores = ['107'];
    const current = period('current', [
      item({ storeId: '107', articleCode: 'A', netSalesAmount: 40, category1: 'Health', category1Code: 'A' }),
      item({ storeId: '107', articleCode: 'B', netSalesAmount: 25, category1: 'Health', category1Code: 'B' }),
    ], stores);
    const previous = period('previous', [
      item({ storeId: '107', articleCode: 'A', netSalesAmount: 10, category1: 'Wellness', category1Code: 'A' }),
      item({ storeId: '107', articleCode: 'B', netSalesAmount: 5, category1: 'Health', category1Code: 'B' }),
    ], stores);
    render(AnalysisContributions, { props: { t, locale: 'zh-TW', data: buildSalesContributions(current, previous, () => true, 'ready') } });
    expect(screen.getByRole('listitem', { name: /Health（A）/ })).toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: /Health（B）/ })).toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: /107 Store（107）/ })).toBeInTheDocument();
  });

  it('renders a remainder when every listed change is below the preview threshold', () => {
    const stores = ['S1', 'S2', 'S3'];
    const current = period('current', stores.map((id, index) => item({
      storeId: id, articleCode: `C${index}`, netSalesAmount: 1.004, category1: `Cat${index}`, category1Code: `C${index}`,
    })), stores);
    const previous = period('previous', stores.map((id, index) => item({
      storeId: id, articleCode: `C${index}`, netSalesAmount: 1, category1: `Cat${index}`, category1Code: `C${index}`,
    })), stores);
    render(AnalysisContributions, { props: { t, locale: 'zh-TW', data: buildSalesContributions(current, previous, () => true, 'ready') } });
    expect(screen.getAllByText(/其餘 3 組/).length).toBeGreaterThan(0);
    expect(screen.queryByText('此篩選範圍沒有可列出的增減門店或分類。')).not.toBeInTheDocument();
  });

  it('bounds category rename disclosures and keeps the omitted count visible', () => {
    const stores = ['107'];
    const current = period('current', Array.from({ length: 100 }, (_, index) => item({
      storeId: '107', articleCode: `P${index}`, netSalesAmount: 1, category1: `Now${index}`, category1Code: `C${index}`,
    })), stores);
    const previous = period('previous', Array.from({ length: 100 }, (_, index) => item({
      storeId: '107', articleCode: `P${index}`, netSalesAmount: 1, category1: `Was${index}`, category1Code: `C${index}`,
    })), stores);
    render(AnalysisContributions, { props: { t, locale: 'zh-TW', data: buildSalesContributions(current, previous, () => true, 'ready') } });
    expect(screen.getAllByText(/上期名稱：/).length).toBe(CONTRIBUTION_PREVIEW_LIMIT);
    expect(screen.getByText('其餘 95 筆名稱變更見匯出資料。')).toBeInTheDocument();
  });
});
