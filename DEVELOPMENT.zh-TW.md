# 開發說明

[English](DEVELOPMENT.md) | 繁體中文

一般使用請看 [README.zh-TW.md](README.zh-TW.md)。這份文件給要改程式、跑命令列、或把套件嵌進別的 Go 專案的人。

Windows 桌面程式的產品名稱、捷徑、開始功能表名稱都是 **RTA 銷售分析**；英文介面稱為 **RTA Sales Analyzer**。執行檔還是 `RTA-Excel-Filler.exe`，舊檔名沒改，是為了少動安裝與 CI。視窗標題是 **RTA 銷售分析**。

同一個 repo 裡還有：

- 命令列：`go run ./cmd/rta-xlsx-fill`，不開視窗，做同一件填 Excel 的事
- Go library：給別的程式呼叫

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

常用的還有：`-sheet`（預設 `Dairly`）、`-overwrite`、`-allow-partial`、`-max-jobs`（預設 2000）、`-concurrency`（預設 160）、`-mapping`、`-timeout 20m`。`-row` 只拿來查某一列，不能跟 `-write` 一起。

螢幕上會印 JSON。`matched_rows` 是符合日期的列，`selected_rows` 是這本帳號有權的，`skipped_store_rows` 是別人的店。一個授權門店都對不上就會失敗，不會默默存一份沒改過的檔。報告裡沒有帳密和銷售數字。

核對安裝檔雜湊：

```powershell
(Get-FileHash -Algorithm SHA256 .\RTA-Excel-Filler-portable.exe).Hash.ToLowerInvariant()
Get-Content .\SHA256SUMS.txt
```

## 在自己的 Go 程式裡怎麼叫

Go 1.25 以上：

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

帳密用環境變數。一個 `Client` 對應一本帳號，第一次查詢時才會登入，cookie 過期會自己再登。

```go
import rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"

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
	Concurrency:             160,
})
report, err := xlsxfill.Apply(ctx, plan, xlsxfill.ApplyRequest{
	OutputPath: `C:\reports\august.filled.xlsx`,
})
```

`Config` 裡比較常動的是 `PageConcurrency`（預設 16）跟 `LoginAttempts`（預設 4，最多 10）。`CookieStore` 跟 `CookieFile` 只能選一個。不同帳號請用不同 Client、不同 cookie 路徑。

驗證碼預設用內建 OCR，一般 CPU 就好，不用裝 Tesseract。看不懂的圖它不會硬送，會換一張或交給你串的下一個 solver（例如 `NewTwoCaptchaSolver`）。錯誤用 `errors.As` 看 `AuthError`、`CaptchaError`、`UpstreamError` 那些。任何一分頁失敗，整次查詢就失敗。

內建 OCR 是模板比對。要再訓練：

```
go run ./cmd/rta-ocr-train capture -dir samples -count 60
go run ./cmd/rta-ocr-train propose -dir samples
# 檔名就是答案，看圖改錯的；看不懂的 unnamed-*.bin 也要手標
go run ./cmd/rta-ocr-train gen -dir samples
go run ./cmd/rta-ocr-train eval -dir samples
go test ./rtasales/
```

`gen` 會覆寫 `rtasales/embedded_ocr_trained.go`。樣本目錄不要提交。

## 開發環境

改桌面版請用 Windows。library 測試在 Linux CI 也能跑。

要裝的：

- Go 1.25 或更新（CI 使用 1.25.13，govulncheck 才不會掃到已修的標準庫漏洞）
- [Bun 1.3.14](https://bun.sh)（frontend 指定 Bun，別改用 npm）
- Git、PowerShell
- 本機要有 WebView2（Windows 10/11 通常已隨 Edge 安裝），不然視窗開不起來
- Microsoft Edge WebView2 Runtime（缺少時手動安裝）；不需要 NSIS。

Wails 釘在 `v3.0.0-beta.8`。不必先手動裝，腳本找不到就會 `go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8`。想先裝也可以：

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
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

只建置 Windows 免安裝版：

```powershell
./scripts/build-desktop.ps1
```

完成後看 `release\`。腳本編譯 frontend，將 `wails.json` productVersion 注入 `internal/buildinfo.Version`、核對 Windows 資源版本，建置並在簽章工具可用時簽署免安裝檔，最後只對本次輸出計算 SHA256。正式簽章建置使用 `-RequireSign`；一般 `go build` 版本為 `dev`，無法更新。CI 未簽章檔只暫存為草稿。正式發佈須在乾淨 checkout 明確執行 `scripts/publish-portable.ps1 -Tag vX.Y.Z -Commit <完整-sha> -PublisherReference <先前可信免安裝檔>`；它會拒絕修改已公開版本，並重新下載草稿驗證後才發佈。詳見[更新安全與簽章沙箱驗證](docs/portable-updates.md)。WebView2 需手動安裝。

產品名稱跟版本改 `cmd/rta-excel-filler/wails.json` 跟 `cmd/rta-excel-filler/build/windows/info.json`。

CI 在 Ubuntu 跑測試跟 frontend，在 Windows 再跑一次 Windows 專用測試並呼叫同一個 `build-desktop.ps1`。本機 verify 不含打包跟漏洞掃描。

## 別提交出去的東西

`.env`、cookie、填過正式數字的對照、`*.filled.xlsx`、`cmd/rta-excel-filler/build/bin/`。Wails 產生的 `desktop/frontend/src/lib/wails/` 跟 `desktop/frontend/bindings/` 也被忽略。

桌面版帳密不要改成明文檔。log 裡不要出現 cookie、密碼、完整上游回應或 `SaleItem.Raw`。
