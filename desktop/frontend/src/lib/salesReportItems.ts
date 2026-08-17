import { itemMatchesCategorySelection, type AnalysisCategoryKey } from './salesAnalysisItems';
import type { SalesAnalysisItem } from './types';

export type SalesReportCategoryLevel = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';
export type SalesReportFilterMode = 'blacklist' | 'whitelist';
export type SalesReportFacets = Partial<Record<SalesReportCategoryLevel, string[]>>;

export interface SalesReportFilter {
  mode: SalesReportFilterMode;
  excludeZeroGifts: boolean;
  excludeStamps: boolean;
  categories: string[];
  facets?: SalesReportFacets;
  search?: string;
  uncategorized?: string;
}

const GIFT_CATEGORY = /(?:^|[\s_-])(?:FREE[\s_-]*)?GIFT(?:[\s_-]|$)/;
const CASH_COUPON = /現金(?:優惠)?券/;
const STAMP_ITEM = /印花/;
const NOMINAL_AMOUNT = 0.01;

const CATEGORY_CODE: Record<SalesReportCategoryLevel, keyof SalesAnalysisItem> = {
  category1: 'category1Code',
  category2: 'category2Code',
  category3: 'category3Code',
  category4: 'category4Code',
  category5: 'category5Code',
};

const CATEGORY_LEVELS: SalesReportCategoryLevel[] = [
  'category1', 'category2', 'category3', 'category4', 'category5',
];

export function defaultSalesReportFilter(): SalesReportFilter {
  return {
    mode: 'blacklist',
    excludeZeroGifts: true,
    excludeStamps: true,
    categories: [],
    facets: {},
    search: '',
  };
}

export function salesReportHasScreenFilters(filter: SalesReportFilter): boolean {
  if (filter.search?.trim()) return true;
  return CATEGORY_LEVELS.some((key) => (filter.facets?.[key]?.length ?? 0) > 0);
}

export function isGiftCategoryItem(item: SalesAnalysisItem): boolean {
  return [
    item.category1, item.category1Code, item.category2, item.category2Code,
    item.category3, item.category3Code, item.category4, item.category4Code,
    item.category5, item.category5Code,
  ].some((value) => {
    const text = (value ?? '').trim();
    return Boolean(text) && (GIFT_CATEGORY.test(text.toUpperCase()) || text.includes('贈品'));
  });
}

export function isCashCouponItem(item: SalesAnalysisItem): boolean {
  return CASH_COUPON.test(itemText(item));
}

export function isStampItem(item: SalesAnalysisItem): boolean {
  return STAMP_ITEM.test(itemText(item)) || [
    item.category1, item.category2, item.category3, item.category4, item.category5,
  ].some((value) => STAMP_ITEM.test(value ?? ''));
}

export function reportCategoryId(item: SalesAnalysisItem, level: SalesReportCategoryLevel): string {
  const code = typeof item[CATEGORY_CODE[level]] === 'string' ? String(item[CATEGORY_CODE[level]]).trim() : '';
  const name = typeof item[level] === 'string' ? item[level].trim() : '';
  return code || name;
}

export function includeInSalesReport(
  item: SalesAnalysisItem,
  filter: SalesReportFilter = defaultSalesReportFilter(),
  level: SalesReportCategoryLevel = 'category2',
): boolean {
  if (filter.excludeStamps && isStampItem(item)) return false;
  if (
    filter.excludeZeroGifts
    && Math.abs(item.netSalesAmount) <= NOMINAL_AMOUNT
    && isGiftCategoryItem(item)
    && !isCashCouponItem(item)
  ) return false;
  if (!matchesReportFacets(item, filter)) return false;
  if (!matchesReportSearch(item, filter.search ?? '')) return false;
  if (filter.categories.length === 0) return filter.mode === 'blacklist';
  const listed = filter.categories.includes(reportCategoryId(item, level));
  return filter.mode === 'whitelist' ? listed : !listed;
}

function matchesReportFacets(item: SalesAnalysisItem, filter: SalesReportFilter): boolean {
  const uncategorized = filter.uncategorized?.trim() || '未分類';
  return CATEGORY_LEVELS.every((key) => {
    const selected = filter.facets?.[key] ?? [];
    if (selected.length === 0) return true;
    return itemMatchesCategorySelection(item, key as AnalysisCategoryKey, new Set(selected), uncategorized);
  });
}

function matchesReportSearch(item: SalesAnalysisItem, searchTerm: string): boolean {
  const term = searchTerm.trim().toLocaleLowerCase();
  if (!term) return true;
  return [
    item.storeId, item.storeLabel, item.articleCode, item.articleName, item.brandName ?? '',
    item.category1, item.category1Code ?? '', item.category2, item.category2Code ?? '',
    item.category3, item.category3Code ?? '', item.category4, item.category4Code ?? '',
    item.category5, item.category5Code ?? '',
  ].some((value) => value.toLocaleLowerCase().includes(term));
}

function itemText(item: SalesAnalysisItem): string {
  return `${item.articleName} ${item.articleCode} ${item.brandName ?? ''}`;
}
