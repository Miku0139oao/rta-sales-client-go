import type { jsPDF } from 'jspdf';
import notoSansTCURL from './assets/NotoSansTC-Regular.ttf?url';
import { buildFocusGroups, FOCUS_GROUP_PREFIXES, type FocusGroup, type FocusProduct } from './analysisFocus';
import {
  defaultSalesReportFilter,
  includeInSalesReport,
  type SalesReportFilter,
} from './salesReportItems';
import { weeklySegmentRows } from './storeSegment';
import { DEFAULT_RANKING_LIMIT, normalizeRankingLimit } from './settings';
import { AppError } from './types';

export { DEFAULT_RANKING_LIMIT, normalizeRankingLimit };
import type {
  Locale,
  SalesAnalysisItem,
  SalesAnalysisPeriodResult,
  SalesAnalysisResult,
  SalesAnalysisStore,
  SalesAnalysisTotals,
  SalesAnalysisPeriodMemo,
  SalesAnalysisReportMemo,
  SalesAnalysisWeek,
  SalesAnalysisWeekStore,
} from './types';

export type SalesReportCategoryLevel = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';
type Metric = 'amount' | 'quantity';
type RGB = [number, number, number];

interface StorePeriod {
  key: string;
  label: string;
  from: string;
  to: string;
  totals: SalesAnalysisTotals;
  items: SalesAnalysisItem[];
  amountGroups?: CategoryGroup[];
  quantityGroups?: CategoryGroup[];
  topAmount?: RankedItem[];
  topQuantity?: RankedItem[];
  focusGroups?: FocusGroup[];
  focusCatalog?: boolean;
}

type AccProduct = RankedItem & { category2Code: string; category3Code: string; category4Code: string };

interface PeriodAccumulator {
  products: Map<string, AccProduct>;
  categories: Map<string, { id: string; code: string; name: string; amount: number; quantity: number; products: Map<string, RankedItem> }>;
  totals?: SalesAnalysisTotals;
  focusGroups?: FocusGroup[];
  focusCatalog?: boolean;
}

export interface SalesReportAccumulator {
  periods: Map<string, PeriodAccumulator>;
}

export interface SalesReportScope {
  groupId: string;
  groupName: string;
  itemCodes: string[];
}

export interface SalesReportChapter {
  scope: SalesReportScope;
  accumulator?: SalesReportAccumulator;
}

interface RankedItem {
  id: string;
  code: string;
  name: string;
  brand: string;
  amount: number;
  quantity: number;
}

interface CategoryGroup {
  id: string;
  code: string;
  name: string;
  amount: number;
  quantity: number;
  items: RankedItem[];
}

interface Labels {
  title: string;
  summary: string;
  period: string;
  generated: string;
  netSales: string;
  netQuantity: string;
  transactions: string;
  basket: string;
  comparison: string;
  metric: string;
  current: string;
  previous: string;
  yearAgo: string;
  vsPrevious: string;
  vsYearAgo: string;
  focusTitle: string;
  focusHealth: string;
  focusSkin: string;
  focusPC: string;
  focusSales: string;
  focusQuantity: string;
  categoryPerformance: string;
  category: string;
  topSales: string;
  topQuantity: string;
  salesRanking: string;
  quantityRanking: string;
  product: string;
  amount: string;
  quantity: string;
  uncategorized: string;
  allStores: string;
  localTotal: string;
  touristTotal: string;
  storeComparison: string;
  store: string;
  groupSummary: string;
  group: string;
  weeklyTitle: string;
  week: string;
  thisWeek: string;
  lastWeek: string;
  variance: string;
  variancePercent: string;
  weekday: string;
  weekend: string;
  customers: string;
}

export const ALL_STORES_REPORT_ID = '__all__';

const FONT = 'NotoSansTC';
const PAGE_WIDTH = 297;
const PAGE_HEIGHT = 210;
const CONTENT_X = 10;
const CONTENT_Y = 30;
const CONTENT_WIDTH = 277;
const CONTENT_HEIGHT = 166;
const CARD_GAP = 4;
const CATEGORY_CARDS_PER_PAGE = 6;

export function categoryRankingCardsPerPage(rankingLimit: number): number {
  return normalizeRankingLimit(rankingLimit) <= 16 ? CATEGORY_CARDS_PER_PAGE : 3;
}

export function rankingPanelCapacity(height: number, rankingLimit: number): number {
  const compact = normalizeRankingLimit(rankingLimit) > 16;
  const headerYOffset = 19;
  const firstOffset = compact ? 6 : 8;
  const row = compact ? 5.35 : 8.55;
  const bottomPad = compact ? 3.5 : 3;
  return Math.max(1, Math.floor((height - headerYOffset - firstOffset - bottomPad) / row) + 1);
}
const COLORS = {
  ink: [24, 39, 58] as RGB,
  navy: [24, 55, 82] as RGB,
  teal: [41, 111, 117] as RGB,
  tealSoft: [229, 242, 241] as RGB,
  slate: [92, 108, 125] as RGB,
  line: [216, 224, 232] as RGB,
  surface: [247, 249, 251] as RGB,
  white: [255, 255, 255] as RGB,
  positive: [33, 128, 91] as RGB,
  negative: [190, 64, 62] as RGB,
};

let fontBytesPromise: Promise<Uint8Array> | undefined;

export async function prepareSalesAnalysisFont(
  result: SalesAnalysisResult,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  filter: SalesReportFilter = defaultSalesReportFilter(),
  scope?: SalesReportScope,
  extraScopes: SalesReportScope[] = [],
): Promise<string> {
  try {
    const [fontBytes, subsetter] = await Promise.all([loadFontBytes(), import('./subsetReportFont')]);
    try {
      const glyphs = collectReportGlyphs(result, categoryLevel, locale, filter, scope, extraScopes);
      return bytesToBase64(await subsetter.subsetReportFont(fontBytes, glyphs));
    } finally {
      releaseLoadedReportFont();
      subsetter.releaseSubsetRuntime();
    }
  } catch (error) {
    if (error instanceof AppError) throw error;
    throw new AppError('pdf_font', error instanceof Error ? error.message : String(error));
  }
}

export async function prepareSalesAnalysisFontFromText(text: string, locale: Locale = 'zh-TW'): Promise<string> {
  try {
    const [fontBytes, subsetter] = await Promise.all([loadFontBytes(), import('./subsetReportFont')]);
    try {
      const glyphs = new Set(REQUIRED_GLYPHS);
      const add = (value: string | undefined) => {
        if (!value) return;
        for (const character of value) glyphs.add(character);
      };
      add(text);
      add('RTA SALES');
      add('RTA Sales Analysis');
      add('Page');
      for (const limit of [16, 24, 32] as const) {
        for (const value of Object.values(reportLabels(locale, limit))) add(value);
        for (const value of Object.values(reportLabels(locale === 'en' ? 'zh-TW' : 'en', limit))) add(value);
      }
      return bytesToBase64(await subsetter.subsetReportFont(fontBytes, [...glyphs].join('')));
    } finally {
      releaseLoadedReportFont();
      subsetter.releaseSubsetRuntime();
    }
  } catch (error) {
    if (error instanceof AppError) throw error;
    throw new AppError('pdf_font', error instanceof Error ? error.message : String(error));
  }
}

export async function generateSalesAnalysisPDF(
  result: SalesAnalysisResult,
  storeId: string,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  filter: SalesReportFilter = defaultSalesReportFilter(),
  fontBase64?: string,
  accumulator?: SalesReportAccumulator,
  extraChapters: SalesReportChapter[] = [],
  scope?: SalesReportScope,
  rankingLimit: number = DEFAULT_RANKING_LIMIT,
): Promise<Uint8Array> {
  try {
    const extraScopes = extraChapters.map((chapter) => chapter.scope);
    const [resolvedFont, { jsPDF: PDFDocument }] = await Promise.all([
      fontBase64 ? Promise.resolve(fontBase64) : prepareSalesAnalysisFont(result, categoryLevel, locale, filter, scope, extraScopes),
      import('jspdf'),
    ]);
    return renderSalesAnalysisPDF(
      result, storeId, categoryLevel, locale, resolvedFont, PDFDocument, filter, accumulator, scope, extraChapters,
      rankingLimit,
    );
  } catch (error) {
    if (error instanceof AppError) throw error;
    throw new AppError('pdf_failed', error instanceof Error ? error.message : String(error));
  }
}

export async function buildSalesAnalysisPDF(
  result: SalesAnalysisResult,
  storeId: string,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  fontBase64: string,
  filter: SalesReportFilter = defaultSalesReportFilter(),
  scope?: SalesReportScope,
  extraChapters: SalesReportScope[] = [],
  rankingLimit: number = DEFAULT_RANKING_LIMIT,
): Promise<Uint8Array> {
  try {
    const { jsPDF: PDFDocument } = await import('jspdf');
    return renderSalesAnalysisPDF(
      result, storeId, categoryLevel, locale, fontBase64, PDFDocument, filter, undefined, scope,
      extraChapters.map((chapter) => ({ scope: chapter })),
      rankingLimit,
    );
  } catch (error) {
    if (error instanceof AppError) throw error;
    throw new AppError('pdf_failed', error instanceof Error ? error.message : String(error));
  }
}

function renderSalesAnalysisPDF(
  result: SalesAnalysisResult,
  storeId: string,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  fontBase64: string,
  PDFDocument: typeof import('jspdf').jsPDF,
  filter: SalesReportFilter = defaultSalesReportFilter(),
  accumulator?: SalesReportAccumulator,
  scope?: SalesReportScope,
  extraChapters: SalesReportChapter[] = [],
  rankingLimit: number = DEFAULT_RANKING_LIMIT,
): Uint8Array {
  const limit = normalizeRankingLimit(rankingLimit);
  const labels = reportLabels(locale, limit);
  const combined = isAllStoresReport(storeId);
  const stores = listSuccessfulReportStores(result);
  const store = stores.find((candidate) => candidate.businessId === storeId);
  const headerId = combined ? 'ALL' : storeId;
  const baseStoreLabel = combined ? `${labels.allStores} (${stores.length})` : (store?.label.trim() || storeId);
  const firstPeriods = storePeriods(result, storeId, filter, categoryLevel, accumulator, labels.uncategorized, scope);
  const firstCurrent = periodByKey(firstPeriods, 'current') ?? firstPeriods[0];
  if (!firstCurrent) throw new AppError('pdf_failed', 'The selected store has no current sales period');

  const chapterNames = extraChapters.map((chapter) => chapter.scope.groupName).filter(Boolean);
  const doc = new PDFDocument({
    orientation: 'landscape', unit: 'mm', format: 'a4', compress: true, putOnlyUsedFonts: true,
  });
  doc.addFileToVFS('NotoSansTC-Regular.ttf', fontBase64);
  doc.addFont('NotoSansTC-Regular.ttf', FONT, 'normal');
  doc.setFont(FONT, 'normal');
  doc.setProperties({
    title: `RTA Sales Analysis - ${headerId}${scope ? ` - ${scope.groupName}` : ''}`,
    subject: `${firstCurrent.from} - ${firstCurrent.to}${chapterNames.length ? ` · ${chapterNames.join(' · ')}` : scope ? ` · ${scope.groupName}` : ''}`,
    author: 'RTA 銷售分析',
    creator: 'RTA 銷售分析',
  });

  const footers: FooterChapter[] = [];
  appendReportSection(doc, {
    result, storeId, categoryLevel, locale, filter, labels, headerId, baseStoreLabel, combined, stores,
    accumulator, scope, startOnCurrentPage: true, rankingLimit: limit,
  }, footers);
  if (!scope && extraChapters.length > 0) {
    appendGroupSummaryPages(doc, {
      result, storeId, categoryLevel, filter, labels, headerId, baseStoreLabel, extraChapters,
    }, footers);
  }

  addFooters(doc, headerId, footers);
  return new Uint8Array(doc.output('arraybuffer'));
}

