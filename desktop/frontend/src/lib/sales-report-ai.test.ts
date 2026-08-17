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
      key: 'yearAgo',
      totals: { saleQuantity: 2, saleAmount: 90, returnQuantity: 0, returnAmount: 0, netQuantity: 2, netSalesAmount: 90, transactionCount: 6 },
    }],
  };
}

describe('sales analysis AI export', () => {
  it('names the Copilot briefing beside the matching PDF', () => {
    expect(salesAnalysisAIFilename('107', '2026-08-01', '2026-08-16')).toBe('RTA-Sales-107-20260801-20260816-ai.md');
    expect(salesAnalysisAIFilename(ALL_STORES_REPORT_ID, '2026-08-01', '2026-08-16')).toBe('RTA-Sales-all-20260801-20260816-ai.md');
  });

  it('writes a compact Copilot markdown brief with group totals and JSON', () => {
    const markdown = buildSalesAnalysisAIMarkdown({
      locale: 'zh-TW',
      storeId: '107',
      storeLabel: '107 - Tai Wai',
      from: '2026-08-01',
      to: '2026-08-16',
      categoryLevel: 'category2',
      filter: defaultSalesReportFilter(),
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
          }],
        },
      }],
    });
    expect(markdown).toContain('Microsoft Copilot');
    expect(markdown).toContain('107 - Tai Wai');
    expect(markdown).toContain('我的護膚');
    expect(markdown).toContain('HK$180.00');
    expect(markdown).toContain('+100.0%');
    expect(markdown).toContain('只准使用這份檔案裡的數字');
    expect(markdown).toContain('不准發明商品');
    expect(markdown).toContain('```json');
    expect(markdown).toContain('"id": "g-skin"');
    expect(markdown).not.toContain('undefined');
  });

  it('tells Copilot the file is already filtered', () => {
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
});
