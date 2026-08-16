# RTA 銷售分析 (RTA Sales Analyzer)

English | [繁體中文](README.zh-TW.md)

A **Windows desktop app** to review stores, export a meeting PDF, and fill the company Excel file.  
After install you only need the mouse. **No programming and no command line.**

It can:

1. **Sign in to RTA** and solve the captcha for you
2. **Show store sales**, compared with last month and the same month last year
3. **Export PDF reports** (one combined file plus one file per store, for meetings)
4. **Fill today's sales amount and transaction count** into the company's existing Excel workbook

The Start menu and shortcut name is **RTA 銷售分析**.  
The installer is still called `RTA-Excel-Filler-setup.exe`. That is the old filename; it is the same app.

---

## What you need

- **64-bit Windows 10 or 11**
- A network connection that can reach RTA
- One or more RTA accounts that can see the stores you care about

Most people should use the installer. Windows 10/11 normally provides WebView2 with Edge; on the few PCs missing the runtime, the installer downloads it.

---

## Install

1. Download the latest files from [Releases](https://github.com/Miku0139oao/rta-sales-client-go/releases).
2. Run **`RTA-Excel-Filler-setup.exe`**.
3. Open the Start menu, search for **RTA 銷售分析**, and launch it.

Only one window can run at a time. If a click seems to do nothing, check the taskbar — it may already be open.

A two-minute silent walkthrough with captions:

[Usage tutorial](https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.4.1/RTA-Sales-Analyzer-tutorial.mp4)

### The other files

| File | When to use it |
| --- | --- |
| `RTA-Excel-Filler-setup.exe` | **Use this** |
| `RTA-Excel-Filler-portable.exe` | No installer; double-click to run. Windows 10/11 normally provides WebView2 with Edge; if it does not start, install [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/) or use the installer |
| `SHA256SUMS.txt` | For IT to check the file was not altered. Everyday users can ignore it |

---

## First launch: add an account

Open **Accounts** on the left, then **Add account**.

Fill in:

- **Display name** — a nickname for yourself, such as “Main” or “North”
- **Account** — the RTA login
- **Password** — the RTA password

Prefer **Test and enable**. The app really signs in, so captcha and store access are checked.  
Disabled accounts are ignored by Sales analysis and Excel fill.

![Accounts](release/account-pool-desktop-verified.png)

### If you have more than one account

The list order is priority, top to bottom.  
If two enabled accounts can see the same store, the one higher in the list is used.

Use **Move up** / **Move down** on each row to change the order.

Passwords go in Windows Credential Manager. They are not stored as plain text.  
Deleting an account also deletes its password.

---

## Sales analysis

Use this to review a month, compare it with last year, and export PDFs.

1. Open **Sales analysis**.
2. Pick an enabled account.
3. Tick the stores (Select all is fine).
4. Choose a period:
   - **Month comparison** (usual choice): pick one month
   - **Date range**: set your own start and end dates
5. Click **Run analysis** and wait.

![Sales analysis overview](release/sales-analysis-overview.png)

### What a month comparison includes

If you pick August, the app loads five periods:

| Label | Meaning |
| --- | --- |
| Current | The month you picked |
| Previous | The month before that |
| Two periods ago | The month before previous |
| Same month last year | August last year |
| Following month last year | September last year (useful when planning what to restock or promote next) |

If the selected month is still in progress, the first four periods stop on today's day-of-month, so the comparison stays fair.  
On 16 August, previous stops on 16 July, and last year stops on 16 August last year.  
Last year's following month is always the full month.

### After the query

- **Overview** — net sales, sales amount, returns, transactions, basket value, top products
- **Weekly** — how each week and store moved inside the current period
- **Focus** — products that sold well in last year's following month
- **Categories** — how each category moved across periods
- **Products** — line items; narrow them with the category filters or search
- **Store comparison** — each store's current, previous, and year-ago totals

Transactions and basket value are whole-store figures.  
If you filter to one category or one product, those two numbers are hidden, because the source has no figure at that grain.

### Export PDFs

Click **Export PDFs (combined + stores)** and pick a folder.

You get:

- 1 combined report for every successful store
- 1 report per successful store

The success banner shows the folder. Click **Open folder** to jump there.  
If a file name already exists, a number is added. **Old files are not overwritten.**

---

## Fill numbers into Excel

Use this for the company's daily workbook.  
The app only writes two columns, and it **always saves a new file. The original Excel file is never changed.**

The default sheet name is `Dairly` (that spelling is in the real workbook):

| Column | Contents |
| --- | --- |
| C | Store id |
| F | Date |
| L | That day's sales amount (sales less returns) |
| AB | That day's transaction count |

### Three steps

**1. Select the range**

- Click **Open Excel file**, or drop an `.xlsx` onto the window (`.xls` is not supported)
- Pick the sheet and the inclusive dates
- Check the scan summary: store count, date count, and available accounts
- Click **Start analysis**

![Select Excel range](release/excel-range-desktop.png)

**2. Review the results**

Each row is marked:

- **Will change** — a new number will be written
- **Unchanged** — already matches RTA
- **Issue / Query failed** — this row will not be written until you fix or skip it

![Review Excel results](release/excel-results-desktop.png)

**3. Save a copy**

When the preview looks right, click **Save and write** and choose where the new file should go.  
The success card shows the **file name** and **folder** separately.  
Click **Open file** to open it in Excel, or **Show in folder**.

![Save finished](release/excel-success-desktop.png)

### Before you write

- **The source file is never overwritten.**
- If a cell already has a different number, turn on **Overwrite all differing values**, or those rows stay as issues.
- Formula cells are left alone.
- If some queries fail, use **Retry failed items**.
- If you cancelled before login finished, run **Start analysis** again.
- An unfinished analysis cannot be saved.
- To skip issue rows and write the rest: wait until analysis has finished, then turn on **Skip all issue rows and write the rest**. The app asks for confirmation.

If column C is not an RTA store id, turn on the local mapping file in Settings (JSON or CSV). Do not share a mapping that contains real store codes.

---

## Settings you may actually change

Open **Settings**.

| Setting | Typical use |
| --- | --- |
| Theme and language | Switch to Traditional Chinese, or change light / dark. **Applies immediately** — no Save click |
| Max jobs per run | Default 2,000. A very wide date range times many stores will stop here |
| Query concurrency | Default 160, for one account querying many stores at once. If the network is flaky, try 32 or 8 |
| Local mapping | Only if Excel store codes are not RTA store ids |

After changing workload or mapping, click **Save**.

---

## Common questions

**The app will not start, or the window flashes and disappears**  
Windows 10/11 normally already has Edge WebView2. If the portable build still does not start, use `RTA-Excel-Filler-setup.exe`; it handles the uncommon case where the runtime is missing.

**A second window closes immediately**  
That is expected. Only one copy can run.

**Account test keeps failing on captcha**  
Wait a minute and test again. If it still fails, check the password, and try signing in to RTA in a browser.

**Sales analysis says there are no stores**  
On Accounts, confirm the profile is enabled and the last test succeeded. Some logins genuinely have no store access.

**Many Excel rows say the store could not be mapped**  
Column C does not match an RTA store id. Fix the workbook, or add a mapping file in Settings.

**Save says the source file changed**  
Do not edit the source workbook in Excel after analysis starts. Scan and analyze again.

**After uninstall, the accounts are still there on the next install**  
Uninstall keeps accounts on purpose. To remove them, delete each profile in the Accounts page first.

**Are passwords stored safely?**  
They are kept by Windows Credential Manager. Sales figures and Excel previews stay in memory for this session only.

---

## Where account data lives

Saved accounts and encrypted login state are here (the folder still uses the old product name, so existing data still loads):

`C:\Users\<your user name>\AppData\Roaming\RTA Excel Filler`

You do not need to edit this folder. To wipe a PC completely: delete every account in the app, then delete the folder.

---

## For developers

CLI, Go library, and building from source are in [DEVELOPMENT.md](DEVELOPMENT.md).