interface ReportSectionOptions {
  result: SalesAnalysisResult;
  storeId: string;
  categoryLevel: SalesReportCategoryLevel;
  locale: Locale;
  filter: SalesReportFilter;
  labels: Labels;
  headerId: string;
  baseStoreLabel: string;
  combined: boolean;
  stores: SalesAnalysisStore[];
  accumulator?: SalesReportAccumulator;
  scope?: SalesReportScope;
  startOnCurrentPage: boolean;
  rankingLimit: number;
}

interface FooterChapter {
  start: number;
  end: number;
  label: string;
}

function appendReportSection(doc: jsPDF, options: ReportSectionOptions, footers: FooterChapter[]): void {
  const {
    result, storeId, categoryLevel, locale, filter, labels, headerId, baseStoreLabel, combined, stores,
    accumulator, scope, startOnCurrentPage, rankingLimit,
  } = options;
  const periods = storePeriods(result, storeId, filter, categoryLevel, accumulator, labels.uncategorized, scope, rankingLimit);
  const current = periodByKey(periods, 'current') ?? periods[0];
  if (!current) return;
  if (!startOnCurrentPage) doc.addPage();
  const start = doc.getNumberOfPages();
  const storeLabel = scope ? `${baseStoreLabel} · ${scope.groupName}` : baseStoreLabel;
  const pageTitle = (title: string) => scope ? `${title} · ${scope.groupName}` : title;

  drawSummaryPage(doc, periods, current, headerId, storeLabel, categoryLevel, labels, locale, pageTitle(labels.summary));
  const weeks = scope ? [] : weeksForReport(result.weeks ?? [], storeId, combined);
  if (weeks.length) {
    doc.addPage();
    drawWeeklyPages(doc, weeks, current, headerId, storeLabel, labels, locale);
  }
  if (!scope && combined && stores.length > 1) {
    doc.addPage();
    drawStoreComparisonPage(doc, result, headerId, storeLabel, labels, locale);
  }
  doc.addPage();
  drawOverallRankingsPage(doc, current, headerId, storeLabel, categoryLevel, labels, locale, rankingLimit, pageTitle(`${labels.topSales} / ${labels.topQuantity}`));
  const yearAgoNext = periodByKey(periods, 'yearAgoNext');
  if (yearAgoNext && (!scope || periodHasFocusRows(yearAgoNext))) {
    doc.addPage();
    drawFocusPage(doc, yearAgoNext, current, headerId, storeLabel, labels, pageTitle(labels.focusTitle));
  }

  for (const key of ['current', 'yearAgo', 'yearAgoNext']) {
    const period = periodByKey(periods, key);
    if (!period) continue;
    if (!scope || periodHasCategoryRows(period, 'amount')) {
      doc.addPage();
      drawCategoryRankingPage(doc, period, headerId, storeLabel, categoryLevel, 'amount', labels, locale, rankingLimit, pageTitle);
    }
  }
  for (const key of ['current', 'previous', 'previous2']) {
    const period = periodByKey(periods, key);
    if (!period) continue;
    if (!scope || periodHasCategoryRows(period, 'quantity')) {
      doc.addPage();
      drawCategoryRankingPage(doc, period, headerId, storeLabel, categoryLevel, 'quantity', labels, locale, rankingLimit, pageTitle);
    }
  }

  footers.push({ start, end: doc.getNumberOfPages(), label: scope?.groupName ?? '' });
}

const GROUP_SUMMARY_ROWS_PER_PAGE = 16;

interface GroupSummaryRow {
  name: string;
  current?: number;
  previous?: number;
  yearAgo?: number;
}

function appendGroupSummaryPages(
  doc: jsPDF,
  options: {
    result: SalesAnalysisResult;
    storeId: string;
    categoryLevel: SalesReportCategoryLevel;
    filter: SalesReportFilter;
    labels: Labels;
    headerId: string;
    baseStoreLabel: string;
    extraChapters: SalesReportChapter[];
  },
  footers: FooterChapter[],
): void {
  const { result, storeId, categoryLevel, filter, labels, headerId, baseStoreLabel, extraChapters } = options;
  const rows = extraChapters.map((chapter) => {
    const periods = storePeriods(
      result, storeId, filter, categoryLevel, chapter.accumulator, labels.uncategorized, chapter.scope,
    );
    const current = periodByKey(periods, 'current') ?? periods[0];
    const previous = periodByKey(periods, 'previous');
    const yearAgo = periodByKey(periods, 'yearAgo');
    return {
      name: chapter.scope.groupName,
      current: current?.totals.netSalesAmount,
      previous: previous?.totals.netSalesAmount,
      yearAgo: yearAgo?.totals.netSalesAmount,
    } satisfies GroupSummaryRow;
  });
  if (rows.length === 0) return;
  doc.addPage();
  const start = doc.getNumberOfPages();
  const span = (() => {
    const periods = storePeriods(result, storeId, filter, categoryLevel, undefined, labels.uncategorized);
    const current = periodByKey(periods, 'current') ?? periods[0];
    return current ? `${current.from} - ${current.to}` : '';
  })();
  for (let offset = 0; offset < rows.length; offset += GROUP_SUMMARY_ROWS_PER_PAGE) {
    if (offset > 0) doc.addPage();
    drawGroupSummaryPage(
      doc, rows.slice(offset, offset + GROUP_SUMMARY_ROWS_PER_PAGE), headerId, baseStoreLabel, span, labels,
    );
  }
  footers.push({ start, end: doc.getNumberOfPages(), label: labels.groupSummary });
}

function drawGroupSummaryPage(
  doc: jsPDF,
  rows: GroupSummaryRow[],
  storeId: string,
  storeLabel: string,
  period: string,
  labels: Labels,
): void {
  drawPageHeader(doc, labels.groupSummary, period, storeId, storeLabel);
  card(doc, CONTENT_X, CONTENT_Y, CONTENT_WIDTH, CONTENT_HEIGHT);
  panelTitle(doc, CONTENT_X, CONTENT_Y, CONTENT_WIDTH, labels.groupSummary);
  const innerX = CONTENT_X + 4;
  const tableY = CONTENT_Y + 18;
  const innerWidth = CONTENT_WIDTH - 8;
  const columns = [92, 38, 38, 32, 38, innerWidth - 238];
  drawTableHeader(doc, innerX, tableY, columns, [
    labels.group, labels.current, labels.previous, labels.vsPrevious, labels.yearAgo, labels.vsYearAgo,
  ]);
  rows.forEach((row, index) => {
    const rowY = tableY + 9.4 + index * 8.6;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 5, innerWidth, 8, 1.2, 1.2, 'F');
    }
    setText(doc, COLORS.ink, 7.6, 'bold');
    doc.text(fitText(doc, row.name || '-', columns[0] - 4), innerX + 2, rowY);
    const values: Array<{ text: string; color: RGB; bold?: boolean }> = [
      { text: row.current === undefined ? '-' : formatMoney(row.current), color: COLORS.ink, bold: true },
      { text: row.previous === undefined ? '-' : formatMoney(row.previous), color: COLORS.slate },
      percentCell(delta(row.current, row.previous)),
      { text: row.yearAgo === undefined ? '-' : formatMoney(row.yearAgo), color: COLORS.slate },
      percentCell(delta(row.current, row.yearAgo)),
    ];
    let cellX = innerX + columns[0];
    values.forEach((value, valueIndex) => {
      setText(doc, value.color, 7.1, value.bold ? 'bold' : 'normal');
      doc.text(value.text, cellX + columns[valueIndex + 1] - 2, rowY, { align: 'right' });
      cellX += columns[valueIndex + 1];
    });
  });
}

export function listSuccessfulReportStores(result: SalesAnalysisResult): SalesAnalysisStore[] {
  const periods = normalizedPeriods(result);
  const current = periods.find((period) => period.key === 'current') ?? periods[0];
  const stores = current?.stores?.length ? current.stores : result.stores;
  const labels = new Map<string, string>();
  for (const store of stores ?? []) {
    const id = store.businessId.trim();
    if (id) labels.set(id, store.label.trim());
  }
  if (labels.size === 0) {
    for (const item of current?.items ?? result.items ?? []) {
      const id = item.storeId.trim();
      if (id) labels.set(id, item.storeLabel.trim());
    }
  }
  return [...labels].map(([businessId, label]) => ({ businessId, label })).sort((left, right) =>
    left.businessId.localeCompare(right.businessId, undefined, { numeric: true }),
  );
}

export function isAllStoresReport(storeId: string): boolean {
  return storeId === ALL_STORES_REPORT_ID;
}

export function salesAnalysisPDFFilename(storeId: string, from: string, to: string, groupName?: string): string {
  const safeStore = isAllStoresReport(storeId) ? 'all' : (storeId.trim().replace(/[^\p{L}\p{N}_-]+/gu, '-') || 'store');
  const safeGroup = (groupName ?? '').trim().replace(/[^\p{L}\p{N}_-]+/gu, '-').replace(/^-+|-+$/g, '');
  const start = from.replaceAll('-', '') || 'report';
  const end = to.replaceAll('-', '');
  const period = end && end !== start ? `${start}-${end}` : start;
  return `RTA-Sales-${safeStore}${safeGroup ? `-${safeGroup}` : ''}-${period}.pdf`;
}

