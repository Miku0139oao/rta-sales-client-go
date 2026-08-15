import type { AnalysisResult, PreviewFilter } from './types';

export function localDateKey(now = new Date()): string {
  return [
    now.getFullYear(),
    String(now.getMonth() + 1).padStart(2, '0'),
    String(now.getDate()).padStart(2, '0'),
  ].join('-');
}

export function defaultWorkbookEndDate(minimum: string, maximum: string, today = localDateKey()): string {
  return minimum && minimum <= today && today < maximum ? today : maximum;
}

export function initialPreviewFilter(result: AnalysisResult): PreviewFilter {
  if ((result.issueCount ?? 0) + (result.failedCount ?? 0) > 0) return 'issue';
  if ((result.changeCount ?? 0) > 0) return 'change';
  return 'all';
}
