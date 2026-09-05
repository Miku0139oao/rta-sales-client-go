// @ts-nocheck
// Synthetic/local PDF text+render harness. Does not query RTA or transmit sales data.
// PDFs/images stay in node_modules/.cache/sales-report-pdf/
// Default: regenerate fixtures. Pass --reuse (or PDF_VALIDATE_REUSE=1) to reuse existing files; reuse is labelled.
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { extname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import {
  compareParity,
  continuationPageNumbers,
  extractDocument,
  extractPdf,
  parseValidateArgs as parseArgsFromLib,
} from '../src/lib/sales-report-pdf-section.ts';

import { compareSmallWorkbook, paginationParityExpected } from '../src/lib/sales-report-export-expected.ts';

const frontendRoot = resolve(process.cwd());
const cache = resolve(process.env.PDF_VALIDATE_OUTPUT || resolve(frontendRoot, 'node_modules/.cache/sales-report-pdf'));
const repoRoot = resolve(frontendRoot, '../..');
const limits = [16, 24, 40, 100];
const port = Number(process.env.UI_PDF_PORT || process.env.PORT || 5184);
const browserName = process.env.UI_BROWSER || 'msedge';
const playwrightModule = process.env.PLAYWRIGHT_MODULE
  || 'file:///C:/Users/miku0139/.bun/install/cache/playwright-core@1.63.0@@@1/index.js';
mkdirSync(cache, { recursive: true });

const mime = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.pdf': 'application/pdf',
  '.json': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.map': 'application/json',
};

const winShell = process.platform === 'win32';
const PAGINATION_SUMMARY = {
  netSales: { current: 229657053, previous: 208987918.23, yearAgo: 202098206.64 },
  netQuantity: { current: 1205427, previous: 1096939, yearAgo: 1060776 },
  vsPrevious: '+9.9%',
  vsYearAgo: '+13.6%',
};
const PAGINATION_EXPECTED = {
  16: { lastSales: { rank: 16, code: '0100016', quantity: 4755 }, lastQuantitySample: { rank: 16, code: '0200002' } },
  24: { lastSales: { rank: 24, code: '0100024', quantity: 4728 }, lastQuantitySample: { rank: 24, code: '0200005' } },
  40: {
    lastSales: { rank: 40, code: '0100040', quantity: 4679 },
    lastQuantitySample: { rank: 40, code: '0100027' },
    overallContinuationStart: 26,
    categoryContinuationStart: 34,
    categoryContinuation: { rank: 40, code: '0100040' },
  },
  100: {
    lastSales: { rank: 100, code: '0100100', quantity: 4499 },
    lastQuantitySample: { rank: 100, code: '0500006' },
    overallContinuationStart: 76,
    categoryContinuationStart: 34,
    categoryContinuation: { rank: 100, code: '0100100' },
  },
};

export function parseValidateArgs(argv = process.argv.slice(2), env = process.env) {
  return parseArgsFromLib(argv, env);
}

function run(command, args, cwd = frontendRoot) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, { cwd, stdio: 'inherit', windowsHide: true, shell: winShell });
    child.on('error', reject);
    child.on('exit', (code) => code === 0 ? resolvePromise() : reject(new Error(`${command} ${args.join(' ')} exited ${code}`)));
  });
}

function runCapture(command, args, cwd = frontendRoot, input = '') {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, { cwd, windowsHide: true, shell: winShell });
    const chunks = [];
    const err = [];
    child.stdout.on('data', (data) => chunks.push(data));
    child.stderr.on('data', (data) => err.push(data));
    child.on('error', reject);
    child.on('exit', (code) => {
      const stdout = Buffer.concat(chunks);
      if (code !== 0) reject(new Error(`${command} failed: ${Buffer.concat(err).toString() || stdout.toString()}`));
      else resolvePromise(stdout);
    });
    if (input) child.stdin.end(input);
    else child.stdin.end();
  });
}

export function pdfsReady() {
  return limits.every((limit) => existsSync(join(cache, `ranking-${limit}.pdf`)))
    && existsSync(join(cache, 'workbook-snapshot.json'));
}

