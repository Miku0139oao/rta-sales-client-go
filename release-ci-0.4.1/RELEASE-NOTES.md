# RTA 銷售分析 0.4.1

RTA 銷售分析 0.4.1 是一套 Windows 桌面應用程式，用於檢視門店銷售、匯出 PDF 報告，以及將數字填入公司既有的 Excel 活頁簿。

如需安裝方式、帳號設定、銷售分析、PDF 匯出與 Excel 填入的完整說明，請參閱：

- **[繁體中文使用說明](https://github.com/Miku0139oao/rta-sales-client-go/blob/v0.4.1/README.zh-TW.md)**
- **English:** [README.md](https://github.com/Miku0139oao/rta-sales-client-go/blob/v0.4.1/README.md)
- **[使用教學](https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.4.1/RTA-Sales-Analyzer-tutorial.mp4)**（約 45 秒，附中文字幕，無需開啟聲音）

## 下載哪個檔案

| 檔案 | 誰該用 |
| --- | --- |
| `RTA-Excel-Filler-setup.exe` | **一般使用者請選擇此檔案。** 雙擊安裝後，從「開始」功能表搜尋「RTA 銷售分析」即可開啟。 |
| `RTA-Excel-Filler-portable.exe` | 免安裝版本，雙擊即可執行。Windows 10/11 通常已隨 Microsoft Edge 提供 WebView2；若無法啟動，請安裝 [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/) 或改用安裝版。 |
| `SHA256SUMS.txt` | 供資訊人員核對檔案完整性，一般使用可略過。 |

## 本版更新內容

- **群組報告整合為單一檔案**：每間門店與總報告各輸出一份 PDF；全部商品報告置於前段，所選商品群組依序附加於同一份檔案。
- **匯出介面重整**：介面區分為報告範圍、附加群組章節與分類條件；內容區可捲動，底部操作列固定，避免互相擠壓。
- **深色模式修正**：商品範圍下拉選單在 Windows WebView2 深色模式下可正常閱讀，並改善窄視窗排列。
- **WebView2 說明更準確**：Windows 10/11 通常已隨 Microsoft Edge 安裝 Runtime；僅少數缺少 Runtime 的電腦需要由安裝版協助下載。

## 升級注意事項

- 既有帳號、設定與 Item Code 群組將予以保留，無需重新建立。
- 安裝檔檔名仍為 `RTA-Excel-Filler-*.exe`（舊檔名），程式本身名稱為 **RTA 銷售分析**。
- 同一時間僅能開啟一個視窗。
