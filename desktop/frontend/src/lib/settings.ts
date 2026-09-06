import type { AppSettings } from './types';

const STORAGE_KEY = 'rta-sales-desktop-settings-v2';
const LEGACY_STORAGE_KEY = 'rta-sales-desktop-settings-v1';

export const CONCURRENCY_CHOICES = [8, 16, 32, 48, 64, 80, 128, 160] as const;
export const SIMULATE_STORE_CHOICES = [0, 16] as const;
export const RANKING_LIMIT_CHOICES = [16, 24, 32] as const;
export const RANKING_LIMIT_MIN = 5;
export const RANKING_LIMIT_MAX = 100;
export const DEFAULT_RANKING_LIMIT = 24;

export const defaultSettings: AppSettings = {
  autoCheckUpdates: true,
  locale: 'zh-TW',
  theme: 'system',
  maxJobs: 2000,
  accountConcurrency: 160,
  useLocalMapping: false,
  mappingPath: '',
  simulateStoreCount: 0,
  rankingLimit: 24,
};

function clampInteger(value: unknown, fallback: number, min: number, max: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.min(max, Math.max(min, Math.round(parsed))) : fallback;
}

function snapToChoices(value: number, choices: readonly number[]): number {
  return choices.reduce((best, choice) =>
    Math.abs(choice - value) < Math.abs(best - value) ? choice : best,
  );
}

export function normalizeRankingLimit(value: unknown): number {
  return clampInteger(value, DEFAULT_RANKING_LIMIT, RANKING_LIMIT_MIN, RANKING_LIMIT_MAX);
}

export function isPresetRankingLimit(value: number): boolean {
  return (RANKING_LIMIT_CHOICES as readonly number[]).includes(value);
}

export function normalizeSettings(value: Partial<AppSettings>): AppSettings {
  return {
    autoCheckUpdates: value.autoCheckUpdates !== false,
    locale: value.locale === 'en' ? 'en' : 'zh-TW',
    theme: value.theme === 'light' || value.theme === 'dark' ? value.theme : 'system',
    maxJobs: clampInteger(value.maxJobs, defaultSettings.maxJobs, 1, 2000),
    accountConcurrency: snapToChoices(
      clampInteger(value.accountConcurrency, defaultSettings.accountConcurrency, 1, 160),
      CONCURRENCY_CHOICES,
    ),
    useLocalMapping: value.useLocalMapping === true,
    mappingPath: typeof value.mappingPath === 'string' ? value.mappingPath : '',
    simulateStoreCount: snapToChoices(
      clampInteger(value.simulateStoreCount, defaultSettings.simulateStoreCount, 0, 32),
      SIMULATE_STORE_CHOICES,
    ),
    rankingLimit: normalizeRankingLimit(value.rankingLimit),
  };
}

export function loadSettings(): AppSettings {
  if (typeof localStorage === 'undefined') return { ...defaultSettings };
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) return normalizeSettings(JSON.parse(saved) as Partial<AppSettings>);
    const legacy = localStorage.getItem(LEGACY_STORAGE_KEY);
    if (legacy) {
      const migrated = normalizeSettings({
        ...(JSON.parse(legacy) as Partial<AppSettings>),
        accountConcurrency: defaultSettings.accountConcurrency,
      });
      localStorage.setItem(STORAGE_KEY, JSON.stringify(migrated));
      return migrated;
    }
    return { ...defaultSettings };
  } catch {
    return { ...defaultSettings };
  }
}

export function saveSettings(settings: AppSettings): AppSettings {
  const normalized = normalizeSettings(settings);
  if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, JSON.stringify(normalized));
  return normalized;
}
