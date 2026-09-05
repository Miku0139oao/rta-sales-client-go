import { describe, expect, it } from 'vitest';
import { ALL_STORES_REPORT_ID } from './sales-report-pdf';
import { buildSalesAnalysisAIMarkdown, salesAnalysisAIFilename } from './sales-report-ai';
import { defaultSalesReportFilter } from './salesReportItems';
import type { SalesAnalysisReportMemo } from './types';

function memo(): SalesAnalysisReportMemo {
  return {
    periods: [{
      key: 'current',
      totals: { saleQuantity: 4, saleAmount: 180, returnQuantity: 0, returnAmount: 0, netQuantity: 4, netSalesAmount: 180, transactionCount: 12 },
      topAmount: [{ id: '552646', code: '552646', name: 'Mask', amount: 100, quantity: 2 }],
      topQuantity: [{ id: '552646', code: '552646', name: 'Mask', amount: 100, quantity: 2 }],
      amountGroups: [{ id: 'A02', code: 'A02', name: 'BEAUTY CARE', amount: 100, quantity: 2 }],
    }, {
      key: 'previous',
      totals: { saleQuantity: 3, saleAmount: 150, returnQuantity: 0, returnAmount: 0, netQuantity: 3, netSalesAmount: 150, transactionCount: 10 },
      amountGroups: [{ id: 'A02', code: 'A02', name: 'BEAUTY CARE', amount: 80, quantity: 1 }],
    }, {
      key: 'previous2',
      totals: { saleQuantity: 3, saleAmount: 120, returnQuantity: 0, returnAmount: 0, netQuantity: 3, netSalesAmount: 120, transactionCount: 9 },
      amountGroups: [{ id: 'A02', code: 'A02', name: 'BEAUTY CARE', amount: 70, quantity: 1 }],
    }, {
      key: 'yearAgo',
      totals: { saleQuantity: 2, saleAmount: 90, returnQuantity: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 90, transactionCount: 6 },
      amountGroups: [{ id: 'A02', code: 'A02', name: 'BEAUTY CARE', amount: 50, quantity: 1 }],
    }, {
      key: 'yearAgoNext',
      totals: { saleQuantity: 5, saleAmount: 200, returnQuantity: 0, returnAmount: 0, netQuantity: 5, netSalesAmount: 200, transactionCount: 14 },
      focusGroups: [{
        id: 'skin',
        prefix: 'A0201',
        name: '護膚',
        sales: [{ id: '552646', code: '552646', name: 'Mask', amount: 80, quantity: 2, currentAmount: 100, currentQuantity: 2 }],
      }],
    }],
  };
}

