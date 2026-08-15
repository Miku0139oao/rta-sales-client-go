import { describe, expect, it } from 'vitest';
import { defaultSettings, normalizeSettings } from './settings';

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
});
