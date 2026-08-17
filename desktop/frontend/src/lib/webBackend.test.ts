import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { backend, configureBackend } from './backend';
import { installWebBackend } from './webBackend';
import { clearWebSnapshot, loadWebSnapshot } from './webStorage';

beforeEach(() => {
  localStorage.clear();
  configureBackend(undefined);
  installWebBackend();
});

afterEach(() => {
  configureBackend(undefined);
  clearWebSnapshot();
});

describe('web localStorage backend', () => {
  it('persists created accounts and item-code groups', async () => {
    const profile = await backend.saveProfile({
      displayName: '店長', account: 'sa01', password: 'secret', enabled: false,
    });
    const group = await backend.saveManCodeGroup({ name: '保健', codes: ['123456'] });

    configureBackend(undefined);
    installWebBackend();

    await expect(backend.listProfiles()).resolves.toEqual([expect.objectContaining({
      id: profile.id, displayName: '店長', hasCredentials: true,
    })]);
    await expect(backend.listManCodeGroups()).resolves.toEqual([expect.objectContaining({
      id: group.id, name: '保健', codes: ['123456'],
    })]);
    expect(loadWebSnapshot().secrets[profile.id]).toEqual({ account: 'sa01', password: 'secret' });
  });

  it('sends live RTA calls to the web API', async () => {

    const profile = await backend.saveProfile({
      displayName: '店長', account: 'sa01', password: 'secret', enabled: true,
    });
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/api/session')) {
        return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { method?: string };
      if (url.includes('/api/rpc') && body.method === 'TestProfile') {
        return new Response(JSON.stringify({ result: { success: true, storeCount: 2, message: 'ok' } }), {
          status: 200, headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({ error: { code: 'backend_error', message: 'unexpected' } }), { status: 400 });
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(backend.testProfile(profile.id)).resolves.toMatchObject({ success: true, storeCount: 2 });
    expect(fetchMock).toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('keeps live analysis results after reload', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes('/api/session')) {
        return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response(JSON.stringify({
        result: {
          operationId: 'live-1', from: '2026-08-01', to: '2026-08-14', complete: true,
          selectedStores: 1, successfulStores: 1, queryDurationMs: 10,
          totals: { saleQuantity: 1, saleAmount: 10, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 10 },
          stores: [{ businessId: '107', label: '107', totals: { saleQuantity: 1, saleAmount: 10, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 10 } }],
          periods: [{
            key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-14', complete: true, successfulStores: 1,
            totals: { saleQuantity: 1, saleAmount: 10, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 10 },
            items: [{ storeId: '107', articleCode: '552646', articleName: 'Mask', netSalesAmount: 10, netQuantity: 1 }],
          }],
        },
      }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);
    const result = await backend.runSalesAnalysis({
      storeIds: ['107'],
      concurrency: 1,
      periods: [{ key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-14', includeTrend: false }],
    });
    expect(result.operationId).toBe('live-1');
    vi.unstubAllGlobals();

    configureBackend(undefined);
    installWebBackend();
    const names = await backend.getLatestArticleNames();
    expect(names['552646']).toBe('Mask');
    expect(loadWebSnapshot().analysis?.operationId).toBe('live-1');
  });

  it('writes PDF bytes through a browser download', async () => {
    const click = vi.fn();
    const createObjectURL = vi.fn(() => 'blob:preview');
    const revokeObjectURL = vi.fn();
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'a') return { click, href: '', download: '', remove() {} } as unknown as HTMLElement;
      return document.createElementNS('http://www.w3.org/1999/xhtml', tag);
    });
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });

    const path = await backend.writeSalesAnalysisPDF({
      directory: 'downloads',
      filename: 'report.pdf',
      dataBase64: btoa('%PDF-1.7'),
    });
    expect(path).toBe('report.pdf');
    expect(click).toHaveBeenCalledTimes(1);
  });
});
