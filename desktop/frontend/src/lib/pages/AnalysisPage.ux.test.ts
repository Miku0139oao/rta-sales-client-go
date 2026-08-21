import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import type { SalesAnalysisResult, SalesAnalysisTotals } from '../types';

vi.mock('../sales-report-pdf', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../sales-report-pdf')>();
  return {
    ...actual,
    generateSalesAnalysisPDF: vi.fn(async () => new TextEncoder().encode('%PDF-1.7\nmock report')),
    prepareSalesAnalysisFontFromText: vi.fn(async () => 'Zm9udA=='),
  };
});
vi.mock('../runtime', () => ({ isWebRuntime: vi.fn(() => false) }));
vi.mock('../webStorage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../webStorage')>();
  return { ...actual, loadWebAnalysisSnapshot: vi.fn(() => null) };
});

import { isWebRuntime } from '../runtime';
import { loadWebAnalysisSnapshot } from '../webStorage';
import AnalysisPage from './AnalysisPage.svelte';

const totals: SalesAnalysisTotals = {
  saleQuantity: 2,
  saleAmount: 20,
  returnQuantity: 0,
  returnAmount: 0,
  netQuantity: 2,
  netSalesAmount: 20,
};

function analysisResult(pending = false): SalesAnalysisResult {
  const store = { businessId: '107', label: '107 - Central', totals };
  const item = {
    storeId: '107', storeLabel: '107 - Central', articleCode: '552646', articleName: 'Mask',
    category1: 'A-HEALTH & BEAUTY', category2: 'BEAUTY CARE', category2Code: 'A02',
    category3: 'SKIN CARE', category4: 'FACIAL', category5: 'MASQUE',
    transactionCount: 1, saleQuantity: 2, saleAmount: 20, returnQuantity: 0,
    returnTransactionCount: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 20,
  };
  return {
    operationId: 'pending-1', from: '2026-08-01', to: '2026-08-21',
    complete: !pending, pending, selectedStores: 1, successfulStores: 1,
    totals, stores: [store], queryDurationMs: 10, weeks: [],
    periods: [{
      key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-21',
      complete: true, successfulStores: 1, totals, stores: [store], items: [item],
    }],
  };
}

function profile(id: string, displayName: string) {
  return { id, displayName, enabled: true, priority: 1, hasCredentials: true };
}

beforeEach(() => {
  configureBackend(undefined);
  vi.mocked(isWebRuntime).mockReturnValue(false);
  vi.mocked(loadWebAnalysisSnapshot).mockReturnValue(null);
});

afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('sales analysis UX recovery', () => {
  it('clears the old store search when the account changes', async () => {
    const listStores = vi.fn(async (request: unknown) => {
      const profileId = (request as { profileId: string }).profileId;
      if (profileId === 'profile-1') {
        return Array.from({ length: 9 }, (_, index) => ({
          businessId: `1${index + 1}`,
          label: index === 0 ? 'First account only' : `First store ${index + 1}`,
        }));
      }
      return [{ businessId: '201', label: 'Second account store' }];
    });
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'First'), profile('profile-2', 'Second')]),
      ListSalesAnalysisStores: listStores,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });

    await waitFor(() => expect(screen.getByText('已選 9 間門店')).toBeInTheDocument());
    await fireEvent.input(screen.getByLabelText('搜尋門店'), { target: { value: 'First account only' } });
    expect(screen.getByText('First account only')).toBeInTheDocument();

    await fireEvent.change(screen.getByLabelText('帳號'), { target: { value: 'profile-2' } });
    await waitFor(() => expect(screen.getByText('Second account store')).toBeInTheDocument());
    expect(screen.getByText('已選 1 間門店')).toBeInTheDocument();
  });

  it('shows pending tabs as loading and leaves the result after cancellation', async () => {
    const cancel = vi.fn(async () => undefined);
    const clear = vi.fn(async () => undefined);
    const onBusyChange = vi.fn();
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'Production')]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => analysisResult(true)),
      CancelSalesAnalysis: cancel,
      ClearSalesAnalysis: clear,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings, onBusyChange } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByText('本期已可看，正在補其餘資料')).toBeInTheDocument());
    await waitFor(() => expect(onBusyChange).toHaveBeenLastCalledWith(true));
    const performance = screen.getByRole('heading', { name: '銷售表現' }).closest('section');
    const grossSales = within(performance!).getByText('銷售金額').closest('tr');
    expect(grossSales).toHaveTextContent('—');
    expect(grossSales).not.toHaveTextContent('HK$0.00');

    await fireEvent.click(screen.getByRole('tab', { name: '分類' }));
    const categoryTable = screen.getByRole('heading', { name: '三期分類比較' }).closest('section');
    const category = within(categoryTable!).getByText('BEAUTY CARE').closest('tr');
    expect(category).toHaveTextContent('HK$20.00');
    expect(category).not.toHaveTextContent('HK$0.00');

    await fireEvent.click(screen.getByRole('tab', { name: /每週變化/ }));
    const weekly = screen.getByRole('heading', { name: '每週銷售變化' }).closest('section');
    expect(within(weekly!).getByText('載入中')).toBeInTheDocument();
    expect(within(weekly!).queryByText('沒有可用的每週 Trend 資料。')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('tab', { name: /關注/ }));
    const focus = screen.getByRole('heading', { name: '接下來關注' }).closest('section');
    expect(within(focus!).getByText('載入中')).toBeInTheDocument();
    expect(within(focus!).queryByText('需要月份比較才有去年下月資料。')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByText('取消'));
    await waitFor(() => expect(screen.getByText('開始分析')).toBeInTheDocument());
    expect(cancel).toHaveBeenCalledWith({ operationId: 'pending-1' });
    expect(clear).toHaveBeenCalledWith({ operationId: 'pending-1' });
    await waitFor(() => expect(onBusyChange).toHaveBeenLastCalledWith(false));
  });

  it('does not reopen a pending web snapshot that can no longer report progress', async () => {
    vi.mocked(isWebRuntime).mockReturnValue(true);
    vi.mocked(loadWebAnalysisSnapshot).mockReturnValue(analysisResult(true));
    const clear = vi.fn(async () => undefined);
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'Production')]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      ClearSalesAnalysis: clear,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });

    await waitFor(() => expect(screen.getByText('開始分析')).toBeInTheDocument());
    expect(clear).toHaveBeenCalledWith({ operationId: 'pending-1' });
    expect(screen.queryByText('本期已可看，正在補其餘資料')).not.toBeInTheDocument();
  });

  it('keeps 去年下月 focus in a loading state until its slim rows arrive', async () => {
    let resolveItems!: (value: { periodKey: string; dict: string[]; rows: Record<string, number>[] }) => void;
    const items = new Promise<{ periodKey: string; dict: string[]; rows: Record<string, number>[] }>((resolve) => {
      resolveItems = resolve;
    });
    const result = analysisResult(false);
    result.periods = [
      ...result.periods!,
      {
        key: 'yearAgoNext', label: '去年下月', from: '2025-09-01', to: '2025-09-30',
        complete: true, successfulStores: 1, totals, stores: result.stores, itemCount: 1,
      },
    ];
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'Production')]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => result),
      GetSalesAnalysisItems: vi.fn(async () => items),
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 15' })).toBeInTheDocument());
    await fireEvent.click(screen.getByRole('tab', { name: '關注' }));

    const focus = screen.getByRole('heading', { name: '接下來關注' }).closest('section');
    expect(within(focus!).getByText('載入中')).toBeInTheDocument();
    expect(within(focus!).queryByText('沒有符合條件的資料')).not.toBeInTheDocument();

    resolveItems({
      periodKey: 'yearAgoNext',
      dict: ['', '552646', 'Mask', 'BEAUTY CARE', 'A02'],
      rows: [{ s: 0, ac: 1, an: 2, c2: 3, k2: 4, sq: 2, sa: 20, nq: 2, ns: 20 }],
    });
    await waitFor(() => expect(within(focus!).getAllByText('Mask').length).toBeGreaterThan(0));
  });
});
