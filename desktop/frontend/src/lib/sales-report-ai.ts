import { ALL_STORES_REPORT_ID, isAllStoresReport, salesAnalysisPDFFilename } from './sales-report-pdf';
import { salesReportHasScreenFilters, type SalesReportFilter } from './salesReportItems';
import type {
  Locale,
  SalesAnalysisFocusGroup,
  SalesAnalysisPeriodMemo,
  SalesAnalysisRankedItem,
  SalesAnalysisReportMemo,
  SalesAnalysisTotals,
} from './types';

const PERIOD_KEYS = ['current', 'previous', 'previous2', 'yearAgo', 'yearAgoNext'] as const;

export interface SalesReportAIPeriodMeta {
  key: string;
  label?: string;
  from?: string;
  to?: string;
}

export interface SalesReportAIGroup {
  groupId: string;
  groupName: string;
  itemCodeCount: number;
  memo: SalesAnalysisReportMemo;
}

export interface SalesReportAIInput {
  locale: Locale;
  storeId: string;
  storeLabel: string;
  from: string;
  to: string;
  categoryLevel: string;
  filter: SalesReportFilter;
  base: SalesAnalysisReportMemo;
  groups: SalesReportAIGroup[];
  periodMeta?: SalesReportAIPeriodMeta[];
}

export function salesAnalysisAIFilename(storeId: string, from: string, to: string): string {
  return salesAnalysisPDFFilename(storeId, from, to).replace(/\.pdf$/i, '-ai.md');
}

