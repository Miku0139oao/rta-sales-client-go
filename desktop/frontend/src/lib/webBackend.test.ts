import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { backend, configureBackend, callBackend } from './backend';
import { installWebBackend } from './webBackend';
import { clearWebSnapshot, loadWebSnapshot } from './webStorage';
import 'fake-indexeddb/auto';
import { clearBrowserReport, saveBrowserReport } from './browserReportCache';
import { packSalesAnalysisItems, unpackSalesAnalysisItems } from './salesAnalysisItems';
import { dataFixture } from '../test/analysisDataFixture';

beforeEach(async () => {
  await clearBrowserReport();
  localStorage.clear();
  configureBackend(undefined);
  installWebBackend();
});

afterEach(async () => {
  await clearBrowserReport();
  configureBackend(undefined);
  clearWebSnapshot();
});

describe('web localStorage backend', () => {
  it('reloads changed amounts after retry even when row counts match the saved report', async () => {
    const original = dataFixture();
    await saveBrowserReport({
      version: 1,
      savedAt: '2026-10-02T01:00:00Z',
      profileId: 'owner',
      profileName: 'Owner',
      localQuery: { periodMode: 'range', month: '2026-08', weekCompare: true },
      result: { ...original, periods: original.periods!.map((period) => ({ ...period, items: undefined })) },
      packed: Object.fromEntries(
        original.periods!.map((period) => [
          period.key,
          packSalesAnalysisItems(period.key, period.items!, period.stores),
        ]),
      ),
    });
    installWebBackend();
    const updated = structuredClone(original);
    updated.periods![0]!.items![0]!.netSalesAmount = 400;
    const rpc = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      if (String(input).includes('/api/session')) return new Response(JSON.stringify({ ok: true }));
      const request = JSON.parse(String(init?.body));
      const result =
        request.method === 'RetrySalesAnalysis'
          ? { ...updated, periods: updated.periods!.map((period) => ({ ...period, items: undefined })) }
          : packSalesAnalysisItems('current', updated.periods![0]!.items!, updated.stores);
      return new Response(JSON.stringify({ result }));
    });
    vi.stubGlobal('fetch', rpc);
    try {
      expect((await backend.loadSalesAnalysisSnapshot()).localQuery?.weekCompare).toBe(true);
      await callBackend('RetrySalesAnalysis', [{ operationId: original.operationId }]);
      const details = await backend.getSalesAnalysisItems({ operationId: original.operationId, periodKey: 'current' });
      expect(unpackSalesAnalysisItems(details, original.stores)[0]!.netSalesAmount).toBe(400);
      expect(rpc.mock.calls.some(([, init]) => String(init?.body).includes('GetSalesAnalysisItems'))).toBe(true);
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it('starts events only after the session cookie is established and reuses one bootstrap', async () => {
    let finish!: () => void;
    const gate = new Promise<void>((resolve) => {
      finish = resolve;
    });
    const events = vi.fn();
    vi.stubGlobal(
      'EventSource',
      class {
        constructor() {
          events();
        }
        addEventListener() {}
        removeEventListener() {}
        close() {}
      },
    );
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo) => {
        if (String(input).includes('/api/session')) {
          await gate;
          return new Response(JSON.stringify({ ok: true }));
        }
        return new Response(JSON.stringify({ result: [] }));
      }),
    );
    try {
      const stores = backend.listSalesAnalysisStores('profile');
      expect(events).not.toHaveBeenCalled();
      finish();
      await stores;
      expect(events).toHaveBeenCalledTimes(1);
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it('reopens local details, glyphs and PDF memo without a live server session and preserves store scope', async () => {
    const result = dataFixture();
    await saveBrowserReport({
      version: 1,
      savedAt: '2026-10-02T01:00:00Z',
      profileId: 'original-owner',
      profileName: 'Original',
      result: { ...result, periods: result.periods!.map((period) => ({ ...period, items: undefined })) },
      packed: Object.fromEntries(
        result.periods!.map((period) => [period.key, packSalesAnalysisItems(period.key, period.items!, period.stores)]),
      ),
    });
    installWebBackend();
    const fetchMock = vi.fn(async () => {
      throw new Error('session expired');
    });
    vi.stubGlobal('fetch', fetchMock);
    try {
      const saved = await backend.loadSalesAnalysisSnapshot();
      expect(saved.profileId).toBe('original-owner');
      const packed = await backend.getSalesAnalysisItems({ operationId: result.operationId, periodKey: 'previous' });
      expect(packed.r ?? packed.rows).toHaveLength(3);
      const one = await backend.getSalesAnalysisItems({
        operationId: result.operationId,
        periodKey: 'current',
        storeId: '108',
      });
      expect(one.r ?? one.rows).toHaveLength(1);
      expect(await backend.getSalesAnalysisReportGlyphs(result.operationId)).toContain('Mask');
      const memo = await backend.getSalesAnalysisReportMemo({
        operationId: result.operationId,
        storeId: '108',
        categoryLevel: 'category4',
        uncategorized: 'Other',
        excludeZeroGifts: false,
        excludeStamps: false,
        mode: 'blacklist',
      });
      expect(memo.periods[0]!.totals?.netSalesAmount).toBe(20);
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it('shares session synchronization and resynchronizes changed credentials', async () => {
    const profile = await backend.saveProfile({
      displayName: 'Store',
      account: 'sa01',
      password: 'secret',
      enabled: true,
    });
    const fetchMock = vi.fn(
      async (input: RequestInfo) =>
        new Response(JSON.stringify(String(input).includes('/api/session') ? { ok: true } : { result: [] }), {
          status: 200,
        }),
    );
    vi.stubGlobal('fetch', fetchMock);
    try {
      await Promise.all([backend.listSalesAnalysisStores(profile.id), backend.listSalesAnalysisStores(profile.id)]);
      expect(fetchMock.mock.calls.filter(([url]) => String(url).includes('/api/session'))).toHaveLength(1);
      await backend.saveProfile({
        id: profile.id,
        displayName: 'Store',
        account: 'sa01',
        password: 'changed',
        enabled: true,
      });
      await backend.listSalesAnalysisStores(profile.id);
      expect(fetchMock.mock.calls.filter(([url]) => String(url).includes('/api/session'))).toHaveLength(2);
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it('persists created accounts and item-code groups', async () => {
    const profile = await backend.saveProfile({
      displayName: '店長',
      account: 'sa01',
      password: 'secret',
      enabled: false,
    });
    const group = await backend.saveManCodeGroup({ name: '保健', codes: ['123456'] });

    configureBackend(undefined);
    installWebBackend();

    await expect(backend.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        id: profile.id,
        displayName: '店長',
        hasCredentials: true,
      }),
    ]);
    await expect(backend.listManCodeGroups()).resolves.toEqual([
      expect.objectContaining({
        id: group.id,
        name: '保健',
        codes: ['123456'],
      }),
    ]);
    expect(loadWebSnapshot().secrets[profile.id]).toEqual({ account: 'sa01', password: 'secret' });
  });

  it('sends live RTA calls to the web API', async () => {
    const profile = await backend.saveProfile({
      displayName: '店長',
      account: 'sa01',
      password: 'secret',
      enabled: true,
    });
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/api/session')) {
        return new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      const body = JSON.parse(String(init?.body ?? '{}')) as { method?: string };
      if (url.includes('/api/rpc') && body.method === 'TestProfile') {
        return new Response(JSON.stringify({ result: { success: true, storeCount: 2, message: 'ok' } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
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
        return new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(
        JSON.stringify({
          result: {
            operationId: 'live-1',
            from: '2026-08-01',
            to: '2026-08-14',
            complete: true,
            selectedStores: 1,
            successfulStores: 1,
            queryDurationMs: 10,
            totals: {
              saleQuantity: 1,
              saleAmount: 10,
              returnQuantity: 0,
              returnAmount: 0,
              netQuantity: 1,
              netSalesAmount: 10,
            },
            stores: [
              {
                businessId: '107',
                label: '107',
                totals: {
                  saleQuantity: 1,
                  saleAmount: 10,
                  returnQuantity: 0,
                  returnAmount: 0,
                  netQuantity: 1,
                  netSalesAmount: 10,
                },
              },
            ],
            periods: [
              {
                key: 'current',
                label: '本期',
                from: '2026-08-01',
                to: '2026-08-14',
                complete: true,
                successfulStores: 1,
                totals: {
                  saleQuantity: 1,
                  saleAmount: 10,
                  returnQuantity: 0,
                  returnAmount: 0,
                  netQuantity: 1,
                  netSalesAmount: 10,
                },
                items: [
                  { storeId: '107', articleCode: '552646', articleName: 'Mask', netSalesAmount: 10, netQuantity: 1 },
                ],
              },
            ],
          },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
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
