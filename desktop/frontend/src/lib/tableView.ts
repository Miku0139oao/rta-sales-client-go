import type { AnalysisTable } from "./analysisTable";
export const tablePositions = new Map<
  string,
  { page: number; top: number; left: number }
>();
export function filterAnalysisTable(
  table: AnalysisTable,
  query: string,
): AnalysisTable {
  const term = query.trim().normalize("NFKC").toLocaleLowerCase();
  if (!term) return table;
  return {
    ...table,
    rows: table.rows.filter(
      (row) =>
        !row.fixed &&
        row.cells.some(
          (cell) =>
            cell !== null &&
            String(cell).normalize("NFKC").toLocaleLowerCase().includes(term),
        ),
    ),
  };
}
export function readColumns(key: string, count: number): number[] {
  try {
    const saved = JSON.parse(
      localStorage.getItem(`rta-table-columns-${key}`) ?? "null",
    );
    if (Array.isArray(saved)) {
      const columns = [
        ...new Set<number>(
          saved.filter(
            (i: unknown) =>
              typeof i === "number" &&
              Number.isInteger(i) &&
              i >= 0 &&
              i < count,
          ),
        ),
      ];
      if (columns.length) return columns;
    }
  } catch {
    /* Optional preferences cannot block report reading. */
  }
  return Array.from({ length: count }, (_, i) => i);
}
