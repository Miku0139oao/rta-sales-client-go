import type { jsPDF } from 'jspdf';
import notoSansTCURL from './assets/NotoSansTC-Regular.ttf?url';
import type {
  Locale,
  SalesAnalysisItem,
  SalesAnalysisPeriodResult,
  SalesAnalysisResult,
  SalesAnalysisStore,
  SalesAnalysisTotals,
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
}

const FONT = 'NotoSansTC';
const PAGE_WIDTH = 297;
const PAGE_HEIGHT = 210;
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

let fontBase64Promise: Promise<string> | undefined;

export async function generateSalesAnalysisPDF(
  result: SalesAnalysisResult,
  storeId: string,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
): Promise<Uint8Array> {
  const [fontBase64, { jsPDF: PDFDocument }] = await Promise.all([loadFontBase64(), import('jspdf')]);
  return renderSalesAnalysisPDF(result, storeId, categoryLevel, locale, fontBase64, PDFDocument);
}

export async function buildSalesAnalysisPDF(
  result: SalesAnalysisResult,
  storeId: string,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  fontBase64: string,
): Promise<Uint8Array> {
  const { jsPDF: PDFDocument } = await import('jspdf');
  return renderSalesAnalysisPDF(result, storeId, categoryLevel, locale, fontBase64, PDFDocument);
}

