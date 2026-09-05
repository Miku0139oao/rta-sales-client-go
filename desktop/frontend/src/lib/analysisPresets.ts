// Local-only query recipes. Store IDs and filters, never credentials or report rows.
export const ANALYSIS_PRESETS_KEY = 'rta-sales-analysis-presets-v1';
export const MAX_ANALYSIS_PRESETS = 20;
export const PRESET_NAME_MAX = 60;
export const MAX_PINNED_PRESETS = 3;
export const PRESET_CATEGORIES = ['category1', 'category2', 'category3', 'category4', 'category5'] as const;
export type PresetCategory = typeof PRESET_CATEGORIES[number];
export type PresetMonthMode = 'fixed' | 'current' | 'previous';
export interface PresetQuery {
  profileId: string;
  profileName: string;
  periodMode: 'month' | 'range';
  monthMode: PresetMonthMode;
  month: string;
  from: string;
  to: string;
  weekCompare: boolean;
  storeIds: string[];
}
export interface PresetFilters {
  search: string;
  groupId: string;
  groupLevel: PresetCategory;
  categories: Record<PresetCategory, string[]>;
}
export interface AnalysisPresetDraft { query: PresetQuery; filters: PresetFilters }
export interface AnalysisPreset extends AnalysisPresetDraft { id: string; name: string; pinned?: boolean; lastUsedAt?: number }
export class PresetError extends Error {
  constructor(public code: 'invalid' | 'name' | 'duplicate' | 'limit' | 'pinLimit') { super(code); }
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new PresetError('invalid');
  return value as Record<string, unknown>;
}
function text(value: unknown, max: number, required = false): string {
  if (typeof value !== 'string' || value.length > max || (required && !value.trim())) throw new PresetError('invalid');
  return value.trim();
}
function strings(value: unknown, max: number): string[] {
  if (!Array.isArray(value) || value.length > max) throw new PresetError('invalid');
  return [...new Set(value.map((item) => text(item, 256, true)))];
}
function validDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T12:00:00Z`);
  return Number.isFinite(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}
function nameKey(value: string): string { return value.trim().normalize('NFKC').toLowerCase(); }

export function normalizePresetDraft(value: unknown): AnalysisPresetDraft {
  const raw = record(value), query = record(raw.query), filters = record(raw.filters), categories = record(filters.categories);
  if (query.periodMode !== 'month' && query.periodMode !== 'range') throw new PresetError('invalid');
  if (!['fixed', 'current', 'previous'].includes(String(query.monthMode))) throw new PresetError('invalid');
  const month = text(query.month, 7), from = text(query.from, 10), to = text(query.to, 10);
  if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(month) || !validDate(from) || !validDate(to)) throw new PresetError('invalid');
  if (query.periodMode === 'range' && (from > to || query.monthMode !== 'fixed')) throw new PresetError('invalid');
  if (typeof query.weekCompare !== 'boolean' || !PRESET_CATEGORIES.includes(filters.groupLevel as PresetCategory)) throw new PresetError('invalid');
  const storeIds = strings(query.storeIds, 2000);
  if (!storeIds.length) throw new PresetError('invalid');
  return {
    query: {
      profileId: text(query.profileId, 256, true), profileName: text(query.profileName, 256),
      periodMode: query.periodMode, monthMode: query.monthMode as PresetMonthMode,
      month, from, to, weekCompare: query.weekCompare, storeIds,
    },
    filters: {
      search: text(filters.search, 1000), groupId: text(filters.groupId, 256),
      groupLevel: filters.groupLevel as PresetCategory,
      categories: Object.fromEntries(PRESET_CATEGORIES.map((key) => [key, strings(categories[key], 1000)])) as PresetFilters['categories'],
    },
  };
}
function normalizeList(value: unknown): AnalysisPreset[] {
  if (!Array.isArray(value)) throw new PresetError('invalid');
  if (value.length > MAX_ANALYSIS_PRESETS) throw new PresetError('limit');
  const ids = new Set<string>(), names = new Set<string>();
  let pinnedCount = 0;
  return value.map((entry) => {
    const raw = record(entry), id = text(raw.id, 80, true), name = text(raw.name, PRESET_NAME_MAX, true);
    if (ids.has(id) || names.has(nameKey(name))) throw new PresetError('duplicate');
    ids.add(id); names.add(nameKey(name));
    if (raw.pinned !== undefined && typeof raw.pinned !== 'boolean') throw new PresetError('invalid');
    if (raw.lastUsedAt !== undefined && (typeof raw.lastUsedAt !== 'number' || !Number.isSafeInteger(raw.lastUsedAt) || raw.lastUsedAt < 0 || raw.lastUsedAt > 8640000000000000)) throw new PresetError('invalid');
    if (raw.pinned && ++pinnedCount > MAX_PINNED_PRESETS) throw new PresetError('pinLimit');
    return { id, name, ...normalizePresetDraft(raw), ...(raw.pinned !== undefined ? { pinned: raw.pinned as boolean } : {}), ...(raw.lastUsedAt !== undefined ? { lastUsedAt: raw.lastUsedAt as number } : {}) };
  });
}
export function loadAnalysisPresets(): AnalysisPreset[] {
  const saved = localStorage.getItem(ANALYSIS_PRESETS_KEY);
  if (!saved) return [];
  const raw = record(JSON.parse(saved));
  if (raw.version !== 1) throw new PresetError('invalid');
  return normalizeList(raw.presets);
}
export function saveAnalysisPresets(presets: AnalysisPreset[]): AnalysisPreset[] {
  const normalized = normalizeList(presets);
  localStorage.setItem(ANALYSIS_PRESETS_KEY, JSON.stringify({ version: 1, presets: normalized }));
  return normalized;
}
export function putAnalysisPreset(presets: AnalysisPreset[], draft: AnalysisPresetDraft, name: string, id?: string): AnalysisPreset[] {
  name = name.trim();
  if (!name || name.length > PRESET_NAME_MAX) throw new PresetError('name');
  if (presets.some((preset) => preset.id !== id && nameKey(preset.name) === nameKey(name))) throw new PresetError('duplicate');
  if (id && !presets.some((preset) => preset.id === id)) throw new PresetError('invalid');
  if (!id && presets.length >= MAX_ANALYSIS_PRESETS) throw new PresetError('limit');
  const existing = id ? presets.find((preset) => preset.id === id) : undefined;
  const next = { ...existing, id: id ?? crypto.randomUUID(), name, ...normalizePresetDraft(draft) };
  return saveAnalysisPresets(id ? presets.map((preset) => preset.id === id ? next : preset) : [...presets, next]);
}

export function setAnalysisPresetPinned(presets: AnalysisPreset[], id: string, pinned: boolean): AnalysisPreset[] {
  if (!presets.some((preset) => preset.id === id)) throw new PresetError('invalid');
  return saveAnalysisPresets(presets.map((preset) => preset.id === id ? { ...preset, pinned } : preset));
}
export function markAnalysisPresetUsed(id: string, now = Date.now()): AnalysisPreset[] {
  const presets = loadAnalysisPresets();
  // A deleted preset must not be resurrected by a late successful query.
  if (!presets.some((preset) => preset.id === id)) return presets;
  return saveAnalysisPresets(presets.map((preset) => preset.id === id ? { ...preset, lastUsedAt: now } : preset));
}
export function analysisPresetShortcuts(presets: AnalysisPreset[]): { pinned: AnalysisPreset[]; recent: AnalysisPreset[] } {
  return { pinned: presets.filter((preset) => preset.pinned).slice(0, MAX_PINNED_PRESETS), recent: presets.filter((preset) => !preset.pinned && preset.lastUsedAt !== undefined).sort((a, b) => b.lastUsedAt! - a.lastUsedAt!).slice(0, 3) };
}

// Resolve on each application, in local calendar time (including January rollover).
export function resolvePresetQuery(query: PresetQuery, now = new Date()): PresetQuery {
  if (query.periodMode !== 'month' || query.monthMode === 'fixed') return { ...query, storeIds: [...query.storeIds] };
  const date = new Date(now.getFullYear(), now.getMonth() - (query.monthMode === 'previous' ? 1 : 0), 1);
  const month = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`;
  return { ...query, month, storeIds: [...query.storeIds] };
}
