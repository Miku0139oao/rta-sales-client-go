# 常用查詢與篩選條件

後續已加入最多 3 組釘選、最近成功使用捷徑及完整分析工具，最新預覽版與驗證請見 [分析工具完整版](analysis-tools.md)。以下保留初版功能與驗證紀錄。

## 使用方式

1. 選好帳號、月份／日期範圍與門店；如有報表，可先設定商品搜尋、分類及商品群組。
2. 按分析頁右上方「常用條件」。下方會列出目前準備儲存的查詢摘要。
3. 選擇期間規則、輸入名稱，再按「另存常用條件」。最多保存 20 組，名稱為 1–60 字。
4. 下次在選單選取已儲存的條件，按「套用條件」。確認門店後，再明確按「開始分析／重新分析」。

### 期間規則

- **保留目前選定的期間**：固定月份或日期範圍；日期範圍的星期比較選項也會儲存。
- **每次套用本月／上月**：依套用當天的本機月份計算，跨年也會正確調整。視窗會顯示目前對應月份。

### 保存與管理

- 保存目前查詢草稿的帳號、期間、門店，以及商品搜尋、五級分類、商品群組及分類排行層級。
- 可重新命名；更新為目前條件與刪除均需要確認。相同名稱不會默默覆蓋。
- 僅存於此裝置的應用程式／瀏覽器 localStorage，鍵為 `rta-sales-analysis-presets-v1`；不保存密碼、登入憑證或報表明細，不跨裝置同步。
- 資料無法讀取時不覆蓋原始內容；儲存失敗會顯示錯誤，不顯示成功通知。

### 套用安全性

- 套用只帶入草稿，不自動執行銷售查詢、不清除既有報表，也不改動既有報表的畫面篩選。
- 商品篩選於下一次明確執行且成功回傳分析後套用。分析失敗時保留待套用條件供重試。
- 帳號或商品群組不可用時拒絕套用。會重新確認可用門店，略過已不存在的門店並列出警告；不自動加入新門店。若全部不可用則不更動草稿。
- 可「放棄條件變更」；尚無報表時可「取消套用常用條件」。

## 驗證

```bash
cd desktop/frontend
bun run verify
```

203 項前端測試通過，包含新增的 13 項儲存測試及 13 項整合測試。svelte-check 0 errors / 0 warnings，production 前端建置成功。Vite 仍有既有模組 externalization 與靜態／動態匯入提示。

瀏覽器測試：

```bash
# 終端一，在 desktop/frontend
bun run dev:web --host 127.0.0.1 --port 5179

# 終端二，同目錄；需可匯入的 Playwright 及 Edge
UI_BROWSER=msedge node scripts/analysis-presets-smoke.mjs
UI_BROWSER=msedge node scripts/analysis-ui-smoke.mjs
```

可設定 `PLAYWRIGHT_MODULE=file:///.../playwright-core/index.js`、`UI_ORIGIN`、`UI_OUTPUT`。不新增 Playwright 至正式依賴。

兩組腳本皆在 Edge 的 1440×1000、1024×768、390×844 及深色模式通過。常用條件腳本涵蓋儲存、跨頁面重新載入、動態月份、只帶入不執行、明確執行後套用篩選、重新命名、刪除確認及 Escape 焦點回復；所有 API 均以合成資料攔截，不連線到真實 RTA。

常用條件畫面與結果：`desktop/frontend/node_modules/.cache/analysis-presets/`。舊功能回歸結果：`desktop/frontend/node_modules/.cache/analysis-presets-regression/`。

## Windows 預覽版

最新執行檔：`release/RTA-Excel-Filler-presets-preview.exe`（25,786,656 bytes）。使用 production 編譯旗標與 Windows 資源，Microsoft Trusted Signing 簽署成功，Authenticode 驗證為 Valid（Signature verified）。

SHA256：`e7dd9af6ca7dcc0e44de53a0f04bc117240a1e63a8f332219469c12ba471e35c`

保留先前所有執行檔，不覆蓋正在執行的程式。請先自行關閉舊版再開啟本預覽。此輪未啟動原生 Windows 程式，未執行真實 RTA 查詢或實際匯出 PDF；Go 後端未修改，未重跑其測試。
