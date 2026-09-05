# 分析工具完整版：排序、商品詳情、查詢捷徑與 Excel

> 最新版本另包含 Grok UI、可追查洞察及競態修復，見[全面精修交付與驗證](analysis-refinement.md)。

## 新增操作

### 表格排序

- 商品、分類比較、門店比較、每週變化，以及商品詳情表格均可點欄名排序。
- 首次點擊由大到小，再點切換為由小到大；欄頭以方向與 `aria-sort` 表示目前順序。鍵盤亦可操作。
- 數值以實際數字比較；缺值始終排在末端，同值維持原順序。每週的分組合計與總計不會被混入商品／門店列。
- 商品表格每頁 50 筆；搜尋或排序後回到第一頁。二十萬筆排序有回歸測試，不使用可能超出參數上限的大型展開呼叫。
- 排行卡仍依其銷售額／數量規則排列，保留 16／24／32 與自訂 5–100，Focus 深度不變。

### 商品詳情

- 從概覽、分類、關注排行或商品表格點商品名稱即可開啟。
- 顯示相同商品編碼的跨期間銷售、數量、退貨，以及本期對上期變化；可切換期間查看各門店明細。
- 範圍是現有報表的門店和期間，不套用主畫面的分類／商品群組篩選；不執行新的 RTA 銷售查詢。
- 若報表只有摘要，沿用既有明細佇列取得該次操作的已保存資料；失敗期間可單獨重試。
- 未載入或失敗資料顯示不可用，不當作零銷售；已成功取得門店資料但沒有該商品時才顯示零。未完整期間不提供誤導的成長率。
- 視窗有固定關閉區域、窄螢幕捲動、Escape 關閉及焦點復原。

### 釘選與最近使用

- 在「常用條件」選取已儲存項目，按「釘選到主頁」，最多 3 組。
- 主頁另顯示最近成功查詢使用的 3 組未釘選條件，避免重複。
- 捷徑只帶入草稿；仍需明確按「開始分析／重新分析」。取消或查詢失敗不記錄最近使用。
- 重新命名及更新條件保留釘選與使用時間；刪除中的查詢較晚完成也不會復活該預設。
- 相容原本 v1 資料；新增的只有可選 `pinned`、`lastUsedAt`，仍不保存密碼或報表。
- 最近使用紀錄寫入失敗會提示，但不丟失成功的分析結果。

### Excel 與複製

- 「匯出此頁 Excel」使用當下頁籤、商品篩選、排序與排行深度的快照，包含所有分頁，不僅眼前 50 筆。
- 多表格頁籤會分工作表；商品詳情也能單獨匯出。說明頁記錄報表帳號、日期及篩選範圍，不使用未套用的查詢草稿。
- 每週數據沿用全店範圍，不套用商品篩選，並在匯出說明中明確標記。
- 數字保留數值型別，商品編碼保持文字與前導零；表頭加色、凍結並提供 Excel 篩選。文字不會變成可執行公式。
- 原生版選擇目錄後儲存；同名檔案自動改用序號，不覆蓋原檔。網頁版使用下載，HTTP 不暴露原生檔案寫入方法。
- 「複製表格」輸出可貼入 Excel 的 TSV；多表格可先選擇要複製的表。剪貼簿權限受限時提供已選取的手動複製文字。
- 單次限制 50 萬個資料儲存格；超出時請縮小篩選。資料仍缺少時先補齊明細，避免匯出不完整表格。
- PDF／AI 匯出沿用既有流程，不因新增 Excel 按鈕而改變。

## 驗證

- `cd desktop/frontend && bun run verify`：**233 項測試、30 個檔案全部通過**；svelte-check **0 errors / 0 warnings**，正式前端建置成功。
- `go test ./desktop -count=1 -timeout 180s`：桌面／網頁後端完整測試通過；新增 XLSX 型別、公式安全、參數限制、取消、防覆蓋與 RPC 邊界測試。
- 三套 Edge 腳本各通過 1440×1000、1024×768、390×844、深色模式，共 **12 個情境**。
- 新腳本實際產生、下載並以 Excelize 重讀 XLSX；確認 73 筆資料跨兩頁、數值排序、前導零，以及跨店詳情的期間數據。
- 套用捷徑、排序、詳情、Excel 與複製不增加分析呼叫；只有明確按分析才建立一次合成查詢。
- 檢視了桌面與窄螢幕截圖；未出現頁面橫向溢出、視窗超界、未捕捉例外或未預期 API。

Vite 仍提示 Node 模組外部化、混合靜態／動態匯入，以及主 bundle 約 508 kB 的分塊建議；不阻擋建置。這一輪沒有進行真實 RTA 查詢／速度量測、race 測試、原生 GUI 實際操作或 PDF 視覺驗證，不將瀏覽器測試當成這些項目的替代。

### 重跑瀏覽器驗證

在專案根目錄先建立只處理合成工作簿的輔助程式：

```bash
go build -o desktop/frontend/node_modules/.cache/workbook-fixture.exe desktop/frontend/scripts/workbook-fixture.go
```

在 `desktop/frontend` 啟動 `bun run dev:web --host 127.0.0.1 --port 5179`，另一終端執行：

```bash
UI_BROWSER=msedge node scripts/analysis-tools-smoke.mjs
UI_BROWSER=msedge node scripts/analysis-presets-smoke.mjs
UI_BROWSER=msedge node scripts/analysis-ui-smoke.mjs
```

可用 `PLAYWRIGHT_MODULE=file:///.../playwright-core/index.js` 指定既有 Playwright，未新增正式依賴。支援 `UI_ORIGIN`、`UI_OUTPUT`；新腳本另支援 `WORKBOOK_FIXTURE` 指定輔助執行檔。所有 API 均被攔截，沒有真實 RTA 連線。

最新紀錄位於 `desktop/frontend/node_modules/.cache/analysis-tools/`、`tools-presets-regression/`、`tools-ui-regression/`。僅用於本輪的 Vite 程序已關閉。

## 獨立 Windows 交付

- 執行檔：`release/RTA-Excel-Filler-analysis-tools-preview.exe`
- 大小：**25,886,504 bytes**
- Microsoft Trusted Signing；Authenticode **Valid / Signature verified.**
- SHA256：`6981cc6ed6e2d00ac37abe44942ff18225d9b4708371654edba8dd3e0f9844f0`
- 校驗碼與四張操作截圖存於 `release/`，使用 `analysis-tools-` 前綴。

請先自行關閉舊版，再開啟這個預覽版。保留既有 ui／ux／presets 預覽、正式 portable、installer 及所有原始執行檔，未強制結束正在使用的桌面程式；本輪未建立安裝包，也未提交 Git commit。
