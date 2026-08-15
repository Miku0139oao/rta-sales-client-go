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
let mockCancelled = false;

const pause = (duration = 90) => new Promise<void>((resolve) => setTimeout(resolve, duration));

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

  onProgress(listener: (progress: AnalysisProgress) => void): () => void {
    const source = eventSource();
    if (!source) {
      mockListeners.add(listener);
      return () => mockListeners.delete(listener);
    }

    const cleanups = ['rta:progress', 'analysis-progress'].map((name) => source.on(name, (payload) => listener(payload as AnalysisProgress)));
    return () => cleanups.forEach((cleanup) => cleanup?.());
  },

  onFileDrop(listener: (paths: string[]) => void): () => void {
    return fileDropSource()?.on(listener) ?? (() => undefined);
  },
};
