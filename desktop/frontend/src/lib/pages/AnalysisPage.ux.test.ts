import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings, loadSettings, saveSettings } from '../settings';
import type { SalesAnalysisPackedItems, SalesAnalysisResult, SalesAnalysisTotals } from '../types';
import { packSalesAnalysisItems } from '../salesAnalysisItems';

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

async function renderReport(value = analysisResult()) {
  const run = vi.fn(async () => value);
  const clear = vi.fn(async () => undefined);
  const listStores = vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]);
  configureBackend({ methods: {
    ListProfiles: vi.fn(async () => [profile('profile-1', 'Production'), profile('profile-2', 'Second')]),
    ListSalesAnalysisStores: listStores,
    ListManCodeGroups: vi.fn(async () => [{ id: 'mask', name: '面膜組', codes: ['552646'] }]),
    RunSalesAnalysis: run,
    ClearSalesAnalysis: clear,
    CancelSalesAnalysis: vi.fn(async () => undefined),
  } });
  const change = vi.fn((next: typeof defaultSettings) => {
    void view.rerender({ settings: saveSettings(next) });
  });
  const view = render(AnalysisPage, { props: { t: translator('zh-TW'), settings: { ...defaultSettings }, onSettingsChange: change } });
  await waitFor(() => expect(screen.getByText('107 - Central')).toBeInTheDocument());
  await fireEvent.click(screen.getByText('開始分析'));
  await screen.findByRole('heading', { name: '銷售額 Top 24' });
  return { ...view, run, clear, change, listStores };
}

