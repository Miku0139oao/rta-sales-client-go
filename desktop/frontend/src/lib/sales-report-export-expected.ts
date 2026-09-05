// Independent expected identities for export validation.
// Numbers are hand-checked from SMALL_EXPORT_SPECS / the large-fixture generation formula.
// Do not import ranking helpers or screen-built identities here.

import type { PdfParityExpected, PdfRankRow } from './sales-report-pdf-section';

export interface ExpectedRank {
  rank: number;
  code: string;
  name: string;
  amount: number;
  quantity: number;
}

export interface ExpectedTotals {
  netSalesAmount: number;
  netQuantity: number;
  transactionCount: number;
}

export interface ExpectedCategory {
  code: string;
  name: string;
  amount: number;
  quantity: number;
}

function pdfMoney(value: number): string {
  return `HK$${new Intl.NumberFormat('en-HK', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)}`;
}

function pdfCompactMoney(value: number): string {
  const absolute = Math.abs(value);
  if (absolute >= 1_000_000) return `HK$${(value / 1_000_000).toFixed(2)}M`;
  if (absolute >= 100_000) return `HK$${(value / 1_000).toFixed(0)}K`;
  return `HK$${new Intl.NumberFormat('en-HK', {
    minimumFractionDigits: absolute >= 1_000 ? 0 : 2,
    maximumFractionDigits: absolute >= 1_000 ? 0 : 2,
  }).format(value)}`;
}

function pdfQuantity(value: number): string {
  return new Intl.NumberFormat('en-HK', { minimumFractionDigits: 0, maximumFractionDigits: 0 }).format(Math.round(value));
}

function pdfPercent(current: number, base: number): string {
  const change = ((current - base) / Math.abs(base)) * 100;
  const sign = change > 0 ? '+' : '';
  return `${sign}${change.toFixed(1)}%`;
}