export function buildSalesAnalysisAIMarkdown(input: SalesReportAIInput): string {
  const copy = aiCopy(input.locale);
  const current = periodMemo(input.base, 'current');
  const previous = periodMemo(input.base, 'previous');
  const previous2 = periodMemo(input.base, 'previous2');
  const yearAgo = periodMemo(input.base, 'yearAgo');
  const yearAgoNext = periodMemo(input.base, 'yearAgoNext');
  const storeName = isAllStoresReport(input.storeId) ? copy.allStores : (input.storeLabel || input.storeId);
  const filterLines = describeFilter(input.filter, copy);
  const metaByKey = periodMetaMap(input.periodMeta);
  const periodKeys = presentPeriodKeys([input.base, ...input.groups.map((group) => group.memo)]);
  const compareKeys = periodKeys.filter((key) => key !== 'current' && key !== 'yearAgoNext');
  const payload = {
    storeId: isAllStoresReport(input.storeId) ? ALL_STORES_REPORT_ID : input.storeId,
    storeLabel: storeName,
    from: input.from,
    to: input.to,
    categoryLevel: input.categoryLevel,
    currency: 'HKD',
    alreadyFiltered: filterLines.length > 0,
    filter: {
      mode: input.filter.mode,
      excludeZeroGifts: input.filter.excludeZeroGifts,
      excludeStamps: input.filter.excludeStamps,
      categories: input.filter.categories,
      facets: input.filter.facets ?? {},
      search: input.filter.search ?? '',
    },
    totals: Object.fromEntries(periodKeys.map((key) => [
      key,
      {
        ...periodRange(key, metaByKey, copy, input),
        ...compactTotals(periodMemo(input.base, key)?.totals),
      },
    ])),
    categories: [] as ReturnType<typeof comparedCategories>,
    topSales: (current?.topAmount ?? []).slice(0, 15).map(compactItem),
    topQuantity: (current?.topQuantity ?? []).slice(0, 15).map(compactItem),
    focusGroups: compactFocusGroups(yearAgoNext?.focusGroups),
    groups: input.groups.map((group) => {
      const groupCurrent = periodMemo(group.memo, 'current');
      return {
        id: group.groupId,
        name: group.groupName,
        itemCodeCount: group.itemCodeCount,
        totals: Object.fromEntries(periodKeys.map((key) => [key, compactTotals(periodMemo(group.memo, key)?.totals)])),
        topSales: (groupCurrent?.topAmount ?? []).slice(0, 8).map(compactItem),
      };
    }),
  };

  const lines: string[] = [
    `---`,
    `title: ${yamlEscape(`${copy.title} — ${storeName}`)}`,
    `purpose: sales-analysis-briefing`,
    `audience: ${yamlEscape(copy.audience)}`,
    `store: ${yamlEscape(storeName)}`,
    `period: ${input.from} / ${input.to}`,
    `currency: HKD`,
    `---`,
    ``,
    `# ${copy.title} — ${storeName}`,
    ``,
    `## ${copy.instructions}`,
    ``,
    `1. ${copy.ruleSource}`,
    `2. ${copy.ruleNoInvent}`,
    `3. ${copy.ruleCurrency}`,
    `4. ${copy.ruleScope}`,
    `5. ${copy.ruleRankings}`,
    `6. ${copy.ruleZero}`,
    `7. ${copy.ruleJson}`,
    `8. ${copy.ruleOutput}`,
    ``,
    `### ${copy.askTitle}`,
    ``,
    `- ${copy.ask1}`,
    `- ${copy.ask2}`,
    `- ${copy.ask3}`,
    ``,
    `## ${copy.snapshot}`,
    ``,
    `- ${copy.store}: ${storeName}`,
    `- ${copy.period}: ${input.from} → ${input.to}`,
    `- ${copy.currencyLabel}: HKD / HK$`,
  ];

  if (filterLines.length > 0) {
    lines.push(`- ${copy.scopeNote}: ${copy.scopeFiltered}`);
    for (const line of filterLines) lines.push(`  - ${line}`);
  } else {
    lines.push(`- ${copy.scopeNote}: ${copy.scopeAll}`);
  }

  lines.push('', `### ${copy.periodTotals}`, '');
  lines.push(
    `| ${copy.periodName} | ${copy.dates} | ${copy.netSales} | ${copy.netQuantity} |`,
    '| --- | --- | ---: | ---: |',
  );
  for (const key of periodKeys) {
    const period = periodMemo(input.base, key);
    const range = periodRange(key, metaByKey, copy, input);
    lines.push(`| ${mdCell(range.label)} | ${mdCell(range.dates || '—')} | ${money(period?.totals?.netSalesAmount)} | ${qty(period?.totals?.netQuantity)} |`);
  }
  lines.push('');
  const currentSales = current?.totals?.netSalesAmount;
  const currentQty = current?.totals?.netQuantity;
  for (const key of compareKeys) {
    const period = periodMemo(input.base, key);
    lines.push(`- ${copy.netSales} ${vsLabel(key, copy)}: ${percent(currentSales, period?.totals?.netSalesAmount)}`);
    lines.push(`- ${copy.netQuantity} ${vsLabel(key, copy)}: ${percent(currentQty, period?.totals?.netQuantity)}`);
  }
  if (compareKeys.length > 0) lines.push('');

  if (current?.totals?.trendNetSalesAmount !== undefined) {
    lines.push(`- ${copy.storeTrendSales}: ${money(current.totals.trendNetSalesAmount)} (${copy.storeTrendHint})`);
  }
  if (current?.totals?.transactionCount !== undefined) {
    lines.push(`- ${copy.transactions}: ${qty(current.totals.transactionCount)} (${copy.storeTrendHint})`);
  }
  if (current?.totals?.trendNetSalesAmount !== undefined || current?.totals?.transactionCount !== undefined) {
    lines.push('');
  }
  lines.push(`## ${copy.groupSummary}`, '');

  if (input.groups.length === 0) {
    lines.push(copy.noGroups, '');
  } else {
    const headers = [copy.group, copy.current];
    const aligns = ['---', '---:'];
    for (const key of compareKeys) {
      headers.push(periodLabel(key, copy), vsLabel(key, copy));
      aligns.push('---:', '---:');
    }
    if (periodKeys.includes('yearAgoNext')) {
      headers.push(copy.yearAgoNext);
      aligns.push('---:');
    }
    lines.push(`| ${headers.join(' | ')} |`, `| ${aligns.join(' | ')} |`);
    for (const group of payload.groups) {
      const cells = [mdCell(group.name), money(group.totals.current?.netSalesAmount)];
      for (const key of compareKeys) {
        cells.push(money(group.totals[key]?.netSalesAmount), percent(group.totals.current?.netSalesAmount, group.totals[key]?.netSalesAmount));
      }
      if (periodKeys.includes('yearAgoNext')) cells.push(money(group.totals.yearAgoNext?.netSalesAmount));
      lines.push(`| ${cells.join(' | ')} |`);
    }
    lines.push('');
  }

  lines.push(`## ${copy.categoryPerformance}`, '');
  const categories = comparedCategories(current, previous, previous2, yearAgo);
  payload.categories = categories;
  if (categories.length === 0) {
    lines.push(copy.noRows, '');
  } else {
    const headers = [copy.category, copy.current];
    const aligns = ['---', '---:'];
    if (previous) {
      headers.push(copy.previous, copy.vsPrevious);
      aligns.push('---:', '---:');
    }
    if (previous2) {
      headers.push(copy.previous2, copy.vsPrevious2);
      aligns.push('---:', '---:');
    }
    if (yearAgo) {
      headers.push(copy.yearAgo, copy.vsYearAgo);
      aligns.push('---:', '---:');
    }
    headers.push(copy.quantity);
    aligns.push('---:');
    lines.push(`| ${headers.join(' | ')} |`, `| ${aligns.join(' | ')} |`);
    for (const group of categories) {
      const cells = [mdCell(group.code ? `${group.code} ${group.name}` : group.name), money(group.amount)];
      if (previous) cells.push(money(group.previousAmount), percent(group.amount, group.previousAmount));
      if (previous2) cells.push(money(group.previous2Amount), percent(group.amount, group.previous2Amount));
      if (yearAgo) cells.push(money(group.yearAgoAmount), percent(group.amount, group.yearAgoAmount));
      cells.push(qty(group.quantity));
      lines.push(`| ${cells.join(' | ')} |`);
    }
    lines.push('');
  }

  lines.push(`## ${copy.topSales}`, '');
  appendItemTable(lines, payload.topSales, copy);
  lines.push(`## ${copy.topQuantity}`, '');
  appendItemTable(lines, payload.topQuantity, copy);

  if ((yearAgoNext?.focusGroups ?? []).length > 0) {
    const range = periodRange('yearAgoNext', metaByKey, copy, input);
    lines.push(`## ${copy.focusTitle}`, '');
    lines.push(copy.focusHint.replace('{dates}', range.dates || '—'), '');
    for (const group of yearAgoNext?.focusGroups ?? []) {
      lines.push(`### ${mdCell(group.name || group.prefix || group.id)}`, '');
      const items = (group.sales ?? []).slice(0, 8).map((item) => ({
        code: item.code,
        name: item.name,
        amount: item.amount,
        quantity: item.quantity,
        currentAmount: item.currentAmount,
      }));
      if (items.length === 0) {
        lines.push(copy.noRows, '');
        continue;
      }
      lines.push(
        `| ${copy.product} | ${copy.yearAgoNext} | ${copy.current} | ${copy.quantity} |`,
        '| --- | ---: | ---: | ---: |',
      );
      for (const item of items) {
        lines.push(`| ${mdCell(item.name || item.code)} \`${item.code}\` | ${money(item.amount)} | ${money(item.currentAmount)} | ${qty(item.quantity)} |`);
      }
      lines.push('');
    }
  }

  lines.push(`## ${copy.structured}`, '');
  lines.push(copy.jsonHint, '');
  lines.push('```json', JSON.stringify(payload, null, 2), '```', '');
  return lines.join('\n');
}

