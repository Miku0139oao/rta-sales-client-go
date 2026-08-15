# rta-sales-client-go

English | [繁體中文](README.zh-TW.md)

The Windows app is **RTA Sales Analyzer** (`0.1.1`). It signs in to RTA, solves the captcha, loads the account's stores, can export per-store PDFs, and can fill the two manual daily cells in the existing company workbook.

The same repo also has:

- a CLI, `go run ./cmd/rta-xlsx-fill`, for the workbook job without a window
- a Go library other programs can import

The exe is still named `RTA-Excel-Filler.exe`. That filename was left alone so installers and CI artifacts did not have to move. The window title is the new name.

## Install the desktop app

64-bit Windows 10 or 11. CI publishes `RTA-Excel-Filler-windows-amd64` with three files:

- `RTA-Excel-Filler-setup.exe` — use this. It downloads WebView2 if the machine does not have it.
- `RTA-Excel-Filler-portable.exe` — no installer. [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/) must already be installed.
- `SHA256SUMS.txt` — checksums.

```powershell
(Get-FileHash -Algorithm SHA256 .\RTA-Excel-Filler-setup.exe).Hash.ToLowerInvariant()
Get-Content .\SHA256SUMS.txt
```

After setup, open **RTA Sales Analyzer** from the Start menu. The portable build is just the exe. Only one window can run at a time.

Uninstall keeps accounts. Profiles, encrypted cookies, and Credential Manager entries stay under `%AppData%\RTA Excel Filler` (old folder name on purpose, so existing data still loads). To wipe saved RTA accounts, delete each profile in the app first.

## Using the desktop app

Add an account first, then use **Sales analysis** or **Excel fill**. The UI defaults to Traditional Chinese. Settings can switch to English and change the theme.

![Accounts](release/account-pool-desktop-verified.png)

### Accounts

**Accounts** → **Add account**. Display name, RTA login, password. Use **Test** or **Test and enable** — that actually signs in, so captcha and permissions are checked. Disabled profiles are ignored by analysis and Excel fill.

List order is store ownership. If two enabled profiles can see the same store, the one higher in the list wins. Drag, or use move up / move down.

Passwords go in Windows Credential Manager. Cookies are DPAPI-encrypted, then written under the user's app data. `profiles.json` is display metadata only. Workbook previews, sales figures, and analysis plans stay in memory. Deleting a profile deletes its password and cookies too.

### Sales analysis

Pick one enabled multi-store profile, tick stores, choose a month comparison or a date range, then **Analyze**. Job concurrency is in Settings (default 32, max 32).

A month run loads five periods: current, previous, two periods ago, same month last year, and the following month last year. If the selected month is still in progress, the first four periods stop on today's day-of-month (or the last day of a short month). Last year's following month is always the full month.

After the query you can switch overview, focus, categories, products, and store comparison. Product and category numbers follow Article View. Whole-store transactions and basket value come from Trend View (`net sales / transaction count`). They are hidden when a category or product filter is on, because Trend View has no number at that grain.

**Export store PDFs** asks for a folder and writes one landscape report per successful store. A numeric suffix is added if the name already exists.

### Excel fill

Default sheet name is `Dairly` (that spelling is in the real workbook):

- `C` — business store id
- `F` — calendar date
- `L` — that day's Trend View gross sales (sales less returns)
- `AB` — that day's Trend View transaction count

![Range](release/excel-range-desktop.png)

Open or drop an `.xlsx`, pick the sheet and inclusive dates, check the scan counts. The ceiling is 2,000 unique date/store jobs unless you change it in Settings. **Analyze** queries each pair once, one calendar day per request.

**Save as** always writes a new file. The source is never overwritten. Before writing, the app checks SHA-256, size, and mtime; if you touched the source, it refuses.

Different existing values need **Overwrite existing values**. Formula cells in `L` or `AB` are left alone. If you cancel after a plan exists, or a temporary job dies after retries, use **Retry failed/pending**. If you cancelled during login or store load, there is no plan yet — run **Analyze** again. An incomplete plan cannot be written.

Partial write (skip issue rows, keep their original values) is only offered after a finished analysis, and it asks for confirmation.

If column `C` is not an RTA business id, turn on the private JSON/CSV mapping in Settings. Do not commit a file that contains real store codes.

![Preview](release/excel-results-desktop.png)

### Settings you will actually touch

Concurrency defaults to 32. Max jobs per run defaults to 2,000. Optional local mapping. Those apply to both analysis and Excel fill.

## Using the CLI

Put this in a repo-root `.env` (Git ignores it):

```dotenv
RTA_ACCOUNT=your-account
RTA_PASSWORD=your-password
RTA_COOKIE_FILE=.rta-sales.cookies.json
```

Dry-run first (no `-write`):

```powershell
go run ./cmd/rta-xlsx-fill -input "C:\path\source.xlsx" -date 2026-08-13
```

