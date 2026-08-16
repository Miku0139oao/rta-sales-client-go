import type { SalesAnalysisItem } from './types';

export interface FocusProduct {
  id: string;
  code: string;
  name: string;
  brand: string;
  amount: number;
  quantity: number;
  currentAmount: number;
  currentQuantity: number;
}

export interface FocusGroup {
  id: string;
  prefix: string;
  name?: string;
  sales: FocusProduct[];
  quantity: FocusProduct[];
}

export interface FocusCatalogGroup {
  id: string;
  name: string;
  codes: string[];
}

export const FOCUS_GROUP_PREFIXES = [
  { id: 'health', prefix: 'A01' },
  { id: 'skin', prefix: 'A02' },
  { id: 'pc', prefix: 'A03' },
] as const;

const TOP_N = 10;

export function buildFocusGroups(
  yearAgoNextItems: SalesAnalysisItem[],
  currentItems: SalesAnalysisItem[] = [],
  topN = TOP_N,
  catalog: readonly FocusCatalogGroup[] = [],
): FocusGroup[] {
  const currentByCode = aggregateProducts(currentItems);
  return resolveFocusSpecs(catalog).map((group) => {
    const products = aggregateProducts(yearAgoNextItems.filter((item) => matchesFocusSpec(item, group)));
    const ranked = [...products.values()].map((product) => {
      const current = currentByCode.get(product.code);
      return {
        ...product,
        currentAmount: current?.amount ?? 0,
        currentQuantity: current?.quantity ?? 0,
      };
    });
    return {
      id: group.id,
      prefix: group.prefix,
      name: group.name || undefined,
      sales: [...ranked].sort(compareFocusSales).slice(0, topN),
      quantity: [...ranked].sort(compareFocusQuantity).slice(0, topN),
    };
  }).filter((group) => group.sales.length > 0 || group.quantity.length > 0);
}

export function catalogCodeSet(catalog: readonly FocusCatalogGroup[]): Set<string> {
  const codes = new Set<string>();
  for (const group of catalog) {
    for (const code of group.codes ?? []) {
      const trimmed = code.trim();
      if (trimmed) codes.add(trimmed);
    }
  }
  return codes;
}

export function matchesFocusPrefix(item: SalesAnalysisItem, prefix: string): boolean {
  const department = item.category2Code?.trim();
  if (department) return department === prefix || department.startsWith(prefix);
  const fallback = item.category3Code?.trim() || item.category4Code?.trim() || '';
  return fallback === prefix || fallback.startsWith(prefix);
}

interface FocusSpec {
  id: string;
  name: string;
  prefix: string;
  codes?: Set<string>;
}

function resolveFocusSpecs(catalog: readonly FocusCatalogGroup[]): FocusSpec[] {
  const used: FocusSpec[] = [];
  for (const group of catalog) {
    const codes = catalogCodeSet([group]);
    if (codes.size === 0) continue;
    used.push({ id: group.id, name: group.name, prefix: '', codes });
  }
  if (used.length === 0) {
    return FOCUS_GROUP_PREFIXES.map((group) => ({ id: group.id, name: '', prefix: group.prefix }));
  }
  return used;
}

function matchesFocusSpec(item: SalesAnalysisItem, spec: FocusSpec): boolean {
  if (spec.codes) return spec.codes.has(item.articleCode.trim());
  return matchesFocusPrefix(item, spec.prefix);
}

function aggregateProducts(items: SalesAnalysisItem[]): Map<string, FocusProduct> {
  const products = new Map<string, FocusProduct>();
  for (const item of items) {
    const code = item.articleCode.trim();
    if (!code) continue;
    const existing = products.get(code);
    if (existing) {
      existing.amount += item.netSalesAmount;
      existing.quantity += item.netQuantity;
      if (!existing.name && item.articleName.trim()) existing.name = item.articleName.trim();
      if (!existing.brand && item.brandName?.trim()) existing.brand = item.brandName.trim();
      continue;
    }
    products.set(code, {
      id: code,
      code,
      name: item.articleName.trim(),
      brand: item.brandName?.trim() ?? '',
      amount: item.netSalesAmount,
      quantity: item.netQuantity,
      currentAmount: 0,
      currentQuantity: 0,
    });
  }
  return products;
}

function compareFocusSales(left: FocusProduct, right: FocusProduct): number {
  if (right.amount !== left.amount) return right.amount - left.amount;
  if (right.quantity !== left.quantity) return right.quantity - left.quantity;
  return left.code.localeCompare(right.code);
}

function compareFocusQuantity(left: FocusProduct, right: FocusProduct): number {
  if (right.quantity !== left.quantity) return right.quantity - left.quantity;
  if (right.amount !== left.amount) return right.amount - left.amount;
  return left.code.localeCompare(right.code);
}
