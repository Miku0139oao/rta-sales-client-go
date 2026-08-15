# rta-sales-client-go

[English](README.md) | 繁體中文

`rta-sales-client-go` 是可供其他專案引用的 RTA 合作夥伴銷售查詢 Go library，提供：

- 自動 SSO 登入、Cookie 保存與 session 過期恢復；
- 純 CPU 的內建驗證碼 OCR，以及可選的備援 solver；
- 自動載入登入帳號獲授權的門店；
- 以業務門店編號精確查詢指定日曆日期區間；
- 完整分頁、型別化明細、原始欄位、報表總計與彙總；
- 安全填入既有 `.xlsx` 的每日人工輸入欄位。

client 會直接向 RTA 取得該帳號專屬的門店關係。呼叫端只需使用 `Stores` 回傳的業務門店編號；RTA 查詢所需的內部值只保留在 client 內部，不會成為公開設定。

## 環境需求與安裝

- Go 1.25 或更新版本。
- 可存取目標門店的 RTA 帳號。
- 選用：需要遠端 OCR 備援時，可準備 2Captcha API key。

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

## Windows 桌面版

RTA Excel Filler 是免用命令列的 Windows 桌面程式，可處理單日或包含起訖日的日期範圍。CI 會在 `RTA-Excel-Filler-windows-amd64` artifact 產生三個檔案：

- `RTA-Excel-Filler-setup.exe`：目前使用者範圍的 NSIS 安裝程式；缺少 WebView2 時會下載安裝；
- `RTA-Excel-Filler-portable.exe`：可攜版；電腦必須已安裝 Microsoft Edge WebView2 Runtime；
- `SHA256SUMS.txt`：上述兩個執行檔的 SHA-256。

解除安裝只會移除程式與捷徑，會刻意保留目前使用者的設定檔、加密 Cookie 狀態與 Windows Credential Manager 項目，供日後重新安裝沿用。若也要清除已儲存的 RTA 帳號資料，請在解除安裝前先於程式內逐一刪除設定檔。

執行前可先核對下載檔案：

```powershell
(Get-FileHash -Algorithm SHA256 .\RTA-Excel-Filler-setup.exe).Hash.ToLowerInvariant()
Get-Content .\SHA256SUMS.txt
```

### 帳號與私密資料

進入「帳號」，填入顯示名稱、RTA 帳號與密碼，先執行「測試」再啟用。帳號設定檔的排列順序也是門店歸屬優先順序；兩個帳號都可存取同一門店時，由第一個已啟用的設定檔負責。同一 RTA 帳號的查詢永遠串行；不同帳號依「設定」的併發上限執行（預設 `2`，最高 `4`）。

密碼保存在 Windows Credential Manager。每個設定檔的 Cookie 都會先用 Windows DPAPI 加密，再存到目前使用者的應用程式資料目錄；`profiles.json` 只含顯示用 metadata。活頁簿預覽、銷售數值、門店路由與分析 plan 只存在程式記憶體，不會寫入設定或 log。刪除設定檔時，也會移除其帳密與加密 Cookie。

### 多日活頁簿流程

1. 開啟 `.xlsx`，選擇工作表與包含起訖日的日期範圍。
2. 檢查掃描摘要；預設安全上限是 `2,000` 個不重複的日期／門店 jobs。
3. 執行「分析」。每個日期／門店組合只查一次，而且每筆 RTA 請求都只涵蓋一個日曆日。
4. 檢查預覽：`L` 欄是 Article View 當日銷售額，`AB` 欄是 Trend View 當日交易次數。
5. 若在活頁簿 plan 建立後取消，或暫時性 job 在內建重試後仍失敗，可執行「重試失敗／未排程項目」。若在登入或載入授權門店時取消，因尚未建立可重試的 plan，必須重新執行「分析」。取消後的不完整 plan 絕不可寫入。
6. 另存新活頁簿。嚴格模式要求 plan 已完整結束且沒有問題。只有分析完整結束後才能選擇部分輸出；所有問題列會整列維持原值，且必須明確確認。

