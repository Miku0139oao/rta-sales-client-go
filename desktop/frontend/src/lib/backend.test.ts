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
});
