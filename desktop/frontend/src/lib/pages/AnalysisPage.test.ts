import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import type { SalesAnalysisRequest, SalesAnalysisResult } from '../types';

vi.mock('../sales-report-pdf', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../sales-report-pdf')>();
  return {
    ...actual,
    generateSalesAnalysisPDF: vi.fn(async () => new TextEncoder().encode('%PDF-1.7\nmock report')),
    prepareSalesAnalysisFont: vi.fn(async () => 'Zm9udA=='),
    prepareSalesAnalysisFontFromText: vi.fn(async () => 'Zm9udA=='),
  };
});

import AnalysisPage from './AnalysisPage.svelte';

function reportMemo() {
  return {
    periods: [{
      key: 'current',
      topAmount: [{ id: '552646', code: '552646', name: 'Mask', amount: 100, quantity: 2 }],
      amountGroups: [{ id: 'A02', code: 'A02', name: 'BEAUTY CARE', amount: 100, quantity: 2 }],
    }],
  };
}

async function confirmExport(files: 'combined' | 'all' = 'combined') {
  await fireEvent.click(screen.getByText('匯出 PDF'));
  await waitFor(() => expect(screen.getByRole('heading', { name: '匯出前先選一下' })).toBeInTheDocument());
  if (files === 'all') await fireEvent.click(screen.getByText('總報告 + 各店各一份'));
  const confirm = screen.getByText('選資料夾並匯出').closest('md-filled-button') ?? screen.getByText('選資料夾並匯出');
  await fireEvent.click(confirm);
}

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
      storeId: '107', storeLabel: '107 - Central', category1: 'A-HEALTH & BEAUTY', category1Code: 'A', category2: 'BEAUTY CARE', category2Code: 'A02',
      category3: 'SKIN CARE', category3Code: 'A0201', category4: 'FACIAL', category5: 'MASQUE', articleCode: '552646', articleName: 'Mask',
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
  weeks: [{
    from: '2026-08-03', to: '2026-08-09',
    totals: {
      salesTw: 200, salesLw: 100, customersTw: 20, customersLw: 10,
      weekdaySalesTw: 120, weekdaySalesLw: 70, weekendSalesTw: 80, weekendSalesLw: 30,
      weekdayCustomersTw: 12, weekdayCustomersLw: 6, weekendCustomersTw: 8, weekendCustomersLw: 4,
    },
    stores: [{
      businessId: '107', label: '107 - Central',
      salesTw: 200, salesLw: 100, customersTw: 20, customersLw: 10,
      weekdaySalesTw: 120, weekdaySalesLw: 70, weekendSalesTw: 80, weekendSalesLw: 30,
      weekdayCustomersTw: 12, weekdayCustomersLw: 6, weekendCustomersTw: 8, weekendCustomersLw: 4,
    }],
  }],
};