function renderSalesAnalysisPDF(
  result: SalesAnalysisResult,
  storeId: string,
  categoryLevel: SalesReportCategoryLevel,
  locale: Locale,
  fontBase64: string,
  PDFDocument: typeof import('jspdf').jsPDF,
): Uint8Array {
  const labels = reportLabels(locale);
  const periods = storePeriods(result, storeId);
  const current = periodByKey(periods, 'current') ?? periods[0];
  if (!current) throw new Error('The selected store has no current sales period');

  const store = listSuccessfulReportStores(result).find((candidate) => candidate.businessId === storeId);
  const storeLabel = store?.label.trim() || storeId;
  const doc = new PDFDocument({
    orientation: 'landscape', unit: 'mm', format: 'a4', compress: true, putOnlyUsedFonts: true,
  });
  doc.addFileToVFS('NotoSansTC-Regular.ttf', fontBase64);
  doc.addFont('NotoSansTC-Regular.ttf', FONT, 'normal');
  doc.addFont('NotoSansTC-Regular.ttf', FONT, 'bold');
  doc.setFont(FONT, 'normal');
  doc.setProperties({
    title: `RTA Sales Analysis - ${storeId}`,
    subject: `${current.from} - ${current.to}`,
    author: 'RTA Excel Filler',
    creator: 'RTA Excel Filler',
  });

  drawSummaryPage(doc, periods, current, storeId, storeLabel, categoryLevel, labels, locale);
  doc.addPage();
  drawOverallRankingsPage(doc, current, storeId, storeLabel, categoryLevel, labels, locale);

  for (const key of ['current', 'yearAgo', 'yearAgoNext']) {
    const period = periodByKey(periods, key);
    if (!period) continue;
    doc.addPage();
    drawCategoryRankingPage(doc, period, storeId, storeLabel, categoryLevel, 'amount', labels, locale);
  }
  for (const key of ['current', 'previous', 'previous2']) {
    const period = periodByKey(periods, key);
    if (!period) continue;
    doc.addPage();
    drawCategoryRankingPage(doc, period, storeId, storeLabel, categoryLevel, 'quantity', labels, locale);
  }

  addFooters(doc, storeId);
  return new Uint8Array(doc.output('arraybuffer'));
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

export function salesAnalysisPDFFilename(storeId: string, from: string, to: string): string {
  const safeStore = storeId.trim().replace(/[^\p{L}\p{N}_-]+/gu, '-') || 'store';
  const start = from.replaceAll('-', '') || 'report';
  const end = to.replaceAll('-', '');
  const period = end && end !== start ? `${start}-${end}` : start;
  return `RTA-Sales-${safeStore}-${period}.pdf`;
}

export function bytesToBase64(bytes: Uint8Array): string {
  let binary = '';
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  return btoa(binary);
}

async function loadFontBase64(): Promise<string> {
  if (!fontBase64Promise) {
    fontBase64Promise = fetch(notoSansTCURL).then(async (response) => {
      if (!response.ok) throw new Error(`Unable to load report font (${response.status})`);
      return bytesToBase64(new Uint8Array(await response.arrayBuffer()));
    });
  }
  return fontBase64Promise;
}

function normalizedPeriods(result: SalesAnalysisResult): SalesAnalysisPeriodResult[] {
  if (result.periods?.length) return result.periods;
  return [{
    key: 'current', label: 'Current', from: result.from, to: result.to,
    complete: result.complete, successfulStores: result.successfulStores,
    totals: result.totals, stores: result.stores, items: result.items, issues: result.issues,
  }];
}

function storePeriods(result: SalesAnalysisResult, storeId: string): StorePeriod[] {
  return normalizedPeriods(result).flatMap((period) => {
    const items = period.items.filter((item) => item.storeId === storeId);
    const summary = period.stores.find((store) => store.businessId === storeId);
    if (!summary && items.length === 0 && period.key !== 'current') return [];
    return [{
      key: period.key, label: period.label, from: period.from, to: period.to,
      totals: summary?.totals ?? aggregateTotals(items), items,
    }];
  });
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
): void {
  drawPageHeader(doc, labels.summary, `${current.from} - ${current.to}`, storeId, storeLabel);
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

  drawComparisonPanel(doc, 10, 62, 132, 124, current, previous, yearAgo, labels);
  drawCategoryPerformancePanel(doc, 147, 62, 140, 124, current, previous, yearAgo, level, labels, locale);
}

function drawOverallRankingsPage(
  doc: jsPDF,
  current: StorePeriod,
  storeId: string,
  storeLabel: string,
  level: SalesReportCategoryLevel,
  labels: Labels,
  locale: Locale,
): void {
  drawPageHeader(doc, `${labels.topSales} / ${labels.topQuantity}`, `${current.from} - ${current.to}`, storeId, storeLabel);
  drawRankingPanel(doc, 10, 31, 136.5, 159, labels.topSales, rankItems(current.items, 'amount').slice(0, 15), 'amount', level, labels, locale);
  drawRankingPanel(doc, 150.5, 31, 136.5, 159, labels.topQuantity, rankItems(current.items, 'quantity').slice(0, 15), 'quantity', level, labels, locale);
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
): void {
  const title = `${period.label} - ${metric === 'amount' ? labels.salesRanking : labels.quantityRanking}`;
  drawPageHeader(doc, title, `${period.from} - ${period.to}`, storeId, storeLabel);
  const groups = categoryGroups(period.items, level, metric, labels.uncategorized).slice(0, 6);
  const gap = 4;
  const cardWidth = (277 - gap * 2) / 3;
  const cardHeight = 76.5;
  for (let index = 0; index < 6; index += 1) {
    const column = index % 3;
    const row = Math.floor(index / 3);
    drawCategoryCard(
      doc,
      10 + column * (cardWidth + gap),
      30 + row * (cardHeight + 4),
      cardWidth,
      cardHeight,
      groups[index],
      metric,
      labels,
      locale,
    );
  }
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
  const tableY = y + 19;
  const innerWidth = width - 8;
  const columns = [32, 23, 23, 23, innerWidth - 101];
  drawTableHeader(doc, innerX, tableY, columns, [labels.metric, labels.current, labels.previous, labels.yearAgo, labels.vsPrevious]);
  const rows = [
    { label: labels.netSales, current: current.totals.netSalesAmount, previous: previous?.totals.netSalesAmount, yearAgo: yearAgo?.totals.netSalesAmount, format: formatMoney },
    { label: labels.netQuantity, current: current.totals.netQuantity, previous: previous?.totals.netQuantity, yearAgo: yearAgo?.totals.netQuantity, format: formatQuantity },
    { label: labels.transactions, current: current.totals.transactionCount, previous: previous?.totals.transactionCount, yearAgo: yearAgo?.totals.transactionCount, format: formatQuantity },
    { label: labels.basket, current: basketNumber(current.totals), previous: basketNumber(previous?.totals), yearAgo: basketNumber(yearAgo?.totals), format: formatMoney },
  ];
  rows.forEach((row, index) => {
    const rowY = tableY + 10 + index * 21;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 6, innerWidth, 17, 1.5, 1.5, 'F');
    }
    let cellX = innerX;
    setText(doc, COLORS.ink, 7.4, 'bold');
    doc.text(fitText(doc, row.label, columns[0] - 3), cellX + 2, rowY + 3);
    cellX += columns[0];
    const values = [row.current, row.previous, row.yearAgo];
    values.forEach((value, valueIndex) => {
      setText(doc, valueIndex === 0 ? COLORS.ink : COLORS.slate, 7.1, valueIndex === 0 ? 'bold' : 'normal');
      doc.text(value === undefined ? '-' : row.format(value), cellX + columns[valueIndex + 1] - 2, rowY + 3, { align: 'right' });
      cellX += columns[valueIndex + 1];
    });
    const change = delta(row.current, row.previous);
    setText(doc, change === undefined ? COLORS.slate : change >= 0 ? COLORS.positive : COLORS.negative, 7.1, 'bold');
    doc.text(change === undefined ? '-' : formatPercent(change), innerX + innerWidth - 2, rowY + 3, { align: 'right' });
  });
}

