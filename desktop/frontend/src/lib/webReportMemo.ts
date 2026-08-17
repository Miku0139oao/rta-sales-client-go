import { buildFocusGroups } from './analysisFocus';
import {
  defaultSalesReportFilter,
  includeInSalesReport,
  reportCategoryId,
  type SalesReportCategoryLevel,
  type SalesReportFilter,
} from './salesReportItems';
import type {
  ManCodeGroup,
  SalesAnalysisFocusGroup,
  SalesAnalysisItem,
  SalesAnalysisPeriodMemo,
  SalesAnalysisRankedItem,
  SalesAnalysisReportMemo,
  SalesAnalysisReportMemoRequest,
  SalesAnalysisResult,
  SalesAnalysisTotals,
} from './types';

const TOP_PRODUCTS = 15;
const GROUP_ITEMS = 24;

function categoryLevel(value: string): SalesReportCategoryLevel {
  if (value === 'category1' || value === 'category3' || value === 'category4' || value === 'category5') return value;
  return 'category2';
}

function categoryCode(item: SalesAnalysisItem, level: SalesReportCategoryLevel): string {
  if (level === 'category1') return item.category1Code?.trim() ?? '';
  if (level === 'category3') return item.category3Code?.trim() ?? '';
  if (level === 'category4') return item.category4Code?.trim() ?? '';
  if (level === 'category5') return item.category5Code?.trim() ?? '';
  return item.category2Code?.trim() ?? '';
}

function categoryName(item: SalesAnalysisItem, level: SalesReportCategoryLevel): string {
  return (item[level] ?? '').trim();
}

