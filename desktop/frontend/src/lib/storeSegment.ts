import type { SalesAnalysisWeekStore } from './types';

export type StoreSegment = 'local' | 'tourist';
export type WeeklySegmentKind = 'store' | 'localTotal' | 'touristTotal' | 'allTotal';

export interface WeeklySegmentRow {
  kind: WeeklySegmentKind;
  label: string;
  values: SalesAnalysisWeekStore;
}

export const TOURIST_STORE_CODES = ['NT', 'BB', 'SL'] as const;
export const LOCAL_STORE_ORDER = ['WC', 'TA', 'HK', 'SO', 'LY', 'TX', 'ML', 'MN', 'AC', 'SH', 'ZY', 'TP'] as const;

const TOURIST = new Set<string>(TOURIST_STORE_CODES);
const KNOWN = new Set<string>([...LOCAL_STORE_ORDER, ...TOURIST_STORE_CODES]);
const LOCAL_RANK = new Map(LOCAL_STORE_ORDER.map((code, index) => [code, index]));
const TOURIST_RANK = new Map(TOURIST_STORE_CODES.map((code, index) => [code, index]));

export function storeShortCode(store: { businessId?: string; label?: string }): string {
  const text = `${store.businessId ?? ''} ${store.label ?? ''}`.toUpperCase();
  const paren = /\(([A-Z]{2})\)/.exec(text);
  if (paren && KNOWN.has(paren[1]!)) return paren[1]!;
  for (const token of text.split(/[^A-Z0-9]+/)) {
    if (KNOWN.has(token)) return token;
  }
  const id = (store.businessId ?? '').trim().toUpperCase();
  return /^[A-Z]{2}$/.test(id) ? id : '';
}

export function storeSegment(store: { businessId?: string; label?: string }): StoreSegment {
  return TOURIST.has(storeShortCode(store)) ? 'tourist' : 'local';
}

export function compareStoresBySegment(
  left: { businessId?: string; label?: string },
  right: { businessId?: string; label?: string },
): number {
  const leftCode = storeShortCode(left);
  const rightCode = storeShortCode(right);
  const leftSegment = storeSegment(left);
  const rightSegment = storeSegment(right);
  if (leftSegment !== rightSegment) return leftSegment === 'local' ? -1 : 1;
  const rank: Map<string, number> = leftSegment === 'tourist' ? TOURIST_RANK : LOCAL_RANK;
  const leftRank = rank.get(leftCode) ?? 100;
  const rightRank = rank.get(rightCode) ?? 100;
  if (leftRank !== rightRank) return leftRank - rightRank;
  return (left.businessId ?? '').localeCompare(right.businessId ?? '', undefined, { numeric: true });
}

export function emptyWeekStore(): SalesAnalysisWeekStore {
  return {
    salesTw: 0, salesLw: 0, customersTw: 0, customersLw: 0,
    weekdaySalesTw: 0, weekdaySalesLw: 0, weekendSalesTw: 0, weekendSalesLw: 0,
    weekdayCustomersTw: 0, weekdayCustomersLw: 0, weekendCustomersTw: 0, weekendCustomersLw: 0,
  };
}

export function sumWeekStores(stores: SalesAnalysisWeekStore[]): SalesAnalysisWeekStore {
  return stores.reduce<SalesAnalysisWeekStore>((totals, store) => ({
    salesTw: totals.salesTw + store.salesTw,
    salesLw: totals.salesLw + store.salesLw,
    customersTw: totals.customersTw + store.customersTw,
    customersLw: totals.customersLw + store.customersLw,
    weekdaySalesTw: totals.weekdaySalesTw + store.weekdaySalesTw,
    weekdaySalesLw: totals.weekdaySalesLw + store.weekdaySalesLw,
    weekendSalesTw: totals.weekendSalesTw + store.weekendSalesTw,
    weekendSalesLw: totals.weekendSalesLw + store.weekendSalesLw,
    weekdayCustomersTw: totals.weekdayCustomersTw + store.weekdayCustomersTw,
    weekdayCustomersLw: totals.weekdayCustomersLw + store.weekdayCustomersLw,
    weekendCustomersTw: totals.weekendCustomersTw + store.weekendCustomersTw,
    weekendCustomersLw: totals.weekendCustomersLw + store.weekendCustomersLw,
  }), emptyWeekStore());
}

export function weeklySegmentRows(
  stores: SalesAnalysisWeekStore[],
  labels: { store: (row: SalesAnalysisWeekStore) => string; localTotal: string; touristTotal: string; allStores: string },
): WeeklySegmentRow[] {
  if (stores.length <= 1) {
    const store = stores[0];
    return store ? [{ kind: 'store', label: labels.store(store), values: store }] : [];
  }
  const local = stores.filter((store) => storeSegment(store) === 'local').sort(compareStoresBySegment);
  const tourist = stores.filter((store) => storeSegment(store) === 'tourist').sort(compareStoresBySegment);
  const rows: WeeklySegmentRow[] = local.map((store) => ({ kind: 'store', label: labels.store(store), values: store }));
  if (local.length > 0 && (tourist.length > 0 || local.length > 1)) {
    rows.push({ kind: 'localTotal', label: labels.localTotal, values: sumWeekStores(local) });
  }
  rows.push(...tourist.map((store): WeeklySegmentRow => ({ kind: 'store', label: labels.store(store), values: store })));
  if (tourist.length > 0 && (local.length > 0 || tourist.length > 1)) {
    rows.push({ kind: 'touristTotal', label: labels.touristTotal, values: sumWeekStores(tourist) });
  }
  if (local.length > 0 && tourist.length > 0) {
    rows.push({ kind: 'allTotal', label: labels.allStores, values: sumWeekStores(stores) });
  }
  return rows;
}
