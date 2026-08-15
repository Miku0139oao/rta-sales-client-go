# rta-sales-client-go

[English](README.md) | 繁體中文

Windows 上用的是桌面程式 **RTA 銷售分析**（安裝清單裡寫 RTA Sales Analyzer，現在是 `0.1.1`）。登入 RTA、認驗證碼、查門店銷售、匯出 PDF，也可以把數字填進公司那份既有的 Excel。

同一個 repo 裡還有：

- 命令列：`go run ./cmd/rta-xlsx-fill`，不開視窗，做同一件事
- Go library：給別的程式呼叫

執行檔還是叫 `RTA-Excel-Filler.exe`，這是舊檔名，沒改是為了少動安裝與 CI。畫面標題已經換成新名字。

## 下載後怎麼開

64-bit 的 Windows 10 或 11。CI 產物叫 `RTA-Excel-Filler-windows-amd64`，裡面三個檔：

- `RTA-Excel-Filler-setup.exe`：一般人裝這個。沒有 WebView2 它會幫你下載。
- `RTA-Excel-Filler-portable.exe`：免安裝。電腦上要先有 [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/)。
- `SHA256SUMS.txt`：核對用。

```powershell
(Get-FileHash -Algorithm SHA256 .\RTA-Excel-Filler-setup.exe).Hash.ToLowerInvariant()
Get-Content .\SHA256SUMS.txt
```

裝完從開始選單開「RTA Sales Analyzer」。可攜版直接雙擊 exe。同時只能開一個視窗。

解除安裝不會清帳號。設定、加密 Cookie、Windows 認證都還在 `%AppData%\RTA Excel Filler`（資料夾沿用舊名，免得舊資料找不到）。真的要清掉，先到程式裡把帳號一筆一筆刪除。

## 桌面版怎麼用

先加帳號，再去做「銷售分析」或「Excel 填入」。預設繁中，設定裡可改英文、改亮暗色。

![帳號](release/account-pool-desktop-verified.png)

### 帳號

進「帳號」→「新增帳號」，填顯示名稱、RTA 帳號、密碼。先按「測試」或「測試並啟用」，程式會真的登入，確認驗證碼跟權限沒問題。沒啟用的帳號，分析跟填 Excel 都不會用到。

清單由上到下是門店歸屬順序。兩本帳號都能進同一間店時，用上面那本。可以拖、也可以按上移下移。

密碼放在 Windows 認證管理員。Cookie 用 DPAPI 加密後再存。`profiles.json` 只有顯示名稱之類的東西。銷售數字、活頁簿預覽、分析 plan 都只在記憶體。刪帳號會連密碼和 Cookie 一起刪。

### 銷售分析

選一個已啟用、而且有多間店的帳號，勾門店，選「月份比較」或自己指定日期，按「開始分析」。並行數在設定裡，預設 32。

月份比較一次查五段：本期、上期、前期、去年同期、去年下月。若這個月還沒過完，前四段會切到跟今天同一天（二月這種短月就切到月底）；去年下月仍是整月。

查完可以切概覽、關注、分類、商品、門店比較。商品跟五層分類都跟 Article View 下載檔同一套口徑。全店交易次數跟客單價來自 Trend View，不是把商品列的交易次數加總。篩了分類或商品之後，這兩項會藏起來，因為 Trend View 沒有那麼細。

「匯出門店 PDF」選一個資料夾，成功查到的店各出一份橫向報告。檔名重複就自動加後綴，不會蓋掉舊檔。

### Excel 填入

公司活頁簿預設工作表叫 `Dairly`（中間那個拼法是原本檔案就這樣）：

- `C`：門店編號
- `F`：日期
- `L`：當天總銷售（銷售減退貨，Trend View）
- `AB`：當天交易次數（同一個 Trend View）

![選範圍](release/excel-range-desktop.png)

打開或把 `.xlsx` 拖進去，選工作表跟日期，看一下掃描摘要。預設最多 2000 個「日期 × 門店」，設定裡可改。按「開始分析」，每個組合只查一天。

