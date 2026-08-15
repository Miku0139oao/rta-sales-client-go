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

## Windows desktop app

RTA Excel Filler is the Windows desktop client for processing one day or an
inclusive date range without using the command line. CI publishes three files
in the `RTA-Excel-Filler-windows-amd64` artifact:

- `RTA-Excel-Filler-setup.exe`: per-user NSIS installer; it downloads WebView2
  when the runtime is missing;
- `RTA-Excel-Filler-portable.exe`: portable application; Microsoft Edge
  WebView2 Runtime must already be installed;
- `SHA256SUMS.txt`: SHA-256 hashes for both executables.

Uninstalling removes the application and shortcuts but intentionally preserves
per-user profiles, encrypted cookie state, and Windows Credential Manager
entries for a later reinstall. To remove saved RTA account data as well, delete
each profile in the application before uninstalling it.

Verify a downloaded file before running it:

```powershell
(Get-FileHash -Algorithm SHA256 .\RTA-Excel-Filler-setup.exe).Hash.ToLowerInvariant()
Get-Content .\SHA256SUMS.txt
```

### Accounts and private data

Open **Accounts**, add a display name and the RTA account/password, then use
**Test** before enabling the profile for analysis. Profile order is also store
ownership priority: if two profiles can access the same store, the first
enabled profile wins. The query-job concurrency limit in **Settings** applies
across stores and dates even when one multi-store account handles every job
(default `2`, maximum `4`).

Passwords are kept in Windows Credential Manager. Each profile's cookies are
encrypted with Windows DPAPI before being saved under the current user's
application-data directory. `profiles.json` contains only display metadata.
Workbook previews, sales values, store routing, and analysis plans remain in
process memory and are not written to settings or logs. Deleting a profile also
removes its saved credentials and encrypted cookies.

### Multi-day workbook workflow

1. Open an `.xlsx` workbook and select the sheet and inclusive date range.
2. Review the scan counts. The default safety ceiling is `2,000` unique
   date/store jobs.
3. Select **Analyze**. Each unique date/store pair is queried once, and every
   RTA request covers exactly one calendar day.
4. Review the proposed values: column `L` is Trend View daily gross sales
   (sales less returns), and column `AB` is the same Trend View day's
   transaction count.
5. If work was cancelled after a workbook plan was created, or a temporary job
   still failed after the built-in retries, select **Retry failed/pending**. If
   cancellation happened while signing in or loading authorized stores, run
   **Analyze** again because no retryable plan exists yet. A cancelled,
   incomplete plan can never be written.
6. Save to a new workbook. Strict mode requires a complete plan with no issues.
   Partial output is available only after analysis completed; affected rows are
   kept entirely unchanged and require an explicit confirmation.

The source workbook is never overwritten. Before applying a plan, the app
checks the source SHA-256, size, and modification time and rejects the write if
the file changed. Existing different values require **Overwrite existing
values**, and formulas in `L` or `AB` are never replaced.

For workbooks whose column `C` does not contain the RTA business store ID,
enable the optional private JSON/CSV mapping in **Settings**. Keep populated
mapping files outside version control.

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

RTA returns the authorized stores as a `data` array. For each entry, the client keeps `key` only for the Article View private filter and derives `BusinessID` from the part of `value` before the first `-`. The same `BusinessID` is used by Trend View. The ID stays a string, so leading zeroes are preserved. Only `BusinessID` and the full `Label` are exposed; no store mapping is written to disk.

### Sales query fields

| Field | Meaning |
| --- | --- |
| `BusinessStoreID` | Required exact ID from `Stores` |
| `StartDate` | Required inclusive start calendar date |
| `EndDate` | Required inclusive end date; cannot precede `StartDate` |
| `Category` | Optional caller-owned result label; it does not filter RTA by itself |
| `ItemCodes` | Optional SKU/ManCode filter; empty queries all products |

Blank and duplicate `ItemCodes` are removed before the request. `TrendGrossSaleAmount` and `TotalTransactionCount` are read directly from matching daily rows in RTA Trend View (`gross_sales_gross_sale_untaxed_amt` and `group_sales_ticket_num`) and summed over the requested inclusive date range. They are whole-store values and are not derived from Article View item rows. `TotalAmount` remains the item-filterable Article View aggregate.

## Fill an existing Excel workbook