describe('analysis workspace interactions', () => {
  it('keeps report context immutable while editing and hiding a draft query', async () => {
    const { container, run, clear } = await renderReport();
    const summary = container.querySelector('.report-context')!;
    expect(summary).toHaveTextContent('Production · 2026-08-01 — 2026-08-21 · 1 間門店');
    await fireEvent.click(screen.getByText('調整條件'));
    await fireEvent.change(screen.getByLabelText('帳號'), { target: { value: 'profile-2' } });
    await waitFor(() => expect(screen.getByText('已選 1 間門店')).toBeInTheDocument());
    await fireEvent.input(screen.getByLabelText('月份'), { target: { value: '2026-07' } });
    await fireEvent.click(screen.getByText('收合條件'));
    expect(summary).toHaveTextContent('Production · 2026-08-01 — 2026-08-21 · 1 間門店');
    expect(screen.getByText('條件已變更，尚未重新分析')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '銷售額 Top 24' })).toBeInTheDocument();
    expect(run).toHaveBeenCalledTimes(1);
    expect(clear).not.toHaveBeenCalled();
  });

  it('discards a changed account and ignores its late store reply without rerunning', async () => {
    const { listStores, run, clear } = await renderReport();
    let resolveStores!: (value: Array<{ businessId: string; label: string }>) => void;
    listStores.mockReturnValueOnce(new Promise((resolve) => { resolveStores = resolve; }));
    await fireEvent.click(screen.getByText('調整條件'));
    const month = (screen.getByLabelText('月份') as HTMLInputElement).value;
    await fireEvent.input(screen.getByLabelText('月份'), { target: { value: '2000-01' } });
    await fireEvent.change(screen.getByLabelText('帳號'), { target: { value: 'profile-2' } });
    await fireEvent.click(screen.getByRole('button', { name: '放棄條件變更' }));
    expect(screen.getByLabelText('帳號')).toHaveValue('profile-1');
    expect(screen.getByLabelText('月份')).toHaveValue(month);
    expect(screen.getByText('已選 1 間門店')).toBeInTheDocument();
    resolveStores([{ businessId: '999', label: 'Late wrong account store' }]);
    await waitFor(() => expect(screen.queryByText('Late wrong account store')).not.toBeInTheDocument());
    expect(screen.queryByText('條件已變更，尚未重新分析')).not.toBeInTheDocument();
    expect(run).toHaveBeenCalledTimes(1);
    expect(clear).not.toHaveBeenCalled();
  });

  it('limits bulk store selection to matches and preserves hidden selections', async () => {
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'Production')]),
      ListSalesAnalysisStores: vi.fn(async () => Array.from({ length: 9 }, (_, index) => ({ businessId: String(index), label: `Store ${index}` }))),
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await screen.findByText('已選 9 間門店');
    await fireEvent.input(screen.getByLabelText('搜尋門店'), { target: { value: 'Store 0' } });
    await fireEvent.click(screen.getByRole('button', { name: '取消選取搜尋結果' }));
    expect(screen.getByText('已選 8 間門店')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: '全選搜尋結果' }));
    expect(screen.getByText('已選 9 間門店')).toBeInTheDocument();
    await fireEvent.input(screen.getByLabelText('搜尋門店'), { target: { value: 'missing' } });
    expect(screen.getByRole('button', { name: '全選搜尋結果' })).toBeDisabled();
  });

  it('limits facet bulk actions to search matches while keeping other selections', async () => {
    const value = analysisResult();
    value.periods![0]!.items!.push({ ...value.periods![0]!.items![0]!, articleCode: 'other', category2: 'HEALTH', category2Code: 'B01' });
    const { container, run } = await renderReport(value);
    await fireEvent.click(screen.getByRole('button', { name: '篩選' }));
    const facet = container.querySelectorAll('.facet-menu')[1] as HTMLElement;
    await fireEvent.click(facet.querySelector('summary')!);
    await fireEvent.click(within(facet).getByLabelText(/BEAUTY CARE/));
    await fireEvent.input(screen.getByLabelText('搜尋 商品部門'), { target: { value: 'health' } });
    await fireEvent.click(within(facet).getByRole('button', { name: '全選搜尋結果' }));
    expect(screen.getByRole('button', { name: /移除 .*BEAUTY CARE/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /移除 .*HEALTH/ })).toBeInTheDocument();
    await fireEvent.click(within(facet).getByRole('button', { name: '取消選取搜尋結果' }));
    expect(screen.getByRole('button', { name: /移除 .*BEAUTY CARE/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /移除 .*HEALTH/ })).not.toBeInTheDocument();
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('restores snapshot dates without attributing the report to the first account', async () => {
    vi.mocked(isWebRuntime).mockReturnValue(true);
    vi.mocked(loadWebAnalysisSnapshot).mockReturnValue(analysisResult());
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'Wrong account')]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
    } });
    const { container } = render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await screen.findByRole('heading', { name: '銷售額 Top 24' });
    expect(container.querySelector('.report-context')).toHaveTextContent('已儲存報表 · 2026-08-01 — 2026-08-21');
    await fireEvent.click(screen.getByText('調整條件'));
    expect(container.querySelector('#analysis-from')).toHaveValue('2026-08-01');
    expect(screen.queryByText('條件已變更，尚未重新分析')).not.toBeInTheDocument();
    await fireEvent.input(container.querySelector('#analysis-to')!, { target: { value: '2026-08-20' } });
    expect(screen.getByText('條件已變更，尚未重新分析')).toBeInTheDocument();
  });

  it('reveals filters, searches facet options, removes chips and clears scope without querying', async () => {
    const { container, run } = await renderReport();
    const toggle = screen.getByRole('button', { name: '篩選' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(container.querySelector('#analysis-filter-panel')).toHaveAttribute('hidden');
    await fireEvent.click(toggle);
    const facet = container.querySelectorAll('.facet-menu')[1]!;
    await fireEvent.click(facet.querySelector('summary')!);
    await fireEvent.input(screen.getByLabelText('搜尋 商品部門'), { target: { value: 'beauty' } });
    await fireEvent.click(within(facet as HTMLElement).getByLabelText(/BEAUTY CARE/));
    expect(screen.getByRole('button', { name: /移除 .*BEAUTY CARE/ })).toBeInTheDocument();
    await fireEvent.keyDown(screen.getByLabelText('搜尋 商品部門'), { key: 'Escape' });
    expect(facet).not.toHaveAttribute('open');
    expect(facet.querySelector('summary')).toHaveFocus();
    await fireEvent.change(screen.getByLabelText('商品範圍'), { target: { value: 'mask' } });
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'Mask' } });
    await fireEvent.click(screen.getByRole('button', { name: /移除 .*BEAUTY CARE/ }));
    expect(screen.queryByRole('button', { name: /移除 .*BEAUTY CARE/ })).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: '清除全部篩選' }));
    expect(screen.getByLabelText('商品範圍')).toHaveValue('');
    expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('');
    expect(screen.getByText('共 1 筆商品明細')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '清除全部篩選' })).not.toBeInTheDocument();
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('offers a recovery path for no matches and preserves the selected tab', async () => {
    await renderReport();
    await fireEvent.click(screen.getByRole('tab', { name: '商品' }));
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'not-a-product' } });
    expect(screen.getByText('試試不同的關鍵字，或清除篩選查看全部商品。')).toBeInTheDocument();
    expect(screen.getByText('符合 0 筆商品明細')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: '清除全部篩選' }));
    expect(screen.getByRole('tab', { name: '商品' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('Mask')).toBeInTheDocument();
  });

  it('supports arrow, Home and End navigation with one tab stop', async () => {
    await renderReport();
    const overview = screen.getByRole('tab', { name: '概覽' });
    overview.focus();
    await fireEvent.keyDown(overview, { key: 'ArrowLeft' });
    const stores = screen.getByRole('tab', { name: '門店' });
    expect(stores).toHaveFocus();
    expect(screen.getByRole('tabpanel')).toHaveAttribute('aria-labelledby', stores.id);
    await fireEvent.keyDown(stores, { key: 'Home' });
    expect(overview).toHaveFocus();
    await fireEvent.keyDown(overview, { key: 'End' });
    expect(stores).toHaveAttribute('tabindex', '0');
    expect(overview).toHaveAttribute('tabindex', '-1');
    await fireEvent.keyDown(stores, { key: 'ArrowRight' });
    expect(overview).toHaveAttribute('aria-selected', 'true');
  });

  it('persists presets and custom depth and immediately refreshes category rows', async () => {
    const value = analysisResult();
    const original = value.periods![0]!.items![0]!;
    value.periods![0]!.items = Array.from({ length: 45 }, (_, index) => ({ ...original, articleCode: `item-${index}`, articleName: `Item ${index}` }));
    const { container, run } = await renderReport(value);
    await fireEvent.click(screen.getByRole('button', { name: '16' }));
    await waitFor(() => expect(container.querySelectorAll('.top-card:first-child li')).toHaveLength(16));
    expect(loadSettings().rankingLimit).toBe(16);
    await fireEvent.click(screen.getByRole('tab', { name: '分類' }));
    expect(container.querySelector('.ranking-group ol')!.children).toHaveLength(16);
    const input = screen.getByRole('spinbutton', { name: '自訂' });
    await fireEvent.focus(input);
    await fireEvent.input(input, { target: { value: '40' } });
    await fireEvent.blur(input);
    await waitFor(() => expect(container.querySelector('.ranking-group ol')!.children).toHaveLength(40));
    expect(loadSettings().rankingLimit).toBe(40);
    await fireEvent.click(screen.getByText('匯出 PDF'));
    expect(within(screen.getByRole('dialog')).getByRole('spinbutton', { name: '自訂' })).toHaveValue(40);
    expect(run).toHaveBeenCalledTimes(1);
    localStorage.removeItem('rta-sales-desktop-settings-v2');
  });

  it('keeps the current depth on empty input and clamps custom bounds', async () => {
    await renderReport();
    const input = screen.getByRole('spinbutton', { name: '自訂' });
    for (const [value, expected] of [['', 24], ['999', 100], ['2', 5]] as const) {
      await fireEvent.focus(input);
      await fireEvent.input(input, { target: { value } });
      await fireEvent.blur(input);
      await waitFor(() => expect(input).toHaveValue(expected));
    }
    input.focus();
    await fireEvent.input(input, { target: { value: '80' } });
    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(input).toHaveValue(5);
    localStorage.removeItem('rta-sales-desktop-settings-v2');
  });

  it('ends the loading indicator and shows an error when background item hydration fails', async () => {
    const value = analysisResult();
    value.periods![0]!.items = undefined;
    value.periods![0]!.itemCount = 1;
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile('profile-1', 'Production')]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
      RunSalesAnalysis: vi.fn(async () => value),
      GetSalesAnalysisItems: vi.fn(async () => { throw new Error('offline'); }),
    } });
    render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
    await screen.findByText('107 - Central');
    await fireEvent.click(screen.getByText('開始分析'));
    await screen.findByRole('alert');
    await waitFor(() => expect(screen.queryByText('正在載入商品明細')).not.toBeInTheDocument());
    expect(screen.getByRole('heading', { name: '銷售額 Top 24' })).toBeInTheDocument();
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