A range is `-from` and `-to`, not combined with `-date`. After a clean dry-run:

```powershell
go run ./cmd/rta-xlsx-fill `
  -input "C:\path\source.xlsx" `
  -output "C:\path\source.filled.xlsx" `
  -date 2026-08-13 `
  -write
```

Other flags worth knowing: `-sheet` (default `Dairly`), `-overwrite`, `-allow-partial`, `-max-jobs` (2000), `-concurrency` (32), `-mapping`, `-timeout 20m`. `-row` is diagnostic and cannot be used with `-write`.

Stdout is a JSON report. `matched_rows` are rows for the date, `selected_rows` are ones this account may query, `skipped_store_rows` belong to other accounts. If nothing matches an authorized store, the command fails instead of writing an unchanged book. The report has row numbers and issue codes, not passwords or amounts.

## Using the library

Go 1.25 or newer:

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

Keep credentials in the environment. One `Client` is one account. It logs in on the first request and refreshes an expired session.

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

Select stores by the exact `BusinessID` from `Stores`. Dates are the calendar y/m/d on the `time.Time`; there is no timezone conversion. Call `RefreshStores` if permissions may have changed.

`SalesQuery` also accepts `ItemCodes` and `SkipTrend`. `TotalAmount` is Article View (filterable). `TrendGrossSaleAmount` and `TotalTransactionCount` are whole-store Trend View values, summed across the inclusive range.

Workbook filling is two steps: `xlsxfill.Analyze` never writes, `RetryFailed` resumes a cancelled or flaky run, `Apply` writes only a complete plan whose source file has not changed. Transport / 408 / 429 / 5xx retries twice (1s, 3s). Missing data, auth, mapping, and format errors do not retry.

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

`PageConcurrency` defaults to 4, `LoginAttempts` to 4 (max 10). `CookieStore` and `CookieFile` cannot both be set. Use a separate client and cookie path per account.

The embedded OCR is CPU-only. Uncertain glyphs are not submitted; the client asks for a new captcha or the next solver (`NewTwoCaptchaSolver` if you want a remote fallback). Typed errors work with `errors.As`. A failed page fails the whole sales call.

## Development setup

Desktop work is Windows-only. Library tests also run on the Linux CI runners.

Install:

- Go 1.25+ (CI uses 1.25.12 and 1.26.6)
- [Bun 1.3.14](https://bun.sh) — the frontend is pinned to Bun, not npm
- Git and PowerShell
- WebView2, or the desktop window will not start
- NSIS 3.12 only if you want the installer: `choco install nsis --version 3.12.0`

Wails is pinned at `v2.14.0`. You do not have to install it yourself; the scripts `go run` that version. To install it globally:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0
```

No GPU, CGO, or Tesseract. Tests use synthetic images and a local HTTP fixture. They never call RTA.

Layout: repo root is the library; `cmd/rta-excel-filler` is the desktop entry; `desktop/frontend` is Svelte; `cmd/rta-xlsx-fill` is the CLI; `scripts` is the local tooling.

## Build from source

Clone, then work from the repo root.

```powershell
git clone https://github.com/Miku0139oao/rta-sales-client-go.git
cd rta-sales-client-go

cd desktop\frontend
bun install --frozen-lockfile
cd ..\..
```

Checks before you compile:

```powershell
./scripts/verify.ps1
```

That is Go test (including race), vet, build, then the frontend lint / typecheck / Vitest / `vite build`. Faster loop:

```powershell
./scripts/verify.ps1 -SkipRace
```

CLI only:

```powershell
go build -o rta-xlsx-fill.exe ./cmd/rta-xlsx-fill
.\rta-xlsx-fill.exe -input "C:\path\source.xlsx" -date 2026-08-13
```

`go run ./cmd/rta-xlsx-fill ...` is fine too.

Desktop with hot reload (UI reloads; Go binding changes restart the window):

```powershell
./scripts/dev.ps1
```

Ship an exe. Portable only, if you do not have NSIS:

```powershell
./scripts/build-desktop.ps1 -SkipInstaller
```

Portable plus installer:

```powershell
./scripts/build-desktop.ps1
```

Output lands in `release\`. The script builds the frontend first (and empties `dist` so old hashed files are not embedded), then runs the pinned Wails CLI twice: portable fails loudly without WebView2; the installer may download it.

Change the product name or version in `cmd/rta-excel-filler/wails.json`. Editing only `build/windows/info.json` is pointless — the next `wails build` overwrites it.

CI tests on Ubuntu, then builds on Windows with the same `build-desktop.ps1`. Local verify does not package the app or run `govulncheck`.

## Do not commit

`.env`, cookies, populated mappings, `*.filled.xlsx`, `cmd/rta-excel-filler/build/bin/`. Generated Wails bindings under `desktop/frontend/src/lib/wails/` are ignored too.

Do not log cookies, passwords, full upstream bodies, or `SaleItem.Raw`.