function describeFilter(filter: SalesReportFilter, copy: ReturnType<typeof aiCopy>): string[] {
  const lines: string[] = [];
  const labels: Record<string, string> = {
    category1: copy.category1,
    category2: copy.category2,
    category3: copy.category3,
    category4: copy.category4,
    category5: copy.category5,
  };
  for (const key of ['category1', 'category2', 'category3', 'category4', 'category5'] as const) {
    const values = filter.facets?.[key] ?? [];
    if (values.length > 0) lines.push(`${labels[key]}: ${values.join('；')}`);
  }
  if (filter.search?.trim()) lines.push(`${copy.search}: ${filter.search.trim()}`);
  if (filter.mode === 'whitelist' && filter.categories.length > 0) {
    lines.push(`${copy.keepOnly}: ${filter.categories.join('；')}`);
  } else if (filter.mode === 'blacklist' && filter.categories.length > 0) {
    lines.push(`${copy.exclude}: ${filter.categories.join('；')}`);
  }
  if (salesReportHasScreenFilters(filter) || filter.categories.length > 0) return lines;
  return lines;
}

function appendItemTable(
  lines: string[],
  items: Array<{ code: string; name: string; amount: number; quantity: number }>,
  copy: ReturnType<typeof aiCopy>,
): void {
  if (items.length === 0) {
    lines.push(copy.noRows, '');
    return;
  }
  lines.push(`| # | ${copy.product} | ${copy.amount} | ${copy.quantity} |`, '| ---: | --- | ---: | ---: |');
  items.forEach((item, index) => {
    lines.push(`| ${index + 1} | ${mdCell(item.name || item.code)} \`${item.code}\` | ${money(item.amount)} | ${qty(item.quantity)} |`);
  });
  lines.push('');
}

