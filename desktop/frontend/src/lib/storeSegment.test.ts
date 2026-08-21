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

  it('keeps weekday and weekend comparison buckets separate in segment totals', () => {
    const row = (businessId: string, tourist: boolean): SalesAnalysisWeekStore => ({
      businessId,
      label: businessId,
      salesTw: tourist ? 20 : 10,
      salesLw: tourist ? 18 : 9,
      customersTw: tourist ? 4 : 2,
      customersLw: tourist ? 3 : 1,
      weekdaySalesTw: tourist ? 12 : 6,
      weekdaySalesLw: tourist ? 10 : 5,
      weekendSalesTw: tourist ? 8 : 4,
      weekendSalesLw: tourist ? 8 : 4,
      weekdayCustomersTw: tourist ? 2 : 1,
      weekdayCustomersLw: tourist ? 1.5 : 0.5,
      weekendCustomersTw: tourist ? 2 : 1,
      weekendCustomersLw: tourist ? 1.5 : 0.5,
    });
    const rows = weeklySegmentRows([row('TA', false), row('NT', true)], {
      store: (store) => store.businessId ?? '',
      localTotal: '本地合計', touristTotal: '旅客合計', allStores: '全部門店',
    });
    const all = rows.find((candidate) => candidate.kind === 'allTotal')?.values;
    expect(all).toMatchObject({
      salesTw: 30, salesLw: 27,
      weekdaySalesTw: 18, weekdaySalesLw: 15,
      weekendSalesTw: 12, weekendSalesLw: 12,
    });
  });
});
