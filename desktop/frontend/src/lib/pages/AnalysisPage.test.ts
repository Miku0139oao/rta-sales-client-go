import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import type { SalesAnalysisRequest, SalesAnalysisResult } from '../types';
import AnalysisPage from './AnalysisPage.svelte';

const analysisResult: SalesAnalysisResult = {
  operationId: 'sales-analysis-1',
  from: '2026-08-01',
  to: '2026-08-31',
  complete: true,
  selectedStores: 2,
  successfulStores: 2,
  totals: {
    saleQuantity: 5,
    saleAmount: 190,
    returnQuantity: 1,
    returnAmount: 10,
    netQuantity: 4,
    netSalesAmount: 180,
    trendNetSalesAmount: 240,
    transactionCount: 12,
  },
  stores: [],
  items: [
    {
      storeId: '107', storeLabel: '107 - Central', category1: 'A-HEALTH & BEAUTY', category2: 'BEAUTY CARE',
      category3: 'SKIN CARE', category4: 'FACIAL', category5: 'MASQUE', articleCode: '552646', articleName: 'Mask',
      transactionCount: 2, saleQuantity: 3, saleAmount: 110, returnTransactionCount: 1, returnQuantity: 1, returnAmount: 10, netQuantity: 2, netSalesAmount: 100,
    },
    {
      storeId: '108', storeLabel: '108 - Harbour', category1: 'B-NON FOOD', category2: 'HOUSEHOLD',
      category3: 'CLEANING', category4: 'SURFACE', category5: 'WIPES', articleCode: '900001', articleName: 'Wipes',
      transactionCount: 1, saleQuantity: 2, saleAmount: 80, returnTransactionCount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 80,
    },
  ],
  issues: [],
  queryDurationMs: 100,
};

analysisResult.periods = [
  { key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-31', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'previous', label: '上期', from: '2026-07-01', to: '2026-07-31', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'previous2', label: '前期', from: '2026-05-31', to: '2026-06-30', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'yearAgo', label: '去年同期', from: '2025-08-01', to: '2025-08-31', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
];

beforeEach(() => configureBackend(undefined));
afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('sales analysis page', () => {
  it('queries multiple stores through one profile and filters all five category levels locally', async () => {
    const runSalesAnalysis = vi.fn(async (..._args: unknown[]) => analysisResult);
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [
        { businessId: '107', label: '107 - Central' },
        { businessId: '108', label: '108 - Harbour' },
      ]),
      RunSalesAnalysis: runSalesAnalysis,
    } });
    const { container } = render(AnalysisPage, {
      props: { t: translator('zh-TW'), settings: { ...defaultSettings, accountConcurrency: 4 } },
    });

    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    const storeCheckboxes = container.querySelectorAll('.store-grid input[type="checkbox"]');
    expect(storeCheckboxes).toHaveLength(2);
    await fireEvent.click(storeCheckboxes[1]!);
    await fireEvent.change(screen.getByLabelText('分析期間'), { target: { value: 'range' } });
    await waitFor(() => expect(screen.getByLabelText('開始日期')).toBeInTheDocument());
    await fireEvent.input(screen.getByLabelText('開始日期'), { target: { value: '2026-08-01' } });
    await fireEvent.input(screen.getByLabelText('結束日期'), { target: { value: '2026-08-31' } });
    await fireEvent.click(screen.getByText('開始分析'));

    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售表現' })).toBeInTheDocument());
    const basketKpi = screen.getAllByText('客單價').find((element) => element.tagName === 'DT');
    expect(basketKpi?.parentElement).toHaveTextContent('20.00');
    expect(runSalesAnalysis).toHaveBeenCalledWith({
      profileId: 'profile-1', storeIds: ['107', '108'], concurrency: 4,
      periods: [
        { key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-31', includeTrend: true },
        { key: 'previous', label: '上期', from: '2026-07-01', to: '2026-07-31', includeTrend: true },
        { key: 'previous2', label: '前期', from: '2026-05-31', to: '2026-06-30', includeTrend: true },
        { key: 'yearAgo', label: '去年同期', from: '2025-08-01', to: '2025-08-31', includeTrend: true },
      ],
    });
    await fireEvent.click(screen.getByRole('tab', { name: '商品' }));
    expect(screen.getByText('Mask')).toBeInTheDocument();
    expect(screen.getByText('Wipes')).toBeInTheDocument();

    await fireEvent.click(screen.getAllByText('商品分類')[0]!);
    await fireEvent.click(screen.getByLabelText('A-HEALTH & BEAUTY'));
    await waitFor(() => expect(screen.queryByText('Wipes')).not.toBeInTheDocument());
    expect(screen.getByText('Mask')).toBeInTheDocument();
    expect(container.textContent).toContain('100.00');

    await fireEvent.click(screen.getByRole('tab', { name: '分類' }));
    expect(screen.getByRole('heading', { name: '三期分類比較' })).toBeInTheDocument();
    expect(screen.getAllByText('商品部門').length).toBeGreaterThan(0);
    expect(screen.getAllByText('商品種類').length).toBeGreaterThan(0);
    expect(screen.getAllByText('四級類目').length).toBeGreaterThan(0);
    expect(screen.getAllByText('小分類').length).toBeGreaterThan(0);
  });

  it('does not offer analysis when no enabled profile has credentials', async () => {
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Disabled', enabled: false, priority: 1, hasCredentials: true,
      }]),
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByRole('heading', { name: '沒有可用帳號' })).toBeInTheDocument());
    expect(screen.getByText('管理帳號')).toBeInTheDocument();
  });

  it('compares the current month at the same month-to-date cutoff', async () => {
    const runSalesAnalysis = vi.fn(async (..._args: unknown[]) => analysisResult);
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: runSalesAnalysis,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(runSalesAnalysis).toHaveBeenCalledOnce());

    const now = new Date();
    const localDate = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
    const expectedEnd = (monthOffset: number) => {
      const target = new Date(now.getFullYear(), now.getMonth() + monthOffset, 1);
      const lastDay = new Date(target.getFullYear(), target.getMonth() + 1, 0).getDate();
      return `${target.getFullYear()}-${String(target.getMonth() + 1).padStart(2, '0')}-${String(Math.min(now.getDate(), lastDay)).padStart(2, '0')}`;
    };
    const periods = (runSalesAnalysis.mock.calls[0]![0] as SalesAnalysisRequest).periods!;
    expect(periods[0]!.to).toBe(localDate);
    expect(periods[1]!.to).toBe(expectedEnd(-1));
    expect(periods[2]!.to).toBe(expectedEnd(-2));
  });
});