function periodMemo(memo: SalesAnalysisReportMemo, key: string): SalesAnalysisPeriodMemo | undefined {
  return (memo.periods ?? []).find((period) => period.key === key);
}

function compactTotals(totals: SalesAnalysisTotals | undefined) {
  if (!totals) return undefined;
  return {
    netSalesAmount: roundMoney(totals.netSalesAmount),
    netQuantity: roundMoney(totals.netQuantity),
    transactionCount: totals.transactionCount === undefined ? undefined : roundMoney(totals.transactionCount),
    trendNetSalesAmount: totals.trendNetSalesAmount === undefined ? undefined : roundMoney(totals.trendNetSalesAmount),
  };
}

function compactItem(item: SalesAnalysisRankedItem) {
  return {
    id: item.id,
    code: item.code,
    name: item.name,
    brand: item.brand ?? '',
    amount: roundMoney(item.amount),
    quantity: roundMoney(item.quantity),
  };
}

function comparedCategories(
  current: SalesAnalysisPeriodMemo | undefined,
  previous: SalesAnalysisPeriodMemo | undefined,
  previous2: SalesAnalysisPeriodMemo | undefined,
  yearAgo: SalesAnalysisPeriodMemo | undefined,
) {
  const previousById = categoryMap(previous);
  const previous2ById = categoryMap(previous2);
  const yearAgoById = categoryMap(yearAgo);
  return (current?.amountGroups ?? []).slice(0, 12).map((group) => ({
    id: group.id,
    code: group.code ?? '',
    name: group.name,
    amount: roundMoney(group.amount),
    quantity: roundMoney(group.quantity),
    previousAmount: previousById.get(group.id),
    previous2Amount: previous2ById.get(group.id),
    yearAgoAmount: yearAgoById.get(group.id),
  }));
}

function presentPeriodKeys(memos: SalesAnalysisReportMemo[]): string[] {
  const found = new Set<string>();
  for (const memo of memos) {
    for (const period of memo.periods ?? []) found.add(period.key);
  }
  return PERIOD_KEYS.filter((key) => found.has(key));
}

function periodMetaMap(meta: SalesReportAIPeriodMeta[] | undefined): Map<string, SalesReportAIPeriodMeta> {
  const byKey = new Map<string, SalesReportAIPeriodMeta>();
  for (const period of meta ?? []) byKey.set(period.key, period);
  return byKey;
}

function periodLabel(key: string, copy: ReturnType<typeof aiCopy>): string {
  if (key === 'current') return copy.current;
  if (key === 'previous') return copy.previous;
  if (key === 'previous2') return copy.previous2;
  if (key === 'yearAgo') return copy.yearAgo;
  if (key === 'yearAgoNext') return copy.yearAgoNext;
  return key;
}

function vsLabel(key: string, copy: ReturnType<typeof aiCopy>): string {
  if (key === 'previous') return copy.vsPrevious;
  if (key === 'previous2') return copy.vsPrevious2;
  if (key === 'yearAgo') return copy.vsYearAgo;
  return `${copy.vsPrefix} ${periodLabel(key, copy)}`;
}

function periodRange(
  key: string,
  metaByKey: Map<string, SalesReportAIPeriodMeta>,
  copy: ReturnType<typeof aiCopy>,
  input: SalesReportAIInput,
): { label: string; from?: string; to?: string; dates: string } {
  const meta = metaByKey.get(key);
  const from = meta?.from ?? (key === 'current' ? input.from : undefined);
  const to = meta?.to ?? (key === 'current' ? input.to : undefined);
  const dates = from && to ? `${from} → ${to}` : '';
  return { label: meta?.label || periodLabel(key, copy), from, to, dates };
}

