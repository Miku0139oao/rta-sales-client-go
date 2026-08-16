import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { subsetReportFont } from './subsetReportFont';

describe('subsetReportFont', () => {
  it('keeps only the requested glyphs and stays far smaller than Noto Sans TC', async () => {
    const font = new Uint8Array(readFileSync(resolve(process.cwd(), 'src/lib/assets/NotoSansTC-Regular.ttf')));
    expect(font.byteLength).toBeGreaterThan(5_000_000);
    const subset = await subsetReportFont(font, 'RTA 銷售分析 107 HK$1,234.56');
    expect(subset.byteLength).toBeGreaterThan(1_000);
    expect(subset.byteLength).toBeLessThan(80_000);
    expect([...subset.slice(0, 4)]).toEqual([0, 1, 0, 0]);
  });
});
