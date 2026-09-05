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
import type { SalesAnalysisPackedRow } from './types';

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

  it('prefers compact aliases and keeps legacy object rows', () => {
    const stores = [{
      businessId: '107', label: '107 - Central',
      totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 10.25 },
    }];
    const compact = unpackSalesAnalysisItems({
      k: 'current', d: ['', 'A1', 'Mask', 'AHC'], r: [[0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 1.5, 10.25, 0, 0, 0, 1.5, 10.25]],
      dict: ['ignored'], rows: [{ s: 0, ac: 1, an: 2, ns: 99 }],
    }, stores);
    expect(compact).toEqual([expect.objectContaining({
      storeId: '107', storeLabel: '107 - Central', articleCode: 'A1', articleName: 'Mask', brandName: 'AHC',
      transactionCount: 2, saleQuantity: 1.5, saleAmount: 10.25, netQuantity: 1.5, netSalesAmount: 10.25,
    })]);
    const legacy = unpackSalesAnalysisItems({
      periodKey: 'current', dict: ['', 'B2', 'Wipes'], rows: [{ s: 0, ac: 1, an: 2, ns: 8.5 }],
    }, stores);
    expect(legacy).toEqual([expect.objectContaining({
      storeId: '107', articleCode: 'B2', articleName: 'Wipes', netSalesAmount: 8.5, saleAmount: 0, transactionCount: 0,
    })]);
  });

  it('treats missing dictionary values, short compact rows and unknown stores as empty or zero', () => {
    const items = unpackSalesAnalysisItems({
      d: ['', 'A1'],
      rows: [
        [0, 1],
        [9, 99, -1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4.5, 99],
        { ac: 1, an: 2, ns: 3 } as SalesAnalysisPackedRow,
      ],
    }, [{ businessId: '107', label: '107 - Central', totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 } }]);
    expect(items).toHaveLength(3);
    expect(items[0]).toEqual(expect.objectContaining({
      storeId: '107', articleCode: 'A1', articleName: '', brandName: '', netSalesAmount: 0, saleAmount: 0,
    }));
    expect(items[1]).toEqual(expect.objectContaining({
      storeId: '', storeLabel: '', articleCode: '', articleName: '', netSalesAmount: 4.5,
    }));
    expect(items[2]).toEqual(expect.objectContaining({
      storeId: '', articleCode: 'A1', articleName: '', transactionCount: 0, netSalesAmount: 3,
    }));
  });

  it('round-trips compact rows through JSON without changing counts, sums or codes', () => {
    const stores = [
      { businessId: '107', label: '107 - Central', totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 10.25 } },
      { businessId: '108', label: '108 - Harbour', totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 8.5 } },
    ];
    const items = [
      {
        storeId: '107', storeLabel: '107 - Central', category1: 'HEALTH', category1Code: 'A',
        category2: 'BEAUTY', category2Code: 'A02', category3: '', category3Code: '', category4: '', category4Code: '', category5: '', category5Code: '',
        articleCode: '0552646', articleName: 'Mask', brandName: 'AHC',
        transactionCount: 1, saleQuantity: 2, saleAmount: 10.25, returnQuantity: 0, returnTransactionCount: 0,
        returnAmount: 0, netQuantity: 2, netSalesAmount: 10.25,
      },
      {
        storeId: '108', storeLabel: '108 - Harbour', category1: 'HEALTH', category1Code: 'A',
        category2: 'BEAUTY', category2Code: 'A02', category3: '', category3Code: '', category4: '', category4Code: '', category5: '', category5Code: '',
        articleCode: '00002', articleName: 'Wipes', brandName: '',
        transactionCount: 1, saleQuantity: 1, saleAmount: 8.5, returnQuantity: 0, returnTransactionCount: 0,
        returnAmount: 0, netQuantity: 1, netSalesAmount: 8.5,
      },
    ];
    const packed = packSalesAnalysisItems('current', items, stores);
    const wire = JSON.parse(JSON.stringify(packed));
    const unpacked = unpackSalesAnalysisItems(wire, stores);
    expect(unpacked).toEqual(items);
    expect(unpacked).toHaveLength(2);
    expect(unpacked.reduce((sum, item) => sum + item.netSalesAmount, 0)).toBe(18.75);
    expect(unpacked.map((item) => item.articleCode)).toEqual(['0552646', '00002']);
  });
});

describe('sales analysis item batches at synthetic scale', () => {
  function scaleItems(count: number, storeCount: number) {
    const stores = Array.from({ length: storeCount }, (_, index) => {
      const businessId = String(100 + index);
      return {
        businessId,
        label: `${businessId} - Store ${index}`,
        totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
      };
    });
    const items = Array.from({ length: count }, (_, index) => {
      const store = stores[index % storeCount]!;
      const article = index % 2500;
      const quantity = (index % 17) + 1;
      const price = 10.25 + (article % 50);
      const saleAmount = quantity * price;
      return {
        storeId: store.businessId,
        storeLabel: store.label,
        category1: 'HEALTH',
        category1Code: 'A',
        category2: 'BEAUTY',
        category2Code: 'A02',
        category3: '',
        category3Code: '',
        category4: '',
        category4Code: '',
        category5: '',
        category5Code: '',
        articleCode: String(100000 + article),
        articleName: `Item ${article}`,
        brandName: `Brand ${article % 40}`,
        transactionCount: quantity,
        saleQuantity: quantity,
        saleAmount,
        returnQuantity: 0,
        returnTransactionCount: 0,
        returnAmount: 0,
        netQuantity: quantity,
        netSalesAmount: saleAmount,
      };
    });
    return { stores, items };
  }

  function checksum(items: Array<{ articleCode: string; netSalesAmount: number; netQuantity: number }>) {
    return items.reduce((sum, item) => ({
      count: sum.count + 1,
      netSalesAmount: sum.netSalesAmount + item.netSalesAmount,
      netQuantity: sum.netQuantity + item.netQuantity,
      codes: sum.codes + item.articleCode.length,
    }), { count: 0, netSalesAmount: 0, netQuantity: 0, codes: 0 });
  }

  it('round-trips 100000 compact rows with matching counts, sums and codes', () => {
    const { stores, items } = scaleItems(100000, 40);
    const unpacked = unpackSalesAnalysisItems(JSON.parse(JSON.stringify(packSalesAnalysisItems('current', items, stores))), stores);
    expect(checksum(unpacked)).toEqual(checksum(items));
    expect(unpacked[0]).toEqual(items[0]);
    expect(unpacked[99999]).toEqual(items[99999]);
    expect(unpacked[12345]!.storeId).toBe(items[12345]!.storeId);
  }, 60000);

  it('round-trips 200000 compact rows with matching counts, sums and codes', () => {
    const { stores, items } = scaleItems(200000, 80);
    const unpacked = unpackSalesAnalysisItems(JSON.parse(JSON.stringify(packSalesAnalysisItems('current', items, stores))), stores);
    expect(checksum(unpacked)).toEqual(checksum(items));
    expect(unpacked[0]!.articleCode).toBe(items[0]!.articleCode);
    expect(unpacked[199999]!.netSalesAmount).toBe(items[199999]!.netSalesAmount);
    expect(unpacked[150000]!.storeLabel).toBe(items[150000]!.storeLabel);
  }, 60000);
});