function compactFocusGroups(groups: SalesAnalysisFocusGroup[] | undefined) {
  return (groups ?? []).map((group) => ({
    id: group.id,
    prefix: group.prefix,
    name: group.name ?? '',
    sales: (group.sales ?? []).slice(0, 8).map((item) => ({
      code: item.code,
      name: item.name,
      amount: roundMoney(item.amount),
      quantity: roundMoney(item.quantity),
      currentAmount: roundMoney(item.currentAmount),
      currentQuantity: roundMoney(item.currentQuantity),
    })),
  }));
}

function categoryMap(period: SalesAnalysisPeriodMemo | undefined): Map<string, number> {
  const byId = new Map<string, number>();
  for (const group of period?.amountGroups ?? []) byId.set(group.id, roundMoney(group.amount));
  return byId;
}

function roundMoney(value: number): number {
  return Math.round((value + Number.EPSILON) * 100) / 100;
}

function money(value: number | undefined): string {
  return value === undefined ? '—' : `HK$${value.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function qty(value: number | undefined): string {
  return value === undefined ? '—' : Math.round(value).toLocaleString('en-US');
}

function percent(current: number | undefined, comparison: number | undefined): string {
  if (current === undefined || comparison === undefined || comparison === 0) return '—';
  const change = ((current - comparison) / Math.abs(comparison)) * 100;
  const sign = change > 0 ? '+' : '';
  return `${sign}${change.toFixed(1)}%`;
}

function mdCell(value: string): string {
  return value.replaceAll('|', '\\|').replaceAll('\n', ' ').trim() || '—';
}

function yamlEscape(value: string): string {
  return `"${value.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`;
}

