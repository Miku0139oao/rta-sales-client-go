import type {
  SalesAnalysisItem,
  SalesAnalysisPeriodRequest,
  SalesAnalysisPeriodResult,
  SalesAnalysisProgress,
  SalesAnalysisRequest,
  SalesAnalysisResult,
  SalesAnalysisStore,
  SalesAnalysisTotals,
  SalesAnalysisWeek,
} from './types';
import { AppError } from './types';

const mockStoreNames = [
  'Central', 'Harbour', 'North', 'South', 'East', 'West', 'Park', 'Bay',
  'Hill', 'Garden', 'Market', 'Plaza', 'Station', 'Bridge', 'Harbour East', 'Central West',
];

const mockAnalysisItems: SalesAnalysisItem[] = [
  {
    storeId: '107', storeLabel: '107 - Central', category1: 'HEALTH & BEAUTY', category1Code: 'A', category2: 'BEAUTY CARE', category2Code: 'A02',
    category3: 'SKIN CARE', category3Code: 'A0201', category4: 'FACIAL', category4Code: 'A020101', category5: 'MASQUE', category5Code: 'A02010101', articleCode: '552646', articleName: 'AHC 安瓶精華纖維面膜', brandName: 'AHC',
    transactionCount: 1, saleQuantity: 3, saleAmount: 114, returnTransactionCount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 3, netSalesAmount: 114,
  },
  {
    storeId: '107', storeLabel: '107 - Central', category1: 'NON FOOD', category1Code: 'B', category2: 'HOUSEHOLD', category2Code: 'B03',
    category3: 'CLEANING', category3Code: 'B0302', category4: 'SURFACE', category4Code: 'B030201', category5: 'WIPES', category5Code: 'B03020102', articleCode: '900001', articleName: 'Household wipes', brandName: 'Mannings',
    transactionCount: 2, saleQuantity: 5, saleAmount: 86, returnTransactionCount: 1, returnQuantity: 1, returnAmount: 12, netQuantity: 4, netSalesAmount: 74,
  },
  {
    storeId: '108', storeLabel: '108 - Harbour', category1: 'HEALTH & BEAUTY', category1Code: 'A', category2: 'BEAUTY CARE', category2Code: 'A02',
    category3: 'SKIN CARE', category3Code: 'A0201', category4: 'FACIAL', category4Code: 'A020101', category5: 'CLEANSER', category5Code: 'A02010102', articleCode: '285627', articleName: 'BF 深層卸妝潔膚水', brandName: 'Bifesta',
    transactionCount: 4, saleQuantity: 6, saleAmount: 239.4, returnTransactionCount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 6, netSalesAmount: 239.4,
  },
];

const pause = (duration = 20) => new Promise<void>((resolve) => setTimeout(resolve, duration));

export function previewAnalysisStoresFor(count: number): SalesAnalysisStore[] {
  const total = count > 0 ? count : 2;
  return Array.from({ length: total }, (_, index) => ({
    businessId: String(107 + index),
    label: `${107 + index} - ${mockStoreNames[index % mockStoreNames.length]}`,
  }));
}

function mockItemTotals(items: SalesAnalysisItem[]): SalesAnalysisTotals {
  return items.reduce((value, item) => ({
    saleQuantity: value.saleQuantity + item.saleQuantity,
    saleAmount: value.saleAmount + item.saleAmount,
    returnQuantity: value.returnQuantity + item.returnQuantity,
    returnAmount: value.returnAmount + item.returnAmount,
    netQuantity: value.netQuantity + item.netQuantity,
    netSalesAmount: value.netSalesAmount + item.netSalesAmount,
  }), {
    saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0,
  });
}

