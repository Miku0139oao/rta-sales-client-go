import { ALL_STORES_REPORT_ID, isAllStoresReport, salesAnalysisPDFFilename } from './sales-report-pdf';
import type { SalesReportFilter } from './salesReportItems';
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
  const payload = {
    storeId: isAllStoresReport(input.storeId) ? ALL_STORES_REPORT_ID : input.storeId,
    storeLabel: storeName,
    from: input.from,
    to: input.to,
    categoryLevel: input.categoryLevel,
    filter: {
      mode: input.filter.mode,
      excludeZeroGifts: input.filter.excludeZeroGifts,
      excludeStamps: input.filter.excludeStamps,
      categories: input.filter.categories,
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
    `store: ${yamlEscape(storeName)}`,
    `period: ${input.from} / ${input.to}`,
    `---`,
    ``,
    `# ${copy.title} — ${storeName}`,
    ``,
    copy.howto,
    ``,
    `## ${copy.snapshot}`,
    ``,
    `- ${copy.store}: ${storeName}`,
    `- ${copy.period}: ${input.from} → ${input.to}`,
    `- ${copy.netSales}: ${money(current?.totals?.netSalesAmount)} (${copy.vsPrevious} ${percent(current?.totals?.netSalesAmount, previous?.totals?.netSalesAmount)}; ${copy.vsYearAgo} ${percent(current?.totals?.netSalesAmount, yearAgo?.totals?.netSalesAmount)})`,
    `- ${copy.netQuantity}: ${qty(current?.totals?.netQuantity)} (${copy.vsYearAgo} ${percent(current?.totals?.netQuantity, yearAgo?.totals?.netQuantity)})`,
    `- ${copy.transactions}: ${qty(current?.totals?.transactionCount)}`,
    ``,
    `## ${copy.groupSummary}`,
    ``,
  ];

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
  lines.push('```json', JSON.stringify(payload, null, 2), '```', '');
  return lines.join('\n');
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
      allStores: 'All stores',
      howto: [
        'Use this file as the only source for Microsoft Copilot.',
        'Upload it to Windows Copilot, copilot.microsoft.com, or Microsoft 365 Copilot, then ask for exceptions, declining groups, and where to put effort next month.',
        'Answer in the user\'s language. Currency is HKD. Do not invent numbers that are not in this file.',
      ].join(' '),
      snapshot: 'Snapshot',
      store: 'Store',
      period: 'Period',
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
      structured: 'Structured data',
    };
  }
  return {
    title: 'RTA 銷售分析',
    allStores: '全部門店',
    howto: [
      '這份檔案是給 Microsoft Copilot 用的分析摘要。',
      '上傳到 Windows Copilot、copilot.microsoft.com 或 Microsoft 365 Copilot，然後問異常、掉量群組，以及下個月人力該放哪。',
      '請用繁中回答。金額是港幣。不要發明這份檔沒有出現的數字。',
    ].join(''),
    snapshot: '總覽',
    store: '門店',
    period: '期間',
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
    structured: '結構化資料',
  };
}
