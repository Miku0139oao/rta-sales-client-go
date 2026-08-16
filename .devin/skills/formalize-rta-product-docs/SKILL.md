# Formalize RTA Product Documentation

Use this skill when the user wants to rewrite, formalize, or standardize the README and/or release notes for the `rta-sales-client-go` project.

## Product Facts to Preserve

Read these build/config files to confirm product metadata before editing:
- `cmd/rta-excel-filler/wails.json` — `productName` and `productVersion`
- `cmd/rta-excel-filler/build/windows/info.json` — Windows product metadata
- `go.mod` — module / repository path

Do not change the following facts:
- Canonical product name: **RTA 銷售分析**
- English alias: **RTA Sales Analyzer**
- Installer filename: `RTA-Excel-Filler-setup.exe`
- Portable filename: `RTA-Excel-Filler-portable.exe`
- Data folder: `RTA Excel Filler`
- Default Excel sheet: `Dairly`
- Excel columns: C (store id), F (date), L (daily sales amount), AB (transaction count)

## Files to Edit

- `README.md` — English user documentation
- `README.zh-TW.md` — Traditional Chinese user documentation
- `release-ci-<version>/RELEASE-NOTES.md` — versioned release notes

## Steps

1. Read the existing doc files and the relevant `DEVELOPMENT.md` / build configs to verify technical facts.
2. Rewrite in a formal, professional, and easy-to-read tone. Keep all technical details, file names, and section structure. Do not add emojis.
3. Preserve the Chinese usage-doc test strings in `README.zh-TW.md` so `go test ./cmd/rta-excel-filler` still passes.
4. If image links are broken because the screenshots live in the `.gitignore`d `release/` folder:
   - Copy the referenced PNGs to `docs/images/`
   - Update image references from `](release/` to `](docs/images/`
   - Do not commit `.exe`, `.mp4`, or `SHA256SUMS.txt` from `release/` or `release-ci-*` unless explicitly asked.
5. Run `go test ./cmd/rta-excel-filler` to confirm the usage-doc test still passes.
6. Commit and push only the intended doc/image files.

## Delegating to Grok via Paseo

If the user explicitly asks for Grok:
1. Verify the Paseo daemon is reachable and Grok is available:
   ```powershell
   paseo provider ls --host "tcp://localhost:6767?password=<PASEO_PASSWORD>"
   ```
2. Launch the Paseo agent with the full task prompt, including all product facts and constraints:
   ```powershell
   paseo run --provider grok --title "[Subagent] Formalize docs" --cwd <repo-root> --host "tcp://localhost:6767?password=<PASEO_PASSWORD>" --wait-timeout 5m "<prompt>"
   ```
3. Allow `edit` and `execute` permission requests as they appear, or pre-allow all for the agent when appropriate.
4. After the agent returns, review the changes critically, fix any remaining image or naming issues, run the Go test, and commit.

## Constraints

- Do not modify application code, build configs, or `.gitignore` unless asked.
- Do not rename product executables, the Start-menu name, or the data folder.
- Keep English and Chinese versions aligned in tone and structure.