export function bytesToBase64(bytes: Uint8Array): string {
  const view = bytes.buffer.byteLength === bytes.byteLength
    ? bytes
    : new Uint8Array(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (typeof Buffer !== 'undefined' && typeof Buffer.from === 'function') {
    return Buffer.from(view).toString('base64');
  }
  let binary = '';
  const chunkSize = 0x2000;
  for (let offset = 0; offset < view.length; offset += chunkSize) {
    binary += String.fromCharCode(...view.subarray(offset, offset + chunkSize));
  }
  return btoa(binary);
}

function releaseLoadedReportFont(): void {
  fontBytesPromise = undefined;
}

async function loadFontBytes(): Promise<Uint8Array> {
  if (!fontBytesPromise) {
    fontBytesPromise = fetchReportFontBytes().catch((error) => {
      fontBytesPromise = undefined;
      throw error;
    });
  }
  return fontBytesPromise;
}

async function fetchReportFontBytes(): Promise<Uint8Array> {
  const candidates = [notoSansTCURL];
  if (typeof document !== 'undefined' && document.baseURI) {
    try { candidates.push(new URL(notoSansTCURL, document.baseURI).href); } catch { /* ignore */ }
  }
  try { candidates.push(new URL(notoSansTCURL, import.meta.url).href); } catch { /* ignore */ }
  let lastError: unknown;
  for (const url of [...new Set(candidates)]) {
    try {
      const response = await fetch(url);
      if (!response.ok) {
        lastError = new AppError('pdf_font', `Unable to load report font (${response.status})`);
        continue;
      }
      return new Uint8Array(await response.arrayBuffer());
    } catch (error) {
      lastError = error;
    }
  }
  try {
    const { readFileSync } = await import('node:fs');
    const { resolve } = await import('node:path');
    return new Uint8Array(readFileSync(resolve(process.cwd(), 'src/lib/assets/NotoSansTC-Regular.ttf')));
  } catch (error) {
    lastError = lastError ?? error;
  }
  if (lastError instanceof AppError) throw lastError;
  throw new AppError('pdf_font', lastError instanceof Error ? lastError.message : 'Unable to load report font');
}

const REQUIRED_GLYPHS = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz $HKD,.%+-–—…|/():;[]\'"`~@#*_<>?';

export function collectReportGlyphs(
  result: SalesAnalysisResult,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  filter: SalesReportFilter = defaultSalesReportFilter(),
  scope?: SalesReportScope,
  extraScopes: SalesReportScope[] = [],
): string {
  const glyphs = new Set(REQUIRED_GLYPHS);
  const add = (value: string | undefined) => {
    if (!value) return;
    for (const character of value) glyphs.add(character);
  };
  add('RTA SALES');
  add('RTA Sales Analysis');
  add('Page');
  add(scope?.groupName);
  for (const extra of extraScopes) add(extra.groupName);
  add(reportLabels(locale).title);
  for (const value of Object.values(reportLabels(locale))) add(value);
  add(result.from);
  add(result.to);
  const scopeCodes = itemCodeSet(scope);
  for (const store of listSuccessfulReportStores(result)) {
    add(store.businessId);
    add(store.label);
  }
  for (const period of normalizedPeriods(result)) {
    add(period.key);
    add(period.label);
    add(period.from);
    add(period.to);
    for (const store of period.stores ?? []) {
      add(store.businessId);
      add(store.label);
    }
    for (const item of period.items ?? result.items ?? []) {
      if (!includeInSalesReport(item, filter, categoryLevel)) continue;
      if (scope && !scopeCodes.has(item.articleCode.trim())) continue;
      add(item.storeId);
      add(item.storeLabel);
      add(item.articleCode);
      add(item.articleName);
      add(item.brandName);
      add(item.category1);
      add(item.category1Code);
      add(item.category2);
      add(item.category2Code);
      add(item.category3);
      add(item.category3Code);
      add(item.category4);
      add(item.category4Code);
      add(item.category5);
      add(item.category5Code);
    }
  }
  for (const week of result.weeks ?? []) {
    add(week.from);
    add(week.to);
    for (const store of week.stores ?? []) {
      add(store.businessId);
      add(store.label);
    }
  }
  return [...glyphs].join('');
}

function normalizedPeriods(result: SalesAnalysisResult): SalesAnalysisPeriodResult[] {
  if (result.periods?.length) return result.periods;
  return [{
    key: 'current', label: 'Current', from: result.from, to: result.to,
    complete: result.complete, successfulStores: result.successfulStores,
    totals: result.totals, stores: result.stores, items: result.items, issues: result.issues,
  }];
}

export function filterSalesAnalysisResult(
  result: SalesAnalysisResult,
  filter: SalesReportFilter,
  level: SalesReportCategoryLevel,
): SalesAnalysisResult {
  const keep = (item: SalesAnalysisItem) => includeInSalesReport(item, filter, level);
  return {
    ...result,
    items: result.items?.filter(keep),
    periods: result.periods?.map((period) => ({
      ...period,
      items: period.items?.filter(keep),
    })),
  };
}

export function createSalesReportAccumulator(): SalesReportAccumulator {
  return { periods: new Map() };
}

export function salesReportAccumulatorFromMemo(memo: SalesAnalysisReportMemo): SalesReportAccumulator {
  const accumulator = createSalesReportAccumulator();
  for (const period of memo.periods ?? []) {
    accumulator.periods.set(period.key, periodAccumulatorFromMemo(period));
  }
  return accumulator;
}

function periodAccumulatorFromMemo(period: SalesAnalysisPeriodMemo): PeriodAccumulator {
  const products = new Map<string, AccProduct>();
  const addProduct = (item: { id: string; code: string; name: string; brand?: string; amount: number; quantity: number; category2Code?: string; category3Code?: string; category4Code?: string }) => {
    const id = item.id || item.code;
    if (!id) return;
    const existing = products.get(id) ?? {
      id, code: item.code, name: item.name, brand: item.brand ?? '',
      amount: 0, quantity: 0, category2Code: '', category3Code: '', category4Code: '',
    };
    existing.amount = item.amount;
    existing.quantity = item.quantity;
    if (item.category2Code) existing.category2Code = item.category2Code;
    if (item.category3Code) existing.category3Code = item.category3Code;
    if (item.category4Code) existing.category4Code = item.category4Code;
    products.set(id, existing);
  };
  for (const item of period.topAmount ?? []) addProduct(item);
  for (const item of period.topQuantity ?? []) addProduct(item);
  const categories = new Map<string, { id: string; code: string; name: string; amount: number; quantity: number; products: Map<string, RankedItem> }>();
  for (const group of period.amountGroups ?? []) {
    const productsInGroup = new Map<string, RankedItem>();
    for (const item of group.items ?? []) {
      addProduct(item);
      productsInGroup.set(item.id || item.code, {
        id: item.id || item.code, code: item.code, name: item.name, brand: item.brand ?? '',
        amount: item.amount, quantity: item.quantity,
      });
    }
    categories.set(group.id, {
      id: group.id, code: group.code ?? '', name: group.name,
      amount: group.amount, quantity: group.quantity, products: productsInGroup,
    });
  }
  for (const group of period.quantityGroups ?? []) {
    const existing = categories.get(group.id) ?? {
      id: group.id, code: group.code ?? '', name: group.name,
      amount: group.amount, quantity: group.quantity, products: new Map(),
    };
    for (const item of group.items ?? []) {
      existing.products.set(item.id || item.code, {
        id: item.id || item.code, code: item.code, name: item.name, brand: item.brand ?? '',
        amount: item.amount, quantity: item.quantity,
      });
    }
    categories.set(group.id, existing);
  }
  return {
    products,
    categories,
    totals: period.totals,
    focusCatalog: period.focusCatalog,
    focusGroups: period.focusGroups
      ? period.focusGroups.map((group) => ({
        id: group.id,
        prefix: group.prefix,
        name: group.name,
        sales: (group.sales ?? []).map((item) => ({
          id: item.id, code: item.code, name: item.name, brand: item.brand ?? '',
          amount: item.amount, quantity: item.quantity,
          currentAmount: item.currentAmount, currentQuantity: item.currentQuantity,
        })),
        quantity: (group.quantity ?? []).map((item) => ({
          id: item.id, code: item.code, name: item.name, brand: item.brand ?? '',
          amount: item.amount, quantity: item.quantity,
          currentAmount: item.currentAmount, currentQuantity: item.currentQuantity,
        })),
      }))
      : (period.focusCatalog ? [] : undefined),
  };
}

export function addSalesReportPeriodItems(
  accumulator: SalesReportAccumulator,
  periodKey: string,
  items: SalesAnalysisItem[],
  filter: SalesReportFilter,
  level: SalesReportCategoryLevel,
): void {
  let period = accumulator.periods.get(periodKey);
  if (!period) {
    period = { products: new Map(), categories: new Map() };
    accumulator.periods.set(periodKey, period);
  }
  for (const item of items) {
    if (!includeInSalesReport(item, filter, level)) continue;
    const id = item.articleCode.trim() || item.articleName.trim();
    if (!id) continue;
    const product = period.products.get(id) ?? {
      id, code: item.articleCode.trim(), name: item.articleName.trim(), brand: item.brandName?.trim() ?? '',
      amount: 0, quantity: 0, category2Code: '', category3Code: '', category4Code: '',
    };
    product.amount += item.netSalesAmount;
    product.quantity += item.netQuantity;
    if (!product.name && item.articleName.trim()) product.name = item.articleName.trim();
    if (!product.brand && item.brandName?.trim()) product.brand = item.brandName.trim();
    if (!product.category2Code && item.category2Code?.trim()) product.category2Code = item.category2Code.trim();
    if (!product.category3Code && item.category3Code?.trim()) product.category3Code = item.category3Code.trim();
    if (!product.category4Code && item.category4Code?.trim()) product.category4Code = item.category4Code.trim();
    period.products.set(id, product);

    const code = categoryCode(item, level);
    const name = categoryName(item, level);
    const categoryId = code || name;
    if (!categoryId) continue;
    const category = period.categories.get(categoryId) ?? {
      id: categoryId, code, name, amount: 0, quantity: 0, products: new Map(),
    };
    category.amount += item.netSalesAmount;
    category.quantity += item.netQuantity;
    if (!category.name && name) category.name = name;
    const ranked = category.products.get(id) ?? {
      id, code: product.code, name: product.name, brand: product.brand, amount: 0, quantity: 0,
    };
    ranked.amount += item.netSalesAmount;
    ranked.quantity += item.netQuantity;
    if (!ranked.name && product.name) ranked.name = product.name;
    category.products.set(id, ranked);
    period.categories.set(categoryId, category);
  }
}

function storePeriods(
  result: SalesAnalysisResult,
  storeId: string,
  filter: SalesReportFilter,
  level: SalesReportCategoryLevel,
  accumulator?: SalesReportAccumulator,
  uncategorized = 'Uncategorized',
  scope?: SalesReportScope,
  rankingLimit: number = DEFAULT_RANKING_LIMIT,
): StorePeriod[] {
  const scopeCodes = itemCodeSet(scope);
  const limit = normalizeRankingLimit(rankingLimit);
  return normalizedPeriods(result).flatMap((period) => {
    const memo = accumulator ? storePeriodMemo(accumulator, period.key, uncategorized, period.key === 'yearAgoNext', limit) : undefined;
    const keep = (item: SalesAnalysisItem) => includeInSalesReport(item, filter, level)
      && (!scope || scopeCodes.has(item.articleCode.trim()));
    if (isAllStoresReport(storeId)) {
      const items = memo ? [] : (period.items ?? []).filter(keep);
      return [{
        key: period.key, label: period.label, from: period.from, to: period.to,
        totals: memo?.totals ?? (scope ? aggregateScopedTotals(items) : period.totals),
        items,
        ...memo,
      }];
    }
    const items = (period.items ?? []).filter((item) => item.storeId === storeId && keep(item));
    const summary = period.stores?.find((store) => store.businessId === storeId);
    if (!summary && items.length === 0 && !memo && period.key !== 'current') return [];
    return [{
      key: period.key, label: period.label, from: period.from, to: period.to,
      totals: memo?.totals ?? (scope ? aggregateScopedTotals(items) : (summary?.totals ?? aggregateTotals(items))), items,
      ...memo,
    }];
  });
}

function storePeriodMemo(
  accumulator: SalesReportAccumulator,
  periodKey: string,
  uncategorized: string,
  includeFocus = false,
  rankingLimit: number = DEFAULT_RANKING_LIMIT,
): (Pick<StorePeriod, 'amountGroups' | 'quantityGroups' | 'topAmount' | 'topQuantity' | 'focusGroups' | 'focusCatalog'>
  & { totals?: SalesAnalysisTotals }) | undefined {
  const period = accumulator.periods.get(periodKey);
  if (!period) return undefined;
  const limit = normalizeRankingLimit(rankingLimit);
  return {
    ...(period.totals ? { totals: period.totals } : {}),
    amountGroups: groupsFromAccumulator(period, 'amount', uncategorized, limit),
    quantityGroups: groupsFromAccumulator(period, 'quantity', uncategorized, limit),
    topAmount: sortRankedItems([...period.products.values()], 'amount').slice(0, limit),
    topQuantity: sortRankedItems([...period.products.values()], 'quantity').slice(0, limit),
    focusCatalog: period.focusCatalog,
    focusGroups: period.focusGroups
      ?? (period.focusCatalog ? [] : (includeFocus ? focusGroupsFromAccumulator(period, accumulator.periods.get('current')) : undefined)),
  };
}

function groupsFromAccumulator(period: PeriodAccumulator, metric: Metric, uncategorized: string, rankingLimit: number = DEFAULT_RANKING_LIMIT): CategoryGroup[] {
  const limit = normalizeRankingLimit(rankingLimit);
  return [...period.categories.values()].map((group) => ({
    id: group.id,
    code: group.code,
    name: group.name || uncategorized,
    amount: group.amount,
    quantity: group.quantity,
    items: sortRankedItems([...group.products.values()], metric).slice(0, limit),
  })).filter((group) => group.items.length > 0).sort((left, right) =>
    (metric === 'amount' ? right.amount - left.amount : right.quantity - left.quantity)
      || left.id.localeCompare(right.id, undefined, { numeric: true }),
  );
}

function sortRankedItems(items: RankedItem[], metric: Metric): RankedItem[] {
  return [...items].sort((left, right) =>
    (metric === 'amount' ? right.amount - left.amount : right.quantity - left.quantity)
      || left.id.localeCompare(right.id, undefined, { numeric: true }),
  );
}

function focusGroupsFromAccumulator(
  yearAgoNext: PeriodAccumulator | undefined,
  current: PeriodAccumulator | undefined,
): FocusGroup[] | undefined {
  if (!yearAgoNext) return undefined;
  const currentByCode = current?.products ?? new Map();
  return FOCUS_GROUP_PREFIXES.map((group) => {
    const ranked = [...yearAgoNext.products.values()]
      .filter((product) => productMatchesFocus(product, group.prefix))
      .map((product) => {
        const live = currentByCode.get(product.id) ?? currentByCode.get(product.code);
        return {
          id: product.id,
          code: product.code,
          name: product.name,
          brand: product.brand,
          amount: product.amount,
          quantity: product.quantity,
          currentAmount: live?.amount ?? 0,
          currentQuantity: live?.quantity ?? 0,
        };
      });
    return {
      id: group.id,
      prefix: group.prefix,
      sales: [...ranked].sort((left, right) => right.amount - left.amount || left.id.localeCompare(right.id)).slice(0, 8),
      quantity: [...ranked].sort((left, right) => right.quantity - left.quantity || left.id.localeCompare(right.id)).slice(0, 8),
    };
  }).filter((group) => group.sales.length > 0 || group.quantity.length > 0);
}

function productMatchesFocus(product: AccProduct, prefix: string): boolean {
  const department = product.category2Code.trim();
  if (department) return department === prefix || department.startsWith(prefix);
  const fallback = product.category3Code.trim() || product.category4Code.trim();
  return fallback === prefix || fallback.startsWith(prefix);
}

function aggregateTotals(items: SalesAnalysisItem[]): SalesAnalysisTotals {
  return items.reduce<SalesAnalysisTotals>((totals, item) => ({
    saleQuantity: totals.saleQuantity + item.saleQuantity,
    saleAmount: totals.saleAmount + item.saleAmount,
    returnQuantity: totals.returnQuantity + item.returnQuantity,
    returnAmount: totals.returnAmount + item.returnAmount,
    netQuantity: totals.netQuantity + item.netQuantity,
    netSalesAmount: totals.netSalesAmount + item.netSalesAmount,
    transactionCount: (totals.transactionCount ?? 0) + item.transactionCount,
    trendNetSalesAmount: (totals.trendNetSalesAmount ?? 0) + item.netSalesAmount,
  }), {
    saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0,
    netQuantity: 0, netSalesAmount: 0, transactionCount: 0, trendNetSalesAmount: 0,
  });
}

function aggregateScopedTotals(items: SalesAnalysisItem[]): SalesAnalysisTotals {
  const totals = aggregateTotals(items);
  delete totals.transactionCount;
  delete totals.trendNetSalesAmount;
  return totals;
}

function itemCodeSet(scope: SalesReportScope | undefined): Set<string> {
  return new Set((scope?.itemCodes ?? []).map((code) => code.trim()).filter(Boolean));
}

function periodHasFocusRows(period: StorePeriod): boolean {
  const groups = period.focusGroups
    ?? (period.focusCatalog ? [] : buildFocusGroups(period.items, [], 1));
  return groups.some((group) => group.sales.length > 0 || group.quantity.length > 0);
}

function periodHasCategoryRows(period: StorePeriod, metric: Metric): boolean {
  const groups = metric === 'amount' ? period.amountGroups : period.quantityGroups;
  if (groups) return groups.length > 0;
  return period.items.some((item) => metric === 'amount' ? item.netSalesAmount !== 0 : item.netQuantity !== 0);
}

function periodByKey(periods: StorePeriod[], key: string): StorePeriod | undefined {
  return periods.find((period) => period.key === key);
}

function drawSummaryPage(
  doc: jsPDF,
  periods: StorePeriod[],
  current: StorePeriod,
  storeId: string,
  storeLabel: string,
  level: SalesReportCategoryLevel,
  labels: Labels,
  locale: Locale,
  title = labels.summary,
): void {
  drawPageHeader(doc, title, `${current.from} - ${current.to}`, storeId, storeLabel);
  const previous = periodByKey(periods, 'previous');
  const yearAgo = periodByKey(periods, 'yearAgo');
  const metrics = [
    { label: labels.netSales, value: formatMoney(current.totals.netSalesAmount), delta: delta(current.totals.netSalesAmount, yearAgo?.totals.netSalesAmount) },
    { label: labels.netQuantity, value: formatQuantity(current.totals.netQuantity), delta: delta(current.totals.netQuantity, yearAgo?.totals.netQuantity) },
    { label: labels.transactions, value: optionalQuantity(current.totals.transactionCount), delta: delta(current.totals.transactionCount, yearAgo?.totals.transactionCount) },
    { label: labels.basket, value: basketValue(current.totals), delta: delta(basketNumber(current.totals), basketNumber(yearAgo?.totals)) },
  ];
  const gap = 4;
  const cardWidth = (277 - gap * 3) / 4;
  metrics.forEach((metric, index) => drawMetricCard(doc, 10 + index * (cardWidth + gap), 31, cardWidth, 24, metric.label, metric.value, metric.delta, labels.yearAgo));

  drawComparisonPanel(doc, 10, 59, 277, 48, current, previous, yearAgo, labels);
  drawCategoryPerformancePanel(doc, 10, 111, 277, 75, current, previous, yearAgo, level, labels, locale);
}

function weeksForReport(weeks: SalesAnalysisWeek[], storeId: string, combined: boolean): SalesAnalysisWeek[] {
  if (combined) return weeks;
  return weeks.flatMap((week) => {
    const store = week.stores?.find((row) => row.businessId === storeId);
    if (!store) return [];
    return [{ ...week, stores: [store], totals: store }];
  });
}

const WEEKLY_ROW = 7.2;

function drawWeeklyPages(
  doc: jsPDF,
  weeks: SalesAnalysisWeek[],
  current: StorePeriod,
  storeId: string,
  storeLabel: string,
  labels: Labels,
  locale: Locale,
): void {
  const span = weeks.length
    ? `${weeks[0]!.from} - ${weeks[weeks.length - 1]!.to}`
    : `${current.from} - ${current.to}`;
  drawPageHeader(doc, labels.weeklyTitle, span, storeId, storeLabel);
  const singleStore = weeks.every((week) => (week.stores?.length ?? 0) <= 1);
  if (singleStore) {
    drawWeeklyPeriodTable(doc, weeks, labels, locale);
    return;
  }
  let y = CONTENT_Y;
  for (const week of weeks) {
    const height = weeklyStoreBlockHeight(week, labels);
    if (y + height > CONTENT_Y + CONTENT_HEIGHT && y > CONTENT_Y) {
      doc.addPage();
      drawPageHeader(doc, labels.weeklyTitle, span, storeId, storeLabel);
      y = CONTENT_Y;
    }
    y = drawWeeklyStoreBlock(doc, week, y, labels);
  }
}

function drawWeeklyPeriodTable(doc: jsPDF, weeks: SalesAnalysisWeek[], labels: Labels, locale: Locale): void {
  const columns = [28, 44, 34, 34, 32, 26, 26, 26, CONTENT_WIDTH - 8 - 250];
  const innerX = CONTENT_X + 4;
  const innerWidth = CONTENT_WIDTH - 8;
  const headerY = CONTENT_Y + 10;
  const cardHeight = Math.min(CONTENT_HEIGHT, 16 + weeks.length * WEEKLY_ROW + 8);
  card(doc, CONTENT_X, CONTENT_Y, CONTENT_WIDTH, cardHeight);
  drawTableHeader(doc, innerX, headerY, columns, [
    labels.week, labels.period, labels.thisWeek, labels.lastWeek, labels.variance, labels.variancePercent,
    labels.weekday, labels.weekend, labels.customers,
  ]);
  weeks.forEach((week, index) => {
    const values = week.stores[0] ?? week.totals;
    drawWeeklyMetricRow(doc, innerX, headerY + 8 + index * WEEKLY_ROW, innerWidth, columns, {
      label: weekLabel(index, locale),
      detail: `${week.from} - ${week.to}`,
      values,
    }, index, 2);
  });
}

function weeklyStoreBlockHeight(week: SalesAnalysisWeek, labels: Labels): number {
  return 12 + 8 + weeklyStoreRows(week, labels).length * WEEKLY_ROW + 6;
}

function weeklyStoreRows(week: SalesAnalysisWeek, labels: Labels): Array<{ label: string; values: SalesAnalysisWeekStore; emphasis?: boolean }> {
  return weeklySegmentRows(week.stores ?? [], {
    store: storeLabelText,
    localTotal: labels.localTotal,
    touristTotal: labels.touristTotal,
    allStores: labels.allStores,
  }).map((row) => ({
    label: row.label,
    values: row.values,
    emphasis: row.kind !== 'store',
  }));
}

function storeLabelText(store: SalesAnalysisWeekStore | undefined): string {
  if (!store) return '';
  if (store.label && store.businessId && store.label !== store.businessId) return `${store.businessId}  ${store.label}`;
  return store.businessId || store.label || '';
}

function drawWeeklyStoreBlock(doc: jsPDF, week: SalesAnalysisWeek, y: number, labels: Labels): number {
  const rows = weeklyStoreRows(week, labels);
  const height = weeklyStoreBlockHeight(week, labels);
  const innerX = CONTENT_X + 4;
  const innerWidth = CONTENT_WIDTH - 8;
  const columns = [52, 32, 32, 30, 26, 26, 26, innerWidth - 224];
  card(doc, CONTENT_X, y, CONTENT_WIDTH, height);
  setText(doc, COLORS.ink, 8, 'bold');
  doc.text(`${week.from} - ${week.to}`, innerX, y + 8);
  const headerY = y + 14;
  drawTableHeader(doc, innerX, headerY, columns, [
    labels.store, labels.thisWeek, labels.lastWeek, labels.variance, labels.variancePercent,
    labels.weekday, labels.weekend, labels.customers,
  ]);
  rows.forEach((row, index) => {
    drawWeeklyMetricRow(doc, innerX, headerY + 8 + index * WEEKLY_ROW, innerWidth, columns, {
      label: row.label || labels.allStores,
      values: row.values,
      emphasis: row.emphasis,
    }, index);
  });
  return y + height + 4;
}

function drawWeeklyMetricRow(
  doc: jsPDF,
  x: number,
  rowY: number,
  innerWidth: number,
  columns: number[],
  row: { label: string; detail?: string; values: SalesAnalysisWeekStore; emphasis?: boolean },
  index: number,
  valueStart = 1,
): void {
  if (row.emphasis || index % 2 === 0) {
    setFill(doc, row.emphasis ? COLORS.tealSoft : COLORS.surface);
    doc.roundedRect(x, rowY - 4.2, innerWidth, WEEKLY_ROW - 0.6, 1.2, 1.2, 'F');
  }
  setText(doc, COLORS.ink, 7.2, 'bold');
  doc.text(fitText(doc, row.label, columns[0] - 3), x + 2, rowY);
  if (row.detail && valueStart > 1) {
    setText(doc, COLORS.slate, 6.4, 'normal');
    doc.text(fitText(doc, row.detail, columns[1] - 3), x + columns[0] + 2, rowY);
  }
  const salesChange = delta(row.values.salesTw, row.values.salesLw);
  const cells: Array<{ text: string; color: RGB; bold?: boolean }> = [
    { text: formatMoney(row.values.salesTw), color: COLORS.ink, bold: true },
    { text: formatMoney(row.values.salesLw), color: COLORS.slate },
    { text: formatMoney(row.values.salesTw - row.values.salesLw), color: changeColor(salesChange), bold: true },
    percentCell(salesChange),
    percentCell(delta(row.values.weekdaySalesTw, row.values.weekdaySalesLw)),
    percentCell(delta(row.values.weekendSalesTw, row.values.weekendSalesLw)),
    percentCell(delta(row.values.customersTw, row.values.customersLw)),
  ];
  let cellX = x + columns.slice(0, valueStart).reduce((sum, value) => sum + value, 0);
  cells.forEach((cell, cellIndex) => {
    const width = columns[valueStart + cellIndex];
    setText(doc, cell.color, 6.8, cell.bold ? 'bold' : 'normal');
    doc.text(cell.text, cellX + width - 2, rowY, { align: 'right' });
    cellX += width;
  });
}

function weekLabel(index: number, locale: Locale): string {
  return locale === 'en' ? `Week ${index + 1}` : `第${index + 1}週`;
}

function changeColor(change: number | undefined): RGB {
  if (change === undefined || change === 0) return COLORS.slate;
  return change > 0 ? COLORS.positive : COLORS.negative;
}

function drawStoreComparisonPage(
  doc: jsPDF,
  result: SalesAnalysisResult,
  storeId: string,
  storeLabel: string,
  labels: Labels,
  locale: Locale,
): void {
  const periods = normalizedPeriods(result);
  const current = periods.find((period) => period.key === 'current') ?? periods[0];
  if (!current) return;
  drawPageHeader(doc, labels.storeComparison, `${current.from} - ${current.to}`, storeId, storeLabel);

  const previous = periods.find((period) => period.key === 'previous');
  const yearAgo = periods.find((period) => period.key === 'yearAgo');
  const rows = listSuccessfulReportStores(result).map((store) => {
    const currentTotals = current.stores?.find((item) => item.businessId === store.businessId)?.totals;
    const previousTotals = previous?.stores?.find((item) => item.businessId === store.businessId)?.totals;
    const yearAgoTotals = yearAgo?.stores?.find((item) => item.businessId === store.businessId)?.totals;
    return {
      id: store.businessId,
      label: store.label,
      current: currentTotals?.netSalesAmount,
      previous: previousTotals?.netSalesAmount,
      yearAgo: yearAgoTotals?.netSalesAmount,
      transactions: currentTotals?.transactionCount,
      basket: basketNumber(currentTotals),
    };
  }).sort((left, right) => (right.current ?? 0) - (left.current ?? 0) || left.id.localeCompare(right.id, locale, { numeric: true }));

  const x = CONTENT_X;
  const y = CONTENT_Y;
  const width = CONTENT_WIDTH;
  const height = CONTENT_HEIGHT;
  card(doc, x, y, width, height);
  panelTitle(doc, x, y, width, labels.storeComparison);
  const innerX = x + 4;
  const tableY = y + 18;
  const innerWidth = width - 8;
  const columns = [70, 32, 32, 32, 26, 30, 26, innerWidth - 248];
  drawTableHeader(doc, innerX, tableY, columns, [
    labels.store, labels.current, labels.previous, labels.yearAgo, labels.vsPrevious, labels.vsYearAgo, labels.transactions, labels.basket,
  ]);
  const rowHeight = Math.min(7.2, Math.max(5.4, (height - 28) / Math.max(rows.length, 1)));
  rows.forEach((row, index) => {
    const rowY = tableY + 8.4 + index * rowHeight;
    if (rowY > y + height - 6) return;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 4.2, innerWidth, rowHeight - 0.6, 1.2, 1.2, 'F');
    }
    const name = row.label && row.label !== row.id ? `${row.id}  ${row.label}` : row.id;
    setText(doc, COLORS.ink, 6.6, 'bold');
    doc.text(fitText(doc, name, columns[0] - 3), innerX + 2, rowY);
    let cellX = innerX + columns[0];
    const values: Array<{ text: string; color: RGB; bold?: boolean }> = [
      { text: row.current === undefined ? '-' : formatMoney(row.current), color: COLORS.ink, bold: true },
      { text: row.previous === undefined ? '-' : formatMoney(row.previous), color: COLORS.slate },
      { text: row.yearAgo === undefined ? '-' : formatMoney(row.yearAgo), color: COLORS.slate },
      percentCell(delta(row.current, row.previous)),
      percentCell(delta(row.current, row.yearAgo)),
      { text: row.transactions === undefined ? '-' : formatQuantity(row.transactions), color: COLORS.slate },
      { text: row.basket === undefined ? '-' : formatMoney(row.basket), color: COLORS.slate },
    ];
    values.forEach((value, valueIndex) => {
      setText(doc, value.color, 6.4, value.bold ? 'bold' : 'normal');
      doc.text(value.text, cellX + columns[valueIndex + 1] - 2, rowY, { align: 'right' });
      cellX += columns[valueIndex + 1];
    });
  });
}

