import "fake-indexeddb/auto";
import { beforeEach, describe, it, expect, vi } from "vitest";
import {
  listHistory,
  saveHistory,
  deleteHistory,
  compareHistory,
  type HistoryReport,
} from "./reportHistory";
vi.mock("./runtime", () => ({ isWebRuntime: () => true }));
const report = (amount = 100): HistoryReport => ({
  version: 1,
  id: crypto.randomUUID(),
  savedAt: new Date().toISOString(),
  name: "Test",
  from: "2026-09-01",
  to: "2026-09-30",
  scope: "account:107",
  scopeLabel: "107",
  complete: true,
  totals: {
    saleQuantity: 1,
    saleAmount: amount,
    returnQuantity: 0,
    returnAmount: 0,
    netQuantity: 1,
    netSalesAmount: amount,
  },
  stores: [],
});
beforeEach(async () => {
  for (const entry of await listHistory()) await deleteHistory(entry.id);
});
describe("local report history", () => {
  it("commits, reloads and deletes summaries without replacing an existing ID", async () => {
    const a = report();
    await saveHistory(a);
    await expect(saveHistory({ ...a, name: "Changed" })).rejects.toThrow();
    expect(await listHistory()).toEqual([a]);
    await deleteHistory(a.id);
    expect(await listHistory()).toEqual([]);
  });
  it("retains all existing reports when the limit or completeness check fails", async () => {
    for (let i = 0; i < 20; i++) await saveHistory(report(i));
    await expect(saveHistory(report())).rejects.toThrow();
    await expect(
      saveHistory({ ...report(), complete: false }),
    ).rejects.toThrow();
    expect(await listHistory()).toHaveLength(20);
  });
  it("requires matching scope and completeness and handles zero and negative baselines", () => {
    expect(compareHistory(report(150), report(100)).percent).toBe(0.5);
    expect(compareHistory(report(10), report(0)).percent).toBeUndefined();
    expect(compareHistory(report(0), report(-10)).percent).toBe(1);
    expect(
      compareHistory(report(), { ...report(), scope: "other" }).comparable,
    ).toBe(false);
    expect(
      compareHistory(report(), { ...report(), complete: false }).difference,
    ).toBeUndefined();
  });
});