function buildPreviewPeriods(
  requestedPeriods: SalesAnalysisPeriodRequest[],
  selected: SalesAnalysisStore[],
): SalesAnalysisPeriodResult[] {
  const scales: Record<string, number> = { current: 1, previous: 0.92, previous2: 0.84, yearAgo: 0.88, yearAgoNext: 1.06 };
  return requestedPeriods.map((period) => {
    const scale = scales[period.key] ?? 1;
    const items = selected.flatMap((store, storeIndex) => {
      const source = mockAnalysisItems[storeIndex % mockAnalysisItems.length] ?? mockAnalysisItems[0];
      if (!source) return [];
      const storeScale = scale * (0.55 + storeIndex * 0.05);
      return [{
        ...source,
        storeId: store.businessId,
        storeLabel: store.label,
        saleQuantity: source.saleQuantity * storeScale,
        saleAmount: source.saleAmount * storeScale,
        returnQuantity: source.returnQuantity * storeScale,
        returnAmount: source.returnAmount * storeScale,
        netQuantity: source.netQuantity * storeScale,
        netSalesAmount: source.netSalesAmount * storeScale,
      }];
    });
    const stores = selected.map((store, index) => {
      const totals = mockItemTotals(items.filter((item) => item.storeId === store.businessId));
      if (period.includeTrend) {
        totals.trendNetSalesAmount = totals.netSalesAmount + (index + 1) * 4 * scale;
        totals.transactionCount = (26 - index * 3) * scale;
      }
      return { ...store, totals };
    });
    const totals = mockItemTotals(items);
    if (period.includeTrend) {
      totals.trendNetSalesAmount = stores.reduce((sum, store) => sum + (store.totals.trendNetSalesAmount ?? 0), 0);
      totals.transactionCount = stores.reduce((sum, store) => sum + (store.totals.transactionCount ?? 0), 0);
    }
    return {
      key: period.key, label: period.label, from: period.from, to: period.to, complete: true,
      successfulStores: selected.length, totals,
      stores, items, issues: [],
    };
  });
}

function previewWeeklyPeriods(from: string, to: string, stores: SalesAnalysisPeriodResult['stores']): SalesAnalysisWeek[] {
  if (!from || !to || from > to || stores.length === 0) return [];
  const start = new Date(`${from}T00:00:00`);
  const end = new Date(`${to}T00:00:00`);
  const weekday = start.getDay() || 7;
  start.setDate(start.getDate() - (weekday - 1));
  const weeks: SalesAnalysisWeek[] = [];
  for (let cursor = new Date(start); cursor <= end; cursor.setDate(cursor.getDate() + 7)) {
    const weekFrom = cursor.toISOString().slice(0, 10);
    const weekToDate = new Date(cursor);
    weekToDate.setDate(weekToDate.getDate() + 6);
    const weekTo = weekToDate.toISOString().slice(0, 10);
    const weekStores = stores.map((store, index) => {
      const salesTw = 80_000 - index * 7_000;
      const salesLw = salesTw * 0.92;
      return {
        businessId: store.businessId, label: store.label,
        salesTw, salesLw, customersTw: 400 - index * 20, customersLw: 380 - index * 18,
        weekdaySalesTw: salesTw * 0.58, weekdaySalesLw: salesLw * 0.62,
        weekendSalesTw: salesTw * 0.42, weekendSalesLw: salesLw * 0.38,
        weekdayCustomersTw: 240 - index * 10, weekdayCustomersLw: 230 - index * 9,
        weekendCustomersTw: 160 - index * 10, weekendCustomersLw: 150 - index * 9,
      };
    });
    const totals = weekStores.reduce((sum, row) => ({
      salesTw: sum.salesTw + row.salesTw, salesLw: sum.salesLw + row.salesLw,
      customersTw: sum.customersTw + row.customersTw, customersLw: sum.customersLw + row.customersLw,
      weekdaySalesTw: sum.weekdaySalesTw + row.weekdaySalesTw, weekdaySalesLw: sum.weekdaySalesLw + row.weekdaySalesLw,
      weekendSalesTw: sum.weekendSalesTw + row.weekendSalesTw, weekendSalesLw: sum.weekendSalesLw + row.weekendSalesLw,
      weekdayCustomersTw: sum.weekdayCustomersTw + row.weekdayCustomersTw, weekdayCustomersLw: sum.weekdayCustomersLw + row.weekdayCustomersLw,
      weekendCustomersTw: sum.weekendCustomersTw + row.weekendCustomersTw, weekendCustomersLw: sum.weekendCustomersLw + row.weekendCustomersLw,
    }), {
      salesTw: 0, salesLw: 0, customersTw: 0, customersLw: 0,
      weekdaySalesTw: 0, weekdaySalesLw: 0, weekendSalesTw: 0, weekendSalesLw: 0,
      weekdayCustomersTw: 0, weekdayCustomersLw: 0, weekendCustomersTw: 0, weekendCustomersLw: 0,
    });
    weeks.push({ from: weekFrom, to: weekTo, stores: weekStores, totals });
  }
  return weeks;
}

