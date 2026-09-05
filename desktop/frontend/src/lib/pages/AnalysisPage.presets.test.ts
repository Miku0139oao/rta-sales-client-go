import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import { ANALYSIS_PRESETS_KEY, loadAnalysisPresets, putAnalysisPreset, saveAnalysisPresets, setAnalysisPresetPinned, type AnalysisPresetDraft } from '../analysisPresets';
import type { SalesAnalysisResult } from '../types';
import AnalysisPage from './AnalysisPage.svelte';

function draft(): AnalysisPresetDraft {
  return {
    query: { profileId: 'profile-2', profileName: 'Second', periodMode: 'range', monthMode: 'fixed', month: '2026-07', from: '2026-07-01', to: '2026-07-21', weekCompare: true, storeIds: ['108'] },
    filters: { search: 'Mask', groupId: 'mask', groupLevel: 'category3', categories: { category1: [], category2: ['BEAUTY CARE'], category3: [], category4: [], category5: [] } },
  };
}
function report(): SalesAnalysisResult {
  const totals = { saleQuantity: 2, saleAmount: 20, returnQuantity: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 20 };
  const store = { businessId: '107', label: '107 - Central', totals };
  const item = { storeId: '107', storeLabel: store.label, articleCode: '552646', articleName: 'Mask', category1: 'Health', category2: 'BEAUTY CARE', category2Code: 'A02', category3: 'Skin', category4: 'Face', category5: 'Mask', transactionCount: 1, saleQuantity: 2, saleAmount: 20, returnQuantity: 0, returnTransactionCount: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 20 };
  return { operationId: 'initial-report', from: '2026-08-01', to: '2026-08-21', complete: true, pending: false, selectedStores: 1, successfulStores: 1, totals, stores: [store], queryDurationMs: 10, weeks: [], periods: [
    { key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-21', complete: true, successfulStores: 1, totals, stores: [store], items: [item, { ...item, articleName: 'Wipes', articleCode: 'other' }] },
  ] };
}
async function setup(withReport = true) {
  const run = vi.fn(async (_request: unknown) => report());
  const clear = vi.fn(async () => undefined);
  const listStores = vi.fn(async (request: unknown) => (request as { profileId: string }).profileId === 'profile-1'
    ? [{ businessId: '107', label: '107 - Central' }]
    : [{ businessId: '108', label: '108 - Harbour' }, { businessId: '109', label: '109 - East' }]);
  configureBackend({ methods: {
    ListProfiles: vi.fn(async () => ['profile-1', 'profile-2'].map((id, index) => ({ id, displayName: index ? 'Second' : 'Production', enabled: true, priority: 1, hasCredentials: true }))),
    ListSalesAnalysisStores: listStores,
    ListManCodeGroups: vi.fn(async () => [{ id: 'mask', name: '面膜組', codes: ['552646'] }]),
    RunSalesAnalysis: run, ClearSalesAnalysis: clear, CancelSalesAnalysis: vi.fn(async () => undefined),
  } });
  const view = render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
  await screen.findByText('107 - Central');
  if (withReport) {
    await fireEvent.click(screen.getByText('開始分析'));
    await screen.findByRole('heading', { name: '銷售額 Top 24' });
  }
  return { ...view, run, clear, listStores };
}
async function open() {
  const trigger = screen.getByRole('button', { name: '常用條件' });
  trigger.focus();
  await fireEvent.click(trigger);
  return within(screen.getByRole('dialog', { name: '常用條件' }));
}
async function close() {
  await fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }));
}
async function applySaved() {
  const dialog = await open();
  await fireEvent.click(dialog.getByRole('button', { name: '套用條件' }));
  await screen.findByText(/已帶入「/);
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
}

beforeEach(() => { localStorage.clear(); configureBackend(undefined); });
afterEach(() => { cleanup(); configureBackend(undefined); vi.restoreAllMocks(); });

