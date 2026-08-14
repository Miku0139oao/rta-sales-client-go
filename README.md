# rta-sales-client-go

English | [繁體中文](README.zh-TW.md)

`rta-sales-client-go` is a standalone Go client for the RTA partner sales view. It provides:

- automatic SSO login, cookie persistence, and expired-session recovery;
- built-in, CPU-only captcha OCR with ordered fallbacks;
- exact account/store binding from a caller-owned private mapping;
- inclusive date-range sales queries using the bound account session;
- optional SKU/ManCode filtering;
- complete typed rows, raw upstream fields, totals, and category aggregates;
- safe date-scoped filling of existing Excel workbooks without replacing formulas or styles;
- bounded concurrent pagination with whole-query error handling.

The module has no database or global process-state dependency and does not require GPU hardware, CGO, Tesseract, or a separate OCR model. Use one `Client` per RTA account and business-store binding so cookies and store scope remain isolated.

RTA applies this report's store scope through the authenticated account session. Its store-tree endpoint is a global catalogue, not an authorization list, and its identifiers are not valid values for this report's `store_id` filter. The client therefore sends the expected filter fields empty and uses `BusinessStoreID` only as a strict local routing guard. The caller remains responsible for loading the correct account/store pair from its private mapping.

## Requirements and installation

- Go 1.25 or newer.
- An RTA account that can access the required stores.
- Optional: a 2Captcha API key if remote fallback is wanted.

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

Set credentials through your environment or secret manager:

```dotenv
RTA_ACCOUNT=
RTA_PASSWORD=
RTA_BUSINESS_STORE_ID=
RTA_COOKIE_FILE=
TWOCAPTCHA_API_KEY=
```

`TWOCAPTCHA_API_KEY` is optional. Without it, the example uses only embedded OCR.

## Quick start

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
		// Must exactly match the private binding supplied to NewClient.
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

`StartDate` and `EndDate` are inclusive. Their displayed year, month, and day are sent without timezone conversion.

## Common operations

`SalesQuery` fields:

| Field | Meaning |
| --- | --- |
| `BusinessStoreID` | Required exact ID bound to this `Client` by the caller's private mapping |
| `StartDate` | Required inclusive start calendar date |
| `EndDate` | Required inclusive end calendar date; cannot precede `StartDate` |
| `Category` | Optional caller-owned result label; it does not filter RTA by itself |
| `ItemCodes` | Optional SKU/ManCode filter; empty queries all products |

### Inspect the configured store binding

`BoundStore` returns the single caller-configured binding without network I/O:

```go
store := client.BoundStore()
```

`Stores` is retained for callers that expect a slice, but it also returns only that one configured binding:

```go
stores, err := client.Stores(ctx)
if err != nil {
	return err
}
for _, store := range stores {
	fmt.Printf("%s\t%s\n", store.BusinessID, store.Label)
}
```

`RefreshStores` is retained for API compatibility and returns the same binding:

```go
stores, err := client.RefreshStores(ctx)
```

Neither method treats RTA's global store catalogue as account authorization.

### Query one day

Set both dates to the same calendar date:

```go
day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
result, err := client.Sales(ctx, rtasales.SalesQuery{
	BusinessStoreID: os.Getenv("RTA_BUSINESS_STORE_ID"),
	StartDate:       day,
	EndDate:         day,
})
```

### Query selected products

`ItemCodes` is optional. Empty means all products. Duplicate and blank values are removed before the request.

```go
result, err := client.Sales(ctx, rtasales.SalesQuery{
	BusinessStoreID: os.Getenv("RTA_BUSINESS_STORE_ID"),
	StartDate:       start,
	EndDate:         end,
	Category:        "HA",
	ItemCodes:       []string{"SKU-DEMO-A", "SKU-DEMO-B"},
})
```

Prepare any category-specific `ItemCodes` in the caller. `Category` is metadata copied into `SalesResult`, while `ItemCodes` is the field that filters the upstream query.

