import { buildFocusGroups, type FocusGroup } from './analysisFocus';
import { defaultSalesReportFilter } from './salesReportItems';
import { normalizeRankingLimit } from './settings';
import type {
  SalesAnalysisItem,
  SalesAnalysisPeriodMemo,
  SalesAnalysisPeriodResult,
  SalesAnalysisRankedItem,
  SalesAnalysisReportMemo,
  SalesAnalysisResult,
  SalesAnalysisTotals,
} from './types';

export const EXPORT_FIXTURE_STORE_ID = '107';
export const EXPORT_FIXTURE_FROM = '2026-08-01';
export const EXPORT_FIXTURE_TO = '2026-08-16';
export const EXPORT_FIXTURE_PARTIAL = '部分資料尚未完成，缺值不代表零銷售';
export const EXPORT_FIXTURE_LONG_NAME = '超長中文商品名稱用於檢查截斷缺字與最後一列是否遺失之保濕修護精華液';

export const EXPORT_FIXTURE_CATEGORIES = [
  { code: 'A01', name: '保健護理' },
  { code: 'A02', name: '肌膚護理' },
  { code: 'A03', name: '個人護理' },
  { code: 'B05', name: '嬰兒用品' },
  { code: 'B10', name: '服飾配件' },
  { code: 'B12', name: '紙品用品' },
  { code: 'E08', name: '果汁飲品' },
  { code: 'A04', name: '保健贈品' },
] as const;

const CATEGORY_COUNTS = [100, 40, 32, 24, 16, 16, 16, 16];

export interface RankedIdentity {
  id: string;
  code: string;
  name: string;
  brand: string;
  amount: number;
  quantity: number;
}

export interface CategoryIdentity {
  id: string;
  code: string;
  name: string;
  amount: number;
  quantity: number;
  items: RankedIdentity[];
}

export interface ExportIdentities {
  from: string;
  to: string;
  storeId: string;
  rankingLimit: number;
  partial: boolean;
  netSalesAmount: number;
  netQuantity: number;
  topSales: RankedIdentity[];
  topQuantity: RankedIdentity[];
  amountGroups: CategoryIdentity[];
  quantityGroups: CategoryIdentity[];
  focusScreen: FocusGroup[];
  focusExport: FocusGroup[];
}

function zeroPaddedCode(categoryCode: string, index: number): string {
  return `${categoryCode.replace(/\D/g, '').padStart(2, '0')}${String(index + 1).padStart(5, '0')}`;
}

function totalsFor(items: SalesAnalysisItem[]): SalesAnalysisTotals {
  return items.reduce<SalesAnalysisTotals>((totals, item) => ({
    saleQuantity: totals.saleQuantity + item.saleQuantity,
    saleAmount: totals.saleAmount + item.saleAmount,
    returnQuantity: totals.returnQuantity + item.returnQuantity,
    returnAmount: totals.returnAmount + item.returnAmount,
    netQuantity: totals.netQuantity + item.netQuantity,
    netSalesAmount: totals.netSalesAmount + item.netSalesAmount,
    transactionCount: (totals.transactionCount ?? 0) + item.transactionCount,
    trendNetSalesAmount: (totals.trendNetSalesAmount ?? 0) + item.netSalesAmount,
  }), {
    saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0,
    netQuantity: 0, netSalesAmount: 0, transactionCount: 0, trendNetSalesAmount: 0,
  });
}

function scaleItems(items: SalesAnalysisItem[], scale: number): SalesAnalysisItem[] {
  return items.map((item) => ({
    ...item,
    transactionCount: item.transactionCount * scale,
    saleQuantity: item.saleQuantity * scale,
    saleAmount: item.saleAmount * scale,
    returnQuantity: item.returnQuantity * scale,
    returnAmount: item.returnAmount * scale,
    netQuantity: item.netQuantity * scale,
    netSalesAmount: item.netSalesAmount * scale,
  }));
}

export function exportFixtureItems(): SalesAnalysisItem[] {
  const items: SalesAnalysisItem[] = [];
  EXPORT_FIXTURE_CATEGORIES.forEach((category, groupIndex) => {
    const count = CATEGORY_COUNTS[groupIndex] ?? 16;
    for (let index = 0; index < count; index += 1) {
      const code = zeroPaddedCode(category.code, index);
      const amount = 900_000 - groupIndex * 8_000 - index * 17 - (index % 3);
      const quantity = 4_800 - groupIndex * 40 - index * 3 - (index % 5);
      items.push({
        storeId: EXPORT_FIXTURE_STORE_ID,
        storeLabel: '107 大圍合成門店',
        category1: '健康與美容',
        category1Code: 'A',
        category2: category.name,
        category2Code: category.code,
        category3: '商品種類',
        category3Code: `${category.code}01`,
        category4: '四級類目',
        category4Code: `${category.code}0101`,
        category5: '小分類',
        category5Code: `${category.code}010101`,
        articleCode: code,
        articleName: `${code} ${EXPORT_FIXTURE_LONG_NAME} ${index + 1}`,
        brandName: `品牌 ${((index + groupIndex) % 4) + 1}`,
        transactionCount: 2 + (index % 4),
        saleQuantity: quantity,
        saleAmount: amount,
        returnTransactionCount: 0,
        returnQuantity: 0,
        returnAmount: 0,
        netQuantity: quantity,
        netSalesAmount: amount,
      });
    }
  });
  return items;
}

