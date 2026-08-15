import type { AppSettings } from './types';

const STORAGE_KEY = 'rta-sales-desktop-settings-v1';

export const defaultSettings: AppSettings = {
  locale: 'zh-TW',
  theme: 'system',
  maxJobs: 2000,
  accountConcurrency: 16,
  useLocalMapping: false,
  mappingPath: '',
};

function clampInteger(value: unknown, fallback: number, min: number, max: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.min(max, Math.max(min, Math.round(parsed))) : fallback;
}

export function normalizeSettings(value: Partial<AppSettings>): AppSettings {
  return {
    locale: value.locale === 'en' ? 'en' : 'zh-TW',
    theme: value.theme === 'light' || value.theme === 'dark' ? value.theme : 'system',
    maxJobs: clampInteger(value.maxJobs, defaultSettings.maxJobs, 1, 2000),
    accountConcurrency: clampInteger(value.accountConcurrency, defaultSettings.accountConcurrency, 1, 32),
    useLocalMapping: value.useLocalMapping === true,
    mappingPath: typeof value.mappingPath === 'string' ? value.mappingPath : '',
  };
}

export function loadSettings(): AppSettings {
  if (typeof localStorage === 'undefined') return { ...defaultSettings };
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved ? normalizeSettings(JSON.parse(saved) as Partial<AppSettings>) : { ...defaultSettings };
  } catch {
    return { ...defaultSettings };
  }
}

export function saveSettings(settings: AppSettings): AppSettings {
  const normalized = normalizeSettings(settings);
  if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, JSON.stringify(normalized));
  return normalized;
}
