// All APIs are intercepted. Only local synthetic workbooks are generated/read by workbook-fixture.go.
// Build helper (repo root): go build -o desktop/frontend/node_modules/.cache/workbook-fixture.exe desktop/frontend/scripts/workbook-fixture.go
// Start frontend: bun run dev:web --host 127.0.0.1 --port 5179
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
const playwright = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { chromium } = playwright.default ?? playwright;
const origin = process.env.UI_ORIGIN || 'http://127.0.0.1:5179';
const output = resolve(process.env.UI_OUTPUT || 'node_modules/.cache/analysis-tools');
const helper = resolve(process.env.WORKBOOK_FIXTURE || 'node_modules/.cache/workbook-fixture.exe');
await mkdir(output, { recursive: true });
function workbook(args, input = '') {
  return new Promise((resolve, reject) => {
    const child = execFile(helper, args, { maxBuffer: 16 * 1024 * 1024, windowsHide: true }, (error, stdout) => error ? reject(error) : resolve(stdout));
    child.stdin.end(input);
  });
}
const profileId = '11111111-1111-4111-8111-111111111111';
const totals = { saleQuantity: 200, saleAmount: 28000, returnQuantity: 0, returnAmount: 0, netQuantity: 200, netSalesAmount: 28000 };
const stores = ['107', '108', '109'].map(businessId => ({ businessId, label: `${businessId} · 本機測試門店`, totals }));
const items = Array.from({ length: 72 }, (_, index) => ({
  storeId: '107', storeLabel: stores[0].label, articleCode: `00${100 + index}`, articleName: `保濕修護精華液與日常肌膚護理 ${index + 1}`, brandName: '本機合成品牌',
  category1: '健康與美容', category2: '肌膚護理', category2Code: 'A02', category3: '日常護理', category4: '人氣商品', category5: '標準裝',
  transactionCount: 2, returnTransactionCount: 0, saleQuantity: index + 1, saleAmount: (73 - index) * 100, returnQuantity: 0, returnAmount: 0, netQuantity: index + 1, netSalesAmount: (73 - index) * 100,
}));
items.push({ ...items[0], storeId: '108', storeLabel: stores[1].label, netSalesAmount: 200, saleAmount: 200, netQuantity: 2, saleQuantity: 2 });
const analysis = { operationId: 'local-tools-fixture', from: '2026-08-01', to: '2026-08-31', complete: true, pending: false, selectedStores: 3, successfulStores: 3, totals, stores, weeks: [], queryDurationMs: 42,
  periods: ['current', 'previous'].map((key, index) => ({ key, label: index ? '上期' : '本期', from: index ? '2026-07-01' : '2026-08-01', to: index ? '2026-07-31' : '2026-08-31', complete: true, successfulStores: 3, totals, stores,
    items: items.map(item => ({ ...item, netSalesAmount: item.netSalesAmount / (index + 1), saleAmount: item.saleAmount / (index + 1) })) })) };
