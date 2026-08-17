import { ALL_STORES_REPORT_ID, isAllStoresReport, salesAnalysisPDFFilename } from './sales-report-pdf';
import { salesReportHasScreenFilters, type SalesReportFilter } from './salesReportItems';
import type {
  Locale,
  SalesAnalysisPeriodMemo,
  SalesAnalysisRankedItem,
  SalesAnalysisReportMemo,
  SalesAnalysisTotals,
} from './types';

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
}

export function salesAnalysisAIFilename(storeId: string, from: string, to: string): string {
  return salesAnalysisPDFFilename(storeId, from, to).replace(/\.pdf$/i, '-ai.md');
}

export function buildSalesAnalysisAIMarkdown(input: SalesReportAIInput): string {
  const copy = aiCopy(input.locale);
  const current = periodMemo(input.base, 'current');
  const previous = periodMemo(input.base, 'previous');
  const yearAgo = periodMemo(input.base, 'yearAgo');
  const storeName = isAllStoresReport(input.storeId) ? copy.allStores : (input.storeLabel || input.storeId);
  const filterLines = describeFilter(input.filter, copy);
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
    totals: {
      current: compactTotals(current?.totals),
      previous: compactTotals(previous?.totals),
      yearAgo: compactTotals(yearAgo?.totals),
    },
    categories: (current?.amountGroups ?? []).slice(0, 12).map((group) => ({
      id: group.id,
      code: group.code ?? '',
      name: group.name,
      amount: group.amount,
      quantity: group.quantity,
    })),
    topSales: (current?.topAmount ?? []).slice(0, 15).map(compactItem),
    topQuantity: (current?.topQuantity ?? []).slice(0, 15).map(compactItem),
    groups: input.groups.map((group) => {
      const groupCurrent = periodMemo(group.memo, 'current');
      const groupPrevious = periodMemo(group.memo, 'previous');
      const groupYearAgo = periodMemo(group.memo, 'yearAgo');
      return {
        id: group.groupId,
        name: group.groupName,
        itemCodeCount: group.itemCodeCount,
        totals: {
          current: compactTotals(groupCurrent?.totals),
          previous: compactTotals(groupPrevious?.totals),
          yearAgo: compactTotals(groupYearAgo?.totals),
        },
        topSales: (groupCurrent?.topAmount ?? []).slice(0, 8).map(compactItem),
      };
    }),
  };

  const lines: string[] = [
    `---`,
    `title: ${yamlEscape(`${copy.title} — ${storeName}`)}`,
    `purpose: microsoft-copilot-sales-briefing`,
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
    `5. ${copy.ruleOutput}`,
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

  lines.push(
    `- ${copy.netSales}: ${money(current?.totals?.netSalesAmount)} (${copy.vsPrevious} ${percent(current?.totals?.netSalesAmount, previous?.totals?.netSalesAmount)}; ${copy.vsYearAgo} ${percent(current?.totals?.netSalesAmount, yearAgo?.totals?.netSalesAmount)})`,
    `- ${copy.netQuantity}: ${qty(current?.totals?.netQuantity)} (${copy.vsYearAgo} ${percent(current?.totals?.netQuantity, yearAgo?.totals?.netQuantity)})`,
    `- ${copy.transactions}: ${qty(current?.totals?.transactionCount)}`,
    ``,
    `## ${copy.groupSummary}`,
    ``,
  );

  if (input.groups.length === 0) {
    lines.push(copy.noGroups, '');
  } else {
    lines.push(`| ${copy.group} | ${copy.current} | ${copy.previous} | ${copy.vsPrevious} | ${copy.yearAgo} | ${copy.vsYearAgo} |`, '| --- | ---: | ---: | ---: | ---: | ---: |');
    for (const group of payload.groups) {
      lines.push(`| ${mdCell(group.name)} | ${money(group.totals.current?.netSalesAmount)} | ${money(group.totals.previous?.netSalesAmount)} | ${percent(group.totals.current?.netSalesAmount, group.totals.previous?.netSalesAmount)} | ${money(group.totals.yearAgo?.netSalesAmount)} | ${percent(group.totals.current?.netSalesAmount, group.totals.yearAgo?.netSalesAmount)} |`);
    }
    lines.push('');
  }

  lines.push(`## ${copy.categoryPerformance}`, '');
  const categories = payload.categories;
  if (categories.length === 0) {
    lines.push(copy.noRows, '');
  } else {
    lines.push(`| ${copy.category} | ${copy.current} | ${copy.quantity} |`, '| --- | ---: | ---: |');
    for (const group of categories) {
      lines.push(`| ${mdCell(group.code ? `${group.code} ${group.name}` : group.name)} | ${money(group.amount)} | ${qty(group.quantity)} |`);
    }
    lines.push('');
  }

  lines.push(`## ${copy.topSales}`, '');
  appendItemTable(lines, payload.topSales, copy);
  lines.push(`## ${copy.topQuantity}`, '');
  appendItemTable(lines, payload.topQuantity, copy);

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
    netSalesAmount: totals.netSalesAmount,
    netQuantity: totals.netQuantity,
    transactionCount: totals.transactionCount,
  };
}