## Fill an existing Excel workbook

The `xlsxfill` package and `cmd/rta-xlsx-fill` command can populate the two manual daily-input fields in an existing workbook while retaining its formulas, formatting, merged cells, and other sheets. The default worksheet layout is:

- column `C`: workbook-facing store ID;
- column `F`: calendar date;
- column `L`: daily sales amount;
- column `AB`: daily customer/transaction count.

Rows labeled `Total`, rows without both a store and date, and dates other than the requested date are skipped. Column `C` is passed through the configured `StoreMapper` and then routed to a provider with an exact business-store match. If another project uses different workbook IDs, `-mapping` accepts a private JSON object or a CSV with `sheet_store_id` and `rta_business_store_id` headers. Never commit a populated mapping.

The command is intentionally scoped to one account/store pair. It loads `RTA_ACCOUNT`, `RTA_PASSWORD`, and `RTA_BUSINESS_STORE_ID` from the environment or an ignored local `.env`, and uses embedded OCR only. Start with a dry run:

```bash
go run ./cmd/rta-xlsx-fill \
  -input <source.xlsx> \
  -date <YYYY-MM-DD>
```

To save a new workbook after a clean dry run:

```bash
go run ./cmd/rta-xlsx-fill \
  -input <source.xlsx> \
  -output <filled.xlsx> \
  -date <YYYY-MM-DD> \
  -write
```

Important safety defaults:

- the source workbook can never be used as the output path;
- existing values that differ are not replaced unless `-overwrite` is explicit;
- any missing mapping, store-binding mismatch, missing transaction aggregate, or query failure prevents all output unless `-allow-partial` is explicit;
- formula cells in `L` or `AB` are never replaced;
- successful output is marked for full recalculation when opened in Excel;
- the JSON report contains row numbers and issue codes, not credentials, store IDs, or returned sales values.

Use `-mapping <private.local.csv>` only when column `C` differs from the business ID in the private account/store binding. The repository ignores `*.local.csv`, `*.local.json`, cookie files, and `*.filled.xlsx`.

For a permission-limited or single-account run, add `-row <worksheet-row>` and `-max-queries 1`. This queries at most the single store/date represented by that row and remains a dry run unless `-write` is also explicit.

For a multi-store integration, create one `Client` for every private account/store binding and pass them through `xlsxfill.NewProviderRouter`. The router performs exact local dispatch; it does not place a store ID into the RTA report filter.

## Client configuration

| Field | Purpose | Default |
| --- | --- | --- |
| `Account` | RTA login account; required | none |
| `Password` | RTA login password; required | none |
| `BusinessStoreID` | Caller-owned ID bound to this account; required | none |
| `BusinessStoreLabel` | Optional display label for the binding | `BusinessStoreID` |
| `CaptchaSolvers` | Solvers attempted in order; at least one required | none |
| `CookieFile` | Persistent cookie-jar path | in-memory cookies |
| `HTTPClient` | Optional custom transport, proxy, timeout, or cookie jar | 30-second client |
| `PageConcurrency` | Maximum concurrent requests after the first sales page | `4` |
| `LoginAttempts` | Fresh captcha/login attempts; accepted range `1`–`10` | `4` |

`Client` is safe for concurrent use. Create separate clients for separate accounts and give each persistent client a different cookie file.

## Captcha strategy and hardware

The recommended order is embedded OCR first and 2Captcha second:

1. `EmbeddedOCRSolver` extracts every complete glyph cell through independent color-component and grayscale paths.
2. Two template models classify the result: one normalizes stroke topology, while the other preserves the original aspect ratio.
3. A character is accepted only when both models agree, its worst match distance is at most `0.20`, and its smallest runner-up margin is at least `0.02`.
4. If the image is malformed or the gate rejects any character, the next solver receives the same image.
5. If no fallback solver succeeds, the next login attempt obtains a fresh challenge. Login performs four attempts by default; `LoginAttempts` can set `1`–`10`.