function period(
  key: string,
  label: string,
  from: string,
  to: string,
  items: SalesAnalysisItem[],
  complete: boolean,
  issues: SalesAnalysisPeriodResult['issues'] = [],
): SalesAnalysisPeriodResult {
  const totals = totalsFor(items);
  return {
    key,
    label,
    from,
    to,
    complete,
    successfulStores: complete ? 1 : 0,
    totals,
    stores: [{ businessId: EXPORT_FIXTURE_STORE_ID, label: '107 大圍合成門店', totals }],
    items,
    issues,
  };
}

export function exportFixtureResult(): SalesAnalysisResult {
  const currentItems = exportFixtureItems();
  const previousItems = scaleItems(currentItems, 0.91);
  const previous2Items = scaleItems(currentItems, 0.84);
  const yearAgoItems = scaleItems(currentItems, 0.88);
  const yearAgoNextItems = scaleItems(currentItems, 1.07);
  const yearAgoIssues = [{
    periodKey: 'yearAgo',
    storeId: '109',
    storeLabel: '109 失敗門店',
    message: 'synthetic timeout',
  }];
  const periods = [
    period('current', '本期', EXPORT_FIXTURE_FROM, EXPORT_FIXTURE_TO, currentItems, true),
    period('previous', '上期', '2026-07-01', '2026-07-16', previousItems, true),
    period('previous2', '前期', '2026-06-01', '2026-06-16', previous2Items, true),
    period('yearAgo', '去年同期', '2025-08-01', '2025-08-16', yearAgoItems, false, yearAgoIssues),
    period('yearAgoNext', '去年下月', '2025-09-01', '2025-09-30', yearAgoNextItems, true),
  ];
  const current = periods[0]!;
  return {
    operationId: 'export-fixture-synthetic',
    from: EXPORT_FIXTURE_FROM,
    to: EXPORT_FIXTURE_TO,
    complete: false,
    pending: false,
    selectedStores: 2,
    successfulStores: 1,
    totals: current.totals,
    stores: current.stores,
    items: current.items,
    issues: yearAgoIssues,
    periods,
    queryDurationMs: 42,
  };
}

export function rankFixtureProducts(items: SalesAnalysisItem[], metric: 'amount' | 'quantity'): RankedIdentity[] {
  const grouped = new Map<string, RankedIdentity>();
  for (const item of items) {
    const id = item.articleCode.trim() || item.articleName.trim();
    if (!id) continue;
    const existing = grouped.get(id) ?? {
      id,
      code: item.articleCode.trim(),
      name: item.articleName.trim(),
      brand: item.brandName?.trim() ?? '',
      amount: 0,
      quantity: 0,
    };
    existing.amount += item.netSalesAmount;
    existing.quantity += item.netQuantity;
    grouped.set(id, existing);
  }
  return [...grouped.values()].sort((left, right) =>
    (metric === 'amount' ? right.amount - left.amount : right.quantity - left.quantity)
      || left.id.localeCompare(right.id, undefined, { numeric: true }),
  );
}

export function categoryFixtureGroups(
  items: SalesAnalysisItem[],
  metric: 'amount' | 'quantity',
  rankingLimit: number,
): CategoryIdentity[] {
  const limit = normalizeRankingLimit(rankingLimit);
  const grouped = new Map<string, { code: string; name: string; amount: number; quantity: number; source: SalesAnalysisItem[] }>();
  for (const item of items) {
    const code = item.category2Code?.trim() ?? '';
    const name = item.category2.trim();
    const id = code || name;
    const group = grouped.get(id) ?? { code, name, amount: 0, quantity: 0, source: [] };
    group.amount += item.netSalesAmount;
    group.quantity += item.netQuantity;
    group.source.push(item);
    grouped.set(id, group);
  }
  return [...grouped.entries()].map(([id, group]) => ({
    id,
    code: group.code,
    name: group.name,
    amount: group.amount,
    quantity: group.quantity,
    items: rankFixtureProducts(group.source, metric).slice(0, limit),
  })).sort((left, right) =>
    (metric === 'amount' ? right.amount - left.amount : right.quantity - left.quantity)
      || left.id.localeCompare(right.id, undefined, { numeric: true }),
  );
}