function drawCategoryPerformancePanel(doc: jsPDF, x: number, y: number, width: number, height: number, current: StorePeriod, previous: StorePeriod | undefined, yearAgo: StorePeriod | undefined, level: SalesReportCategoryLevel, labels: Labels, locale: Locale): void {
  card(doc, x, y, width, height);
  panelTitle(doc, x, y, width, labels.categoryPerformance);
  const currentGroups = categoryGroups(current.items, level, 'amount', labels.uncategorized).slice(0, 6);
  const previousMap = categoryGroupMap(previous?.items ?? [], level, labels.uncategorized);
  const yearAgoMap = categoryGroupMap(yearAgo?.items ?? [], level, labels.uncategorized);
  const innerX = x + 4;
  const tableY = y + 19;
  const innerWidth = width - 8;
  const columns = [innerWidth - 84, 28, 28, 28];
  drawTableHeader(doc, innerX, tableY, columns, [labels.category, labels.current, labels.previous, labels.yearAgo]);
  currentGroups.forEach((group, index) => {
    const rowY = tableY + 10 + index * 14;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 5.5, innerWidth, 11.5, 1.5, 1.5, 'F');
    }
    setText(doc, COLORS.ink, 7, 'bold');
    doc.text(fitText(doc, categoryLabel(group, locale), columns[0] - 3), innerX + 2, rowY + 1.7);
    const values = [group.amount, previousMap.get(group.id)?.amount, yearAgoMap.get(group.id)?.amount];
    let cellX = innerX + columns[0];
    values.forEach((value, valueIndex) => {
      setText(doc, valueIndex === 0 ? COLORS.ink : COLORS.slate, 6.7, valueIndex === 0 ? 'bold' : 'normal');
      doc.text(value === undefined ? '-' : compactMoney(value), cellX + columns[valueIndex + 1] - 2, rowY + 1.7, { align: 'right' });
      cellX += columns[valueIndex + 1];
    });
  });
}

function drawRankingPanel(doc: jsPDF, x: number, y: number, width: number, height: number, title: string, items: RankedItem[], metric: Metric, level: SalesReportCategoryLevel, labels: Labels, locale: Locale): void {
  card(doc, x, y, width, height);
  panelTitle(doc, x, y, width, title);
  const itemsById = new Map(items.map((item) => [item.id, item]));
  const innerX = x + 4;
  const headerY = y + 19;
  const columns = [8, width - 79, 34, 29];
  drawTableHeader(doc, innerX, headerY, columns, ['#', labels.product, labels.amount, labels.quantity]);
  items.forEach((item, index) => {
    const rowY = headerY + 8 + index * 8.55;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.roundedRect(innerX, rowY - 4.7, width - 8, 7.7, 1.2, 1.2, 'F');
    }
    setText(doc, index < 3 ? COLORS.teal : COLORS.slate, 7, 'bold');
    doc.text(String(index + 1), innerX + 2, rowY);
    setText(doc, COLORS.ink, 6.8, 'bold');
    doc.text(fitText(doc, item.name || item.code || '-', columns[1] - 3), innerX + columns[0] + 1, rowY - 1.1);
    setText(doc, COLORS.slate, 5.3, 'normal');
    const meta = [item.code, item.brand].filter(Boolean).join(' / ');
    doc.text(fitText(doc, meta || categoryForRankedItem(itemsById.get(item.id), level, locale), columns[1] - 3), innerX + columns[0] + 1, rowY + 2.3);
    const amountX = innerX + columns[0] + columns[1] + columns[2] - 2;
    const quantityX = innerX + columns.reduce((sum, value) => sum + value, 0) - 2;
    setText(doc, metric === 'amount' ? COLORS.ink : COLORS.slate, 6.8, metric === 'amount' ? 'bold' : 'normal');
    doc.text(compactMoney(item.amount), amountX, rowY + 0.7, { align: 'right' });
    setText(doc, metric === 'quantity' ? COLORS.ink : COLORS.slate, 6.8, metric === 'quantity' ? 'bold' : 'normal');
    doc.text(formatQuantity(item.quantity), quantityX, rowY + 0.7, { align: 'right' });
  });
}