預覽沒問題再「另存並寫入」，一定是新檔，來源不會被改。寫之前會再對一次檔案的雜湊、大小、修改時間，你中間若改過來源，它會拒絕。

`L` / `AB` 已經有不同數字時，要勾「允許覆寫全部不同值」才會改。格子裡是公式的話，一律不碰。分析做到一半取消、或暫時失敗，有 plan 才能按「重試失敗項目」；連登入都還沒跑完就取消，沒有 plan，只能重新分析。沒跑完的結果不能寫檔。

若一定要略過問題列、只寫其他列，等分析完整結束後再勾「略過全部問題列並寫入其他列」，它會再問一次。

`C` 欄若不是 RTA 門店編號，到設定打開本機 JSON／CSV 對照。那種檔不要提交進 Git。

![預覽](release/excel-results-desktop.png)

### 設定裡常改的

並行預設 32，最高也是 32。每次最多查 2000 個工作。對照檔可選。這些數字對分析和填 Excel 都有效。

## 命令列怎麼叫

根目錄放 `.env`（Git 會忽略）：

```dotenv
RTA_ACCOUNT=你的帳號
RTA_PASSWORD=你的密碼
RTA_COOKIE_FILE=.rta-sales.cookies.json
```

先不要加 `-write`，只看它打算改什麼：

```powershell
go run ./cmd/rta-xlsx-fill -input "C:\path\來源.xlsx" -date 2026-08-13
```

一整段日期用 `-from`、`-to`，不要跟 `-date` 一起用。確認沒問題再另存：

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\來源.xlsx" `
  -output "C:\path\來源.filled.xlsx" `
  -date 2026-08-13 `
  -write
```

常用的還有：`-sheet`（預設 `Dairly`）、`-overwrite`、`-allow-partial`、`-max-jobs`（預設 2000）、`-concurrency`（預設 32）、`-mapping`、`-timeout 20m`。`-row` 只拿來查某一列，不能跟 `-write` 一起。

螢幕上會印 JSON。`matched_rows` 是符合日期的列，`selected_rows` 是這本帳號有權的，`skipped_store_rows` 是別人的店。一個授權門店都對不上就會失敗，不會默默存一份沒改過的檔。報告裡沒有帳密和銷售數字。

## 在自己的 Go 程式裡怎麼叫

Go 1.25 以上：

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

帳密用環境變數。一個 `Client` 對應一本帳號，第一次查詢時才會登入，cookie 過期會自己再登。

```go
client, err := rtasales.NewClient(rtasales.Config{
	Account:    os.Getenv("RTA_ACCOUNT"),
	Password:   os.Getenv("RTA_PASSWORD"),
	CookieFile: "state/rta.cookies.json",
	CaptchaSolvers: []rtasales.CaptchaSolver{
		rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{}),
	},
})
stores, err := client.Stores(ctx)
result, err := client.Sales(ctx, rtasales.SalesQuery{
	BusinessStoreID: stores[0].BusinessID,
	StartDate:       day,
	EndDate:         day,
})
```

店一定要用 `Stores` 回傳的完整 `BusinessID`，不要拿店名去對。日期用畫面上的年月日，套件不會先轉時區。權限改了就 `RefreshStores`。

`SalesQuery` 還能帶 `ItemCodes`（SKU／ManCode）或 `SkipTrend`（只要商品明細、不要再打一槍全店 Trend View）。`TotalAmount` 是 Article View，可以跟商品篩選走；`TrendGrossSaleAmount` 跟 `TotalTransactionCount` 是 Trend View 全店數字，跨日會加總。

填 Excel 用兩段：先 `xlsxfill.Analyze`（不改檔），必要時 `RetryFailed`，最後 `Apply` 寫到另一個路徑。來源中間被改過，Apply 會拒絕。暫時性的網路／408／429／5xx 會隔 1 秒、3 秒再試；沒資料、沒權限、格式不對不會重試。

```go
plan, err := xlsxfill.Analyze(ctx, client, xlsxfill.BatchRequest{
	InputPath:               `C:\reports\august.xlsx`,
	From:                    from,
	To:                      to,
	AllowedBusinessStoreIDs: allowedStoreIDs,
	MaxJobs:                 2000,
	Concurrency:             32,
})
report, err := xlsxfill.Apply(ctx, plan, xlsxfill.ApplyRequest{
	OutputPath: `C:\reports\august.filled.xlsx`,
})
```

`Config` 裡比較常動的是 `PageConcurrency`（預設 4）跟 `LoginAttempts`（預設 4，最多 10）。`CookieStore` 跟 `CookieFile` 只能選一個。不同帳號請用不同 Client、不同 cookie 路徑。

驗證碼預設用內建 OCR，一般 CPU 就好，不用裝 Tesseract。看不懂的圖它不會硬送，會換一張或交給你串的下一個 solver（例如 `NewTwoCaptchaSolver`）。錯誤用 `errors.As` 看 `AuthError`、`CaptchaError`、`UpstreamError` 那些。任何一分頁失敗，整次查詢就失敗。

## 開發環境

改桌面版請用 Windows。library 測試在 Linux CI 也能跑。

要裝的：

- Go 1.25 或更新（CI 會測 1.25.12 跟 1.26.6）
- [Bun 1.3.14](https://bun.sh)（frontend 指定 Bun，別改用 npm）
- Git、PowerShell
- 本機要有 WebView2，不然視窗開不起來
- 只有要做安裝檔時才需要 NSIS 3.12：`choco install nsis --version 3.12.0`

Wails 釘在 `v2.14.0`。不必先手動裝，腳本找不到就會 `go run github.com/wailsapp/wails/v2/cmd/wails@v2.14.0`。想先裝也可以：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0
```