function compactItem(item: SalesAnalysisRankedItem) {
  return {
    id: item.id,
    code: item.code,
    name: item.name,
    brand: item.brand ?? '',
    amount: item.amount,
    quantity: item.quantity,
  };
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
      audience: 'Microsoft Copilot',
      allStores: 'All stores',
      instructions: 'Instructions for Copilot',
      ruleSource: 'Use only the numbers in this file. If a figure is not here, say it is not in the file.',
      ruleNoInvent: 'Do not invent products, categories, stores, or amounts.',
      ruleCurrency: 'All money is Hong Kong dollars (HKD / HK$). Do not convert currency.',
      ruleScope: 'The tables below are already filtered. Do not add categories or products that are not listed.',
      ruleOutput: 'Answer in the user\'s language. Keep answers short. Every claim must cite a number from this file.',
      askTitle: 'Suggested questions',
      ask1: 'In 3 sentences, how did this period perform versus last period and last year?',
      ask2: 'Which listed products or categories dropped, and by how much?',
      ask3: 'Give at most 5 next-month actions. Each action must point to one number in the tables.',
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
      netSales: 'Net sales',
      netQuantity: 'Net quantity',
      transactions: 'Transactions',
      vsPrevious: 'vs previous',
      vsYearAgo: 'vs year ago',
      groupSummary: 'Promoter group summary',
      group: 'Group',
      current: 'Current',
      previous: 'Previous',
      yearAgo: 'Year ago',
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
    audience: 'Microsoft Copilot',
    allStores: '全部門店',
    instructions: '給 Copilot 的規則（必須遵守）',
    ruleSource: '只准使用這份檔案裡的數字。檔案沒有的數字，就寫「檔案沒有」。',
    ruleNoInvent: '不准發明商品、分類、門店或金額。',
    ruleCurrency: '金額一律是港幣（HKD / HK$），不要換算其他幣別。',
    ruleScope: '下面的表已經是篩選後的結果。沒有出現的分類或商品視為不在範圍，不是銷售為 0。',
    ruleOutput: '用繁體中文回答，短句。每一個判斷都要引用這份檔裡的一個數字。',
    askTitle: '建議這樣問',
    ask1: '用 3 句話說明本期相對上期、去年同期的表現。',
    ask2: '表裡哪些商品或分類在掉？掉多少？',
    ask3: '下個月最多給 5 個行動，每一點都要對到表裡的一個數字。',
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
    netSales: '淨銷售額',
    netQuantity: '淨銷售數量',
    transactions: '交易次數',
    vsPrevious: '較上期',
    vsYearAgo: '較去年同期',
    groupSummary: 'Promoter group 總結',
    group: '群組',
    current: '本期',
    previous: '上期',
    yearAgo: '去年同期',
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
