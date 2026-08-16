import type { SalesAnalysisItem, SalesAnalysisPackedItems, SalesAnalysisPeriodResult, SalesAnalysisResult, SalesAnalysisStoreSummary } from './types';

export function needsSalesAnalysisItemHydration(result: SalesAnalysisResult): boolean {
  return (result.periods ?? []).some((period) => (period.itemCount ?? 0) > (period.items?.length ?? 0));
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
