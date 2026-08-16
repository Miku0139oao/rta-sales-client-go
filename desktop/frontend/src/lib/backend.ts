import type {
  AnalysisProgress,
  AnalysisResult,
  AnalyzeRequest,
  AppErrorShape,
  ApplyRequest,
  ApplyResult,
  BackendApi,
  PreviewRow,
  Profile,
  ProfileTestResult,
  ProfileUpsertRequest,
  SaveWorkbookRequest,
  ScanWorkbookRequest,
  SalesAnalysisItem,
  SalesAnalysisPDFWriteRequest,
  SalesAnalysisProgress,
  SalesAnalysisPeriodResult,
  SalesAnalysisItemsRequest,
  SalesAnalysisPackedItems,
  SalesAnalysisReportMemo,
  SalesAnalysisReportMemoRequest,
  SalesAnalysisRequest,
  SalesAnalysisResult,
  SalesAnalysisStore,
  SalesAnalysisTotals,
  WorkbookScan,
} from './types';
import { AppError } from './types';

type AnyMethod = (...args: unknown[]) => unknown;
type MethodSource = Record<string, AnyMethod | undefined>;
type EventSource = {
  on: (name: string, listener: (payload: unknown) => void) => (() => void) | void;
};
type FileDropSource = {
  on: (listener: (paths: string[]) => void) => (() => void) | void;
};

interface BackendInjection {
  methods: MethodSource;
  events?: EventSource;
  fileDrops?: FileDropSource;
}

declare global {
  interface Window {
    go?: Record<string, Record<string, MethodSource>>;
    runtime?: {
      EventsOn?: (name: string, listener: (payload: unknown) => void) => (() => void) | void;
      EventsOff?: (name: string) => void;
      OnFileDrop?: (listener: (x: number, y: number, paths: string[]) => void, useDropTarget: boolean) => void;
      OnFileDropOff?: () => void;
    };
  }
}

let injection: BackendInjection | undefined;

export function configureBackend(next: BackendInjection | undefined): void {
  injection = next;
}

function findMethod(names: string[]): AnyMethod | undefined {
  if (injection?.methods) {
    for (const name of names) {
      if (typeof injection.methods[name] === 'function') return injection.methods[name]!.bind(injection.methods);
    }
  }
  if (typeof window === 'undefined' || !window.go) return undefined;
  for (const namespace of Object.values(window.go)) {
    for (const service of Object.values(namespace ?? {})) {
      for (const name of names) {
        if (service && typeof service[name] === 'function') return service[name]!.bind(service);
      }
    }
  }
  return undefined;
}

function asAppError(error: unknown): AppError {
  if (error instanceof AppError) return error;
  if (error instanceof Error) {
    const coded = error as Error & Partial<AppErrorShape>;
    return new AppError(coded.code ?? 'backend_error', coded.message);
  }
  if (typeof error === 'object' && error) {
    const value = error as Partial<AppErrorShape>;
    return new AppError(value.code ?? 'backend_error', value.message ?? 'Unknown backend error');
  }
  return new AppError('backend_error', String(error));
}

async function invoke<T>(names: string[], args: unknown[], fallback: () => Promise<T>): Promise<T> {
  const method = findMethod(names);
  if (!method) {
    if (import.meta.env.DEV) return fallback();
    throw new AppError('backend_unavailable', 'Desktop backend is unavailable');
  }
  try {
    return (await method(...args)) as T;
  } catch (error) {
    throw asAppError(error);
  }
}

const sampleRows: PreviewRow[] = [
  {
    id: 'row-18', date: '2026-08-12', row: 18, storeLabel: '北區門市 01', profileLabel: '主要帳號',
    currentL: '', currentAB: '', proposedL: '128', proposedAB: '3,840', status: 'change',
  },
  {
    id: 'row-19', date: '2026-08-12', row: 19, storeLabel: '北區門市 02', profileLabel: '主要帳號',
    currentL: '96', currentAB: '2,910', proposedL: '96', proposedAB: '2,910', status: 'unchanged',
  },
  {
    id: 'row-20', date: '2026-08-13', row: 20, storeLabel: '中區門市 01', profileLabel: '備援帳號',
    currentL: '74', currentAB: '2,205', proposedL: '82', proposedAB: '2,448', status: 'change',
  },
  {
    id: 'row-21', date: '2026-08-13', row: 21, storeLabel: '待確認門市', profileLabel: '—',
    currentL: '', currentAB: '', proposedL: '—', proposedAB: '—', status: 'issue', message: 'store_not_mapped',
  },
  {
    id: 'row-22', date: '2026-08-14', row: 22, storeLabel: '南區門市 01', profileLabel: '備援帳號',
    currentL: '110', currentAB: '3,300', proposedL: '—', proposedAB: '—', status: 'failed', message: 'query_failed',
  },
];

