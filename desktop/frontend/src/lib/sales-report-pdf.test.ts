import { writeFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { defaultSalesReportFilter } from './salesReportItems';
import {
  ALL_STORES_REPORT_ID,
  addSalesReportPeriodItems,
  buildSalesAnalysisPDF,
  categoryCardRowMetrics,
  categoryRankingCardSlots,
  createSalesReportAccumulator,
  focusReportCards,
  generateSalesAnalysisPDF,
  listSuccessfulReportStores,
  prepareSalesAnalysisFont,
  salesAnalysisPDFFilename,
  salesReportAccumulatorFromMemo,
} from './sales-report-pdf';
import type { SalesAnalysisItem, SalesAnalysisResult, SalesAnalysisTotals, SalesAnalysisWeek } from './types';

function weekRow(from: string, to: string, salesTw: number, salesLw: number) {
  const values = {
    salesTw, salesLw, customersTw: 20, customersLw: 10,
    weekdaySalesTw: salesTw * 0.6, weekdaySalesLw: salesLw * 0.7,
    weekendSalesTw: salesTw * 0.4, weekendSalesLw: salesLw * 0.3,
    weekdayCustomersTw: 12, weekdayCustomersLw: 6, weekendCustomersTw: 8, weekendCustomersLw: 4,
  };
  return {
    from, to, totals: values,
    stores: [{ businessId: '107', label: '107 - Tai Wai', ...values }],
  };
}

function itemsFor(storeId: string): SalesAnalysisItem[] {
  return Array.from({ length: 90 }, (_, index) => ({
    storeId,
    storeLabel: `${storeId} - Tai Wai`,
    category1: '健康與美容', category1Code: 'A',
    category2: ['HEALTH CARE', 'PERSONAL CARE', 'BEAUTY CARE', 'BABY NEEDS', 'PAPER GOODS', 'APPAREL'][index % 6]!,
    category2Code: ['A01', 'A03', 'A02', 'B05', 'B12', 'B10'][index % 6]!,
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

async function reportFont(result = resultFixture()) {
  return prepareSalesAnalysisFont(result, 'category2', 'zh-TW');
}

describe('sales analysis PDF', () => {
  it('builds a nine-page landscape store report with an embedded Chinese font', async () => {
    const fontBase64 = await reportFont();
    expect(fontBase64.length).toBeLessThan(400_000);
    const pdf = await buildSalesAnalysisPDF(resultFixture(), '107', 'category2', 'zh-TW', fontBase64);
    expect(new TextDecoder().decode(pdf.slice(0, 8))).toBe('%PDF-1.3');
    expect(pdf.byteLength).toBeGreaterThan(5_000);
    expect(pdf.byteLength).toBeLessThan(400_000);
    const source = new TextDecoder('latin1').decode(pdf);
    expect(source.match(/\/Type \/Page\b/g)).toHaveLength(9);
    expect(source.match(/\/FontFile2\b/g)).toHaveLength(1);
    if (process.env.SALES_REPORT_QA_OUTPUT) writeFileSync(process.env.SALES_REPORT_QA_OUTPUT, pdf);
  });

  it('lists successful stores and creates stable per-store filenames', () => {
    expect(listSuccessfulReportStores(resultFixture()).map((store) => store.businessId)).toEqual(['107', '108']);
    expect(salesAnalysisPDFFilename('107', '2026-08-01', '2026-08-16')).toBe('RTA-Sales-107-20260801-20260816.pdf');
    expect(salesAnalysisPDFFilename(ALL_STORES_REPORT_ID, '2026-08-01', '2026-08-16')).toBe('RTA-Sales-all-20260801-20260816.pdf');
  });

  it('builds a combined all-stores report with a store comparison page', async () => {
    const result = resultFixture();
    const items108 = itemsFor('108').map((item) => ({ ...item, netSalesAmount: item.netSalesAmount * 0.7, netQuantity: item.netQuantity * 0.7 }));
    for (const period of result.periods ?? []) {
      const extra = items108.map((item) => ({
        ...item,
        transactionCount: item.transactionCount * (period.key === 'current' ? 1 : 0.9),
        netSalesAmount: item.netSalesAmount * (period.key === 'current' ? 1 : 0.9),
        netQuantity: item.netQuantity * (period.key === 'current' ? 1 : 0.9),
      }));
      period.items = [...(period.items ?? []), ...extra];
      period.totals = totalsFor(period.items ?? []);
      period.stores = [
        { businessId: '107', label: '107 - Tai Wai', totals: totalsFor(period.items.filter((item) => item.storeId === '107')) },
        { businessId: '108', label: '108 - Harbour', totals: totalsFor(period.items.filter((item) => item.storeId === '108')) },
      ];
    }
    result.items = result.periods?.[0]?.items ?? result.items;
    result.totals = totalsFor(result.items ?? []);
    const pdf = await buildSalesAnalysisPDF(result, ALL_STORES_REPORT_ID, 'category2', 'zh-TW', await reportFont(result));
    expect(new TextDecoder().decode(pdf.slice(0, 8))).toBe('%PDF-1.3');
    expect(new TextDecoder('latin1').decode(pdf).match(/\/Type \/Page\b/g)).toHaveLength(10);
  });

  it('keeps a three-category store on nine pages without placeholder cards', async () => {
    const result = resultFixture();
    for (const period of result.periods ?? []) {
      period.items = (period.items ?? []).filter((_, index) => index % 6 < 3);
      period.totals = totalsFor(period.items ?? []);
      period.stores = period.stores.map((store) => ({ ...store, totals: period.totals }));
    }
    result.items = result.periods?.[0]?.items ?? result.items;
    result.totals = totalsFor(result.items ?? []);
    const pdf = await buildSalesAnalysisPDF(result, '107', 'category2', 'zh-TW', await reportFont(result));
    expect(new TextDecoder('latin1').decode(pdf).match(/\/Type \/Page\b/g)).toHaveLength(9);
    if (process.env.SALES_REPORT_3CAT_OUTPUT) writeFileSync(process.env.SALES_REPORT_3CAT_OUTPUT, pdf);
  });

  it('keeps every week on one compact page for a single-store report', async () => {
    const result = resultFixture();
    result.weeks = [
      weekRow('2026-07-27', '2026-08-02', 711906, 854364),
      weekRow('2026-08-03', '2026-08-09', 1452746, 711906),
      weekRow('2026-08-10', '2026-08-16', 606950, 1452746),
    ];
    const pdf = await buildSalesAnalysisPDF(result, '107', 'category2', 'zh-TW', await reportFont(result));
    expect(new TextDecoder('latin1').decode(pdf).match(/\/Type \/Page\b/g)).toHaveLength(10);
    if (process.env.SALES_REPORT_WEEKLY_OUTPUT) writeFileSync(process.env.SALES_REPORT_WEEKLY_OUTPUT, pdf);
  });

  it('paginates leftover ranking categories and keeps cards on a page the same size', async () => {
    const categories = [
      ['HEALTH CARE', 'A01'], ['PERSONAL CARE', 'A03'], ['BEAUTY CARE', 'A02'],
      ['BABY NEEDS', 'B05'], ['PAPER GOODS', 'B12'], ['APPAREL', 'B10'],
      ['JUICE & DRINK', 'E08'], ['_OBSOLETE___', 'B20'], ['HEALTH-FREE GIFT', 'A04'],
      ['PC-FREE GIFT', 'A07'], ['SANPRO-FREE GIFT', 'A08'], ['BABY-FREE GIFT', 'B19'],
    ];
    const items: SalesAnalysisItem[] = categories.flatMap(([name, code], groupIndex) => {
      const shortCard = code === 'E08' || code === 'B20';
      const itemCount = shortCard ? 2 : 16;
      const unitAmount = groupIndex < 6 ? 1000 - groupIndex * 50 : shortCard ? 9 : 0;
      return Array.from({ length: itemCount }, (_, index) => ({
        storeId: '107', storeLabel: '107 - Tai Wai',
        category1: '測試', category1Code: 'T',
        category2: name, category2Code: code,
        category3: '商品種類', category3Code: 'T01',
        category4: '四級類目', category4Code: 'T0101',
        category5: '小分類', category5Code: 'T010101',
        articleCode: `${code}-${index + 1}`,
        articleName: `${name} ${index + 1}`,
        brandName: '品牌',
        transactionCount: 1,
        saleQuantity: shortCard ? 1 : 2,
        saleAmount: unitAmount,
        returnTransactionCount: 0,
        returnQuantity: 0,
        returnAmount: 0,
        netQuantity: shortCard ? 1 : 2,
        netSalesAmount: unitAmount,
      }));
    });
    const result = resultFixture();
    for (const period of result.periods ?? []) {
      period.items = items;
      period.totals = totalsFor(items);
      period.stores = period.stores.map((store) => ({ ...store, totals: period.totals }));
    }
    result.items = items;
    result.totals = totalsFor(items);
    const pdf = await buildSalesAnalysisPDF(result, '107', 'category2', 'zh-TW', await reportFont(result));
    expect(new TextDecoder('latin1').decode(pdf).match(/\/Type \/Page\b/g)?.length).toBeGreaterThan(9);
    if (process.env.SALES_REPORT_MIXED_OUTPUT) writeFileSync(process.env.SALES_REPORT_MIXED_OUTPUT, pdf);
  });

  it('builds a combined report from per-store totals without keeping every article', async () => {
    const result = resultFixture();
    const fontBase64 = await reportFont(result);
    const accumulator = createSalesReportAccumulator();
    for (const period of result.periods ?? []) {
      addSalesReportPeriodItems(accumulator, period.key, period.items ?? [], defaultSalesReportFilter(), 'category2');
    }
    const slim = {
      ...result,
      items: undefined,
      periods: (result.periods ?? []).map((period) => ({ ...period, items: undefined })),
    };
    const pdf = await generateSalesAnalysisPDF(slim, ALL_STORES_REPORT_ID, 'category2', 'zh-TW', defaultSalesReportFilter(), fontBase64, accumulator);
    expect(new TextDecoder().decode(pdf.slice(0, 8))).toBe('%PDF-1.3');
    expect(new TextDecoder('latin1').decode(pdf).match(/\/Type \/Page\b/g)).toHaveLength(10);
    const fromMemo = salesReportAccumulatorFromMemo({
      periods: [{
        key: 'current',
        topAmount: [{ id: 'x', code: 'x', name: '測試', amount: 10, quantity: 1 }],
        amountGroups: [{ id: 'A02', code: 'A02', name: 'BEAUTY CARE', amount: 10, quantity: 1, items: [{ id: 'x', code: 'x', name: '測試', amount: 10, quantity: 1 }] }],
      }],
    });
    expect(fromMemo.periods.get('current')?.products.get('x')?.amount).toBe(10);
  });

  it('builds a per-store report when period store summaries are missing', async () => {
    const result = resultFixture();
    result.weeks = [{
      from: '2026-08-03', to: '2026-08-09',
      totals: {
        salesTw: 200, salesLw: 100, customersTw: 20, customersLw: 10,
        weekdaySalesTw: 120, weekdaySalesLw: 70, weekendSalesTw: 80, weekendSalesLw: 30,
        weekdayCustomersTw: 12, weekdayCustomersLw: 6, weekendCustomersTw: 8, weekendCustomersLw: 4,
      },
    } as SalesAnalysisWeek];
    for (const period of result.periods ?? []) {
      delete (period as { stores?: unknown }).stores;
    }
    const pdf = await buildSalesAnalysisPDF(result, '107', 'category2', 'zh-TW', await reportFont(result));
    expect(new TextDecoder().decode(pdf.slice(0, 8))).toBe('%PDF-1.3');
  });

  it('fills the ranking page with only the categories that exist', () => {
    expect(categoryRankingCardSlots(0)).toEqual([]);
    const three = categoryRankingCardSlots(3);
    expect(three).toHaveLength(3);
    expect(three.every((slot) => slot.y === 30 && slot.height === 166)).toBe(true);
    expect(three[0]?.x).toBe(10);
    expect(Math.round((three[2]!.x + three[2]!.width) * 10) / 10).toBe(287);

    const five = categoryRankingCardSlots(5);
    expect(five).toHaveLength(5);
    expect(five.slice(0, 3).every((slot) => slot.y === 30)).toBe(true);
    expect(five.slice(3).every((slot) => slot.y > 30 && slot.width > five[0]!.width)).toBe(true);

    const six = categoryRankingCardSlots(6);
    expect(six).toHaveLength(6);
    expect(six.filter((slot) => slot.y === 30)).toHaveLength(3);
    expect(six.filter((slot) => slot.y > 30)).toHaveLength(3);
    expect(new Set(six.slice(0, 3).map((slot) => slot.height)).size).toBe(1);
    expect(new Set(six.slice(3).map((slot) => slot.height)).size).toBe(1);
    expect(six[0]?.height).toBe(six[3]?.height);
    expect(categoryCardRowMetrics(166).limit).toBeGreaterThan(15);
    expect(categoryCardRowMetrics(81).limit).toBeGreaterThanOrEqual(15);
  });

  it('keeps catalog focus cards named and does not reserve Health/Skin/PC slots', () => {
    const named = (id: string, name: string) => ({
      id, prefix: '', name, sales: [], quantity: [],
    });
    const four = [named('g1', 'Alpha'), named('g2', 'Beta'), named('g3', 'Gamma'), named('g4', 'Delta')];
    expect(focusReportCards(four, true).map((group) => group?.name)).toEqual(['Alpha', 'Beta', 'Gamma', 'Delta']);
    expect(focusReportCards([named('g1', 'Alpha')], true)).toHaveLength(1);
    expect(focusReportCards([], true)).toEqual([]);
    expect(focusReportCards([], false).map((group) => group?.id)).toEqual([undefined, undefined, undefined]);
    const fromMemo = salesReportAccumulatorFromMemo({
      periods: [{
        key: 'yearAgoNext',
        focusCatalog: true,
        focusGroups: [],
      }],
    });
    expect(fromMemo.periods.get('yearAgoNext')).toMatchObject({ focusCatalog: true, focusGroups: [] });
  });

  it('paginates catalog focus groups and stays empty when none match', async () => {
    const result = resultFixture();
    const fontBase64 = await reportFont(result);
    const named = (id: string, name: string) => ({
      id, prefix: '', name,
      sales: [{ id: 'x', code: 'x', name: 'Item', brand: '', amount: 1, quantity: 1, currentAmount: 0, currentQuantity: 0 }],
      quantity: [],
    });
    const withGroups = async (groups: ReturnType<typeof named>[], catalog: boolean) => {
      const accumulator = createSalesReportAccumulator();
      for (const period of result.periods ?? []) {
        addSalesReportPeriodItems(accumulator, period.key, period.items ?? [], defaultSalesReportFilter(), 'category2');
      }
      const next = accumulator.periods.get('yearAgoNext');
      if (next) {
        next.focusCatalog = catalog;
        next.focusGroups = groups;
      }
      const slim = {
        ...result,
        items: undefined,
        periods: (result.periods ?? []).map((period) => ({ ...period, items: undefined })),
      };
      return generateSalesAnalysisPDF(slim, '107', 'category2', 'en', defaultSalesReportFilter(), fontBase64, accumulator);
    };
    const fourPages = new TextDecoder('latin1').decode(await withGroups([
      named('g1', 'Alpha'), named('g2', 'Beta'), named('g3', 'Gamma'), named('g4', 'Delta'),
    ], true));
    expect(fourPages.match(/\/Type \/Page\b/g)).toHaveLength(10);
    const onePages = new TextDecoder('latin1').decode(await withGroups([named('g1', 'Alpha')], true));
    expect(onePages.match(/\/Type \/Page\b/g)).toHaveLength(9);
    const missPages = new TextDecoder('latin1').decode(await withGroups([], true));
    expect(missPages.match(/\/Type \/Page\b/g)).toHaveLength(9);
  });
});
