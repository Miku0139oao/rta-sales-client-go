import { it, expect } from "vitest";
import { unzipSync } from "fflate";
import { ReportBundle } from "./reportBundle";
it("packs different formats with exact bytes and rejects duplicate or unsafe names", async () => {
  const bundle = new ReportBundle();
  const bytes = new TextEncoder().encode("test");
  bundle.add("report.xlsx", bytes);
  bundle.add("report.pdf", bytes);
  expect(() => bundle.add("report.pdf", bytes)).toThrow();
  expect(() => bundle.add("../report.pdf", bytes)).toThrow();
  const files = unzipSync(await bundle.build());
  expect(Array.from(files["report.xlsx"]!)).toEqual(Array.from(bytes));
  expect(Object.keys(files)).toHaveLength(2);
});