程式絕不覆寫來源檔。套用前會重新核對來源的 SHA-256、大小與修改時間；檔案若已改變就拒絕寫入。現有值不同時必須啟用「覆寫現有值」，`L` 或 `AB` 的公式則永遠不會被取代。

若活頁簿 `C` 欄不是 RTA 業務門店編號，可在「設定」啟用私有 JSON／CSV 對照檔。含正式資料的對照檔請保持在版本控制之外。

帳密應由環境變數或秘密管理服務提供：

```dotenv
RTA_ACCOUNT=
RTA_PASSWORD=
RTA_COOKIE_FILE=
TWOCAPTCHA_API_KEY=
```

`Config` 不再需要任何門店 filter ID；登入後會自動載入授權門店。

## Library 快速開始

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

func main() {
	client, err := rtasales.NewClient(rtasales.Config{
		Account:        os.Getenv("RTA_ACCOUNT"),
		Password:       os.Getenv("RTA_PASSWORD"),
		CookieFile:     "state/rta.cookies.json",
		CaptchaSolvers: []rtasales.CaptchaSolver{
			rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	stores, err := client.Stores(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if len(stores) == 0 {
		log.Fatal("此帳號沒有可用的授權門店")
	}

	// 正式程式應按業務需求選擇完全相同的 BusinessID。
	target := stores[0]
	day := time.Date(2026, 8, 13, 0, 0, 0, 0, time.Local)
	result, err := client.Sales(ctx, rtasales.SalesQuery{
		BusinessStoreID: target.BusinessID,
		StartDate:       day,
		EndDate:         day,
	})
	if err != nil {
		log.Fatal(err)
	}

	transactions := 0.0
	if result.TotalTransactionCount != nil {
		transactions = *result.TotalTransactionCount
	}
	fmt.Printf("store=%s amount=%.2f transactions=%.0f rows=%d\n",
		result.Store.Label,
		result.TotalAmount,
		transactions,
		len(result.Items),
	)
}
```

正式環境請以完整 `BusinessID` 精確選店，不要用名稱、前綴或模糊比對。`StartDate` 與 `EndDate` 都包含在查詢範圍內；套件會直接使用 `time.Time` 顯示的年月日，不會先轉換時區。

### 授權門店

`Stores` 會在需要時自動登入、讀取帳號的授權門店，然後快取結果：

```go
stores, err := client.Stores(ctx)
for _, store := range stores {
	fmt.Printf("%s\t%s\n", store.BusinessID, store.Label)
}
```

權限可能變更時，可強制重新載入：

```go
stores, err := client.RefreshStores(ctx)
```

RTA 會以 `data` array 回傳授權門店。套件逐筆處理：`key` 只保留作 Article View 私有 filter；`value` 只切第一個 `-`，左邊作 `BusinessID` 並供 Trend View 使用，完整字串作 `Label`。門店代碼全程保留為字串，因此前導 `0` 不會遺失。程式只公開 `BusinessID` 與 `Label`，不會把門店對照寫入磁碟。

### SalesQuery 欄位

| 欄位 | 說明 |
| --- | --- |
| `BusinessStoreID` | 必填，必須與 `Stores` 回傳的 ID 完全相同 |
| `StartDate` | 必填，包含在查詢範圍內的開始日期 |
| `EndDate` | 必填，包含在查詢範圍內，且不可早於 `StartDate` |
| `Category` | 選用的呼叫端結果標籤，本身不會篩選 RTA 資料 |
| `ItemCodes` | 選用的 SKU／ManCode 篩選；空值查詢全部商品 |

送出前會移除空白與重複的 `ItemCodes`。`TotalTransactionCount` 直接讀取 RTA Trend View 同門店、同日期的 `group_sales_ticket_num`，日期範圍超過一天時會加總範圍內的每日列；不會由 Article View 商品明細推算。

## 填入既有 Excel 活頁簿

`xlsxfill` package 與 `cmd/rta-xlsx-fill` 可填入兩個每日人工欄位，同時保留公式、格式、合併儲存格與其他工作表。預設工作表為 `Dairly`：

- `C` 欄：業務門店編號；
- `F` 欄：日曆日期；
- `L` 欄：當日銷售額；
- `AB` 欄：相同日期的 Trend View 交易次數。

指令會先登入並取得此帳號的 `data[]` 授權門店，再自動比對指定日期的所有列。程式以每筆 `value` 第一個 `-` 前的字串與 `C` 欄精確比對；私有 `key` 只供 Article View 使用，Trend View 使用該字串前綴。屬於其他帳號的門店列會在查詢前略過。不需要手動提供列號、本機門店對照或額外門店環境變數。

先建立本機 `.env`，此檔案已被 Git 忽略：

```dotenv
RTA_ACCOUNT=你的帳號
RTA_PASSWORD=你的密碼
RTA_COOKIE_FILE=.rta-sales.cookies.json
```

一定先跑 dry-run。PowerShell：

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\來源.xlsx" `
  -date 2026-08-13
```

Bash：

```bash
go run ./cmd/rta-xlsx-fill \
  -input /path/source.xlsx \
  -date 2026-08-13
```

跨日處理須同時使用 `-from` 與 `-to`（包含起訖日），而且不可與 `-date` 並用：

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\來源.xlsx" `
  -from 2026-08-01 `
  -to 2026-08-31
```

確認 dry-run 成功後，再另存新檔：

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\來源.xlsx" `
  -output "C:\path\來源.filled.xlsx" `
  -date 2026-08-13 `
  -write
```

正常使用不要加入 `-row`；它只供診斷，而且不可與 `-write` 並用。`-max-jobs` 是自動選店後的安全上限（預設 `2,000`）；`-max-queries` 只保留作 deprecated alias。`-concurrency` 最高為 `4`，同帳號的 jobs 仍維持串行。預設沒有整體 timeout；需要時才明確加入例如 `-timeout 20m`。

JSON 報告會分開顯示比對階段：`matched_rows` 是符合日期的列數，`selected_rows` 是此帳號有權限的列數，`skipped_store_rows` 是屬於其他帳號而略過的列數。若該日期沒有任何授權門店相符，指令會明確失敗，不會靜默產生沒有變更的活頁簿。

安全預設：

- 預設只做 dry-run；
- 輸出檔不可覆寫來源活頁簿；
- 現有數值不同時，必須明確加入 `-overwrite` 才會取代；
- `L` 或 `AB` 若為公式儲存格，絕不覆寫；
- 其他帳號的門店會在送出銷售請求前略過；
- 完全沒有授權門店相符、缺少報表總數或查詢失敗時，預設不輸出；只有明確加入 `-allow-partial` 才可保存安全完成的列；
- JSON 報告只包含列號與問題代碼，不包含帳密、門店 ID 或實際銷售數值。

若其他活頁簿的 `C` 欄不是 RTA 的業務門店編號，可用 `-mapping` 指定本機私有 JSON 或 CSV。含資料的對照檔、Cookie、`.env` 與 `*.filled.xlsx` 都已被忽略，不可提交。

直接使用 `xlsxfill.Fill` 的 library 呼叫端，可將 `Client.Stores` 回傳的 ID 放入 `Request.AllowedBusinessStoreIDs`，取得相同的自動選列行為。

### 兩階段批次 API

新整合建議使用兩階段 API。`Analyze` 絕不修改活頁簿；`RetryFailed` 可接續暫時性失敗與取消後尚未執行的 jobs；`Apply` 只會寫入完整、且來源指紋仍相同的 plan：

```go
plan, err := xlsxfill.Analyze(ctx, provider, xlsxfill.BatchRequest{
	InputPath:               `C:\reports\august.xlsx`,
	From:                    from,
	To:                      to,
	AllowedBusinessStoreIDs: allowedStoreIDs,
	MaxJobs:                 2000,
	Concurrency:             2,
})
if errors.Is(err, context.Canceled) {
	plan, err = xlsxfill.RetryFailed(context.Background(), plan)
}
if err != nil {
	return err
}

report, err := xlsxfill.Apply(ctx, plan, xlsxfill.ApplyRequest{
	OutputPath: `C:\reports\august.filled.xlsx`,
})
```

傳輸錯誤與 HTTP 408／429／5xx 會在 `1s`、`3s` 後重試兩次。無資料、權限、門店對照與活頁簿格式問題不會重試。只有已完整結束但仍有問題的 plan 才能設定 `AllowPartial`；每一個問題列都會維持原樣。原有單日 `xlsxfill.Fill` 與 CLI `-date` 用法仍相容。

## Client 設定

| 欄位 | 用途 | 預設值 |
| --- | --- | --- |
| `Account` | RTA 登入帳號，必填 | 無 |
| `Password` | RTA 登入密碼，必填 | 無 |
| `CaptchaSolvers` | 依序嘗試的驗證碼 solver，至少需要一個 | 無 |
| `CookieFile` | Cookie jar 保存路徑 | 僅記憶體 |
| `CookieStore` | 可替換的 Cookie 保存介面，不可與 `CookieFile` 並用 | 無 |
| `HTTPClient` | 自訂 transport、proxy、timeout 或 cookie jar | timeout 30 秒 |
| `PageConcurrency` | 第一頁之後的最大並行查詢數 | `4` |
| `LoginAttempts` | 重新取得驗證碼並登入的次數，可設為 `1`–`10` | `4` |

`Client` 可安全地並行使用。不同帳號應使用不同 `Client` 與不同 Cookie 路徑。

## 驗證碼 OCR 與硬體需求

內建 solver 使用兩條獨立的擷取／分類路徑，只有兩者結果相同且通過信心門檻時才接受。信心不足的圖片不會送出；系統會取得新的 challenge，或交給下一個已設定的 solver。

它只使用一般 CPU 指令，不需要 GPU、CGO、Tesseract 執行檔、背景服務或外部模型檔。建議設定：

```go
solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
```

在一批獨立的 1,000 張 challenge 驗證中，raw top-1 辨識為 `989/1000`；預設門檻送出 `905/1000`，有限樣本中的 `905` 筆送出結果均獲接受。這不能證明單張圖片有 99.99% 準確率。四次新 challenge 提供的是重試層可靠度；被拒絕的圖片不會當作正確結果。

需要遠端備援的應用可在 solver 清單後加入 `NewTwoCaptchaSolver`。已部署 Tesseract 的應用可用 `TesseractSolver`；自訂 solver 只需實作：

```go
type CaptchaSolver interface {
	Solve(context.Context, []byte) (string, error)
}
```

## 回傳結果與錯誤

`SalesResult` 包含所選 `Store`、日期範圍、正規化商品編號、依分頁順序排列的所有明細、`TotalAmount`、Trend View 的 `TotalTransactionCount`、`GrossQuantity`、分類彙總與查詢時間。每個 `SaleItem` 的 `Raw` 亦保留完整上游列。

可使用 `errors.As` 判斷：

- `InputError`
- `AuthError`
- `StoreNotFoundError`
- `CaptchaError`
- `UpstreamError`
- `ProtocolError`

傳輸失敗與 HTTP 408、429、5xx 會令 `UpstreamError.Retryable()` 回傳 `true`。任一分頁失敗會令整次銷售查詢失敗，不會回傳部分結果。

## 安全與開發驗證

- 帳密與選用服務 key 應放在環境變數或秘密管理服務。
- 不可提交 `.env`、Cookie、含資料的本機對照或產生的活頁簿。
- 避免記錄 Cookie、帳密、完整上游回應、`SaleItem.Raw` 或選店內部值。
- 支援的平台會把持久化 Cookie 檔權限設為 `0600`。

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...

cd desktop/frontend
bun install --frozen-lockfile
bun run verify

cd ../../cmd/rta-excel-filler
wails build -platform windows/amd64
```

桌面版固定使用 Wails CLI `v2.14.0`、Bun 與 NSIS 3。可用 `go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0` 安裝固定版本 CLI。

repository 內的測試只使用合成圖片與本機 HTTP fixture，不會連線 RTA 或外部驗證碼服務，也不包含正式門店資料。
