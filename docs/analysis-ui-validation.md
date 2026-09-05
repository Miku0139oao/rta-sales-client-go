# 分析頁 UI 重整與驗證

最新的排序、商品詳情、釘選／最近使用與 Excel／複製功能，233 項測試結果及新版交付，請見 [分析工具完整版](analysis-tools.md)。常用查詢的原始功能另見 [常用條件說明](analysis-presets.md)。以下保留前兩輪驗證紀錄。

## 操作變更

- 頁首顯示實際報表的帳號、完整日期及門店數，不會跟著尚未套用的查詢條件變動。舊快照缺乏帳號來源時顯示「已儲存報表」。
- 查詢可展開／收合；修改後保留結果並提示重新分析，不會自動重新查詢。
- 只有精簡頁籤／排行列吸頂；完整比較期間與銷售表現明細改為按需展開。
- 搜尋常駐。分類及商品範圍可展開；已選條件顯示可移除標籤，支援清除全部篩選。
- 四項主要指標與次要數據分層，商品排行跟隨整頁捲動，移除多層排行捲軸。
- 保留 16／24／32 與自訂 5–100；概覽、分類、匯出同步更新。空白輸入保留原值、Escape 放棄輸入。
- 頁籤支援方向鍵及 Home／End；分類選單支援搜尋與 Escape 返回觸發按鈕。從長排行切換頁籤會回到新報表開頭。

## 第二輪：操作復原與明細重試

- 新增「放棄條件變更」，還原該報表使用的帳號、期間及門店；不重跑分析，亦不受較晚回傳的門店清單干擾。
- 門店及分類的批次選取只作用於目前搜尋結果；未顯示的選取保持不變。
- 明細採共用、序列載入佇列，合併重複請求。成功期間即時保留，失敗期間可單獨重試，不重跑整份分析。
- 明細不足時顯示載入中或失敗原因，不將未載入的商品／篩選後報表誤顯示為零筆結果。舊操作的延遲回覆不影響新報表。
- 吸頂列新增「回到搜尋與篩選」，捲動至搜尋框並聚焦，保留既有篩選。
- 在等待前次分析取消前即鎖定新提交，避免快速連點建立重複查詢。

## 已執行的驗證

```bash
cd desktop/frontend
bun run verify
```

結果：177 項測試通過（分析頁 46 項）；svelte-check 0 errors / 0 warnings；正式前端建置成功。Vite 仍有既有 Node 模組 externalization 與混用靜態／動態匯入的提示，未阻擋建置。

```bash
go test ./desktop -count=1 -timeout 180s -run 'SalesAnalysis|AccountQuery'
```

結果：前一輪通過。本輪僅調整前端，未重跑 Go 測試；未進行真實 RTA 查詢、速度量測或 race 檢查。

## 可重跑的瀏覽器驗證

先在 `desktop/frontend` 啟動：

```bash
bun run dev:web --host 127.0.0.1 --port 5179
```

另一個終端在同目錄執行（需可匯入的 Playwright 與已安裝瀏覽器）：

```bash
UI_BROWSER=msedge node scripts/analysis-ui-smoke.mjs
```

可指定 `PLAYWRIGHT_MODULE=file:///.../playwright-core/index.js`、`UI_ORIGIN` 及 `UI_OUTPUT`。Playwright 不加入正式應用依賴。

腳本注入合成報表，攔截並模擬事件串流，阻擋查詢 API，避免使用真實帳號或資料。

已在 Edge 驗證：1440×1000、1024×768、390×844，以及深色主題。涵蓋橫向溢出、篩選選單、完整年份、比較表展開、100 筆排行、設定儲存、吸頂、切頁回到內容開頭與匯出視窗。第二輪另驗證搜尋範圍批次選取、放棄條件變更及返回搜尋框的可見位置與焦點。最新截圖及 JSON 結果寫入 `desktop/frontend/node_modules/.cache/analysis-ux/`（使用 `UI_OUTPUT=node_modules/.cache/analysis-ux`）。

## 獨立 Windows 預覽版

第二輪預覽版為 `release/RTA-Excel-Filler-ux-preview.exe`，使用 production 編譯旗標與 Windows 資源建置，保留先前的 `ui-preview.exe` 及所有原有執行檔。Authenticode 簽章驗證結果為 Valid（Signature verified）。

SHA256：`0d28d19ef5bf32d3f7857db8bc440b31abe81c5cbbf1621ef5fe1504802b696d`

此輪未啟動預覽版執行檔，也未另製安裝包；瀏覽器驗證涵蓋匯出視窗，未實際產出 PDF。請先關閉舊版，再開啟新版預覽。
