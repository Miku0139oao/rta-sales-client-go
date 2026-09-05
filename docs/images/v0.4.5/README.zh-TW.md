# 0.4.5 功能截圖

這四張畫面來自 v0.4.5 所包含分析程式碼的隔離驗證；全數使用合成資料，沒有真實帳號、憑證或營業紀錄。此輪發佈只更新版本資源與文件，分析程式碼未變。

- `overview.png`：Windows 原生 WebView2／Wails 橋接的銷售概覽，含排行深度、常用條件、分析重點及 Excel 匯出。
- `contributions.png`：同一次原生驗證的計算依據與門店／分類差額拆解。兩個維度各自核對 HK$21,440，不可相加。
- `product-detail-narrow.png`：390px 瀏覽器合成資料驗證，展示商品跨期間詳情及 Excel 匯出；非原生手機 App。
- `pdf-top100.png`：實際產出的 Top 100 PDF 第 8／30 頁，展示續頁最後順位、下一分類及不完整資料提示；非手繪版型。

驗證方法與限制見 `docs/analysis-optimization.md`。這不是對真實 RTA 服務的速度或連線保證。
