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
    k: periodKey,
    d: dict,
    r: items.map((item) => trimPackedValues([
      storeIndex.get(item.storeId) ?? 0,
      intern(item.articleCode),
      intern(item.articleName),
      intern(item.brandName),
      intern(item.category1),
      intern(item.category1Code),
      intern(item.category2),
      intern(item.category2Code),
      intern(item.category3),
      intern(item.category3Code),
      intern(item.category4),
      intern(item.category4Code),
      intern(item.category5),
      intern(item.category5Code),
      item.transactionCount,
      item.saleQuantity,
      item.saleAmount,
      item.returnQuantity,
      item.returnTransactionCount,
      item.returnAmount,
      item.netQuantity,
      item.netSalesAmount,
    ])),
  };
}

export function unpackSalesAnalysisItems(batch: SalesAnalysisPackedItems, stores: SalesAnalysisStoreSummary[] = []): SalesAnalysisItem[] {
  const dict = batch.d ?? batch.dict ?? [];
  const storeList = stores ?? [];
  const rows = batch.r ?? batch.rows ?? [];
  return rows.map((raw) => {
    const row = packedRowFields(raw);
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

function packedRowFields(row: SalesAnalysisPackedRow | number[]): SalesAnalysisPackedRow {
  if (!Array.isArray(row)) return row;
  const at = (index: number) => row[index] ?? 0;
  return {
    s: at(0), ac: at(1), an: at(2), br: at(3),
    c1: at(4), k1: at(5), c2: at(6), k2: at(7),
    c3: at(8), k3: at(9), c4: at(10), k4: at(11),
    c5: at(12), k5: at(13),
    t: at(14), sq: at(15), sa: at(16), rq: at(17), rt: at(18), ra: at(19), nq: at(20), ns: at(21),
  };
}

function trimPackedValues(values: number[]): number[] {
  let end = values.length;
  while (end > 3 && !values[end - 1]) end -= 1;
  return values.slice(0, end);
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