export function exportFixtureIdentities(
  rankingLimit: number,
  result: SalesAnalysisResult = exportFixtureResult(),
): ExportIdentities {
  const limit = normalizeRankingLimit(rankingLimit);
  const current = result.periods?.find((period) => period.key === 'current') ?? result;
  const currentItems = current.items ?? result.items ?? [];
  const yearAgoNext = result.periods?.find((period) => period.key === 'yearAgoNext');
  const totals = current.totals ?? result.totals;
  return {
    from: result.from,
    to: result.to,
    storeId: EXPORT_FIXTURE_STORE_ID,
    rankingLimit: limit,
    partial: true,
    netSalesAmount: totals.netSalesAmount,
    netQuantity: totals.netQuantity,
    topSales: rankFixtureProducts(currentItems, 'amount').slice(0, limit),
    topQuantity: rankFixtureProducts(currentItems, 'quantity').slice(0, limit),
    amountGroups: categoryFixtureGroups(currentItems, 'amount', limit),
    quantityGroups: categoryFixtureGroups(currentItems, 'quantity', limit),
    focusScreen: buildFocusGroups(yearAgoNext?.items ?? [], currentItems, 10),
    focusExport: buildFocusGroups(yearAgoNext?.items ?? [], currentItems, 8),
  };
}

function toRankedItem(item: RankedIdentity): SalesAnalysisRankedItem {
  return {
    id: item.id,
    code: item.code,
    name: item.name,
    brand: item.brand,
    amount: item.amount,
    quantity: item.quantity,
  };
}

export function exportFixtureMemo(
  rankingLimit: number,
  result: SalesAnalysisResult = exportFixtureResult(),
): SalesAnalysisReportMemo {
  const limit = normalizeRankingLimit(rankingLimit);
  const periods: SalesAnalysisPeriodMemo[] = (result.periods ?? []).map((period) => {
    const items = period.items ?? [];
    const memo: SalesAnalysisPeriodMemo = {
      key: period.key,
      totals: period.totals,
      topAmount: rankFixtureProducts(items, 'amount').slice(0, Math.max(limit, 100)).map(toRankedItem),
      topQuantity: rankFixtureProducts(items, 'quantity').slice(0, Math.max(limit, 100)).map(toRankedItem),
      amountGroups: categoryFixtureGroups(items, 'amount', limit).map((group) => ({
        id: group.id,
        code: group.code,
        name: group.name,
        amount: group.amount,
        quantity: group.quantity,
        items: group.items.map(toRankedItem),
      })),
      quantityGroups: categoryFixtureGroups(items, 'quantity', limit).map((group) => ({
        id: group.id,
        code: group.code,
        name: group.name,
        amount: group.amount,
        quantity: group.quantity,
        items: group.items.map(toRankedItem),
      })),
    };
    if (period.key === 'yearAgoNext') {
      memo.focusGroups = buildFocusGroups(items, result.periods?.find((row) => row.key === 'current')?.items ?? [], 8);
    }
    return memo;
  });
  return { periods };
}

export const exportFixtureFilter = defaultSalesReportFilter();

export const SMALL_EXPORT_STORE_LABEL = '107 大圍合成門店';
export const SMALL_EXPORT_GROUP_ID = 'focus-pair';
export const SMALL_EXPORT_GROUP_NAME = '焦點兩件';
export const SMALL_EXPORT_GROUP_CODES = ['0200001', '0300001'] as const;
export const SMALL_EXPORT_HEALTH_FILTER = 'A01  保健護理';

export interface SmallExportSpec {
  code: string;
  name: string;
  categoryCode: string;
  categoryName: string;
  amount: number;
  quantity: number;
  transactionCount: number;
}