describe('saved analysis conditions', () => {
  it('saves the current draft and screen filters, reloads them, and restores focus on Escape', async () => {
    const { run, unmount } = await setup();
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'Mask' } });
    const dialog = await open();
    await fireEvent.input(dialog.getByLabelText('條件名稱'), { target: { value: '  每月美容  ' } });
    await fireEvent.click(dialog.getByRole('button', { name: '另存常用條件' }));
    expect(dialog.getByText('常用條件已儲存。')).toBeInTheDocument();
    expect(loadAnalysisPresets()[0]).toMatchObject({ name: '每月美容', query: { profileId: 'profile-1', storeIds: ['107'] }, filters: { search: 'Mask' } });
    expect(run).toHaveBeenCalledTimes(1);
    await close();
    await waitFor(() => expect(screen.getByRole('button', { name: '常用條件' })).toHaveFocus());
    unmount();
    await setup(false);
    const reopened = await open();
    expect(reopened.getByRole('option', { name: '每月美容' })).toBeInTheDocument();
  });

  it('renames, updates after confirmation, and deletes only after confirmation', async () => {
    putAnalysisPreset([], draft(), 'Old');
    await setup();
    const dialog = await open();
    await fireEvent.click(dialog.getByRole('button', { name: '重新命名' }));
    await fireEvent.input(dialog.getByLabelText('新名稱'), { target: { value: 'New' } });
    await fireEvent.click(dialog.getByRole('button', { name: '確認' }));
    expect(loadAnalysisPresets()[0]!.name).toBe('New');
    await fireEvent.click(dialog.getByRole('button', { name: '更新為目前條件' }));
    expect(loadAnalysisPresets()[0]!.query.profileId).toBe('profile-2');
    await fireEvent.click(dialog.getByRole('button', { name: '確認' }));
    expect(loadAnalysisPresets()[0]!.query.profileId).toBe('profile-1');
    await fireEvent.click(dialog.getByRole('button', { name: '刪除' }));
    await fireEvent.click(dialog.getByRole('button', { name: '取消' }));
    expect(loadAnalysisPresets()).toHaveLength(1);
    await fireEvent.click(dialog.getByRole('button', { name: '刪除' }));
    await fireEvent.click(dialog.getByRole('button', { name: '確認' }));
    expect(loadAnalysisPresets()).toEqual([]);
  });

  it('reports duplicate names and failed writes without claiming success', async () => {
    putAnalysisPreset([], draft(), 'Existing');
    await setup();
    const dialog = await open();
    await fireEvent.input(dialog.getByLabelText('條件名稱'), { target: { value: 'existing' } });
    await fireEvent.click(dialog.getByRole('button', { name: '另存常用條件' }));
    expect(dialog.getByRole('alert')).toHaveTextContent('已有相同名稱');
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Quota'); });
    await fireEvent.input(dialog.getByLabelText('條件名稱'), { target: { value: 'Next' } });
    await fireEvent.click(dialog.getByRole('button', { name: '另存常用條件' }));
    expect(dialog.getByRole('alert')).toHaveTextContent('儲存失敗');
    expect(loadAnalysisPresets()).toHaveLength(1);
    expect(dialog.queryByText('常用條件已儲存。')).not.toBeInTheDocument();
  });

  it('does not overwrite unreadable storage and allows retrying the read', async () => {
    localStorage.setItem(ANALYSIS_PRESETS_KEY, 'invalid');
    await setup();
    const dialog = await open();
    expect(dialog.getByRole('alert')).toHaveTextContent('無法讀取');
    expect(dialog.getByRole('button', { name: '另存常用條件' })).toBeDisabled();
    expect(localStorage.getItem(ANALYSIS_PRESETS_KEY)).toBe('invalid');
    localStorage.removeItem(ANALYSIS_PRESETS_KEY);
    await fireEvent.click(dialog.getByRole('button', { name: '重新讀取' }));
    expect(dialog.queryByRole('alert')).not.toBeInTheDocument();
    expect(dialog.getByRole('button', { name: '另存常用條件' })).not.toBeDisabled();
  });

  it('stages a query without touching the old report, then applies all filters on explicit analysis', async () => {
    putAnalysisPreset([], draft(), 'July masks');
    const { run, clear, container } = await setup();
    await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'), { target: { value: 'Wipes' } });
    await applySaved();
    expect(screen.getByLabelText('帳號')).toHaveValue('profile-2');
    expect(screen.getByLabelText('開始日期')).toHaveValue('2026-07-01');
    expect(screen.getByLabelText('以星期比較')).toBeChecked();
    expect(screen.getByText('已選 1 間門店')).toBeInTheDocument();
    expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('Wipes');
    expect(container.querySelector('.report-context')).toHaveTextContent('Production');
    expect(run).toHaveBeenCalledTimes(1);
    expect(clear).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByText('重新分析'));
    await waitFor(() => expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('Mask'));
    expect(run).toHaveBeenLastCalledWith(expect.objectContaining({ profileId: 'profile-2', storeIds: ['108'] }));
    expect(screen.getByRole('button', { name: /移除 .*BEAUTY CARE/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /移除 面膜組/ })).toBeInTheDocument();
    expect(screen.queryByText(/已帶入「July masks」/)).not.toBeInTheDocument();
  });

  it('can discard staged conditions and keep the previous report and filters', async () => {
    putAnalysisPreset([], draft(), 'July masks');
    const { run } = await setup();
    await applySaved();
    await fireEvent.click(screen.getByRole('button', { name: '放棄條件變更' }));
    expect(screen.getByLabelText('帳號')).toHaveValue('profile-1');
    expect(screen.queryByText(/已帶入/)).not.toBeInTheDocument();
    expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('');
    expect(run).toHaveBeenCalledTimes(1);
  });

  it.each(['account', 'group', 'stores'])('rejects unavailable %s without changing the draft', async (kind) => {
    const value = draft();
    if (kind === 'account') value.query.profileId = 'removed';
    if (kind === 'group') value.filters.groupId = 'removed';
    if (kind === 'stores') value.query.storeIds = ['removed'];
    putAnalysisPreset([], value, 'Missing');
    const { run, container } = await setup();
    const dialog = await open();
    await fireEvent.click(dialog.getByRole('button', { name: '套用條件' }));
    await waitFor(() => expect(dialog.getByRole('alert')).toHaveTextContent(kind === 'account' ? '帳號已移除' : kind === 'group' ? '商品群組已不存在' : '門店目前皆不可用'));
    expect(container.querySelector('.report-context')).toHaveTextContent('Production');
    expect(run).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/已帶入/)).not.toBeInTheDocument();
  });

  it('warns about missing stores, keeps only saved available IDs, and never adds newly listed stores', async () => {
    const value = draft(); value.query.storeIds = ['108', 'removed'];
    putAnalysisPreset([], value, 'Partial');
    const { run } = await setup();
    await applySaved();
    expect(screen.getByText(/已略過 1 間不可用門店：removed/)).toBeInTheDocument();
    expect(screen.getByText('已選 1 間門店')).toBeInTheDocument();
    await fireEvent.click(screen.getByText('重新分析'));
    await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
    expect(run).toHaveBeenLastCalledWith(expect.objectContaining({ storeIds: ['108'] }));
  });

  it('leaves the draft intact when store verification fails', async () => {
    putAnalysisPreset([], draft(), 'Retry');
    const { listStores, run } = await setup(false);
    listStores.mockRejectedValueOnce(new Error('offline'));
    const dialog = await open();
    await fireEvent.click(dialog.getByRole('button', { name: '套用條件' }));
    await waitFor(() => expect(dialog.getByRole('alert')).toHaveTextContent('無法確認門店'));
    await close();
    expect(screen.getByLabelText('帳號')).toHaveValue('profile-1');
    expect(screen.getByText('107 - Central')).toBeInTheDocument();
    expect(run).not.toHaveBeenCalled();
  });

  it('saves a moving month rule and resolves it when applied without a report', async () => {
    const { run } = await setup(false);
    const dialog = await open();
    await fireEvent.change(dialog.getByLabelText('期間規則'), { target: { value: 'previous' } });
    await fireEvent.input(dialog.getByLabelText('條件名稱'), { target: { value: 'Last month' } });
    await fireEvent.click(dialog.getByRole('button', { name: '另存常用條件' }));
    expect(loadAnalysisPresets()[0]!.query.monthMode).toBe('previous');
    await fireEvent.click(dialog.getByRole('button', { name: '套用條件' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    const previous = new Date(); previous.setDate(1); previous.setMonth(previous.getMonth() - 1);
    expect(screen.getByLabelText('月份')).toHaveValue(`${previous.getFullYear()}-${String(previous.getMonth() + 1).padStart(2, '0')}`);
    expect(run).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: '取消套用常用條件' }));
    expect(screen.queryByText(/已帶入/)).not.toBeInTheDocument();
  });

  it('pins a preset, stages it from the shortcut without querying, and can unpin it', async () => {
    putAnalysisPreset([], draft(), 'Pinned query');
    const { run, container } = await setup();
    const dialog = await open();
    await fireEvent.click(dialog.getByRole('button', { name: '釘選到主頁' }));
    expect(dialog.getByRole('button', { name: '取消釘選' })).toHaveAttribute('aria-pressed', 'true');
    await close();
    await fireEvent.click(screen.getByRole('button', { name: '帶入常用條件：Pinned query' }));
    await screen.findByText(/已帶入「Pinned query」/);
    expect(run).toHaveBeenCalledTimes(1);
    expect(container.querySelector('.report-context')).toHaveTextContent('Production');
    expect(loadAnalysisPresets()[0]!.lastUsedAt).toBeUndefined();
    await fireEvent.click(screen.getByRole('button', { name: '放棄條件變更' }));
    const again = await open();
    await fireEvent.click(again.getByRole('button', { name: '取消釘選' }));
    await close();
    expect(screen.queryByRole('navigation', { name: '常用查詢捷徑' })).not.toBeInTheDocument();
  });

  it('records recent use only after an explicit successful query', async () => {
    putAnalysisPreset([], draft(), 'Recent query');
    const { run } = await setup();
    await applySaved();
    expect(loadAnalysisPresets()[0]!.lastUsedAt).toBeUndefined();
    await fireEvent.click(screen.getByText('重新分析'));
    await waitFor(() => expect(loadAnalysisPresets()[0]!.lastUsedAt).toEqual(expect.any(Number)));
    expect(screen.getByRole('navigation', { name: '常用查詢捷徑' })).toHaveTextContent('最近使用');
    expect(screen.getByRole('button', { name: '帶入常用條件：Recent query' })).toBeInTheDocument();
    expect(run).toHaveBeenCalledTimes(2);
  });

  it('does not reuse a shortcut deleted in another window', async () => {
    let list = putAnalysisPreset([], draft(), 'Deleted shortcut');
    setAnalysisPresetPinned(list, list[0]!.id, true);
    const { run } = await setup();
    saveAnalysisPresets([]);
    await fireEvent.click(screen.getByRole('button', { name: '帶入常用條件：Deleted shortcut' }));
    await screen.findByRole('alert');
    expect(screen.queryByText(/已帶入「/)).not.toBeInTheDocument();
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('keeps the successful report if recent-use persistence fails', async () => {
    putAnalysisPreset([], draft(), 'Write failure');
    const { run } = await setup();
    await applySaved();
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota'); });
    await fireEvent.click(screen.getByText('重新分析'));
    await screen.findByText('查詢已完成，但最近使用紀錄儲存失敗；報表不受影響。');
    expect(screen.getByRole('heading', { name: '銷售額 Top 24' })).toBeInTheDocument();
    expect(loadAnalysisPresets()[0]!.lastUsedAt).toBeUndefined();
    expect(run).toHaveBeenCalledTimes(2);
  });

  it('retains staged filters through a failed analysis and applies them on retry', async () => {
    putAnalysisPreset([], draft(), 'Retry query');
    const { run } = await setup();
    await applySaved();
    run.mockRejectedValueOnce(new Error('offline'));
    await fireEvent.click(screen.getByText('重新分析'));
    await screen.findByRole('alert');
    expect(screen.getByText(/已帶入「Retry query」/)).toBeInTheDocument();
    expect(loadAnalysisPresets()[0]!.lastUsedAt).toBeUndefined();
    await fireEvent.click(screen.getByText('開始分析'));
    await waitFor(() => expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('Mask'));
    expect(run).toHaveBeenCalledTimes(3);
  });
});
