import type { SalesAnalysisItem, SalesAnalysisPackedItems, SalesAnalysisPeriodResult, SalesAnalysisResult, SalesAnalysisStoreSummary } from './types';

export type AnalysisCategoryKey = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';

export function periodNeedsItemHydration(period: SalesAnalysisPeriodResult | undefined): boolean {
  if (!period) return false;
  if (!Array.isArray(period.items)) return true;
  return (period.itemCount ?? 0) > period.items.length;
}

export function needsSalesAnalysisItemHydration(result: SalesAnalysisResult): boolean {
  return (result.periods ?? []).some(periodNeedsItemHydration);
}

export function categoryCodeOf(item: SalesAnalysisItem, key: AnalysisCategoryKey): string {
  if (key === 'category1') return item.category1Code?.trim() ?? '';
  if (key === 'category2') return item.category2Code?.trim() ?? '';
  if (key === 'category3') return item.category3Code?.trim() ?? '';
  if (key === 'category4') return item.category4Code?.trim() ?? '';
  return item.category5Code?.trim() ?? '';
}

export function categoryNameOf(item: SalesAnalysisItem, key: AnalysisCategoryKey): string {
  return item[key]?.trim() ?? '';
}

export function categoryLabelOf(item: SalesAnalysisItem, key: AnalysisCategoryKey, uncategorized: string): string {
  const code = categoryCodeOf(item, key);
  const name = categoryNameOf(item, key);
  if (code && name && name !== code) return `${code}  ${name}`;
  return name || code || uncategorized;
}

export function itemMatchesCategorySelection(
  item: SalesAnalysisItem,
  key: AnalysisCategoryKey,
  selected: ReadonlySet<string>,
  uncategorized: string,
): boolean {
  if (selected.size === 0) return true;
  const code = categoryCodeOf(item, key);
  const name = categoryNameOf(item, key) || uncategorized;
  const label = categoryLabelOf(item, key, uncategorized);
  return selected.has(label) || selected.has(name) || (Boolean(code) && selected.has(code));
}

export function packSalesAnalysisItems(
  periodKey: string,
  items: SalesAnalysisItem[],
  stores: SalesAnalysisStoreSummary[] = [],
): SalesAnalysisPackedItems {
  const dict = [''];
  const index = new Map<string, number>([['', 0]]);
  const intern = (value?: string): number => {
    const text = (value ?? '').trim();
    if (!text) return 0;
    const existing = index.get(text);
    if (existing !== undefined) return existing;
    const next = dict.length;
    dict.push(text);
    index.set(text, next);
    return next;
  };
  const storeIndex = new Map(stores.map((store, current) => [store.businessId, current]));
  return {
    periodKey,
    dict,
    rows: items.map((item) => ({
      s: storeIndex.get(item.storeId) ?? 0,
      ac: intern(item.articleCode),
      an: intern(item.articleName),
      br: intern(item.brandName),
      c1: intern(item.category1),
      k1: intern(item.category1Code),
      c2: intern(item.category2),
      k2: intern(item.category2Code),
      c3: intern(item.category3),
      k3: intern(item.category3Code),
      c4: intern(item.category4),
      k4: intern(item.category4Code),
      c5: intern(item.category5),
      k5: intern(item.category5Code),
      t: item.transactionCount,
      sq: item.saleQuantity,
      sa: item.saleAmount,
      rq: item.returnQuantity,
      rt: item.returnTransactionCount,
      ra: item.returnAmount,
      nq: item.netQuantity,
      ns: item.netSalesAmount,
    })),
  };
}

export function unpackSalesAnalysisItems(batch: SalesAnalysisPackedItems, stores: SalesAnalysisStoreSummary[] = []): SalesAnalysisItem[] {
  const dict = batch.dict ?? [];
  const storeList = stores ?? [];
  return (batch.rows ?? []).map((row) => {
    const store = storeList[row.s];
    return {
      storeId: store?.businessId ?? '',
      storeLabel: store?.label ?? '',
      articleCode: packedString(dict, row.ac),
      articleName: packedString(dict, row.an),
      brandName: packedString(dict, row.br),
      category1: packedString(dict, row.c1),
      category1Code: packedString(dict, row.k1),
      category2: packedString(dict, row.c2),
      category2Code: packedString(dict, row.k2),
      category3: packedString(dict, row.c3),
      category3Code: packedString(dict, row.k3),
      category4: packedString(dict, row.c4),
      category4Code: packedString(dict, row.k4),
      category5: packedString(dict, row.c5),
      category5Code: packedString(dict, row.k5),
      transactionCount: row.t ?? 0,
      saleQuantity: row.sq ?? 0,
      saleAmount: row.sa ?? 0,
      returnQuantity: row.rq ?? 0,
      returnTransactionCount: row.rt ?? 0,
      returnAmount: row.ra ?? 0,
      netQuantity: row.nq ?? 0,
      netSalesAmount: row.ns ?? 0,
    };
  });
}

function packedString(dict: string[], index: number | undefined): string {
  if (!index || index < 0 || index >= dict.length) return '';
  return dict[index] ?? '';
}

export function periodItems(period: SalesAnalysisPeriodResult | undefined): SalesAnalysisItem[] {
  return period?.items ?? [];
}

export function periodKeysForView(view: 'overview' | 'focus' | 'categories' | 'products' | 'stores' | 'weekly', rankingKeys: string[] = []): string[] {
  if (view === 'stores' || view === 'weekly') return [];
  if (view === 'overview' || view === 'products') return ['current'];
  if (view === 'focus') return ['current', 'yearAgoNext'];
  const keys = ['current', 'previous', 'previous2', 'yearAgo', ...rankingKeys];
  return [...new Set(keys.filter(Boolean))];
}