export async function ensurePdfs(options = {}) {
  const reuse = Boolean(options.reuse);
  const ready = pdfsReady();
  if (reuse && ready) {
    return { generated: false, reused: true, label: options.reuseLabel || 'explicit-reuse' };
  }
  await run(process.execPath.includes('bun') ? 'bun' : 'bun', ['run', 'test', 'src/lib/sales-report-export-parity.test.ts']);
  return { generated: true, reused: false, label: reuse && !ready ? 'reuse-requested-but-missing-regenerated' : 'regenerated-by-default' };
}

function persistReport(report) {
  mkdirSync(cache, { recursive: true });
  const path = join(cache, 'validation-report.json');
  writeFileSync(path, JSON.stringify(report, null, 2));
  writeFileSync(join(cache, 'validation-diagnostics.json'), JSON.stringify({
    ok: report.ok,
    errors: report.errors,
    generated: report.generated,
    reuse: report.reuse,
    note: report.note,
  }, null, 2));
  return path;
}

const viewerHtml = `<!doctype html>
<meta charset="utf-8">
<title>pdf-page</title>
<style>
  html, body { margin: 0; background: #5c6c7d; }
  canvas { display: block; margin: 0 auto; background: #fff; }
</style>
<canvas id="c"></canvas>
<script type="module">
  import * as pdfjs from '/pdfjs/pdf.mjs';
  pdfjs.GlobalWorkerOptions.workerSrc = '/pdfjs/pdf.worker.mjs';
  const params = new URLSearchParams(location.search);
  const file = params.get('file');
  const pageNumber = Number(params.get('page') || '1');
  const pdf = await pdfjs.getDocument({ url: '/pdfs/' + file, verbosity: 0 }).promise;
  const page = await pdf.getPage(pageNumber);
  const viewport = page.getViewport({ scale: 1.7 });
  const canvas = document.getElementById('c');
  canvas.width = viewport.width;
  canvas.height = viewport.height;
  await page.render({ canvasContext: canvas.getContext('2d', { alpha: false }), viewport }).promise;
  document.title = 'ready-' + file + '-' + pageNumber;
</script>
`;

function startServer() {
  const pdfjsDir = resolve(frontendRoot, 'node_modules/pdfjs-dist/build');
  const server = createServer((req, res) => {
    const url = new URL(req.url || '/', `http://127.0.0.1:${port}`);
    let file = '';
    if (url.pathname === '/' || url.pathname === '/viewer.html') {
      res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
      res.end(viewerHtml);
      return;
    }
    if (url.pathname.startsWith('/pdfjs/')) file = join(pdfjsDir, url.pathname.slice('/pdfjs/'.length));
    else if (url.pathname.startsWith('/pdfs/')) file = join(cache, url.pathname.slice('/pdfs/'.length));
    else {
      res.writeHead(404);
      res.end('not found');
      return;
    }
    if (!existsSync(file)) {
      res.writeHead(404);
      res.end('missing');
      return;
    }
    res.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream' });
    res.end(readFileSync(file));
  });
  return new Promise((resolvePromise, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => resolvePromise(server));
  });
}

