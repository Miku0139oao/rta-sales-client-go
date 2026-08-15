import { describe, expect, it } from 'vitest';
import { defaultWorkbookEndDate, initialPreviewFilter, localDateKey } from './excelWorkflow';
import type { AnalysisResult } from './types';

function result(overrides: Partial<AnalysisResult> = {}): AnalysisResult {
  return {
    operationId: 'operation', complete: true, changedCellCount: 0, problemCount: 0,
    aggregateProblemCount: 0, preview: [], totalCount: 0, changeCount: 0,
    unchangedCount: 0, issueCount: 0, failedCount: 0, overlapCount: 0,
    issues: [], canApply: false, ...overrides,
  };
}

describe('Excel workflow defaults', () => {
  it('uses a stable local ISO date key', () => {
    expect(localDateKey(new Date(2026, 7, 5, 23, 59))).toBe('2026-08-05');
  });

  it('stops a current-month workbook at today without changing historical ranges', () => {
    expect(defaultWorkbookEndDate('2026-08-01', '2026-08-31', '2026-08-15')).toBe('2026-08-15');
    expect(defaultWorkbookEndDate('2026-07-01', '2026-07-31', '2026-08-15')).toBe('2026-07-31');
  });

  it('focuses blocking rows first, then changes', () => {
    expect(initialPreviewFilter(result({ issueCount: 1, changeCount: 2 }))).toBe('issue');
    expect(initialPreviewFilter(result({ changeCount: 2 }))).toBe('change');
    expect(initialPreviewFilter(result())).toBe('all');
  });
});
