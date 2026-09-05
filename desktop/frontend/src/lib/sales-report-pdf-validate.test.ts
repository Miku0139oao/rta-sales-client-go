import { readFileSync, existsSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { spawn } from 'node:child_process';
import { describe, expect, it } from 'vitest';
import {
  compareParity,
  extractDocument,
  parseOverallRankings,
  parseRankRows,
  parseSummary,
  parseValidateArgs,
} from './sales-report-pdf-section';
import { compareSmallWorkbook, PAGINATION_EXPECTED, PAGINATION_SUMMARY, SMALL_EXPORT_EXPECTED } from './sales-report-export-expected';

function spawnNode(args: string[], env: NodeJS.ProcessEnv = {}) {
  return new Promise<{ code: number; stdout: string; stderr: string }>((resolvePromise) => {
    const child = spawn('bun', args, {
      cwd: resolve(process.cwd()),
      env: { ...process.env, ...env },
      windowsHide: true,
    });
    const out: Buffer[] = [];
    const err: Buffer[] = [];
    child.stdout.on('data', (data) => out.push(data));
    child.stderr.on('data', (data) => err.push(data));
    child.on('exit', (code) => resolvePromise({
      code: code ?? 1,
      stdout: Buffer.concat(out).toString(),
      stderr: Buffer.concat(err).toString(),
    }));
  });
}

describe('sales-report PDF validation freshness and failure behavior', () => {
  it('does not borrow values from adjacent rows and honors quantity-column order', () => {
    expect(parseRankRows('1 0100001 H01 HK$1,200 120 2 0100002 H02 HK$1,100 110').map(row => [row.amount, row.quantity])).toEqual([[1200, 120], [1100, 110]]);
    expect(parseRankRows('1 0100001 H01 HK$1,200 2 0100002 H02 HK$1,100 110')[0]?.quantity).toBeUndefined();
    expect(parseRankRows('1 0100001 H01 120 HK$1,200 2 0100002 H02 110 HK$1,100', 'quantity').map(row => row.quantity)).toEqual([120, 110]);
  });

  it('rejects deleted, duplicated, reordered and wrong-period continuation pages', () => {
    const rows = [{ rank: 1, code: '0100001', quantity: 120, compactAmount: 'HK$1,200' }, { rank: 2, code: '0100002', quantity: 110, compactAmount: 'HK$1,100' }];
    const page = (index: number, period = '本期') => ({ pageNumber: index + 3, text: `${period} - 分類商品銷售排行 A01 保健護理 ${index + 1} ${rows[index]!.code} H ${rows[index]!.compactAmount} ${rows[index]!.quantity}` });
    const expected = { categoryRows: [{ period: '本期', metric: 'sales' as const, code: 'A01', rows, cardSize: 1 }] };
    expect(compareParity(extractDocument([page(0), page(1)]), expected).ok).toBe(true);
    for (const pages of [[page(1)], [page(0), page(0), page(1)], [page(1), page(0)], [page(0, '去年同期'), page(1)]]) {
      expect(compareParity(extractDocument(pages), expected).ok).toBe(false);
    }
  });

  it('checks reread workbook interior ranks, all values and percentage cells', () => {
    const e = SMALL_EXPORT_EXPECTED;
    const data = { '1 銷售表現': [['指標'], ['淨銷售額', '14700', '7350', '11760', '1', '0.25'], ['淨銷售數量', '1830', '915', '1464', '1', '0.25']],
      '2 銷售額 Top 16': [['rank'], ...e.topSales.map(row => [String(row.rank), row.code, row.name, String(row.amount), String(row.quantity)])],
      '3 銷量 Top 16': [['rank'], ...e.topQuantity.map(row => [String(row.rank), row.code, row.name, String(row.amount), String(row.quantity)])] };
    expect(compareSmallWorkbook(data)).toEqual([]);
    const swapped = structuredClone(data);
    [swapped['2 銷售額 Top 16'][3], swapped['2 銷售額 Top 16'][4]] = [swapped['2 銷售額 Top 16'][4]!, swapped['2 銷售額 Top 16'][3]!];
    expect(compareSmallWorkbook(swapped).length).toBeGreaterThan(0);
    const wrong = structuredClone(data); wrong['3 銷量 Top 16'][5]![3] = '0'; wrong['1 銷售表現'][1]![4] = '0';
    expect(compareSmallWorkbook(wrong)).toHaveLength(2);
  });
  it('requires explicit labelled reuse and regenerates by default', () => {
    expect(parseValidateArgs([]).reuse).toBe(false);
    expect(parseValidateArgs([]).reuseLabel).toBe('');
    const flagged = parseValidateArgs(['--reuse']);
    expect(flagged.reuse).toBe(true);
    expect(flagged.reuseLabel).toContain('explicit-reuse');
    const envFlag = parseValidateArgs([], { PDF_VALIDATE_REUSE: '1' });
    expect(envFlag.reuse).toBe(true);
    expect(envFlag.reuseLabel).toContain('PDF_VALIDATE_REUSE');
  });

  it('fails coverage when last-row identities or comparison values change', () => {
    const summaryText = '銷售摘要 部分資料尚未完成，缺值不代表零銷售 淨銷售額   HK$14,700.00   HK$7,350.00   HK$11,760.00   +100.0%   +25.0%  淨銷售數量   1,830   915   1,464   +100.0%   +25.0%';
    const summary = parseSummary(summaryText);
    expect(summary.netSales?.current).toBe(SMALL_EXPORT_EXPECTED.current.netSalesAmount);
    expect(summary.netSales?.previous).toBe(SMALL_EXPORT_EXPECTED.previous.netSalesAmount);
    const wrongSummary = parseSummary(summaryText.replace('HK$7,350.00', 'HK$0.00'));
    expect(wrongSummary.netSales?.previous).toBe(0);
    expect(wrongSummary.netSales?.previous).not.toBe(SMALL_EXPORT_EXPECTED.previous.netSalesAmount);

    const overall = parseOverallRankings('銷售額 Top 16 / 銷量 Top 16 銷售額 Top 16 1 0200001 S01 HK$2,000 200 16 0100012 H12 HK$100 10 銷量 Top 16 1 0300001 P01 HK$1,800 400');
    expect(overall.sales.at(-1)?.code).toBe('0100012');
    const changed = parseOverallRankings('銷售額 Top 16 1 0200001 S01 HK$2,000 200 16 9999999 XX HK$100 10 銷量 Top 16 1 0300001 P01 HK$1,800 400');
    expect(changed.sales.at(-1)?.code).not.toBe('0100012');

    const document = extractDocument([
      { pageNumber: 1, text: summaryText },
      { pageNumber: 2, text: '銷售額 Top 40 / 銷量 Top 40 銷售額 Top 40 40 0100040 last HK$899K 4,679 銷量 Top 40 40 0100027 qty HK$900K 4,721' },
      { pageNumber: 5, text: '本期 - 分類商品銷售排行 A01 保健護理 HK$89.92M 34 0100034 c HK$899K 4,698 40 0100040 last HK$899K 4,679' },
    ]);
    const ok = compareParity(document, {
      netSales: PAGINATION_SUMMARY.netSales,
      lastSales: PAGINATION_EXPECTED[40].lastSales,
      lastQuantitySample: PAGINATION_EXPECTED[40].lastQuantitySample,
      categoryContinuation: { rank: 40, code: '0100040' },
    }, { requirePartial: true });
    expect(ok.ok).toBe(false);
    expect(ok.errors.some((error) => error.includes('current sales'))).toBe(true);

    const matching = compareParity(document, {
      lastSales: PAGINATION_EXPECTED[40].lastSales,
      lastQuantitySample: PAGINATION_EXPECTED[40].lastQuantitySample,
      categoryContinuation: { rank: 40, code: '0100040' },
    });
    expect(matching.ok).toBe(true);

    const missingLast = compareParity(document, {
      lastSales: { rank: 40, code: '0000000', quantity: 1 },
    });
    expect(missingLast.ok).toBe(false);
    expect(missingLast.errors.join(' ')).toMatch(/code 0100040 != 0000000|missing overall sales/);
  });

  it('persists diagnostics and returns nonzero when required coverage fails', async () => {
    const stale = 'RTA SALES 銷售摘要 淨銷售額 HK$1.00 HK$0.00 HK$0.00 +0.0% +0.0%';
    const coverage = compareParity(extractDocument([{ pageNumber: 1, text: stale }]), PAGINATION_SUMMARY, { requirePartial: true });
    expect(coverage.ok).toBe(false);
    expect(coverage.errors.join(' ')).toMatch(/summary current sales|missing partial/);
    coverage.errors.push('injected missing-code failure');
    expect(coverage.errors.join(' ')).toMatch(/injected missing-code failure/);

    const script = resolve(process.cwd(), 'scripts/sales-report-pdf-validate.mjs');
    const output = mkdtempSync(resolve(tmpdir(), 'rta-pdf-negative-'));
    const result = await spawnNode([script, '--fixture-fail'], { PDF_VALIDATE_OUTPUT: output });
    const reportPath = resolve(output, 'validation-report.json');
    const diagnosticsPath = resolve(output, 'validation-diagnostics.json');
    expect(existsSync(reportPath)).toBe(true);
    expect(existsSync(diagnosticsPath)).toBe(true);
    const report = JSON.parse(readFileSync(reportPath, 'utf8'));
    expect(report.ok).toBe(false);
    expect(result.code).not.toBe(0);
    expect(report.errors.join(' ')).toMatch(/injected missing-code failure|summary/);
    expect(report.reuse.used === true || report.reuse.label).toBeTruthy();
  }, 60_000);
});