`xlsxfill` and `cmd/rta-xlsx-fill` populate the two manual daily fields while preserving formulas, formatting, merged cells, and other sheets. The default worksheet is `Dairly`:

- column `C`: business-facing store ID;
- column `F`: calendar date;
- column `L`: the matching date's Trend View gross sales amount (sales less returns);
- column `AB`: the matching date's Trend View transaction count.

The command logs in first, loads the account's `data[]` authorized-store list, and then compares every row for the requested date. Column `C` is resolved against the string prefix of each store `value`; the private `key` is used only by Article View, while Trend View uses that prefix. Rows belonging to stores outside the signed-in account are skipped without being queried. No row number, local store mapping, or extra store environment variable is needed.

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

Use `-from` and `-to` together for an inclusive range. They are mutually
exclusive with `-date`:

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\source.xlsx" `
  -from 2026-08-01 `
  -to 2026-08-31
```

After a clean dry run, save to a different file:

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\source.xlsx" `
  -output "C:\path\source.filled.xlsx" `
  -date 2026-08-13 `
  -write
```

Do not pass `-row` during normal use; it is diagnostic-only and cannot be used
with `-write`. `-max-jobs` is the safety ceiling after automatic store
matching (default `2,000`); `-max-queries` remains as a deprecated alias.
`-concurrency` is capped at `4` and applies to date/store query jobs, including
jobs handled by the same multi-store account.
There is no whole-operation timeout by default. Use an explicit value such as
`-timeout 20m` when one is required.

The JSON report distinguishes the stages: `matched_rows` counts rows for the date, `selected_rows` counts rows authorized for this account, and `skipped_store_rows` counts date rows belonging to other accounts. If none of the date rows match an authorized store, the command fails instead of silently producing an unchanged workbook.

Safety defaults:

- dry-run is the default;
- output can never overwrite the source workbook;
- existing different values require explicit `-overwrite`;
- formula cells in `L` or `AB` are never replaced;
- stores outside the signed-in account are skipped before any sales request;
- no authorized store match, missing report totals, or query failures prevent output unless `-allow-partial` is explicit;
- the JSON report contains row numbers and issue codes, not credentials, store IDs, or sales values.

If a different workbook uses codes that are not RTA business-facing IDs, `-mapping` accepts a private local JSON object or CSV. Populated mapping files, cookies, `.env`, and `*.filled.xlsx` are ignored and must not be committed.

Library callers using `xlsxfill.Fill` can supply the IDs returned by `Client.Stores` through `Request.AllowedBusinessStoreIDs` to apply the same automatic selection.

### Two-phase batch API

New integrations should use the two-phase API. `Analyze` never changes the
workbook, `RetryFailed` resumes temporary failures and cancelled pending work,
and `Apply` writes only a complete plan whose source fingerprint still matches:

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

Temporary transport and HTTP 408/429/5xx failures are retried twice after
`1s` and `3s`. No-data, permission, mapping, and workbook-format problems are
terminal. Set `AllowPartial` only for a completed plan with issues; every issue
row remains unchanged. The original single-date `xlsxfill.Fill` and CLI
`-date` behavior remain supported.

## Client configuration

| Field | Purpose | Default |
| --- | --- | --- |
| `Account` | RTA login account; required | none |
| `Password` | RTA login password; required | none |
| `CaptchaSolvers` | Solvers attempted in order; at least one required | none |
| `CookieFile` | Persistent cookie-jar path | in-memory cookies |
| `CookieStore` | Pluggable cookie persistence, mutually exclusive with `CookieFile` | none |
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

`SalesResult` contains the selected `Store`, date range, normalized item codes, every item page in deterministic order, Article View `TotalAmount`, Trend View `TrendGrossSaleAmount` and `TotalTransactionCount`, `GrossQuantity`, category aggregates, and query duration. Each `SaleItem` also retains the complete upstream row in `Raw`.

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

cd desktop/frontend
bun install --frozen-lockfile
bun run verify

cd ../../cmd/rta-excel-filler
wails build -platform windows/amd64
```

Desktop builds use Wails CLI `v2.14.0`, Bun, and NSIS 3. Install the pinned
CLI with `go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0`.

Committed tests use synthetic images and local HTTP fixtures. They do not contact RTA or external captcha services and contain no production store data.