analysisResult.periods = [
  { key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-31', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'previous', label: '上期', from: '2026-07-01', to: '2026-07-31', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'previous2', label: '前期', from: '2026-05-31', to: '2026-06-30', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'yearAgo', label: '去年同期', from: '2025-08-01', to: '2025-08-31', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
  { key: 'yearAgoNext', label: '去年下月', from: '2025-09-01', to: '2025-09-30', complete: true, successfulStores: 2, totals: analysisResult.totals, stores: [], items: analysisResult.items, issues: [] },
];

beforeEach(() => configureBackend(undefined));
afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('sales analysis page', () => {
  it('lists sixteen simulated stores when the testing option is enabled', async () => {
    const listStores = vi.fn(async (_request: unknown) => (
      Array.from({ length: 16 }, (_, index) => ({
        businessId: index === 0 ? '107' : `107~sim${String(index + 1).padStart(2, '0')}`,
        label: index === 0 ? '107 - Central' : `107 - Central · 模擬 ${String(index + 1).padStart(2, '0')}`,
      }))
    ));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: listStores,
    } });
    render(AnalysisPage, {
      props: { t: translator('zh-TW'), settings: { ...defaultSettings, simulateStoreCount: 16 } },
    });

    await waitFor(() => expect(screen.getByText('已選 16 間門店')).toBeInTheDocument());
    expect(listStores).toHaveBeenCalledWith({ profileId: 'profile-1', simulateStoreCount: 16 });
    expect(screen.getByText('107 - Central · 模擬 16')).toBeInTheDocument();
  });

  it('loads packed product rows after a slim analysis summary', async () => {
    const getSalesAnalysisItems = vi.fn(async () => ({
      periodKey: 'current',
      dict: ['', '552646', 'Mask', 'A-HEALTH & BEAUTY', 'A', 'BEAUTY CARE', 'A02', 'SKIN CARE', 'A0201', 'FACIAL', 'MASQUE'],
      rows: [{ s: 0, ac: 1, an: 2, c1: 3, k1: 4, c2: 5, k2: 6, c3: 7, k3: 8, c4: 9, c5: 10, t: 2, sq: 3, sa: 110, nq: 2, ns: 100 }],
    }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => ({
        operationId: 'slim-1', from: '2026-08-01', to: '2026-08-31', complete: true,
        selectedStores: 1, successfulStores: 1, queryDurationMs: 10,
        totals: analysisResult.totals, stores: [{ businessId: '107', label: '107 - Central', totals: analysisResult.totals }],
        periods: [{
          key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-31', complete: true,
          successfulStores: 1, itemCount: 1, totals: analysisResult.totals,
          stores: [{ businessId: '107', label: '107 - Central', totals: analysisResult.totals }],
        }],
      })),
      GetSalesAnalysisItems: getSalesAnalysisItems,
      ClearSalesAnalysis: vi.fn(async () => undefined),
    } });
    render(AnalysisPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings },
    });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(getSalesAnalysisItems).toHaveBeenCalledWith({ operationId: 'slim-1', periodKey: 'current' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 15' }).closest('section')).toHaveTextContent('Mask'));
    expect(screen.queryByText('沒有符合條件的資料')).not.toBeInTheDocument();
  });

  it('queries multiple stores through one profile and filters all five category levels locally', async () => {
    const runSalesAnalysis = vi.fn(async (..._args: unknown[]) => analysisResult);
    const chooseSalesAnalysisPDFDirectory = vi.fn(async () => 'D:\\RTA Reports');
    const writeSalesAnalysisPDF = vi.fn(async (...args: unknown[]) => {
      const request = args[0] as { directory: string; filename: string; dataBase64: string };
      return `${request.directory}\\${request.filename}`;
    });
    const openSavedFolder = vi.fn(async () => undefined);
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [
        { businessId: '107', label: '107 - Central' },
        { businessId: '108', label: '108 - Harbour' },
      ]),
      RunSalesAnalysis: runSalesAnalysis,
      ClearSalesAnalysis: vi.fn(async () => undefined),
      GetSalesAnalysisItems: vi.fn(async (request: unknown) => {
        const { periodKey } = request as { periodKey: string };
        return {
          periodKey,
          dict: ['', '552646', 'Mask', 'A-HEALTH & BEAUTY', 'A', 'BEAUTY CARE', 'A02', 'SKIN CARE', 'A0201', 'FACIAL', 'MASQUE', '900001', 'Wipes', 'B-NON FOOD', 'HOUSEHOLD', 'CLEANING', 'SURFACE', 'WIPES'],
          rows: [
            { s: 0, ac: 1, an: 2, c1: 3, k1: 4, c2: 5, k2: 6, c3: 7, k3: 8, c4: 9, c5: 10, t: 2, sq: 3, sa: 110, nq: 2, ns: 100 },
            { s: 1, ac: 11, an: 12, c1: 13, c2: 14, c3: 15, c4: 16, c5: 17, t: 1, sq: 2, sa: 80, nq: 2, ns: 80 },
          ],
        };
      }),
      ChooseSalesAnalysisPDFDirectory: chooseSalesAnalysisPDFDirectory,
      GetSalesAnalysisReportMemo: vi.fn(async () => reportMemo()),
      WriteSalesAnalysisPDF: writeSalesAnalysisPDF,
      OpenSavedFolder: openSavedFolder,
    } });
    const { container } = render(AnalysisPage, {
      props: { t: translator('zh-TW'), settings: { ...defaultSettings, accountConcurrency: 4 } },
    });

    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    const storeCheckboxes = container.querySelectorAll('.store-grid input[type="checkbox"]');
    expect(storeCheckboxes).toHaveLength(2);
    expect([...storeCheckboxes].every((checkbox) => (checkbox as HTMLInputElement).checked)).toBe(true);
    expect(screen.getByText('已選 2 間門店')).toBeInTheDocument();
    await fireEvent.change(screen.getByLabelText('分析期間'), { target: { value: 'range' } });
    await waitFor(() => expect(screen.getByLabelText('開始日期')).toBeInTheDocument());
    await fireEvent.input(screen.getByLabelText('開始日期'), { target: { value: '2026-08-01' } });
    await fireEvent.input(screen.getByLabelText('結束日期'), { target: { value: '2026-08-31' } });
    await fireEvent.click(screen.getByText('開始分析'));

    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售表現' })).toBeInTheDocument());
    const basketKpi = screen.getAllByText('客單價').find((element) => element.tagName === 'DT');
    expect(basketKpi?.parentElement).toHaveTextContent('20.00');
    const topSales = screen.getByRole('heading', { name: '銷售額 Top 15' }).closest('section');
    const topQuantity = screen.getByRole('heading', { name: '銷量 Top 15' }).closest('section');
    expect(topSales?.querySelector('.top-metrics')).toHaveTextContent('100.00');
    expect(topSales?.querySelector('.top-metrics')).toHaveTextContent('2 件');
    expect(topQuantity?.querySelector('.top-metrics')).toHaveTextContent('2 件');
    expect(topQuantity?.querySelector('.top-metrics')).toHaveTextContent('100.00');
    await confirmExport('all');
    await waitFor(() => expect(writeSalesAnalysisPDF).toHaveBeenCalledTimes(3));
    expect(chooseSalesAnalysisPDFDirectory).toHaveBeenCalledOnce();
    const firstPDF = writeSalesAnalysisPDF.mock.calls[0]![0] as { directory: string; filename: string; dataBase64: string };
    expect(firstPDF).toMatchObject({
      directory: 'D:\\RTA Reports', filename: 'RTA-Sales-107-20260801-20260831.pdf',
    });
    expect(firstPDF.dataBase64).toBe(btoa('%PDF-1.7\nmock report'));
    expect(writeSalesAnalysisPDF.mock.calls[1]?.[0]).toMatchObject({ filename: 'RTA-Sales-108-20260801-20260831.pdf' });
    expect(writeSalesAnalysisPDF.mock.calls[2]?.[0]).toMatchObject({ filename: 'RTA-Sales-all-20260801-20260831.pdf' });
    expect(screen.getByText(/已匯出 1 份總報告與 2 份分店報告/)).toBeInTheDocument();
    expect(screen.getByText('D:\\RTA Reports')).toBeInTheDocument();
    await fireEvent.click(screen.getByText('開啟資料夾'));
    await waitFor(() => expect(openSavedFolder).toHaveBeenCalledWith({ path: 'D:\\RTA Reports' }));
    expect(runSalesAnalysis).toHaveBeenCalledWith({
      profileId: 'profile-1', storeIds: ['107', '108'], concurrency: 4, simulateStoreCount: 0,
      periods: [
        { key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-31', includeTrend: true },
        { key: 'previous', label: '上期', from: '2026-07-01', to: '2026-07-31', includeTrend: true },
        { key: 'previous2', label: '前期', from: '2026-05-31', to: '2026-06-30', includeTrend: true },
        { key: 'yearAgo', label: '去年同期', from: '2025-08-01', to: '2025-08-31', includeTrend: true },
        { key: 'yearAgoNext', label: '去年下月', from: '2025-09-01', to: '2025-09-30', includeTrend: false },
      ],
    });
    await fireEvent.click(screen.getByRole('tab', { name: '每週變化' }));
    expect(screen.getByRole('heading', { name: '每週銷售變化' })).toBeInTheDocument();
    expect(screen.getByText('本週')).toBeInTheDocument();
    expect(screen.getByText('上週')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('tab', { name: '關注' }));
    expect(screen.getByRole('heading', { name: '接下來關注' })).toBeInTheDocument();
    expect(screen.getByText('去年下月熱賣，用來準備接下來要補貨或推廣的商品。')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('護膚')).toBeInTheDocument());
    expect(screen.getAllByText('Mask').length).toBeGreaterThan(0);
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
    expect(screen.getByRole('heading', { name: '分類商品銷售排行' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '分類商品銷量排行' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '去年下月' })).toBeInTheDocument();
    expect(screen.getAllByText('商品部門').length).toBeGreaterThan(0);
    expect(screen.getAllByText('商品種類').length).toBeGreaterThan(0);
    expect(screen.getAllByText('四級類目').length).toBeGreaterThan(0);
    expect(screen.getAllByText('小分類').length).toBeGreaterThan(0);
  });

  it('exports PDFs from a slim analysis without dropping period rows', async () => {
    const writeSalesAnalysisPDF = vi.fn(async (...args: unknown[]) => {
      const request = args[0] as { directory: string; filename: string };
      return `${request.directory}\\${request.filename}`;
    });
    const getSalesAnalysisItems = vi.fn(async (request: unknown) => {
      const { periodKey } = request as { periodKey: string };
      return {
        periodKey,
        dict: ['', '552646', 'Mask', 'A-HEALTH & BEAUTY', 'A', 'BEAUTY CARE', 'A02', 'SKIN CARE', 'A0201', 'FACIAL', 'MASQUE'],
        rows: [{ s: 0, ac: 1, an: 2, c1: 3, k1: 4, c2: 5, k2: 6, c3: 7, k3: 8, c4: 9, c5: 10, t: 2, sq: 3, sa: 110, nq: 2, ns: 100 }],
      };
    });
    const totals = analysisResult.totals;
    const slimPeriod = (key: string, label: string, from: string, to: string) => ({
      key, label, from, to, complete: true, successfulStores: 1, itemCount: 1, totals,
      stores: [{ businessId: '107', label: '107 - Central', totals }],
    });
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => ({
        operationId: 'slim-export', from: '2026-08-01', to: '2026-08-31', complete: true,
        selectedStores: 1, successfulStores: 1, queryDurationMs: 10,
        totals, stores: [{ businessId: '107', label: '107 - Central', totals }],
        periods: [
          slimPeriod('current', '本期', '2026-08-01', '2026-08-31'),
          slimPeriod('previous', '上期', '2026-07-01', '2026-07-31'),
          slimPeriod('previous2', '前期', '2026-06-01', '2026-06-30'),
          slimPeriod('yearAgo', '去年同期', '2025-08-01', '2025-08-31'),
          slimPeriod('yearAgoNext', '去年下月', '2025-09-01', '2025-09-30'),
        ],
      })),
      GetSalesAnalysisItems: getSalesAnalysisItems,
      ClearSalesAnalysis: vi.fn(async () => undefined),
      ChooseSalesAnalysisPDFDirectory: vi.fn(async () => 'D:\\RTA Reports'),
      GetSalesAnalysisReportMemo: vi.fn(async () => reportMemo()),
      WriteSalesAnalysisPDF: writeSalesAnalysisPDF,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 15' })).toBeInTheDocument());
    await waitFor(() => expect(screen.queryByText('正在載入商品明細')).not.toBeInTheDocument());
    await fireEvent.click(screen.getByRole('tab', { name: '分類' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: '三期分類比較' })).toBeInTheDocument());
    await waitFor(() => expect(screen.queryByText('正在載入商品明細')).not.toBeInTheDocument());
    await confirmExport();
    await waitFor(() => expect(writeSalesAnalysisPDF).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('已匯出 1 份報告')).toBeInTheDocument();
    expect(getSalesAnalysisItems.mock.calls.map((call) => (call[0] as { periodKey: string }).periodKey)).toEqual(
      expect.arrayContaining(['current', 'previous', 'previous2', 'yearAgo']),
    );
  });

  it('shows a PDF-specific error when report rendering fails', async () => {
    const { generateSalesAnalysisPDF } = await import('../sales-report-pdf');
    vi.mocked(generateSalesAnalysisPDF).mockRejectedValueOnce(new Error('font exploded'));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => analysisResult),
      GetSalesAnalysisItems: vi.fn(async () => ({
        periodKey: 'current',
        dict: ['', '552646', 'Mask'],
        rows: [{ s: 0, ac: 1, an: 2, t: 2, sq: 3, sa: 110, nq: 2, ns: 100 }],
      })),
      ClearSalesAnalysis: vi.fn(async () => undefined),
      ChooseSalesAnalysisPDFDirectory: vi.fn(async () => 'D:\\RTA Reports'),
      GetSalesAnalysisReportMemo: vi.fn(async () => reportMemo()),
      WriteSalesAnalysisPDF: vi.fn(async () => 'D:\\RTA Reports\\report.pdf'),
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 15' })).toBeInTheDocument());
    await confirmExport();
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('無法產生 PDF 報告，請再試一次。'));
    expect(screen.queryByText('發生未預期的錯誤，請再試一次。')).not.toBeInTheDocument();
  });

  it('opens a simple export chooser and can cancel without writing a file', async () => {
    const writeSalesAnalysisPDF = vi.fn(async () => 'D:\\RTA Reports\\report.pdf');
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => analysisResult),
      GetSalesAnalysisItems: vi.fn(async () => ({ periodKey: 'current', dict: [''], rows: [] })),
      ClearSalesAnalysis: vi.fn(async () => undefined),
      ChooseSalesAnalysisPDFDirectory: vi.fn(async () => 'D:\\RTA Reports'),
      WriteSalesAnalysisPDF: writeSalesAnalysisPDF,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 15' })).toBeInTheDocument());
    await fireEvent.click(screen.getByText('匯出 PDF'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '匯出前先選一下' })).toBeInTheDocument());
    expect(screen.getByText('不要放沒有金額的贈品（現金券會留）')).toBeInTheDocument();
    expect(screen.getByText('不要放印花')).toBeInTheDocument();
    await fireEvent.click(screen.getByText('取消'));
    await waitFor(() => expect(screen.queryByRole('heading', { name: '匯出前先選一下' })).not.toBeInTheDocument());
    expect(writeSalesAnalysisPDF).not.toHaveBeenCalled();
  });

  it('can export only the combined report instead of one file per store', async () => {
    const writeSalesAnalysisPDF = vi.fn(async (...args: unknown[]) => {
      const request = args[0] as { directory: string; filename: string };
      return `${request.directory}\\${request.filename}`;
    });
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [
        { businessId: '107', label: '107 - Central' },
        { businessId: '108', label: '108 - Harbour' },
      ]),
      RunSalesAnalysis: vi.fn(async () => analysisResult),
      GetSalesAnalysisItems: vi.fn(async () => ({
        periodKey: 'current',
        dict: ['', '552646', 'Mask', 'A-HEALTH & BEAUTY', 'A', 'BEAUTY CARE', 'A02', 'SKIN CARE', 'A0201', 'FACIAL', 'MASQUE'],
        rows: [{ s: 0, ac: 1, an: 2, c1: 3, k1: 4, c2: 5, k2: 6, c3: 7, k3: 8, c4: 9, c5: 10, t: 2, sq: 3, sa: 110, nq: 2, ns: 100 }],
      })),
      ClearSalesAnalysis: vi.fn(async () => undefined),
      ChooseSalesAnalysisPDFDirectory: vi.fn(async () => 'D:\\RTA Reports'),
      GetSalesAnalysisReportMemo: vi.fn(async () => reportMemo()),
      WriteSalesAnalysisPDF: writeSalesAnalysisPDF,
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 15' })).toBeInTheDocument());
    await fireEvent.click(screen.getByText('匯出 PDF'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '匯出前先選一下' })).toBeInTheDocument());
    expect(screen.getByText('只要總報告')).toBeInTheDocument();
    expect(screen.getByText('1 個 PDF，所有門店放在一起')).toBeInTheDocument();
    expect(screen.getByText('總報告 + 各店各一份')).toBeInTheDocument();
    const confirm = screen.getByText('選資料夾並匯出').closest('md-filled-button') ?? screen.getByText('選資料夾並匯出');
    await fireEvent.click(confirm);
    await waitFor(() => expect(writeSalesAnalysisPDF).toHaveBeenCalledTimes(1));
    expect(writeSalesAnalysisPDF.mock.calls[0]?.[0]).toMatchObject({ filename: 'RTA-Sales-all-20260801-20260831.pdf' });
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
    expect(periods[4]).toMatchObject({ key: 'yearAgoNext', includeTrend: false });
    const nextMonthLastYear = new Date(now.getFullYear() - 1, now.getMonth() + 1, 1);
    const nextMonthLastYearEnd = new Date(nextMonthLastYear.getFullYear(), nextMonthLastYear.getMonth() + 1, 0);
    expect(periods[4]!.from).toBe(`${nextMonthLastYear.getFullYear()}-${String(nextMonthLastYear.getMonth() + 1).padStart(2, '0')}-01`);
    expect(periods[4]!.to).toBe(`${nextMonthLastYearEnd.getFullYear()}-${String(nextMonthLastYearEnd.getMonth() + 1).padStart(2, '0')}-${String(nextMonthLastYearEnd.getDate()).padStart(2, '0')}`);
  });
});
