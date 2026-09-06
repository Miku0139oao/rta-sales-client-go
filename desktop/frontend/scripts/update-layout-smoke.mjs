// Isolated source-backed browser regression. No Wails, real accounts, or release requests.
// PLAYWRIGHT_MODULE must point to an already installed Playwright module (file URL or package).
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = fileURLToPath(new URL('../', import.meta.url));
const out = fileURLToPath(new URL('../node_modules/.cache/update-layout/', import.meta.url));
const entry = `
import '/src/app.css';
import '@fontsource/material-symbols-rounded/400.css';
import '@material/web/button/filled-button.js';
import '@material/web/button/outlined-button.js';
import '@material/web/button/text-button.js';
import '@material/web/button/filled-tonal-button.js';
import '@material/web/iconbutton/icon-button.js';
import '@material/web/switch/switch.js';
import { mount } from 'svelte';
import App from '/src/App.svelte';
import { configureBackend } from '/src/lib/backend.ts';
const latest = new URLSearchParams(location.search).get('latest') || '0.4.7';
const newer = latest === '0.4.8';
const notes = '# Official fixture changelog\\n\\n修正全螢幕設定對齊。\\n'+('Changelog is escaped text; downloads require consent.\\n'.repeat(40))+'<img src=x onerror=alert(1)>'+('x'.repeat(500));
const idle = {currentVersion:'0.4.7',availableVersion:'',phase:'idle',candidateId:'',installSupported:true,error:'',releaseNotes:'',changelogVersion:'',changelogBody:''};
const checked = {...idle,phase:newer?'available':'current',availableVersion:newer?latest:'',candidateId:newer?'isolated-candidate':'',releaseNotes:newer?'Candidate-only notes <b>escaped</b>':'',changelogVersion:latest,changelogBody:notes};
window.fixtureCalls=[];
let status=idle;
configureBackend({methods:new Proxy({}, {get:(_,name)=>async (...args)=>{
 window.fixtureCalls.push([name,args]);
 if(name==='GetUpdateStatus')return status;
 if(name==='CheckForUpdate'||name==='CheckForUpdateStartup')return status=checked;
 if(name==='InstallUpdate')throw Error('Unexpected installation in isolated fixture');
 return [];
}})});
mount(App,{target:document.getElementById('app')});
`;
const server = await createServer({ root, server: { host: '127.0.0.1', port: 9358, strictPort: true }, plugins: [{
  name: 'isolated-update-layout',
  resolveId(id) { if (id === 'virtual:update-layout') return '\0virtual:update-layout'; },
  load(id) { if (id === '\0virtual:update-layout') return entry; },
  configureServer(s) { s.middlewares.use('/__update-layout', (_req, res) => {
    res.setHeader('Content-Type', 'text/html');
    res.end('<!doctype html><html lang="zh-Hant"><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module" src="/@id/__x00__virtual:update-layout"></script></html>');
  }); },
}] });
let browser;
const results = [];
const errors = [];
try {
  await mkdir(out, { recursive: true });
  await server.listen();
  browser = await chromium.launch({ headless: true });
  for (const theme of ['light', 'dark']) {
    const context = await browser.newContext();
    await context.route('**/*', route => new URL(route.request().url()).hostname === '127.0.0.1' ? route.continue() : route.abort());
    const page = await context.newPage();
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(theme => localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'zh-TW', theme, autoCheckUpdates: true })), theme);
    await page.goto('http://127.0.0.1:9358/__update-layout');
    await page.getByRole('button', { name: '設定', exact: true }).click();
    const summary = page.locator('.update-card summary');
    await summary.waitFor();
    assert.equal(await summary.textContent(), '更新日誌 — GitHub 最新正式版本 v0.4.7');
    assert.equal(await page.locator('.release-notes').evaluate(el => el.open), false);
    await summary.focus(); await page.keyboard.press('Enter');
    await page.evaluate(() => document.fonts.ready);
    for (const [width, height] of [[3840, 2078], [2560, 1440], [1920, 1080], [1440, 1050], [390, 844], [320, 740]]) {
      await page.setViewportSize({ width, height });
      await page.locator('main').evaluate(el => { el.scrollTop = 0; });
      const geometry = await page.evaluate(() => {
        const rect = selector => { const r = document.querySelector(selector).getBoundingClientRect(); return { x: r.x, width: r.width }; };
        const notes = document.querySelector('.release-notes pre');
        return { update: rect('.update-card'), settings: rect('.settings-page'), card: rect('.settings-grid > .settings-card'),
          overflow: document.documentElement.scrollWidth > innerWidth || document.querySelector('main').scrollWidth > document.querySelector('main').clientWidth,
          notesHeight: notes.clientHeight, notesScroll: notes.scrollHeight, injectedImages: notes.querySelectorAll('img').length };
      });
      for (const target of [geometry.settings, geometry.card]) {
        assert.ok(Math.abs(geometry.update.x - target.x) < 1, JSON.stringify(geometry));
        assert.ok(Math.abs(geometry.update.width - target.width) < 1, JSON.stringify(geometry));
      }
      assert.equal(geometry.overflow, false);
      assert.equal(geometry.injectedImages, 0);
      assert.ok(geometry.notesHeight <= 220 && geometry.notesScroll > geometry.notesHeight);
      await page.screenshot({ path: `${out}${theme}-${width}.png` });
      results.push({ theme, width, height, ...geometry });
    }
    await page.getByRole('button', { name: '銷售分析', exact: true }).click();
    assert.equal(await page.locator('.update-card').count(), 0);
    await page.getByRole('button', { name: '設定', exact: true }).click();
    await summary.waitFor();
    const calls = await page.evaluate(() => window.fixtureCalls);
    assert.equal(calls.filter(([name]) => name === 'CheckForUpdateStartup').length, 1);
    assert.equal(calls.filter(([name]) => name === 'CheckForUpdate').length, 0);
    assert.equal(calls.filter(([name]) => name === 'InstallUpdate').length, 0);
    await context.close();
  }
  // User's native screenshot is approximately a 150% Windows scale viewport.
  for (const theme of ['light', 'dark']) {
    const context = await browser.newContext({ viewport: { width: 2559, height: 1385 }, deviceScaleFactor: 1.5 });
    const page = await context.newPage();
    await page.addInitScript(theme => localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'zh-TW', theme, autoCheckUpdates: true })), theme);
    await page.goto('http://127.0.0.1:9358/__update-layout');
    await page.getByRole('button', { name: '設定', exact: true }).click();
    await page.locator('.update-card summary').waitFor();
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: `${out}${theme}-native-equivalent.png` });
    await context.close();
  }
  const page = await browser.newPage({ viewport: { width: 1440, height: 1050 } });
  await page.addInitScript(() => localStorage.setItem('rta-sales-desktop-settings-v2', JSON.stringify({ locale: 'en', theme: 'dark', autoCheckUpdates: false })));
  for (const latest of ['0.4.6', '0.4.8']) {
    await page.goto(`http://127.0.0.1:9358/__update-layout?latest=${latest}`);
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByText('Current version: 0.4.7').waitFor();
    assert.equal(await page.evaluate(() => window.fixtureCalls.filter(([name]) => name === 'CheckForUpdate' || name === 'CheckForUpdateStartup').length), 0);
    await page.getByRole('button', { name: 'Check for updates', exact: true }).click();
    await page.getByText(`Changelog — Latest GitHub Release v${latest}`, { exact: true }).waitFor();
    if (latest === '0.4.6') assert.equal(await page.getByRole('button', { name: 'Download and restart…', exact: true }).count(), 0);
    else {
      await page.getByRole('button', { name: 'Download and restart…', exact: true }).click();
      await page.getByRole('dialog').locator('summary').click();
      assert.equal(await page.getByRole('dialog').locator('pre').textContent(), 'Candidate-only notes <b>escaped</b>');
      assert.equal(await page.getByRole('dialog').locator('pre b').count(), 0);
      await page.screenshot({ path: `${out}newer-confirmation.png` });
      await page.keyboard.press('Escape');
    }
    assert.equal(await page.evaluate(() => window.fixtureCalls.filter(([name]) => name === 'InstallUpdate').length), 0);
  }
  assert.deepEqual(errors, []);
  await writeFile(`${out}result.json`, JSON.stringify({ passed: true, engine: await browser.version(), results, errors }, null, 2));
  console.log(`PASS isolated update layout/changelog: ${out}`);
} finally { await browser?.close(); await server.close(); }