let mockProfiles: Profile[] = [
  { id: 'profile-primary', displayName: '主要帳號', enabled: true, priority: 1, hasCredentials: true, accountHint: 'sa••••01', lastTestStatus: 'success' },
  { id: 'profile-backup', displayName: '備援帳號', enabled: true, priority: 2, hasCredentials: true, accountHint: 'sa••••02', lastTestStatus: 'untested' },
];
const mockListeners = new Set<(progress: AnalysisProgress) => void>();
const mockSalesAnalysisListeners = new Set<(progress: SalesAnalysisProgress) => void>();
let mockCancelled = false;
let mockSalesCancelled = false;

const mockStoreNames = [
  'Central', 'Harbour', 'North', 'South', 'East', 'West', 'Park', 'Bay',
  'Hill', 'Garden', 'Market', 'Plaza', 'Station', 'Bridge', 'Harbour East', 'Central West',
];

function mockAnalysisStoresFor(count: number): SalesAnalysisStore[] {
  const total = count > 0 ? count : 2;
  return Array.from({ length: total }, (_, index) => ({
    businessId: String(107 + index),
    label: `${107 + index} - ${mockStoreNames[index % mockStoreNames.length]}`,
  }));
}

const mockAnalysisStores: SalesAnalysisStore[] = mockAnalysisStoresFor(2);

const mockAnalysisItems: SalesAnalysisItem[] = [
  {
    storeId: '107', storeLabel: '107 - Central', category1: 'HEALTH & BEAUTY', category1Code: 'A', category2: 'BEAUTY CARE', category2Code: 'A02',
    category3: 'SKIN CARE', category3Code: 'A0201', category4: 'FACIAL', category4Code: 'A020101', category5: 'MASQUE', category5Code: 'A02010101', articleCode: '552646', articleName: 'AHC 安瓶精華纖維面膜', brandName: 'AHC',
    transactionCount: 1, saleQuantity: 3, saleAmount: 114, returnTransactionCount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 3, netSalesAmount: 114,
  },
  {
    storeId: '107', storeLabel: '107 - Central', category1: 'NON FOOD', category1Code: 'B', category2: 'HOUSEHOLD', category2Code: 'B03',
    category3: 'CLEANING', category3Code: 'B0302', category4: 'SURFACE', category4Code: 'B030201', category5: 'WIPES', category5Code: 'B03020102', articleCode: '900001', articleName: 'Household wipes', brandName: 'Mannings',
    transactionCount: 2, saleQuantity: 5, saleAmount: 86, returnTransactionCount: 1, returnQuantity: 1, returnAmount: 12, netQuantity: 4, netSalesAmount: 74,
  },
  {
    storeId: '108', storeLabel: '108 - Harbour', category1: 'HEALTH & BEAUTY', category1Code: 'A', category2: 'BEAUTY CARE', category2Code: 'A02',
    category3: 'SKIN CARE', category3Code: 'A0201', category4: 'FACIAL', category4Code: 'A020101', category5: 'CLEANSER', category5Code: 'A02010102', articleCode: '285627', articleName: 'BF 深層卸妝潔膚水', brandName: 'Bifesta',
    transactionCount: 4, saleQuantity: 6, saleAmount: 239.4, returnTransactionCount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 6, netSalesAmount: 239.4,
  },
];

const pause = (duration = 90) => new Promise<void>((resolve) => setTimeout(resolve, duration));

