// Start with: bun run dev:web --host 127.0.0.1 --port 5179
// Uses only synthetic data. Every API request is intercepted; no real backend is contacted.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
const playwright = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { chromium } = playwright.default ?? playwright;
const origin = process.env.UI_ORIGIN || 'http://127.0.0.1:5179';
const output = resolve(process.env.UI_OUTPUT || 'node_modules/.cache/analysis-presets');
await mkdir(output, { recursive: true });
const key = 'rta-sales-analysis-presets-v1';
const profileId = '11111111-1111-4111-8111-111111111111';
const totals = { saleQuantity: 200, saleAmount: 28000, returnQuantity: 0, returnAmount: 0, netQuantity: 200, netSalesAmount: 28000 };
const stores = ['107', '108'].map((businessId) => ({ businessId, label: `${businessId} · 本機測試門店`, totals }));
const items = Array.from({ length: 8 }, (_, index) => ({
  storeId: '107', storeLabel: stores[0].label, articleCode: String(552000 + index), articleName: `保濕修護精華液 ${index + 1}`,
  category1: '健康與美容', category2: '肌膚護理', category2Code: 'A02', category3: '日常護理', category4: '人氣商品', category5: '標準裝',
  transactionCount: 2, returnTransactionCount: 0, saleQuantity: 20, saleAmount: 2800, returnQuantity: 0, returnAmount: 0, netQuantity: 20, netSalesAmount: 2800,
}));
const analysis = { operationId: 'local-preset-fixture', from: '2026-08-01', to: '2026-08-31', complete: true, pending: false, selectedStores: 2, successfulStores: 2, totals, stores, weeks: [], queryDurationMs: 42,
  periods: [{ key: 'current', label: '本期', from: '2026-08-01', to: '2026-08-31', complete: true, successfulStores: 2, totals, stores, items }] };
