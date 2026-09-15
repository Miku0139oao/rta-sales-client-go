import type { SalesAnalysisItem } from './types';

export const ANALYSIS_SEARCH_DEBOUNCE_MS = 150;

const haystacks = new WeakMap<SalesAnalysisItem, string>();

export function itemSearchHaystack(item: SalesAnalysisItem): string {
  const cached = haystacks.get(item);
  if (cached !== undefined) return cached;
  const haystack = [
    item.storeId,
    item.storeLabel,
    item.articleCode,
    item.articleName,
    item.brandName ?? '',
    item.category1,
    item.category1Code ?? '',
    item.category2,
    item.category2Code ?? '',
    item.category3,
    item.category3Code ?? '',
    item.category4,
    item.category4Code ?? '',
    item.category5,
    item.category5Code ?? '',
  ].join('\u0000').toLocaleLowerCase();
  haystacks.set(item, haystack);
  return haystack;
}

export function itemMatchesSearch(item: SalesAnalysisItem, searchTerm: string): boolean {
  const term = searchTerm.trim().toLocaleLowerCase();
  if (!term) return true;
  return itemSearchHaystack(item).includes(term);
}