function drawOverallRankingsPage(
  doc: jsPDF,
  current: StorePeriod,
  storeId: string,
  storeLabel: string,
  level: SalesReportCategoryLevel,
  labels: Labels,
  locale: Locale,
  rankingLimit: number,
  title = `${labels.topSales} / ${labels.topQuantity}`,
): void {
  const limit = normalizeRankingLimit(rankingLimit);
  const compact = limit > 16;
  const sales = (current.topAmount ?? rankItems(current.items, 'amount')).slice(0, limit);
  const quantity = (current.topQuantity ?? rankItems(current.items, 'quantity')).slice(0, limit);
  const capacity = rankingPanelCapacity(159, limit);
  const pages = Math.max(1, Math.ceil(Math.max(sales.length, quantity.length, 1) / capacity));
  for (let page = 0; page < pages; page += 1) {
    if (page > 0) doc.addPage();
    drawPageHeader(doc, title, `${current.from} - ${current.to}`, storeId, storeLabel);
    const from = page * capacity;
    const to = from + capacity;
    drawRankingPanel(doc, 10, 31, 136.5, 159, labels.topSales, sales.slice(from, to), 'amount', level, labels, locale, compact, from);
    drawRankingPanel(doc, 150.5, 31, 136.5, 159, labels.topQuantity, quantity.slice(from, to), 'quantity', level, labels, locale, compact, from);
  }
}