function drawCategoryCard(doc: jsPDF, x: number, y: number, width: number, height: number, group: CategoryGroup | undefined, metric: Metric, labels: Labels, locale: Locale): void {
  card(doc, x, y, width, height);
  if (!group) {
    setText(doc, COLORS.slate, 8, 'normal');
    doc.text('-', x + width / 2, y + height / 2, { align: 'center' });
    return;
  }
  setFill(doc, COLORS.tealSoft);
  doc.roundedRect(x, y, width, 11, 2, 2, 'F');
  setText(doc, COLORS.ink, 7.7, 'bold');
  doc.text(fitText(doc, categoryLabel(group, locale), width - 31), x + 3, y + 7.1);
  setText(doc, COLORS.teal, 6.6, 'bold');
  doc.text(metric === 'amount' ? compactMoney(group.amount) : formatQuantity(group.quantity), x + width - 3, y + 7.1, { align: 'right' });

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

  group.items.slice(0, 15).forEach((item, index) => {
    const rowY = headerY + 5.2 + index * 3.72;
    if (index % 2 === 0) {
      setFill(doc, COLORS.surface);
      doc.rect(innerX, rowY - 2.5, width - 5, 3.55, 'F');
    }
    setText(doc, index < 3 ? COLORS.teal : COLORS.slate, 5.5, 'bold');
    doc.text(String(index + 1), innerX + 1, rowY);
    setText(doc, COLORS.ink, 5.7, 'normal');
    doc.text(fitText(doc, item.name || item.code || '-', columns[1] - 2), innerX + columns[0] + 1, rowY);
    const primary = metric === 'amount' ? compactMoney(item.amount) : formatQuantity(item.quantity);
    const secondary = metric === 'amount' ? formatQuantity(item.quantity) : compactMoney(item.amount);
    setText(doc, COLORS.ink, 5.6, 'bold');
    doc.text(primary, innerX + columns[0] + columns[1] + columns[2] - 1, rowY, { align: 'right' });
    setText(doc, COLORS.slate, 5.3, 'normal');
    doc.text(secondary, innerX + columns.reduce((sum, value) => sum + value, 0) - 1, rowY, { align: 'right' });
  });
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

function addFooters(doc: jsPDF, storeId: string): void {
  const pages = doc.getNumberOfPages();
  for (let page = 1; page <= pages; page += 1) {
    doc.setPage(page);
    setDraw(doc, COLORS.line);
    doc.line(10, 199.5, 287, 199.5);
    setText(doc, COLORS.slate, 6, 'normal');
    doc.text(`RTA Sales Analysis  |  ${storeId}`, 10, 204);
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

function categoryGroups(items: SalesAnalysisItem[], level: SalesReportCategoryLevel, metric: Metric, uncategorized: string): CategoryGroup[] {
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
    items: rankItems(group.source, metric),
  })).sort((left, right) =>
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
  return item[level].trim();
}

function categoryLabel(group: Pick<CategoryGroup, 'code' | 'name'>, _locale: Locale): string {
  if (!group.code) return group.name;
  if (!group.name) return group.code;
  return `${group.code}  ${group.name}`;
}

function categoryForRankedItem(_item: RankedItem | undefined, _level: SalesReportCategoryLevel, _locale: Locale): string {
  return '';
}

function reportLabels(locale: Locale): Labels {
  if (locale === 'en') {
    return {
      title: 'Store sales analysis', summary: 'Sales summary', period: 'Period', generated: 'Generated',
      netSales: 'Net sales', netQuantity: 'Net quantity', transactions: 'Transactions', basket: 'Average basket',
      comparison: 'Performance comparison', metric: 'Metric', current: 'Current', previous: 'Previous', yearAgo: 'Year ago',
      vsPrevious: 'vs previous', categoryPerformance: 'Category performance', category: 'Category',
      topSales: 'Top 15 by sales', topQuantity: 'Top 15 by quantity', salesRanking: 'Category sales ranking',
      quantityRanking: 'Category quantity ranking', product: 'Product', amount: 'Sales', quantity: 'Qty', uncategorized: 'Uncategorized',
    };
  }
  return {
    title: '門店銷售分析', summary: '銷售摘要', period: '分析期間', generated: '產生時間',
    netSales: '淨銷售額', netQuantity: '淨銷售數量', transactions: '交易次數', basket: '客單價',
    comparison: '銷售表現', metric: '指標', current: '本期', previous: '上期', yearAgo: '去年同期',
    vsPrevious: '較上期', categoryPerformance: '分類表現', category: '分類',
    topSales: '銷售額 Top 15', topQuantity: '銷量 Top 15', salesRanking: '分類商品銷售排行',
    quantityRanking: '分類商品銷量排行', product: '商品', amount: '銷售額', quantity: '銷量', uncategorized: '未分類',
  };
}

function setText(doc: jsPDF, color: RGB, size: number, style: 'normal' | 'bold'): void {
  doc.setTextColor(...color);
  doc.setFont(FONT, style);
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
  return formatFixed(value, Math.abs(value - Math.round(value)) < 1e-9 ? 0 : 2);
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
