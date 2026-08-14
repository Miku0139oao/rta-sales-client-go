# rta-sales-client-go

[English](README.md) | 繁體中文

`rta-sales-client-go` 是獨立的 RTA 合作夥伴銷售查詢 Go library，提供：

- 自動 SSO 登入、Cookie 保存與登入逾期自動恢復；
- 純 Go、CPU-only 的內建驗證碼 OCR，以及依序執行的備援 solver；
- 從呼叫端的私有對照取得精確的帳號／門店綁定；
- 使用綁定帳號的 session 查詢指定日期區間銷售資料；
- 可選的 SKU／ManCode 篩選；
- 完整型別化明細、原始欄位、總計與分類彙總；
- 可按指定日期安全填入既有 Excel，同時保留公式與樣式；
- 有上限的並行分頁查詢，任一頁失敗時不回傳不完整結果。

本套件不依賴資料庫或全域程序狀態，也不需要 GPU、CGO、Tesseract 或外部 OCR 模型。每一組 RTA 帳號與業務門店綁定都應建立各自的 `Client`，避免 Cookie 與門店範圍互相混用。

這份 RTA 報表的門店範圍由登入帳號的 session 決定。RTA 的門店樹是全域目錄，不是該帳號的授權清單，其 ID 也不是這份報表可用的 `store_id` filter。client 會保留上游預期的 filter 欄位但送空值，`BusinessStoreID` 只用於本機端的嚴格路由檢查。呼叫端必須從自己的私有對照載入正確的帳號／門店組合。

## 環境需求與安裝

- Go 1.25 或更新版本。
- 可存取目標門店的 RTA 帳號。
- 選用：需要遠端備援辨識時，準備 2Captcha API key。

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

請透過環境變數或秘密管理服務提供設定：

```dotenv
RTA_ACCOUNT=
RTA_PASSWORD=
RTA_BUSINESS_STORE_ID=
RTA_COOKIE_FILE=
TWOCAPTCHA_API_KEY=
```

`TWOCAPTCHA_API_KEY` 為選用；未設定時，下方範例只使用內建 OCR。

## 快速開始

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