function mockItemTotals(items: SalesAnalysisItem[]): SalesAnalysisTotals {
  return items.reduce((value, item) => ({
    saleQuantity: value.saleQuantity + item.saleQuantity,
    saleAmount: value.saleAmount + item.saleAmount,
    returnQuantity: value.returnQuantity + item.returnQuantity,
    returnAmount: value.returnAmount + item.returnAmount,
    netQuantity: value.netQuantity + item.netQuantity,
    netSalesAmount: value.netSalesAmount + item.netSalesAmount,
  }), {
    saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0,
  });
}

async function mockRunSalesAnalysis(request: SalesAnalysisRequest): Promise<SalesAnalysisResult> {
  mockSalesCancelled = false;
  const operationId = `mock-sales-${Date.now()}`;
  const catalog = mockAnalysisStoresFor(request.simulateStoreCount || mockAnalysisStores.length);
  const selected = catalog.filter((store) => request.storeIds.includes(store.businessId));
  const requestedPeriods = request.periods?.length ? request.periods : [{
    key: 'current', label: 'Current', from: request.from ?? '', to: request.to ?? '', includeTrend: false,
  }];
  const totalTasks = selected.length * requestedPeriods.length;
  mockSalesAnalysisListeners.forEach((listener) => listener({ operationId, current: 0, total: totalTasks, status: 'running' }));
  let completed = 0;
  for (const period of requestedPeriods) {
    for (const store of selected) {
      await pause(20);
      if (mockSalesCancelled) throw new AppError('cancelled', 'Sales analysis cancelled');
      completed += 1;
      mockSalesAnalysisListeners.forEach((listener) => listener({
        operationId, current: completed, total: totalTasks, storeId: store.businessId, storeLabel: store.label,
        periodKey: period.key, periodLabel: period.label, status: 'success',
      }));
    }
  }
  const scales: Record<string, number> = { current: 1, previous: 0.92, previous2: 0.84, yearAgo: 0.88 };
  const periods: SalesAnalysisPeriodResult[] = requestedPeriods.map((period) => {
    const scale = scales[period.key] ?? 1;
    const templates = mockAnalysisItems.length > 0 ? mockAnalysisItems : [];
    const items = selected.flatMap((store, storeIndex) => {
      const source = templates[storeIndex % templates.length] ?? templates[0];
      if (!source) return [];
      const storeScale = scale * (0.55 + storeIndex * 0.05);
      return [{
        ...source,
        storeId: store.businessId,
        storeLabel: store.label,
        saleQuantity: source.saleQuantity * storeScale,
        saleAmount: source.saleAmount * storeScale,
        returnQuantity: source.returnQuantity * storeScale,
        returnAmount: source.returnAmount * storeScale,
        netQuantity: source.netQuantity * storeScale,
        netSalesAmount: source.netSalesAmount * storeScale,
      }];
    });
    const stores = selected.map((store, index) => {
      const totals = mockItemTotals(items.filter((item) => item.storeId === store.businessId));
      if (period.includeTrend) {
        totals.trendNetSalesAmount = totals.netSalesAmount + (index + 1) * 4 * scale;
        totals.transactionCount = (26 - index * 3) * scale;
      }
      return { ...store, totals };
    });
    const totals = mockItemTotals(items);
    if (period.includeTrend) {
      totals.trendNetSalesAmount = stores.reduce((sum, store) => sum + (store.totals.trendNetSalesAmount ?? 0), 0);
      totals.transactionCount = stores.reduce((sum, store) => sum + (store.totals.transactionCount ?? 0), 0);
    }
    return {
      key: period.key, label: period.label, from: period.from, to: period.to, complete: true,
      successfulStores: selected.length, totals,
      stores, items, issues: [],
    };
  });
  const primary = periods[0]!;
  return {
    operationId, from: primary.from, to: primary.to, complete: true,
    selectedStores: selected.length, successfulStores: primary.successfulStores, totals: primary.totals,
    stores: primary.stores, items: primary.items, issues: [], periods,
    weeks: mockWeeklyPeriods(primary.from, primary.to, primary.stores),
    queryDurationMs: 180,
  };
}

