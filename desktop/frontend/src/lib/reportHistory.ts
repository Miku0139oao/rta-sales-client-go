import { callBackend } from "./backend";
import { isWebRuntime } from "./runtime";
import type { SalesAnalysisTotals, SalesAnalysisStoreSummary } from "./types";

export interface HistoryReport {
  version: 1;
  id: string;
  savedAt: string;
  name: string;
  from: string;
  to: string;
  scope: string;
  scopeLabel: string;
  complete: boolean;
  totals: SalesAnalysisTotals;
  stores: SalesAnalysisStoreSummary[];
}
const LIMIT = 20;
const DB = "rta-sales-report-history-v1";
async function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB, 1);
    request.onupgradeneeded = () =>
      request.result.createObjectStore("reports", { keyPath: "id" });
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
    request.onblocked = () =>
      reject(
        new Error("History database is blocked / 請關閉其他舊版分頁後重試"),
      );
  });
}
async function transaction<T>(
  mode: IDBTransactionMode,
  action: (store: IDBObjectStore, set: (value: T) => void) => void,
): Promise<T> {
  const db = await database();
  return new Promise((resolve, reject) => {
    let value: T;
    const tx = db.transaction("reports", mode);
    tx.oncomplete = () => {
      db.close();
      resolve(value);
    };
    tx.onabort = () => {
      db.close();
      reject(
        tx.error ?? new Error("History storage failed / 歷史資料儲存失敗"),
      );
    };
    tx.onerror = () => {
      /* onabort reports the failure; never claim success before commit */
    };
    try {
      action(tx.objectStore("reports"), (next) => {
        value = next;
      });
    } catch (error) {
      tx.abort();
      db.close();
      reject(error);
    }
  });
}
export async function listHistory(): Promise<HistoryReport[]> {
  const reports = isWebRuntime()
    ? await transaction<HistoryReport[]>("readonly", (store, set) => {
        const request = store.getAll();
        request.onsuccess = () => set(request.result);
      })
    : await callBackend<HistoryReport[]>("ListReportHistory");
  if (
    reports.some(
      (report) =>
        report.version !== 1 || !report.id || !report.totals || !report.stores,
    )
  )
    throw new Error("Invalid report history / 歷史報表格式損壞");
  return reports.sort((a, b) => b.savedAt.localeCompare(a.savedAt));
}
export async function saveHistory(report: HistoryReport): Promise<void> {
  if (!report.complete)
    throw new Error("Only complete reports can be saved / 只能儲存完整報表");
  if (new TextEncoder().encode(JSON.stringify(report)).length > 2 * 1024 * 1024)
    throw new Error("History too large / 歷史報表過大");
  if (!isWebRuntime()) return callBackend<void>("SaveReportHistory", [report]);
  return transaction<void>("readwrite", (store, set) => {
    const count = store.count();
    count.onsuccess = () => {
      if (count.result >= LIMIT) {
        /* Abort preserves existing reports. */ throwLimit();
      } else {
        store.add(report);
        set(undefined);
      }
    };
    function throwLimit() {
      store.transaction.abort();
    }
  }).catch((error) => {
    throw new Error(
      `Unable to save history (maximum 20 reports; browser storage may be full) / 無法儲存歷史報表（上限 20 份，或瀏覽器空間不足）：${error instanceof Error ? error.message : error}`,
    );
  });
}
export async function deleteHistory(id: string): Promise<void> {
  if (!isWebRuntime()) return callBackend<void>("DeleteReportHistory", [id]);
  return transaction<void>("readwrite", (store, set) => {
    store.delete(id);
    set(undefined);
  });
}
export function compareHistory(
  current: HistoryReport,
  previous: HistoryReport,
) {
  const comparable =
    current.complete && previous.complete && current.scope === previous.scope;
  const difference = comparable
    ? current.totals.netSalesAmount - previous.totals.netSalesAmount
    : undefined;
  const percent =
    difference !== undefined && previous.totals.netSalesAmount !== 0
      ? difference / Math.abs(previous.totals.netSalesAmount)
      : undefined;
  return { comparable, difference, percent };
}
