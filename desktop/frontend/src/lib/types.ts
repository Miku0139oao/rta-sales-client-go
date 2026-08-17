export type Locale = 'zh-TW' | 'en';
export type Page = 'excel' | 'analysis' | 'accounts' | 'itemcodes' | 'settings';
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

export interface ManCodeGroup {
  id: string;
  name: string;
  codes: string[];
}

export interface SaveManCodeGroupRequest {
  id?: string;
  name: string;
  codes?: string[];
  raw?: string;
}

export interface ReplaceManCodeGroupCodesRequest {
  id: string;
  raw?: string;
  codes?: string[];
}

export interface ManCodeCatalogTransferResult {
  cancelled: boolean;
  path?: string;
  groups?: ManCodeGroup[];
}

export interface AnalysisProgress {
  operationId: string;
  stage: AnalysisStage;
  current: number;
  total: number;
  message?: string;
	date?: string;
	storeId?: string;
	profile?: string;
	attempt?: number;
	status?: 'success' | 'issue' | 'complete' | string;
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

export interface SalesAnalysisStore {
  businessId: string;
  label: string;
  profileId?: string;
  profile?: string;
}

export interface SalesAnalysisRequest {
  profileId?: string;
  storeIds: string[];
  from?: string;
  to?: string;
  periods?: SalesAnalysisPeriodRequest[];
  concurrency: number;
  simulateStoreCount?: number;
}

export interface SalesAnalysisPeriodRequest {
  key: string;
  label: string;
  from: string;
  to: string;
  includeTrend: boolean;
}

export interface SalesAnalysisItem {
  storeId: string;
  storeLabel: string;
  category1: string;
  category1Code?: string;
  category2: string;
  category2Code?: string;
  category3: string;
  category3Code?: string;
  category4: string;
  category4Code?: string;
  category5: string;
  category5Code?: string;
  articleCode: string;
  articleName: string;
  brandName?: string;
  transactionCount: number;
  saleQuantity: number;
  saleAmount: number;
  returnQuantity: number;
  returnTransactionCount: number;
  returnAmount: number;
  netQuantity: number;
  netSalesAmount: number;
}

export interface SalesAnalysisTotals {
  saleQuantity: number;
  saleAmount: number;
  returnQuantity: number;
  returnAmount: number;
  netQuantity: number;
  netSalesAmount: number;
  trendNetSalesAmount?: number;
  transactionCount?: number;
}

export interface SalesAnalysisStoreSummary {
  businessId: string;
  label: string;
  totals: SalesAnalysisTotals;
}

export interface SalesAnalysisIssue {
  periodKey?: string;
  storeId: string;
  storeLabel: string;
  message: string;
}

export interface SalesAnalysisPeriodResult {
  key: string;
  label: string;
  from: string;
  to: string;
  complete: boolean;
  successfulStores: number;
  totals: SalesAnalysisTotals;
  stores: SalesAnalysisStoreSummary[];
  items?: SalesAnalysisItem[];
  itemCount?: number;
  issues?: SalesAnalysisIssue[];
}

export interface SalesAnalysisItemsRequest {
  operationId: string;
  periodKey: string;
  storeId?: string;
}

export interface SalesAnalysisPackedRow {
  s: number;
  ac: number;
  an: number;
  br?: number;
  c1?: number;
  k1?: number;
  c2?: number;
  k2?: number;
  c3?: number;
  k3?: number;
  c4?: number;
  k4?: number;
  c5?: number;
  k5?: number;
  t?: number;
  sq?: number;
  sa?: number;
  rq?: number;
  rt?: number;
  ra?: number;
  nq?: number;
  ns?: number;
}

export interface SalesAnalysisPackedItems {
  periodKey: string;
  dict: string[];
  rows: SalesAnalysisPackedRow[];
}

export interface SalesAnalysisReportMemoRequest {
  operationId: string;
  storeId?: string;
  groupId?: string;
  categoryLevel: string;
  excludeZeroGifts: boolean;
  excludeStamps: boolean;
  mode: 'blacklist' | 'whitelist' | string;
  categories?: string[];
}

export interface SalesAnalysisRankedItem {
  id: string;
  code: string;
  name: string;
  brand?: string;
  amount: number;
  quantity: number;
  category2Code?: string;
  category3Code?: string;
  category4Code?: string;
}

export interface SalesAnalysisCategoryGroup {
  id: string;
  code?: string;
  name: string;
  amount: number;
  quantity: number;
  items?: SalesAnalysisRankedItem[];
}

export interface SalesAnalysisFocusProduct {
  id: string;
  code: string;
  name: string;
  brand?: string;
  amount: number;
  quantity: number;
  currentAmount: number;
  currentQuantity: number;
}

export interface SalesAnalysisFocusGroup {
  id: string;
  prefix: string;
  name?: string;
  sales?: SalesAnalysisFocusProduct[];
  quantity?: SalesAnalysisFocusProduct[];
}

export interface SalesAnalysisPeriodMemo {
  key: string;
  totals?: SalesAnalysisTotals;
  topAmount?: SalesAnalysisRankedItem[];
  topQuantity?: SalesAnalysisRankedItem[];
  amountGroups?: SalesAnalysisCategoryGroup[];
  quantityGroups?: SalesAnalysisCategoryGroup[];
  focusGroups?: SalesAnalysisFocusGroup[];
  focusCatalog?: boolean;
}

export interface SalesAnalysisReportMemo {
  periods: SalesAnalysisPeriodMemo[];
}

export interface SalesAnalysisResult {
  operationId: string;
  from: string;
  to: string;
  complete: boolean;
  pending?: boolean;
  selectedStores: number;
  successfulStores: number;
  totals: SalesAnalysisTotals;
  stores: SalesAnalysisStoreSummary[];
  items?: SalesAnalysisItem[];
  issues?: SalesAnalysisIssue[];
  periods?: SalesAnalysisPeriodResult[];
  weeks?: SalesAnalysisWeek[];
  queryDurationMs: number;
}

export interface SalesAnalysisWeek {
  from: string;
  to: string;
  stores: SalesAnalysisWeekStore[];
  totals: SalesAnalysisWeekStore;
}

export interface SalesAnalysisWeekStore {
  businessId?: string;
  label?: string;
  salesTw: number;
  salesLw: number;
  customersTw: number;
  customersLw: number;
  weekdaySalesTw: number;
  weekdaySalesLw: number;
  weekendSalesTw: number;
  weekendSalesLw: number;
  weekdayCustomersTw: number;
  weekdayCustomersLw: number;
  weekendCustomersTw: number;
  weekendCustomersLw: number;
}

export interface SalesAnalysisProgress {
  operationId: string;
  current: number;
  total: number;
  storeId?: string;
  storeLabel?: string;
  periodKey?: string;
  periodLabel?: string;
  status?: 'running' | 'success' | 'failed' | string;
}

export interface SalesAnalysisPDFWriteRequest {
  directory: string;
  filename: string;
  dataBase64: string;
}

export interface AppSettings {
  locale: Locale;
  theme: ThemePreference;
  maxJobs: number;
  accountConcurrency: number;
  useLocalMapping: boolean;
  mappingPath: string;
  simulateStoreCount: number;
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
  listManCodeGroups(): Promise<ManCodeGroup[]>;
  saveManCodeGroup(request: SaveManCodeGroupRequest): Promise<ManCodeGroup>;
  deleteManCodeGroup(id: string): Promise<void>;
  replaceManCodeGroupCodes(request: ReplaceManCodeGroupCodesRequest): Promise<ManCodeGroup>;
  exportManCodeCatalog(): Promise<ManCodeCatalogTransferResult>;
  importManCodeCatalog(): Promise<ManCodeCatalogTransferResult>;
  getLatestArticleNames(): Promise<Record<string, string>>;
  listSalesAnalysisStores(profileId: string, simulateStoreCount?: number): Promise<SalesAnalysisStore[]>;
  listManCodeGroups(): Promise<ManCodeGroup[]>;
  runSalesAnalysis(request: SalesAnalysisRequest): Promise<SalesAnalysisResult>;
  getSalesAnalysisItems(request: SalesAnalysisItemsRequest): Promise<SalesAnalysisPackedItems>;
  getSalesAnalysisReportGlyphs(operationId: string): Promise<string>;
  getSalesAnalysisReportMemo(request: SalesAnalysisReportMemoRequest): Promise<SalesAnalysisReportMemo>;
  clearSalesAnalysis(operationId: string): Promise<void>;
  cancelSalesAnalysis(operationId: string): Promise<void>;
  chooseSalesAnalysisPDFDirectory(): Promise<string>;
  writeSalesAnalysisPDF(request: SalesAnalysisPDFWriteRequest): Promise<string>;
  writeSalesAnalysisTextExport(request: SalesAnalysisPDFWriteRequest): Promise<string>;
  analyze(request: AnalyzeRequest): Promise<AnalysisResult>;
  cancelAnalysis(operationId: string): Promise<void>;
  retryFailed(operationId: string): Promise<AnalysisResult>;
  apply(request: ApplyRequest): Promise<ApplyResult>;
  openSavedWorkbook(path: string): Promise<void>;
  revealSavedWorkbook(path: string): Promise<void>;
  openSavedFolder(path: string): Promise<void>;
  onProgress(listener: (progress: AnalysisProgress) => void): () => void;
  onSalesAnalysisProgress(listener: (progress: SalesAnalysisProgress) => void): () => void;
  onSalesAnalysisUpdate(listener: (result: SalesAnalysisResult) => void): () => void;
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