function mockWeeklyPeriods(from: string, to: string, stores: SalesAnalysisPeriodResult['stores']): import('./types').SalesAnalysisWeek[] {
  if (!from || !to || from > to || stores.length === 0) return [];
  const start = new Date(`${from}T00:00:00`);
  const end = new Date(`${to}T00:00:00`);
  const weekday = start.getDay() || 7;
  start.setDate(start.getDate() - (weekday - 1));
  const weeks: import('./types').SalesAnalysisWeek[] = [];
  for (let cursor = new Date(start); cursor <= end; cursor.setDate(cursor.getDate() + 7)) {
    const weekFrom = cursor.toISOString().slice(0, 10);
    const weekToDate = new Date(cursor);
    weekToDate.setDate(weekToDate.getDate() + 6);
    const weekTo = weekToDate.toISOString().slice(0, 10);
    const weekStores = stores.map((store, index) => {
      const salesTw = 80_000 - index * 7_000;
      const salesLw = salesTw * 0.92;
      return {
        businessId: store.businessId, label: store.label,
        salesTw, salesLw, customersTw: 400 - index * 20, customersLw: 380 - index * 18,
        weekdaySalesTw: salesTw * 0.58, weekdaySalesLw: salesLw * 0.62,
        weekendSalesTw: salesTw * 0.42, weekendSalesLw: salesLw * 0.38,
        weekdayCustomersTw: 240 - index * 10, weekdayCustomersLw: 230 - index * 9,
        weekendCustomersTw: 160 - index * 10, weekendCustomersLw: 150 - index * 9,
      };
    });
    const totals = weekStores.reduce((sum, row) => ({
      salesTw: sum.salesTw + row.salesTw, salesLw: sum.salesLw + row.salesLw,
      customersTw: sum.customersTw + row.customersTw, customersLw: sum.customersLw + row.customersLw,
      weekdaySalesTw: sum.weekdaySalesTw + row.weekdaySalesTw, weekdaySalesLw: sum.weekdaySalesLw + row.weekdaySalesLw,
      weekendSalesTw: sum.weekendSalesTw + row.weekendSalesTw, weekendSalesLw: sum.weekendSalesLw + row.weekendSalesLw,
      weekdayCustomersTw: sum.weekdayCustomersTw + row.weekdayCustomersTw, weekdayCustomersLw: sum.weekdayCustomersLw + row.weekdayCustomersLw,
      weekendCustomersTw: sum.weekendCustomersTw + row.weekendCustomersTw, weekendCustomersLw: sum.weekendCustomersLw + row.weekendCustomersLw,
    }), {
      salesTw: 0, salesLw: 0, customersTw: 0, customersLw: 0,
      weekdaySalesTw: 0, weekdaySalesLw: 0, weekendSalesTw: 0, weekendSalesLw: 0,
      weekdayCustomersTw: 0, weekdayCustomersLw: 0, weekendCustomersTw: 0, weekendCustomersLw: 0,
    });
    weeks.push({ from: weekFrom, to: weekTo, stores: weekStores, totals });
  }
  return weeks;
}

function mockResult(operationId = 'mock-analysis'): AnalysisResult {
  return {
    operationId,
    complete: false,
    changedCellCount: 4,
    problemCount: 2,
    preview: sampleRows,
    rows: sampleRows,
    totalCount: sampleRows.length,
    changeCount: 2,
    unchangedCount: 1,
    issueCount: 1,
    failedCount: 1,
    retryableCount: 1,
    overlapCount: 1,
    overlapWarning: 'overlap_detected',
    issues: [
      { row: 21, message: 'store_not_mapped', retryable: false },
      { row: 22, message: 'query_failed', retryable: true },
    ],
    canApply: false,
  };
}

async function mockAnalyze(): Promise<AnalysisResult> {
  mockCancelled = false;
  const operationId = `mock-${Date.now()}`;
  const stages: AnalysisProgress['stage'][] = ['scan', 'login', 'stores', 'query', 'preview'];
  for (const [index, stage] of stages.entries()) {
    if (mockCancelled) throw new AppError('cancelled', 'Analysis cancelled');
    mockListeners.forEach((listener) => listener({ operationId, stage, current: index + 1, total: stages.length }));
    await pause();
  }
  return mockResult(operationId);
}

function eventSource(): EventSource | undefined {
  if (injection?.events) return injection.events;
  if (typeof window === 'undefined' || !window.runtime?.EventsOn) return undefined;
  return {
    on(name, listener) {
      const cleanup = window.runtime!.EventsOn!(name, listener);
      return typeof cleanup === 'function' ? cleanup : () => window.runtime?.EventsOff?.(name);
    },
  };
}