async function renderPages(origin) {
  const playwright = await import(playwrightModule);
  const { chromium } = playwright.default ?? playwright;
  const browser = await chromium.launch({ headless: true, channel: browserName });
  const shots = [];
  try {
    const page = await browser.newPage({ viewport: { width: 1400, height: 920 } });
    const targets = [
      { limit: 16, pages: [1, 2, 4] },
      { limit: 24, pages: [1, 2, 3] },
      { limit: 40, pages: [1, 2, 3] },
      { limit: 100, pages: [1, 2, 5] },
    ];
    for (const target of targets) {
      const extracted = JSON.parse(readFileSync(join(cache, `extract-${target.limit}.json`), 'utf8'));
      const pages = extracted.pages ?? extracted;
      const expected = PAGINATION_EXPECTED[target.limit];
      const lastPage = pages.length || 1;
      const extra = continuationPageNumbers(pages, expected);
      const pageList = [...new Set([...target.pages, lastPage, ...extra])];
      for (const pageNumber of pageList) {
        if (pageNumber < 1 || pageNumber > lastPage) continue;
        await page.goto(`${origin}/viewer.html?file=ranking-${target.limit}.pdf&page=${pageNumber}`, { waitUntil: 'networkidle' });
        await page.waitForFunction((expectedTitle) => document.title === expectedTitle, `ready-ranking-${target.limit}.pdf-${pageNumber}`, { timeout: 30000 });
        const file = `render-ranking-${target.limit}-p${pageNumber}.png`;
        await page.locator('canvas').screenshot({ path: join(cache, file) });
        shots.push({
          file,
          limit: target.limit,
          pageNumber,
          continuation: extra.includes(pageNumber),
          note: 'synthetic/local render; not real RTA speed',
        });
      }
    }
  } finally {
    await browser.close();
  }
  return shots;
}

function namedSheet(inspected, fragment) {
  const name = Object.keys(inspected).find((key) => key.includes(fragment));
  return name ? { name, rows: inspected[name] } : undefined;
}

async function exerciseWorkbook() {
  const snapshotPath = join(cache, 'workbook-snapshot.json');
  if (!existsSync(snapshotPath)) return { skipped: true, reason: 'snapshot missing', errors: ['workbook snapshot missing'] };
  const helper = resolve(frontendRoot, 'node_modules/.cache/workbook-fixture.exe');
  // Always rebuild from this checkout; an existing executable is not freshness evidence.
  await run('go', ['build', '-o', helper, 'desktop/frontend/scripts/workbook-fixture.go'], repoRoot);
  const snapshot = JSON.parse(readFileSync(snapshotPath, 'utf8'));
  const request = {
    filename: snapshot.filename,
    context: snapshot.context,
    sheets: snapshot.sheets,
  };
  const encoded = await runCapture(helper, [], frontendRoot, JSON.stringify(request));
  const xlsxPath = join(cache, 'export-identities.xlsx');
  writeFileSync(xlsxPath, Buffer.from(encoded.toString().trim(), 'base64'));
  const inspected = JSON.parse((await runCapture(helper, [xlsxPath], frontendRoot)).toString());
  const errors = compareSmallWorkbook(inspected);
  writeFileSync(join(cache, 'workbook-reread.json'), JSON.stringify(inspected, null, 2));
  const performance = namedSheet(inspected, '銷售表現');
  const sales = namedSheet(inspected, '銷售額 Top');
  const quantity = namedSheet(inspected, '銷量 Top');
  if (!performance) errors.push('missing named sheet 銷售表現');
  if (!sales) errors.push('missing named sheet 銷售額 Top');
  if (!quantity) errors.push('missing named sheet 銷量 Top');
  const netRow = performance?.rows.find((row) => row[0] === '淨銷售額') ?? performance?.rows[1];
  const current = Number(netRow?.[1]);
  const previous = Number(netRow?.[2]);
  const yearAgo = Number(netRow?.[3]);
  if (current !== 14700) errors.push(`xlsx current ${current} != 14700`);
  if (previous !== 7350) errors.push(`xlsx previous ${previous} != 7350`);
  if (yearAgo !== 11760) errors.push(`xlsx yearAgo ${yearAgo} != 11760`);
  const firstCode = sales?.rows[1]?.[1];
  const lastCode = sales?.rows.at(-1)?.[1];
  if (firstCode !== '0200001') errors.push(`xlsx first code ${firstCode} != 0200001`);
  if (lastCode !== '0100012') errors.push(`xlsx last code ${lastCode} != 0100012`);
  return {
    skipped: false,
    xlsxPath,
    sheets: Object.keys(inspected),
    performance: netRow,
    firstCode,
    lastCode,
    errors,
    note: 'synthetic/local workbook; not real RTA speed',
  };
}

