import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { buildFocusGroups } from './analysisFocus';
import { buildAnalysisTables } from './analysisTableViews';
import { workbookSnapshot } from './analysisTable';
import { translator } from './i18n';
import { buildSalesAnalysisAIMarkdown } from './sales-report-ai';
import {
  EXPORT_FIXTURE_FROM,
  EXPORT_FIXTURE_STORE_ID,
  EXPORT_FIXTURE_TO,
  exportFixtureFilter,
  exportFixtureResult,
  smallExportResult,
} from './sales-report-export-fixture';
import {
  FOCUS_EXPORT_LIMIT,
  FOCUS_SCREEN_LIMIT,
  PAGINATION_EXPECTED,
  PAGINATION_SUMMARY,
  SMALL_EXPORT_EXPECTED,
  paginationParityExpected,
} from './sales-report-export-expected';
import {
  buildSalesAnalysisPDF,
  categoryCardRowMetrics,
  categoryRankingCardRowLimit,
  categoryRankingCardSlots,
  categoryRankingCardsPerPage,
  prepareSalesAnalysisFont,
  salesReportIsPartial,
} from './sales-report-pdf';
import { compareParity, extractDocument, extractPdf } from './sales-report-pdf-section';
import type { SalesAnalysisItem, SalesAnalysisPeriodMemo, SalesAnalysisReportMemo } from './types';

const CACHE = resolve(process.cwd(), 'node_modules/.cache/sales-report-pdf');
const LIMITS = [16, 24, 40, 100] as const;

function periodOf(result: ReturnType<typeof smallExportResult>, key: string) {
  return result.periods?.find((period) => period.key === key);
}

function ranksFrom(items: SalesAnalysisItem[], metric: 'amount' | 'quantity') {
  return [...items].sort((left, right) => (
    metric === 'amount' ? right.netSalesAmount - left.netSalesAmount : right.netQuantity - left.netQuantity
  ) || left.articleCode.localeCompare(right.articleCode)).map((item, index) => ({
    rank: index + 1,
    code: item.articleCode,
    name: item.articleName,
    amount: item.netSalesAmount,
    quantity: item.netQuantity,
  }));
}

function categorySums(items: SalesAnalysisItem[]) {
  const grouped = new Map<string, { code: string; name: string; amount: number; quantity: number }>();
  for (const item of items) {
    const code = item.category2Code?.trim() ?? '';
    const row = grouped.get(code) ?? { code, name: item.category2, amount: 0, quantity: 0 };
    row.amount += item.netSalesAmount;
    row.quantity += item.netQuantity;
    grouped.set(code, row);
  }
  return [...grouped.values()].sort((left, right) => right.amount - left.amount || left.code.localeCompare(right.code));
}

function payloadFromMarkdown(markdown: string) {
  const match = markdown.match(/```json\n([\s\S]*?)\n```/);
  expect(match?.[1]).toBeTruthy();
  return JSON.parse(match![1]!) as {
    from: string;
    to: string;
    totals: Record<string, { netSalesAmount?: number; netQuantity?: number }>;
    topSales: Array<{ code: string; amount: number; quantity: number }>;
    topQuantity: Array<{ code: string; amount: number; quantity: number }>;
    focusGroups: Array<{ sales: Array<{ code: string; amount: number }> }>;
  };
}

function memoFromResult(result: ReturnType<typeof smallExportResult>): SalesAnalysisReportMemo {
  const current = periodOf(result, 'current')!;
  const previous = periodOf(result, 'previous')!;
  const previous2 = periodOf(result, 'previous2')!;
  const yearAgo = periodOf(result, 'yearAgo')!;
  const yearAgoNext = periodOf(result, 'yearAgoNext')!;
  const toRanked = (rows: ReturnType<typeof ranksFrom>): NonNullable<SalesAnalysisPeriodMemo['topAmount']> =>
    rows.map((row) => ({ id: row.code, code: row.code, name: row.name, amount: row.amount, quantity: row.quantity }));
  const groupsFor = (items: SalesAnalysisItem[]) => categorySums(items).map((group) => ({
    id: group.code, code: group.code, name: group.name, amount: group.amount, quantity: group.quantity,
    items: toRanked(ranksFrom(items.filter((item) => item.category2Code === group.code), 'amount')),
  }));
  const focusExport = buildFocusGroups(yearAgoNext.items ?? [], current.items ?? [], FOCUS_EXPORT_LIMIT)
    .find((group) => group.id === 'health');
  return {
    periods: [
      { key: 'current', totals: current.totals, topAmount: toRanked(ranksFrom(current.items ?? [], 'amount')), topQuantity: toRanked(ranksFrom(current.items ?? [], 'quantity')), amountGroups: groupsFor(current.items ?? []) },
      { key: 'previous', totals: previous.totals, amountGroups: groupsFor(previous.items ?? []) },
      { key: 'previous2', totals: previous2.totals, amountGroups: groupsFor(previous2.items ?? []) },
      { key: 'yearAgo', totals: yearAgo.totals, amountGroups: groupsFor(yearAgo.items ?? []) },
      {
        key: 'yearAgoNext',
        totals: yearAgoNext.totals,
        focusGroups: focusExport ? [{ id: focusExport.id, prefix: focusExport.prefix, name: '保健', sales: focusExport.sales, quantity: focusExport.quantity }] : [],
      },
    ],
  };
}

