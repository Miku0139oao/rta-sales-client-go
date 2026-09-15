import { describe, expect, it } from 'vitest';
import { itemMatchesSearch, itemSearchHaystack } from './analysisSearch';
import type { SalesAnalysisItem } from './types';

function item(overrides: Partial<SalesAnalysisItem> = {}): SalesAnalysisItem {
  return {
    storeId: '107',
    storeLabel: '107 - Central',
    category1: 'HEALTH',
    category1Code: 'A',
    category2: 'BEAUTY CARE',
    category2Code: 'A02',
    category3: 'SKIN CARE',
    category3Code: 'A0201',
    category4: 'FACIAL',
    category4Code: 'A020101',
    category5: 'MASQUE',
    category5Code: 'A02010101',
    articleCode: '552646',
    articleName: 'AHC Mask',
    brandName: 'AHC',
    transactionCount: 1,
    saleQuantity: 1,
    saleAmount: 100,
    returnQuantity: 0,
    returnTransactionCount: 0,
    returnAmount: 0,
    netQuantity: 1,
    netSalesAmount: 100,
    ...overrides,
  };
}

describe('analysis search haystack', () => {
  it('matches code, name, brand, store, and category without rebuilding arrays', () => {
    const mask = item();
    expect(itemMatchesSearch(mask, '')).toBe(true);
    expect(itemMatchesSearch(mask, '  ')).toBe(true);
    expect(itemMatchesSearch(mask, 'ahc')).toBe(true);
    expect(itemMatchesSearch(mask, '552646')).toBe(true);
    expect(itemMatchesSearch(mask, 'CENTRAL')).toBe(true);
    expect(itemMatchesSearch(mask, 'a0201')).toBe(true);
    expect(itemMatchesSearch(mask, 'wipes')).toBe(false);
  });

  it('does not match a term split across adjacent fields', () => {
    const glued = item({ category1Code: 'A01', category2Code: '01' });
    expect(itemMatchesSearch(glued, 'a0101')).toBe(false);
    expect(itemSearchHaystack(glued)).toBe(itemSearchHaystack(glued));
  });
});