function finishPreviewResult(
  operationId: string,
  selected: SalesAnalysisStore[],
  periods: SalesAnalysisPeriodResult[],
  pending: boolean,
): SalesAnalysisResult {
  const primary = periods[0]!;
  return {
    operationId, from: primary.from, to: primary.to, complete: !pending,
    pending,
    selectedStores: selected.length, successfulStores: primary.successfulStores, totals: primary.totals,
    stores: primary.stores, items: primary.items, issues: [], periods,
    weeks: pending ? [] : previewWeeklyPeriods(primary.from, primary.to, primary.stores),
    queryDurationMs: pending ? 80 : 180,
  };
}

export interface PreviewSalesHooks {
  onProgress?: (progress: SalesAnalysisProgress) => void;
  onUpdate?: (result: SalesAnalysisResult) => void;
  isCancelled?: () => boolean;
}

export async function runPreviewSalesAnalysis(
  request: SalesAnalysisRequest,
  hooks: PreviewSalesHooks = {},
): Promise<SalesAnalysisResult> {
  const operationId = `web-preview-${Date.now()}`;
  const catalog = previewAnalysisStoresFor(request.simulateStoreCount || Math.max(request.storeIds.length, 2));
  const selected = catalog.filter((store) => request.storeIds.includes(store.businessId));
  const usable = selected.length > 0 ? selected : catalog;
  const requestedPeriods = request.periods?.length ? request.periods : [{
    key: 'current', label: 'Current', from: request.from ?? '', to: request.to ?? '', includeTrend: false,
  }];
  const currentTasks = usable.length;
  const supplementTasks = Math.max(usable.length * Math.max(requestedPeriods.length - 1, 0), 1);
  hooks.onProgress?.({ operationId, current: 0, total: currentTasks, status: 'running' });
  let completed = 0;
  for (const store of usable) {
    await pause(16);
    if (hooks.isCancelled?.()) throw new AppError('cancelled', 'Sales analysis cancelled');
    completed += 1;
    hooks.onProgress?.({
      operationId, current: completed, total: currentTasks, storeId: store.businessId, storeLabel: store.label,
      periodKey: 'current', periodLabel: '本期', status: 'success',
    });
  }
  const allPeriods = buildPreviewPeriods(requestedPeriods, usable);
  const first = allPeriods.filter((period) => period.key === 'current');
  const staged = first.length > 0 ? first : allPeriods.slice(0, 1);
  window.setTimeout(() => {
    if (hooks.isCancelled?.()) return;
    let extra = 0;
    const tick = window.setInterval(() => {
      if (hooks.isCancelled?.()) {
        window.clearInterval(tick);
        return;
      }
      extra += 1;
      hooks.onProgress?.({
        operationId, current: extra, total: supplementTasks,
        periodKey: requestedPeriods[Math.min(extra, requestedPeriods.length - 1)]?.key,
        periodLabel: requestedPeriods[Math.min(extra, requestedPeriods.length - 1)]?.label,
        storeId: usable[extra % Math.max(usable.length, 1)]?.businessId,
        storeLabel: usable[extra % Math.max(usable.length, 1)]?.label,
        status: 'success',
      });
      if (extra >= supplementTasks) {
        window.clearInterval(tick);
        hooks.onUpdate?.(finishPreviewResult(operationId, usable, allPeriods, false));
      }
    }, 120);
  }, 40);
  return finishPreviewResult(operationId, usable, staged, staged.length < allPeriods.length);
}
