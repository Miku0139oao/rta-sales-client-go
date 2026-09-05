import { describe, expect, it } from 'vitest';
import { dataFixture } from '../test/analysisDataFixture';
import { buildSalesInsights, salesInsightsTable } from './salesInsights';
import { translator } from './i18n';
const t = translator('zh-TW');
describe('local sales highlights', () => {
  it('ranks monetary impact, reports refunds, and keeps evidence with each highlight', () => {
    const data = dataFixture(), [current, previous] = data.periods!;
    current!.items![2]!.netSalesAmount = 20; current!.items![2]!.returnAmount = -12;
    const result = buildSalesInsights(current, previous);
    expect(result.reason).toBe('ready');
    expect(result.entries.map(entry => entry.kind)).toEqual(['decline', 'growth', 'returns']);
    expect(result.entries[0]).toMatchObject({ code: '00002', current: 20, previous: 40, difference: -20, percent: -.5 });
    expect(result.entries[1]).toMatchObject({ code: '00107', current: 60, previous: 30, difference: 30 });
    expect(result.entries[2]!.refunds).toBe(12);
  });
  it('uses all product rows, not ranking depth or the first page', () => {
    const [current, previous] = dataFixture().periods!;
    current!.items = Array.from({ length: 200000 }, (_, index) => ({ ...current!.items![0]!, articleCode: `code-${index % 1000}`, netSalesAmount: 1 }));
    current!.itemCount = current!.items.length;
    const result = buildSalesInsights(current, previous);
    expect(result.entries.find(entry => entry.kind === 'growth')).toMatchObject({ current: 200, previous: 0, percent: null });
    expect(result.entries.length).toBeLessThanOrEqual(3);
  });
  it('applies the same screen predicate to both periods without mutating them', () => {
    const [current, previous] = dataFixture().periods!, before = JSON.stringify([current, previous]);
    const result = buildSalesInsights(current, previous, item => item.articleCode === '00107');
    expect(result.entries.every(entry => entry.code === '00107')).toBe(true);
    expect(result.entries[0]).toMatchObject({ current: 60, previous: 30 });
    expect(JSON.stringify([current, previous])).toBe(before);
  });
  it('does not invent current highlights from missing or partial rows', () => {
    const [current, previous] = dataFixture().periods!;
    current!.items = [];
    expect(buildSalesInsights(current, previous)).toMatchObject({ entries: [], reason: 'currentMissing' });
    current!.items = dataFixture().periods![0]!.items; current!.complete = false;
    expect(buildSalesInsights(current, previous)).toMatchObject({ entries: [], reason: 'currentPartial' });
  });
  it('withholds growth and decline when previous details are missing or partial', () => {
    const [current, previous] = dataFixture().periods!;
    previous!.items = [];
    expect(buildSalesInsights(current, previous).reason).toBe('previousMissing');
    expect(buildSalesInsights(current, previous).entries.every(entry => entry.kind === 'leader' || entry.kind === 'returns')).toBe(true);
    previous!.items = dataFixture().periods![1]!.items; previous!.complete = false;
    expect(buildSalesInsights(current, previous).reason).toBe('previousPartial');
  });
  it('refuses comparisons with different store coverage', () => {
    const [current, previous] = dataFixture().periods!;
    previous!.stores = previous!.stores.filter(store => store.businessId !== '109'); previous!.successfulStores = 2;
    const result = buildSalesInsights(current, previous);
    expect(result.reason).toBe('storesDiffer'); expect(result.entries.some(entry => entry.kind === 'growth')).toBe(false);
  });
  it('treats a missing product as zero only in a fully loaded comparable period', () => {
    const [current, previous] = dataFixture().periods!;
    current!.items = current!.items!.filter(item => item.articleCode !== '00002'); current!.itemCount = 2;
    expect(buildSalesInsights(current, previous).entries.find(entry => entry.kind === 'decline')).toMatchObject({ code: '00002', current: 0, previous: 40, percent: -1 });
  });
  it('does not display infinity for a zero baseline or bad numeric input', () => {
    const [current, previous] = dataFixture().periods!;
    previous!.items!.forEach(item => { item.netSalesAmount = 0; });
    expect(buildSalesInsights(current, previous).entries[0]!.percent).toBeNull();
    current!.items![0]!.netSalesAmount = Number.NaN;
    expect(buildSalesInsights(current, previous)).toMatchObject({ entries: [], reason: 'invalidData' });
  });
  it('keeps known empty results empty and emits typed evidence for exports', () => {
    const [current, previous] = dataFixture().periods!;
    const result = buildSalesInsights(current, previous);
    const table = salesInsightsTable(result, t);
    expect(table.rows[0]!.cells.slice(1,7)).toEqual(['00002', 'Wipes', 80, 40, 40, 1]);
    current!.items = []; current!.itemCount = 0; previous!.items = []; previous!.itemCount = 0;
    expect(buildSalesInsights(current, previous)).toMatchObject({ entries: [], reason: 'ready' });
  });
});
