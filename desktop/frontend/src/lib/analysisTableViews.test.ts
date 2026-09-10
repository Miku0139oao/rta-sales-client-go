import { describe, expect, it } from 'vitest';
import { translator } from './i18n';
import { categoryShare, buildAnalysisTables } from './analysisTableViews';
import type { SalesAnalysisPeriodResult } from './types';

const loaded = (key: string): SalesAnalysisPeriodResult => ({
  key, label: key, from: '2026-08-01', to: '2026-08-31', complete: true, successfulStores: 1, itemCount: 0, items: [],
  totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
  stores: [],
});

describe('categoryShare', () => {
  it('divides a category by the same-period total', () => {
    expect(categoryShare(80, 100)).toBe(0.8);
    expect(categoryShare(0, 100)).toBe(0);
  });
  it('is empty when the total is missing or zero', () => {
    expect(categoryShare(80, 0)).toBeUndefined();
    expect(categoryShare(80, undefined)).toBeUndefined();
    expect(categoryShare(undefined, 100)).toBeUndefined();
  });
  it('keeps the signed total for negative net sales', () => {
    expect(categoryShare(-20, 80)).toBe(-0.25);
    expect(categoryShare(40, -80)).toBe(-0.5);
  });
});

describe('category comparison shares', () => {
  it('inserts unsigned share columns beside each period amount', () => {
    const tables = buildAnalysisTables({
      items: [],
      performance: [],
      categories: [
        { name: '保健', code: 'A01', current: 80, previous: 50, previous2: 40, yearAgo: 20 },
        { name: '美容', code: 'A02', current: 20, previous: 50, previous2: 60, yearAgo: 80 },
      ],
      stores: [],
      periods: [loaded('current'), loaded('previous'), loaded('previous2'), loaded('yearAgo')],
      weekAligned: false,
      topSales: [], topQuantity: [], salesGroups: [], quantityGroups: [], focus: [],
    }, translator('zh-TW'), 'zh-TW', {});
    const table = tables.categories.find((entry) => entry.id === 'categories');
    expect(table?.columns.map((column) => [column.label, column.format])).toEqual([
      ['分類', 'text'],
      ['本期', 'money'], ['本期佔比', 'share'],
      ['上期', 'money'], ['上期佔比', 'share'],
      ['前期', 'money'], ['前期佔比', 'share'],
      ['去年同期', 'money'], ['去年同期佔比', 'share'],
      ['較上期', 'percent'], ['較去年同期', 'percent'],
    ]);
    expect(table?.rows[0]?.cells).toEqual(['保健', 80, 0.8, 50, 0.5, 40, 0.4, 20, 0.2, 0.6, 3]);
    expect(table?.rows[1]?.cells).toEqual(['美容', 20, 0.2, 50, 0.5, 60, 0.6, 80, 0.8, -0.6, -0.75]);
  });
  it('leaves share blank when a period has not loaded', () => {
    const tables = buildAnalysisTables({
      items: [],
      performance: [],
      categories: [{ name: '保健', code: 'A01', current: 80, previous: 50, previous2: 0, yearAgo: 0 }],
      stores: [],
      periods: [loaded('current')],
      weekAligned: false,
      topSales: [], topQuantity: [], salesGroups: [], quantityGroups: [], focus: [],
    }, translator('zh-TW'), 'zh-TW', {});
    const row = tables.categories.find((entry) => entry.id === 'categories')?.rows[0]?.cells;
    expect(row?.[1]).toBe(80);
    expect(row?.[2]).toBe(1);
    expect(row?.[3]).toBeNull();
    expect(row?.[4]).toBeNull();
  });
});
