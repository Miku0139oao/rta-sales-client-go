import { afterEach, describe, expect, it } from 'vitest';
import { clearWebSnapshot, loadWebSnapshot, saveWebSnapshot, WEB_STORAGE_KEY } from './webStorage';

afterEach(() => {
  localStorage.clear();
});

describe('web localStorage snapshot', () => {
  it('starts empty when nothing is stored', () => {
    expect(loadWebSnapshot()).toEqual({
      profiles: [],
      secrets: {},
      manCodeGroups: [],
      analysis: null,
      articleNames: {},
    });
  });

  it('round-trips accounts, groups, and analysis across reload', () => {
    saveWebSnapshot({
      profiles: [{
        id: 'profile-1', displayName: '店長', enabled: true, priority: 1, hasCredentials: true, accountHint: 'sa••••01',
      }],
      secrets: { 'profile-1': { account: 'sa01', password: 'secret' } },
      manCodeGroups: [{ id: 'group-1', name: '保健', codes: ['123456'] }],
      analysis: {
        operationId: 'op-1', from: '2026-08-01', to: '2026-08-14', complete: true,
        selectedStores: 1, successfulStores: 1, queryDurationMs: 10,
        totals: { saleQuantity: 1, saleAmount: 1, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 1 },
        stores: [],
      },
      articleNames: { '123456': '維他命' },
    });

    localStorage.setItem(WEB_STORAGE_KEY, localStorage.getItem(WEB_STORAGE_KEY) ?? '');
    const reloaded = loadWebSnapshot();
    expect(reloaded.profiles[0]).toMatchObject({ displayName: '店長', hasCredentials: true });
    expect(reloaded.secrets['profile-1']).toEqual({ account: 'sa01', password: 'secret' });
    expect(reloaded.manCodeGroups).toEqual([{ id: 'group-1', name: '保健', codes: ['123456'] }]);
    expect(reloaded.analysis?.operationId).toBe('op-1');
    expect(reloaded.articleNames).toEqual({ '123456': '維他命' });
  });

  it('drops a malformed saved analysis instead of reopening a broken result screen', () => {
    localStorage.setItem(WEB_STORAGE_KEY, JSON.stringify({
      profiles: [],
      analysis: { operationId: 'truncated-result' },
    }));

    expect(loadWebSnapshot().analysis).toBeNull();
  });

  it('clears the stored snapshot', () => {
    saveWebSnapshot({
      profiles: [{ id: 'p', displayName: 'A', enabled: false, priority: 1, hasCredentials: false }],
      secrets: {},
      manCodeGroups: [],
      analysis: null,
      articleNames: {},
    });
    clearWebSnapshot();
    expect(localStorage.getItem(WEB_STORAGE_KEY)).toBeNull();
    expect(loadWebSnapshot().profiles).toEqual([]);
  });
});
