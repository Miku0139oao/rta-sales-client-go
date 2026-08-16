import { afterEach, describe, expect, it, vi } from 'vitest';
import { backend, configureBackend } from './backend';
import type { AnalysisProgress } from './types';

afterEach(() => configureBackend(undefined));

describe('Wails backend adapter', () => {
  it('maps the stable frontend profile methods to the Wails method names', async () => {
    const createOrUpdate = vi.fn(async (request) => ({
      id: 'p1', displayName: request.displayName, enabled: true, priority: 1, hasCredentials: true,
    }));
    const reorder = vi.fn(async () => []);
    const enable = vi.fn(async ({ profileId, enabled }) => ({
      id: profileId, displayName: 'Primary', enabled, priority: 1, hasCredentials: true,
    }));
    const openSavedFolder = vi.fn(async () => undefined);
    configureBackend({ methods: { CreateOrUpdateProfile: createOrUpdate, Reorder: reorder, Enable: enable, OpenSavedFolder: openSavedFolder } });

    await backend.saveProfile({ displayName: 'Primary', account: 'user', password: 'secret', enabled: true });
    await backend.reorderProfiles(['p1']);
    await backend.setProfileEnabled('p1', false);
    await backend.openSavedFolder('D:\\RTA Reports');
    expect(openSavedFolder).toHaveBeenCalledWith({ path: 'D:\\RTA Reports' });

    expect(createOrUpdate).toHaveBeenCalledWith(expect.objectContaining({ displayName: 'Primary' }));
    expect(reorder).toHaveBeenCalledWith({ profileIds: ['p1'] });
    expect(enable).toHaveBeenCalledWith({ profileId: 'p1', enabled: false });
  });

  it('maps ItemCode catalog methods to the Wails names', async () => {
    const list = vi.fn(async () => [{ id: 'g1', name: '保健', codes: ['1'] }]);
    const save = vi.fn(async (request) => ({ id: 'g2', name: request.name, codes: [] }));
    const replace = vi.fn(async (request) => ({ id: request.id, name: '保健', codes: request.codes }));
    const remove = vi.fn(async () => undefined);
    configureBackend({ methods: {
      ListManCodeGroups: list,
      SaveManCodeGroup: save,
      ReplaceManCodeGroupCodes: replace,
      DeleteManCodeGroup: remove,
    } });

    await expect(backend.listManCodeGroups()).resolves.toEqual([{ id: 'g1', name: '保健', codes: ['1'] }]);
    await backend.saveManCodeGroup({ name: '護膚' });
    await backend.replaceManCodeGroupCodes({ id: 'g1', codes: ['1', '2'] });
    await backend.deleteManCodeGroup('g1');

    expect(save).toHaveBeenCalledWith({ name: '護膚' });
    expect(replace).toHaveBeenCalledWith({ id: 'g1', codes: ['1', '2'] });
    expect(remove).toHaveBeenCalledWith('g1');
  });

  it('maps ItemCode catalog export and import to the Wails names', async () => {
    const exported = vi.fn(async () => ({ cancelled: false, path: 'D:\\item-codes.json', groups: [] }));
    const imported = vi.fn(async () => ({ cancelled: false, path: 'D:\\item-codes.json', groups: [{ id: 'g1', name: '保健', codes: ['1'] }] }));
    configureBackend({ methods: {
      ExportManCodeCatalog: exported,
      ImportManCodeCatalog: imported,
    } });

    await expect(backend.exportManCodeCatalog()).resolves.toEqual({ cancelled: false, path: 'D:\\item-codes.json', groups: [] });
    await expect(backend.importManCodeCatalog()).resolves.toEqual({
      cancelled: false, path: 'D:\\item-codes.json', groups: [{ id: 'g1', name: '保健', codes: ['1'] }],
    });
    expect(exported).toHaveBeenCalledTimes(1);
    expect(imported).toHaveBeenCalledTimes(1);
  });

  it('maps malformed catalog import errors to a dedicated code', async () => {
    for (const message of [
      'decode mancode catalog: unexpected end of JSON input',
      'mancode catalog groups must be an array',
      'mancode catalog contains trailing data',
    ]) {
      configureBackend({ methods: {
        ImportManCodeCatalog: vi.fn(async () => {
          throw new Error(message);
        }),
      } });

      await expect(backend.importManCodeCatalog()).rejects.toMatchObject({
        code: 'mancode_catalog_invalid',
      });
    }
  });

  it('reads latest article names from the in-memory analysis cache', async () => {
    const remote = vi.fn(async () => ({ '999': 'should not be used' }));
    configureBackend({ methods: {
      GetLatestArticleNames: remote,
      RunSalesAnalysis: vi.fn(async () => ({
        operationId: 'op1', from: '2026-08-01', to: '2026-08-14', complete: true,
        selectedStores: 1, successfulStores: 1, queryDurationMs: 1,
        totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
        stores: [],
        items: [{
          storeId: '107', storeLabel: '107', category1: '', category2: '', category3: '', category4: '', category5: '',
          articleCode: '552646', articleName: 'AHC 安瓶精華纖維面膜',
          transactionCount: 1, saleQuantity: 1, saleAmount: 1, returnQuantity: 0, returnTransactionCount: 0,
          returnAmount: 0, netQuantity: 1, netSalesAmount: 1,
        }],
      })),
    } });

    await backend.runSalesAnalysis({ storeIds: ['107'], concurrency: 1 });
    await expect(backend.getLatestArticleNames()).resolves.toEqual({ '552646': 'AHC 安瓶精華纖維面膜' });
    expect(remote).not.toHaveBeenCalled();
  });

  it('subscribes to the current and compatibility progress events', () => {
    const listeners = new Map<string, (payload: unknown) => void>();
    const cleanups: string[] = [];
    configureBackend({
      methods: {},
      events: {
        on(name, listener) {
          listeners.set(name, listener);
          return () => cleanups.push(name);
        },
      },
    });
    const received: AnalysisProgress[] = [];
    const unsubscribe = backend.onProgress((progress) => received.push(progress));
    listeners.get('rta:progress')?.({
      operationId: 'op1', stage: 'query', current: 2, total: 4,
      storeId: '107', date: '2026-08-07', profile: 'Production', attempt: 1, status: 'success',
    });

    expect(received).toHaveLength(1);
    expect(received[0]).toMatchObject({
      stage: 'query', current: 2, storeId: '107', date: '2026-08-07', profile: 'Production', status: 'success',
    });
    unsubscribe();
    expect(cleanups).toEqual(['rta:progress', 'analysis-progress']);
  });

  it('subscribes to Wails file drops using drop-target filtering', () => {
    const previousRuntime = window.runtime;
    const onFileDrop = vi.fn();
    const onFileDropOff = vi.fn();
    window.runtime = { ...previousRuntime, OnFileDrop: onFileDrop, OnFileDropOff: onFileDropOff };
    configureBackend({ methods: {} });
    const received: string[][] = [];

    try {
      const unsubscribe = backend.onFileDrop((paths) => received.push(paths));
      expect(onFileDrop).toHaveBeenCalledTimes(1);
      expect(onFileDrop.mock.calls[0]?.[1]).toBe(true);

      onFileDrop.mock.calls[0]?.[0](120, 240, ['D:\\dropped.xlsx']);
      expect(received).toEqual([['D:\\dropped.xlsx']]);

      unsubscribe();
      expect(onFileDropOff).toHaveBeenCalledTimes(1);
    } finally {
      window.runtime = previousRuntime;
    }
  });

  it('passes the bounded workload and local mapping path to Analyze', async () => {
    const analyze = vi.fn(async () => ({
      operationId: 'op1', complete: true, changedCellCount: 0, problemCount: 0,
      preview: [], totalCount: 0, changeCount: 0, unchangedCount: 0,
      issueCount: 0, failedCount: 0, overlapCount: 0, issues: [], canApply: false,
    }));
    configureBackend({ methods: { Analyze: analyze } });

    await backend.analyze({
      inputPath: 'D:\\workbook.xlsx', sheetName: 'August', from: '2026-08-01', to: '2026-08-14',
      maxJobs: 2000, accountConcurrency: 2, overwrite: false, useLocalMapping: true,
      mappingPath: 'D:\\map.json',
    });

    expect(analyze).toHaveBeenCalledWith(expect.objectContaining({ maxJobs: 2000, mappingPath: 'D:\\map.json' }));
  });

  it('falls back to the mock backend in DEV when a bound method throws', async () => {
    configureBackend({
      fallbackOnUnavailable: true,
      methods: {
        ListProfiles: vi.fn(async () => {
          throw new Error('Call.ByName failed');
        }),
      },
    });

    const profiles = await backend.listProfiles();
    expect(profiles.length).toBeGreaterThan(0);
    expect(profiles[0]).toMatchObject({ displayName: '主要帳號' });
  });

  it('does not hide Wails application errors behind the DEV mock', async () => {
    const applicationError = new Error('mancode catalog groups must be an array');
    applicationError.name = 'RuntimeError';
    configureBackend({
      fallbackOnUnavailable: true,
      methods: {
        ImportManCodeCatalog: vi.fn(async () => {
          throw applicationError;
        }),
      },
    });

    await expect(backend.importManCodeCatalog()).rejects.toMatchObject({
      code: 'mancode_catalog_invalid',
    });
  });

  it('does not turn ordinary DEV bridge failures into mock success', async () => {
    configureBackend({
      fallbackOnUnavailable: true,
      methods: {
        ListProfiles: vi.fn(async () => {
          throw new TypeError('network connection reset');
        }),
      },
    });

    await expect(backend.listProfiles()).rejects.toMatchObject({
      code: 'backend_error',
      message: 'network connection reset',
    });
  });
});
