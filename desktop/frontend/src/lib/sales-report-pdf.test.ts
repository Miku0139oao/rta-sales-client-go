import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  buildSalesAnalysisPDF,
  listSuccessfulReportStores,
  salesAnalysisPDFFilename,
} from './sales-report-pdf';
import type { SalesAnalysisItem, SalesAnalysisResult, SalesAnalysisTotals } from './types';

function itemsFor(storeId: string): SalesAnalysisItem[] {
  return Array.from({ length: 90 }, (_, index) => ({
    storeId,
    storeLabel: `${storeId} - Tai Wai`,
    category1: '健康與美容', category1Code: 'A',
    category2: `商品類別 ${index % 6 + 1}`, category2Code: `A0${index % 6 + 1}`,
    category3: '商品種類', category3Code: 'A0101',
    category4: '四級類目', category4Code: 'A010101',
    category5: '小分類', category5Code: 'A01010101',
    articleCode: `${storeId}${String(index + 1).padStart(3, '0')}`,
    articleName: `測試商品 ${index + 1}`,
    brandName: `品牌 ${index % 4 + 1}`,
    transactionCount: index + 1,
    saleQuantity: index + 4,
    saleAmount: (index + 4) * 92,
    returnTransactionCount: 0,
    returnQuantity: 0,
    returnAmount: 0,
    netQuantity: index + 4,
    netSalesAmount: (index + 4) * 92,
  }));
}

function totalsFor(items: SalesAnalysisItem[]): SalesAnalysisTotals {
  const netSalesAmount = items.reduce((sum, item) => sum + item.netSalesAmount, 0);
  const netQuantity = items.reduce((sum, item) => sum + item.netQuantity, 0);
  return {
    saleQuantity: netQuantity, saleAmount: netSalesAmount, returnQuantity: 0, returnAmount: 0,
    netQuantity, netSalesAmount, trendNetSalesAmount: netSalesAmount,
    transactionCount: items.reduce((sum, item) => sum + item.transactionCount, 0),
  };
}

function resultFixture(): SalesAnalysisResult {
  const items107 = itemsFor('107');
  const specs: Array<[string, string, string, string, number]> = [
    ['current', '本期', '2026-08-01', '2026-08-16', 1],
    ['previous', '上期', '2026-07-01', '2026-07-16', 0.91],
    ['previous2', '前期', '2026-06-01', '2026-06-16', 0.84],
    ['yearAgo', '去年同期', '2025-08-01', '2025-08-16', 0.88],
    ['yearAgoNext', '去年下月', '2025-09-01', '2025-09-30', 1.07],
  ];
  const periods = specs.map(([key, label, from, to, scale]) => {
    const items = items107.map((item) => ({
      ...item,
      transactionCount: item.transactionCount * scale,
      saleQuantity: item.saleQuantity * scale,
      saleAmount: item.saleAmount * scale,
      returnTransactionCount: item.returnTransactionCount * scale,
      returnQuantity: item.returnQuantity * scale,
      returnAmount: item.returnAmount * scale,
      netQuantity: item.netQuantity * scale,
      netSalesAmount: item.netSalesAmount * scale,
    }));
    const totals = totalsFor(items);
    return {
      key, label, from, to, complete: true, successfulStores: 2, totals,
      stores: [
        { businessId: '107', label: '107 - Tai Wai', totals },
        { businessId: '108', label: '108 - Harbour', totals },
      ],
      items,
      issues: [],
    };
  });
  const current = periods[0]!;
  return {
    operationId: 'pdf-test', from: '2026-08-01', to: '2026-08-16', complete: true,
    selectedStores: 2, successfulStores: 2, totals: current.totals,
    stores: current.stores, items: current.items, issues: [], periods, queryDurationMs: 100,
  };
}

describe('sales analysis PDF', () => {
  it('builds an eight-page landscape store report with an embedded Chinese font', async () => {
    const fontBase64 = readFileSync(resolve(process.cwd(), 'src/lib/assets/NotoSansTC-Regular.ttf')).toString('base64');
    const pdf = await buildSalesAnalysisPDF(resultFixture(), '107', 'category2', 'zh-TW', fontBase64);
    expect(new TextDecoder().decode(pdf.slice(0, 8))).toBe('%PDF-1.3');
    expect(pdf.byteLength).toBeGreaterThan(100_000);
    const source = new TextDecoder('latin1').decode(pdf);
    expect(source.match(/\/Type \/Page\b/g)).toHaveLength(8);
    if (process.env.SALES_REPORT_QA_OUTPUT) writeFileSync(process.env.SALES_REPORT_QA_OUTPUT, pdf);
  });

  it('lists successful stores and creates stable per-store filenames', () => {
    expect(listSuccessfulReportStores(resultFixture()).map((store) => store.businessId)).toEqual(['107', '108']);
    expect(salesAnalysisPDFFilename('107', '2026-08-01', '2026-08-16')).toBe('RTA-Sales-107-20260801-20260816.pdf');
  });
});
