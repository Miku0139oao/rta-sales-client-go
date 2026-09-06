# Development

English | [繁體中文](DEVELOPMENT.zh-TW.md)

Everyday use is documented in [README.md](README.md). This file is for people changing the code, running the CLI, or importing the library.

The Windows app is **RTA 銷售分析** (RTA Sales Analyzer). The exe is still named `RTA-Excel-Filler.exe`. That filename was left alone so installers and CI artifacts did not have to move. The window title is **RTA 銷售分析**.

The same repo also has:

- a CLI, `go run ./cmd/rta-xlsx-fill`, for the workbook job without a window
- a Go library other programs can import

## Pages update metadata

The native updater reads only `https://miku0139oao.github.io/rta-sales-client-go/updates/latest.json` for version/notes; executable and checksum assets remain on GitHub Releases. No API fallback, user login, token in the app, or separate server exists. `Inspect` / `CheckForUpdate` request HTTP cache revalidation; `InspectStartup` / `CheckForUpdateStartup` use validated public metadata for at most one hour plus persisted exponential failure backoff. Both paths preserve backend candidate IDs and installation gates; web RPC rejects both.

`%APPDATA%/RTA-Excel-Filler/updates-v1.json` contains only bounded metadata/timestamps, never IDs or secrets. Cache reads are size-bound and revalidated; same-directory temporary-file replacement is best-effort. Unwritable storage falls back to in-memory state. Server retry hints cap at one hour and also apply to manual checks; cancellation does not count as failure. There are no background retry timers.

The Pages workflow checks out `main` for every docs/manual/release run, verifies the authenticated latest stable Release on Windows, and uploads the complete docs site plus a generated (uncommitted) manifest before Ubuntu deployment. It checks both GitHub SHA256 digests, exact asset URLs/sizes, the unique executable checksum, Authenticode publisher against a hash-pinned v0.4.8 reference, AMD64/numeric PE version, and re-resolves latest/asset identities after downloads. Failed verification leaves deployed Pages unchanged. Release build CI remains drafts only.

Offline validation (PowerShell 7.5+): `pwsh -NoProfile -File scripts/test-update-manifest.ps1`. The real generator is a release-artifact verification operation, not part of ordinary tests. After review and merging to main, authorized deployment is `gh workflow run pages.yml --repo Miku0139oao/rta-sales-client-go --ref main`; see [portable update deployment](docs/portable-updates.md). Existing 0.4.8 API clients require one manual upgrade; do not promise they can discover Pages automatically.

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

Other flags worth knowing: `-sheet` (default `Dairly`), `-overwrite`, `-allow-partial`, `-max-jobs` (2000), `-concurrency` (160), `-mapping`, `-timeout 20m`. `-row` is diagnostic and cannot be used with `-write`.

Stdout is a JSON report. `matched_rows` are rows for the date, `selected_rows` are ones this account may query, `skipped_store_rows` belong to other accounts. If nothing matches an authorized store, the command fails instead of writing an unchanged book. The report has row numbers and issue codes, not passwords or amounts.

Verify a portable executable checksum:

```powershell
(Get-FileHash -Algorithm SHA256 .\RTA-Excel-Filler-portable.exe).Hash.ToLowerInvariant()
Get-Content .\SHA256SUMS.txt
```

## Using the library

Go 1.25 or newer:

```bash
go get github.com/Miku0139oao/rta-sales-client-go@latest
```

Keep credentials in the environment. One `Client` is one account. It logs in on the first request and refreshes an expired session.

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
	Concurrency:             160,
})
report, err := xlsxfill.Apply(ctx, plan, xlsxfill.ApplyRequest{
	OutputPath: `C:\reports\august.filled.xlsx`,
})
```

`PageConcurrency` defaults to 16, `LoginAttempts` to 4 (max 10). `CookieStore` and `CookieFile` cannot both be set. Use a separate client and cookie path per account.

The embedded OCR is CPU-only. Uncertain glyphs are not submitted; the client asks for a new captcha or the next solver (`NewTwoCaptchaSolver` if you want a remote fallback). Typed errors work with `errors.As`. A failed page fails the whole sales call.

The solver is template matching. To train more glyphs:

```
go run ./cmd/rta-ocr-train capture -dir samples -count 60
go run ./cmd/rta-ocr-train propose -dir samples
# The file name is the label. Fix wrong names and label leftover unnamed-*.bin files.
go run ./cmd/rta-ocr-train gen -dir samples
go run ./cmd/rta-ocr-train eval -dir samples
go test ./rtasales/
```

`gen` overwrites `rtasales/embedded_ocr_trained.go`. Do not commit the sample directories.

## Development setup

Desktop work is Windows-only. Library tests also run on the Linux CI runners.

Install:

- Go 1.25+ (CI installs 1.25.13 so govulncheck matches a patched standard library)
- [Bun 1.3.14](https://bun.sh) — the frontend is pinned to Bun, not npm
- Git and PowerShell
- WebView2 (normally installed with Edge on Windows 10/11), or the desktop window will not start
- Microsoft Edge WebView2 Runtime (install manually if absent); NSIS is not required.

Wails is pinned at `v3.0.0-beta.8`. You do not have to install it yourself; the scripts `go run` that version. To install it globally:

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
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

Build the Windows portable executable only:

```powershell
./scripts/build-desktop.ps1
```

Output lands in `release\`. The script builds the frontend, injects `wails.json` productVersion into `internal/buildinfo.Version`, checks Windows resource versions, builds and signs the portable executable when signing tools are available, then hashes that exact output only. Use `-RequireSign` for signed builds. Ordinary `go build` uses `dev` and cannot update. CI stages unsigned artifacts as draft only. For explicit signed publication from a clean checkout, use `scripts/publish-portable.ps1 -Tag vX.Y.Z -Commit <full-sha> -PublisherReference <previous-trusted-portable.exe>`; it refuses historical published releases and validates downloaded draft copies before publishing. See [update safety and signed sandbox validation](docs/portable-updates.md). WebView2 installation is manual.

Change the product name or version in `cmd/rta-excel-filler/wails.json` and `cmd/rta-excel-filler/build/windows/info.json`.

CI tests on Ubuntu, then builds on Windows with the same `build-desktop.ps1`. Local verify does not package the app or run `govulncheck`.

## Do not commit

`.env`, cookies, populated mappings, `*.filled.xlsx`, `cmd/rta-excel-filler/build/bin/`. Generated Wails files under `desktop/frontend/src/lib/wails/` and `desktop/frontend/bindings/` are ignored too.

Do not log cookies, passwords, full upstream bodies, or `SaleItem.Raw`.