func main() {
	solvers := []rtasales.CaptchaSolver{
		rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{}),
	}
	if key := strings.TrimSpace(os.Getenv("TWOCAPTCHA_API_KEY")); key != "" {
		solvers = append(solvers, rtasales.NewTwoCaptchaSolver(key))
	}

	client, err := rtasales.NewClient(rtasales.Config{
		Account:         os.Getenv("RTA_ACCOUNT"),
		Password:        os.Getenv("RTA_PASSWORD"),
		BusinessStoreID: os.Getenv("RTA_BUSINESS_STORE_ID"),
		CookieFile:      "state/rta.cookies.json",
		CaptchaSolvers:  solvers,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.Sales(ctx, rtasales.SalesQuery{
		// 必須與 NewClient 使用的私有綁定 ID 完全相同。
		BusinessStoreID: os.Getenv("RTA_BUSINESS_STORE_ID"),
		StartDate:       time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local),
		EndDate:         time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("amount=%.2f gross-qty=%.2f rows=%d\n",
		result.TotalAmount,
		result.GrossQuantity,
		len(result.Items),
	)
}
```

`StartDate` 與 `EndDate` 都包含在查詢範圍內。套件會直接使用 `time.Time` 顯示的年月日，不會先轉換時區。

## 常用操作

`SalesQuery` 欄位：

| 欄位 | 說明 |
| --- | --- |
| `BusinessStoreID` | 必填，必須與呼叫端私有對照綁定到此 `Client` 的 ID 完全相同 |
| `StartDate` | 必填，包含在查詢範圍內的開始日期 |
| `EndDate` | 必填，包含在查詢範圍內，且不可早於 `StartDate` |
| `Category` | 選用，由呼叫端管理的結果標籤；本身不會篩選 RTA 資料 |
| `ItemCodes` | 選用的 SKU／ManCode 篩選；空值查詢全部商品 |

### 檢查目前門店綁定

`BoundStore` 不需連線即可回傳呼叫端設定的唯一門店綁定：

```go
store := client.BoundStore()
```

`Stores` 保留給需要 slice 的呼叫端，但同樣只回傳這一筆設定：

```go
stores, err := client.Stores(ctx)
if err != nil {
	return err
}
for _, store := range stores {
	fmt.Printf("%s\t%s\n", store.BusinessID, store.Label)
}
```

`RefreshStores` 為相容既有 API 而保留，也會回傳同一筆綁定：

```go
stores, err := client.RefreshStores(ctx)
```

這兩個方法都不會把 RTA 的全域門店目錄誤當成帳號授權清單。

### 查詢單日資料

將開始與結束日期設為同一個日曆日期：

```go
day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
result, err := client.Sales(ctx, rtasales.SalesQuery{
	BusinessStoreID: os.Getenv("RTA_BUSINESS_STORE_ID"),
	StartDate:       day,
	EndDate:         day,
})
```

### 查詢指定商品

`ItemCodes` 為選用；空值表示全部商品。送出請求前會移除空白與重複項目。

```go
result, err := client.Sales(ctx, rtasales.SalesQuery{
	BusinessStoreID: os.Getenv("RTA_BUSINESS_STORE_ID"),
	StartDate:       start,
	EndDate:         end,
	Category:        "HA",
	ItemCodes:       []string{"SKU-DEMO-A", "SKU-DEMO-B"},
})
```

若需依分類查詢，請由呼叫端準備相應的 `ItemCodes`。`Category` 只會複製到 `SalesResult` 作為中繼資料，實際送給上游的商品篩選欄位是 `ItemCodes`。

## 填入既有 Excel 活頁簿

`xlsxfill` package 與 `cmd/rta-xlsx-fill` 指令可填入既有活頁簿的兩個每日人工輸入欄，同時保留公式、格式、合併儲存格與其他工作表。預設工作表配置為：

- `C` 欄：活頁簿使用的門店 ID；
- `F` 欄：日曆日期；
- `L` 欄：當日銷售額；
- `AB` 欄：當日顧客／交易總數。

標示為 `Total` 的列、缺少門店或日期的列，以及非指定日期的列都會略過。`C` 欄會先通過設定的 `StoreMapper`，再以業務門店 ID 精確路由到相應 provider。如果其他專案的活頁簿使用不同 ID，可用 `-mapping` 指定私有 JSON object，或包含 `sheet_store_id`、`rta_business_store_id` 標題的 CSV；請勿提交任何已填入資料的對照檔。

這個指令刻意只處理一組帳號／門店綁定。它會從環境變數或已被 Git 忽略的本機 `.env` 讀取 `RTA_ACCOUNT`、`RTA_PASSWORD`、`RTA_BUSINESS_STORE_ID`，且只使用內建 OCR。請先執行 dry-run：

```bash
go run ./cmd/rta-xlsx-fill \
  -input <來源.xlsx> \
  -date <YYYY-MM-DD>
```

確認 dry-run 沒有問題後，另存新的活頁簿：

```bash
go run ./cmd/rta-xlsx-fill \
  -input <來源.xlsx> \
  -output <填入完成.xlsx> \
  -date <YYYY-MM-DD> \
  -write
```

重要的安全預設：

- 輸出路徑不可與來源檔相同；
- 原有數值不同時不會覆寫，除非明確加入 `-overwrite`；
- 對照缺漏、門店綁定不符、缺少聚合交易總數或查詢失敗時，預設不會產生任何部分結果；只有明確加入 `-allow-partial` 才會另存安全完成的列；
- `L` 或 `AB` 若是公式儲存格，絕不覆寫；
- 成功輸出的檔案會設定為使用 Excel 開啟時完整重算；
- JSON 報告只包含列號與問題代碼，不包含帳密、門店 ID 或實際銷售數值。

只有 `C` 欄與私有帳號／門店對照使用的業務 ID 不同時，才需要加入 `-mapping <private.local.csv>`。repository 已忽略 `*.local.csv`、`*.local.json`、Cookie 檔及 `*.filled.xlsx`。

使用權限有限或單一門店帳號執行時，可加入 `-row <工作表列號>` 與 `-max-queries 1`。這樣最多只會查詢該列所代表的一個門店與日期；除非另外明確加入 `-write`，否則仍是 dry-run。

多門店整合時，請依私有帳號／門店對照為每間店建立一個 `Client`，再交給 `xlsxfill.NewProviderRouter`。router 只做本機端的精確分流，不會把門店 ID 填進 RTA 報表 filter。

## Client 設定

| 欄位 | 用途 | 預設值 |
| --- | --- | --- |
| `Account` | RTA 登入帳號，必填 | 無 |
| `Password` | RTA 登入密碼，必填 | 無 |
| `BusinessStoreID` | 呼叫端綁定到此帳號的業務門店 ID，必填 | 無 |
| `BusinessStoreLabel` | 綁定的選用顯示名稱 | `BusinessStoreID` |
| `CaptchaSolvers` | 依序嘗試的驗證碼 solver，至少需要一個 | 無 |
| `CookieFile` | Cookie jar 保存路徑 | 僅記憶體 |
| `HTTPClient` | 自訂 transport、proxy、timeout 或 cookie jar | timeout 30 秒 |
| `PageConcurrency` | 第一頁之後的最大並行查詢數 | `4` |
| `LoginAttempts` | 重新取得驗證碼並登入的次數，可設為 `1`–`10` | `4` |

`Client` 可安全地並行使用。不同帳號請建立不同 `Client`；若要保存 Cookie，也必須使用不同檔案路徑。

## 驗證碼策略與硬體需求

建議順序為內建 OCR 第一、2Captcha 第二：

1. `EmbeddedOCRSolver` 會用彩色元件與灰階兩條獨立路徑，擷取每一個完整字格。
2. 兩套模板模型分別辨識結果：一套正規化筆畫拓撲，另一套保留原始寬高比例。
3. 只有兩套模型結果相同、最差比對距離不高於 `0.20`，且最小第二名領先幅度至少為 `0.02` 時，才接受該字元。
4. 圖片格式錯誤或任一字元未通過信心門檻時，下一個 solver 會收到同一張圖片。
5. 若沒有備援 solver 成功，下次登入會取得全新 challenge。登入預設嘗試四次；可用 `LoginAttempts` 設為 `1`–`10`。

只設定內建 OCR 時，信心不足的圖片不會送出；下一次登入嘗試會改抓新的驗證碼。模型或擷取路徑結果不同時會刻意安全拒絕，因此不建議只為了減少重試而放寬 `MaximumDistance` 或 `MinimumScoreMargin`。

在一批未參與訓練的 1,000 張 challenge 驗證中，raw top-1 辨識為 `989/1000`；預設信心門檻實際送出 `905/1000`，且 RTA 確認送出的 `905/905` 全部正確，其餘 95 張皆安全拒絕。有限樣本不能證明 99.99% 準確率。若以觀測到的 9.5% 拒絕率估算，四張互相獨立的新 challenge 至少一張通過本機門檻的機率為 `1 - 0.095^4 = 99.9919%`。這是登入重試層的估算，不是單張 OCR 準確率。

內建 OCR 只使用一般 CPU 指令。兩套字形模板都已編譯進套件，每個程序只會準備一次，並由所有 solver 實例共用。它沒有背景服務，也不需要 GPU、CGO、外部執行檔或模型檔。

建議直接使用零值設定：

```go
solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
```

只有上游驗證碼格式變更且已重新校準時，才建議調整進階欄位：

| 欄位 | 說明 |
| --- | --- |
| `Length` | 固定字元數；非正數使用 `5` |
| `Alphabet` | 允許辨識的 ASCII 字元；空值使用十六進位字元 |
| `MaximumDistance` | 比對距離高於此值就拒絕；零值使用 `0.20`，越小越嚴格 |
| `MinimumScoreMargin` | 第一名至少要領先第二名的分數；零值使用 `0.02`，越大越嚴格 |

除非已有獨立樣本驗證，否則請保留預設值。已部署 Tesseract 的應用仍可使用 `TesseractSolver`。自訂 solver 只需實作：

```go
type CaptchaSolver interface {
	Solve(context.Context, []byte) (string, error)
}
```

若本機 OCR 失敗時應直接終止請求，就不要設定付費備援 solver。

## 回傳結果

`SalesResult` 包含：

- `Store`、`StartDate`、`EndDate`、`Category` 與正規化後的 `ItemCodes`；
- `Items`，依 RTA 分頁順序包含所有明細；
- `TotalAmount`、`TotalTransactionCount` 與 `GrossQuantity`；
- `Categories`，依第四與第五層分類彙總；
- `QueryDuration`。

每個 `SaleItem` 都提供常用的型別化欄位，以及保留完整上游資料的 `Raw` map。數量使用 `float64`，避免秤重商品被截斷。任何分頁失敗時，`Sales` 會回傳錯誤，不會回傳部分 `SalesResult`。

`TotalAmount` 為 `tp_sale_amount` 加總；`GrossQuantity` 為 `tp_gross_sale_qty` 加總。`TotalTransactionCount` 直接讀取 `countResult.result[0].tp_transaction_count`。它不會使用商品列的 `tp_transaction_count` 或 `tp_transaction_count_agg` 推算，因為同一張交易可能包含多個商品。

## 錯誤處理

可使用 `errors.As` 判斷下列錯誤型別：

- `InputError`
- `AuthError`
- `StoreNotFoundError`
- `CaptchaError`
- `UpstreamError`
- `ProtocolError`

```go
result, err := client.Sales(ctx, query)
if err != nil {
	var missing *rtasales.StoreNotFoundError
	var upstream *rtasales.UpstreamError
	switch {
	case errors.As(err, &missing):
		// 路由到綁定這個完整業務門店 ID 的 Client。
	case errors.As(err, &upstream) && upstream.Retryable():
		// 由呼叫端執行有次數上限的重試。
	default:
		return err
	}
}
_ = result
```

傳輸錯誤，以及 HTTP 408、429 或 5xx 回應，會讓 `UpstreamError.Retryable()` 回傳 `true`。每次已驗證請求的登入恢復由套件內部處理一次。

## Cookie 與秘密資料

- `CookieFile` 留空時使用記憶體 Cookie jar。
- 設定檔案後會保存有效期限內的 Cookie；支援的平台會將權限設為 `0600`。
- 帳號、密碼與備援 API key 應放在秘密管理服務或環境變數。
- 每一組帳號／門店綁定使用不同的 Cookie 路徑。
- 不要提交 Cookie 或 `.env` 檔案。
- 正式環境避免記錄完整 `Store`、`SaleItem.Raw`、Cookie、登入資料或完整上游回應內容。

本 repository 已忽略常見 Cookie 檔名與 `.env*`。

## 開發驗證

```bash
go test ./...
go test -race ./...
go vet ./...
```

所有提交的測試都使用合成圖片與本機 HTTP fixture，不會連線 RTA 或 2Captcha；repository 也不包含原始驗證碼圖片或正式門店資料。