function drawCategoryRankingPage(
  doc: jsPDF,
  period: StorePeriod,
  storeId: string,
  storeLabel: string,
  level: SalesReportCategoryLevel,
  metric: Metric,
  labels: Labels,
  locale: Locale,
  rankingLimit: number,
  titleFor: (title: string) => string = (title) => title,
): void {
  const title = titleFor(`${period.label} - ${metric === 'amount' ? labels.salesRanking : labels.quantityRanking}`);
  const limit = normalizeRankingLimit(rankingLimit);
  const groups = metric === 'amount'
    ? period.amountGroups ?? categoryGroups(period.items, level, metric, labels.uncategorized, limit)
    : period.quantityGroups ?? categoryGroups(period.items, level, metric, labels.uncategorized, limit);
  if (groups.length === 0) {
    drawPageHeader(doc, title, `${period.from} - ${period.to}`, storeId, storeLabel);
    return;
  }
  const cardsPerPage = categoryRankingCardsPerPage(limit);
  const slotHeight = categoryRankingCardSlots(Math.min(cardsPerPage, 3))[0]?.height ?? CONTENT_HEIGHT;
  const cards = expandCategoryRankingCards(groups, limit, categoryCardRowMetrics(slotHeight).limit);
  for (let offset = 0; offset < cards.length; offset += cardsPerPage) {
    if (offset > 0) doc.addPage();
    drawPageHeader(doc, title, `${period.from} - ${period.to}`, storeId, storeLabel);
    const pageGroups = cards.slice(offset, offset + cardsPerPage);
    const slots = categoryRankingCardSlots(pageGroups.length);
    pageGroups.forEach((group, index) => {
      const slot = slots[index];
      if (!slot) return;
      drawCategoryCard(doc, slot.x, slot.y, slot.width, slot.height, group, metric, labels, locale, group.rankOffset ?? 0);
    });
  }
}

