import type { SalesAnalysisItem, SalesAnalysisPackedItems, SalesAnalysisPackedRow, SalesAnalysisPeriodResult, SalesAnalysisResult, SalesAnalysisStoreSummary } from './types';

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
  const items = new Array<SalesAnalysisItem>(rows.length);
  for (let index = 0; index < rows.length; index += 1) {
    const raw = rows[index]!;
    items[index] = Array.isArray(raw)
      ? unpackCompactPackedRow(raw, dict, storeList)
      : unpackObjectPackedRow(raw, dict, storeList);
  }
  return items;
}

function unpackCompactPackedRow(row: number[], dict: string[], storeList: SalesAnalysisStoreSummary[]): SalesAnalysisItem {
  const store = storeList[row[0] ?? 0];
  return {
    storeId: store?.businessId ?? '',
    storeLabel: store?.label ?? '',
    articleCode: packedString(dict, row[1]),
    articleName: packedString(dict, row[2]),
    brandName: packedString(dict, row[3]),
    category1: packedString(dict, row[4]),
    category1Code: packedString(dict, row[5]),
    category2: packedString(dict, row[6]),
    category2Code: packedString(dict, row[7]),
    category3: packedString(dict, row[8]),
    category3Code: packedString(dict, row[9]),
    category4: packedString(dict, row[10]),
    category4Code: packedString(dict, row[11]),
    category5: packedString(dict, row[12]),
    category5Code: packedString(dict, row[13]),
    transactionCount: row[14] ?? 0,
    saleQuantity: row[15] ?? 0,
    saleAmount: row[16] ?? 0,
    returnQuantity: row[17] ?? 0,
    returnTransactionCount: row[18] ?? 0,
    returnAmount: row[19] ?? 0,
    netQuantity: row[20] ?? 0,
    netSalesAmount: row[21] ?? 0,
  };
}

function unpackObjectPackedRow(row: SalesAnalysisPackedRow, dict: string[], storeList: SalesAnalysisStoreSummary[]): SalesAnalysisItem {
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
  if (view === 'stores' || view === 'weekly' || view === 'overview') return [];
  if (view === 'products') return ['current'];
  if (view === 'focus') return ['current', 'yearAgoNext'];
  const keys = ['current', 'previous', 'previous2', 'yearAgo', ...rankingKeys];
  return [...new Set(keys.filter(Boolean))];
}
