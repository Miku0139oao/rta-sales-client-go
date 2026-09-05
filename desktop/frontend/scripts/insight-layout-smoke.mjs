// Standalone synthetic/local insight-card layout check. Does not call live RTA.
// Start frontend: bun run dev:web --host 127.0.0.1 --port 5181
// PLAYWRIGHT_MODULE=file:///.../playwright-core/index.js UI_BROWSER=msedge UI_ORIGIN=http://127.0.0.1:5181
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const playwright = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { chromium } = playwright.default ?? playwright;
const origin = process.env.UI_ORIGIN || 'http://127.0.0.1:5181';
const output = resolve(process.env.UI_OUTPUT || 'node_modules/.cache/insight-layout');
const phase = process.env.INSIGHT_LAYOUT_PHASE || 'after';
await mkdir(output, { recursive: true });

const totals = {
  saleQuantity: 10, saleAmount: 1300, returnQuantity: 0, returnAmount: 0,
  netQuantity: 10, netSalesAmount: 1300, transactionCount: 5,
};
const stores = ['107', '108'].map((businessId, index) => ({
  businessId, label: `${businessId} · ${index ? '銅鑼灣' : '中環'}門店`, totals,
}));
const longName = '超長名稱保濕修護精華液與日常肌膚護理 Super-Long Hydrating Repair Essence Daily Skin Care 50ml 限量組合';

function item(overrides = {}) {
  return {
    storeId: '107', storeLabel: stores[0].label, articleCode: '00100',
    articleName: '原生成商品 01', brandName: '本機合成品牌',
    category1: '健康與美容', category2: '肌膚護理', category2Code: 'A02',
    category3: '日常護理', category4: '人氣商品', category5: '標準裝',
    transactionCount: 2, saleQuantity: 10, saleAmount: 1300,
    returnQuantity: 0, returnAmount: 0, returnTransactionCount: 0,
    netQuantity: 10, netSalesAmount: 1300, ...overrides,
  };
}

function period(key, label, from, to, items, extra = {}) {
  return {
    key, label, from, to, complete: true, successfulStores: 2, totals, stores,
    items, itemCount: extra.itemCount ?? items.length, ...extra,
  };
}

function analysis(periods, extra = {}) {
  return {
    operationId: 'local-insight-layout-fixture', from: '2026-08-01', to: '2026-08-31',
    complete: true, pending: false, selectedStores: 2, successfulStores: 2, totals, stores,
    periods, weeks: [], queryDurationMs: 12, ...extra,
  };
}

const fixtures = {
  empty: analysis([
    period('current', '本期', '2026-08-01', '2026-08-31', []),
    period('previous', '上期', '2026-07-01', '2026-07-31', []),
  ]),
  single: analysis([
    period('current', '本期', '2026-08-01', '2026-08-31', [item({ netSalesAmount: 1300, saleAmount: 1300 })]),
    period('previous', '上期', '2026-07-01', '2026-07-31', [item({ netSalesAmount: 650, saleAmount: 650 })]),
  ]),
  triple: analysis([
    period('current', '本期', '2026-08-01', '2026-08-31', [
      item({ articleCode: '00002', articleName: '下滑商品', netSalesAmount: 20, saleAmount: 20 }),
      item({ articleCode: '00107', articleName: '成長商品', netSalesAmount: 90, saleAmount: 90 }),
      item({ articleCode: '00999', articleName: '退款商品', netSalesAmount: 40, saleAmount: 55, returnAmount: -15, returnQuantity: 1 }),
    ]),
    period('previous', '上期', '2026-07-01', '2026-07-31', [
      item({ articleCode: '00002', articleName: '下滑商品', netSalesAmount: 80, saleAmount: 80 }),
      item({ articleCode: '00107', articleName: '成長商品', netSalesAmount: 30, saleAmount: 30 }),
      item({ articleCode: '00999', articleName: '退款商品', netSalesAmount: 40, saleAmount: 40 }),
    ]),
  ]),
  missing: analysis([
    period('current', '本期', '2026-08-01', '2026-08-31', [item()], { complete: false }),
    period('previous', '上期', '2026-07-01', '2026-07-31', [item({ netSalesAmount: 650, saleAmount: 650 })]),
  ]),
  long: analysis([
    period('current', '本期', '2026-08-01', '2026-08-31', [
      item({ articleName: longName, netSalesAmount: 1300, saleAmount: 1300 }),
    ]),
    period('previous', '上期', '2026-07-01', '2026-07-31', [
      item({ articleName: longName, netSalesAmount: 650, saleAmount: 650 }),
    ]),
  ]),
};

function roundBox(box) {
  if (!box) return null;
  const n = (value) => Math.round(value * 10) / 10;
  return { x: n(box.x), y: n(box.y), width: n(box.width), height: n(box.height) };
}