// Hand-checkable current-period rows. Previous=0.5, yearAgo=0.8, yearAgoNext=1.1, previous2=0.4.
export const SMALL_EXPORT_SPECS: readonly SmallExportSpec[] = [
  { code: '0200001', name: '0200001 S01', categoryCode: 'A02', categoryName: '肌膚護理', amount: 2000, quantity: 200, transactionCount: 10 },
  { code: '0300001', name: '0300001 P01', categoryCode: 'A03', categoryName: '個人護理', amount: 1800, quantity: 400, transactionCount: 10 },
  { code: '0300002', name: '0300002 P02', categoryCode: 'A03', categoryName: '個人護理', amount: 1600, quantity: 300, transactionCount: 10 },
  { code: '0200002', name: '0200002 S02', categoryCode: 'A02', categoryName: '肌膚護理', amount: 1500, quantity: 150, transactionCount: 10 },
  { code: '0100001', name: '0100001 H01', categoryCode: 'A01', categoryName: '保健護理', amount: 1200, quantity: 120, transactionCount: 10 },
  { code: '0100002', name: '0100002 H02', categoryCode: 'A01', categoryName: '保健護理', amount: 1100, quantity: 110, transactionCount: 10 },
  { code: '0100003', name: '0100003 H03', categoryCode: 'A01', categoryName: '保健護理', amount: 1000, quantity: 100, transactionCount: 10 },
  { code: '0100004', name: '0100004 H04', categoryCode: 'A01', categoryName: '保健護理', amount: 900, quantity: 90, transactionCount: 10 },
  { code: '0100005', name: '0100005 H05', categoryCode: 'A01', categoryName: '保健護理', amount: 800, quantity: 80, transactionCount: 10 },
  { code: '0100006', name: '0100006 H06', categoryCode: 'A01', categoryName: '保健護理', amount: 700, quantity: 70, transactionCount: 10 },
  { code: '0100007', name: '0100007 H07', categoryCode: 'A01', categoryName: '保健護理', amount: 600, quantity: 60, transactionCount: 10 },
  { code: '0100008', name: '0100008 H08', categoryCode: 'A01', categoryName: '保健護理', amount: 500, quantity: 50, transactionCount: 10 },
  { code: '0100009', name: '0100009 H09', categoryCode: 'A01', categoryName: '保健護理', amount: 400, quantity: 40, transactionCount: 10 },
  { code: '0100010', name: '0100010 H10', categoryCode: 'A01', categoryName: '保健護理', amount: 300, quantity: 30, transactionCount: 10 },
  { code: '0100011', name: '0100011 H11', categoryCode: 'A01', categoryName: '保健護理', amount: 200, quantity: 20, transactionCount: 10 },
  { code: '0100012', name: '0100012 H12', categoryCode: 'A01', categoryName: '保健護理', amount: 100, quantity: 10, transactionCount: 10 },
];

export const SMALL_EXPORT_SCALES = {
  current: 1,
  previous: 0.5,
  previous2: 0.4,
  yearAgo: 0.8,
  yearAgoNext: 1.1,
} as const;

function smallItem(spec: SmallExportSpec, scale: number): SalesAnalysisItem {
  return {
    storeId: EXPORT_FIXTURE_STORE_ID,
    storeLabel: SMALL_EXPORT_STORE_LABEL,
    category1: '健康與美容',
    category1Code: 'A',
    category2: spec.categoryName,
    category2Code: spec.categoryCode,
    category3: '商品種類',
    category3Code: `${spec.categoryCode}01`,
    category4: '四級類目',
    category4Code: `${spec.categoryCode}0101`,
    category5: '小分類',
    category5Code: `${spec.categoryCode}010101`,
    articleCode: spec.code,
    articleName: spec.name,
    brandName: '品牌A',
    transactionCount: spec.transactionCount * scale,
    saleQuantity: spec.quantity * scale,
    saleAmount: spec.amount * scale,
    returnTransactionCount: 0,
    returnQuantity: 0,
    returnAmount: 0,
    netQuantity: spec.quantity * scale,
    netSalesAmount: spec.amount * scale,
  };
}

export function smallExportItems(scale: number = 1): SalesAnalysisItem[] {
  return SMALL_EXPORT_SPECS.map((spec) => smallItem(spec, scale));
}

export function smallExportResult(): SalesAnalysisResult {
  const specs: Array<[keyof typeof SMALL_EXPORT_SCALES, string, string, string]> = [
    ['current', '本期', EXPORT_FIXTURE_FROM, EXPORT_FIXTURE_TO],
    ['previous', '上期', '2026-07-01', '2026-07-16'],
    ['previous2', '前期', '2026-06-01', '2026-06-16'],
    ['yearAgo', '去年同期', '2025-08-01', '2025-08-16'],
    ['yearAgoNext', '去年下月', '2025-09-01', '2025-09-30'],
  ];
  const periods = specs.map(([key, label, from, to]) => {
    const items = smallExportItems(SMALL_EXPORT_SCALES[key]);
    return period(key, label, from, to, items, true);
  });
  const current = periods[0]!;
  return {
    operationId: 'export-fixture-small',
    from: EXPORT_FIXTURE_FROM,
    to: EXPORT_FIXTURE_TO,
    complete: true,
    pending: false,
    selectedStores: 1,
    successfulStores: 1,
    totals: current.totals,
    stores: current.stores,
    items: current.items,
    issues: [],
    periods,
    queryDurationMs: 7,
  };
}