function aiMoney(value: number): string {
  return `HK$${value.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function aiQty(value: number): string {
  return Math.round(value).toLocaleString('en-US');
}

function aiPercent(current: number, base: number): string {
  const change = ((current - base) / Math.abs(base)) * 100;
  const sign = change > 0 ? '+' : '';
  return `${sign}${change.toFixed(1)}%`;
}

function screenMoney(value: number): string {
  return new Intl.NumberFormat('zh-TW', {
    style: 'currency', currency: 'HKD', minimumFractionDigits: 2, maximumFractionDigits: 2,
  }).format(value);
}

function screenNumber(value: number): string {
  return new Intl.NumberFormat('zh-TW', { maximumFractionDigits: 0 }).format(Math.round(value));
}

function screenPercent(current: number, base: number): string {
  return new Intl.NumberFormat('zh-TW', {
    style: 'percent', signDisplay: 'always', maximumFractionDigits: 1,
  }).format((current - base) / Math.abs(base));
}

const HEALTH_SALES: ExpectedRank[] = [
  { rank: 1, code: '0100001', name: 'H01', amount: 1200, quantity: 120 },
  { rank: 2, code: '0100002', name: 'H02', amount: 1100, quantity: 110 },
  { rank: 3, code: '0100003', name: 'H03', amount: 1000, quantity: 100 },
  { rank: 4, code: '0100004', name: 'H04', amount: 900, quantity: 90 },
  { rank: 5, code: '0100005', name: 'H05', amount: 800, quantity: 80 },
  { rank: 6, code: '0100006', name: 'H06', amount: 700, quantity: 70 },
  { rank: 7, code: '0100007', name: 'H07', amount: 600, quantity: 60 },
  { rank: 8, code: '0100008', name: 'H08', amount: 500, quantity: 50 },
  { rank: 9, code: '0100009', name: 'H09', amount: 400, quantity: 40 },
  { rank: 10, code: '0100010', name: 'H10', amount: 300, quantity: 30 },
  { rank: 11, code: '0100011', name: 'H11', amount: 200, quantity: 20 },
  { rank: 12, code: '0100012', name: 'H12', amount: 100, quantity: 10 },
];

const HEALTH_FOCUS_NEXT: ExpectedRank[] = HEALTH_SALES.map((item) => ({
  ...item,
  amount: item.amount * 1.1,
  quantity: item.quantity * 1.1,
}));

// Health 1200+...+100=7800/780; Skin 2000+1500=3500/350; PC 1800+1600=3400/700.
// Current 14700/1830/160. Previous x0.5=7350/915/80. YearAgo x0.8=11760/1464/128.
export const SMALL_EXPORT_EXPECTED = {
  from: '2026-08-01',
  to: '2026-08-16',
  storeId: '107',
  rankingLimit: 16,
  current: { netSalesAmount: 14700, netQuantity: 1830, transactionCount: 160 } satisfies ExpectedTotals,
  previous: { netSalesAmount: 7350, netQuantity: 915, transactionCount: 80 } satisfies ExpectedTotals,
  previous2: { netSalesAmount: 5880, netQuantity: 732, transactionCount: 64 } satisfies ExpectedTotals,
  yearAgo: { netSalesAmount: 11760, netQuantity: 1464, transactionCount: 128 } satisfies ExpectedTotals,
  yearAgoNext: { netSalesAmount: 16170, netQuantity: 2013, transactionCount: 176 } satisfies ExpectedTotals,
  vsPreviousSales: 1,
  vsYearAgoSales: 0.25,
  categories: [
    { code: 'A01', name: '保健護理', amount: 7800, quantity: 780 },
    { code: 'A02', name: '肌膚護理', amount: 3500, quantity: 350 },
    { code: 'A03', name: '個人護理', amount: 3400, quantity: 700 },
  ] satisfies ExpectedCategory[],
  topSales: [
    { rank: 1, code: '0200001', name: 'S01', amount: 2000, quantity: 200 },
    { rank: 2, code: '0300001', name: 'P01', amount: 1800, quantity: 400 },
    { rank: 3, code: '0300002', name: 'P02', amount: 1600, quantity: 300 },
    { rank: 4, code: '0200002', name: 'S02', amount: 1500, quantity: 150 },
    ...HEALTH_SALES.map((item, index) => ({ ...item, rank: index + 5 })),
  ] satisfies ExpectedRank[],
  topQuantity: [
    { rank: 1, code: '0300001', name: 'P01', amount: 1800, quantity: 400 },
    { rank: 2, code: '0300002', name: 'P02', amount: 1600, quantity: 300 },
    { rank: 3, code: '0200001', name: 'S01', amount: 2000, quantity: 200 },
    { rank: 4, code: '0200002', name: 'S02', amount: 1500, quantity: 150 },
    ...HEALTH_SALES.map((item, index) => ({ ...item, rank: index + 5 })),
  ] satisfies ExpectedRank[],
  focusScreenHealth: HEALTH_FOCUS_NEXT.slice(0, 10),
  focusExportHealth: HEALTH_FOCUS_NEXT.slice(0, 8),
  filterHealth: {
    netSalesAmount: 7800,
    netQuantity: 780,
    transactionCount: 120,
    previousNetSalesAmount: 3900,
    yearAgoNetSalesAmount: 6240,
    topSales: HEALTH_SALES,
    last: HEALTH_SALES[11]!,
  },
  groupPair: {
    netSalesAmount: 3800,
    netQuantity: 600,
    previousNetSalesAmount: 1900,
    yearAgoNetSalesAmount: 3040,
    topSales: [
      { rank: 1, code: '0200001', name: 'S01', amount: 2000, quantity: 200 },
      { rank: 2, code: '0300001', name: 'P01', amount: 1800, quantity: 400 },
    ] satisfies ExpectedRank[],
  },
  formats: {
    pdfMoney,
    pdfCompactMoney,
    pdfQuantity,
    pdfPercent,
    aiMoney,
    aiQty,
    aiPercent,
    screenMoney,
    screenNumber,
    screenPercent,
  },
};

export const PAGINATION_EXPECTED: Record<16 | 24 | 40 | 100, {
  lastSales: ExpectedRank;
  lastQuantitySample: ExpectedRank;
  overallContinuationStart: number;
  categoryContinuationStart: number;
  categoryContinuationCode: string;
}> = {
  // amount = 900000 - index*17 - (index%3); qty = 4800 - index*3 - (index%5); code 0100NNN
  16: {
    lastSales: { rank: 16, code: '0100016', name: '0100016', amount: 899745, quantity: 4755 },
    lastQuantitySample: { rank: 16, code: '0200002', name: '0200002', amount: 891982, quantity: 4756 },
    overallContinuationStart: 0,
    categoryContinuationStart: 0,
    categoryContinuationCode: '0100016',
  },
  24: {
    lastSales: { rank: 24, code: '0100024', name: '0100024', amount: 899607, quantity: 4728 },
    lastQuantitySample: { rank: 24, code: '0200005', name: '0200005', amount: 891931, quantity: 4744 },
    overallContinuationStart: 0,
    categoryContinuationStart: 0,
    categoryContinuationCode: '0100024',
  },
  40: {
    lastSales: { rank: 40, code: '0100040', name: '0100040', amount: 899337, quantity: 4679 },
    lastQuantitySample: { rank: 40, code: '0100027', name: '0100027', amount: 899556, quantity: 4721 },
    overallContinuationStart: 26,
    categoryContinuationStart: 34,
    categoryContinuationCode: '0100040',
  },
  100: {
    lastSales: { rank: 100, code: '0100100', name: '0100100', amount: 898317, quantity: 4499 },
    lastQuantitySample: { rank: 100, code: '0500006', name: '0500006', amount: 875913, quantity: 4665 },
    overallContinuationStart: 76,
    categoryContinuationStart: 34,
    categoryContinuationCode: '0100100',
  },
};

export const PAGINATION_SUMMARY = {
  // Independent generator specification, not a snapshot of PDF output.
  netSales: { current: paginationTotals(1).amount, previous: paginationTotals(0.91).amount, yearAgo: paginationTotals(0.88).amount },
  netQuantity: { current: paginationTotals(1).quantity, previous: paginationTotals(0.91).quantity, yearAgo: paginationTotals(0.88).quantity },
  vsPrevious: '+9.9%',
  vsYearAgo: '+13.6%',
  partial: '部分資料尚未完成，缺值不代表零銷售',
  firstSalesCode: '0100001',
  categoryA01: { code: 'A01', current: 89915751, previous: 81823333.41, yearAgo: 79125860.88 },
};

export function compareSmallWorkbook(inspected: Record<string, string[][]>): string[] {
  const errors: string[] = [];
  const sheet = (fragment: string) => Object.entries(inspected).find(([name]) => name.includes(fragment))?.[1];
  const expected = SMALL_EXPORT_EXPECTED;
  const performance = sheet('銷售表現');
  for (const [label, values] of [
    ['淨銷售額', [14700, 7350, 11760, 1, 0.25]],
    ['淨銷售數量', [1830, 915, 1464, 1, 0.25]],
  ] as const) {
    const row = performance?.find(row => row[0] === label);
    if (!row || values.some((value, index) => row[index + 1] === undefined || row[index + 1] === '' || Number(row[index + 1]) !== value)) errors.push(`xlsx ${label}: comparison cells mismatch`);
  }
  for (const [name, ranks] of [['銷售額 Top', expected.topSales], ['銷量 Top', expected.topQuantity]] as const) {
    const rows = sheet(name)?.slice(1);
    if (rows?.length !== ranks.length) errors.push(`xlsx ${name}: row count mismatch`);
    ranks.forEach((rank, index) => {
      const row = rows?.[index];
      if (!row || Number(row[0]) !== rank.rank || row[1] !== rank.code || Number(row[3]) !== rank.amount || Number(row[4]) !== rank.quantity) errors.push(`xlsx ${name}: row ${index + 1} mismatch`);
    });
  }
  return errors;
}

export const FOCUS_EXPORT_LIMIT = 8;
export const FOCUS_SCREEN_LIMIT = 10;

// Deliberately independent of fixture builders and production ranking helpers.
function paginationRows() {
  const categories = ['A01', 'A02', 'A03', 'B05', 'B10', 'B12', 'E08', 'A04'];
  const counts = [100, 40, 32, 24, 16, 16, 16, 16];
  return categories.flatMap((category, group) => Array.from({ length: counts[group]! }, (_, index) => ({
    category, code: category.slice(1) + String(index + 1).padStart(5, '0'),
    amount: 900000 - group * 8000 - index * 17 - index % 3,
    quantity: 4800 - group * 40 - index * 3 - index % 5,
  })));
}
function paginationTotals(scale: number) {
  const rows = paginationRows();
  return { amount: Number(rows.reduce((sum, row) => sum + row.amount * scale, 0).toFixed(2)), quantity: Math.round(rows.reduce((sum, row) => sum + row.quantity * scale, 0)) };
}
export function paginationParityExpected(limit: 16 | 24 | 40 | 100): PdfParityExpected {
  const all = paginationRows();
  const ranked = (rows: typeof all, metric: 'sales' | 'quantity', scale = 1): PdfRankRow[] => [...rows]
    .sort((a, b) => (metric === 'sales' ? b.amount - a.amount : b.quantity - a.quantity) || a.code.localeCompare(b.code))
    .slice(0, limit).map((row, index) => ({ rank: index + 1, code: row.code, quantity: Math.round(row.quantity * scale), compactAmount: pdfCompactMoney(row.amount * scale) }));
  const categoryRows: NonNullable<PdfParityExpected['categoryRows']> = [];
  for (const metric of ['sales', 'quantity'] as const) {
    const periods: Array<[string, number]> = metric === 'sales' ? [['本期', 1], ['去年同期', 0.88], ['去年下月', 1.07]] : [['本期', 1], ['上期', 0.91], ['前期', 0.84]];
    for (const [period, scale] of periods) for (const code of new Set(all.map(row => row.category))) {
      categoryRows.push({ period, metric, code, rows: ranked(all.filter(row => row.category === code), metric, scale), cardSize: limit <= 16 ? 16 : 33 });
    }
  }
  return { ...PAGINATION_SUMMARY, ...PAGINATION_EXPECTED[limit], overallRows: { sales: ranked(all, 'sales'), quantity: ranked(all, 'quantity') }, categoryRows };
}