function itemTotals(items: SalesAnalysisItem[]): SalesAnalysisTotals {
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

function rankedProduct(item: SalesAnalysisItem): SalesAnalysisRankedItem {
  const code = item.articleCode.trim();
  const name = item.articleName.trim();
  return {
    id: code || name,
    code,
    name,
    brand: item.brandName?.trim() || undefined,
    amount: item.netSalesAmount,
    quantity: item.netQuantity,
    category2Code: item.category2Code,
    category3Code: item.category3Code,
    category4Code: item.category4Code,
  };
}

function mergeRanked(items: SalesAnalysisItem[]): SalesAnalysisRankedItem[] {
  const byId = new Map<string, SalesAnalysisRankedItem>();
  for (const item of items) {
    const next = rankedProduct(item);
    if (!next.id) continue;
    const current = byId.get(next.id);
    if (!current) {
      byId.set(next.id, next);
      continue;
    }
    current.amount += next.amount;
    current.quantity += next.quantity;
    if (!current.name && next.name) current.name = next.name;
  }
  return [...byId.values()];
}

function sortRanked(items: SalesAnalysisRankedItem[], key: 'amount' | 'quantity'): SalesAnalysisRankedItem[] {
  return [...items].sort((left, right) => {
    const delta = (right[key] - left[key]) || left.id.localeCompare(right.id);
    return delta;
  });
}

function buildCategoryGroups(items: SalesAnalysisItem[], level: SalesReportCategoryLevel) {
  const grouped = new Map<string, { id: string; code: string; name: string; amount: number; quantity: number; items: SalesAnalysisItem[] }>();
  for (const item of items) {
    const code = categoryCode(item, level);
    const name = categoryName(item, level);
    const id = reportCategoryId(item, level) || name || code;
    if (!id) continue;
    const current = grouped.get(id) ?? { id, code, name, amount: 0, quantity: 0, items: [] };
    current.amount += item.netSalesAmount;
    current.quantity += item.netQuantity;
    if (!current.name && name) current.name = name;
    current.items.push(item);
    grouped.set(id, current);
  }
  return [...grouped.values()].map((group) => ({
    id: group.id,
    code: group.code,
    name: group.name || '未分類',
    amount: group.amount,
    quantity: group.quantity,
    items: mergeRanked(group.items),
  }));
}

function filterItems(
  items: SalesAnalysisItem[],
  request: SalesAnalysisReportMemoRequest,
  group?: ManCodeGroup,
): SalesAnalysisItem[] {
  const filter: SalesReportFilter = {
    ...defaultSalesReportFilter(),
    excludeZeroGifts: request.excludeZeroGifts,
    excludeStamps: request.excludeStamps,
    mode: request.mode === 'whitelist' ? 'whitelist' : 'blacklist',
    categories: request.categories ?? [],
  };
  const level = categoryLevel(request.categoryLevel);
  const codes = new Set((group?.codes ?? []).map((code) => code.trim()).filter(Boolean));
  return items.filter((item) => {
    if (request.storeId && item.storeId !== request.storeId) return false;
    if (codes.size > 0 && !codes.has(item.articleCode.trim())) return false;
    return includeInSalesReport(item, filter, level);
  });
}

function toFocusMemo(groups: ReturnType<typeof buildFocusGroups>): SalesAnalysisFocusGroup[] {
  return groups.map((group) => ({
    id: group.id,
    prefix: group.prefix,
    name: group.name,
    sales: group.sales,
    quantity: group.quantity,
  }));
}

export function buildWebReportMemo(
  result: SalesAnalysisResult,
  request: SalesAnalysisReportMemoRequest,
  groups: ManCodeGroup[],
): SalesAnalysisReportMemo {
  const level = categoryLevel(request.categoryLevel);
  const group = request.groupId ? groups.find((candidate) => candidate.id === request.groupId) : undefined;
  const catalog = group ? [group] : groups;
  const currentItems = filterItems(result.periods?.find((period) => period.key === 'current')?.items ?? result.items ?? [], request, group);
  const periods: SalesAnalysisPeriodMemo[] = (result.periods ?? []).map((period) => {
    const items = filterItems(period.items ?? [], request, group);
    const products = mergeRanked(items);
    const categories = buildCategoryGroups(items, level);
    const memo: SalesAnalysisPeriodMemo = {
      key: period.key,
      totals: itemTotals(items),
      topAmount: sortRanked(products, 'amount').slice(0, TOP_PRODUCTS),
      topQuantity: sortRanked(products, 'quantity').slice(0, TOP_PRODUCTS),
      amountGroups: [...categories]
        .sort((left, right) => right.amount - left.amount || left.id.localeCompare(right.id))
        .map((entry) => ({ ...entry, items: sortRanked(entry.items, 'amount').slice(0, GROUP_ITEMS) })),
      quantityGroups: [...categories]
        .sort((left, right) => right.quantity - left.quantity || left.id.localeCompare(right.id))
        .map((entry) => ({ ...entry, items: sortRanked(entry.items, 'quantity').slice(0, GROUP_ITEMS) })),
    };
    return memo;
  });
  const yearAgoNext = periods.find((period) => period.key === 'yearAgoNext');
  if (yearAgoNext) {
    const nextItems = filterItems(result.periods?.find((period) => period.key === 'yearAgoNext')?.items ?? [], request, group);
    yearAgoNext.focusGroups = toFocusMemo(buildFocusGroups(nextItems, currentItems, 8, catalog));
    yearAgoNext.focusCatalog = catalog.length > 0;
  }
  return { periods };
}

export function collectReportGlyphs(result: SalesAnalysisResult, groups: ManCodeGroup[] = []): string {
  const seen = new Set<string>();
  const add = (value?: string) => {
    for (const char of value ?? '') seen.add(char);
  };
  add(result.from);
  add(result.to);
  for (const store of result.stores ?? []) add(store.label);
  for (const period of result.periods ?? []) {
    add(period.label);
    for (const store of period.stores ?? []) add(store.label);
    for (const item of period.items ?? []) {
      add(item.storeLabel);
      add(item.articleCode);
      add(item.articleName);
      add(item.brandName);
      add(item.category1);
      add(item.category2);
      add(item.category3);
      add(item.category4);
      add(item.category5);
    }
  }
  for (const group of groups) add(group.name);
  return [...seen].join('');
}
