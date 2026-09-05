import type { SalesAnalysisItem, SalesAnalysisStoreSummary } from '../src/lib/types';

export const SYNTHETIC_BENCH_KIND = 'synthetic-local';

export interface SyntheticPackedFixture {
  stores: SalesAnalysisStoreSummary[];
  items: SalesAnalysisItem[];
  uniqueArticles: number;
  storeCount: number;
  itemCount: number;
}

function emptyTotals() {
  return { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 };
}

export function syntheticStores(count: number): SalesAnalysisStoreSummary[] {
  return Array.from({ length: count }, (_, index) => {
    const businessId = String(100 + index);
    return {
      businessId,
      label: `${businessId} - Store ${index}`,
      totals: emptyTotals(),
    };
  });
}

export function syntheticItems(
  count: number,
  stores: SalesAnalysisStoreSummary[],
  uniqueArticles = 1000,
): SalesAnalysisItem[] {
  const storeCount = Math.max(stores.length, 1);
  const articleCount = Math.max(uniqueArticles, 1);
  return Array.from({ length: count }, (_, index) => {
    const store = stores[index % storeCount]!;
    const article = index % articleCount;
    const code = String(100000 + article);
    const quantity = (index % 17) + 1;
    const price = 10.25 + (article % 50);
    const saleAmount = quantity * price;
    const returnQuantity = index % 23 === 0 ? 1 : 0;
    const returnAmount = returnQuantity * price;
    return {
      storeId: store.businessId,
      storeLabel: store.label,
      category1: 'HEALTH',
      category1Code: 'A',
      category2: article % 3 === 0 ? 'BEAUTY' : 'CARE',
      category2Code: article % 3 === 0 ? 'A02' : 'A03',
      category3: 'SKIN',
      category3Code: 'A0201',
      category4: 'FACE',
      category4Code: 'A020101',
      category5: '',
      category5Code: '',
      articleCode: code,
      articleName: `Item ${article}`,
      brandName: `Brand ${article % 40}`,
      transactionCount: quantity,
      saleQuantity: quantity,
      saleAmount,
      returnQuantity,
      returnTransactionCount: returnQuantity,
      returnAmount,
      netQuantity: quantity - returnQuantity,
      netSalesAmount: saleAmount - returnAmount,
    };
  });
}

export function syntheticPackedFixture(
  itemCount: number,
  storeCount: number,
  uniqueArticles = 1000,
): SyntheticPackedFixture {
  const stores = syntheticStores(storeCount);
  return {
    stores,
    items: syntheticItems(itemCount, stores, uniqueArticles),
    uniqueArticles,
    storeCount,
    itemCount,
  };
}

export function itemChecksum(items: SalesAnalysisItem[]): {
  count: number;
  netSalesAmount: number;
  netQuantity: number;
  saleAmount: number;
  returnAmount: number;
  codes: number;
} {
  let netSalesAmount = 0;
  let netQuantity = 0;
  let saleAmount = 0;
  let returnAmount = 0;
  let codes = 0;
  for (const item of items) {
    netSalesAmount += item.netSalesAmount;
    netQuantity += item.netQuantity;
    saleAmount += item.saleAmount;
    returnAmount += item.returnAmount;
    codes += item.articleCode.length;
  }
  return { count: items.length, netSalesAmount, netQuantity, saleAmount, returnAmount, codes };
}
