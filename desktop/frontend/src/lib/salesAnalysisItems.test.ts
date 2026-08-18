import { describe, expect, it } from 'vitest';
import {
  categoryLabelOf,
  itemMatchesCategorySelection,
  needsSalesAnalysisItemHydration,
  packSalesAnalysisItems,
  periodKeysForView,
  periodNeedsItemHydration,
  unpackSalesAnalysisItems,
} from './salesAnalysisItems';

describe('sales analysis item batches', () => {
  it('hydrates only when the summary omitted item rows', () => {
    expect(needsSalesAnalysisItemHydration({
      operationId: 'op', from: '', to: '', complete: true, selectedStores: 1, successfulStores: 1,
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
      stores: [], queryDurationMs: 0,
      periods: [{ key: 'current', label: 'Current', from: '', to: '', complete: true, successfulStores: 1, totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 }, stores: [], items: [], itemCount: 2 }],
    })).toBe(true);
    expect(needsSalesAnalysisItemHydration({
      operationId: 'op', from: '', to: '', complete: true, selectedStores: 1, successfulStores: 1,
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
      stores: [], items: [], queryDurationMs: 0,
    })).toBe(false);
  });

  it('keeps rows when the period store list is missing', () => {
    const items = unpackSalesAnalysisItems({
      periodKey: 'current',
      dict: ['', 'A1', 'Mask'],
      rows: [{ s: 0, ac: 1, an: 2, ns: 10 }],
    });
    expect(items).toEqual([expect.objectContaining({
      storeId: '', storeLabel: '', articleCode: 'A1', articleName: 'Mask', netSalesAmount: 10,
    })]);
  });

  it('packs item rows so unpack restores codes and store labels', () => {
    const stores = [{
      businessId: '107', label: '107 - Central',
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 10 },
    }];
    const items = [{
      storeId: '107', storeLabel: '107 - Central', category1: 'HEALTH', category1Code: 'A',
      category2: 'BEAUTY', category2Code: 'A02', category3: '', category4: '', category5: '',
      articleCode: '552646', articleName: 'Mask', brandName: 'AHC',
      transactionCount: 1, saleQuantity: 2, saleAmount: 10, returnQuantity: 0, returnTransactionCount: 0,
      returnAmount: 0, netQuantity: 2, netSalesAmount: 10,
    }];
    const packed = packSalesAnalysisItems('current', items, stores);
    expect(packed.r?.[0]).toBeDefined();
    expect(JSON.stringify(packed)).not.toContain('"ac":');
    expect(unpackSalesAnalysisItems(packed, stores)).toEqual([expect.objectContaining({
      storeId: '107', articleCode: '552646', articleName: 'Mask', netSalesAmount: 10,
    })]);
  });

  it('restores store labels from the period store list', () => {
    const items = unpackSalesAnalysisItems({
      periodKey: 'current',
      dict: ['', 'A1', 'Mask', 'HEALTH', 'A'],
      rows: [{ s: 0, ac: 1, an: 2, c1: 3, k1: 4, ns: 10 }],
    }, [{ businessId: '107', label: '107 - Central', totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 10 } }]);
    expect(items).toEqual([expect.objectContaining({
      storeId: '107', storeLabel: '107 - Central', articleCode: 'A1', articleName: 'Mask',
      category1: 'HEALTH', category1Code: 'A', netSalesAmount: 10,
    })]);
  });

  it('hydrates a period after export strips rows without an itemCount', () => {
    expect(periodNeedsItemHydration({
      key: 'current', label: 'Current', from: '', to: '', complete: true, successfulStores: 1,
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
      stores: [],
    })).toBe(true);
  });

  it('hydrates a period when the summary sent an empty items array', () => {
    expect(periodNeedsItemHydration({
      key: 'current', label: 'Current', from: '', to: '', complete: true, successfulStores: 1,
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
      stores: [], items: [], itemCount: 12,
    })).toBe(true);
    expect(periodNeedsItemHydration({
      key: 'current', label: 'Current', from: '', to: '', complete: true, successfulStores: 1,
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
      stores: [], items: [{
        storeId: '107', storeLabel: '107', category1: '', category2: 'BEAUTY CARE', category3: '', category4: '', category5: '',
        articleCode: '1', articleName: 'Mask', transactionCount: 1, saleQuantity: 1, saleAmount: 1,
        returnQuantity: 0, returnTransactionCount: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 1,
      }], itemCount: 1,
    })).toBe(false);
  });

  it('labels 商品部門 with code and name and matches either form', () => {
    const item = {
      storeId: '107', storeLabel: '107', category1: 'A-HEALTH & BEAUTY', category1Code: 'A',
      category2: 'BEAUTY CARE', category2Code: 'A02', category3: '', category4: '', category5: '',
      articleCode: '552646', articleName: 'Mask', transactionCount: 1, saleQuantity: 1, saleAmount: 1,
      returnQuantity: 0, returnTransactionCount: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 1,
    };
    expect(categoryLabelOf(item, 'category2', '未分類')).toBe('A02  BEAUTY CARE');
    expect(itemMatchesCategorySelection(item, 'category2', new Set(['A02  BEAUTY CARE']), '未分類')).toBe(true);
    expect(itemMatchesCategorySelection(item, 'category2', new Set(['BEAUTY CARE']), '未分類')).toBe(true);
    expect(itemMatchesCategorySelection(item, 'category2', new Set(['A02']), '未分類')).toBe(true);
    expect(itemMatchesCategorySelection(item, 'category2', new Set(['HOUSEHOLD']), '未分類')).toBe(false);
  });

  it('loads only the periods a report view needs', () => {
    expect(periodKeysForView('overview')).toEqual([]);
    expect(periodKeysForView('products')).toEqual(['current']);
    expect(periodKeysForView('stores')).toEqual([]);
    expect(periodKeysForView('weekly')).toEqual([]);
    expect(periodKeysForView('focus')).toEqual(['current', 'yearAgoNext']);
    expect(periodKeysForView('categories', ['yearAgo', 'previous2'])).toEqual([
      'current', 'previous', 'previous2', 'yearAgo',
    ]);
  });
});