When embedded OCR is the only configured solver, an uncertain image is not submitted. The next login attempt requests a fresh captcha instead. Model or extraction-path disagreements are intentionally rejected, so loosening `MaximumDistance` or `MinimumScoreMargin` merely to reduce retries is not recommended.

In an independent 1,000-challenge validation run, raw top-1 recognition was `989/1000`. The default confidence gate submitted `905/1000`, and RTA accepted all `905/905` submitted answers; the other 95 were safely rejected. This finite observation is not a 99.99% accuracy guarantee. At the observed 9.5% rejection rate, four independent fresh challenges give an estimated `1 - 0.095^4 = 99.9919%` chance that at least one passes the local gate. That is an operational retry estimate, not per-image OCR accuracy.

Embedded OCR uses ordinary CPU instructions. Both template sets are compiled into the package, prepared once per process, and shared across solver instances. It has no background service and requires no GPU, CGO, executable, or external model file.

The zero-value configuration is recommended:

```go
solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
```

Advanced fields are available when the upstream captcha format changes:

| Field | Meaning |
| --- | --- |
| `Length` | Fixed glyph count; non-positive uses `5` |
| `Alphabet` | Supported ASCII candidates; empty uses hexadecimal digits |
| `MaximumDistance` | Reject above this match distance; zero uses `0.20`, and smaller is stricter |
| `MinimumScoreMargin` | Required lead over the runner-up; zero uses `0.02`, and larger is stricter |

Keep the defaults unless they have been recalibrated with independent samples. `TesseractSolver` remains available for applications that already deploy Tesseract. A custom solver only needs this interface:

```go
type CaptchaSolver interface {
	Solve(context.Context, []byte) (string, error)
}
```

Do not configure a paid fallback if local OCR failure should fail the request instead.

## Results

`SalesResult` includes:

- `Store`, `StartDate`, `EndDate`, `Category`, and normalized `ItemCodes`;
- `Items`, containing every result page in deterministic page order;
- `TotalAmount`, `TotalTransactionCount`, and `GrossQuantity`;
- `Categories`, aggregated from category levels 4 and 5;
- `QueryDuration`.

Each `SaleItem` exposes commonly used typed fields and a `Raw` map containing the complete upstream row. Quantities use `float64` so weighted-product sales are not truncated. If any page fails, `Sales` returns an error and no partial `SalesResult`.

`TotalAmount` sums `tp_sale_amount`; `GrossQuantity` sums `tp_gross_sale_qty`. `TotalTransactionCount` is read from `countResult.result[0].tp_transaction_count`. It is deliberately not calculated from item-level `tp_transaction_count` values or `tp_transaction_count_agg`, because one transaction can contain multiple items.

## Error handling

The library returns typed errors suitable for `errors.As`:

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
		// Route to the Client bound to this exact business-store ID.
	case errors.As(err, &upstream) && upstream.Retryable():
		// Apply a bounded caller-level retry.
	default:
		return err
	}
}
_ = result
```

`UpstreamError.Retryable()` returns true for transport failures and HTTP 408, 429, or 5xx responses. Authentication recovery is handled internally once per authenticated request.

## Cookie and secret handling

- Empty `CookieFile` uses an in-memory jar.
- A configured file persists expiring cookies and is chmodded to `0600` where supported.
- Keep credentials and fallback API keys in a secret manager or environment variables.
- Use a separate cookie path for each account/store binding.
- Never commit cookie files or `.env` files.
- Avoid logging complete `Store`, `SaleItem.Raw`, cookies, credentials, or upstream response bodies in production.

Common cookie filenames and `.env*` are ignored by this repository.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
```

All committed tests use synthetic images and local HTTP fixtures. They do not call RTA or 2Captcha, and the repository contains no original captcha images or production store data.
