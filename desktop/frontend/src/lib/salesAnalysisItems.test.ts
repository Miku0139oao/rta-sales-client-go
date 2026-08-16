import { describe, expect, it } from 'vitest';
import { needsSalesAnalysisItemHydration, periodKeysForView, unpackSalesAnalysisItems } from './salesAnalysisItems';

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

  it('loads only the periods a report view needs', () => {
    expect(periodKeysForView('overview')).toEqual(['current']);
    expect(periodKeysForView('products')).toEqual(['current']);
    expect(periodKeysForView('stores')).toEqual([]);
    expect(periodKeysForView('weekly')).toEqual([]);
    expect(periodKeysForView('focus')).toEqual(['current', 'yearAgoNext']);
    expect(periodKeysForView('categories', ['yearAgo', 'previous2'])).toEqual([
      'current', 'previous', 'previous2', 'yearAgo',
    ]);
  });
});
