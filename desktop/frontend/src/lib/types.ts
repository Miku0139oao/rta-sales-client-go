export type Locale = 'zh-TW' | 'en';
export type Page = 'excel' | 'accounts' | 'settings';
export type ThemePreference = 'system' | 'light' | 'dark';
export type ResolvedTheme = Exclude<ThemePreference, 'system'>;
export type AnalysisStage = 'scan' | 'login' | 'stores' | 'query' | 'preview';
export type PreviewStatus = 'change' | 'unchanged' | 'issue' | 'failed';
export type PreviewFilter = 'all' | 'change' | 'unchanged' | 'issue';

export interface SheetSummary {
  name: string;
  dateMin?: string;
  dateMax?: string;
  rows?: number;
}

export interface WorkbookScan {
  inputPath: string;
  fileName?: string;
  sheetName: string;
  sheets: SheetSummary[];
  dateMin: string;
  dateMax: string;
  dates: string[];
  rows: number;
  stores: number;
  jobs: number;
  accounts: number;
  warnings?: string[];
}

export interface ScanWorkbookRequest {
  inputPath: string;
  sheetName?: string;
  date?: string;
  from?: string;
  to?: string;
  mappingPath?: string;
}

export interface Profile {
  id: string;
  displayName: string;
  enabled: boolean;
  priority: number;
  hasCredentials: boolean;
  accountHint?: string;
  lastTestStatus?: 'success' | 'failed' | 'untested';
  lastTestMessage?: string;
}

export interface ProfileUpsertRequest {
  id?: string;
  displayName: string;
  account: string;
  password: string;
  enabled: boolean;
}

export interface ProfileTestResult {
  success: boolean;
  message?: string;
}

export interface AnalysisProgress {
  operationId: string;
  stage: AnalysisStage;
  current: number;
  total: number;
  message?: string;
}

export interface AnalyzeRequest {
  inputPath: string;
  sheetName: string;
  from: string;
  to: string;
  date?: string;
  maxJobs: number;
  maxQueries?: number;
  accountConcurrency: number;
  overwrite: boolean;
  useLocalMapping: boolean;
  mappingPath: string;
}

export interface PreviewRow {
  id: string;
  date: string;
  row: number;
  storeLabel: string;
  profileLabel: string;
  currentL: string;
  currentAB: string;
  proposedL: string;
  proposedAB: string;
  status: PreviewStatus;
  message?: string;
}

export interface AnalysisIssue {
  row?: number;
  message: string;
  retryable?: boolean;
}

export interface AnalysisResult {
  operationId: string;
  complete: boolean;
  changedCellCount: number;
  problemCount: number;
  aggregateProblemCount?: number;
  preview: PreviewRow[];
  rows?: PreviewRow[];
  totalCount: number;
  changeCount: number;
  unchangedCount: number;
  issueCount: number;
  failedCount: number;
  retryableCount?: number;
  pendingCount?: number;
  overlapCount: number;
  overlapWarning?: string;
  issues: AnalysisIssue[];
  canApply: boolean;
}

export interface ApplyRequest {
  operationId: string;
  inputPath: string;
  outputPath: string;
  overwrite: boolean;
  allowPartial: boolean;
  keepIssueOriginal: boolean;
}

export interface ApplyResult {
  outputPath: string;
  changedCells: number;
  skippedRows: number;
}

export interface SaveWorkbookRequest {
  inputPath: string;
  date?: string;
  from?: string;
  to?: string;
}

export interface AppSettings {
  locale: Locale;
  theme: ThemePreference;
  maxJobs: number;
  accountConcurrency: number;
  useLocalMapping: boolean;
  mappingPath: string;
}

export interface BackendApi {
  openWorkbook(): Promise<string>;
  openMappingFile(): Promise<string>;
  saveWorkbook(request: SaveWorkbookRequest): Promise<string>;
  scanWorkbook(request: ScanWorkbookRequest): Promise<WorkbookScan>;
  listProfiles(): Promise<Profile[]>;
  saveProfile(request: ProfileUpsertRequest): Promise<Profile>;
  testProfile(profileId: string): Promise<ProfileTestResult>;
  deleteProfile(profileId: string): Promise<void>;
  reorderProfiles(profileIds: string[]): Promise<Profile[]>;
  setProfileEnabled(profileId: string, enabled: boolean): Promise<Profile>;
  analyze(request: AnalyzeRequest): Promise<AnalysisResult>;
  cancelAnalysis(operationId: string): Promise<void>;
  retryFailed(operationId: string): Promise<AnalysisResult>;
  apply(request: ApplyRequest): Promise<ApplyResult>;
  onProgress(listener: (progress: AnalysisProgress) => void): () => void;
  onFileDrop(listener: (paths: string[]) => void): () => void;
}

export interface AppErrorShape {
  code: string;
  message: string;
}

export class AppError extends Error {
  code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = 'AppError';
    this.code = code;
  }
}