function expandCategoryRankingCards(
  groups: CategoryGroup[],
  rankingLimit: number,
  rowsPerCard: number,
): Array<CategoryGroup & { rankOffset: number }> {
  const rowLimit = Math.max(1, rowsPerCard);
  const cards: Array<CategoryGroup & { rankOffset: number }> = [];
  for (const group of groups) {
    const items = group.items.slice(0, rankingLimit);
    if (items.length === 0) continue;
    for (let offset = 0; offset < items.length; offset += rowLimit) {
      cards.push({
        ...group,
        items: items.slice(offset, offset + rowLimit),
        rankOffset: offset,
      });
    }
  }
  return cards;
}

export function categoryRankingCardSlots(count: number): Array<{ x: number; y: number; width: number; height: number }> {
  if (count <= 0) return [];
  const { x, y, width, height } = { x: CONTENT_X, y: CONTENT_Y, width: CONTENT_WIDTH, height: CONTENT_HEIGHT };
  if (count <= 3) {
    const cardWidth = (width - CARD_GAP * (count - 1)) / count;
    return Array.from({ length: count }, (_, index) => ({
      x: x + index * (cardWidth + CARD_GAP),
      y,
      width: cardWidth,
      height,
    }));
  }
  if (count === 4) {
    const cardWidth = (width - CARD_GAP) / 2;
    const cardHeight = (height - CARD_GAP) / 2;
    return Array.from({ length: 4 }, (_, index) => ({
      x: x + (index % 2) * (cardWidth + CARD_GAP),
      y: y + Math.floor(index / 2) * (cardHeight + CARD_GAP),
      width: cardWidth,
      height: cardHeight,
    }));
  }
  const firstRow = 3;
  const secondRow = count - firstRow;
  const topWidth = (width - CARD_GAP * 2) / 3;
  const cardHeight = (height - CARD_GAP) / 2;
  const slots = Array.from({ length: firstRow }, (_, index) => ({
    x: x + index * (topWidth + CARD_GAP),
    y,
    width: topWidth,
    height: cardHeight,
  }));
  const bottomWidth = (width - CARD_GAP * (secondRow - 1)) / secondRow;
  for (let index = 0; index < secondRow; index += 1) {
    slots.push({
      x: x + index * (bottomWidth + CARD_GAP),
      y: y + cardHeight + CARD_GAP,
      width: bottomWidth,
      height: cardHeight,
    });
  }
  return slots;
}

function drawPageHeader(doc: jsPDF, title: string, period: string, storeId: string, storeLabel: string): void {
  setFill(doc, COLORS.surface);
  doc.rect(0, 0, PAGE_WIDTH, 24, 'F');
  setFill(doc, COLORS.teal);
  doc.rect(0, 0, 4, 24, 'F');
  setText(doc, COLORS.teal, 6.5, 'bold');
  doc.text('RTA SALES', 10, 8);
  setText(doc, COLORS.ink, 15, 'bold');
  doc.text(fitText(doc, title, 165), 10, 18);
  setText(doc, COLORS.ink, 9, 'bold');
  doc.text(fitText(doc, storeLabel, 78), 287, 10, { align: 'right' });
  setText(doc, COLORS.slate, 7, 'normal');
  doc.text(`${storeId}  |  ${period}`, 287, 18, { align: 'right' });
}

function drawMetricCard(doc: jsPDF, x: number, y: number, width: number, height: number, label: string, value: string, change: number | undefined, comparisonLabel: string): void {
  card(doc, x, y, width, height);
  setFill(doc, COLORS.teal);
  doc.roundedRect(x, y, width, 2.1, 1.5, 1.5, 'F');
  setText(doc, COLORS.slate, 7.2, 'normal');
  doc.text(label, x + 4, y + 8);
  setText(doc, COLORS.ink, 14, 'bold');
  doc.text(fitText(doc, value, width - 8), x + 4, y + 18);
  if (change !== undefined) {
    setText(doc, change >= 0 ? COLORS.positive : COLORS.negative, 6.4, 'bold');
    doc.text(`${comparisonLabel} ${formatPercent(change)}`, x + width - 4, y + 8, { align: 'right' });
  }
}

function drawComparisonPanel(doc: jsPDF, x: number, y: number, width: number, height: number, current: StorePeriod, previous: StorePeriod | undefined, yearAgo: StorePeriod | undefined, labels: Labels): void {
  card(doc, x, y, width, height);
  panelTitle(doc, x, y, width, labels.comparison);
  const innerX = x + 4;
  const tableY = y + 18;
  const innerWidth = width - 8;
  const columns = [40, 48, 46, 46, 42, innerWidth - 222];
  drawTableHeader(doc, innerX, tableY, columns, [labels.metric, labels.current, labels.previous, labels.yearAgo, labels.vsPrevious, labels.vsYearAgo]);
  const rows = [
    { label: labels.netSales, current: current.totals.netSalesAmount, previous: previous?.totals.netSalesAmount, yearAgo: yearAgo?.totals.netSalesAmount, format: formatMoney },
    { label: labels.netQuantity, current: current.totals.netQuantity, previous: previous?.totals.netQuantity, yearAgo: yearAgo?.totals.netQuantity, format: formatQuantity },
    { label: labels.transactions, current: current.totals.transactionCount, previous: previous?.totals.transactionCount, yearAgo: yearAgo?.totals.transactionCount, format: formatQuantity },
    { label: labels.basket, current: basketNumber(current.totals), previous: basketNumber(previous?.totals), yearAgo: basketNumber(yearAgo?.totals), format: formatMoney },
  ];
  rows.forEach((row, index) => {
    const rowY = tableY + 8.2 + index * 6.6;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 4.2, innerWidth, 6.2, 1.2, 1.2, 'F');
    }
    let cellX = innerX;
    setText(doc, COLORS.ink, 7.4, 'bold');
    doc.text(fitText(doc, row.label, columns[0] - 3), cellX + 2, rowY);
    cellX += columns[0];
    const amounts = [row.current, row.previous, row.yearAgo];
    amounts.forEach((value, valueIndex) => {
      setText(doc, valueIndex === 0 ? COLORS.ink : COLORS.slate, 7.2, valueIndex === 0 ? 'bold' : 'normal');
      doc.text(value === undefined ? '-' : row.format(value), cellX + columns[valueIndex + 1] - 2, rowY, { align: 'right' });
      cellX += columns[valueIndex + 1];
    });
    [delta(row.current, row.previous), delta(row.current, row.yearAgo)].forEach((change, changeIndex) => {
      const columnWidth = columns[4 + changeIndex];
      setText(doc, change === undefined ? COLORS.slate : change >= 0 ? COLORS.positive : COLORS.negative, 7.2, 'bold');
      doc.text(change === undefined ? '-' : formatPercent(change), cellX + columnWidth - 2, rowY, { align: 'right' });
      cellX += columnWidth;
    });
  });
}

function drawCategoryPerformancePanel(doc: jsPDF, x: number, y: number, width: number, height: number, current: StorePeriod, previous: StorePeriod | undefined, yearAgo: StorePeriod | undefined, level: SalesReportCategoryLevel, labels: Labels, locale: Locale): void {
  card(doc, x, y, width, height);
  panelTitle(doc, x, y, width, labels.categoryPerformance);
  const currentGroups = (current.amountGroups ?? categoryGroups(current.items, level, 'amount', labels.uncategorized)).slice(0, 6);
  const previousMap = previous?.amountGroups
    ? new Map(previous.amountGroups.map((group) => [group.id, group]))
    : categoryGroupMap(previous?.items ?? [], level, labels.uncategorized);
  const yearAgoMap = yearAgo?.amountGroups
    ? new Map(yearAgo.amountGroups.map((group) => [group.id, group]))
    : categoryGroupMap(yearAgo?.items ?? [], level, labels.uncategorized);
  const innerX = x + 4;
  const tableY = y + 18;
  const innerWidth = width - 8;
  const columns = [92, 38, 38, 32, 38, innerWidth - 238];
  drawTableHeader(doc, innerX, tableY, columns, [labels.category, labels.current, labels.previous, labels.vsPrevious, labels.yearAgo, labels.vsYearAgo]);
  currentGroups.forEach((group, index) => {
    const rowY = tableY + 9.4 + index * 8.6;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 5, innerWidth, 8, 1.2, 1.2, 'F');
    }
    setText(doc, COLORS.ink, 7.6, 'bold');
    doc.text(fitText(doc, categoryLabel(group, locale), columns[0] - 4), innerX + 2, rowY);
    const previousAmount = previousMap.get(group.id)?.amount;
    const yearAgoAmount = yearAgoMap.get(group.id)?.amount;
    const values: Array<{ text: string; color: RGB; bold?: boolean }> = [
      { text: formatMoney(group.amount), color: COLORS.ink, bold: true },
      { text: previousAmount === undefined ? '-' : formatMoney(previousAmount), color: COLORS.slate },
      percentCell(delta(group.amount, previousAmount)),
      { text: yearAgoAmount === undefined ? '-' : formatMoney(yearAgoAmount), color: COLORS.slate },
      percentCell(delta(group.amount, yearAgoAmount)),
    ];
    let cellX = innerX + columns[0];
    values.forEach((value, valueIndex) => {
      setText(doc, value.color, 7.1, value.bold ? 'bold' : 'normal');
      doc.text(value.text, cellX + columns[valueIndex + 1] - 2, rowY, { align: 'right' });
      cellX += columns[valueIndex + 1];
    });
  });
}

