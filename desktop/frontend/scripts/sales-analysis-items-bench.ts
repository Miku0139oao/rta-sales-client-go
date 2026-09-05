/**
 * Local synthetic pack/unpack/JSON/table-snapshot timings.
 * Not a real RTA query speed measurement.
 */
import { mkdirSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { packSalesAnalysisItems, unpackSalesAnalysisItems } from '../src/lib/salesAnalysisItems';
import { sortAnalysisTable, workbookSnapshot, type AnalysisTable } from '../src/lib/analysisTable';
import { itemChecksum, SYNTHETIC_BENCH_KIND, syntheticPackedFixture } from './sales-analysis-items-fixture';
import type { SalesAnalysisItem, SalesAnalysisPackedItems, SalesAnalysisPackedRow, SalesAnalysisStoreSummary } from '../src/lib/types';

const here = path.dirname(fileURLToPath(import.meta.url));
const cacheDir = path.join(here, '..', 'node_modules', '.cache');
const label = readLabel(process.argv.slice(2));
const warmup = 2;
const measuredRuns = 5;

interface Timed {
  name: string;
  samplesMs: number[];
  medianMs: number;
  p95Ms: number;
  maxMs: number;
  heapDeltaBytes: number[];
  medianHeapDeltaBytes: number;
}

function readLabel(argv: string[]): string {
  const flagged = argv.find((value) => value.startsWith('--label='));
  if (flagged) return flagged.slice('--label='.length) || 'run';
  const index = argv.indexOf('--label');
  if (index >= 0 && argv[index + 1]) return argv[index + 1]!;
  return 'run';
}

function median(values: number[]): number {
  if (!values.length) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1 ? sorted[mid]! : (sorted[mid - 1]! + sorted[mid]!) / 2;
}

function percentile(values: number[], p: number): number {
  if (!values.length) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  const rank = Math.min(sorted.length - 1, Math.max(0, Math.ceil((p / 100) * sorted.length) - 1));
  return sorted[rank]!;
}

function maybeGC(): void {
  const bunGc = (globalThis as { Bun?: { gc?: (force?: boolean) => void } }).Bun?.gc;
  if (typeof bunGc === 'function') bunGc(true);
  else if (typeof (globalThis as { gc?: () => void }).gc === 'function') (globalThis as { gc: () => void }).gc();
}

function heapUsed(): number {
  return process.memoryUsage().heapUsed;
}

function packedString(dict: string[], index: number | undefined): string {
  if (!index || index < 0 || index >= dict.length) return '';
  return dict[index] ?? '';
}

function packedRowFields(row: SalesAnalysisPackedRow | number[]): SalesAnalysisPackedRow {
  if (!Array.isArray(row)) return row;
  const at = (index: number) => row[index] ?? 0;
  return {
    s: at(0), ac: at(1), an: at(2), br: at(3),
    c1: at(4), k1: at(5), c2: at(6), k2: at(7),
    c3: at(8), k3: at(9), c4: at(10), k4: at(11),
    c5: at(12), k5: at(13),
    t: at(14), sq: at(15), sa: at(16), rq: at(17), rt: at(18), ra: at(19), nq: at(20), ns: at(21),
  };
}

function unpackSalesAnalysisItemsBaseline(batch: SalesAnalysisPackedItems, stores: SalesAnalysisStoreSummary[] = []): SalesAnalysisItem[] {
  const dict = batch.d ?? batch.dict ?? [];
  const storeList = stores ?? [];
  const rows = batch.r ?? batch.rows ?? [];
  return rows.map((raw) => {
    const row = packedRowFields(raw);
    const store = storeList[row.s];
    return {
      storeId: store?.businessId ?? '',
      storeLabel: store?.label ?? '',
      articleCode: packedString(dict, row.ac),
      articleName: packedString(dict, row.an),
      brandName: packedString(dict, row.br),
      category1: packedString(dict, row.c1),
      category1Code: packedString(dict, row.k1),
      category2: packedString(dict, row.c2),
      category2Code: packedString(dict, row.k2),
      category3: packedString(dict, row.c3),
      category3Code: packedString(dict, row.k3),
      category4: packedString(dict, row.c4),
      category4Code: packedString(dict, row.k4),
      category5: packedString(dict, row.c5),
      category5Code: packedString(dict, row.k5),
      transactionCount: row.t ?? 0,
      saleQuantity: row.sq ?? 0,
      saleAmount: row.sa ?? 0,
      returnQuantity: row.rq ?? 0,
      returnTransactionCount: row.rt ?? 0,
      returnAmount: row.ra ?? 0,
      netQuantity: row.nq ?? 0,
      netSalesAmount: row.ns ?? 0,
    };
  });
}

function summarizeSamples(name: string, samplesMs: number[], heapDeltaBytes: number[]): Timed {
  return {
    name,
    samplesMs,
    medianMs: median(samplesMs),
    p95Ms: percentile(samplesMs, 95),
    maxMs: Math.max(...samplesMs),
    heapDeltaBytes,
    medianHeapDeltaBytes: median(heapDeltaBytes),
  };
}

function timeSamples(name: string, repeats: number, run: () => void): Timed {
  for (let index = 0; index < warmup; index += 1) run();
  const samplesMs: number[] = [];
  const heapDeltaBytes: number[] = [];
  for (let index = 0; index < repeats; index += 1) {
    maybeGC();
    const beforeHeap = heapUsed();
    const started = performance.now();
    run();
    samplesMs.push(performance.now() - started);
    heapDeltaBytes.push(heapUsed() - beforeHeap);
  }
  return summarizeSamples(name, samplesMs, heapDeltaBytes);
}

function timeOnce(run: () => void): { ms: number; heapDeltaBytes: number } {
  maybeGC();
  const beforeHeap = heapUsed();
  const started = performance.now();
  run();
  return { ms: performance.now() - started, heapDeltaBytes: heapUsed() - beforeHeap };
}

function pairUnpackSamples(
  wire: SalesAnalysisPackedItems,
  stores: SalesAnalysisStoreSummary[],
  repeats: number,
): { current: Timed; baseline: Timed; currentItems: SalesAnalysisItem[]; baselineItems: SalesAnalysisItem[] } {
  let currentItems: SalesAnalysisItem[] = [];
  let baselineItems: SalesAnalysisItem[] = [];
  const runCurrent = () => { currentItems = unpackSalesAnalysisItems(wire, stores); };
  const runBaseline = () => { baselineItems = unpackSalesAnalysisItemsBaseline(wire, stores); };
  for (let index = 0; index < warmup; index += 1) {
    if (index % 2 === 0) { runCurrent(); runBaseline(); }
    else { runBaseline(); runCurrent(); }
  }
  const currentMs: number[] = [];
  const baselineMs: number[] = [];
  const currentHeap: number[] = [];
  const baselineHeap: number[] = [];
  for (let index = 0; index < repeats; index += 1) {
    const currentFirst = index % 2 === 0;
    const first = timeOnce(currentFirst ? runCurrent : runBaseline);
    const second = timeOnce(currentFirst ? runBaseline : runCurrent);
    if (currentFirst) {
      currentMs.push(first.ms); currentHeap.push(first.heapDeltaBytes);
      baselineMs.push(second.ms); baselineHeap.push(second.heapDeltaBytes);
    } else {
      baselineMs.push(first.ms); baselineHeap.push(first.heapDeltaBytes);
      currentMs.push(second.ms); currentHeap.push(second.heapDeltaBytes);
    }
  }
  return {
    current: summarizeSamples('unpack-current', currentMs, currentHeap),
    baseline: summarizeSamples('unpack-baseline', baselineMs, baselineHeap),
    currentItems,
    baselineItems,
  };
}

function assertSameChecksum(actual: SalesAnalysisItem[], expected: SalesAnalysisItem[], context: string): void {
  const left = itemChecksum(actual);
  const right = itemChecksum(expected);
  if (left.count !== right.count) throw new Error(`${context}: count ${left.count} != ${right.count}`);
  if (left.netSalesAmount !== right.netSalesAmount) {
    throw new Error(`${context}: netSalesAmount ${left.netSalesAmount} != ${right.netSalesAmount}`);
  }
  if (left.netQuantity !== right.netQuantity) throw new Error(`${context}: netQuantity ${left.netQuantity} != ${right.netQuantity}`);
  if (left.codes !== right.codes) throw new Error(`${context}: codes ${left.codes} != ${right.codes}`);
  const last = expected.length - 1;
  if (last >= 0) {
    if (actual[0]!.articleCode !== expected[0]!.articleCode || actual[0]!.storeId !== expected[0]!.storeId) {
      throw new Error(`${context}: first row identity mismatch`);
    }
    if (actual[last]!.articleCode !== expected[last]!.articleCode || actual[last]!.netSalesAmount !== expected[last]!.netSalesAmount) {
      throw new Error(`${context}: last row mismatch`);
    }
  }
}

function productsTable(items: SalesAnalysisItem[]): AnalysisTable {
  return {
    id: 'products',
    name: '商品',
    columns: [
      { label: '門店', format: 'text' },
      { label: '分類', format: 'text' },
      { label: '編碼', format: 'text' },
      { label: '商品', format: 'text' },
      { label: '數量', format: 'number' },
      { label: '淨銷售額', format: 'money' },
    ],
    rows: items.map((item) => ({
      cells: [item.storeId, item.category2, item.articleCode, item.articleName, item.netQuantity, item.netSalesAmount],
    })),
  };
}

function aggregateByArticle(items: SalesAnalysisItem[]): { codes: number; netSalesAmount: number } {
  const byCode = new Map<string, number>();
  for (const item of items) {
    byCode.set(item.articleCode, (byCode.get(item.articleCode) ?? 0) + item.netSalesAmount);
  }
  let netSalesAmount = 0;
  for (const amount of byCode.values()) netSalesAmount += amount;
  return { codes: byCode.size, netSalesAmount };
}

function snapshotRowLimit(itemCount: number): number {
  const columns = 6;
  return Math.min(itemCount, Math.floor(500000 / columns));
}

function measureCase(itemCount: number, storeCount: number, uniqueArticles: number) {
  const fixture = syntheticPackedFixture(itemCount, storeCount, uniqueArticles);
  const expected = itemChecksum(fixture.items);
  let packed: SalesAnalysisPackedItems = { k: 'current', d: [''], r: [] };
  const pack = timeSamples('pack', measuredRuns, () => {
    packed = packSalesAnalysisItems('current', fixture.items, fixture.stores);
  });
  let jsonText = '';
  const serialize = timeSamples('serialize', measuredRuns, () => {
    jsonText = JSON.stringify(packed);
  });
  let wire: SalesAnalysisPackedItems = packed;
  const deserialize = timeSamples('deserialize', measuredRuns, () => {
    wire = JSON.parse(jsonText) as SalesAnalysisPackedItems;
  });
  const paired = pairUnpackSamples(wire, fixture.stores, measuredRuns);
  const unpacked = paired.currentItems;
  const unpack = paired.current;
  assertSameChecksum(unpacked, fixture.items, `unpack ${itemCount}`);
  assertSameChecksum(paired.baselineItems, fixture.items, `unpack-baseline ${itemCount}`);
  const roundTrip = unpackSalesAnalysisItems(packed, fixture.stores);
  assertSameChecksum(roundTrip, fixture.items, `pack-unpack ${itemCount}`);
  if (roundTrip[0]!.storeLabel !== fixture.stores[0]!.label) {
    throw new Error(`store identity lost at ${itemCount}`);
  }

  let aggregated = { codes: 0, netSalesAmount: 0 };
  const aggregate = timeSamples('aggregate', measuredRuns, () => {
    aggregated = aggregateByArticle(unpacked);
  });
  if (aggregated.netSalesAmount !== expected.netSalesAmount) {
    throw new Error(`aggregate sum mismatch at ${itemCount}`);
  }

  const table = productsTable(unpacked);
  let sorted = table;
  const sort = timeSamples('sort', measuredRuns, () => {
    sorted = sortAnalysisTable(table, { column: 5, direction: 'descending' }, 'zh-TW');
  });
  if (sorted.rows.length !== unpacked.length) throw new Error(`sort lost rows at ${itemCount}`);

  const snapshotCount = snapshotRowLimit(itemCount);
  const snapshotSource = snapshotCount === unpacked.length ? sorted : {
    ...sorted,
    rows: sorted.rows.slice(0, snapshotCount),
  };
  const snapshot = timeSamples('workbookSnapshot', measuredRuns, () => {
    workbookSnapshot([snapshotSource], ['synthetic-local', `rows=${snapshotCount}`], 'synthetic-local.xlsx');
  });

  return {
    kind: SYNTHETIC_BENCH_KIND,
    notRealRtaSpeed: true,
    itemCount,
    storeCount,
    uniqueArticles,
    dictSize: (packed.d ?? packed.dict ?? []).length,
    packedRows: (packed.r ?? packed.rows ?? []).length,
    jsonBytes: jsonText.length,
    checksum: expected,
    aggregateCodes: aggregated.codes,
    snapshotRows: snapshotCount,
    timings: { pack, serialize, deserialize, unpack, unpackBaseline: paired.baseline, aggregate, sort, snapshot },
    pipeline: pipelineShare(pack, serialize, deserialize, unpack, paired.baseline, aggregate, sort, snapshot),
  };
}

function pipelineShare(
  pack: Timed, serialize: Timed, deserialize: Timed, unpack: Timed, unpackBaseline: Timed,
  aggregate: Timed, sort: Timed, snapshot: Timed,
) {
  const currentTotal = pack.medianMs + serialize.medianMs + deserialize.medianMs + unpack.medianMs + aggregate.medianMs + sort.medianMs + snapshot.medianMs;
  const baselineTotal = pack.medianMs + serialize.medianMs + deserialize.medianMs + unpackBaseline.medianMs + aggregate.medianMs + sort.medianMs + snapshot.medianMs;
  const share = (value: number, total: number) => total === 0 ? 0 : value / total;
  return {
    notEndToEndOrRta: true,
    estimationMethod: 'sum of separately measured phase medians; other phases held constant, not a measured pipeline median',
    estimatedCurrentPhaseMedianSumMs: currentTotal,
    estimatedBaselinePhaseMedianSumMs: baselineTotal,
    unpackShareOfEstimatedCurrentSum: share(unpack.medianMs, currentTotal),
    unpackShareOfEstimatedBaselineSum: share(unpackBaseline.medianMs, baselineTotal),
    unpackMedianDeltaMs: unpack.medianMs - unpackBaseline.medianMs,
    estimatedDeltaHoldingOtherPhasesConstantMs: currentTotal - baselineTotal,
  };
}

function runtimeDimensions() {
  const cpu = os.cpus()[0];
  return {
    kind: SYNTHETIC_BENCH_KIND,
    notRealRtaSpeed: true,
    bun: typeof Bun !== 'undefined' ? Bun.version : undefined,
    node: process.version,
    platform: os.platform(),
    arch: os.arch(),
    cpus: os.cpus().length,
    cpuModel: cpu?.model,
    totalMemBytes: os.totalmem(),
    hostname: os.hostname(),
    cwd: process.cwd(),
  };
}

const cases = [
  { itemCount: 1000, storeCount: 8, uniqueArticles: 200 },
  { itemCount: 10000, storeCount: 20, uniqueArticles: 800 },
  { itemCount: 100000, storeCount: 50, uniqueArticles: 2500 },
  { itemCount: 200000, storeCount: 80, uniqueArticles: 4000 },
];

const results = cases.map((entry) => measureCase(entry.itemCount, entry.storeCount, entry.uniqueArticles));
const report = {
  kind: SYNTHETIC_BENCH_KIND,
  notRealRtaSpeed: true,
  label,
  measuredAt: new Date().toISOString(),
  methodology: {
    warmup,
    measuredRuns,
    timing: 'performance.now() around each phase after warmup; median and p95 of measured samples',
    unpackComparison: 'same packed JSON payload; baseline packedRowFields path vs current direct compact decode; alternating order each measured run',
    memory: 'process.memoryUsage().heapUsed delta after optional Bun.gc/global.gc; noisy, not a precise allocator trace',
    data: 'deterministic in-process synthetic multi-store article rows; no RTA network',
    caveat: 'unpack delta is only a share of this local pack/JSON/table pipeline, not end-to-end UI or real RTA speed',
  },
  runtime: runtimeDimensions(),
  cases: results,
};

mkdirSync(cacheDir, { recursive: true });
const jsonPath = path.join(cacheDir, `sales-analysis-items-bench-${label}.json`);
const logPath = path.join(cacheDir, `sales-analysis-items-bench-${label}.log`);
writeFileSync(jsonPath, `${JSON.stringify(report, null, 2)}\n`);
const lines = [
  `synthetic-local bench label=${label} (not real RTA speed)`,
  `runtime bun=${report.runtime.bun} node=${report.runtime.node} ${report.runtime.platform}/${report.runtime.arch} cpus=${report.runtime.cpus} ${report.runtime.cpuModel}`,
  ...results.map((entry) => {
    const unpack = entry.timings.unpack;
    const serialize = entry.timings.serialize;
    return [
      `${entry.itemCount} items / ${entry.storeCount} stores / dict=${entry.dictSize} jsonBytes=${entry.jsonBytes}`,
      `  pack median=${entry.timings.pack.medianMs.toFixed(2)}ms p95=${entry.timings.pack.p95Ms.toFixed(2)}ms`,
      `  serialize median=${serialize.medianMs.toFixed(2)}ms p95=${serialize.p95Ms.toFixed(2)}ms`,
      `  deserialize median=${entry.timings.deserialize.medianMs.toFixed(2)}ms p95=${entry.timings.deserialize.p95Ms.toFixed(2)}ms`,
      `  unpack-current median=${unpack.medianMs.toFixed(2)}ms p95=${unpack.p95Ms.toFixed(2)}ms share=${(entry.pipeline.unpackShareOfEstimatedCurrentSum * 100).toFixed(1)}%`,
      `  unpack-baseline median=${entry.timings.unpackBaseline.medianMs.toFixed(2)}ms p95=${entry.timings.unpackBaseline.p95Ms.toFixed(2)}ms share=${(entry.pipeline.unpackShareOfEstimatedBaselineSum * 100).toFixed(1)}%`,
      `  unpack delta=${entry.pipeline.unpackMedianDeltaMs.toFixed(2)}ms estimated phase-sum delta=${entry.pipeline.estimatedDeltaHoldingOtherPhasesConstantMs.toFixed(2)}ms (other phases held constant; not measured end-to-end/RTA)`,
      `  aggregate median=${entry.timings.aggregate.medianMs.toFixed(2)}ms sort median=${entry.timings.sort.medianMs.toFixed(2)}ms snapshot(${entry.snapshotRows}) median=${entry.timings.snapshot.medianMs.toFixed(2)}ms`,
    ].join('\n');
  }),
  `wrote ${jsonPath}`,
];
const log = `${lines.join('\n')}\n`;
writeFileSync(logPath, log);
process.stdout.write(log);
