import type { ManCodeGroup, Profile, SalesAnalysisResult } from './types';

export const WEB_STORAGE_KEY = 'rta-sales-web-v1';

export interface StoredProfileSecret {
  account: string;
  password: string;
}

export interface WebSnapshot {
  profiles: Profile[];
  secrets: Record<string, StoredProfileSecret>;
  manCodeGroups: ManCodeGroup[];
  analysis: SalesAnalysisResult | null;
  articleNames: Record<string, string>;
}

const emptySnapshot = (): WebSnapshot => ({
  profiles: [],
  secrets: {},
  manCodeGroups: [],
  analysis: null,
  articleNames: {},
});

function canUseStorage(): boolean {
  return typeof localStorage !== 'undefined';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function asBoolean(value: unknown): boolean {
  return value === true;
}

function asNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

function normalizeProfile(value: unknown): Profile | undefined {
  if (!isRecord(value) || !asString(value.id) || !asString(value.displayName)) return undefined;
  const lastTestStatus = value.lastTestStatus;
  return {
    id: asString(value.id),
    displayName: asString(value.displayName),
    enabled: asBoolean(value.enabled),
    priority: Math.max(1, asNumber(value.priority, 1)),
    hasCredentials: asBoolean(value.hasCredentials),
    accountHint: asString(value.accountHint) || undefined,
    lastTestStatus: lastTestStatus === 'success' || lastTestStatus === 'failed' || lastTestStatus === 'untested'
      ? lastTestStatus
      : 'untested',
    lastTestMessage: asString(value.lastTestMessage) || undefined,
  };
}

function normalizeGroup(value: unknown): ManCodeGroup | undefined {
  if (!isRecord(value) || !asString(value.id) || !asString(value.name)) return undefined;
  const codes = Array.isArray(value.codes)
    ? value.codes.filter((code): code is string => typeof code === 'string' && code.trim() !== '').map((code) => code.trim())
    : [];
  return { id: asString(value.id), name: asString(value.name), codes };
}

function normalizeSecrets(value: unknown): Record<string, StoredProfileSecret> {
  if (!isRecord(value)) return {};
  const secrets: Record<string, StoredProfileSecret> = {};
  for (const [id, secret] of Object.entries(value)) {
    if (!isRecord(secret)) continue;
    secrets[id] = { account: asString(secret.account), password: asString(secret.password) };
  }
  return secrets;
}

function normalizeArticleNames(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {};
  const names: Record<string, string> = {};
  for (const [code, name] of Object.entries(value)) {
    if (typeof name === 'string' && name.trim()) names[code] = name;
  }
  return names;
}

function normalizeAnalysis(value: unknown): SalesAnalysisResult | null {
  if (!isRecord(value)) return null;
  if (!asString(value.operationId) || !asString(value.from) || !asString(value.to)) return null;
  const totals = value.totals;
  if (typeof value.complete !== 'boolean' || !isRecord(totals) || !Array.isArray(value.stores)) return null;
  if (!isFiniteNumber(value.selectedStores) || !isFiniteNumber(value.successfulStores) || !isFiniteNumber(value.queryDurationMs)) return null;
  const requiredTotals = ['saleQuantity', 'saleAmount', 'returnQuantity', 'returnAmount', 'netQuantity', 'netSalesAmount'];
  if (!requiredTotals.every((key) => isFiniteNumber(totals[key]))) return null;
  if (value.periods !== undefined && !Array.isArray(value.periods)) return null;
  if (value.weeks !== undefined && !Array.isArray(value.weeks)) return null;
  return value as unknown as SalesAnalysisResult;
}

function normalizeSnapshot(value: unknown): WebSnapshot {
  const raw = isRecord(value) ? value : {};
  const profiles = Array.isArray(raw.profiles)
    ? raw.profiles.flatMap((profile) => {
      const next = normalizeProfile(profile);
      return next ? [next] : [];
    })
    : [];
  const manCodeGroups = Array.isArray(raw.manCodeGroups)
    ? raw.manCodeGroups.flatMap((group) => {
      const next = normalizeGroup(group);
      return next ? [next] : [];
    })
    : [];
  return {
    profiles,
    secrets: normalizeSecrets(raw.secrets),
    manCodeGroups,
    analysis: normalizeAnalysis(raw.analysis),
    articleNames: normalizeArticleNames(raw.articleNames),
  };
}

export function loadWebSnapshot(): WebSnapshot {
  if (!canUseStorage()) return emptySnapshot();
  try {
    const raw = localStorage.getItem(WEB_STORAGE_KEY);
    if (!raw) return emptySnapshot();
    return normalizeSnapshot(JSON.parse(raw) as unknown);
  } catch {
    return emptySnapshot();
  }
}

export function saveWebSnapshot(snapshot: WebSnapshot): WebSnapshot {
  const normalized = normalizeSnapshot(snapshot);
  if (canUseStorage()) localStorage.setItem(WEB_STORAGE_KEY, JSON.stringify(normalized));
  return normalized;
}

export function updateWebSnapshot(patch: Partial<WebSnapshot>): WebSnapshot {
  return saveWebSnapshot({ ...loadWebSnapshot(), ...patch });
}

export function loadWebAnalysisSnapshot(): SalesAnalysisResult | null {
  return loadWebSnapshot().analysis;
}

export function saveWebAnalysisSnapshot(analysis: SalesAnalysisResult | null): void {
  updateWebSnapshot({ analysis });
}

export function clearWebSnapshot(): void {
  if (canUseStorage()) localStorage.removeItem(WEB_STORAGE_KEY);
}