function aiCopy(locale: Locale) {
  if (locale === 'en') {
    return {
      title: 'RTA sales analysis',
      audience: 'Any large language model',
      allStores: 'All stores',
      instructions: 'Rules for the AI (must follow)',
      ruleSource: 'Use only the numbers in this file. If a figure is not here, say it is not in the file.',
      ruleNoInvent: 'Do not invent products, categories, stores, or amounts.',
      ruleCurrency: 'All money is Hong Kong dollars (HKD / HK$). Do not convert currency.',
      ruleScope: 'The tables below are already filtered. Do not add categories or products that are not listed.',
      ruleRankings: 'Top-product tables are the top 15 only. Do not add them up and treat the sum as store sales.',
      ruleZero: 'A row with HK$0.00 and a quantity is a real voucher or bag line, not an error.',
      ruleJson: 'JSON matches the tables, rounded to 2 decimal places. Prefer the tables if anything looks inconsistent.',
      ruleOutput: 'Answer in the user\'s language. Keep answers short. Every claim must cite a number from this file.',
      askTitle: 'Suggested questions',
      ask1: 'Using only the period-totals table, how did this period perform versus the previous period, two periods ago, and the same period last year?',
      ask2: 'In the category table, which listed categories fell versus the previous period or two periods ago, and by how much?',
      ask3: 'Give at most 5 next-month actions. Prefer the last-year-next-month section when it exists. Each action must point to one number already in the tables.',
      snapshot: 'Scope and totals',
      store: 'Store',
      period: 'Period',
      currencyLabel: 'Currency',
      scopeNote: 'Coverage',
      scopeFiltered: 'Already limited to the filters below. Treat missing categories as out of scope, not zero sales.',
      scopeAll: 'Whole-store figures in this file, after skipping zero-value gifts and stamps if those flags are on.',
      search: 'Search',
      keepOnly: 'Keep only',
      exclude: 'Exclude',
      category1: 'Merchandise class',
      category2: 'Department',
      category3: 'Category',
      category4: 'Sub-category',
      category5: 'Segment',
      netSales: 'Item-row net sales',
      netQuantity: 'Item-row net quantity',
      storeTrendSales: 'Store trend net sales',
      storeTrendHint: 'store-level Trend View; do not expect this to equal the item-row or category sum',
      transactions: 'Transactions',
      periodTotals: 'Totals by period',
      periodName: 'Period',
      dates: 'Dates',
      vsPrevious: 'vs previous',
      vsPrevious2: 'vs two periods ago',
      vsYearAgo: 'vs year ago',
      vsPrefix: 'vs',
      groupSummary: 'Promoter group summary',
      group: 'Group',
      current: 'Current',
      previous: 'Previous',
      previous2: 'Two periods ago',
      yearAgo: 'Year ago',
      yearAgoNext: 'Next month last year',
      focusTitle: 'Focus: next month last year',
      focusHint: 'Products that sold well in {dates}. Use this to plan the coming month. Do not treat it as this period.',
      noGroups: 'No promoter groups were selected. This file is the store-wide summary only.',
      categoryPerformance: 'Category performance',
      category: 'Category',
      quantity: 'Qty',
      noRows: 'No rows in this section.',
      topSales: 'Top products by sales',
      topQuantity: 'Top products by quantity',
      product: 'Product',
      amount: 'Sales',
      structured: 'Machine-readable data',
      jsonHint: 'Use the JSON only to look up exact figures. Do not ignore the rules above.',
    };
  }
  return {
    title: 'RTA 銷售分析',
    audience: '任何大型語言模型',
    allStores: '全部門店',
    instructions: '給 AI 的規則（必須遵守）',
    ruleSource: '只准使用這份檔案裡的數字。檔案沒有的數字，就寫「檔案沒有」。',
    ruleNoInvent: '不准發明商品、分類、門店或金額。',
    ruleCurrency: '金額一律是港幣（HKD / HK$），不要換算其他幣別。',
    ruleScope: '下面的表已經是篩選後的結果。沒有出現的分類或商品視為不在範圍，不是銷售為 0。',
    ruleRankings: 'Top 商品只是前 15 名。不要把這 15 項加總當成全店銷售。',
    ruleZero: '銷額 HK$0.00 但有銷量的列（現金券、膠袋）是真實列，不是錯誤。',
    ruleJson: 'JSON 與上面的表是同一組數字，金額已四捨五入到兩位小數。若看起來不一致，以表格為準。',
    ruleOutput: '用繁體中文回答，短句。每一個判斷都要引用這份檔裡的一個數字。',
    askTitle: '建議這樣問',
    ask1: '只用「各期間總數」裡的數字，說明本期相對上期、前期、去年同期。',
    ask2: '分類表裡哪些分類較上期或前期下跌？跌多少？沒有前期就只比上期。',
    ask3: '下個月最多給 5 個行動。有「去年下月關注」就先用那一節；沒有再用分類表。每一點都要引用表裡已有的一個數字。',
    snapshot: '範圍與總數',
    store: '門店',
    period: '期間',
    currencyLabel: '幣別',
    scopeNote: '資料範圍',
    scopeFiltered: '已限於下列篩選。沒列出的分類不要當「沒賣掉」。',
    scopeAll: '這份檔是全店數字；若有勾忽略贈品／印花，那些列已不含在內。',
    search: '搜尋',
    keepOnly: '只保留',
    exclude: '排除',
    category1: '商品分類',
    category2: '商品部門',
    category3: '商品種類',
    category4: '四級類目',
    category5: '小分類',
    netSales: '商品列淨銷售額',
    netQuantity: '商品列淨銷售數量',
    storeTrendSales: '全店趨勢淨銷售額',
    storeTrendHint: '門店趨勢數字，不必等於商品列或分類加總',
    transactions: '交易次數',
    periodTotals: '各期間總數',
    periodName: '期間',
    dates: '日期',
    vsPrevious: '較上期',
    vsPrevious2: '較前期',
    vsYearAgo: '較去年同期',
    vsPrefix: '較',
    groupSummary: 'Promoter group 總結',
    group: '群組',
    current: '本期',
    previous: '上期',
    previous2: '前期',
    yearAgo: '去年同期',
    yearAgoNext: '去年下月',
    focusTitle: '去年下月關注',
    focusHint: '這些是 {dates} 賣得好的商品，用來預估下個月。不要把它當成本期。',
    noGroups: '沒有勾選 promoter group，這份檔只有全店總結。',
    categoryPerformance: '分類表現',
    category: '分類',
    quantity: '銷量',
    noRows: '這個區塊沒有資料。',
    topSales: '銷售額 Top 商品',
    topQuantity: '銷量 Top 商品',
    product: '商品',
    amount: '銷售額',
    structured: '機器可讀資料',
    jsonHint: 'JSON 只用來核對精確數字，上面的規則仍要遵守。',
  };
}
