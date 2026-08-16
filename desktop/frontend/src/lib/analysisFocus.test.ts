import { describe, expect, it } from 'vitest';
import { buildFocusGroups, matchesFocusPrefix } from './analysisFocus';
import type { SalesAnalysisItem } from './types';

function item(overrides: Partial<SalesAnalysisItem>): SalesAnalysisItem {
  return {
    storeId: '107', storeLabel: '107',
    category1: 'HEALTH', category1Code: 'A',
    category2: 'HEALTH CARE', category2Code: 'A01',
    category3: 'OTC', category3Code: 'A0101',
    category4: 'PAIN', category4Code: 'A010101',
    category5: 'OIL', category5Code: 'A01010101',
    articleCode: '100', articleName: 'Item', brandName: 'Brand',
    transactionCount: 1, saleQuantity: 1, saleAmount: 10,
    returnQuantity: 0, returnTransactionCount: 0, returnAmount: 0,
    netQuantity: 1, netSalesAmount: 10,
    ...overrides,
  };
}

describe('upcoming focus groups', () => {
  it('ranks last-year-next-month products by sales and quantity within Health/Skin/PC', () => {
    const groups = buildFocusGroups([
      item({ articleCode: 'H1', articleName: '活絡油', category2Code: 'A01', netSalesAmount: 200, netQuantity: 4 }),
      item({ articleCode: 'H2', articleName: '必理痛', category2Code: 'A01', netSalesAmount: 80, netQuantity: 20 }),
      item({ articleCode: 'S1', articleName: '面膜', category2Code: 'A02', category3Code: 'A0201', netSalesAmount: 150, netQuantity: 8 }),
      item({ articleCode: 'P1', articleName: '洗髮露', category2Code: 'A03', category3Code: 'A0301', netSalesAmount: 90, netQuantity: 12 }),
      item({ articleCode: 'X1', articleName: '零食', category1Code: 'E', category2Code: 'E02', netSalesAmount: 999, netQuantity: 99 }),
    ], [
      item({ articleCode: 'H1', articleName: '活絡油', netSalesAmount: 30, netQuantity: 1 }),
    ]);

    expect(groups.map((group) => group.id)).toEqual(['health', 'skin', 'pc']);
    expect(groups[0]!.sales.map((product) => product.code)).toEqual(['H1', 'H2']);
    expect(groups[0]!.quantity.map((product) => product.code)).toEqual(['H2', 'H1']);
    expect(groups[0]!.sales[0]).toMatchObject({ name: '活絡油', currentAmount: 30, currentQuantity: 1 });
    expect(groups[1]!.sales[0]!.code).toBe('S1');
    expect(groups[2]!.sales[0]!.code).toBe('P1');
    expect(groups.some((group) => group.sales.some((product) => product.code === 'X1'))).toBe(false);
  });

  it('matches catalog article codes when any group has codes', () => {
    const groups = buildFocusGroups([
      item({ articleCode: 'H1', articleName: '活絡油', category2Code: 'A01', netSalesAmount: 200, netQuantity: 4 }),
      item({ articleCode: 'H2', articleName: '必理痛', category2Code: 'A01', netSalesAmount: 80, netQuantity: 20 }),
      item({ articleCode: 'S1', articleName: '面膜', category2Code: 'A02', netSalesAmount: 150, netQuantity: 8 }),
    ], [
      item({ articleCode: 'H1', articleName: '活絡油', netSalesAmount: 30, netQuantity: 1 }),
    ], 10, [
      { id: 'g-health', name: '保健', codes: ['H1'] },
      { id: 'g-empty', name: '空組', codes: [] },
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({ id: 'g-health', name: '保健', prefix: '' });
    expect(groups[0]!.sales.map((product) => product.code)).toEqual(['H1']);
    expect(groups[0]!.sales[0]).toMatchObject({ currentAmount: 30, currentQuantity: 1 });
    expect(groups.some((group) => group.id === 'health' || group.id === 'skin')).toBe(false);
  });

  it('falls back to A01/A02/A03 when the catalog is empty or every group has no codes', () => {
    const items = [
      item({ articleCode: 'H1', category2Code: 'A01', netSalesAmount: 200, netQuantity: 4 }),
      item({ articleCode: 'S1', category2Code: 'A02', netSalesAmount: 150, netQuantity: 8 }),
    ];
    expect(buildFocusGroups(items).map((group) => group.id)).toEqual(['health', 'skin']);
    expect(buildFocusGroups(items, [], 10, []).map((group) => group.id)).toEqual(['health', 'skin']);
    expect(buildFocusGroups(items, [], 10, [{ id: 'g-empty', name: '空組', codes: [] }]).map((group) => group.id)).toEqual(['health', 'skin']);
  });

  it('omits a catalog group when none of its codes appear in the rows', () => {
    const groups = buildFocusGroups([
      item({ articleCode: 'H1', category2Code: 'A01', netSalesAmount: 200 }),
    ], [], 10, [{ id: 'g-miss', name: '未命中', codes: ['NOPE'] }]);
    expect(groups).toEqual([]);
  });

  it('matches Excel-style department prefixes on any category code', () => {
    expect(matchesFocusPrefix(item({ category2Code: 'A0201', category3Code: '' }), 'A02')).toBe(true);
    expect(matchesFocusPrefix(item({ category2Code: 'B05' }), 'A01')).toBe(false);
  });
});