不需要 GPU、CGO、Tesseract。測試都是假圖跟本機 HTTP，不會真的打 RTA。

目錄大概是：根目錄是 library；`cmd/rta-excel-filler` 是桌面進入點；`desktop/frontend` 是 Svelte；`cmd/rta-xlsx-fill` 是命令列；`scripts` 是本機腳本。

## 從原始碼編出來

先 clone，在 repo 根目錄做事。

```powershell
git clone https://github.com/Miku0139oao/rta-sales-client-go.git
cd rta-sales-client-go

cd desktop\frontend
bun install --frozen-lockfile
cd ..\..
```

編譯前跑一次檢查：

```powershell
./scripts/verify.ps1
```

會跑 Go 測試（含 race）、vet、build，再跑 frontend 的檢查、測試跟 `vite build`。改一點小東西、不想等 race：

```powershell
./scripts/verify.ps1 -SkipRace
```

只編命令列：

```powershell
go build -o rta-xlsx-fill.exe ./cmd/rta-xlsx-fill
.\rta-xlsx-fill.exe -input "C:\path\來源.xlsx" -date 2026-08-13
```

或直接 `go run ./cmd/rta-xlsx-fill ...`。

開發桌面版（改畫面會熱重載，改 Go 綁定會重開視窗）：

```powershell
./scripts/dev.ps1
```

要給別人用的 exe。只出可攜版、沒裝 NSIS 時：

```powershell
./scripts/build-desktop.ps1 -SkipInstaller
```

可攜版加安裝檔：

```powershell
./scripts/build-desktop.ps1
```

完成後看 `release\`。腳本會先編 frontend（每次清掉舊的 dist，免得舊 js 被打進 exe），再用釘住的 Wails 編兩次：可攜版遇到沒有 WebView2 只提示；安裝檔會下載 Runtime。

產品名稱跟版本改 `cmd/rta-excel-filler/wails.json`。不要只改 `build/windows/info.json`，下次 build 會被蓋掉。

CI 在 Ubuntu 跑測試跟 frontend，在 Windows 再跑一次 Windows 專用測試並呼叫同一個 `build-desktop.ps1`。本機 verify 不含打包跟漏洞掃描。

## 別提交出去的東西

`.env`、cookie、填過正式數字的對照、`*.filled.xlsx`、`cmd/rta-excel-filler/build/bin/`。Wails 產生的 `desktop/frontend/src/lib/wails/` 也被忽略。

桌面版帳密不要改成明文檔。log 裡不要出現 cookie、密碼、完整上游回應或 `SaleItem.Raw`。
