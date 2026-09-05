// Synthetic/local PDF section parser. Does not query RTA.
export interface PdfRankRow {
  rank: number;
  code: string;
  amount?: number;
  quantity?: number;
  compactAmount?: string;
}

export interface PdfPage {
  pageNumber: number;
  text: string;
}

export interface PdfParityExpected {
  netSales?: { current: number; previous: number; yearAgo: number };
  vsPrevious?: string;
  lastSales?: { rank: number; code: string; quantity?: number };
  lastQuantitySample?: { rank: number; code: string };
  topSalesCodes?: string[];
  focusExportCodes?: string[];
  categoryContinuation?: { rank: number; code: string };
  overallContinuationStart?: number;
  categoryContinuationStart?: number;
  overallRows?: { sales: PdfRankRow[]; quantity: PdfRankRow[] };
  categoryRows?: Array<{ period: string; metric: 'sales' | 'quantity'; code: string; rows: PdfRankRow[]; cardSize: number }>;
}

export function parsePdfMoney(text: string): number | undefined {
  const raw = String(text).replace(/,/g, '').trim();
  const million = raw.match(/^HK\$([+-]?\d+(?:\.\d+)?)M$/i);
  if (million) return Number(million[1]) * 1_000_000;
  const thousand = raw.match(/^HK\$([+-]?\d+(?:\.\d+)?)K$/i);
  if (thousand) return Number(thousand[1]) * 1_000;
  const plain = raw.match(/^HK\$([+-]?\d+(?:\.\d+)?)$/i);
  if (plain) return Number(plain[1]);
  const bare = raw.match(/^([+-]?\d+(?:\.\d+)?)$/);
  return bare ? Number(bare[1]) : undefined;
}

export function parsePdfQuantity(text: string): number | undefined {
  const raw = String(text).replace(/,/g, '').trim();
  if (!raw) return undefined;
  const value = Number(raw);
  return Number.isFinite(value) ? value : undefined;
}

const RANK_ROW = /(?:^|\s)(\d{1,3})\s+(?:[A-Za-z\u4e00-\u9fff][^\d]{0,48})?(\d{7})\b/g;

export function parseRankRows(text: string, metric: 'sales' | 'quantity' = 'sales'): PdfRankRow[] {
  const source = String(text);
  const matches = [...source.matchAll(new RegExp(RANK_ROW.source, 'g'))];
  return matches.map((match, index) => {
    const after = source.slice(match.index! + match[0].length, matches[index + 1]?.index ?? source.length);
    const money = /HK\$[+-]?[0-9,.]+(?:M|K)?/.exec(after);
    const valueSide = money ? metric === 'sales' ? after.slice(money.index + money[0].length) : after.slice(0, money.index) : '';
    const qtyMatch = metric === 'sales'
      ? /^\s+([+-]?\d[\d,]*(?:\.\d+)?)(?=\s|$)/.exec(valueSide)
      : /(?:^|\s)([+-]?\d[\d,]*(?:\.\d+)?)\s*$/.exec(valueSide);
    return { rank: Number(match[1]), code: match[2]!, amount: money ? parsePdfMoney(money[0]) : undefined,
      quantity: qtyMatch ? parsePdfQuantity(qtyMatch[1]!) : undefined, compactAmount: money?.[0] };
  });
}

export function pageKind(text: string): 'summary' | 'focus' | 'category-sales' | 'category-quantity' | 'overall' | 'other' {
  const source = String(text);
  if (/銷售摘要|Sales summary/.test(source)) return 'summary';
  if (/接下來關注|Watch next/.test(source)) return 'focus';
  if (/分類商品銷售排行|Category sales ranking/.test(source)) return 'category-sales';
  if (/分類商品銷量排行|Category quantity ranking/.test(source)) return 'category-quantity';
  if (/銷售額 Top \d+|Top \d+ by sales/.test(source)) return 'overall';
  return 'other';
}

function splitOn(source: string, marker: RegExp): [string, string] {
  const index = source.search(marker);
  if (index < 0) return [source, ''];
  return [source.slice(0, index), source.slice(index)];
}

