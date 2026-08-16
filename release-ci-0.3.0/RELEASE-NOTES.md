# RTA 銷售分析 0.3.0

用來看門店、開會出 PDF、把數字填進公司 Excel 的 Windows 桌面版。

怎麼安裝、加帳號、看銷售、匯出 PDF、另存 Excel，請看：

**[使用說明（繁體中文）](https://github.com/Miku0139oao/rta-sales-client-go/blob/v0.3.0/README.zh-TW.md)**  
**[使用教學](https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.3.0/RTA-Sales-Analyzer-tutorial.mp4)**（約 45 秒，有字幕、不用開聲音）  
English: [README.md](https://github.com/Miku0139oao/rta-sales-client-go/blob/v0.3.0/README.md)

## 下載哪個檔

| 檔案 | 誰該用 |
| --- | --- |
| `RTA-Excel-Filler-setup.exe` | **一般請用這個。** 雙擊安裝，開始選單搜尋「RTA 銷售分析」 |
| `RTA-Excel-Filler-portable.exe` | 不想安裝、直接雙擊。電腦必須已有 WebView2 |
| `SHA256SUMS.txt` | 資訊人員核對檔案，一般可略過 |

## 這版新的

- **分析更快**：先查出本期各店商品明細，概覽可以先看；全店趨勢與上期／去年在背景補。
- **趨勢一次查全店**：RTA 本來就是全店加總，不再一店打一次 Trend。
- **獨立店報仍一店一查**：商品明細不會混店。
- **匯出篩選**：可單獨勾總報告或指定門店；可忽略贈品／印花與指定分類。
- **查詢並行預設 160**，給單帳號一次查多店用。舊安裝會從 32 升上來。

## 這版會用到的操作

- **帳號**：新增 → 測試並啟用。多本帳號用上移／下移排優先順序。
- **銷售分析**：選帳號與門店 → 開始分析 → 本期先出，其餘期間自動補上。
- **匯出 PDF**：按「匯出篩選」，選總報告／分店與分類後再選資料夾。
- **Excel 填入**：開啟活頁簿 → 分析 → 另存並寫入。來源檔不會被改寫。

## 已知事項

- 安裝檔檔名仍是 `RTA-Excel-Filler-*.exe`（舊檔名），程式本身叫 **RTA 銷售分析**。
- 同時只能開一個視窗。