async function renderHydratingReport(value: SalesAnalysisResult, load: (request: unknown) => Promise<SalesAnalysisPackedItems>) {
  const getItems = vi.fn(load);
  const run = vi.fn(async () => value);
  const cancel = vi.fn(async () => undefined);
  const listeners = new Map<string, (payload: unknown) => void>();
  configureBackend({ methods: {
    ListProfiles: vi.fn(async () => [profile('profile-1', 'Production')]),
    ListSalesAnalysisStores: vi.fn(async () => [{ businessId: '107', label: '107 - Central' }]),
    RunSalesAnalysis: run,
    GetSalesAnalysisItems: getItems,
    ClearSalesAnalysis: vi.fn(async () => undefined),
    CancelSalesAnalysis: cancel,
  }, events: { on: (name, listener) => { listeners.set(name, listener); return () => { listeners.delete(name); }; } } });
  const view = render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
  await screen.findByText('107 - Central');
  await fireEvent.click(screen.getByText('開始分析'));
  await screen.findByRole('heading', { name: '銷售額 Top 24' });
  return { ...view, run, cancel, getItems, update: (next: SalesAnalysisResult) => listeners.get('rta:sales-analysis-update')!(next) };
}

describe('analysis item recovery', () => {
  it('keeps successful periods, retries only the failed period, and never reruns the query', async () => {
    const original = analysisResult();
    const current = original.periods![0]!;
    const value = { ...original, periods: [
      current,
      { ...current, key: 'previous', label: '上期', items: undefined, itemCount: 1 },
      { ...current, key: 'yearAgo', label: '去年同期', items: undefined, itemCount: 1 },
    ] };
    let fail = true;
    const retry = deferred<SalesAnalysisPackedItems>();
    const { getItems, run } = await renderHydratingReport(value, async (request) => {
      const { periodKey } = request as { periodKey: string };
      if (periodKey === 'previous') {
        if (fail) throw new Error('offline');
        return retry.promise;
      }
      return packSalesAnalysisItems(periodKey, current.items!, original.stores);
    });
    await screen.findByRole('button', { name: '重試失敗明細' });
    await waitFor(() => expect(screen.queryByText('正在載入商品明細')).not.toBeInTheDocument());
    expect(screen.getAllByText('Mask').length).toBeGreaterThan(0);
    expect(getItems.mock.calls.map(([request]) => (request as { periodKey: string }).periodKey)).toEqual(['previous', 'yearAgo']);
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'Mask' } });
    expect(within(screen.getByRole('tabpanel')).getByText('明細載入失敗，請使用上方按鈕重試。')).toBeInTheDocument();
    expect(screen.queryByText('試試不同的關鍵字，或清除篩選查看全部商品。')).not.toBeInTheDocument();
    fail = false;
    await fireEvent.click(screen.getByRole('button', { name: '重試失敗明細' }));
    retry.resolve(packSalesAnalysisItems('previous', current.items!, original.stores));
    await screen.findByRole('heading', { name: '銷售額 Top 24' });
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('Mask');
    expect(getItems.mock.calls.map(([request]) => (request as { periodKey: string }).periodKey)).toEqual(['previous', 'yearAgo', 'previous']);
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('treats short batches as retryable and does not show no-match results while details are missing', async () => {
    const value = analysisResult();
    const items = value.periods![0]!.items!;
    value.periods![0] = { ...value.periods![0]!, items: undefined, itemCount: 1 };
    let recovered = false;
    const { getItems } = await renderHydratingReport(value, async () => recovered
      ? packSalesAnalysisItems('current', items, value.stores) : { rows: [], dict: [] });
    await screen.findByRole('button', { name: '重試失敗明細' });
    expect(screen.getByText(/收到的商品明細不完整/)).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('tab', { name: '商品' }));
    expect(within(screen.getByRole('tabpanel')).queryByText('沒有符合條件的資料')).not.toBeInTheDocument();
    expect(screen.queryByText('共 0 筆商品明細')).not.toBeInTheDocument();
    recovered = true;
    await fireEvent.click(screen.getByRole('button', { name: '重試失敗明細' }));
    await screen.findByText('Mask');
    expect(getItems).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('tab', { name: '商品' })).toHaveAttribute('aria-selected', 'true');
  });

  it('deduplicates in-flight loads and retains hydrated comparison rows across slim updates', async () => {
    const value = analysisResult();
    const source = value.periods![0]!;
    const pending = deferred<SalesAnalysisPackedItems>();
    value.periods = [source, { ...source, key: 'previous', label: '上期', items: undefined, itemCount: 1 }];
    const { getItems, update } = await renderHydratingReport(value, async () => pending.promise);
    update({ ...value });
    await fireEvent.click(screen.getByRole('tab', { name: '分類' }));
    update({ ...value });
    await fireEvent.click(screen.getByRole('tab', { name: '概覽' }));
    expect(getItems).toHaveBeenCalledTimes(1);
    pending.resolve(packSalesAnalysisItems('previous', source.items!, value.stores));
    await waitFor(() => expect(screen.queryByText('正在載入商品明細')).not.toBeInTheDocument());
    update({ ...value });
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'Mask' } });
    await screen.findByRole('heading', { name: '銷售表現' });
    expect(getItems).toHaveBeenCalledTimes(1);
  });

  it('ignores a rejected old operation after a new report replaces it', async () => {
    const value = analysisResult();
    value.periods![0] = { ...value.periods![0]!, items: undefined, itemCount: 1 };
    const old = deferred<SalesAnalysisPackedItems>();
    const { run } = await renderHydratingReport(value, async () => old.promise);
    run.mockResolvedValueOnce({ ...analysisResult(), operationId: 'replacement' });
    await fireEvent.click(screen.getByText('調整條件'));
    await fireEvent.click(screen.getByText('開始分析'));
    await screen.findByText('報表已就緒');
    old.reject(new Error('late old failure'));
    await waitFor(() => expect(screen.getAllByText('Mask').length).toBeGreaterThan(0));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByText('正在載入商品明細')).not.toBeInTheDocument();
  });

  it('locks repeated submissions while cancellation of the previous query is pending', async () => {
    const { run, cancel } = await renderHydratingReport(analysisResult(), async () => ({ rows: [], dict: [] }));
    const cancelled = deferred<undefined>();
    cancel.mockImplementationOnce(() => cancelled.promise);
    await fireEvent.click(screen.getByText('調整條件'));
    const form = screen.getByText('開始分析').closest('form')!;
    await fireEvent.submit(form);
    await fireEvent.submit(form);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(run).toHaveBeenCalledTimes(1);
    cancelled.resolve(undefined);
    await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
    await screen.findByText('報表已就緒');
  });

  it('focuses the product search from the pinned navigation without changing filters', async () => {
    await renderReport();
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'Mask' } });
    await fireEvent.click(screen.getByRole('button', { name: '回到搜尋與篩選' }));
    expect(screen.getByLabelText('搜尋商品或編碼')).toHaveFocus();
    expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('Mask');
  });
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
    await waitFor(() => expect(screen.getByText('調整條件')).toBeInTheDocument());
    expect(screen.getByRole('heading', { name: '接下來關注' })).toBeInTheDocument();
    expect(screen.queryByText('本期已可看，正在補其餘資料')).not.toBeInTheDocument();
    expect(cancel).toHaveBeenCalledWith({ operationId: 'pending-1' });
    expect(clear).not.toHaveBeenCalled();
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
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 24' })).toBeInTheDocument());
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