export function parseSummary(text: string) {
  const source = String(text);
  const net = source.match(/淨銷售額\s+(HK\$[0-9,.]+)\s+(HK\$[0-9,.]+)\s+(HK\$[0-9,.]+)\s+([+\-][0-9.]+%)\s+([+\-][0-9.]+%)/);
  const qty = source.match(/淨銷售數量\s+([0-9,]+)\s+([0-9,]+)\s+([0-9,]+)\s+([+\-][0-9.]+%)\s+([+\-][0-9.]+%)/);
  const categories: Array<{ code: string; current?: number; previous?: number; yearAgo?: number }> = [];
  const categoryRe = /\b([A-Z]\d{2})\s+\S+\s+(HK\$[0-9,.]+)\s+(HK\$[0-9,.]+)\s+[+\-][0-9.]+%\s+(HK\$[0-9,.]+)/g;
  let match: RegExpExecArray | null;
  while ((match = categoryRe.exec(source))) {
    categories.push({
      code: match[1]!,
      current: parsePdfMoney(match[2]!),
      previous: parsePdfMoney(match[3]!),
      yearAgo: parsePdfMoney(match[4]!),
    });
  }
  return {
    hasPartial: /部分資料尚未完成|missing values are not zero/.test(source),
    netSales: net ? {
      current: parsePdfMoney(net[1]!),
      previous: parsePdfMoney(net[2]!),
      yearAgo: parsePdfMoney(net[3]!),
      vsPrevious: net[4],
      vsYearAgo: net[5],
    } : undefined,
    netQuantity: qty ? {
      current: parsePdfQuantity(qty[1]!),
      previous: parsePdfQuantity(qty[2]!),
      yearAgo: parsePdfQuantity(qty[3]!),
      vsPrevious: qty[4],
      vsYearAgo: qty[5],
    } : undefined,
    categories,
  };
}

export function parseOverallRankings(text: string) {
  const source = String(text);
  const qtyMarker = /銷量 Top \d+|Top \d+ by quantity/;
  const [, afterHeader] = splitOn(source, /銷售額 Top \d+|Top \d+ by sales/);
  const [salesPart, quantityPart] = splitOn(afterHeader.replace(/銷售額 Top \d+ \/ 銷量 Top \d+[^\d]*/, ''), qtyMarker);
  return {
    sales: parseRankRows(salesPart),
    // Overall tables keep amount before quantity even when ranked by quantity.
    quantity: parseRankRows(quantityPart),
  };
}

export function parseFocusGroups(text: string) {
  const source = String(text);
  const groups: Array<{ name: string; sales: PdfRankRow[]; quantity: PdfRankRow[] }> = [];
  const chunks = source.split(/(?=保健|護膚|個護|Health|Skin|Personal care)/);
  for (const chunk of chunks) {
    const name = chunk.match(/^(保健|護膚|個護|Health|Skin|Personal care)/)?.[1];
    if (!name) continue;
    const [salesPart, quantityPart] = splitOn(chunk, /銷量 Top 10|Top 10 by quantity/);
    groups.push({
      name,
      sales: parseRankRows(salesPart),
      quantity: parseRankRows(quantityPart, 'quantity'),
    });
  }
  return groups;
}

export function parseCategoryCards(text: string, metric: 'sales' | 'quantity' = 'sales') {
  const source = String(text);
  const cards: Array<{ code: string; name: string; rows: PdfRankRow[]; firstRank: number; lastRank: number; lastCode: string }> = [];
  const parts = source.split(/(?=\b[A-Z]\d{2}\s+[\u4e00-\u9fff])/);
  for (const part of parts) {
    const header = part.match(/^([A-Z]\d{2})\s+(\S+)/);
    if (!header) continue;
    const rows = parseRankRows(part, metric);
    if (rows.length === 0) continue;
    cards.push({
      code: header[1]!,
      name: header[2]!,
      rows,
      firstRank: rows[0]!.rank,
      lastRank: rows[rows.length - 1]!.rank,
      lastCode: rows[rows.length - 1]!.code,
    });
  }
  return cards;
}