function fileDropSource(): FileDropSource | undefined {
  if (injection?.fileDrops) return injection.fileDrops;
  if (typeof window === 'undefined' || !window.runtime?.OnFileDrop) return undefined;
  return {
    on(listener) {
      window.runtime!.OnFileDrop!((_x, _y, paths) => listener(paths), true);
      return () => window.runtime?.OnFileDropOff?.();
    },
  };
}

export const backend: BackendApi = {
  openWorkbook: () => invoke(['OpenWorkbook'], [], async () => 'C:\\Users\\Demo\\Documents\\RTA-sales-2026-08.xlsx'),

  openMappingFile: () => invoke(['OpenMappingFile'], [], async () => 'C:\\Users\\Demo\\Documents\\store-map.local.json'),

  saveWorkbook: (request: SaveWorkbookRequest) =>
    invoke(['SaveWorkbook'], [request], async () => 'C:\\Users\\Demo\\Documents\\RTA-sales-2026-08-filled.xlsx'),

  scanWorkbook: (request: ScanWorkbookRequest) => invoke(['ScanWorkbook'], [request], async () => ({
    inputPath: request.inputPath,
    fileName: request.inputPath.split(/[\\/]/).pop(),
    sheetName: request.sheetName || '8月銷售',
    sheets: [
      { name: '8月銷售', dateMin: '2026-08-01', dateMax: '2026-08-14', rows: 384 },
      { name: '彙總', rows: 14 },
    ],
    dateMin: '2026-08-01',
    dateMax: '2026-08-14',
    dates: Array.from({ length: 14 }, (_, index) => `2026-08-${String(index + 1).padStart(2, '0')}`),
    rows: 384,
    stores: 27,
    jobs: 378,
    accounts: 2,
  })),

  listProfiles: () => invoke(['ListProfiles'], [], async () => [...mockProfiles]),

  saveProfile: (request: ProfileUpsertRequest) =>
    invoke(['CreateOrUpdateProfile', 'SaveProfile'], [request], async () => {
      const current = request.id ? mockProfiles.find((profile) => profile.id === request.id) : undefined;
      const saved: Profile = {
        id: current?.id ?? `profile-${Date.now()}`,
        displayName: request.displayName,
        enabled: request.enabled,
        priority: current?.priority ?? mockProfiles.length + 1,
        hasCredentials: Boolean(request.account && request.password) || current?.hasCredentials === true,
        accountHint: request.account ? `${request.account.slice(0, 2)}••••${request.account.slice(-2)}` : current?.accountHint,
        lastTestStatus: request.account || request.password ? 'untested' : current?.lastTestStatus ?? 'untested',
      };
      mockProfiles = current
        ? mockProfiles.map((profile) => profile.id === saved.id ? saved : profile)
        : [...mockProfiles, saved];
      return saved;
    }),

  testProfile: (profileId: string) =>
    invoke(['TestProfile'], [{ profileId }], async (): Promise<ProfileTestResult> => ({ success: true })),

  deleteProfile: (profileId: string) =>
    invoke(['DeleteProfile'], [{ profileId }], async () => { mockProfiles = mockProfiles.filter((profile) => profile.id !== profileId); }),

  reorderProfiles: (profileIds: string[]) =>
    invoke(['Reorder', 'ReorderProfiles'], [{ profileIds }], async () => {
      const byId = new Map(mockProfiles.map((profile) => [profile.id, profile]));
      mockProfiles = profileIds.flatMap((id, index) => {
        const profile = byId.get(id);
        return profile ? [{ ...profile, priority: index + 1 }] : [];
      });
      return [...mockProfiles];
    }),

  setProfileEnabled: (profileId: string, enabled: boolean) =>
    invoke(['Enable', 'SetProfileEnabled'], [{ profileId, enabled }], async () => {
      const found = mockProfiles.find((profile) => profile.id === profileId);
      if (!found) throw new AppError('profile_not_found', 'Profile not found');
      Object.assign(found, { enabled });
      return { ...found };
    }),

  listSalesAnalysisStores: (profileId: string, simulateStoreCount = 0) =>
    invoke(['ListSalesAnalysisStores'], [{ profileId, simulateStoreCount }], async () => mockAnalysisStoresFor(simulateStoreCount)),

  runSalesAnalysis: (request: SalesAnalysisRequest) =>
    invoke(['RunSalesAnalysis'], [request], () => mockRunSalesAnalysis(request)),

  getSalesAnalysisItems: (request: SalesAnalysisItemsRequest) =>
    invoke(['GetSalesAnalysisItems'], [request], async (): Promise<SalesAnalysisPackedItems> => ({
      periodKey: request.periodKey, dict: [''], rows: [],
    })),

  getSalesAnalysisReportGlyphs: (operationId: string) =>
    invoke(['GetSalesAnalysisReportGlyphs'], [{ operationId }], async () => ''),

  getSalesAnalysisReportMemo: (request: SalesAnalysisReportMemoRequest) =>
    invoke(['GetSalesAnalysisReportMemo'], [request], async (): Promise<SalesAnalysisReportMemo> => ({ periods: [] })),

  clearSalesAnalysis: (operationId: string) =>
    invoke(['ClearSalesAnalysis'], [{ operationId }], async () => undefined),

  cancelSalesAnalysis: (operationId: string) =>
    invoke(['CancelSalesAnalysis'], [{ operationId }], async () => { mockSalesCancelled = true; }),

  chooseSalesAnalysisPDFDirectory: () =>
    invoke(['ChooseSalesAnalysisPDFDirectory'], [], async () => 'C:\\Users\\Demo\\Documents\\RTA Reports'),

  writeSalesAnalysisPDF: (request: SalesAnalysisPDFWriteRequest) =>
    invoke(['WriteSalesAnalysisPDF'], [request], async () => `${request.directory}\\${request.filename}`),

  analyze: (request: AnalyzeRequest) => invoke(['Analyze'], [request], mockAnalyze),

  cancelAnalysis: (operationId: string) =>
    invoke(['Cancel', 'CancelAnalysis'], [{ operationId }], async () => { mockCancelled = true; }),

  retryFailed: (operationId: string) =>
    invoke(['RetryFailed'], [{ operationId }], async () => ({
      ...mockResult(operationId),
      failedCount: 0,
      complete: true,
      problemCount: 1,
      retryableCount: 0,
      issues: mockResult(operationId).issues.filter((issue) => !issue.retryable),
      preview: mockResult(operationId).preview.map((row) => row.status === 'failed' ? { ...row, status: 'unchanged' as const, message: undefined } : row),
    })),

  apply: (request: ApplyRequest) => invoke(['Apply'], [request], async (): Promise<ApplyResult> => ({
    outputPath: request.outputPath,
    changedCells: 4,
    skippedRows: request.allowPartial ? 2 : 0,
  })),

  openSavedWorkbook: (path: string) =>
    invoke(['OpenSavedWorkbook'], [{ path }], async () => undefined),

  revealSavedWorkbook: (path: string) =>
    invoke(['RevealSavedWorkbook'], [{ path }], async () => undefined),

  openSavedFolder: (path: string) =>
    invoke(['OpenSavedFolder'], [{ path }], async () => undefined),

  onProgress(listener: (progress: AnalysisProgress) => void): () => void {
    const source = eventSource();
    if (!source) {
      mockListeners.add(listener);
      return () => mockListeners.delete(listener);
    }

    const cleanups = ['rta:progress', 'analysis-progress'].map((name) => source.on(name, (payload) => listener(payload as AnalysisProgress)));
    return () => cleanups.forEach((cleanup) => cleanup?.());
  },

  onSalesAnalysisProgress(listener: (progress: SalesAnalysisProgress) => void): () => void {
    const source = eventSource();
    if (!source) {
      mockSalesAnalysisListeners.add(listener);
      return () => mockSalesAnalysisListeners.delete(listener);
    }
    const cleanup = source.on('rta:sales-analysis-progress', (payload) => listener(payload as SalesAnalysisProgress));
    return () => cleanup?.();
  },

  onFileDrop(listener: (paths: string[]) => void): () => void {
    return fileDropSource()?.on(listener) ?? (() => undefined);
  },
};