const browser = await chromium.launch({ headless: true, ...(process.env.UI_BROWSER ? { channel: process.env.UI_BROWSER } : {}) });
const results = [];
try {
  for (const scenario of [
    { name: 'desktop', width: 1440, height: 1000, theme: 'light' },
    { name: 'laptop', width: 1024, height: 768, theme: 'light' },
    { name: 'narrow', width: 390, height: 844, theme: 'light' },
    { name: 'dark', width: 1440, height: 1000, theme: 'dark' },
  ]) {
    const page = await browser.newPage({ viewport: { width: scenario.width, height: scenario.height } });
    const errors = [], unexpected = [], calls = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.route('**/api/**', (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/events') return route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': local fixture\n\n' });
      if (path === '/api/session') return route.fulfill({ json: {} });
      if (path === '/api/rpc') {
        const { method, args } = route.request().postDataJSON();
        calls.push({ method, args });
        if (method === 'ListSalesAnalysisStores') return route.fulfill({ json: { result: stores } });
        if (method === 'ClearSalesAnalysis' || method === 'CancelSalesAnalysis') return route.fulfill({ json: { result: null } });
        if (method === 'RunSalesAnalysis') {
          const current = args[0].periods.find((period) => period.key === 'current');
          return route.fulfill({ json: { result: { ...analysis, ...current, operationId: 'local-explicit-query', periods: [{ ...analysis.periods[0], ...current }] } } });
        }
      }
      unexpected.push(route.request().url());
      return route.abort();
    });
    await page.addInitScript(({ profileId, analysis, theme }) => {
      if (!localStorage.getItem('preset-smoke-initialized')) {
        localStorage.setItem('rta-sales-web-v1', JSON.stringify({ profiles: [{ id: profileId, displayName: '本機測試帳號', enabled: true, priority: 1, hasCredentials: true }], secrets: {}, manCodeGroups: [], analysis, articleNames: {} }));
        localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'zh-TW', theme, rankingLimit: 24 }));
        localStorage.setItem('preset-smoke-initialized', '1');
      }
    }, { profileId, analysis, theme: scenario.theme });
    await page.goto(origin);
    await page.getByText('我已知曉', { exact: true }).click();
    await page.getByRole('heading', { name: '銷售額 Top 24', exact: true }).waitFor();
    await page.evaluate(() => document.fonts.ready);
    await page.getByRole('textbox', { name: '搜尋商品或編碼' }).fill('552000');
    await page.getByRole('button', { name: '常用條件', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '常用條件' });
    await dialog.getByLabel('期間規則').selectOption('previous');
    await dialog.getByRole('textbox', { name: '條件名稱', exact: true }).fill('每月門店銷售與商品追蹤');
    await dialog.getByRole('button', { name: '另存常用條件' }).click();
    await dialog.getByText('常用條件已儲存。', { exact: true }).waitFor();
    const stored = await page.evaluate((key) => JSON.parse(localStorage.getItem(key)).presets, key);
    assert.equal(stored.length, 1);
    assert.equal(stored[0].query.monthMode, 'previous');
    assert.equal(stored[0].filters.search, '552000');
    assert.equal(stored[0].query.profileId, profileId);
    assert.equal(calls.filter((call) => call.method === 'RunSalesAnalysis').length, 0);
    const bounds = await dialog.boundingBox();
    assert.ok(bounds.x >= 0 && bounds.y >= 0 && bounds.x + bounds.width <= scenario.width + 1 && bounds.y + bounds.height <= scenario.height + 1);
    assert.equal(await dialog.evaluate((element) => element.scrollWidth > element.clientWidth + 1), false);
    await page.screenshot({ path: resolve(output, `${scenario.name}-presets.png`) });
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.activeElement?.classList.contains('preset-trigger'));
    await page.getByRole('textbox', { name: '搜尋商品或編碼' }).fill('552001');
    await page.getByRole('button', { name: '常用條件', exact: true }).click();
    await dialog.getByRole('button', { name: '套用條件', exact: true }).click();
    await page.getByText(/已帶入「每月門店銷售與商品追蹤」/).waitFor();
    assert.equal(await page.getByRole('textbox', { name: '搜尋商品或編碼' }).inputValue(), '552001', 'Old report filters must remain unchanged');
    assert.equal(calls.filter((call) => call.method === 'RunSalesAnalysis').length, 0, 'Applying must never execute analysis');
    const month = await page.getByLabel('月份', { exact: true }).inputValue();
    const lastMonth = new Date(); lastMonth.setDate(1); lastMonth.setMonth(lastMonth.getMonth() - 1);
    assert.equal(month, `${lastMonth.getFullYear()}-${String(lastMonth.getMonth() + 1).padStart(2, '0')}`);
    await page.locator('main').evaluate((element) => { element.scrollTop = 0; });
    await page.screenshot({ path: resolve(output, `${scenario.name}-staged.png`) });
    const runButton = page.locator('#analysis-query-form md-filled-button');
    assert.ok((await runButton.innerText()).includes('重新分析'));
    await runButton.click();
    await page.waitForFunction(() => document.querySelector('.analysis-search input')?.value === '552000');
    assert.equal(calls.filter((call) => call.method === 'RunSalesAnalysis').length, 1);
    await page.reload();
    await page.getByRole('button', { name: '常用條件', exact: true }).click();
    assert.equal(await dialog.getByRole('combobox', { name: '選擇常用條件' }).inputValue(), stored[0].id);
    await dialog.getByRole('button', { name: '重新命名' }).click();
    await dialog.getByRole('textbox', { name: '新名稱', exact: true }).fill('每月美容分析');
    await dialog.getByRole('button', { name: '確認', exact: true }).click();
    await dialog.getByText('名稱已更新。', { exact: true }).waitFor();
    await dialog.getByRole('button', { name: '刪除', exact: true }).click();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)).presets.length, key), 1);
    await dialog.getByRole('button', { name: '刪除', exact: true }).click();
    await dialog.getByRole('button', { name: '確認', exact: true }).click();
    assert.equal(await page.evaluate((key) => JSON.parse(localStorage.getItem(key)).presets.length, key), 0);
    await page.keyboard.press('Escape');
    const widths = await page.locator('main').evaluate((element) => [element.scrollWidth, element.clientWidth]);
    assert.ok(widths[0] <= widths[1] + 1, 'Page must not overflow horizontally');
    assert.deepEqual(errors, []);
    assert.deepEqual(unexpected, []);
    results.push({ ...scenario, passed: true, explicitQueries: calls.filter((call) => call.method === 'RunSalesAnalysis').length });
    await page.close();
  }
} finally { await browser.close(); }
await writeFile(resolve(output, 'results.json'), JSON.stringify(results, null, 2));
console.log(JSON.stringify({ output, results }, null, 2));