export function extractDocument(pages: PdfPage[]) {
  const summaryPages: Array<PdfPage & { summary: ReturnType<typeof parseSummary> }> = [];
  const overall = { sales: [] as PdfRankRow[], quantity: [] as PdfRankRow[] };
  const focus: ReturnType<typeof parseFocusGroups> = [];
  const categorySales: Array<{ pageNumber: number; period: string; cards: ReturnType<typeof parseCategoryCards> }> = [];
  const categoryQuantity: Array<{ pageNumber: number; period: string; cards: ReturnType<typeof parseCategoryCards> }> = [];
  for (const page of pages) {
    const kind = pageKind(page.text);
    if (kind === 'summary') summaryPages.push({ ...page, summary: parseSummary(page.text) });
    if (kind === 'overall') {
      const parsed = parseOverallRankings(page.text);
      overall.sales.push(...parsed.sales);
      overall.quantity.push(...parsed.quantity);
    }
    if (kind === 'focus') focus.push(...parseFocusGroups(page.text));
    const period = page.text.match(/(?:RTA SALES\s+)?(本期|上期|前期|去年同期|去年下月)\s*-/)?.[1] ?? '';
    if (kind === 'category-sales') {
      categorySales.push({ pageNumber: page.pageNumber, period, cards: parseCategoryCards(page.text) });
    }
    if (kind === 'category-quantity') {
      categoryQuantity.push({ pageNumber: page.pageNumber, period, cards: parseCategoryCards(page.text, 'quantity') });
    }
  }
  return { summaryPages, overall, focus, categorySales, categoryQuantity };
}

export function continuationPageNumbers(pages: PdfPage[], expected?: PdfParityExpected) {
  const numbers: number[] = [];
  const overallStart = expected?.overallContinuationStart ?? 0;
  const categoryStart = expected?.categoryContinuationStart ?? 0;
  for (const page of pages) {
    const kind = pageKind(page.text);
    if (kind === 'overall' && overallStart > 0) {
      const parsed = parseOverallRankings(page.text);
      if (parsed.sales.some((row) => row.rank === overallStart) || parsed.sales.some((row) => row.rank > overallStart)) {
        numbers.push(page.pageNumber);
      }
    }
    if ((kind === 'category-sales' || kind === 'category-quantity') && categoryStart > 0) {
      const cards = parseCategoryCards(page.text);
      if (cards.some((card) => card.firstRank >= categoryStart || card.rows.some((row) => row.rank === categoryStart))) {
        numbers.push(page.pageNumber);
      }
    }
  }
  return [...new Set(numbers)];
}