const browser = await chromium.launch({
  headless: true,
  ...(process.env.UI_BROWSER ? { channel: process.env.UI_BROWSER } : {}),
});
const results = [];
const metrics = { synthetic: true, local: true, notRealRtaSpeed: true, phase, origin, capturedAt: new Date().toISOString(), cases: [] };

try {
  const viewports = [
    { name: 'desktop', width: 1440, height: 1000, theme: 'light' },
    { name: 'laptop', width: 1024, height: 768, theme: 'light' },
    { name: 'narrow', width: 390, height: 844, theme: 'light' },
    { name: 'dark', width: 1440, height: 1000, theme: 'dark' },
  ];
  const casesByViewport = {
    desktop: ['empty', 'single', 'triple', 'missing', 'long'],
    laptop: ['single', 'triple'],
    narrow: ['single', 'triple', 'long'],
    dark: ['single', 'triple'],
  };

  for (const scenario of viewports) {
    for (const fixtureName of casesByViewport[scenario.name]) {
      const page = await browser.newPage({ viewport: { width: scenario.width, height: scenario.height } });
      const errors = [];
      const unexpected = [];
      page.on('pageerror', (error) => errors.push(error.message));
      await page.route('**/api/**', (route) => {
        const path = new URL(route.request().url()).pathname;
        if (path === '/api/events') {
          return route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': synthetic insight layout\n\n' });
        }
        if (path === '/api/session') return route.fulfill({ json: {} });
        unexpected.push(route.request().url());
        return route.abort();
      });
      await page.addInitScript(({ analysis, theme }) => {
        localStorage.setItem('rta-sales-web-v1', JSON.stringify({
          profiles: [], secrets: {}, manCodeGroups: [], analysis, articleNames: {},
        }));
        localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({
          locale: 'zh-TW', theme, rankingLimit: 24,
        }));
      }, { analysis: fixtures[fixtureName], theme: scenario.theme });
      await page.goto(origin);
      await page.getByText('我已知曉', { exact: true }).click();
      await page.getByRole('heading', { name: '銷售額 Top 24', exact: true }).waitFor();
      await page.evaluate(() => document.fonts.ready);
      const insights = page.getByRole('region', { name: '分析重點', exact: true });
      await insights.waitFor();

      if (fixtureName === 'empty') {
        await insights.getByText('此範圍目前沒有可列出的銷售重點。', { exact: true }).waitFor();
        assert.equal(await insights.locator('.insight-grid article, .insight-card').count(), 0);
      } else if (fixtureName === 'missing') {
        await insights.getByRole('status').waitFor();
        assert.match(await insights.getByRole('status').innerText(), /本期資料未完整/);
        assert.equal(await insights.getByRole('status').getAttribute('role'), 'status');
        assert.equal(await insights.locator('.insight-grid article, .insight-card').count(), 0);
      } else {
        await insights.locator('.insight-grid article, .insight-card').first().waitFor();
      }

      const geometry = await page.evaluate(() => {
        const box = (el) => {
          if (!el) return null;
          const r = el.getBoundingClientRect();
          return { x: r.x, y: r.y, width: r.width, height: r.height };
        };
        const section = document.querySelector('.insights');
        const grid = document.querySelector('.insight-grid');
        const cards = [...document.querySelectorAll('.insight-grid > article')];
        const main = document.querySelector('main');
        const performance = document.querySelector('.performance-summary');
        const first = cards[0];
        const styles = first ? getComputedStyle(first) : null;
        const value = first?.querySelector('.insight-value');
        const link = first?.querySelector('.insight-link');
        const contrast = (el) => {
          if (!el) return null;
          const cs = getComputedStyle(el);
          return { color: cs.color, background: cs.backgroundColor };
        };
        return {
          section: box(section),
          grid: box(grid),
          cards: cards.map((card) => box(card)),
          value: box(value),
          link: box(link),
          performance: box(performance),
          mainOverflow: main ? main.scrollWidth - main.clientWidth : 0,
          firstScreen: first ? first.getBoundingClientRect().bottom <= window.innerHeight : null,
          cardDisplay: styles?.display ?? null,
          cardGridTemplateColumns: styles?.gridTemplateColumns ?? null,
          valueContrast: contrast(value),
          cardContrast: contrast(first),
          linkWrap: link ? getComputedStyle(link).overflowWrap : null,
        };
      });

      const shotName = `${phase}-${scenario.name}-${fixtureName}.png`;
      await insights.screenshot({ path: resolve(output, shotName) });
      if (fixtureName === 'single' && (scenario.name === 'desktop' || scenario.name === 'narrow' || scenario.name === 'dark')) {
        await page.screenshot({ path: resolve(output, `${phase}-${scenario.name}-${fixtureName}-full.png`) });
      }

      assert.ok(geometry.mainOverflow <= 1, `${scenario.name}/${fixtureName}: horizontal overflow ${geometry.mainOverflow}`);
      assert.deepEqual(unexpected, [], `${scenario.name}/${fixtureName}: unexpected API`);
      assert.deepEqual(errors, [], `${scenario.name}/${fixtureName}: pageerror ${errors}`);

      if (fixtureName === 'single' || fixtureName === 'long') {
        assert.equal(geometry.cards.length, 1, `${scenario.name}/${fixtureName}: expected 1 card`);
        const button = insights.getByRole('button', { name: /^追查分析重點/ });
        assert.equal(await button.count(), 1);
        if (fixtureName === 'long') {
          assert.match(await button.innerText(), /超長名稱保濕修護精華液/);
          const linkBox = geometry.link;
          assert.ok(linkBox.width + 1 >= 80, 'long name button should have measurable width');
          if (scenario.name === 'narrow') {
            assert.ok(linkBox.width <= scenario.width - 16, `narrow long name overflow ${linkBox.width}`);
          }
        }
        if (scenario.name !== 'narrow' && fixtureName === 'single') {
          const trigger = insights.getByRole('button', { name: /^追查分析重點/ });
          await trigger.focus();
          await trigger.click();
          await page.getByRole('dialog').waitFor();
          await page.keyboard.press('Escape');
          await page.waitForFunction(() => document.activeElement?.classList.contains('insight-link'));
        }
      }
      if (fixtureName === 'triple') {
        assert.equal(geometry.cards.length, 3, `${scenario.name}/${fixtureName}: expected 3 cards`);
        assert.equal(await insights.getByRole('button', { name: /^追查分析重點/ }).count(), 3);
        if (scenario.name === 'desktop' || scenario.name === 'laptop' || scenario.name === 'dark') {
          const ys = geometry.cards.map((card) => card.y);
          assert.ok(Math.max(...ys) - Math.min(...ys) < 8, `${scenario.name} 3-card row alignment ${JSON.stringify(ys)}`);
          geometry.cards.forEach((card) => {
            assert.ok(card.width >= 180, `${scenario.name} 3-card too narrow ${card.width}`);
          });
        }
        if (scenario.name === 'narrow') {
          const ys = geometry.cards.map((card) => card.y);
          assert.ok(ys[1] > ys[0] + 20 && ys[2] > ys[1] + 20, `narrow 3-card should stack ${JSON.stringify(ys)}`);
        }
      }

      if (scenario.name === 'dark' && geometry.valueContrast) {
        assert.notEqual(geometry.valueContrast.color, geometry.valueContrast.background);
        assert.notEqual(geometry.valueContrast.color, 'rgba(0, 0, 0, 0)');
      }

      if (phase === 'after' && fixtureName === 'single' && scenario.name !== 'narrow') {
        assert.ok(geometry.link && geometry.value, 'single insight should expose value and product');
        assert.ok(geometry.link.x >= geometry.value.x + geometry.value.width - 4,
          `${scenario.name} single insight should place product beside ranking ${JSON.stringify({ value: geometry.value, link: geometry.link })}`);
        assert.ok(geometry.cards[0].height <= 130,
          `${scenario.name} single insight card should be compact, height=${geometry.cards[0].height}`);
        assert.ok(geometry.section.height < 230,
          `${scenario.name} single insight section should drop below the stacked 263px row, height=${geometry.section.height}`);
      }
      if (phase === 'after' && fixtureName === 'single' && scenario.name === 'narrow') {
        assert.ok(geometry.link.y >= geometry.value.y + geometry.value.height - 4,
          `narrow single insight should keep stacked ranking-then-product`);
      }

      const record = {
        synthetic: true,
        scenario: scenario.name,
        fixture: fixtureName,
        viewport: { width: scenario.width, height: scenario.height, theme: scenario.theme },
        screenshot: shotName,
        section: roundBox(geometry.section),
        grid: roundBox(geometry.grid),
        cards: geometry.cards.map(roundBox),
        value: roundBox(geometry.value),
        link: roundBox(geometry.link),
        performance: roundBox(geometry.performance),
        firstScreen: geometry.firstScreen,
        cardDisplay: geometry.cardDisplay,
        cardGridTemplateColumns: geometry.cardGridTemplateColumns,
        valueContrast: geometry.valueContrast,
        cardContrast: geometry.cardContrast,
        mainOverflow: geometry.mainOverflow,
      };
      metrics.cases.push(record);
      results.push({ scenario: scenario.name, fixture: fixtureName, passed: true, cardCount: geometry.cards.length, cardHeight: geometry.cards[0]?.height ?? null });
      await page.close();
    }
  }
} finally {
  await browser.close();
}

await writeFile(resolve(output, `${phase}-metrics.json`), JSON.stringify(metrics, null, 2));
await writeFile(resolve(output, `${phase}-results.json`), JSON.stringify(results, null, 2));
console.log(JSON.stringify({ output, phase, synthetic: true, results }, null, 2));
