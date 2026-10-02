import { writable } from 'svelte/store';
import type { SalesAnalysisPackedItems, SalesAnalysisResult, SalesAnalysisRequest } from './types';

export interface BrowserReport {
  version: 1;
  savedAt: string;
  profileId: string;
  profileName: string;
  result: SalesAnalysisResult;
  packed: Record<string, SalesAnalysisPackedItems>;
  localQuery?: SalesAnalysisRequest['localQuery'];
}
export const BROWSER_REPORT_KEY = 'rta-sales-report-v1';
const DB = 'rta-sales-report-cache-v1';
// Leave room for account settings and presets in the shared origin quota.
const LOCAL_LIMIT = 1_000_000;
export const browserReportStatus = writable<{
  operationId?: string;
  savedAt?: string;
  storage?: string;
  error?: boolean;
}>({});
let writes: Promise<unknown> = Promise.resolve();

async function database<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    const open = indexedDB.open(DB, 1);
    let abandoned = false;
    open.onupgradeneeded = () => open.result.createObjectStore('report');
    open.onerror = () => reject(open.error);
    open.onblocked = () => {
      abandoned = true;
      reject(new Error('Report storage blocked'));
    };
    open.onsuccess = () => {
      const db = open.result;
      if (abandoned) {
        db.close();
        return;
      }
      db.onversionchange = () => db.close();
      const tx = db.transaction('report', mode);
      let result: T;
      tx.oncomplete = () => {
        db.close();
        resolve(result);
      };
      tx.onabort = () => {
        db.close();
        reject(tx.error ?? new Error('Report storage failed'));
      };
      try {
        const request = action(tx.objectStore('report'));
        request.onsuccess = () => {
          result = request.result;
        };
      } catch (error) {
        tx.abort();
        db.close();
        reject(error);
      }
    };
  });
}

export function reportIsCached(report: BrowserReport): boolean {
  return (
    report.result.complete &&
    !report.result.pending &&
    Boolean(report.result.periods?.length) &&
    report.result.periods!.every((period) => {
      const batch = report.packed[period.key];
      return batch && (batch.r ?? batch.rows ?? []).length >= (period.itemCount ?? 0);
    })
  );
}
function valid(value: unknown): value is BrowserReport {
  if (!value || typeof value !== 'object') return false;
  const report = value as BrowserReport;
  return (
    report.version === 1 &&
    typeof report.savedAt === 'string' &&
    Number.isFinite(Date.parse(report.savedAt)) &&
    typeof report.profileId === 'string' &&
    typeof report.profileName === 'string' &&
    (!report.localQuery ||
      (['month', 'range'].includes(report.localQuery.periodMode) &&
        /^\d{4}-\d{2}$/.test(report.localQuery.month) &&
        typeof report.localQuery.weekCompare === 'boolean')) &&
    Boolean(report.result?.operationId) &&
    typeof report.result?.from === 'string' &&
    typeof report.result?.to === 'string' &&
    Array.isArray(report.result?.stores) &&
    typeof report.result?.totals?.netSalesAmount === 'number' &&
    Array.isArray(report.result?.periods) &&
    typeof report.packed === 'object' &&
    report.packed !== null &&
    report.result.periods.every((period) => {
      const batch = report.packed[period.key];
      return batch && Array.isArray(batch.d ?? batch.dict) && Array.isArray(batch.r ?? batch.rows);
    }) &&
    reportIsCached(report)
  );
}

export async function loadBrowserReport(): Promise<BrowserReport | null> {
  try {
    const raw = localStorage.getItem(BROWSER_REPORT_KEY);
    const marker = raw ? JSON.parse(raw) : null;
    const report =
      marker?.storage === 'IndexedDB'
        ? await database<BrowserReport>('readonly', (store) => store.get('latest'))
        : marker;
    if (!valid(report)) return null;
    browserReportStatus.set({
      operationId: report.result.operationId,
      savedAt: report.savedAt,
      storage: marker?.storage ?? 'localStorage',
    });
    return report;
  } catch {
    browserReportStatus.set({ error: true });
    return null;
  }
}

export function saveBrowserReport(report: BrowserReport): Promise<void> {
  if (!reportIsCached(report)) return Promise.resolve();
  const task = writes
    .catch(() => undefined)
    .then(async () => {
      // Large packed tables go straight to IndexedDB's structured clone, avoiding
      // a large synchronous JSON string on the UI thread just to measure quota.
      const batches = Object.values(report.packed);
      const large =
        batches.reduce((count, batch) => count + (batch.r ?? batch.rows ?? []).length, 0) > 4000 ||
        batches.some(
          (batch) => (batch.d ?? batch.dict ?? []).reduce((length, text) => length + text.length, 0) > 300_000,
        );
      const json = large ? '' : JSON.stringify(report);
      let storage = 'localStorage';
      let localSaved = false;
      if (!large && json.length <= LOCAL_LIMIT) {
        try {
          localStorage.setItem(BROWSER_REPORT_KEY, json);
          localSaved = true;
        } catch {
          /* use larger browser storage */
        }
      }
      if (!localSaved) {
        await database('readwrite', (store) => store.put(report, 'latest'));
        // The small marker determines which committed storage is authoritative.
        localStorage.setItem(BROWSER_REPORT_KEY, JSON.stringify({ storage: 'IndexedDB' }));
        storage = 'IndexedDB';
      } else {
        // Remove an older large report; never retain multiple full reports.
        if (typeof indexedDB !== 'undefined')
          await database('readwrite', (store) => store.delete('latest')).catch(() => undefined);
      }
      browserReportStatus.set({ operationId: report.result.operationId, savedAt: report.savedAt, storage });
    })
    .catch((error) => {
      browserReportStatus.set({ operationId: report.result.operationId, error: true });
      throw error;
    });
  writes = task;
  return task;
}

export function clearBrowserReport(): Promise<void> {
  const task = writes
    .catch(() => undefined)
    .then(async () => {
      // Clear the large backing record before removing its pointer. Failures
      // remain visible and retryable rather than falsely claiming deletion.
      if (typeof indexedDB !== 'undefined') await database('readwrite', (store) => store.delete('latest'));
      localStorage.removeItem(BROWSER_REPORT_KEY);
      browserReportStatus.set({});
    });
  writes = task;
  return task;
}
