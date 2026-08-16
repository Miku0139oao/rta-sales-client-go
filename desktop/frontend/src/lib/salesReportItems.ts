import type { SalesAnalysisItem } from './types';

export type SalesReportCategoryLevel = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';
export type SalesReportFilterMode = 'blacklist' | 'whitelist';

export interface SalesReportFilter {
  mode: SalesReportFilterMode;
  excludeZeroGifts: boolean;
  excludeStamps: boolean;
  categories: string[];
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

export function defaultSalesReportFilter(): SalesReportFilter {
  return {
    mode: 'blacklist',
    excludeZeroGifts: true,
    excludeStamps: true,
    categories: [],
  };
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
  if (filter.categories.length === 0) return filter.mode === 'blacklist';
  const listed = filter.categories.includes(reportCategoryId(item, level));
  return filter.mode === 'whitelist' ? listed : !listed;
}

function itemText(item: SalesAnalysisItem): string {
  return `${item.articleName} ${item.articleCode} ${item.brandName ?? ''}`;
}
