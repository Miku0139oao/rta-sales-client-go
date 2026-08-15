import { describe, expect, it } from 'vitest';
import { defaultSettings, loadSettings, normalizeSettings } from './settings';

describe('settings normalization', () => {
  it('uses safe defaults', () => {
    expect(normalizeSettings({})).toEqual(defaultSettings);
  });

  it('clamps workload and concurrency limits', () => {
    expect(normalizeSettings({ maxJobs: 9000, accountConcurrency: 12 })).toMatchObject({
      maxJobs: 2000,
      accountConcurrency: 4,
    });
    expect(normalizeSettings({ maxJobs: 0, accountConcurrency: 0 })).toMatchObject({
      maxJobs: 1,
      accountConcurrency: 1,
    });
  });

  it('keeps only a local mapping path and enabled flag', () => {
    expect(normalizeSettings({ useLocalMapping: true, mappingPath: 'D:\\private\\map.json' })).toMatchObject({
      useLocalMapping: true,
      mappingPath: 'D:\\private\\map.json',
    });
  });

  it('upgrades saved settings without a theme to the system preference', () => {
    localStorage.setItem('rta-sales-desktop-settings-v1', JSON.stringify({ locale: 'en', maxJobs: 40 }));
    expect(loadSettings()).toMatchObject({ locale: 'en', theme: 'system', maxJobs: 40 });
    expect(normalizeSettings({ theme: 'dark' })).toMatchObject({ theme: 'dark' });
    expect(normalizeSettings({ theme: 'sepia' as never })).toMatchObject({ theme: 'system' });
  });
});
