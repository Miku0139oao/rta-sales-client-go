import 'fake-indexeddb/auto';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { dataFixture } from '../test/analysisDataFixture';
import { packSalesAnalysisItems } from './salesAnalysisItems';
import {
  BROWSER_REPORT_KEY,
  browserReportStatus,
  clearBrowserReport,
  loadBrowserReport,
  saveBrowserReport,
  type BrowserReport,
} from './browserReportCache';

function report(): BrowserReport {
  const result = dataFixture();
  return {
    version: 1,
    savedAt: '2026-10-02T01:00:00Z',
    profileId: 'owner-2',
    profileName: 'Second account',
    result: { ...result, periods: result.periods!.map((period) => ({ ...period, items: undefined })) },
    packed: Object.fromEntries(
      result.periods!.map((period) => [period.key, packSalesAnalysisItems(period.key, period.items!, period.stores)]),
    ),
  };
}
beforeEach(async () => {
  await clearBrowserReport();
  localStorage.clear();
});
afterEach(async () => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  await clearBrowserReport();
  localStorage.clear();
});

it('restores every period and original owner from localStorage after a reload', async () => {
  const source = report();
  await saveBrowserReport(source);
  expect(JSON.parse(localStorage.getItem(BROWSER_REPORT_KEY)!)).toEqual(source);
  expect(await loadBrowserReport()).toEqual(source);
  expect(get(browserReportStatus)).toMatchObject({ savedAt: source.savedAt, storage: 'localStorage' });
});
it('uses IndexedDB for large reports and replaces the previous large report with one small latest report', async () => {
  const large = report();
  large.packed.current!.d!.push('x'.repeat(1_100_000));
  await saveBrowserReport(large);
  expect(JSON.parse(localStorage.getItem(BROWSER_REPORT_KEY)!)).toEqual({ storage: 'IndexedDB' });
  expect(await loadBrowserReport()).toEqual(large);
  const small = report();
  small.result.operationId = 'new-op';
  await saveBrowserReport(small);
  expect(await loadBrowserReport()).toEqual(small);
});
it('falls back when localStorage quota is exceeded without treating a successful query as a failure', async () => {
  const original = Storage.prototype.setItem;
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (this: Storage, key, value) {
    if (key === BROWSER_REPORT_KEY && value.length > 100) throw new DOMException('full', 'QuotaExceededError');
    return original.call(this, key, value);
  });
  await saveBrowserReport(report());
  expect(get(browserReportStatus).storage).toBe('IndexedDB');
  expect(await loadBrowserReport()).toEqual(report());
});
it('does not replace a readable report with pending or truncated period data', async () => {
  const source = report();
  await saveBrowserReport(source);
  const partial = report();
  partial.result.operationId = 'in-progress';
  partial.result.pending = true;
  await saveBrowserReport(partial);
  partial.result.pending = false;
  partial.result.periods![1]!.itemCount = 999;
  await saveBrowserReport(partial);
  expect(await loadBrowserReport()).toEqual(source);
});
it('clearing waits for queued writes and removes both local backends without deleting accounts', async () => {
  localStorage.setItem('account-setting', 'retained');
  const pending = saveBrowserReport(report());
  await clearBrowserReport();
  await pending;
  expect(await loadBrowserReport()).toBeNull();
  expect(localStorage.getItem('account-setting')).toBe('retained');
});
it('makes a storage failure visible while retaining the previous committed report', async () => {
  await saveBrowserReport(report());
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('unavailable');
  });
  vi.stubGlobal('indexedDB', undefined);
  const next = report();
  next.result.operationId = 'failed';
  await expect(saveBrowserReport(next)).rejects.toThrow();
  expect(get(browserReportStatus)).toMatchObject({ error: true, operationId: 'failed' });
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  expect((await loadBrowserReport())?.result.operationId).toBe('data-test');
});