describe('cross-format export identities', () => {
  it('sizes category continuation from six-card height at ranking 16', () => {
    const sixHeight = categoryRankingCardSlots(6)[0]!.height;
    const threeHeight = categoryRankingCardSlots(3)[0]!.height;
    expect(sixHeight).toBeLessThan(threeHeight);
    expect(categoryRankingCardsPerPage(16)).toBe(6);
    expect(categoryRankingCardsPerPage(24)).toBe(3);
    expect(categoryRankingCardRowLimit(16)).toBe(categoryCardRowMetrics(sixHeight).limit);
    expect(categoryRankingCardRowLimit(24)).toBe(categoryCardRowMetrics(threeHeight).limit);
    expect(categoryRankingCardRowLimit(40)).toBe(categoryCardRowMetrics(threeHeight).limit);
    expect(categoryRankingCardRowLimit(16)).toBeLessThan(categoryCardRowMetrics(threeHeight).limit);
  });

  it('compares screen/Excel/PDF/AI actuals to an independent numeric table', async () => {
    const expected = SMALL_EXPORT_EXPECTED;
    const result = smallExportResult();
    const current = periodOf(result, 'current')!;
    const previous = periodOf(result, 'previous')!;
    const previous2 = periodOf(result, 'previous2')!;
    const yearAgo = periodOf(result, 'yearAgo')!;
    expect(current.totals.netSalesAmount).toBe(expected.current.netSalesAmount);
    expect(previous.totals.netSalesAmount).toBe(expected.previous.netSalesAmount);
    expect(yearAgo.totals.netSalesAmount).toBe(expected.yearAgo.netSalesAmount);
    expect(previous.totals.netSalesAmount).not.toBe(0);
    expect(yearAgo.totals.netSalesAmount).not.toBe(0);

    const currentItems = current.items ?? [];
    const yearAgoNextItems = periodOf(result, 'yearAgoNext')?.items ?? [];
    const actualSales = ranksFrom(currentItems, 'amount');
    const actualQuantity = ranksFrom(currentItems, 'quantity');
    const focusScreen = buildFocusGroups(yearAgoNextItems, currentItems, FOCUS_SCREEN_LIMIT);
    const focusExport = buildFocusGroups(yearAgoNextItems, currentItems, FOCUS_EXPORT_LIMIT);
    const healthScreen = focusScreen.find((group) => group.id === 'health');
    const healthExport = focusExport.find((group) => group.id === 'health');
    expect(actualSales.map((row) => row.code)).toEqual(expected.topSales.map((row) => row.code));
    expect(actualSales.map((row) => row.amount)).toEqual(expected.topSales.map((row) => row.amount));
    expect(actualQuantity.map((row) => row.code)).toEqual(expected.topQuantity.map((row) => row.code));
    expect(actualSales.at(-1)).toMatchObject({
      rank: 16, code: '0100012', amount: 100, quantity: 10,
    });
    expect(healthScreen?.sales.map((row) => row.code)).toEqual(expected.focusScreenHealth.map((row) => row.code));
    expect(healthExport?.sales.map((row) => row.code)).toEqual(expected.focusExportHealth.map((row) => row.code));
    expect(healthScreen?.sales).toHaveLength(FOCUS_SCREEN_LIMIT);
    expect(healthExport?.sales).toHaveLength(FOCUS_EXPORT_LIMIT);
    expect(healthScreen?.sales[9]?.code).toBe('0100010');
    expect(healthExport?.sales.some((row) => row.code === '0100010')).toBe(false);

    const t = translator('zh-TW');
    const previousCats = categorySums(previous.items ?? []);
    const previous2Cats = categorySums(previous2.items ?? []);
    const yearAgoCats = categorySums(yearAgo.items ?? []);
    const tables = buildAnalysisTables({
      items: currentItems,
      performance: [
        {
          label: t('analysis.netSales'),
          current: current.totals.netSalesAmount,
          previous: previous.totals.netSalesAmount,
          yearAgo: yearAgo.totals.netSalesAmount,
          format: 'money',
        },
        {
          label: t('analysis.netQuantity'),
          current: current.totals.netQuantity,
          previous: previous.totals.netQuantity,
          yearAgo: yearAgo.totals.netQuantity,
          format: 'number',
        },
      ],
      categories: categorySums(currentItems).map((group) => ({
        name: group.name,
        code: group.code,
        current: group.amount,
        previous: previousCats.find((row) => row.code === group.code)?.amount ?? 0,
        previous2: previous2Cats.find((row) => row.code === group.code)?.amount ?? 0,
        yearAgo: yearAgoCats.find((row) => row.code === group.code)?.amount ?? 0,
      })),
      stores: result.stores.map((store) => ({ id: store.businessId, label: store.label, current: store.totals, previous: previous.stores[0]?.totals, yearAgo: yearAgo.stores[0]?.totals })),
      periods: result.periods ?? [],
      weekAligned: false,
      topSales: actualSales,
      topQuantity: actualQuantity,
      salesGroups: [],
      quantityGroups: [],
      focus: focusScreen,
    }, t, 'zh-TW', {});

    const performance = tables.overview.find((table) => table.id === 'performance');
    expect(performance?.rows[0]?.cells).toEqual([
      t('analysis.netSales'),
      expected.current.netSalesAmount,
      expected.previous.netSalesAmount,
      expected.yearAgo.netSalesAmount,
      expected.vsPreviousSales,
      expected.vsYearAgoSales,
    ]);
    const topSalesTable = tables.overview.find((table) => table.id === 'top-sales');
    expect(topSalesTable?.rows.map((row) => row.cells[1])).toEqual(expected.topSales.map((row) => row.code));
    expect(topSalesTable?.rows.map((row) => row.cells[3])).toEqual(expected.topSales.map((row) => row.amount));
    expect(topSalesTable?.rows.at(-1)?.cells[1]).toBe('0100012');
    const categories = tables.categories.find((table) => table.id === 'categories');
    expect(categories?.rows[0]?.cells[0]).toBe('保健護理');
    expect(categories?.rows[0]?.cells[1]).toBe(7800);
    expect(categories?.rows[0]?.cells[2]).toBe(3900);
    expect(categories?.rows[0]?.cells[4]).toBe(6240);
    const focusSales = tables.focus.find((table) => table.id === 'focus-sales');
    expect(healthScreen?.sales).toHaveLength(FOCUS_SCREEN_LIMIT);
    expect(focusSales?.rows).toHaveLength(14);

    const snapshot = workbookSnapshot(tables.overview, [
      `${result.from} — ${result.to}`,
      ...result.periods!.map((period) => `${period.label}: ${period.from} — ${period.to}`),
    ], 'RTA-Sales-107-small.xlsx');
    expect(snapshot.sheets[0]?.name).toBe('銷售表現');
    expect(snapshot.sheets[0]?.rows[0]?.slice(1, 4)).toEqual([
      expected.current.netSalesAmount, expected.previous.netSalesAmount, expected.yearAgo.netSalesAmount,
    ]);
    expect(snapshot.sheets[1]?.name).toBe('銷售額 Top 16');
    expect(snapshot.sheets[1]?.rows[0]?.[1]).toBe('0200001');
    expect(snapshot.sheets[1]?.rows[0]?.[3]).toBe(2000);
    expect(snapshot.sheets[1]?.rows.at(-1)?.[1]).toBe('0100012');
    mkdirSync(CACHE, { recursive: true });
    writeFileSync(resolve(CACHE, 'workbook-snapshot.json'), JSON.stringify({
      ...snapshot,
      note: 'synthetic/local Excel snapshot; not real RTA speed',
    }));

    const markdown = buildSalesAnalysisAIMarkdown({
      locale: 'zh-TW',
      storeId: EXPORT_FIXTURE_STORE_ID,
      storeLabel: '107 大圍合成門店',
      from: EXPORT_FIXTURE_FROM,
      to: EXPORT_FIXTURE_TO,
      categoryLevel: 'category2',
      filter: exportFixtureFilter,
      rankingLimit: 16,
      base: memoFromResult(result),
      periodMeta: (result.periods ?? []).map((period) => ({
        key: period.key, label: period.label, from: period.from, to: period.to,
      })),
      groups: [],
    });
    const payload = payloadFromMarkdown(markdown);
    expect(payload.from).toBe(expected.from);
    expect(payload.totals.current?.netSalesAmount).toBe(expected.current.netSalesAmount);
    expect(payload.totals.previous?.netSalesAmount).toBe(expected.previous.netSalesAmount);
    expect(payload.totals.yearAgo?.netSalesAmount).toBe(expected.yearAgo.netSalesAmount);
    expect(payload.topSales.map((item) => item.code)).toEqual(expected.topSales.map((item) => item.code));
    expect(payload.topSales.map((item) => item.amount)).toEqual(expected.topSales.map((item) => item.amount));
    expect(payload.topQuantity.map((item) => item.code)).toEqual(expected.topQuantity.map((item) => item.code));
    expect(payload.focusGroups[0]?.sales).toHaveLength(FOCUS_EXPORT_LIMIT);
    expect(payload.focusGroups[0]?.sales.map((item) => item.code)).toEqual(expected.focusExportHealth.map((item) => item.code));
    expect(markdown).toContain('Top 商品只是前 16 名');
    expect(markdown).toContain(expected.formats.aiMoney(expected.current.netSalesAmount));
    expect(markdown).toContain(expected.formats.aiMoney(expected.previous.netSalesAmount));
    expect(markdown).toContain(expected.formats.aiPercent(expected.current.netSalesAmount, expected.previous.netSalesAmount));
    expect(markdown).toContain(expected.formats.aiPercent(expected.current.netSalesAmount, expected.yearAgo.netSalesAmount));

    const font = await prepareSalesAnalysisFont(result, 'category2', 'zh-TW');
    const pdf = await buildSalesAnalysisPDF(
      result, EXPORT_FIXTURE_STORE_ID, 'category2', 'zh-TW', font, exportFixtureFilter, undefined, [], 16,
    );
    expect(new TextDecoder().decode(pdf.slice(0, 8))).toBe('%PDF-1.3');
    mkdirSync(CACHE, { recursive: true });
    const smallPdf = resolve(CACHE, 'ranking-small.pdf');
    writeFileSync(smallPdf, pdf);
    const pages = await extractPdf(smallPdf, readFileSync);
    const document = extractDocument(pages);
    expect(document.summaryPages[0]?.summary.netSales).toEqual({
      current: expected.current.netSalesAmount,
      previous: expected.previous.netSalesAmount,
      yearAgo: expected.yearAgo.netSalesAmount,
      vsPrevious: expected.formats.pdfPercent(expected.current.netSalesAmount, expected.previous.netSalesAmount),
      vsYearAgo: expected.formats.pdfPercent(expected.current.netSalesAmount, expected.yearAgo.netSalesAmount),
    });
    expect(document.overall.sales.map((row) => row.code)).toEqual(expected.topSales.map((row) => row.code));
    for (const [actual, wanted] of [[document.overall.sales, expected.topSales], [document.overall.quantity, expected.topQuantity]] as const) {
      expect(actual.map(row => [row.rank, row.code, row.compactAmount, row.quantity])).toEqual(wanted.map(row => [row.rank, row.code, expected.formats.pdfCompactMoney(row.amount), row.quantity]));
    }
    expect(document.overall.sales.at(-1)?.rank).toBe(16);
    expect(document.overall.sales.at(-1)?.code).toBe('0100012');
    expect(document.overall.quantity.map((row) => row.code)).toEqual(expected.topQuantity.map((row) => row.code));
    const healthFocus = document.focus.find((group) => group.name === '保健');
    expect(healthFocus?.sales.map((row) => row.code)).toEqual(expected.focusExportHealth.map((row) => row.code));
    expect(healthFocus?.sales).toHaveLength(FOCUS_EXPORT_LIMIT);
    for (const rows of [healthFocus?.sales, healthFocus?.quantity]) {
      expect(rows?.map(row => [row.code, row.compactAmount, row.quantity])).toEqual(expected.focusExportHealth.map(row => [row.code, expected.formats.pdfCompactMoney(row.amount), Math.round(row.quantity)]));
    }
    const currentCategory = document.categorySales[0]?.cards.find((card) => card.code === 'A01');
    expect(currentCategory?.lastCode).toBe('0100012');
    expect(currentCategory?.lastRank).toBe(12);

    const filteredPdf = await buildSalesAnalysisPDF(
      result, EXPORT_FIXTURE_STORE_ID, 'category2', 'zh-TW', font,
      { ...exportFixtureFilter, facets: { category2: ['A01'] } }, undefined, [], 16,
    );
    writeFileSync(resolve(CACHE, 'ranking-small-filter.pdf'), filteredPdf);
    const filtered = extractDocument(await extractPdf(resolve(CACHE, 'ranking-small-filter.pdf'), readFileSync));
    expect(filtered.overall.sales.map((row) => row.code)).toEqual(expected.filterHealth.topSales.map((row) => row.code));
    expect(filtered.overall.sales.at(-1)?.code).toBe(expected.filterHealth.last.code);
    expect(filtered.overall.sales.every((row) => row.code.startsWith('010'))).toBe(true);
  }, 120_000);

  it('writes synthetic ranking PDFs for 16/24/40/100 and checks last-row identities', async () => {
    const result = exportFixtureResult();
    expect(salesReportIsPartial(result)).toBe(true);
    const font = await prepareSalesAnalysisFont(result, 'category2', 'zh-TW');
    mkdirSync(CACHE, { recursive: true });
    const reports: Array<{ limit: number; pages: number; bytes: number; file: string }> = [];
    for (const limit of LIMITS) {
      const expected = PAGINATION_EXPECTED[limit];
      const pdf = await buildSalesAnalysisPDF(
        result, EXPORT_FIXTURE_STORE_ID, 'category2', 'zh-TW', font, exportFixtureFilter, undefined, [], limit,
      );
      const file = `ranking-${limit}.pdf`;
      const pdfPath = resolve(CACHE, file);
      writeFileSync(pdfPath, pdf);
      const pages = await extractPdf(pdfPath, readFileSync);
      writeFileSync(resolve(CACHE, `extract-${limit}.json`), JSON.stringify({
        synthetic: true, local: true, note: 'synthetic/local text extraction; not real RTA speed', pages,
      }, null, 2));
      const document = extractDocument(pages);
      const parity = compareParity(document, paginationParityExpected(limit), { requirePartial: true });
      expect(parity.errors).toEqual([]);
      const summary = document.summaryPages[0]?.summary;
      expect(summary?.hasPartial).toBe(true);
      expect(summary?.netSales?.current).toBe(PAGINATION_SUMMARY.netSales.current);
      expect(summary?.netSales?.previous).toBe(PAGINATION_SUMMARY.netSales.previous);
      expect(summary?.netSales?.yearAgo).toBe(PAGINATION_SUMMARY.netSales.yearAgo);
      expect(summary?.netSales?.vsPrevious).toBe(PAGINATION_SUMMARY.vsPrevious);
      expect(summary?.netQuantity?.current).toBe(PAGINATION_SUMMARY.netQuantity.current);
      expect(document.overall.sales[0]?.code).toBe(PAGINATION_SUMMARY.firstSalesCode);
      const lastSales = document.overall.sales.find((row) => row.rank === expected.lastSales.rank);
      expect(lastSales?.code).toBe(expected.lastSales.code);
      expect(lastSales?.quantity).toBe(expected.lastSales.quantity);
      const lastQty = document.overall.quantity.find((row) => row.rank === expected.lastQuantitySample.rank);
      expect(lastQty?.code).toBe(expected.lastQuantitySample.code);
      if (expected.categoryContinuationStart > 0) {
        const continuation = document.categorySales.some((page) => page.cards.some((card) =>
          card.rows.some((row) => row.rank === expected.lastSales.rank && row.code === expected.categoryContinuationCode)));
        expect(continuation).toBe(true);
      }
      writeFileSync(resolve(CACHE, `identities-${limit}.json`), JSON.stringify({
        synthetic: true,
        local: true,
        note: 'independent expected last-row identities; not real RTA speed',
        rankingLimit: limit,
        lastSales: expected.lastSales,
        lastQuantitySample: expected.lastQuantitySample,
        continuation: {
          cardsPerPage: categoryRankingCardsPerPage(limit),
          rowsPerCard: categoryRankingCardRowLimit(limit),
          overallStart: expected.overallContinuationStart,
          categoryStart: expected.categoryContinuationStart,
        },
      }, null, 2));
      reports.push({ limit, pages: pages.length, bytes: pdf.byteLength, file });
    }
    writeFileSync(resolve(CACHE, 'manifest.json'), JSON.stringify({
      synthetic: true,
      local: true,
      note: 'synthetic/local PDF cache; not real RTA speed',
      generatedAt: new Date().toISOString(),
      reports,
    }, null, 2));
    expect(reports.map((row) => row.limit)).toEqual([...LIMITS]);
    expect(reports[3]!.pages).toBeGreaterThan(reports[0]!.pages);
  }, 180_000);
});