function percentCell(change: number | undefined): { text: string; color: RGB; bold?: boolean } {
  if (change === undefined) return { text: '-', color: COLORS.slate };
  return { text: formatPercent(change), color: change >= 0 ? COLORS.positive : COLORS.negative, bold: true };
}

const FOCUS_GROUP_ORDER = ['health', 'skin', 'pc'] as const;

export function focusReportCards(groups: FocusGroup[], catalog: boolean): Array<FocusGroup | undefined> {
  if (catalog) return groups;
  return FOCUS_GROUP_ORDER.map((id) => groups.find((group) => group.id === id));
}

function drawFocusPage(
  doc: jsPDF,
  yearAgoNext: StorePeriod,
  current: StorePeriod,
  storeId: string,
  storeLabel: string,
  labels: Labels,
  title = labels.focusTitle,
): void {
  drawPageHeader(doc, title, `${yearAgoNext.from} - ${yearAgoNext.to}`, storeId, storeLabel);
  const groups = yearAgoNext.focusGroups
    ?? (yearAgoNext.focusCatalog ? [] : buildFocusGroups(yearAgoNext.items, current.items, 8));
  const titles: Record<string, string> = {
    health: labels.focusHealth,
    skin: labels.focusSkin,
    pc: labels.focusPC,
  };
  const usingCatalog = Boolean(yearAgoNext.focusCatalog) || groups.some((group) => Boolean(group.name));
  const cards = focusReportCards(groups, usingCatalog);
  if (cards.length === 0) return;
  const gap = 4;
  const cardWidth = (277 - gap * 2) / 3;
  for (let start = 0; start < cards.length; start += 3) {
    if (start > 0) {
      doc.addPage();
      drawPageHeader(doc, title, `${yearAgoNext.from} - ${yearAgoNext.to}`, storeId, storeLabel);
    }
    const slice = cards.slice(start, start + 3);
    const count = usingCatalog ? slice.length : 3;
    for (let index = 0; index < count; index += 1) {
      const group = slice[index];
      const fallbackId = FOCUS_GROUP_ORDER[start + index] ?? group?.id ?? '';
      const title = group?.name || titles[group?.id ?? fallbackId] || fallbackId;
      drawFocusGroupCard(doc, 10 + index * (cardWidth + gap), 30, cardWidth, 160, title, group, labels);
    }
  }
}

function drawFocusGroupCard(
  doc: jsPDF,
  x: number,
  y: number,
  width: number,
  height: number,
  title: string,
  group: FocusGroup | undefined,
  labels: Labels,
): void {
  card(doc, x, y, width, height);
  setFill(doc, COLORS.tealSoft);
  doc.roundedRect(x, y, width, 11, 2, 2, 'F');
  setText(doc, COLORS.ink, 8, 'bold');
  doc.text(fitText(doc, title, width - 8), x + 4, y + 7.2);
  drawFocusList(doc, x + 3, y + 16, width - 6, labels.focusSales, group?.sales ?? [], 'amount', labels);
  drawFocusList(doc, x + 3, y + 88, width - 6, labels.focusQuantity, group?.quantity ?? [], 'quantity', labels);
}

function drawFocusList(
  doc: jsPDF,
  x: number,
  y: number,
  width: number,
  title: string,
  items: FocusProduct[],
  metric: 'amount' | 'quantity',
  labels: Labels,
): void {
  setText(doc, COLORS.teal, 6.4, 'bold');
  doc.text(title, x, y);
  const headerY = y + 5;
  const columns = [6, width - 40, 18, 16];
  setText(doc, COLORS.slate, 5.2, 'bold');
  doc.text('#', x + 1, headerY);
  doc.text(labels.product, x + columns[0] + 1, headerY);
  doc.text(metric === 'amount' ? labels.amount : labels.quantity, x + columns[0] + columns[1] + columns[2] - 1, headerY, { align: 'right' });
  doc.text(metric === 'amount' ? labels.quantity : labels.amount, x + columns.reduce((sum, value) => sum + value, 0) - 1, headerY, { align: 'right' });
  setDraw(doc, COLORS.line);
  doc.line(x, headerY + 1.4, x + width, headerY + 1.4);
  const rows = items.slice(0, 8);
  if (rows.length === 0) {
    setText(doc, COLORS.slate, 6, 'normal');
    doc.text('-', x + width / 2, headerY + 16, { align: 'center' });
    return;
  }
  rows.forEach((item, index) => {
    const rowY = headerY + 6 + index * 6.6;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.rect(x, rowY - 3.6, width, 6.2, 'F');
    }
    setText(doc, index < 3 ? COLORS.teal : COLORS.slate, 5.6, 'bold');
    doc.text(String(index + 1), x + 1, rowY);
    setText(doc, COLORS.ink, 5.8, 'normal');
    doc.text(fitText(doc, item.name || item.code || '-', columns[1] - 2), x + columns[0] + 1, rowY);
    const primary = metric === 'amount' ? compactMoney(item.amount) : formatQuantity(item.quantity);
    const secondary = metric === 'amount' ? formatQuantity(item.quantity) : compactMoney(item.amount);
    setText(doc, COLORS.ink, 5.6, 'bold');
    doc.text(primary, x + columns[0] + columns[1] + columns[2] - 1, rowY, { align: 'right' });
    setText(doc, COLORS.slate, 5.3, 'normal');
    doc.text(secondary, x + columns.reduce((sum, value) => sum + value, 0) - 1, rowY, { align: 'right' });
  });
}

function drawRankingPanel(
  doc: jsPDF,
  x: number,
  y: number,
  width: number,
  height: number,
  title: string,
  items: RankedItem[],
  metric: Metric,
  level: SalesReportCategoryLevel,
  labels: Labels,
  locale: Locale,
  compact = false,
  rankOffset = 0,
): void {
  card(doc, x, y, width, height);
  panelTitle(doc, x, y, width, title);
  const itemsById = new Map(items.map((item) => [item.id, item]));
  const innerX = x + 4;
  const headerY = y + 19;
  const columns = [8, width - 79, 34, 29];
  const firstOffset = compact ? 6 : 8;
  const row = compact ? 5.35 : 8.55;
  const barHeight = compact ? 4.8 : 7.7;
  const barOffset = compact ? 3.2 : 4.7;
  drawTableHeader(doc, innerX, headerY, columns, ['#', labels.product, labels.amount, labels.quantity]);
  items.forEach((item, index) => {
    const rowY = headerY + firstOffset + index * row;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - barOffset, width - 8, barHeight, 1.2, 1.2, 'F');
    }
    const rank = rankOffset + index;
    setText(doc, rank < 3 ? COLORS.teal : COLORS.slate, compact ? 6.2 : 7, 'bold');
    doc.text(String(rank + 1), innerX + 2, rowY);
    setText(doc, COLORS.ink, compact ? 6.2 : 6.8, 'bold');
    doc.text(fitText(doc, item.name || item.code || '-', columns[1] - 3), innerX + columns[0] + 1, compact ? rowY : rowY - 1.1);
    if (!compact) {
      setText(doc, COLORS.slate, 5.3, 'normal');
      const meta = [item.code, item.brand].filter(Boolean).join(' / ');
      doc.text(fitText(doc, meta || categoryForRankedItem(itemsById.get(item.id), level, locale), columns[1] - 3), innerX + columns[0] + 1, rowY + 2.3);
    }
    const amountX = innerX + columns[0] + columns[1] + columns[2] - 2;
    const quantityX = innerX + columns.reduce((sum, value) => sum + value, 0) - 2;
    const valueY = compact ? rowY : rowY + 0.7;
    setText(doc, metric === 'amount' ? COLORS.ink : COLORS.slate, compact ? 6.2 : 6.8, metric === 'amount' ? 'bold' : 'normal');
    doc.text(compactMoney(item.amount), amountX, valueY, { align: 'right' });
    setText(doc, metric === 'quantity' ? COLORS.ink : COLORS.slate, compact ? 6.2 : 6.8, metric === 'quantity' ? 'bold' : 'normal');
    doc.text(formatQuantity(item.quantity), quantityX, valueY, { align: 'right' });
  });
}

function drawCategoryCard(doc: jsPDF, x: number, y: number, width: number, height: number, group: CategoryGroup, metric: Metric, labels: Labels, locale: Locale, rankOffset = 0): void {
  card(doc, x, y, width, height);
  setFill(doc, COLORS.tealSoft);
  doc.roundedRect(x, y, width, 11, 2, 2, 'F');
  setText(doc, COLORS.ink, 7.7, 'bold');
  doc.text(fitText(doc, categoryLabel(group, locale), width - 31), x + 3, y + 7.1);
  setText(doc, COLORS.teal, 6.6, 'bold');
  doc.text(metric === 'amount' ? compactMoney(group.amount) : formatQuantity(group.quantity), x + width - 3, y + 7.1, { align: 'right' });

  const metrics = categoryCardRowMetrics(height);
  const innerX = x + 2.5;
  const columns = [6, width - 50, 23, 16];
  const headerY = y + 15;
  setText(doc, COLORS.slate, 5.3, 'bold');
  doc.text('#', innerX + 1, headerY);
  doc.text(labels.product, innerX + columns[0] + 1, headerY);
  doc.text(metric === 'amount' ? labels.amount : labels.quantity, innerX + columns[0] + columns[1] + columns[2] - 1, headerY, { align: 'right' });
  doc.text(metric === 'amount' ? labels.quantity : labels.amount, innerX + columns.reduce((sum, value) => sum + value, 0) - 1, headerY, { align: 'right' });
  setDraw(doc, COLORS.line);
  doc.line(innerX, headerY + 1.6, x + width - 2.5, headerY + 1.6);

  group.items.slice(0, metrics.limit).forEach((item, index) => {
    const rowY = headerY + 5.2 + index * metrics.row;
    const rank = rankOffset + index;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.rect(innerX, rowY - 2.5, width - 5, metrics.row - 0.17, 'F');
    }
    setText(doc, rank < 3 ? COLORS.teal : COLORS.slate, 5.5, 'bold');
    doc.text(String(rank + 1), innerX + 1, rowY);
    setText(doc, COLORS.ink, metrics.nameSize, 'normal');
    doc.text(fitText(doc, item.name || item.code || '-', columns[1] - 2), innerX + columns[0] + 1, rowY);
    const primary = metric === 'amount' ? compactMoney(item.amount) : formatQuantity(item.quantity);
    const secondary = metric === 'amount' ? formatQuantity(item.quantity) : compactMoney(item.amount);
    setText(doc, COLORS.ink, metrics.numberSize, 'bold');
    doc.text(primary, innerX + columns[0] + columns[1] + columns[2] - 1, rowY, { align: 'right' });
    setText(doc, COLORS.slate, 5.3, 'normal');
    doc.text(secondary, innerX + columns.reduce((sum, value) => sum + value, 0) - 1, rowY, { align: 'right' });
  });
}

