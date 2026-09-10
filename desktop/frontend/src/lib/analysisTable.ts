export type CellValue = string | number | null;
export type CellFormat = 'text' | 'number' | 'money' | 'percent' | 'share';
export interface TableColumn { label: string; format: CellFormat }
export interface TableRow { cells: CellValue[]; secondary?: Record<number, string>; product?: { column: number; code: string; name: string }; fixed?: boolean; group?: string; exportCells?: CellValue[] }
export interface AnalysisTable { id: string; name: string; columns: TableColumn[]; rows: TableRow[]; exportColumns?: TableColumn[] }
export interface TableSort { column: number; direction: 'ascending' | 'descending' }
export interface AnalysisWorkbookRequest { filename: string; context: string[]; sheets: Array<{ name: string; columns: TableColumn[]; rows: CellValue[][] }> }
export const MAX_TABLE_CELLS = 500000;

export function sortAnalysisTable(table: AnalysisTable, sort: TableSort | undefined, locale: string): AnalysisTable {
  if (!sort || !table.columns[sort.column]) return table;
  const collator = new Intl.Collator(locale, { numeric: true, sensitivity: 'base' });
  const compare = (a: TableRow, b: TableRow) => {
    const left = a.cells[sort.column], right = b.cells[sort.column];
    // Unknown values always come last, in either direction. Sorting never mutates source rows.
    if (left === null || right === null) return left === right ? 0 : left === null ? 1 : -1;
    const value = typeof left === 'number' && typeof right === 'number' ? left - right : collator.compare(String(left), String(right));
    return sort.direction === 'ascending' ? value : -value;
  };
  // Weekly subtotals stay at the end of each original segment.
  const rows: TableRow[] = []; let segment: TableRow[] = [];
  for (const row of table.rows) {
    if (row.fixed || (segment.length && row.group !== segment[0]!.group)) { for (const sorted of segment.sort(compare)) rows.push(sorted); segment = []; }
    if (row.fixed) rows.push(row); else segment.push(row);
  }
  for (const sorted of segment.sort(compare)) rows.push(sorted);
  return { ...table, rows };
}
export function formatTableCell(value: CellValue, format: CellFormat, locale: string): string {
  if (value === null) return '—';
  if (typeof value === 'string') return value;
  return new Intl.NumberFormat(locale, format === 'money'
    ? { style: 'currency', currency: 'HKD', maximumFractionDigits: 2 }
    : format === 'percent' ? { style: 'percent', signDisplay: 'always', maximumFractionDigits: 1 }
      : format === 'share' ? { style: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 }
        : { maximumFractionDigits: 2 }).format(value);
}
function safeTSV(value: CellValue): string {
  if (value === null) return '';
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : '';
  const clean = value.replace(/[\t\r\n\u0000-\u001f]/g, ' ');
  // Spreadsheet paste must not turn product names into executable formulas.
  return /^[\s]*[=+@-]/.test(clean) || /^0\d+$/.test(clean) || /^\d{16,}$/.test(clean) ? `'${clean}` : clean;
}
function exportColumnsOf(table: AnalysisTable): TableColumn[] {
  return table.exportColumns ?? table.columns;
}
function exportCellsOf(row: TableRow): CellValue[] {
  return row.exportCells ?? row.cells;
}
export function analysisTableTSV(table: AnalysisTable): string {
  const columns = exportColumnsOf(table);
  return [columns.map((column) => safeTSV(column.label)).join('\t'),
    ...table.rows.map((row) => exportCellsOf(row).map((value, index) => {
      const format = columns[index]?.format;
      return (format === 'percent' || format === 'share') && typeof value === 'number'
        ? `${Number((value * 100).toFixed(4))}%` : safeTSV(value);
    }).join('\t'))].join('\r\n');
}
export function workbookSnapshot(tables: AnalysisTable[], context: string[], filename: string): AnalysisWorkbookRequest {
  const cells = tables.reduce((count, table) => count + table.rows.length * exportColumnsOf(table).length, 0);
  if (!tables.length || cells > MAX_TABLE_CELLS) throw new Error('table_limit');
  return { filename, context: [...context], sheets: tables.map((table) => ({ name: table.name,
    columns: exportColumnsOf(table).map((column) => ({ ...column })), rows: table.rows.map((row) => [...exportCellsOf(row)]) })) };
}
