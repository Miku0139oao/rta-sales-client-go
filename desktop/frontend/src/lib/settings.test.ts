import { describe, expect, it } from 'vitest';
import { defaultSettings, loadSettings, normalizeSettings } from './settings';

describe('settings normalization', () => {
  it('uses safe defaults', () => {
    expect(normalizeSettings({})).toEqual(defaultSettings);
  });

  it('clamps workload and concurrency limits', () => {
    expect(normalizeSettings({ maxJobs: 9000, accountConcurrency: 200, simulateStoreCount: 99 })).toMatchObject({
      maxJobs: 2000,
      accountConcurrency: 160,
      simulateStoreCount: 16,
    });
    expect(normalizeSettings({ maxJobs: 0, accountConcurrency: 0, simulateStoreCount: -4 })).toMatchObject({
      maxJobs: 1,
      accountConcurrency: 8,
      simulateStoreCount: 0,
    });
    expect(normalizeSettings({ accountConcurrency: 4, simulateStoreCount: 32 })).toMatchObject({
      accountConcurrency: 8,
      simulateStoreCount: 16,
    });
  });

  it('keeps only a local mapping path and enabled flag', () => {
    expect(normalizeSettings({ useLocalMapping: true, mappingPath: 'D:\\private\\map.json' })).toMatchObject({
      useLocalMapping: true,
      mappingPath: 'D:\\private\\map.json',
    });
  });

  it('migrates the old 32-wide default to 160 without dropping locale or mapping', () => {
    localStorage.setItem(
      'rta-sales-desktop-settings-v1',
      JSON.stringify({ locale: 'en', accountConcurrency: 32, useLocalMapping: true, mappingPath: 'D:\\map.json' }),
    );
    expect(loadSettings()).toMatchObject({
      locale: 'en',
      accountConcurrency: 160,
      useLocalMapping: true,
      mappingPath: 'D:\\map.json',
    });
  });

  it('upgrades saved settings without a theme to the system preference', () => {
    localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'en', maxJobs: 40 }));
    expect(loadSettings()).toMatchObject({ locale: 'en', theme: 'system', maxJobs: 40 });
    expect(normalizeSettings({ theme: 'dark' })).toMatchObject({ theme: 'dark' });
    expect(normalizeSettings({ theme: 'sepia' as never })).toMatchObject({ theme: 'system' });
  });
});
