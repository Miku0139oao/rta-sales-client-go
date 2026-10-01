# Local report workflows

Open **常用條件** in sales analysis. Save the current account, period rule, store selection and product filters, together with the export formats. Select a saved recipe and press **一鍵查詢並匯出**. The workflow reloads store permissions, stops if saved stores are unavailable, waits for comparison periods, loads report details, and exports only after the result is complete. It does not schedule background jobs.

Desktop bundles are written to a user-selected folder with collision-safe filenames. Web bundles remain in memory until the user presses **下載 ZIP 報表包**. A browser refresh discards an undownloaded bundle. AI summaries use the existing local report formatter and do not call an external model.

The **歷史與比較** dialog saves complete current-period summaries, including the active account/store/product scope and store totals. Native records use `report-history-v1` under the existing report cache; browser records use the `rta-sales-report-history-v1` IndexedDB database. Versioned records have unique IDs, a 2 MiB size limit, and a 20-record limit. No existing record is overwritten or automatically removed. Browser storage failures are surfaced only after the transaction aborts; success is shown after commit.

Comparisons require identical scope and complete data. Dates may differ. History contains totals and store summaries, not full SKU snapshots. JSON backup export is available; backup import and device synchronization are not implemented.

Failed query retry requires the in-memory execution context of the current report. It reruns failed jobs only, verifies fresh permissions, and reaggregates immutable successful outcomes with the new results. A restored historical/latest snapshot needs a new query to regain retry context.

Table searches filter every row before pagination/virtual rendering and before workbook/clipboard export. Fixed subtotals are omitted when table search is active because their original totals describe the unfiltered scope. Column visibility is a reading preference; exports keep all columns. Reading positions are bounded to the most recent 100 table instances.
