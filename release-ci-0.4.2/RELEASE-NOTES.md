# RTA 銷售分析 0.4.2

RTA 銷售分析 0.4.2 是一套 Windows 桌面應用程式，用於檢視門店銷售、匯出 PDF 報告，以及將數字填入公司既有的 Excel 活頁簿。

本版著重改善 Windows 主視窗與 PDF 匯出對話框的縮放體驗，使介面在不同 DPI 與視窗尺寸下皆能一致縮放。

如需安裝方式、帳號設定、銷售分析、PDF 匯出與 Excel 填入的完整說明，請參閱：

- **[繁體中文使用說明](https://github.com/Miku0139oao/rta-sales-client-go/blob/v0.4.2/README.zh-TW.md)**
- **English:** [README.md](https://github.com/Miku0139oao/rta-sales-client-go/blob/v0.4.2/README.md)
- **[使用教學](https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.4.2/RTA-Sales-Analyzer-tutorial.mp4)**（約 45 秒，附中文字幕，無需開啟聲音）

## 下載哪個檔案

| 檔案 | 誰該用 |
| --- | --- |
| `RTA-Excel-Filler-setup.exe` | **一般使用者請選擇此檔案。** 雙擊安裝後，從「開始」功能表搜尋「RTA 銷售分析」即可開啟。 |
| `RTA-Excel-Filler-portable.exe` | 免安裝版本，雙擊即可執行。Windows 10/11 通常已隨 Microsoft Edge 提供 WebView2；若無法啟動，請安裝 [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/) 或改用安裝版。 |
| `SHA256SUMS.txt` | 供資訊人員核對檔案完整性，一般使用可略過。 |

## 本版更新內容

- PDF 匯出對話框會隨主視窗大小與 Windows DPI 比例連續縮放。
- 對話框高度依內容調整並限制於可視範圍，避免佔滿視窗。
- 間距、字體、清單與按鈕會一致縮放。
- 標題列與操作按鈕保持可見，並改善巢狀清單的捲動行為。
- 不變更 PDF 產生數量、報告內容或匯出流程。

## 升級注意事項

- 既有帳號、設定與 Item Code 群組將予以保留，無需重新建立。
- 安裝檔檔名仍為 `RTA-Excel-Filler-*.exe`（舊檔名），程式本身名稱為 **RTA 銷售分析**。
- 同一時間僅能開啟一個視窗。

完整性驗證請參考隨附的 `SHA256SUMS.txt`。
