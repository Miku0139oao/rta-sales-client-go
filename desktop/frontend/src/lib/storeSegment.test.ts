import { describe, expect, it } from 'vitest';
import { compareStoresBySegment, storeSegment, storeShortCode, weeklySegmentRows } from './storeSegment';
import type { SalesAnalysisWeekStore } from './types';

describe('store local / tourist segments', () => {
  it('treats NT, BB and SL as tourist and every other store as local', () => {
    expect(storeShortCode({ businessId: '107', label: '107-Tai Wai (TA)' })).toBe('TA');
    expect(storeSegment({ businessId: '107', label: '107-Tai Wai (TA)' })).toBe('local');
    expect(storeSegment({ businessId: 'NT', label: 'NT - Tsim Sha Tsui' })).toBe('tourist');
    expect(storeSegment({ label: 'BB Harbour City' })).toBe('tourist');
    expect(storeSegment({ businessId: 'SL' })).toBe('tourist');
    expect(storeSegment({ businessId: '108', label: '108 - Central' })).toBe('local');
  });

  it('orders local stores then tourist stores the way the MTD weekly sheet does', () => {
    const stores = [
      { businessId: 'NT' }, { businessId: 'TP' }, { businessId: 'WC' },
      { businessId: 'SL' }, { businessId: 'BB' }, { businessId: 'TA' },
    ];
    expect([...stores].sort(compareStoresBySegment).map((store) => store.businessId)).toEqual([
      'WC', 'TA', 'TP', 'NT', 'BB', 'SL',
    ]);
  });

  it('builds weekly rows with local total then tourist total', () => {
    const weekStore = (businessId: string, salesTw: number): SalesAnalysisWeekStore => ({
      businessId, label: businessId, salesTw, salesLw: 0, customersTw: 0, customersLw: 0,
      weekdaySalesTw: 0, weekdaySalesLw: 0, weekendSalesTw: 0, weekendSalesLw: 0,
      weekdayCustomersTw: 0, weekdayCustomersLw: 0, weekendCustomersTw: 0, weekendCustomersLw: 0,
    });
    const rows = weeklySegmentRows(
      [weekStore('NT', 30), weekStore('TA', 10), weekStore('WC', 20)],
      {
        store: (store) => store.businessId ?? '',
        localTotal: '本地合計', touristTotal: '旅客合計', allStores: '全部門店',
      },
    );
    expect(rows.map((row) => [row.kind, row.label, row.values.salesTw])).toEqual([
      ['store', 'WC', 20],
      ['store', 'TA', 10],
      ['localTotal', '本地合計', 30],
      ['store', 'NT', 30],
      ['touristTotal', '旅客合計', 30],
      ['allTotal', '全部門店', 60],
    ]);
  });
});
