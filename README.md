# rta-sales-client-go

English | [繁體中文](README.zh-TW.md)

`rta-sales-client-go` is a reusable Go client for the RTA partner sales view. It provides:

- automatic SSO login, persistent cookies, and expired-session recovery;
- embedded CPU-only captcha OCR, with optional fallback solvers;
- automatic loading of the signed-in account's authorized stores;
- exact business-store selection for inclusive calendar-date sales queries;
- paginated typed rows, raw upstream fields, report totals, and aggregates;
- safe filling of the manual daily-input cells in an existing `.xlsx` workbook.

The client obtains the account-specific store relationship from RTA itself. Callers use only the business-facing ID returned by `Stores`; the query-only values required by RTA remain private inside the client and are never exposed as public configuration.

## Requirements and installation

- Go 1.25 or newer.
- An RTA account with permission for the required stores.
- Optional: a 2Captcha API key if a remote OCR fallback is wanted.

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

Credentials should come from environment variables or a secret manager:

```dotenv
RTA_ACCOUNT=
RTA_PASSWORD=
RTA_COOKIE_FILE=
TWOCAPTCHA_API_KEY=
```

No store filter ID is required in `Config`. The client loads authorized stores after login.

## Library quick start

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
		log.Fatal("the account has no authorized stores")
	}

	// Select the exact business-facing ID required by your application.
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

In production, select a store by exact `BusinessID`; do not use a label, prefix, or fuzzy match. `StartDate` and `EndDate` are inclusive. Their displayed calendar dates are sent without timezone conversion.

### Authorized stores

`Stores` logs in when needed, loads the account's authorized-store list, and caches it:

```go
stores, err := client.Stores(ctx)
for _, store := range stores {
	fmt.Printf("%s\t%s\n", store.BusinessID, store.Label)
}
```

Use `RefreshStores` when permissions may have changed:

```go
stores, err := client.RefreshStores(ctx)
```

Both methods expose only `BusinessID` and `Label`. RTA's internal query values are retained in memory and used automatically by `Sales`.

### Sales query fields

| Field | Meaning |
| --- | --- |
| `BusinessStoreID` | Required exact ID from `Stores` |
| `StartDate` | Required inclusive start calendar date |
| `EndDate` | Required inclusive end date; cannot precede `StartDate` |
| `Category` | Optional caller-owned result label; it does not filter RTA by itself |
| `ItemCodes` | Optional SKU/ManCode filter; empty queries all products |

Blank and duplicate `ItemCodes` are removed before the request. `TotalTransactionCount` comes from the report-level transaction aggregate and is not summed from item rows.

## Fill an existing Excel workbook

`xlsxfill` and `cmd/rta-xlsx-fill` populate the two manual daily fields while preserving formulas, formatting, merged cells, and other sheets. The default worksheet is `Dairly`:

- column `C`: business-facing store ID;
- column `F`: calendar date;
- column `L`: daily sales amount;
- column `AB`: daily customer/transaction count.

Column `C` is resolved exactly through the account's authorized-store list. No extra store environment variable is needed.

Create a local `.env` file; it is ignored by Git:

```dotenv
RTA_ACCOUNT=your-account
RTA_PASSWORD=your-password
RTA_COOKIE_FILE=.rta-sales.cookies.json
```

Always start with a dry run. PowerShell:

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\source.xlsx" `
  -date 2026-08-13
```

Bash:

```bash
go run ./cmd/rta-xlsx-fill \
  -input /path/source.xlsx \
  -date 2026-08-13
```

After a clean dry run, save to a different file:

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\source.xlsx" `
  -output "C:\path\source.filled.xlsx" `
  -date 2026-08-13 `
  -write
```

For a permission-limited test, process only one row and one store query:

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\source.xlsx" `
  -date 2026-08-13 `
  -row <worksheet-row> `
  -max-queries 1
```

Safety defaults:

- dry-run is the default;
- output can never overwrite the source workbook;
- existing different values require explicit `-overwrite`;
- formula cells in `L` or `AB` are never replaced;
- unauthorized stores, missing report totals, or query failures prevent output unless `-allow-partial` is explicit;
- the JSON report contains row numbers and issue codes, not credentials, store IDs, or sales values.

If a different workbook uses codes that are not RTA business-facing IDs, `-mapping` accepts a private local JSON object or CSV. Populated mapping files, cookies, `.env`, and `*.filled.xlsx` are ignored and must not be committed.

## Client configuration

| Field | Purpose | Default |
| --- | --- | --- |
| `Account` | RTA login account; required | none |
| `Password` | RTA login password; required | none |
| `CaptchaSolvers` | Solvers attempted in order; at least one required | none |
| `CookieFile` | Persistent cookie-jar path | in-memory cookies |
| `HTTPClient` | Custom transport, proxy, timeout, or cookie jar | 30-second client |
| `PageConcurrency` | Concurrent requests after the first page | `4` |
| `LoginAttempts` | Fresh captcha/login attempts, from `1` to `10` | `4` |

`Client` is safe for concurrent use. Use a separate client and cookie path for each account.

## Captcha OCR and hardware

The embedded solver uses two independent extraction/classification paths and accepts an answer only when both agree and pass the confidence gate. An uncertain image is not submitted; the next login attempt requests a fresh challenge or the next configured solver is used.

It uses ordinary CPU instructions and needs no GPU, CGO, Tesseract executable, background service, or external model file. The recommended configuration is:

```go
solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
```

In an independent 1,000-challenge validation, raw top-1 recognition was `989/1000`. The default gate submitted `905/1000`, and all `905` submitted answers were accepted in that finite sample. This is not a 99.99% per-image accuracy guarantee. Four fresh challenges provide retry-level reliability; rejected images are not treated as correct results.

Applications that want a remote fallback can append `NewTwoCaptchaSolver`. Applications already deploying Tesseract can use `TesseractSolver`, and custom solvers implement:

```go
type CaptchaSolver interface {
	Solve(context.Context, []byte) (string, error)
}
```

## Results and errors

`SalesResult` contains the selected `Store`, date range, normalized item codes, every item page in deterministic order, `TotalAmount`, report-level `TotalTransactionCount`, `GrossQuantity`, category aggregates, and query duration. Each `SaleItem` also retains the complete upstream row in `Raw`.

Typed errors support `errors.As`:

- `InputError`
- `AuthError`
- `StoreNotFoundError`
- `CaptchaError`
- `UpstreamError`
- `ProtocolError`

`UpstreamError.Retryable()` is true for transport failures and HTTP 408, 429, or 5xx responses. A failed page fails the whole sales query; partial results are never returned.

## Security and development

- Keep credentials and optional service keys in environment variables or a secret manager.
- Never commit `.env`, cookies, populated local mappings, or generated workbooks.
- Avoid logging cookies, credentials, complete upstream bodies, `SaleItem.Raw`, or store-selection internals.
- Persistent cookie files are chmodded to `0600` where supported.

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Committed tests use synthetic images and local HTTP fixtures. They do not contact RTA or external captcha services and contain no production store data.
