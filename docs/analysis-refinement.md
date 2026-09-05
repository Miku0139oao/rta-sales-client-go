# 分析頁全面精修：Grok UI、可追查洞察與可靠性

> 後續已完成首屏、貢獻拆解、效能及匯出驗證，最新交付見[分析優化紀錄](analysis-optimization.md)。

## 交付

- 新的獨立簽章預覽版：`release/RTA-Excel-Filler-refined-preview.exe`。
- 大小：25,908,008 bytes；Authenticode：**Valid / Signature verified.**
- SHA256：`14411e22a58c72b1def02cb8827190d80348705fcc7fab9475fa040e768163ab`。
- 保留所有舊預覽版及正式 portable／installer，不替換或關閉使用者正在執行的正式程式。

## UI 精修

Grok 4.6 實作版面調整，再經獨立回歸審查與整合：

- 調整報表身份、導覽、搜尋與表格工具的層級；維持六個頁籤及既有查詢／篩選行為。
- 商品詳情先呈現銷售額、數量與比較；窄螢幕亦優先看到金額，而不是先被日期佔滿。
- 商品詳情的畫面欄位順序可以不同於 Excel 原始欄位順序，但排序仍對應正確欄位，所有頁面資料仍完整匯出。
- 鍵盤分頁切換、Escape、焦點復原、淺／深色與窄螢幕操作仍受測試保護。
- 修正審查發現的覆蓋率錯誤：未完整或有未知門店時，「有銷售門店數」顯示不可用，不把未知門店算成零銷售。

## 本機分析重點

概覽最多顯示三項重點，不外傳銷售資料，也不自動重跑查詢：

1. 淨銷售額下滑最多、成長最多的商品。
2. 退款絕對金額最高的商品。
3. 若仍有空位，補充尚未出現的營收主力商品。

按相同商品編碼跨門店加總，主畫面的同一篩選條件套用至本期及上期：

- 差額＝本期淨銷售額－上期淨銷售額。
- 變化率＝差額 ÷ 上期淨銷售額絕對值；零基準不顯示百分比。
- 退款重點使用 `abs(sum(returnAmount))`，不是退款異常、風險或因果判斷。
- 本期必須明細完整、門店覆蓋可確認，才提供本期重點。跨期比較還要求上期完整及完全相同的門店集合。
- 只有完整且可比期間內真正沒有該商品時，才以零計算；缺資料、失敗門店、非有限數值或溢位不會被推論為零。
- 每項顯示比較金額／基準；「計算依據與範圍」揭露期間、篩選及不可比較原因。
- 點商品可開啟現有商品詳情。詳情使用完整報表範圍、不沿用分類篩選，此差異有明確提示。
- 概覽 Excel 增加「分析重點」證據工作表，數值保留數字型別。

## 可靠性與大量資料

- 修復文章結果與背景 Trend 結果共用可變切片的競態，將完成後資料分開發布。
- 背景補充與失敗更新先複製會改動的期間／問題清單，避免改寫已回傳、正在被序列化的報表快照。
- 新增趨勢附加獨立性、快照切片所有權及背景補充期間並行 JSON 序列化測試。
- 商品詳情每個期間建立門店覆蓋及商品彙總索引，避免每間店反覆掃描全部商品。
- 合成資料回歸包含二十萬筆排序／洞察，以及十萬筆、兩百間門店的商品詳情。這些是本機正確性與規模測試，不代表實際 RTA 查詢耗時。

## 最終驗證

- `bun run verify`：**248 項測試／31 個檔案**，emoji lint 通過、Svelte **0 errors／0 warnings**，正式前端資源建置成功。
- `go test ./... -count=1 -timeout 180s`：全套通過，未使用 `live` 標籤。
- `go test -race ./desktop -count=1 -timeout 180s`：通過；修復後相關競態測試另連續十輪通過。
- `analysis-tools-smoke.mjs`、`analysis-presets-smoke.mjs`、`analysis-ui-smoke.mjs`：桌面 1440、筆電 1024、窄螢幕 390、深色，共 **12 個情境通過**。
- 工具瀏覽器驗證增加洞察追查及焦點復原；原有排序、73 筆全分頁匯出、前導零、商品跨期明細、常用條件不自動查詢等斷言仍保留。
- 真正 Windows WebView2／Wails 橋接驗證通過：查詢合成資料、50 筆分頁、數值排序、128 筆原生 Excel、商品詳情切換與匯出、Escape 焦點恢復、含洞察證據的概覽 Excel。實際寫出的 XLSX 再由 Go／Excelize 開啟檢查。
- `git diff --check` 通過；建置後臨時 `.syso` 已移除，自有 Vite 與原生測試實例已關閉。

### 原生隔離方式與重現

`desktop/native_smoke_windows.go` 只在 **`windows && native_smoke`** 下編譯，正式建置不包含此測試入口。它使用合成 client、記憶體憑證與 cookie、新的資料目錄，不呼叫 `NewNativeApp()` 或 RTA。測試入口 `desktop/frontend/scripts/native-smoke.go` 不使用正式單一實例識別碼，並指定獨立 WebView 資料目錄及 loopback CDP 埠。

先完成 `bun run verify`，再由專案根目錄編譯：

```powershell
go build -tags=production,native_smoke -trimpath -o desktop/frontend/node_modules/.cache/rta-native-smoke.exe desktop/frontend/scripts/native-smoke.go
go build -o desktop/frontend/node_modules/.cache/workbook-fixture.exe desktop/frontend/scripts/workbook-fixture.go
```

以未被占用的埠及全新目錄啟動測試 EXE：`-root <fresh-directory> -port <port>`。建立包含 `{ "root": "...", "pid": 123, "port": 9327 }` 的 JSON 記錄；WebView 啟動後在前端目錄執行 `node scripts/analysis-native-smoke.mjs`，以 `NATIVE_RUN` 指向記錄、`PLAYWRIGHT_MODULE` 指向本機 Playwright Core 模組。只關閉該測試 PID；不要關閉正式程式。

原生測試的目錄選擇服務**自動回傳沙箱匯出目錄**，因此不宣稱驗證過手動原生目錄選擇器。正式已簽章預覽版未另行以真實帳號啟動；原生證據來自相同前後端程式碼與正式前端資源的隔離測試建置。未執行真實 RTA 速度量測，也未新增高深度 PDF 視覺驗證。

## 畫面與紀錄

- `release/analysis-refined-native-overview.png`
- `release/analysis-refined-native-product-detail.png`
- `release/analysis-refined-narrow-product-detail.png`
- `release/analysis-refined-narrow-insights.png`
- `release/analysis-refined-dark-product-detail.png`

本機詳細紀錄位於前端 `node_modules/.cache/refine-*`，原生證據目錄為 `node_modules/.cache/native-refine-20260905-220740/`。此暫存目錄不是 Git 交付內容。
