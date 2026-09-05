// Run against `bun run dev:web --host 127.0.0.1 --port 5179`.
// Requires an installed Playwright (or PLAYWRIGHT_MODULE=file:///.../index.js).
// Uses synthetic local snapshots only; API requests are blocked.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const playwright = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { chromium } = playwright.default ?? playwright;
const origin = process.env.UI_ORIGIN || 'http://127.0.0.1:5179';
const output = resolve(process.env.UI_OUTPUT || 'node_modules/.cache/analysis-ui');
await mkdir(output, { recursive: true });
const totals = { saleQuantity: 16400, saleAmount: 1284500, returnQuantity: 120, returnAmount: 8400, netQuantity: 16280, netSalesAmount: 1276100, transactionCount: 6720 };
const stores = ['107', '108'].map((businessId, index) => ({ businessId, label: `${businessId} · ${index ? '銅鑼灣' : '中環'}門店`, totals }));
const items = Array.from({ length: 120 }, (_, index) => ({
  storeId: '107', storeLabel: stores[0].label, articleCode: String(552000 + index),
  articleName: ['保濕修護精華液 50ml', '深層潔淨洗面乳 120g', '維他命營養補充裝 60片', '舒緩保濕面膜 10片'][index % 4] + ` · ${index + 1}`,
  brandName: 'RTA SELECT', category1: '健康與美容', category2: ['肌膚護理', '個人護理', '健康食品', '日常用品'][index % 4],
  category2Code: `A0${index % 4 + 1}`, category3: '日常護理', category4: '人氣商品', category5: '標準裝',
  transactionCount: 100, saleQuantity: 300 - index, saleAmount: 24000 - index * 120,
  returnQuantity: 1, returnAmount: 30, returnTransactionCount: 1, netQuantity: 299 - index, netSalesAmount: 23970 - index * 120,
}));
const periods = [
  ['current', '本期', '2026-08-01', '2026-08-31', 1],
  ['previous', '前期', '2026-07-01', '2026-07-31', .9],
  ['previous2', '前二期', '2026-06-01', '2026-06-30', .8],
  ['yearAgo', '去年同期', '2025-08-01', '2025-08-31', .87],
  ['yearAgoNext', '去年下月', '2025-09-01', '2025-09-30', .93],
].map(([key, label, from, to, scale]) => ({
  key, label, from, to, complete: true, successfulStores: 2,
  totals: { ...totals, netSalesAmount: totals.netSalesAmount * scale }, stores,
  items: items.map((item) => ({ ...item, netSalesAmount: item.netSalesAmount * scale })),
}));
const analysis = { operationId: 'local-ui-fixture', from: '2026-08-01', to: '2026-08-31', complete: true, pending: false, selectedStores: 2, successfulStores: 2, totals, stores, periods, weeks: [], queryDurationMs: 1234 };
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
    const errors = [];
    const requests = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.route('**/api/**', (route) => {
      if (new URL(route.request().url()).pathname === '/api/events') {
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': local fixture\n\n' });
      }
      requests.push(route.request().url());
      return route.abort();
    });
    await page.addInitScript(({ analysis, theme }) => {
      localStorage.setItem('rta-sales-web-v1', JSON.stringify({ profiles: [], secrets: {}, manCodeGroups: [{ id: 'featured', name: '重點商品', codes: ['552001', '552002'] }], analysis, articleNames: {} }));
      localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'zh-TW', theme, rankingLimit: 24 }));
    }, { analysis, theme: scenario.theme });
    await page.goto(origin);
    await page.getByText('我已知曉', { exact: true }).click();
    await page.getByRole('heading', { name: '銷售額 Top 24', exact: true }).waitFor();
    await page.evaluate(() => document.fonts.ready);
    const noOverflow = async () => {
      const widths = await page.locator('main').evaluate((element) => [element.scrollWidth, element.clientWidth]);
      assert.ok(widths[0] <= widths[1] + 1, `${scenario.name}: horizontal overflow ${widths}`);
    };
    await noOverflow();
    await page.screenshot({ path: resolve(output, `${scenario.name}-overview.png`) });
    assert.equal(await page.locator('.top-card').first().locator('li').count(), 24);
    await page.locator('.period-disclosure summary').click();
    assert.ok((await page.locator('.period-summary').innerText()).includes('2025-08-01'));
    await page.locator('.period-disclosure summary').click();
    await page.locator('.performance-summary').click();
    assert.equal(await page.locator('.performance-card table').isVisible(), true);
    await noOverflow();
    await page.locator('.performance-summary').click();

    await page.getByRole('button', { name: '篩選', exact: true }).click();
    assert.equal(await page.locator('#analysis-filter-panel').isVisible(), true);
    const facet = page.locator('.facet-menu').nth(1);
    await facet.locator('summary').click();
    await facet.getByRole('textbox').fill('肌膚');
    const popover = await facet.locator('.facet-popover').boundingBox();
    assert.ok(popover.x >= 0 && popover.x + popover.width <= scenario.width + 1, 'Facet must fit viewport');
    await facet.getByRole('button', { name: '全選搜尋結果', exact: true }).click();
    assert.equal(await facet.getByRole('checkbox').isChecked(), true);
    await facet.getByRole('textbox').fill('個人');
    await facet.getByRole('button', { name: '全選搜尋結果', exact: true }).click();
    await facet.getByRole('button', { name: '取消選取搜尋結果', exact: true }).click();
    await facet.getByRole('textbox').fill('肌膚');
    assert.equal(await facet.getByRole('checkbox').isChecked(), true, 'Bulk actions must preserve hidden selections');
    await facet.getByRole('textbox').press('Escape');
    assert.equal(await facet.getAttribute('open'), null);
    await noOverflow();
    await page.screenshot({ path: resolve(output, `${scenario.name}-filters.png`) });
    await page.getByRole('button', { name: '清除全部篩選' }).click();
    await page.locator('.filter-toggle').click();

    const custom = page.getByRole('spinbutton', { name: '自訂' });
    await custom.fill('100');
    await custom.press('Enter');
    await page.getByRole('heading', { name: '銷售額 Top 100', exact: true }).waitFor();
    assert.equal(await page.locator('.top-card').first().locator('li').count(), 100);
    assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('rta-sales-desktop-settings-v2')).rankingLimit), 100);
    assert.equal(await page.locator('.top-card ol').first().evaluate((element) => element.scrollHeight > element.clientHeight + 1), false, 'Rankings must not have nested scrollbars');
    await page.locator('main').evaluate((element) => { element.scrollTop = 650; });
    const nav = await page.locator('.report-navigation').boundingBox();
    const main = await page.locator('main').boundingBox();
    const inset = await page.locator('main').evaluate((element) => parseFloat(getComputedStyle(element).paddingTop));
    assert.ok(nav.y >= main.y - 1 && nav.y <= main.y + inset + 2, `Navigation must remain pinned: ${scenario.name} ${JSON.stringify({ nav, main, inset })}`);
    await page.screenshot({ path: resolve(output, `${scenario.name}-scrolled.png`) });
    await page.getByRole('button', { name: '回到搜尋與篩選', exact: true }).click();
    await page.waitForFunction(() => {
      const input = document.querySelector('.analysis-search input');
      const rect = input.getBoundingClientRect();
      const nav = document.querySelector('.report-navigation').getBoundingClientRect();
      return document.activeElement === input && rect.top >= nav.bottom && rect.bottom < innerHeight;
    });
    await page.screenshot({ path: resolve(output, `${scenario.name}-return-to-filters.png`) });
    await page.locator('main').evaluate((element) => { element.scrollTop = 650; });
    await page.getByRole('tab', { name: '分類', exact: true }).click();
    await page.waitForFunction(() => {
      const panel = document.querySelector('.analysis-workspace').getBoundingClientRect();
      const nav = document.querySelector('.report-navigation').getBoundingClientRect();
      return panel.top >= nav.bottom - 1;
    });
    await noOverflow();
    assert.equal(await page.locator('.ranking-group ol').first().locator('li').count(), 30);
    await page.locator('main').evaluate((element) => { element.scrollTop = 0; });
    await page.screenshot({ path: resolve(output, `${scenario.name}-categories.png`) });
    await page.getByRole('tab', { name: '商品', exact: true }).click();
    await noOverflow();
    await page.getByRole('tab', { name: '門店', exact: true }).click();
    await noOverflow();

    await page.locator('.analysis-heading-actions md-filled-button').click();
    const dialog = page.getByRole('dialog');
    await dialog.waitFor();
    assert.equal(await dialog.getByRole('spinbutton', { name: '自訂' }).inputValue(), '100');
    const bounds = await dialog.boundingBox();
    assert.ok(bounds.x >= 0 && bounds.y >= 0 && bounds.x + bounds.width <= scenario.width + 1 && bounds.y + bounds.height <= scenario.height + 1, 'Export dialog must fit viewport');
    await page.screenshot({ path: resolve(output, `${scenario.name}-export.png`) });
    await page.keyboard.press('Escape');
    assert.equal(await dialog.count(), 0);
    await page.locator('.analysis-heading-actions md-outlined-button').click();
    await page.locator('input[type="date"]').first().fill('2026-07-01');
    await page.getByRole('button', { name: '放棄條件變更', exact: true }).click();
    assert.equal(await page.locator('input[type="date"]').first().inputValue(), '2026-08-01');
    assert.equal(await page.getByText('條件已變更，尚未重新分析', { exact: true }).count(), 0);
    await noOverflow();
    assert.deepEqual(errors, []);
    assert.deepEqual(requests, [], 'UI verification must not call the API');
    results.push({ ...scenario, passed: true });
    await page.close();
  }
} finally {
  await browser.close();
}
await writeFile(resolve(output, 'results.json'), JSON.stringify(results, null, 2));
console.log(JSON.stringify({ output, results }, null, 2));