describe('sales analysis AI export', () => {
  it('names the AI briefing beside the matching PDF', () => {
    expect(salesAnalysisAIFilename('107', '2026-08-01', '2026-08-16')).toBe('RTA-Sales-107-20260801-20260816-ai.md');
    expect(salesAnalysisAIFilename(ALL_STORES_REPORT_ID, '2026-08-01', '2026-08-16')).toBe('RTA-Sales-all-20260801-20260816-ai.md');
  });

  it('writes a compact markdown brief for any model, with every analyzed period', () => {
    const markdown = buildSalesAnalysisAIMarkdown({
      locale: 'zh-TW',
      storeId: '107',
      storeLabel: '107 - Tai Wai',
      from: '2026-08-01',
      to: '2026-08-16',
      categoryLevel: 'category2',
      filter: defaultSalesReportFilter(),
      periodMeta: [
        { key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-16' },
        { key: 'previous', label: '上期', from: '2026-07-01', to: '2026-07-16' },
        { key: 'previous2', label: '前期', from: '2026-06-01', to: '2026-06-16' },
        { key: 'yearAgo', label: '去年同期', from: '2025-08-01', to: '2025-08-16' },
        { key: 'yearAgoNext', label: '去年下月', from: '2025-09-01', to: '2025-09-30' },
      ],
      base: memo(),
      groups: [{
        groupId: 'g-skin',
        groupName: '我的護膚',
        itemCodeCount: 1,
        memo: {
          periods: [{
            key: 'current',
            totals: { saleQuantity: 2, saleAmount: 100, returnQuantity: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 100 },
            topAmount: [{ id: '552646', code: '552646', name: 'Mask', amount: 100, quantity: 2 }],
          }, {
            key: 'previous',
            totals: { saleQuantity: 1, saleAmount: 80, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 80 },
          }, {
            key: 'previous2',
            totals: { saleQuantity: 1, saleAmount: 60, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 60 },
          }, {
            key: 'yearAgo',
            totals: { saleQuantity: 1, saleAmount: 50, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 50 },
          }],
        },
      }],
    });
    expect(markdown).toContain('任何大型語言模型');
    expect(markdown).toContain('這份檔就是完整任務與資料');
    expect(markdown).toContain('立刻寫出這份報告');
    expect(markdown).toContain('使用者不會再另外下指令');
    expect(markdown).not.toContain('建議這樣問');
    expect(markdown).not.toContain('Copilot');
    expect(markdown).not.toContain('microsoft-copilot');
    expect(markdown).toContain('107 - Tai Wai');
    expect(markdown).toContain('我的護膚');
    expect(markdown).toContain('HK$180.00');
    expect(markdown).toContain('+20.0%');
    expect(markdown).toContain('只准使用這份檔案裡的數字');
    expect(markdown).toContain('不准發明商品');
    expect(markdown).toContain('```json');
    expect(markdown).toContain('"id": "g-skin"');
    expect(markdown).not.toContain('undefined');
    expect(markdown).toContain('Top 商品只是前 24 名');
    expect(markdown).toContain('較上期');
    expect(markdown).toContain('較前期');
    expect(markdown).toContain('| 上期 | 2026-07-01 → 2026-07-16 | HK$150.00 |');
    expect(markdown).toContain('| 前期 | 2026-06-01 → 2026-06-16 | HK$120.00 |');
    expect(markdown).toContain('| 去年同期 | 2025-08-01 → 2025-08-16 | HK$90.00 |');
    expect(markdown).toContain('| 去年下月 | 2025-09-01 → 2025-09-30 | HK$200.00 |');
    expect(markdown).toContain('去年下月關注');
    expect(markdown).toContain('護膚');
    expect(markdown).toContain('"previous2"');
    expect(markdown).toContain('"yearAgoNext"');
  });

  it('rounds JSON money and does not ask the model to invent missing comparisons', () => {
    const markdown = buildSalesAnalysisAIMarkdown({
      locale: 'zh-TW',
      storeId: '107',
      storeLabel: '107 - Tai Wai',
      from: '2026-08-01',
      to: '2026-08-16',
      categoryLevel: 'category2',
      filter: defaultSalesReportFilter(),
      base: {
        periods: [{
          key: 'current',
          totals: { saleQuantity: 1, saleAmount: 461842.95999999996, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 461842.95999999996 },
          amountGroups: [{ id: 'A03', code: 'A03', name: 'PERSONAL CARE', amount: 461842.95999999996, quantity: 11338 }],
        }, {
          key: 'previous',
          totals: { saleQuantity: 1, saleAmount: 500000, returnQuantity: 0, returnAmount: 0, netQuantity: 1, netSalesAmount: 500000 },
          amountGroups: [{ id: 'A03', code: 'A03', name: 'PERSONAL CARE', amount: 500000, quantity: 12000 }],
        }],
      },
      groups: [],
    });
    expect(markdown).toContain('461842.96');
    expect(markdown).not.toContain('461842.95999999996');
    expect(markdown).toContain('HK$461,842.96');
    expect(markdown).toContain('-7.6%');
    expect(markdown).toContain('只用「各期間總數」裡的數字');
  });

  it('tells the model the file is already filtered', () => {
    const markdown = buildSalesAnalysisAIMarkdown({
      locale: 'zh-TW',
      storeId: '107',
      storeLabel: '107 - Tai Wai',
      from: '2026-08-01',
      to: '2026-08-16',
      categoryLevel: 'category2',
      filter: {
        ...defaultSalesReportFilter(),
        facets: { category5: ['A01010203  RESPIRATORY SYSTEM'] },
      },
      base: memo(),
      groups: [],
    });
    expect(markdown).toContain('小分類: A01010203  RESPIRATORY SYSTEM');
    expect(markdown).toContain('已限於下列篩選');
    expect(markdown).toContain('"alreadyFiltered": true');
  });

  it('names the actual ranking depth instead of a stale top-15 cap', () => {
    const markdown = buildSalesAnalysisAIMarkdown({
      locale: 'zh-TW',
      storeId: '107',
      storeLabel: '107 - Tai Wai',
      from: '2026-08-01',
      to: '2026-08-16',
      categoryLevel: 'category2',
      filter: defaultSalesReportFilter(),
      rankingLimit: 40,
      base: memo(),
      groups: [],
    });
    expect(markdown).toContain('Top 商品只是前 40 名');
    expect(markdown).toContain('不要把這 40 項加總當成全店銷售');
    expect(markdown).not.toContain('前 15 名');
    expect(markdown).not.toContain('前 24 名');
  });
});
