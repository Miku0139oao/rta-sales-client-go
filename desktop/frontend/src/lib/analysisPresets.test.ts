import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { ANALYSIS_PRESETS_KEY, MAX_ANALYSIS_PRESETS, loadAnalysisPresets, normalizePresetDraft, putAnalysisPreset, resolvePresetQuery, saveAnalysisPresets, type AnalysisPresetDraft } from './analysisPresets';

export function presetDraft(): AnalysisPresetDraft {
  return {
    query: { profileId: 'profile-1', profileName: 'Production', periodMode: 'month', monthMode: 'fixed', month: '2026-08', from: '2026-08-01', to: '2026-08-31', weekCompare: false, storeIds: ['107'] },
    filters: { search: 'Mask', groupId: '', groupLevel: 'category2', categories: { category1: [], category2: ['BEAUTY'], category3: [], category4: [], category5: [] } },
  };
}
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe('local analysis presets', () => {
  it('round trips a named query and filters without sharing mutable objects', () => {
    const draft = presetDraft();
    const saved = putAnalysisPreset([], draft, '  每月美容  ');
    draft.query.storeIds.push('108');
    expect(loadAnalysisPresets()).toEqual(saved);
    expect(saved[0]!.name).toBe('每月美容');
    expect(saved[0]!.query.storeIds).toEqual(['107']);
  });
  it('updates and renames by ID, then deletes without changing unrelated entries', () => {
    let saved = putAnalysisPreset([], presetDraft(), 'A');
    saved = putAnalysisPreset(saved, presetDraft(), 'B');
    const second = saved[1];
    saved = putAnalysisPreset(saved, { ...presetDraft(), filters: { ...presetDraft().filters, search: 'Wipes' } }, 'New A', saved[0]!.id);
    expect(saved[1]).toEqual(second);
    expect(saved[0]!.filters.search).toBe('Wipes');
    saveAnalysisPresets(saved.filter((preset) => preset.id !== second!.id));
    expect(loadAnalysisPresets().map((preset) => preset.name)).toEqual(['New A']);
  });
  it('rejects empty names, oversized names, and equivalent names without overwriting', () => {
    const saved = putAnalysisPreset([], presetDraft(), 'Monthly');
    for (const name of ['', ' ', 'a'.repeat(61), ' monthly ', 'Ｍｏｎｔｈｌｙ']) {
      expect(() => putAnalysisPreset(saved, presetDraft(), name)).toThrow();
    }
    expect(loadAnalysisPresets()).toEqual(saved);
  });
  it('limits list growth but permits updates at capacity', () => {
    const list = Array.from({ length: MAX_ANALYSIS_PRESETS }, (_, index) => ({ ...presetDraft(), id: String(index), name: String(index) }));
    expect(() => putAnalysisPreset(list, presetDraft(), 'Extra')).toThrow('limit');
    expect(putAnalysisPreset(list, presetDraft(), 'Updated', '0')).toHaveLength(MAX_ANALYSIS_PRESETS);
  });
  it('does not persist extra credentials or report content', () => {
    const draft = { ...presetDraft(), password: 'must-not-save', items: [1], query: { ...presetDraft().query, password: 'must-not-save', stores: [{ totals: 42 }] } };
    putAnalysisPreset([], draft, 'Safe');
    expect(localStorage.getItem(ANALYSIS_PRESETS_KEY)).not.toMatch(/must-not-save|password|totals|items/);
  });
  it.each(['{', 'null', '{"version":2,"presets":[]}', '{"version":1,"presets":[{}]}'])('preserves unreadable storage instead of silently clearing it: %s', (value) => {
    localStorage.setItem(ANALYSIS_PRESETS_KEY, value);
    expect(() => loadAnalysisPresets()).toThrow();
    expect(localStorage.getItem(ANALYSIS_PRESETS_KEY)).toBe(value);
  });
  it('surfaces quota errors and leaves existing storage intact', () => {
    const saved = putAnalysisPreset([], presetDraft(), 'Keep');
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('QuotaExceeded'); });
    expect(() => putAnalysisPreset(saved, presetDraft(), 'Next')).toThrow('QuotaExceeded');
    expect(loadAnalysisPresets()).toEqual(saved);
  });
  it('rejects impossible dates and empty store selections', () => {
    const draft = presetDraft();
    expect(() => normalizePresetDraft({ ...draft, query: { ...draft.query, from: '2026-02-30' } })).toThrow();
    expect(() => normalizePresetDraft({ ...draft, query: { ...draft.query, periodMode: 'range', from: '2026-09-01' } })).toThrow();
    expect(() => normalizePresetDraft({ ...draft, query: { ...draft.query, storeIds: [] } })).toThrow();
  });
  it('deduplicates store IDs and category values', () => {
    const draft = presetDraft();
    draft.query.storeIds = ['107', '107'];
    draft.filters.categories.category2 = ['BEAUTY', 'BEAUTY'];
    expect(normalizePresetDraft(draft).query.storeIds).toEqual(['107']);
    expect(normalizePresetDraft(draft).filters.categories.category2).toEqual(['BEAUTY']);
  });
  it('resolves current and previous month at application time across year boundaries', () => {
    const now = new Date(2027, 0, 31, 23, 59);
    const query = presetDraft().query;
    expect(resolvePresetQuery({ ...query, monthMode: 'current' }, now).month).toBe('2027-01');
    expect(resolvePresetQuery({ ...query, monthMode: 'previous' }, now).month).toBe('2026-12');
    expect(resolvePresetQuery(query, now).month).toBe('2026-08');
  });
});
