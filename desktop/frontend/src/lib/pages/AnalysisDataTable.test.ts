import { render, cleanup, screen, fireEvent } from "@testing-library/svelte";
import { afterEach, it, expect, vi } from "vitest";
import AnalysisDataTable from "./AnalysisDataTable.svelte";
import { translator } from "../i18n";
import { filterAnalysisTable } from "../tableView";
import { workbookSnapshot, type AnalysisTable } from "../analysisTable";
afterEach(cleanup);
it("keeps 200k rows bounded in the DOM, exposes search and preserves complete export data", async () => {
  const table: AnalysisTable = {
    id: "large",
    name: "Large",
    columns: [{ label: "SKU", format: "text" }],
    rows: Array.from({ length: 200000 }, (_, i) => ({ cells: [String(i)] })),
  };
  const onSearch = vi.fn();
  render(AnalysisDataTable, {
    props: {
      table,
      t: translator("en"),
      locale: "en",
      onSort: vi.fn(),
      onSearch,
    },
  });
  expect(screen.getAllByRole("row").length).toBeLessThan(30);
  await fireEvent.input(screen.getByRole("searchbox"), {
    target: { value: "199999" },
  });
  expect(onSearch).toHaveBeenCalledWith("large", "199999");
  const filtered = filterAnalysisTable(table, "199999");
  expect(workbookSnapshot([filtered], [], "test.xlsx").sheets[0]?.rows).toEqual(
    [["199999"]],
  );
});