const browser = await chromium.launch({ headless: true, ...(process.env.UI_BROWSER ? { channel: process.env.UI_BROWSER } : {}) });
const results = [];
try {
  for (const scenario of [
    { name: 'desktop', width: 1440, height: 1000, theme: 'light' },
    { name: 'laptop', width: 1024, height: 768, theme: 'light' },
    { name: 'narrow', width: 390, height: 844, theme: 'light' },
    { name: 'dark', width: 1440, height: 1000, theme: 'dark' },
  ]) {
    const page = await browser.newPage({ viewport: { width: scenario.width, height: scenario.height }, acceptDownloads: true });
    const errors = [], unexpected = [], calls = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/events') return route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': synthetic\n\n' });
      if (path === '/api/session') return route.fulfill({ json: {} });
      if (path === '/api/rpc') {
        const { method, args } = route.request().postDataJSON(); calls.push({ method, args });
        if (method === 'BuildSalesAnalysisWorkbook') return route.fulfill({ json: { result: await workbook([], JSON.stringify(args[0])) } });
        if (method === 'ListSalesAnalysisStores') return route.fulfill({ json: { result: stores } });
        if (method === 'CancelSalesAnalysis' || method === 'ClearSalesAnalysis') return route.fulfill({ json: { result: null } });
        if (method === 'RunSalesAnalysis') return route.fulfill({ json: { result: { ...analysis, operationId: 'explicit-synthetic-run' } } });
      }
      unexpected.push(route.request().url()); return route.abort();
    });
    await page.addInitScript(({ analysis, profileId, theme }) => {
      localStorage.setItem('rta-sales-web-v1', JSON.stringify({ profiles: [{ id: profileId, displayName: '本機測試帳號', enabled: true, priority: 1, hasCredentials: true }], secrets: {}, manCodeGroups: [], analysis, articleNames: {} }));
      localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'zh-TW', theme, rankingLimit: 24 }));
      window.__copied = '';
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async text => { window.__copied = text; } } });
    }, { analysis, profileId, theme: scenario.theme });
    await page.goto(origin);
    await page.getByText('我已知曉', { exact: true }).click();
    await page.getByRole('heading', { name: '銷售額 Top 24', exact: true }).waitFor();
    await page.evaluate(() => document.fonts.ready);
    const insights = page.getByRole('region', { name: '分析重點', exact: true });
    await insights.getByRole('heading', { name: '淨銷售成長最多', exact: true }).waitFor();
    await insights.getByRole('button', { name: /^追查分析重點/ }).first().click();
    await page.getByRole('dialog').waitFor();
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.activeElement?.classList.contains('insight-link'));
    await insights.screenshot({ path: resolve(output, `${scenario.name}-insights.png`) });
    assert.equal(calls.filter(call => call.method === 'RunSalesAnalysis').length, 0);
    await page.getByRole('tab', { name: '商品', exact: true }).click();
    const table = page.getByRole('tabpanel').getByRole('table');
    await table.getByRole('columnheader', { name: '淨銷售數量' }).getByRole('button').click();
    assert.equal(await table.locator('tbody tr').count(), 50);
    assert.ok((await table.locator('tbody tr').first().innerText()).includes('00171'));
    await page.getByRole('button', { name: '下一頁', exact: true }).click();
    assert.equal(await table.locator('tbody tr').count(), 23);
    await page.getByRole('button', { name: '複製表格', exact: true }).click();
    const copied = await page.evaluate(() => window.__copied);
    assert.equal(copied.split('\r\n').length, 74, 'Copy must include every page');
    const downloadPromise = page.waitForEvent('download');
    await page.getByRole('button', { name: '匯出此頁 Excel', exact: true }).click();
    const download = await downloadPromise;
    const filename = resolve(output, `${scenario.name}-products.xlsx`); await download.saveAs(filename);
    const exported = JSON.parse(await workbook([filename]));
    const productRows = Object.entries(exported).find(([name]) => name !== 'Report')[1];
    assert.equal(productRows.length, 74); assert.equal(productRows[1][2], '00171');
    assert.equal(Number(productRows[1][7]), 72);
    const request = calls.find(call => call.method === 'BuildSalesAnalysisWorkbook').args[0];
    assert.equal(request.sheets[0].rows.length, 73);
    await page.getByRole('textbox', { name: '搜尋商品或編碼' }).fill('00100');
    assert.equal(await table.locator('tbody tr').count(), 2);
    await page.locator('.table-scroll').evaluateAll(elements => elements.forEach(element => { element.scrollLeft = 0; }));
    await page.locator('main').evaluate(element => { element.scrollTop = 0; });
    await page.screenshot({ path: resolve(output, `${scenario.name}-products.png`) });
    const product = page.getByRole('tabpanel').getByRole('button', { name: '查看商品詳情：保濕修護精華液與日常肌膚護理 1', exact: true }).first();
    await product.click();
    const dialog = page.getByRole('dialog', { name: '保濕修護精華液與日常肌膚護理 1', exact: true });
    await dialog.waitFor();
    await dialog.getByLabel('門店明細期間').selectOption('previous');
    const detailDownloadPromise = page.waitForEvent('download');
    await dialog.getByRole('button', { name: '匯出此頁 Excel', exact: true }).click();
    const detailDownload = await detailDownloadPromise;
    await detailDownload.saveAs(resolve(output, `${scenario.name}-product-detail.xlsx`));
    const detailRequest = calls.filter(call => call.method === 'BuildSalesAnalysisWorkbook').at(-1).args[0];
    assert.deepEqual(detailRequest.sheets[1].rows.map(row => row[2]), [3650, 100, 0]);
    const box = await dialog.boundingBox();
    assert.ok(box.x >= 0 && box.y >= 0 && box.x + box.width <= scenario.width + 1 && box.y + box.height <= scenario.height + 1);
    assert.equal(await dialog.evaluate(element => element.scrollWidth > element.clientWidth + 1), false);
    await page.screenshot({ path: resolve(output, `${scenario.name}-product-detail.png`) });
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.activeElement?.classList.contains('product-link'));
    assert.equal(calls.filter(call => call.method === 'RunSalesAnalysis').length, 0);
    await page.getByRole('button', { name: '常用條件', exact: true }).click();
    const presets = page.getByRole('dialog', { name: '常用條件', exact: true });
    await presets.getByRole('textbox', { name: '條件名稱', exact: true }).fill('每月重點商品追蹤');
    await presets.getByRole('button', { name: '另存常用條件', exact: true }).click();
    await presets.getByRole('button', { name: '釘選到主頁', exact: true }).click();
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: '帶入常用條件：每月重點商品追蹤', exact: true }).click();
    await page.getByText(/已帶入「每月重點商品追蹤」/).waitFor();
    assert.equal(calls.filter(call => call.method === 'RunSalesAnalysis').length, 0);
    await page.locator('#analysis-query-form md-filled-button').click();
    await page.waitForFunction(() => JSON.parse(localStorage.getItem('rta-sales-analysis-presets-v1')).presets[0].lastUsedAt > 0);
    await page.getByRole('button', { name: '常用條件', exact: true }).click();
    await presets.getByRole('button', { name: '取消釘選', exact: true }).click();
    await page.keyboard.press('Escape');
    await page.getByRole('navigation', { name: '常用查詢捷徑', exact: true }).getByText('最近使用', { exact: true }).waitFor();
    await page.locator('main').evaluate(element => { element.scrollTop = 0; });
    await page.screenshot({ path: resolve(output, `${scenario.name}-shortcuts.png`) });
    const widths = await page.locator('main').evaluate(element => [element.scrollWidth, element.clientWidth]);
    assert.ok(widths[0] <= widths[1] + 1, 'No page-wide horizontal overflow');
    assert.deepEqual(errors, []); assert.deepEqual(unexpected, []);
    assert.equal(calls.filter(call => call.method === 'RunSalesAnalysis').length, 1);
    results.push({ ...scenario, passed: true, exportedRows: 73, explicitQueries: 1 });
    await page.close();
  }
} finally { await browser.close(); }
await writeFile(resolve(output, 'results.json'), JSON.stringify(results, null, 2));
console.log(JSON.stringify({ output, results }, null, 2));