export function coverageFromPages(limit, pages, injectMissing = false) {
  const expected = paginationParityExpected(limit);
  const document = extractDocument(pages);
  const compared = compareParity(document, expected, { requirePartial: true });
  const errors = [...compared.errors];
  if (injectMissing) errors.push('injected missing-code failure');
  return {
    limit,
    pages: pages.length,
    ok: errors.length === 0,
    errors,
    lastSales: document.overall.sales.find((row) => row.rank === expected.lastSales.rank),
    continuationPages: continuationPageNumbers(pages, expected),
    hasPartial: Boolean(document.summaryPages[0]?.summary?.hasPartial),
    firstPagePreview: pages[0]?.text.slice(0, 400) ?? '',
  };
}

export async function runValidation(options = {}) {
  const args = { ...parseValidateArgs(), ...options };
  if (args.fixtureFail) {
    const coverage = coverageFromPages(16, [{ pageNumber: 1, text: 'stale pdf without identities' }], true);
    const report = {
      synthetic: true,
      local: true,
      ok: false,
      errors: coverage.errors,
      generated: { generated: false, reused: false, label: 'fixture-fail' },
      reuse: { used: false, label: 'fixture-fail' },
      extractions: [coverage],
      shots: [],
      workbook: { skipped: true },
      note: 'negative fixture proving coverage failure persists diagnostics and exits nonzero',
    };
    persistReport(report);
    return { ok: false, exitCode: 1, report };
  }
  const generated = args.skipGenerate
    ? { generated: false, reused: Boolean(args.reuse), label: args.reuse ? args.reuseLabel : 'skipped-generate' }
    : await ensurePdfs(args);
  const extractions = [];
  const errors = [];
  for (const limit of limits) {
    const pages = await extractPdf(join(cache, `ranking-${limit}.pdf`), readFileSync);
    writeFileSync(join(cache, `extract-${limit}.json`), JSON.stringify({
      synthetic: true,
      local: true,
      note: 'synthetic/local text extraction; not real RTA speed',
      pages,
    }, null, 2));
    const coverage = coverageFromPages(limit, pages, args.failCoverage && limit === 16);
    extractions.push(coverage);
    if (!coverage.ok) errors.push(...coverage.errors.map((error) => `${limit}: ${error}`));
  }
  let shots = [];
  let workbook = { skipped: true };
  let server;
  const origin = `http://127.0.0.1:${port}`;
  try {
    if (!args.skipRender) {
      server = await startServer();
      shots = await renderPages(origin);
    }
    if (!args.skipWorkbook) {
      workbook = await exerciseWorkbook();
      if (workbook.errors?.length) errors.push(...workbook.errors);
    }
  } finally {
    if (server) await new Promise((resolvePromise) => server.close(resolvePromise));
  }
  const ok = errors.length === 0;
  const report = {
    synthetic: true,
    local: true,
    note: 'synthetic/local PDF validation; not real RTA speed',
    ok,
    generated,
    reuse: args.reuse ? { used: generated.reused, label: generated.label } : { used: false, label: generated.label },
    origin,
    errors,
    extractions,
    shots,
    workbook,
  };
  persistReport(report);
  return { ok, exitCode: ok ? 0 : 1, report };
}

async function main() {
  const result = await runValidation();
  console.log(JSON.stringify({
    ok: result.ok,
    exitCode: result.exitCode,
    reuse: result.report.reuse,
    generated: result.report.generated,
    errors: result.report.errors,
    pages: result.report.extractions.map((row) => ({
      limit: row.limit, pages: row.pages, ok: row.ok, errors: row.errors, continuationPages: row.continuationPages,
    })),
    shots: result.report.shots.map((shot) => shot.file),
    workbook: result.report.workbook.skipped ? 'skipped' : result.report.workbook.sheets,
  }, null, 2));
  if (!result.ok) process.exit(result.exitCode);
}

const launchedDirectly = (() => {
  const self = fileURLToPath(import.meta.url);
  const invoked = process.argv[1] ? resolve(process.argv[1]) : '';
  return Boolean(invoked) && pathToFileURL(invoked).href === pathToFileURL(self).href;
})();

if (launchedDirectly) {
  await main();
}
