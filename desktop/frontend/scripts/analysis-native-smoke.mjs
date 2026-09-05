// Connects only to the explicitly built native_smoke adapter (synthetic clients,
// memory credentials, fresh profile/WebView paths, fixed sandbox export folder).
import assert from 'node:assert/strict';
import { readFile, writeFile, readdir } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { execFile } from 'node:child_process';
const playwright = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { chromium } = playwright.default ?? playwright;
const run = JSON.parse(await readFile(process.env.NATIVE_RUN || 'node_modules/.cache/native-refine-run.json', 'utf8'));
const browser = await chromium.connectOverCDP(`http://127.0.0.1:${run.port}`);
const context = browser.contexts()[0];
const page = context.pages()[0];
const errors = [];
page.on('pageerror', error => errors.push(error.message));
function inspectWorkbook(path) {
  return new Promise((resolveResult, reject) => execFile(resolve('node_modules/.cache/workbook-fixture.exe'), [path], { windowsHide: true, maxBuffer: 8 * 1024 * 1024 }, (error, stdout) => error ? reject(error) : resolveResult(JSON.parse(stdout))));
}
try {
  if (await page.getByText('我已知曉', { exact: true }).isVisible()) await page.getByText('我已知曉', { exact: true }).click();
  await page.getByText('107 - Native synthetic', { exact: true }).waitFor();
  await page.getByLabel('月份', { exact: true }).fill('2026-08');
  await page.locator('#analysis-query-form md-filled-button').click();
  await page.getByRole('heading', { name: '銷售額 Top 24', exact: true }).waitFor();
  await page.getByRole('region', { name: '分析重點' }).getByRole('button').first().waitFor();
  await page.getByText('HK$1,429.33', { exact: true }).first().waitFor();
  await page.getByRole('tab', { name: '商品', exact: true }).click();
  const table = page.getByRole('tabpanel').getByRole('table');
  await table.getByRole('columnheader', { name: '淨銷售數量' }).getByRole('button').click();
  assert.equal(await table.locator('tbody tr').count(), 50);
  assert.ok((await table.locator('tbody tr').first().innerText()).includes('00163'));
  await page.getByRole('button', { name: '匯出此頁 Excel', exact: true }).click();
  await page.getByText(/已匯出：.*\.xlsx/).waitFor();
  const directory = join(run.root, 'data', 'exports');
  let files = await readdir(directory); assert.equal(files.length, 1);
  const workbook = await inspectWorkbook(join(directory, files[0]));
  const rows = Object.entries(workbook).find(([name]) => name !== 'Report')[1];
  assert.equal(rows.length, 129); assert.equal(rows[1][2], '00163');
  const trigger = page.getByRole('tabpanel').getByRole('button', { name: /^查看商品詳情/ }).first();
  await trigger.click();
  const dialog = page.getByRole('dialog', { name: '原生合成商品 64', exact: true });
  await dialog.waitFor();
  await dialog.getByLabel('門店明細期間').selectOption('previous');
  await dialog.getByRole('button', { name: '匯出此頁 Excel', exact: true }).click();
  await dialog.getByText(/已匯出：.*\.xlsx/).waitFor();
  await page.screenshot({ path: join(run.root, 'native-product-detail.png') });
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => document.activeElement?.classList.contains('product-link'));
  await page.getByRole('tab', { name: '概覽', exact: true }).click();
  await page.locator('main').evaluate(element => { element.scrollTop = 0; });
  await page.screenshot({ path: join(run.root, 'native-overview.png') });
  await page.getByRole('button', { name: '匯出此頁 Excel', exact: true }).click();
  await page.getByText(/已匯出：.*RTA-overview.*\.xlsx/).waitFor();
  assert.deepEqual(errors, []);
  files = await readdir(directory); assert.equal(files.length, 3);
  const overview = await inspectWorkbook(join(directory, files.find(name => name.startsWith('RTA-overview'))));
  assert.ok(Object.keys(overview).some(name => name.includes('分析重點')), 'Native Excel must contain highlight evidence');
  await writeFile(join(run.root, 'results.json'), JSON.stringify({ passed: true, nativeBridge: true, syntheticOnly: true, nativeFolderPicker: 'automatically selected sandbox directory', exportedRows: 128, files, errors }, null, 2));
  console.log(JSON.stringify({ root: run.root, passed: true, exportedRows: 128, files }, null, 2));
} catch (error) {
  await page.screenshot({ path: join(run.root, 'native-failure.png') }).catch(() => undefined);
  await writeFile(join(run.root, 'failure.txt'), `${error.stack}\n\n${await page.locator('body').innerText()}`);
  throw error;
} finally { await browser.close(); }