export function compareParity(
  document: ReturnType<typeof extractDocument>,
  expected: PdfParityExpected,
  options: { requirePartial?: boolean } = {},
) {
  const errors: string[] = [];
  const note = (message: string) => errors.push(message);
  const summary = document.summaryPages[0]?.summary;
  if (options.requirePartial && !summary?.hasPartial) note('missing partial warning');
  if (expected.netSales) {
    if (summary?.netSales?.current !== expected.netSales.current) {
      note(`summary current sales ${summary?.netSales?.current} != ${expected.netSales.current}`);
    }
    if (summary?.netSales?.previous !== expected.netSales.previous) {
      note(`summary previous sales ${summary?.netSales?.previous} != ${expected.netSales.previous}`);
    }
    if (summary?.netSales?.yearAgo !== expected.netSales.yearAgo) {
      note(`summary yearAgo sales ${summary?.netSales?.yearAgo} != ${expected.netSales.yearAgo}`);
    }
    if (expected.vsPrevious && summary?.netSales?.vsPrevious !== expected.vsPrevious) {
      note(`summary vsPrevious ${summary?.netSales?.vsPrevious} != ${expected.vsPrevious}`);
    }
  }
  const lastSales = expected.lastSales;
  if (lastSales) {
    const actual = document.overall.sales.find((row) => row.rank === lastSales.rank);
    if (!actual) note(`missing overall sales rank ${lastSales.rank}`);
    else {
      if (actual.code !== lastSales.code) note(`overall sales rank ${lastSales.rank} code ${actual.code} != ${lastSales.code}`);
      if (lastSales.quantity != null && actual.quantity !== lastSales.quantity) {
        note(`overall sales rank ${lastSales.rank} qty ${actual.quantity} != ${lastSales.quantity}`);
      }
    }
  }
  const lastQty = expected.lastQuantitySample;
  if (lastQty) {
    const actual = document.overall.quantity.find((row) => row.rank === lastQty.rank);
    if (!actual) note(`missing overall quantity rank ${lastQty.rank}`);
    else if (actual.code !== lastQty.code) note(`overall quantity rank ${lastQty.rank} code ${actual.code} != ${lastQty.code}`);
  }
  if (expected.topSalesCodes) {
    const codes = document.overall.sales.map((row) => row.code);
    if (codes.join() !== expected.topSalesCodes.join()) {
      note(`overall sales codes ${codes.join(',')} != ${expected.topSalesCodes.join(',')}`);
    }
  }
  if (expected.focusExportCodes) {
    const health = document.focus.find((group) => group.name === '保健' || group.name === 'Health');
    const codes = (health?.sales ?? []).map((row) => row.code);
    if (codes.join() !== expected.focusExportCodes.join()) {
      note(`focus export codes ${codes.join(',')} != ${expected.focusExportCodes.join(',')}`);
    }
    if ((health?.sales.length ?? 0) !== expected.focusExportCodes.length) {
      note(`focus export length ${health?.sales.length} != ${expected.focusExportCodes.length}`);
    }
  }
  if (expected.categoryContinuation) {
    const { rank, code } = expected.categoryContinuation;
    const found = document.categorySales.some((page) => page.cards.some((card) =>
      card.rows.some((row) => row.rank === rank && row.code === code)));
    if (!found) note(`missing category continuation rank ${rank} code ${code}`);
  }
  const compareRows = (actual: PdfRankRow[], wanted: PdfRankRow[], label: string) => {
    if (actual.length !== wanted.length) note(`${label}: row count ${actual.length} != ${wanted.length}`);
    wanted.forEach((row, index) => {
      const found = actual[index];
      if (!found || found.rank !== row.rank || found.code !== row.code || found.quantity !== row.quantity || found.compactAmount !== row.compactAmount) note(`${label}: row ${index + 1} identity/value mismatch`);
    });
  };
  if (expected.overallRows) {
    compareRows(document.overall.sales, expected.overallRows.sales, 'overall sales');
    compareRows(document.overall.quantity, expected.overallRows.quantity, 'overall quantity');
  }
  for (const group of expected.categoryRows ?? []) {
    const pages = group.metric === 'sales' ? document.categorySales : document.categoryQuantity;
    const cards = pages.filter(page => page.period === group.period).flatMap(page => page.cards.filter(card => card.code === group.code));
    compareRows(cards.flatMap(card => card.rows), group.rows, `${group.period}/${group.metric}/${group.code}`);
    const sizes = Array.from({ length: Math.ceil(group.rows.length / group.cardSize) }, (_, index) => Math.min(group.cardSize, group.rows.length - index * group.cardSize));
    if (cards.map(card => card.rows.length).join() !== sizes.join()) note(`${group.period}/${group.metric}/${group.code}: continuation boundaries mismatch`);
  }
  return { ok: errors.length === 0, errors };
}

export async function extractPdf(file: string, readFileSync: (path: string) => Buffer): Promise<PdfPage[]> {
  const { getDocument } = await import('pdfjs-dist/legacy/build/pdf.mjs');
  const data = new Uint8Array(readFileSync(file));
  const doc = await getDocument({ data, isEvalSupported: false, verbosity: 0 } as never).promise;
  const pages: PdfPage[] = [];
  for (let pageNumber = 1; pageNumber <= doc.numPages; pageNumber += 1) {
    const page = await doc.getPage(pageNumber);
    const content = await page.getTextContent();
    const text = content.items.map((item) => ('str' in item ? item.str : '')).join(' ');
    pages.push({ pageNumber, text });
  }
  await doc.cleanup?.();
  return pages;
}

export function parseValidateArgs(argv: string[] = [], env: Record<string, string | undefined> = {}) {
  const reuse = argv.includes('--reuse') || env.PDF_VALIDATE_REUSE === '1';
  return {
    reuse,
    reuseLabel: reuse ? 'explicit-reuse (--reuse or PDF_VALIDATE_REUSE=1)' : '',
    failCoverage: argv.includes('--fail-coverage') || env.PDF_VALIDATE_FAIL_COVERAGE === '1',
    fixtureFail: argv.includes('--fixture-fail') || env.PDF_VALIDATE_FIXTURE_FAIL === '1',
    skipRender: argv.includes('--skip-render') || env.PDF_VALIDATE_SKIP_RENDER === '1',
    skipWorkbook: argv.includes('--skip-workbook') || env.PDF_VALIDATE_SKIP_WORKBOOK === '1',
    skipGenerate: argv.includes('--skip-generate') || env.PDF_VALIDATE_SKIP_GENERATE === '1',
  };
}