export function categoryCardRowMetrics(height: number): { row: number; nameSize: number; numberSize: number; limit: number } {
  const compact = height < 100;
  const row = compact ? 3.72 : 4.35;
  const nameSize = compact ? 5.7 : 6.4;
  const numberSize = compact ? 5.6 : 6.2;
  return {
    row,
    nameSize,
    numberSize,
    limit: Math.max(0, Math.floor(1 + (height - 23.45) / row)),
  };
}

function panelTitle(doc: jsPDF, x: number, y: number, width: number, title: string): void {
  setText(doc, COLORS.ink, 9.5, 'bold');
  doc.text(fitText(doc, title, width - 10), x + 4, y + 10);
  setFill(doc, COLORS.teal);
  doc.roundedRect(x + 4, y + 13.5, 19, 1.5, 0.75, 0.75, 'F');
}

function drawTableHeader(doc: jsPDF, x: number, y: number, columns: number[], headers: string[]): void {
  setText(doc, COLORS.slate, 6, 'bold');
  let currentX = x;
  headers.forEach((header, index) => {
    doc.text(header, index === 0 ? currentX + 2 : currentX + columns[index] - 2, y, { align: index === 0 ? 'left' : 'right' });
    currentX += columns[index];
  });
  setDraw(doc, COLORS.line);
  doc.line(x, y + 2.2, x + columns.reduce((sum, value) => sum + value, 0), y + 2.2);
}

function card(doc: jsPDF, x: number, y: number, width: number, height: number): void {
  setFill(doc, COLORS.white);
  setDraw(doc, COLORS.line);
  doc.roundedRect(x, y, width, height, 2.2, 2.2, 'FD');
}

function addFooters(doc: jsPDF, storeId: string, chapters: FooterChapter[] = []): void {
  const pages = doc.getNumberOfPages();
  for (let page = 1; page <= pages; page += 1) {
    doc.setPage(page);
    const chapter = chapters.find((entry) => page >= entry.start && page <= entry.end);
    const suffix = chapter?.label ? `  |  ${chapter.label}` : '';
    setDraw(doc, COLORS.line);
    doc.line(10, 199.5, 287, 199.5);
    setText(doc, COLORS.slate, 6, 'normal');
    doc.text(`RTA Sales Analysis  |  ${storeId}${suffix}`, 10, 204);
    doc.text(`Page ${page} / ${pages}`, 287, 204, { align: 'right' });
  }
}

function rankItems(items: SalesAnalysisItem[], metric: Metric): RankedItem[] {
  const grouped = new Map<string, RankedItem>();
  for (const item of items) {
    const id = item.articleCode.trim() || item.articleName.trim();
    if (!id) continue;
    const ranked = grouped.get(id) ?? {
      id, code: item.articleCode.trim(), name: item.articleName.trim(), brand: item.brandName?.trim() ?? '', amount: 0, quantity: 0,
    };
    ranked.amount += item.netSalesAmount;
    ranked.quantity += item.netQuantity;
    grouped.set(id, ranked);
  }
  return [...grouped.values()].sort((left, right) =>
    (metric === 'amount' ? right.amount - left.amount : right.quantity - left.quantity)
      || left.id.localeCompare(right.id, undefined, { numeric: true }),
  );
}

function categoryGroups(items: SalesAnalysisItem[], level: SalesReportCategoryLevel, metric: Metric, uncategorized: string, rankingLimit: number = DEFAULT_RANKING_LIMIT): CategoryGroup[] {
  const limit = normalizeRankingLimit(rankingLimit);
  const grouped = new Map<string, { code: string; name: string; amount: number; quantity: number; source: SalesAnalysisItem[] }>();
  for (const item of items) {
    const code = categoryCode(item, level);
    const name = categoryName(item, level) || uncategorized;
    const id = code || name;
    const group = grouped.get(id) ?? { code, name, amount: 0, quantity: 0, source: [] };
    group.amount += item.netSalesAmount;
    group.quantity += item.netQuantity;
    group.source.push(item);
    grouped.set(id, group);
  }
  return [...grouped.entries()].map(([id, group]) => ({
    id, code: group.code, name: group.name, amount: group.amount, quantity: group.quantity,
    items: rankItems(group.source, metric).slice(0, limit),
  })).filter((group) => group.items.length > 0).sort((left, right) =>
    (metric === 'amount' ? right.amount - left.amount : right.quantity - left.quantity)
      || left.id.localeCompare(right.id, undefined, { numeric: true }),
  );
}

function categoryGroupMap(items: SalesAnalysisItem[], level: SalesReportCategoryLevel, uncategorized: string): Map<string, CategoryGroup> {
  return new Map(categoryGroups(items, level, 'amount', uncategorized).map((group) => [group.id, group]));
}

function categoryCode(item: SalesAnalysisItem, level: SalesReportCategoryLevel): string {
  const key = `${level}Code` as keyof SalesAnalysisItem;
  const value = item[key];
  return typeof value === 'string' ? value.trim() : '';
}

function categoryName(item: SalesAnalysisItem, level: SalesReportCategoryLevel): string {
  const value = item[level];
  return typeof value === 'string' ? value.trim() : '';
}

function categoryLabel(group: Pick<CategoryGroup, 'code' | 'name'>, _locale: Locale): string {
  if (!group.code) return group.name;
  if (!group.name) return group.code;
  return `${group.code}  ${group.name}`;
}

function categoryForRankedItem(_item: RankedItem | undefined, _level: SalesReportCategoryLevel, _locale: Locale): string {
  return '';
}

function reportLabels(locale: Locale, rankingLimit: number = DEFAULT_RANKING_LIMIT): Labels {
  const limit = normalizeRankingLimit(rankingLimit);
  if (locale === 'en') {
    return {
      title: 'Store sales analysis', summary: 'Sales summary', period: 'Period', generated: 'Generated',
      netSales: 'Net sales', netQuantity: 'Net quantity', transactions: 'Transactions', basket: 'Average basket',
      comparison: 'Performance comparison', metric: 'Metric', current: 'Current', previous: 'Previous', yearAgo: 'Year ago',
      vsPrevious: 'vs previous', vsYearAgo: 'vs year ago',
      focusTitle: 'Watch next', focusHealth: 'Health', focusSkin: 'Skin', focusPC: 'Personal care',
      focusSales: 'Top 10 by sales', focusQuantity: 'Top 10 by quantity',
      categoryPerformance: 'Category performance', category: 'Category',
      topSales: `Top ${limit} by sales`, topQuantity: `Top ${limit} by quantity`, salesRanking: 'Category sales ranking',
      quantityRanking: 'Category quantity ranking', product: 'Product', amount: 'Sales', quantity: 'Qty', uncategorized: 'Uncategorized',
      allStores: 'All stores', localTotal: 'Local total', touristTotal: 'Tourist total',
      storeComparison: 'Store comparison', store: 'Store',
      groupSummary: 'Group summary', group: 'Group',
      weeklyTitle: 'Weekly sales change', week: 'Week', thisWeek: 'This week', lastWeek: 'Last week',
      variance: 'Var', variancePercent: 'Var %', weekday: 'Weekday', weekend: 'Weekend', customers: 'Txns',
    };
  }
  return {
    title: '門店銷售分析', summary: '銷售摘要', period: '分析期間', generated: '產生時間',
    netSales: '淨銷售額', netQuantity: '淨銷售數量', transactions: '交易次數', basket: '客單價',
    comparison: '銷售表現', metric: '指標', current: '本期', previous: '上期', yearAgo: '去年同期',
    vsPrevious: '較上期', vsYearAgo: '較去年同期',
    focusTitle: '接下來關注', focusHealth: '保健', focusSkin: '護膚', focusPC: '個護',
    focusSales: '銷售額 Top 10', focusQuantity: '銷量 Top 10',
    categoryPerformance: '分類表現', category: '分類',
    topSales: `銷售額 Top ${limit}`, topQuantity: `銷量 Top ${limit}`, salesRanking: '分類商品銷售排行',
    quantityRanking: '分類商品銷量排行', product: '商品', amount: '銷售額', quantity: '銷量', uncategorized: '未分類',
    allStores: '全部門店', localTotal: '本地合計', touristTotal: '旅客合計',
    storeComparison: '門店比較', store: '門店',
    groupSummary: '群組總結', group: '群組',
    weeklyTitle: '每週銷售變化', week: '週次', thisWeek: '本週', lastWeek: '上週',
    variance: '差異', variancePercent: '差異 %', weekday: '平日', weekend: '週末', customers: '交易',
  };
}

function setText(doc: jsPDF, color: RGB, size: number, _style: 'normal' | 'bold'): void {
  doc.setTextColor(...color);
  doc.setFont(FONT, 'normal');
  doc.setFontSize(size);
}

function setFill(doc: jsPDF, color: RGB): void {
  doc.setFillColor(...color);
}

function setDraw(doc: jsPDF, color: RGB): void {
  doc.setDrawColor(...color);
  doc.setLineWidth(0.25);
}

function fitText(doc: jsPDF, value: string, width: number): string {
  const text = value.trim() || '-';
  if (doc.getTextWidth(text) <= width) return text;
  const characters = Array.from(text);
  while (characters.length > 1 && doc.getTextWidth(`${characters.join('')}…`) > width) characters.pop();
  return `${characters.join('')}…`;
}

function formatMoney(value: number): string {
  return `HK$${formatFixed(value, 2)}`;
}

function compactMoney(value: number): string {
  const absolute = Math.abs(value);
  if (absolute >= 1_000_000) return `HK$${(value / 1_000_000).toFixed(2)}M`;
  if (absolute >= 100_000) return `HK$${(value / 1_000).toFixed(0)}K`;
  return `HK$${formatFixed(value, absolute >= 1_000 ? 0 : 2)}`;
}

function formatQuantity(value: number): string {
  return formatFixed(Math.round(value), 0);
}

function optionalQuantity(value: number | undefined): string {
  return value === undefined ? '-' : formatQuantity(value);
}

function formatFixed(value: number, digits: number): string {
  return new Intl.NumberFormat('en-HK', { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(value);
}

function basketNumber(totals: SalesAnalysisTotals | undefined): number | undefined {
  if (!totals?.transactionCount || totals.trendNetSalesAmount === undefined) return undefined;
  return totals.trendNetSalesAmount / totals.transactionCount;
}

function basketValue(totals: SalesAnalysisTotals): string {
  const value = basketNumber(totals);
  return value === undefined ? '-' : formatMoney(value);
}

function delta(current: number | undefined, comparison: number | undefined): number | undefined {
  if (current === undefined || comparison === undefined || comparison === 0) return undefined;
  return (current - comparison) / Math.abs(comparison);
}

function formatPercent(value: number): string {
  return `${value >= 0 ? '+' : ''}${(value * 100).toFixed(1)}%`;
}
