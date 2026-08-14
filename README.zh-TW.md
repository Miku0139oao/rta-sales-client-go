# rta-sales-client-go

[English](README.md) | 繁體中文

`rta-sales-client-go` 是獨立的 RTA 合作夥伴銷售查詢 Go library，提供：

- 自動 SSO 登入、Cookie 保存與登入逾期自動恢復；
- 純 Go、CPU-only 的內建驗證碼 OCR，以及依序執行的備援 solver；
- 登入後動態取得帳號可用門店；
- 使用門店 ID 與日期區間精確查詢銷售資料；
- 可選的 SKU／ManCode 篩選；
- 完整型別化明細、原始欄位、總計與分類彙總；
- 有上限的並行分頁查詢，任一頁失敗時不回傳不完整結果。

本套件不依賴資料庫或全域程序狀態，也不需要 GPU、CGO、Tesseract 或外部 OCR 模型。每個 RTA 帳號應建立各自的 `Client`，避免 Cookie 與門店快取互相混用。

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
		Account:        os.Getenv("RTA_ACCOUNT"),
		Password:       os.Getenv("RTA_PASSWORD"),
		CookieFile:     "state/rta.cookies.json",
		CaptchaSolvers: solvers,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.Sales(ctx, rtasales.SalesQuery{
		// 使用 client.Stores 回傳的業務門店 ID，不要傳入 RTA 內部 ID。
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
| `BusinessStoreID` | 必填，必須是 `Stores` 回傳的完整業務門店 ID |
| `StartDate` | 必填，包含在查詢範圍內的開始日期 |
| `EndDate` | 必填，包含在查詢範圍內，且不可早於 `StartDate` |
| `Category` | 選用，由呼叫端管理的結果標籤；本身不會篩選 RTA 資料 |
| `ItemCodes` | 選用的 SKU／ManCode 篩選；空值查詢全部商品 |

### 列出帳號可用門店

使用 `Stores` 載入登入帳號可見的門店表。`Sales` 只接受業務門店 ID。

```go
stores, err := client.Stores(ctx)
if err != nil {
	return err
}
for _, store := range stores {
	fmt.Printf("%s\t%s\n", store.BusinessID, store.Label)
}
```

`Stores` 會回傳快取的防禦性複本。帳號權限或上游門店資料可能已變更時，使用 `RefreshStores` 強制重新載入：

```go
stores, err := client.RefreshStores(ctx)
```

門店資料只會在登入後取得，並快取於該帳號的 `Client`。

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

## Client 設定

| 欄位 | 用途 | 預設值 |
| --- | --- | --- |
| `Account` | RTA 登入帳號，必填 | 無 |
| `Password` | RTA 登入密碼，必填 | 無 |
| `CaptchaSolvers` | 依序嘗試的驗證碼 solver，至少需要一個 | 無 |
| `CookieFile` | Cookie jar 保存路徑 | 僅記憶體 |
| `HTTPClient` | 自訂 transport、proxy、timeout 或 cookie jar | timeout 30 秒 |
| `PageConcurrency` | 第一頁之後的最大並行查詢數 | `4` |
| `LoginAttempts` | 重新取得驗證碼並登入的次數，可設為 `1`–`10` | `4` |

`Client` 可安全地並行使用。不同帳號請建立不同 `Client`；若要保存 Cookie，也必須使用不同檔案路徑。

## 驗證碼策略與硬體需求

建議順序為內建 OCR 第一、2Captcha 第二：

1. `EmbeddedOCRSolver` 會分別用彩色元件與灰階路徑擷取每個字元，再採用模板距離較佳的結果。
2. 圖片格式錯誤或辨識信心不足時，下一個 solver 會收到同一張圖片。
3. 若 RTA 拒絕一個格式合理的答案，下次登入會取得新圖片，並從下一個 solver 開始。
4. 登入預設嘗試四次；可用 `LoginAttempts` 設為 `1`–`10`。

只設定內建 OCR 時，信心不足的圖片不會送出；下一次登入嘗試會改抓新的驗證碼。兩條擷取路徑若結果不同且分數接近，也會安全拒絕，因此不建議只為了減少重試而降低 `MinimumScoreMargin`。

依目前觀測到的單張 5% 安全拒絕率計算，四張互相獨立的新圖全部被拒絕的機率為 `0.05^4 = 0.000625%`，也就是取得可提交答案的機率為 99.999375%。這是登入重試層的估算，不代表已認證單張圖片具有 99.99% 準確率。

內建 OCR 只使用一般 CPU 指令。字形模板已編譯進套件，每個程序只會準備一次，並由所有 solver 實例共用。它沒有背景服務，也不需要 GPU、CGO、外部執行檔或模型檔。

建議直接使用零值設定：

```go
solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
```

只有上游驗證碼格式變更且已重新校準時，才建議調整進階欄位：

| 欄位 | 說明 |
| --- | --- |
| `Length` | 固定字元數；非正數使用 `5` |
| `Alphabet` | 允許辨識的 ASCII 字元；空值使用十六進位字元 |
| `MaximumDistance` | 比對距離高於此值就拒絕；越小越嚴格 |
| `MinimumScoreMargin` | 第一名至少要領先第二名的分數；越大越嚴格 |

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
- `TotalAmount` 與 `GrossQuantity`；
- `Categories`，依第四與第五層分類彙總；
- `QueryDuration`。

每個 `SaleItem` 都提供常用的型別化欄位，以及保留完整上游資料的 `Raw` map。數量使用 `float64`，避免秤重商品被截斷。任何分頁失敗時，`Sales` 會回傳錯誤，不會回傳部分 `SalesResult`。

`TotalAmount` 為 `tp_sale_amount` 加總；`GrossQuantity` 為 `tp_gross_sale_qty` 加總。

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
		// 重新載入門店表，或修正業務門店 ID。
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
