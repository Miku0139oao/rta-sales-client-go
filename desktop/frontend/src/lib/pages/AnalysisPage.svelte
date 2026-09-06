<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { beginNativeExportLease, endNativeExportLease, backend } from '../backend';
  import { errorMessage } from '../i18n';
  import { isWebRuntime } from '../runtime';
  import { loadWebAnalysisSnapshot } from '../webStorage';
  import { buildFocusGroups, type FocusGroup } from '../analysisFocus';
  import {
    categoryCodeOf,
    categoryLabelOf,
    categoryNameOf,
    itemMatchesCategorySelection,
    periodKeysForView,
    periodNeedsItemHydration,
    unpackSalesAnalysisItems,
  } from '../salesAnalysisItems';
  import { modal } from '../modal';
  import AnalysisPresetsDialog from './AnalysisPresetsDialog.svelte';
  import AnalysisDataTable from './AnalysisDataTable.svelte';
  import AnalysisDataActions from './AnalysisDataActions.svelte';
  import ProductDetailsDialog from './ProductDetailsDialog.svelte';
  import AnalysisInsights from './AnalysisInsights.svelte';
  import { buildSalesInsights, salesInsightSheets } from '../salesInsights';
  import { buildAnalysisTables } from '../analysisTableViews';
  import type { TableSort } from '../analysisTable';
  import { ANALYSIS_PRESETS_KEY, loadAnalysisPresets, analysisPresetShortcuts, markAnalysisPresetUsed, normalizePresetDraft, resolvePresetQuery, type AnalysisPreset, type AnalysisPresetDraft, type PresetFilters } from '../analysisPresets';
  import {
    defaultSalesReportFilter,
    includeInSalesReport,
    reportCategoryId,
    type SalesReportFacets,
    type SalesReportFilter,
  } from '../salesReportItems';
  import {
    RANKING_LIMIT_CHOICES,
    RANKING_LIMIT_MAX,
    RANKING_LIMIT_MIN,
    isPresetRankingLimit,
    normalizeRankingLimit,
  } from '../settings';
  import { weeklySegmentRows } from '../storeSegment';
  import { alignRangeComparisonPeriods } from '../periodAlignment';
  import {
    bytesToBase64,
    generateSalesAnalysisPDF,
    prepareSalesAnalysisFontFromText,
    salesReportAccumulatorFromMemo,
    ALL_STORES_REPORT_ID,
    isAllStoresReport,
    listSuccessfulReportStores,
    salesAnalysisPDFFilename,
    type SalesReportChapter,
  } from '../sales-report-pdf';
  import { buildSalesAnalysisAIMarkdown, salesAnalysisAIFilename } from '../sales-report-ai';
  import type { Translator } from '../i18n';
  import type {
    AppSettings,
    Profile,
    SalesAnalysisItem,
    SalesAnalysisPeriodRequest,
    SalesAnalysisPeriodResult,
    SalesAnalysisProgress,
    SalesAnalysisReportMemo,
    SalesAnalysisResult,
    SalesAnalysisStore,
    SalesAnalysisTotals,
    SalesAnalysisWeek,
    ManCodeGroup,
  } from '../types';
  import { AppError } from '../types';

  export let t: Translator;
  export let settings: AppSettings;
  export let onBusyChange: (busy: boolean) => void = () => undefined;
  export let onSettingsChange: (next: AppSettings) => void = () => undefined;
  export let onGoToAccounts: () => void = () => undefined;

  type CategoryKey = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';
  type FacetSelections = Record<CategoryKey, Set<string>>;
  type ReportView = 'overview' | 'weekly' | 'focus' | 'categories' | 'products' | 'stores';
  type PeriodMode = 'month' | 'range';
  type QueryDraft = {
    profileId: string; periodMode: PeriodMode; month: string; from: string; to: string;
    weekCompare: boolean; storeIds: string[]; stores: SalesAnalysisStore[];
  };
  type ItemHydrationRun = {
    operationId: string; generation: number; keys: Set<string>;
    task?: Promise<SalesAnalysisPeriodResult[]>;
  };
  type ValueFormat = 'money' | 'number';
  type FilteredTotals = SalesAnalysisTotals & { skuCount: number; basketValue?: number };
  type PerformanceRow = { label: string; current?: number; previous?: number; yearAgo?: number; format: ValueFormat };
  type CategoryComparisonRow = {
    id: string; name: string; code: string; current: number; previous: number; previous2: number; yearAgo: number;
  };
  type TopItem = { id: string; code: string; name: string; brand: string; amount: number; quantity: number };
  type CategoryRankingGroup = {
    id: string; code: string; name: string; amount: number; quantity: number; items: TopItem[];
  };
  type StoreComparisonRow = {
    id: string; label: string; current?: SalesAnalysisTotals; previous?: SalesAnalysisTotals; yearAgo?: SalesAnalysisTotals;
  };

  const facets: Array<{ key: CategoryKey; label: string }> = [
    { key: 'category1', label: 'analysis.category1' },
    { key: 'category2', label: 'analysis.category2' },
    { key: 'category3', label: 'analysis.category3' },
    { key: 'category4', label: 'analysis.category4' },
    { key: 'category5', label: 'analysis.category5' },
  ];
  const pageSize = 50;

  let profiles: Profile[] = [];
  let profileId = '';
  let stores: SalesAnalysisStore[] = [];
  let selectedStoreIds = new Set<string>();
  let loadingProfiles = true;
  let loadingStores = false;
  let running = false;
  let loadingItems = false;
  let cancelling = false;
  let exportingPDF = false;
  let exportDialog = false;
  let exportFilter: SalesReportFilter = defaultSalesReportFilter();
  let exportIncludeCombined = true;
  let exportStoreIds = new Set<string>();
  let exportGroupIds = new Set<string>();
  let exportGroupDetail = false;
  let exportPDFEnabled = true;
  let exportAIEnabled = false;
  let exportUseScreenFilters = false;
  let exportCanUseScreenFilters = false;
  let exportTargetTotal = 0;
  let exportFilesCount = 0;
  let pdfExportCurrent = 0;
  let pdfExportTotal = 0;
  let error = '';
  let exportNotice = '';
  let exportDirectory = '';
  let openingFolder = false;
  let openFacet: CategoryKey | '' = '';
  let storeQuery = '';
  let result: SalesAnalysisResult | undefined;
  let progress: SalesAnalysisProgress | undefined;
  let operationId = '';
  let periodMode: PeriodMode = 'month';
  let weekCompare = false;
  let month = localISOMonth();
  let from = `${month}-01`;
  let to = localISODate();
  let activeView: ReportView = 'overview';
  let search = '';
  let manCodeGroups: ManCodeGroup[] = [];
  let selectedGroupId = '';
  let selectedGroup: ManCodeGroup | undefined;
  let selectedGroupCodes = new Set<string>();
  let groupScopeActive = false;
  let groupLevel: CategoryKey = 'category2';
  let salesRankingKey = 'current';
  let quantityRankingKey = 'current';
  let selections = emptySelections();
  let reportPeriods: SalesAnalysisPeriodResult[] = [];
  let currentPeriod: SalesAnalysisPeriodResult | undefined;
  let filteredItems: SalesAnalysisItem[] = [];
  let currentTotals: FilteredTotals = emptyTotals();
  let previousTotals: FilteredTotals = emptyTotals();
  let previous2Totals: FilteredTotals = emptyTotals();
  let yearAgoTotals: FilteredTotals = emptyTotals();
  let performanceRows: PerformanceRow[] = [];
  let categoryRows: CategoryComparisonRow[] = [];
  let topSales: TopItem[] = [];
  let topQuantity: TopItem[] = [];
  let salesRankingPeriods: SalesAnalysisPeriodResult[] = [];
  let quantityRankingPeriods: SalesAnalysisPeriodResult[] = [];
  let salesRankingPeriod: SalesAnalysisPeriodResult | undefined;
  let quantityRankingPeriod: SalesAnalysisPeriodResult | undefined;
  let salesRankingGroups: CategoryRankingGroup[] = [];
  let quantityRankingGroups: CategoryRankingGroup[] = [];
  let storeRows: StoreComparisonRow[] = [];
  let focusGroups: FocusGroup[] = [];
  let focusPeriod: SalesAnalysisPeriodResult | undefined;
  let weeklyKey = '';
  let weeklyUsesAlignedComparison = false;
  let page = 1;
  let pageCount = 1;
  let pageRows: SalesAnalysisItem[] = [];
  let loadedSimulateCount: number | undefined;
  let hydrateGeneration = 0;
  let hydrationRun: ItemHydrationRun | undefined;
  let hydrationOperationId = '';
  let hydrationFailures: Record<string, string> = {};
  let hydratingPeriod = '';
  let storeLoadGeneration = 0;
  let queryOpen = false;
  let lastRunKey = '';
  let appliedQuery: QueryDraft | undefined;
  let tableSorts: Record<string, TableSort> = {};
  let exportingData = false;
  let selectedProduct: { code: string; name: string; periodKey: string } | undefined;
  let presetsOpen = false;
  let presetSaveDraft: AnalysisPresetDraft | undefined;
  let savedPresets: AnalysisPreset[] = [];
  let shortcutBusy = false;
  let shortcutNotice = '';
  $: shortcutGroups = analysisPresetShortcuts(savedPresets);
  let stagedPreset: { id: string; name: string; filters: PresetFilters } | undefined;
  let beforePresetQuery: QueryDraft | undefined;
  let presetWarning = '';
  let reportAccount = '';
  let filtersOpen = false;
  let facetSearch = '';
  let reportWorkspace: HTMLDivElement | undefined;
  let reportNavigation: HTMLDivElement | undefined;
  let reportFilter: HTMLElement | undefined;
  let productSearch: HTMLInputElement | undefined;
  let rankingFocused = false;
  let rankingInput = String(settings.rankingLimit);
  const prefetchPeriodKeys = ['current', 'previous', 'previous2', 'yearAgo', 'yearAgoNext'];

  $: if (!loadingProfiles && profileId && loadedSimulateCount !== settings.simulateStoreCount) {
    loadedSimulateCount = settings.simulateStoreCount;
    void loadStores();
  }
  $: busy = loadingProfiles || loadingStores || running || Boolean(result?.pending);
  $: visibleStores = filterStores(stores, storeQuery);
  $: onBusyChange(running || exportingPDF || exportingData || Boolean(result?.pending));
  $: rangeInvalid = periodMode === 'range' && Boolean(from && to && from > to);
  $: reportPeriods = normalizePeriods(result);
  $: currentPeriod = periodByKey(reportPeriods, 'current') ?? reportPeriods[0];
  $: currentReady = Boolean(
    currentPeriod && ((currentPeriod.items?.length ?? 0) > 0 || (currentPeriod.topAmount?.length ?? 0) > 0 || (currentPeriod.itemCount ?? 0) > 0),
  );
  $: draftQueryKey = JSON.stringify({
    profileId, periodMode,
    period: periodMode === 'month' ? month : `${from}:${to}:${weekCompare}`,
    storeIds: [...selectedStoreIds].sort(),
  });
  $: queryDirty = Boolean(result && lastRunKey && (draftQueryKey !== lastRunKey || stagedPreset));
  $: activeFilterCount = facets.reduce((count, facet) => count + selections[facet.key].size, 0)
    + (search.trim() ? 1 : 0) + (selectedGroupId ? 1 : 0);
  $: reportTabs = [
    { key: 'overview', label: t('analysis.overview'), icon: 'space_dashboard' },
    { key: 'weekly', label: t('analysis.weekly'), icon: 'calendar_view_week' },
    { key: 'focus', label: t('analysis.focus'), icon: 'upcoming' },
    { key: 'categories', label: t('analysis.categories'), icon: 'account_tree' },
    { key: 'products', label: t('analysis.products'), icon: 'inventory_2' },
    { key: 'stores', label: t('analysis.stores'), icon: 'storefront' },
  ].filter((tab) => !groupScopeActive || tab.key !== 'weekly');
  $: neededPeriodKeys = result
    ? prefetchPeriodKeys.filter((key) => (result?.periods ?? []).some((period) => period.key === key))
    : periodKeysForAnalysisView(
      activeView,
      [salesRankingKey, quantityRankingKey],
      groupScopeActive || filtersActive(selections, search),
    );
  $: if (result && !exportingPDF && !running) void ensurePeriodItems(neededPeriodKeys);
  $: currentDetailsMissing = periodNeedsItemHydration(currentPeriod);
  $: itemFailureRows = Object.entries(hydrationFailures).map(([key, message]) => ({
    key, label: periodByKey(reportPeriods, key)?.label ?? key, message,
  }));
  $: selectedGroup = manCodeGroups.find((group) => group.id === selectedGroupId);
  $: selectedGroupCodes = new Set((selectedGroup?.codes ?? []).map((code) => code.trim()).filter(Boolean));
  $: groupScopeActive = Boolean(selectedGroup);
  $: filteredItems = (currentPeriod?.items ?? []).filter((item) => {
    if (!matchesFilters(item, selections, search)) return false;
    return !groupScopeActive || selectedGroupCodes.has(item.articleCode.trim());
  });
  $: productScopeActive = groupScopeActive || filtersActive(selections, search);
  $: insightData = activeView === 'overview' ? buildSalesInsights(currentPeriod, periodByKey(reportPeriods, 'previous'),
    (item) => matchesFilters(item, selections, search) && (!groupScopeActive || selectedGroupCodes.has(item.articleCode.trim()))) : undefined;
  $: insightTable = insightData ? salesInsightSheets(insightData, t) : undefined;
  $: viewItemKeys = periodKeysForAnalysisView(activeView, [salesRankingKey, quantityRankingKey], productScopeActive);
  $: viewMissingPeriods = reportPeriods.filter((period) => viewItemKeys.includes(period.key) && periodNeedsItemHydration(period));
  $: scopedViewWaiting = (productScopeActive || activeView === 'products') && viewMissingPeriods.length > 0;
  $: facetOptionMap = listFacetOptionMap(currentPeriod, selections, selectedGroupCodes, groupScopeActive, settings.locale);
  $: currentTotals = totalsForPeriod(currentPeriod, selections, search, selectedGroupCodes, groupScopeActive);
  $: previousTotals = totalsForPeriod(periodByKey(reportPeriods, 'previous'), selections, search, selectedGroupCodes, groupScopeActive);
  $: previous2Totals = totalsForPeriod(periodByKey(reportPeriods, 'previous2'), selections, search, selectedGroupCodes, groupScopeActive);
  $: yearAgoTotals = totalsForPeriod(periodByKey(reportPeriods, 'yearAgo'), selections, search, selectedGroupCodes, groupScopeActive);
  $: performanceRows = buildPerformanceRows(
    currentTotals,
    periodByKey(reportPeriods, 'previous') ? previousTotals : undefined,
    periodByKey(reportPeriods, 'yearAgo') ? yearAgoTotals : undefined,
  );
  $: categoryRows = buildCategoryComparison(reportPeriods, groupLevel, selections, search, selectedGroupCodes, groupScopeActive);
  $: topSales = (productScopeActive || (currentPeriod?.items?.length ?? 0) > 0
    ? buildTopItems(filteredItems, 'amount')
    : rankedToTopItems(currentPeriod?.topAmount)).slice(0, settings.rankingLimit);
  $: topQuantity = (productScopeActive || (currentPeriod?.items?.length ?? 0) > 0
    ? buildTopItems(filteredItems, 'quantity')
    : rankedToTopItems(currentPeriod?.topQuantity)).slice(0, settings.rankingLimit);
  $: salesRankingPeriods = reportPeriods.filter((period) => ['current', 'yearAgo', 'yearAgoNext'].includes(period.key));
  $: quantityRankingPeriods = reportPeriods.filter((period) => ['current', 'previous', 'previous2'].includes(period.key));
  $: salesRankingPeriod = periodByKey(salesRankingPeriods, salesRankingKey) ?? salesRankingPeriods[0];
  $: quantityRankingPeriod = periodByKey(quantityRankingPeriods, quantityRankingKey) ?? quantityRankingPeriods[0];
  $: salesRankingGroups = buildCategoryRankings(salesRankingPeriod, groupLevel, 'amount', selections, search, selectedGroupCodes, groupScopeActive, settings.rankingLimit);
  $: quantityRankingGroups = buildCategoryRankings(quantityRankingPeriod, groupLevel, 'quantity', selections, search, selectedGroupCodes, groupScopeActive, settings.rankingLimit);
  $: storeRows = buildStoreRows(reportPeriods, selections, search, selectedGroupCodes, groupScopeActive);
  $: focusPeriod = periodByKey(reportPeriods, 'yearAgoNext');
  $: focusGroups = focusGroupsForScope(
    focusPeriod, currentPeriod, selections, search, selectedGroup, manCodeGroups, selectedGroupCodes, groupScopeActive,
  );
  $: pageCount = Math.max(1, Math.ceil(filteredItems.length / pageSize));
  $: if (page > pageCount) page = pageCount;
  $: pageRows = filteredItems.slice((page - 1) * pageSize, page * pageSize);
  $: dataTables = buildAnalysisTables({ items: filteredItems, performance: performanceRows, categories: categoryRows, stores: storeRows,
    periods: reportPeriods, week: weeklyWeek, weekAligned: weeklyUsesAlignedComparison, topSales, topQuantity,
    salesGroups: salesRankingGroups, quantityGroups: quantityRankingGroups, focus: focusGroups, insights: insightTable }, t, settings.locale, tableSorts);
  $: dataContext = [t('analysis.querySummary', { account: reportAccount || t('analysis.savedReport'), period: result ? `${result.from} — ${result.to}` : '', count: result?.selectedStores ?? 0 }),
    'HKD', ...reportPeriods.map((period) => `${period.label}: ${period.from} — ${period.to}${period.complete ? '' : ` (${t('data.partial')})`}`),
    ...facets.flatMap(({ key, label }) => selections[key].size ? [`${t(label)}: ${[...selections[key]].join(', ')}`] : []),
    ...(search ? [`${t('analysis.search')}: ${search}`] : []), ...(selectedGroup ? [`${t('analysis.promoterGroup')}: ${selectedGroup.name}`] : []),
    ...(result && (!result.complete || result.pending) ? [t('data.partial')] : []),
    ...(activeView === 'overview' && insightData ? [t('insights.method'), t('insights.scope'), ...(insightData.reason !== 'ready' ? [t(`insights.reason.${insightData.reason}`)] : [])] : []),
    ...(activeView === 'weekly' && weeklyWeek ? [`${t('analysis.weekly')}: ${weeklyWeek.from} — ${weeklyWeek.to}`, t('data.weeklyScope')] : []),
    ...(activeView === 'categories' ? [`${t('analysis.categorySalesRanking')}: ${salesRankingPeriod?.label ?? ''}`, `${t('analysis.monthlyQuantityRanking')}: ${quantityRankingPeriod?.label ?? ''}`] : [])];

  function sortData(id: string, sort: TableSort) { tableSorts = { ...tableSorts, [id]: sort }; }
  function openProduct(code: string, name: string, periodKey = 'current') {
    if (!code || exportingData) return;
    selectedProduct = { code, name, periodKey };
    void ensurePeriodItems(reportPeriods.map((period) => period.key));
  }
  $: weeklyPeriods = result?.weeks ?? [];
  $: weeklyWeek = weeklyPeriods.find((week) => `${week.from}:${week.to}` === weeklyKey) ?? weeklyPeriods[0];
  $: weeklyUsesAlignedComparison = hasWeekAlignedComparison(reportPeriods);
  $: if (weeklyPeriods.length && !weeklyPeriods.some((week) => `${week.from}:${week.to}` === weeklyKey)) {
    weeklyKey = `${weeklyPeriods[0]!.from}:${weeklyPeriods[0]!.to}`;
  }
  $: progressPercent = progress?.total ? Math.round((progress.current / progress.total) * 100) : 0;
  $: exportTargetTotal = (result, exportIncludeCombined, exportStoreIds, exportTargetCount());
  $: exportFilesCount = (exportTargetTotal, exportPDFEnabled, exportAIEnabled, exportGroupDetail, exportGroupIds, manCodeGroups, exportOutputCount());

  onMount(() => {
    reloadShortcuts();
    const syncPresets = (event: StorageEvent) => { if (event.key === null || event.key === ANALYSIS_PRESETS_KEY) reloadShortcuts(); };
    window.addEventListener('storage', syncPresets);
    const unsubscribe = backend.onSalesAnalysisProgress((next) => {
      progress = next;
      operationId = next.operationId;
    });
    const unsubscribeUpdate = backend.onSalesAnalysisUpdate((next) => {
      if (result?.operationId && next.operationId === result.operationId) result = keepHydratedItems(result, next);
    });
    const closeFacetMenus = (event: PointerEvent) => {
      const target = event.target as HTMLElement | null;
      if (!target?.closest('.facet-menu')) openFacet = '';
    };
    document.addEventListener('pointerdown', closeFacetMenus);
    document.addEventListener('keydown', closeFacetOnEscape);
    void initialize();
    return () => {
      window.removeEventListener('storage', syncPresets);
      unsubscribe();
      unsubscribeUpdate();
      document.removeEventListener('pointerdown', closeFacetMenus);
      document.removeEventListener('keydown', closeFacetOnEscape);
      resetHydration();
      ++storeLoadGeneration;
    };
  });

  async function initialize() {
    loadingProfiles = true;
    error = '';
    try {
      const [listedProfiles, listedGroups] = await Promise.all([
        backend.listProfiles(),
        backend.listManCodeGroups().catch(() => [] as ManCodeGroup[]),
      ]);
      profiles = listedProfiles.filter((profile) => profile.enabled && profile.hasCredentials);
      manCodeGroups = listedGroups;
      profileId = profiles[0]?.id ?? '';
      loadedSimulateCount = settings.simulateStoreCount;
      if (isWebRuntime()) {
        const saved = loadWebAnalysisSnapshot();
        if (saved?.pending) {
          await backend.clearSalesAnalysis(saved.operationId).catch(() => undefined);
        } else if (saved) {
          result = saved;
        }
      }
      if (profileId) await loadStores({ keepResult: Boolean(result) });
      if (result) {
        const current = normalizePeriods(result).find((period) => period.key === 'current');
        periodMode = 'range';
        from = current?.from ?? result.from;
        to = current?.to ?? result.to;
        selectedStoreIds = new Set(result.stores.map((store) => store.businessId));
        lastRunKey = currentQueryKey();
        appliedQuery = captureQuery();
        // Saved reports do not carry an account identity. Never attribute them to the first account.
        reportAccount = '';
      }
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      loadingProfiles = false;
    }
  }

  function captureQuery(): QueryDraft {
    return { profileId, periodMode, month, from, to, weekCompare,
      storeIds: [...selectedStoreIds], stores: stores.map((store) => ({ ...store })) };
  }

  function restoreQuery() {
    const saved = appliedQuery ?? beforePresetQuery;
    if (!saved || running || exportingPDF) return;
    // An account switch may still be loading. Ignore that reply after restoring the draft.
    ++storeLoadGeneration;
    loadingStores = false;
    ({ profileId, periodMode, month, from, to, weekCompare } = saved);
    stores = saved.stores.map((store) => ({ ...store }));
    selectedStoreIds = new Set(saved.storeIds);
    stagedPreset = undefined;
    beforePresetQuery = undefined;
    presetWarning = '';
    storeQuery = '';
    error = '';
  }

  function reloadShortcuts() {
    try { savedPresets = loadAnalysisPresets(); shortcutNotice = ''; }
    catch { savedPresets = []; shortcutNotice = t('presets.readError'); }
  }
  function closePresets() { presetsOpen = false; reloadShortcuts(); }
  async function quickApplyPreset(id: string) {
    if (shortcutBusy || loadingProfiles || loadingStores || running || exportingPDF || exportingData) return;
    shortcutBusy = true; shortcutNotice = '';
    try {
      // Re-read before applying so deleted/changed entries in another window are not reused.
      savedPresets = loadAnalysisPresets();
      const preset = savedPresets.find((preset) => preset.id === id);
      shortcutNotice = preset ? (await stagePreset(preset) ?? '') : t('presets.applyError');
    } catch { shortcutNotice = t('presets.readError'); }
    finally { shortcutBusy = false; }
  }
  function openPresets() {
    if (loadingProfiles || loadingStores || running || exportingPDF || exportingData) return;
    try {
      const profile = profiles.find((profile) => profile.id === profileId);
      if (!profile) throw new Error('No account');
      presetSaveDraft = normalizePresetDraft({
        query: { ...captureQuery(), profileName: profile.displayName, monthMode: 'fixed',
          // Inactive date fields should not prevent saving a valid period.
          ...(periodMode === 'month' ? { from: `${month}-01`, to: `${month}-01` } : { month: from.slice(0, 7) }) },
        filters: stagedPreset?.filters ?? { search, groupId: selectedGroupId, groupLevel,
          categories: Object.fromEntries(facets.map(({ key }) => [key, [...selections[key]]])) },
      });
    } catch { presetSaveDraft = undefined; }
    presetsOpen = true;
  }

  async function stagePreset(preset: AnalysisPreset): Promise<string | undefined> {
    if (loadingProfiles || loadingStores || running || exportingPDF || exportingData) return t('presets.applyError');
    if (!profiles.some((profile) => profile.id === preset.query.profileId)) return t('presets.accountMissing');
    if (preset.filters.groupId && !manCodeGroups.some((group) => group.id === preset.filters.groupId)) return t('presets.groupMissing');
    const generation = ++storeLoadGeneration;
    loadingStores = true;
    try {
      const query = resolvePresetQuery(preset.query);
      const listed = await backend.listSalesAnalysisStores(query.profileId, settings.simulateStoreCount);
      if (generation !== storeLoadGeneration) return t('presets.applyError');
      const available = new Set(listed.map((store) => store.businessId));
      const validIds = query.storeIds.filter((id) => available.has(id));
      if (!validIds.length) return t('presets.noStores');
      const missing = query.storeIds.filter((id) => !available.has(id));
      beforePresetQuery ??= captureQuery();
      ({ profileId, periodMode, month, from, to, weekCompare } = query);
      stores = listed;
      selectedStoreIds = new Set(validIds);
      storeQuery = '';
      stagedPreset = { id: preset.id, name: preset.name, filters: normalizePresetDraft(preset).filters };
      presetWarning = missing.length ? t('presets.skippedStores', { count: missing.length, ids: missing.join(', ') }) : '';
      queryOpen = true;
      error = '';
      return undefined;
    } catch { return t('presets.applyError'); }
    finally { if (generation === storeLoadGeneration) loadingStores = false; }
  }

  function finishPresetApplication(submitted: typeof stagedPreset) {
    if (submitted) {
      search = submitted.filters.search;
      selectedGroupId = submitted.filters.groupId;
      groupLevel = submitted.filters.groupLevel;
      selections = Object.fromEntries(facets.map(({ key }) => [key, new Set(submitted.filters.categories[key])])) as FacetSelections;
      try { savedPresets = markAnalysisPresetUsed(submitted.id); shortcutNotice = ''; }
      catch { shortcutNotice = t('presets.historyWriteError'); }
      page = 1;
    }
    stagedPreset = undefined;
    beforePresetQuery = undefined;
    presetWarning = '';
  }

  function currentQueryKey(): string {
    return JSON.stringify({
      profileId,
      periodMode,
      period: periodMode === 'month' ? month : `${from}:${to}:${weekCompare}`,
      storeIds: [...selectedStoreIds].sort(),
    });
  }

  async function loadStores(options: { keepResult?: boolean } = {}) {
    if (!profileId) return;
    const generation = ++storeLoadGeneration;
    loadingStores = true;
    error = '';
    if (!options.keepResult && !result) void discardResult();
    storeQuery = '';
    stores = [];
    selectedStoreIds = new Set<string>();
    try {
      const listed = await backend.listSalesAnalysisStores(profileId, settings.simulateStoreCount);
      if (generation !== storeLoadGeneration) return;
      stores = listed;
      selectedStoreIds = new Set(listed.map((store) => store.businessId));
    } catch (caught) {
      if (generation !== storeLoadGeneration) return;
      error = errorMessage(settings.locale, caught);
    } finally {
      if (generation === storeLoadGeneration) loadingStores = false;
    }
  }

  function changeQuery() {
    dismissExportNotice();
    queryOpen = !queryOpen;
  }

  async function selectReport(view: ReportView) {
    activeView = view;
    openFacet = '';
    await tick();
    const main = reportWorkspace?.closest('main');
    if (!main || !reportWorkspace || !reportNavigation) return;
    const panelTop = reportWorkspace.getBoundingClientRect().top;
    const navBottom = reportNavigation.getBoundingClientRect().bottom;
    // Switching views from deep in a long ranking should start at the new report, not mid-list.
    if (panelTop < navBottom) main.scrollTop += panelTop - navBottom - 16;
  }

  function returnToFilters() {
    const main = reportFilter?.closest('main');
    if (main && reportFilter && reportNavigation) {
      main.scrollTop += reportFilter.getBoundingClientRect().top - reportNavigation.getBoundingClientRect().bottom - 12;
    }
    productSearch?.focus({ preventScroll: true });
  }

  function navigateReport(event: KeyboardEvent) {
    const buttons = [...(event.currentTarget as HTMLElement).parentElement!.querySelectorAll<HTMLButtonElement>('[role="tab"]')];
    const index = buttons.indexOf(event.target as HTMLButtonElement);
    if (index < 0) return;
    const next = event.key === 'ArrowRight' ? (index + 1) % buttons.length
      : event.key === 'ArrowLeft' ? (index + buttons.length - 1) % buttons.length
      : event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : -1;
    if (next < 0) return;
    event.preventDefault();
    buttons[next]?.click();
    buttons[next]?.focus();
  }

  function clearScreenFilters() {
    selections = emptySelections();
    search = '';
    selectedGroupId = '';
    openFacet = '';
    facetSearch = '';
    page = 1;
  }

  function closeFacetOnEscape(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !openFacet) return;
    const menu = (event.target as HTMLElement | null)?.closest('.facet-menu');
    if (!menu) return;
    event.preventDefault();
    openFacet = '';
    menu.querySelector('summary')?.focus();
  }

  $: if (!rankingFocused) rankingInput = String(settings.rankingLimit);

  function setRankingLimit(limit: number) {
    const next = normalizeRankingLimit(limit);
    if (next === settings.rankingLimit) return;
    onSettingsChange({ ...settings, rankingLimit: next });
  }

  function commitRankingInput() {
    rankingFocused = false;
    const value = rankingInput.trim() ? Number(rankingInput) : settings.rankingLimit;
    setRankingLimit(value);
    rankingInput = String(normalizeRankingLimit(value));
  }

  function toggleStore(storeId: string) {
    const next = new Set(selectedStoreIds);
    if (next.has(storeId)) next.delete(storeId);
    else next.add(storeId);
    selectedStoreIds = next;
  }

  function selectAllStores() {
    selectedStoreIds = new Set([...selectedStoreIds, ...visibleStores.map((store) => store.businessId)]);
  }

  function clearStores() {
    const visibleIds = new Set(visibleStores.map((store) => store.businessId));
    selectedStoreIds = new Set([...selectedStoreIds].filter((id) => !visibleIds.has(id)));
  }

  async function discardResult() {
    selectedProduct = undefined;
    resetHydration();
    const operationId = result?.operationId;
    result = undefined;
    lastRunKey = '';
    appliedQuery = undefined;
    queryOpen = false;
    if (!operationId) return;
    try {
      await backend.clearSalesAnalysis(operationId);
    } catch {
      /* The next analysis overwrites any leftover cache. */
    }
  }

  async function loadWebPreview() {
    if (running) return;
    if (profileId && selectedStoreIds.size > 0 && !rangeInvalid) {
      await runAnalysis();
      return;
    }
    const stores = previewStoreIds();
    const submittedKey = currentQueryKey();
    const submittedAccount = profiles.find((profile) => profile.id === profileId)?.displayName ?? '';
    const submittedQuery = captureQuery();
    const submittedPreset = stagedPreset;
    running = true;
    cancelling = false;
    error = '';
    void discardResult();
    progress = undefined;
    operationId = '';
    resetFilters();
    try {
      const summary = await backend.runSalesAnalysis({
        profileId: profileId || 'web-preview',
        storeIds: stores,
        periods: buildPeriodRequests(),
        concurrency: settings.accountConcurrency,
        simulateStoreCount: Math.max(settings.simulateStoreCount, stores.length, 2),
      });
      result = summary;
      operationId = summary.operationId;
      activeView = 'overview';
      queryOpen = false;
      lastRunKey = submittedKey;
      appliedQuery = submittedQuery;
      reportAccount = submittedAccount;
      finishPresetApplication(submittedPreset);
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      running = false;
      cancelling = false;
    }
  }

  function previewStoreIds(): string[] {
    if (selectedStoreIds.size > 0) return [...selectedStoreIds];
    if (stores.length > 0) return stores.map((store) => store.businessId);
    return ['107', '108'];
  }

  async function runAnalysis() {
    if (!profileId || selectedStoreIds.size === 0 || rangeInvalid) return;
    const periods = buildPeriodRequests();
    if (running || exportingData || loadingStores || periods.length === 0) return;
    const submittedKey = currentQueryKey();
    const submittedAccount = profiles.find((profile) => profile.id === profileId)?.displayName ?? profileId;
    const submittedQuery = captureQuery();
    const submittedPreset = stagedPreset;
    // Lock the draft before awaiting cancellation; a rapid second click must not start another run.
    running = true;
    if (operationId) {
      try { await backend.cancelSalesAnalysis(operationId); } catch { /* previous run may already be finished */ }
    }
    cancelling = false;
    error = '';
    void discardResult();
    progress = undefined;
    operationId = '';
    resetFilters();
    try {
      const summary = await backend.runSalesAnalysis({
        profileId,
        storeIds: [...selectedStoreIds],
        periods,
        concurrency: settings.accountConcurrency,
        simulateStoreCount: settings.simulateStoreCount,
      });
      result = summary;
      operationId = summary.operationId;
      activeView = 'overview';
      queryOpen = false;
      lastRunKey = submittedKey;
      appliedQuery = submittedQuery;
      reportAccount = submittedAccount;
      finishPresetApplication(submittedPreset);
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      running = false;
      cancelling = false;
    }
  }

  function resetHydration() {
    ++hydrateGeneration;
    hydrationRun = undefined;
    hydrationOperationId = '';
    hydrationFailures = {};
    hydratingPeriod = '';
    loadingItems = false;
  }

  function ensurePeriodItems(keys: string[]): Promise<SalesAnalysisPeriodResult[]> {
    const summary = result;
    if (!summary?.periods?.length || !summary.operationId) return Promise.resolve(summary?.periods ?? []);
    if (hydrationOperationId !== summary.operationId) {
      resetHydration();
      hydrationOperationId = summary.operationId;
    }
    const missing = keys.filter((key) => !hydrationFailures[key]
      && periodNeedsItemHydration(summary.periods!.find((period) => period.key === key)));
    if (hydrationRun) {
      for (const key of missing) hydrationRun.keys.add(key);
      return hydrationRun.task!;
    }
    if (!missing.length) return Promise.resolve(summary.periods);
    const run: ItemHydrationRun = { operationId: summary.operationId, generation: hydrateGeneration, keys: new Set(missing) };
    hydrationRun = run;
    loadingItems = true;
    run.task = hydratePeriodQueue(run);
    return run.task;
  }

  async function hydratePeriodQueue(run: ItemHydrationRun): Promise<SalesAnalysisPeriodResult[]> {
    const stillCurrent = () => run.generation === hydrateGeneration && result?.operationId === run.operationId;
    try {
      while (run.keys.size && stillCurrent()) {
        const key = run.keys.values().next().value!;
        run.keys.delete(key);
        const period = result?.periods?.find((period) => period.key === key);
        if (!period || !periodNeedsItemHydration(period) || hydrationFailures[key]) continue;
        hydratingPeriod = period.label;
        try {
          const packed = await backend.getSalesAnalysisItems({ operationId: run.operationId, periodKey: key });
          if (!stillCurrent() || !result) break;
          const latest = result.periods!.find((period) => period.key === key)!;
          const storeList = (latest.stores?.length ? latest.stores : result.stores) ?? [];
          const items = unpackSalesAnalysisItems(packed, storeList);
          if (items.length < (latest.itemCount ?? 0)) {
            hydrationFailures = { ...hydrationFailures, [key]: t('analysis.itemsIncomplete') };
            continue;
          }
          // Publish each successful period immediately; a failure in another period must not discard it.
          result = { ...result, periods: result.periods!.map((period) => period.key === key
            ? { ...period, items, itemCount: items.length } : period) };
        } catch (caught) {
          if (!stillCurrent()) break;
          hydrationFailures = { ...hydrationFailures, [key]: errorMessage(settings.locale, caught) };
        }
      }
      return result?.periods ?? [];
    } finally {
      if (stillCurrent()) {
        loadingItems = false;
        hydratingPeriod = '';
        hydrationRun = undefined;
      }
    }
  }

  function retryItemHydration() {
    if (loadingItems || running || exportingPDF || !result) return;
    const failed = Object.keys(hydrationFailures);
    hydrationFailures = {};
    void ensurePeriodItems(failed);
  }

  async function cancelAnalysis() {
    const id = operationId || result?.operationId;
    if (!id || cancelling) return;
    cancelling = true;
    try {
      await backend.cancelSalesAnalysis(id);
      if (result?.operationId === id && result.pending) {
        result = { ...result, pending: false };
        progress = undefined;
      } else if (!result || result.operationId === id) {
        await discardResult();
        progress = undefined;
        operationId = '';
        activeView = 'overview';
        queryOpen = false;
      }
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      cancelling = false;
    }
  }

  function openExportDialog() {
    if (!result || exportingPDF || !currentReady) return;
    const available = listSuccessfulReportStores(result);
    exportIncludeCombined = available.length > 1;
    exportStoreIds = new Set();
    exportGroupIds = new Set(selectedGroup ? [selectedGroup.id] : []);
    exportGroupDetail = false;
    exportPDFEnabled = true;
    exportAIEnabled = false;
    exportCanUseScreenFilters = filtersActive(selections, search);
    exportUseScreenFilters = exportCanUseScreenFilters;
    exportFilter = {
      ...defaultSalesReportFilter(),
      uncategorized: t('analysis.uncategorized'),
      ...(exportUseScreenFilters ? screenExportSnapshot() : {}),
    };
    exportDialog = true;
  }

  function toggleExportStore(storeId: string) {
    const next = new Set(exportStoreIds);
    if (next.has(storeId)) next.delete(storeId);
    else next.add(storeId);
    exportStoreIds = next;
  }

  function selectedExportStores() {
    if (!result) return [];
    const available = listSuccessfulReportStores(result);
    if (available.length === 1) return available;
    return available.filter((store) => exportStoreIds.has(store.businessId));
  }

  function toggleExportGroup(groupId: string) {
    const next = new Set(exportGroupIds);
    if (next.has(groupId)) next.delete(groupId);
    else next.add(groupId);
    exportGroupIds = next;
  }

  function selectedExportGroups(): ManCodeGroup[] {
    return manCodeGroups.filter((group) => exportGroupIds.has(group.id));
  }

  function exportTargetCount(): number {
    if (!result) return 0;
    const stores = listSuccessfulReportStores(result);
    if (stores.length <= 1) return stores.length;
    return (exportIncludeCombined ? 1 : 0) + selectedExportStores().length;
  }

  function exportOutputCount(): number {
    if (exportTargetTotal === 0 || (!exportPDFEnabled && !exportAIEnabled)) return 0;
    const groups = selectedExportGroups().length;
    const pdfMains = exportPDFEnabled ? exportTargetTotal : 0;
    const pdfDetails = exportPDFEnabled && exportGroupDetail ? exportTargetTotal * groups : 0;
    const aiFiles = exportAIEnabled ? exportTargetTotal : 0;
    return pdfMains + pdfDetails + aiFiles;
  }

  function closeExportDialog() {
    if (!exportingPDF) exportDialog = false;
  }

  function setExportMode(mode: SalesReportFilter['mode']) {
    const categories = mode === 'whitelist' && exportFilter.categories.length === 0
      ? exportCategoryOptions().map((option) => option.id)
      : exportFilter.categories;
    exportFilter = { ...exportFilter, mode, categories };
  }

  function screenExportSnapshot(): Pick<SalesReportFilter, 'facets' | 'search'> {
    const selected: SalesReportFacets = {};
    for (const { key } of facets) {
      if (selections[key].size > 0) selected[key] = [...selections[key]];
    }
    return { facets: selected, search: search.trim() };
  }

  function exportScreenFilterRows(): Array<{ label: string; values: string[] }> {
    const rows: Array<{ label: string; values: string[] }> = [];
    for (const facet of facets) {
      const values = exportFilter.facets?.[facet.key] ?? [];
      if (values.length > 0) rows.push({ label: t(facet.label), values });
    }
    if (exportFilter.search?.trim()) {
      rows.push({ label: t('analysis.search'), values: [exportFilter.search.trim()] });
    }
    return rows;
  }

  function setExportUseScreenFilters(next: boolean) {
    exportUseScreenFilters = next;
    exportFilter = {
      ...exportFilter,
      ...(next ? screenExportSnapshot() : { facets: {}, search: '' }),
    };
  }

  function activeExportFilter(): SalesReportFilter {
    if (exportUseScreenFilters) return exportFilter;
    return { ...exportFilter, facets: {}, search: '' };
  }

  function rankedToTopItems(items: Array<{ id?: string; code?: string; name?: string; brand?: string; amount?: number; quantity?: number }> | undefined): TopItem[] {
    return (items ?? []).map((item) => ({
      id: item.id || item.code || item.name || '',
      code: item.code ?? '',
      name: item.name || t('common.notAvailable'),
      brand: item.brand ?? '',
      amount: item.amount ?? 0,
      quantity: item.quantity ?? 0,
    }));
  }

  function exportCategoryOptions(): Array<{ id: string; name: string; code: string }> {
    const grouped = new Map<string, { id: string; name: string; code: string }>();
    if ((currentPeriod?.items?.length ?? 0) === 0) {
      return (currentPeriod?.categoryGroups?.[groupLevel] ?? []).map((group) => ({
        id: group.id, name: group.name, code: group.code ?? '',
      }));
    }
    for (const item of currentPeriod?.items ?? []) {
      const id = reportCategoryId(item, groupLevel);
      if (!id) continue;
      grouped.set(id, { id, name: categoryValue(item, groupLevel), code: categoryCode(item, groupLevel) });
    }
    return [...grouped.values()].sort((left, right) => left.name.localeCompare(right.name, settings.locale));
  }

  function toggleExportCategory(id: string) {
    const next = new Set(exportFilter.categories);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    exportFilter = { ...exportFilter, categories: [...next] };
  }

  async function exportPDF() {
    if (!result || exportingPDF || !currentReady) return;
    if (exportFilter.mode === 'whitelist' && exportFilter.categories.length === 0) return;
    if (exportFilesCount === 0) return;
    exportingPDF = true;
    pdfExportCurrent = 0;
    pdfExportTotal = 0;
    error = '';
    exportNotice = '';
    let nativeExportLease = '';
    try {
      if (!isWebRuntime()) nativeExportLease = await beginNativeExportLease();
      const directory = await backend.chooseSalesAnalysisPDFDirectory();
      if (!directory) return;
      const availableStores = listSuccessfulReportStores(result);
      if (availableStores.length === 0) throw new AppError('pdf_no_stores', 'No successful store is available for PDF export');
      const periodKeys = (result.periods ?? []).map((period) => period.key);
      if (periodKeys.length === 0) throw new AppError('pdf_loading', 'Sales analysis items are still loading');
      const reportStores = selectedExportStores();
      const reportGroups = selectedExportGroups();
      const writeStoreFiles = reportStores.length > 0;
      const writeCombined = availableStores.length > 1 && exportIncludeCombined;
      const targetIds = [
        ...(writeStoreFiles ? reportStores.map((store) => store.businessId) : []),
        ...(writeCombined ? [ALL_STORES_REPORT_ID] : []),
      ];
      pdfExportTotal = exportFilesCount;
      const reportFrom = result.from;
      const reportTo = result.to;
      const operationId = result.operationId;
      const slim: SalesAnalysisResult = {
        ...result,
        items: undefined,
        periods: (result.periods ?? []).map((period) => ({
          ...period,
          items: undefined,
          itemCount: period.itemCount || period.items?.length || 0,
        })),
      };
      result = slim;
      const fontGlyphs = `${await backend.getSalesAnalysisReportGlyphs(operationId)}${reportGroups.map((group) => group.name).join('')}`;
      const fontBase64 = exportPDFEnabled
        ? await prepareSalesAnalysisFontFromText(fontGlyphs, settings.locale)
        : '';
      const written: string[] = [];
      const loadMemo = (storeId: string | undefined, group?: ManCodeGroup) => backend.getSalesAnalysisReportMemo({
        operationId,
        storeId,
        groupId: group?.id,
        categoryLevel: groupLevel,
        excludeZeroGifts: exportFilter.excludeZeroGifts,
        excludeStamps: exportFilter.excludeStamps,
        mode: exportFilter.mode,
        categories: exportFilter.categories,
        facets: exportUseScreenFilters ? exportFilter.facets : undefined,
        search: exportUseScreenFilters ? exportFilter.search : undefined,
        uncategorized: t('analysis.uncategorized'),
      });
      let loaded = false;
      const writeFile = async (storeId: string) => {
        await yieldToUI();
        const memoStoreId = isAllStoresReport(storeId) ? undefined : storeId;
        const baseMemo = await loadMemo(memoStoreId);
        if (reportMemoHasRows(baseMemo)) loaded = true;
        const extraChapters: SalesReportChapter[] = [];
        const groupMemos: Array<{ group: ManCodeGroup; memo: SalesAnalysisReportMemo }> = [];
        for (const group of reportGroups) {
          const memo = await loadMemo(memoStoreId, group);
          if (reportMemoHasRows(memo)) loaded = true;
          groupMemos.push({ group, memo });
          extraChapters.push({
            scope: { groupId: group.id, groupName: group.name, itemCodes: group.codes },
            accumulator: salesReportAccumulatorFromMemo(memo),
          });
        }
        const storeLabel = isAllStoresReport(storeId)
          ? t('analysis.exportFilesCombined')
          : (availableStores.find((store) => store.businessId === storeId)?.label || storeId);
        if (exportPDFEnabled) {
          pdfExportCurrent += 1;
          await yieldToUI();
          let data: Uint8Array;
          try {
            data = await generateSalesAnalysisPDF(
              slim, storeId, groupLevel, settings.locale, activeExportFilter(), fontBase64,
              salesReportAccumulatorFromMemo(baseMemo),
              extraChapters,
              undefined,
              settings.rankingLimit,
            );
          } catch (caught) {
            if (caught instanceof AppError) throw caught;
            throw new AppError('pdf_failed', caught instanceof Error ? caught.message : String(caught));
          }
          written.push(await writeExportBytes(
            directory, salesAnalysisPDFFilename(storeId, reportFrom, reportTo), data, 'pdf_write',
          ));
          if (exportGroupDetail) {
            for (const { group, memo } of groupMemos) {
              pdfExportCurrent += 1;
              await yieldToUI();
              let detail: Uint8Array;
              try {
                detail = await generateSalesAnalysisPDF(
                  slim, storeId, groupLevel, settings.locale, activeExportFilter(), fontBase64,
                  salesReportAccumulatorFromMemo(memo),
                  [],
                  { groupId: group.id, groupName: group.name, itemCodes: group.codes },
                  settings.rankingLimit,
                );
              } catch (caught) {
                if (caught instanceof AppError) throw caught;
                throw new AppError('pdf_failed', caught instanceof Error ? caught.message : String(caught));
              }
              written.push(await writeExportBytes(
                directory,
                salesAnalysisPDFFilename(storeId, reportFrom, reportTo, group.name),
                detail,
                'pdf_write',
              ));
            }
          }
        }
        if (exportAIEnabled) {
          pdfExportCurrent += 1;
          await yieldToUI();
          const markdown = buildSalesAnalysisAIMarkdown({
            locale: settings.locale,
            storeId,
            storeLabel,
            from: reportFrom,
            to: reportTo,
            categoryLevel: groupLevel,
            filter: activeExportFilter(),
            rankingLimit: settings.rankingLimit,
            base: baseMemo,
            periodMeta: (slim.periods ?? []).map((period) => ({
              key: period.key,
              label: period.label,
              from: period.from,
              to: period.to,
            })),
            groups: groupMemos.map(({ group, memo }) => ({
              groupId: group.id,
              groupName: group.name,
              itemCodeCount: group.codes.length,
              memo,
            })),
          });
          written.push(await writeExportBytes(
            directory,
            salesAnalysisAIFilename(storeId, reportFrom, reportTo),
            new TextEncoder().encode(markdown),
            'export_write',
            'text',
          ));
        }
      };
      for (const storeId of targetIds) {
        await writeFile(storeId);
      }
      if (!loaded) throw new AppError('pdf_loading', 'Sales analysis items are still loading');
      exportDirectory = directory;
      exportNotice = exportPDFEnabled && writeCombined && writeStoreFiles && !exportGroupDetail && !exportAIEnabled
        ? t('analysis.exportedPDFWithCombined', { stores: reportStores.length })
        : t('analysis.exportedPDF', { count: written.length });
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      if (nativeExportLease) {
        try { await endNativeExportLease(nativeExportLease); }
        catch (caught) { error = errorMessage(settings.locale, caught); }
      }
      exportingPDF = false;
      exportDialog = false;
      if (result) void ensurePeriodItems(neededPeriodKeys);
    }
  }

  async function writeExportBytes(
    directory: string,
    filename: string,
    data: Uint8Array,
    errorCode: 'pdf_write' | 'export_write',
    kind: 'pdf' | 'text' = 'pdf',
  ): Promise<string> {
    const dataBase64 = bytesToBase64(data);
    try {
      return kind === 'text'
        ? await backend.writeSalesAnalysisTextExport({ directory, filename, dataBase64 })
        : await backend.writeSalesAnalysisPDF({ directory, filename, dataBase64 });
    } catch (caught) {
      if (caught instanceof AppError && caught.code !== 'backend_error') throw caught;
      throw new AppError(errorCode, caught instanceof Error ? caught.message : String(caught));
    }
  }

  function reportMemoHasRows(memo: SalesAnalysisReportMemo): boolean {
    return (memo.periods ?? []).some((period) =>
      period.totals !== undefined
      || (period.topAmount?.length ?? 0) > 0
      || (period.amountGroups?.length ?? 0) > 0
      || (period.quantityGroups?.length ?? 0) > 0,
    );
  }

  function yieldToUI(): Promise<void> {
    return new Promise((resolve) => window.setTimeout(resolve, 16));
  }

  function buildPeriodRequests(): SalesAnalysisPeriodRequest[] {
    if (periodMode === 'month') {
      if (!/^\d{4}-\d{2}$/.test(month)) return [];
      const currentFrom = `${month}-01`;
      const currentMonthSelected = month === localISOMonth();
      const currentTo = currentMonthSelected ? localISODate() : endOfMonth(month);
      const cutoffDay = Number(currentTo.slice(-2));
      const previousMonth = shiftMonth(month, -1);
      const previous2Month = shiftMonth(month, -2);
      const yearAgoNextMonth = shiftMonth(month, -11);
      return [
        periodRequest('current', t('analysis.currentPeriod'), currentFrom, currentTo),
        periodRequest('previous', t('analysis.previousPeriod'), `${previousMonth}-01`, currentMonthSelected ? monthToDayEnd(previousMonth, cutoffDay) : endOfMonth(previousMonth)),
        periodRequest('previous2', t('analysis.previous2Period'), `${previous2Month}-01`, currentMonthSelected ? monthToDayEnd(previous2Month, cutoffDay) : endOfMonth(previous2Month)),
        periodRequest('yearAgo', t('analysis.yearAgoPeriod'), shiftYear(currentFrom, -1), shiftYear(currentTo, -1)),
        periodRequest('yearAgoNext', t('analysis.yearAgoNextPeriod'), `${yearAgoNextMonth}-01`, endOfMonth(yearAgoNextMonth), false),
      ];
    }
    if (!from || !to || from > to) return [];
    const yearAgoNextMonth = shiftMonth(from.slice(0, 7), -11);
    if (weekCompare) {
      const aligned = alignRangeComparisonPeriods(from, to);
      if (!aligned) return [];
      return [
        periodRequest('current', t('analysis.currentPeriod'), from, to),
        periodRequest('previous', t('analysis.previousPeriod'), aligned.previous.from, aligned.previous.to),
        periodRequest('previous2', t('analysis.previous2Period'), aligned.previous2.from, aligned.previous2.to),
        periodRequest('yearAgo', t('analysis.yearAgoPeriod'), aligned.yearAgo.from, aligned.yearAgo.to),
        periodRequest('yearAgoNext', t('analysis.yearAgoNextPeriod'), `${yearAgoNextMonth}-01`, endOfMonth(yearAgoNextMonth), false),
      ];
    }
    const days = daysBetween(from, to) + 1;
    const previousTo = addDays(from, -1);
    const previousFrom = addDays(previousTo, -(days - 1));
    const previous2To = addDays(previousFrom, -1);
    const previous2From = addDays(previous2To, -(days - 1));
    return [
      periodRequest('current', t('analysis.currentPeriod'), from, to),
      periodRequest('previous', t('analysis.previousPeriod'), previousFrom, previousTo),
      periodRequest('previous2', t('analysis.previous2Period'), previous2From, previous2To),
      periodRequest('yearAgo', t('analysis.yearAgoPeriod'), shiftYear(from, -1), shiftYear(to, -1)),
      periodRequest('yearAgoNext', t('analysis.yearAgoNextPeriod'), `${yearAgoNextMonth}-01`, endOfMonth(yearAgoNextMonth), false),
    ];
  }

  function periodRequest(key: string, label: string, periodFrom: string, periodTo: string, includeTrend = true): SalesAnalysisPeriodRequest {
    return { key, label, from: periodFrom, to: periodTo, includeTrend };
  }

  function emptySelections(): FacetSelections {
    return {
      category1: new Set<string>(), category2: new Set<string>(), category3: new Set<string>(),
      category4: new Set<string>(), category5: new Set<string>(),
    };
  }

  function emptyTotals(): FilteredTotals {
    return {
      saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0,
      netQuantity: 0, netSalesAmount: 0, skuCount: 0,
    };
  }

  function resetFilters() {
    selections = emptySelections();
    search = '';
    selectedGroupId = '';
    groupLevel = 'category2';
    salesRankingKey = 'current';
    quantityRankingKey = 'current';
    page = 1;
  }

  function normalizePeriods(value: SalesAnalysisResult | undefined): SalesAnalysisPeriodResult[] {
    if (!value) return [];
    if (value.periods?.length) return value.periods;
    return [{
      key: 'current', label: t('analysis.currentPeriod'), from: value.from, to: value.to,
      complete: value.complete, successfulStores: value.successfulStores, totals: value.totals,
      stores: value.stores, items: value.items ?? [], issues: value.issues,
    }];
  }

  function periodByKey(periods: SalesAnalysisPeriodResult[], key: string): SalesAnalysisPeriodResult | undefined {
    return periods.find((period) => period.key === key);
  }

  function itemEmptyLabel(period: SalesAnalysisPeriodResult | undefined, failures: Record<string, string>): string {
    if (!periodNeedsItemHydration(period)) return t('analysis.noResults');
    return t(failures[period?.key ?? ''] ? 'analysis.itemsUnavailable' : 'common.loading');
  }

  function focusGroupLabel(group: FocusGroup): string {
    if (group.name) return group.name;
    if (group.id === 'health') return t('analysis.focusHealth');
    if (group.id === 'skin') return t('analysis.focusSkin');
    if (group.id === 'pc') return t('analysis.focusPC');
    return group.id;
  }

  function categoryValue(item: SalesAnalysisItem, key: CategoryKey): string {
    return categoryNameOf(item, key) || t('analysis.uncategorized');
  }

  function categoryCode(item: SalesAnalysisItem, key: CategoryKey): string {
    return categoryCodeOf(item, key);
  }

  function categoryLabel(item: SalesAnalysisItem, key: CategoryKey): string {
    return categoryLabelOf(item, key, t('analysis.uncategorized'));
  }

  function keepHydratedItems(previous: SalesAnalysisResult, next: SalesAnalysisResult): SalesAnalysisResult {
    const prevByKey = new Map((previous.periods ?? []).map((period) => [period.key, period]));
    return {
      ...next,
      periods: (next.periods ?? []).map((period) => {
        const prev = prevByKey.get(period.key);
        if (!prev?.items || period.from !== prev.from || period.to !== prev.to
          || (period.items?.length ?? -1) >= prev.items.length
          || (period.itemCount ?? 0) > prev.items.length) return period;
        return { ...period, items: prev.items, itemCount: prev.items.length };
      }),
    };
  }

  function matchesSelections(item: SalesAnalysisItem, current: FacetSelections, skipped?: CategoryKey): boolean {
    return facets.every(({ key }) =>
      key === skipped || itemMatchesCategorySelection(item, key, current[key], t('analysis.uncategorized')));
  }

  function matchesFilters(item: SalesAnalysisItem, current: FacetSelections, searchTerm: string): boolean {
    if (!matchesSelections(item, current)) return false;
    const term = searchTerm.trim().toLocaleLowerCase();
    if (!term) return true;
    return [
      item.storeId, item.storeLabel, item.articleCode, item.articleName, item.brandName ?? '',
      item.category1, item.category1Code ?? '', item.category2, item.category2Code ?? '',
      item.category3, item.category3Code ?? '', item.category4, item.category4Code ?? '',
      item.category5, item.category5Code ?? '',
    ].some((value) => (value ?? '').toLocaleLowerCase().includes(term));
  }

  function matchesGroup(item: SalesAnalysisItem, codes: Set<string>, active: boolean): boolean {
    return !active || codes.has(item.articleCode.trim());
  }

  function changePromoterGroup(event: Event) {
    selectedGroupId = (event.currentTarget as HTMLSelectElement).value;
    page = 1;
    if (activeView === 'weekly' && selectedGroupId) activeView = 'overview';
  }

  function focusGroupsForScope(
    next: SalesAnalysisPeriodResult | undefined,
    current: SalesAnalysisPeriodResult | undefined,
    facets: FacetSelections,
    searchTerm: string,
    group: ManCodeGroup | undefined,
    catalog: ManCodeGroup[],
    codes: Set<string>,
    scoped: boolean,
  ): FocusGroup[] {
    if (!next) return [];
    const nextItems = (next.items ?? []).filter((item) =>
      includeInSalesReport(item) && matchesFilters(item, facets, searchTerm) && matchesGroup(item, codes, scoped));
    const currentItems = (current?.items ?? []).filter((item) =>
      includeInSalesReport(item) && matchesFilters(item, facets, searchTerm) && matchesGroup(item, codes, scoped));
    const groups = buildFocusGroups(nextItems, currentItems, 10, group ? [group] : catalog);
    if (!group || groups.some((candidate) => candidate.id === group.id)) return groups;
    return [{ id: group.id, prefix: '', name: group.name, sales: [], quantity: [] }];
  }

  function filtersActive(current: FacetSelections, searchTerm: string): boolean {
    return Boolean(searchTerm.trim()) || facets.some(({ key }) => current[key].size > 0);
  }

  function periodKeysForAnalysisView(view: ReportView, rankingKeys: string[], productScopeActive: boolean): string[] {
    if (productScopeActive && (view === 'overview' || view === 'stores')) {
      return ['current', 'previous', 'yearAgo'];
    }
    return periodKeysForView(view, rankingKeys);
  }

  function listFacetOptions(
    period: SalesAnalysisPeriodResult | undefined,
    current: FacetSelections,
    groupCodes: Set<string>,
    scoped: boolean,
    locale: string,
    key: CategoryKey,
  ): string[] {
    if (!period) return [];
    if ((period.items?.length ?? 0) === 0 && !scoped && !filtersActive(current, '')) {
      return [...(period.facetOptions?.[key] ?? [])];
    }
    return [...new Set((period.items ?? [])
      .filter((item) => matchesGroup(item, groupCodes, scoped) && matchesSelections(item, current, key))
      .map((item) => categoryLabel(item, key)))]
      .sort((left, right) => left.localeCompare(right, locale, { numeric: true }));
  }

  function listFacetOptionMap(
    period: SalesAnalysisPeriodResult | undefined,
    current: FacetSelections,
    groupCodes: Set<string>,
    scoped: boolean,
    locale: string,
  ): Record<CategoryKey, string[]> {
    return {
      category1: listFacetOptions(period, current, groupCodes, scoped, locale, 'category1'),
      category2: listFacetOptions(period, current, groupCodes, scoped, locale, 'category2'),
      category3: listFacetOptions(period, current, groupCodes, scoped, locale, 'category3'),
      category4: listFacetOptions(period, current, groupCodes, scoped, locale, 'category4'),
      category5: listFacetOptions(period, current, groupCodes, scoped, locale, 'category5'),
    };
  }

  function toggleFacet(key: CategoryKey, value: string) {
    const next = new Set(selections[key]);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    selections = { ...selections, [key]: next };
    page = 1;
  }

  function visibleFacetOptions(key: CategoryKey, query: string, options: string[]): string[] {
    const term = openFacet === key ? query.trim().toLocaleLowerCase() : '';
    return options.filter((option) => option.toLocaleLowerCase().includes(term));
  }

  function clearFacet(key: CategoryKey) {
    const visible = new Set(visibleFacetOptions(key, facetSearch, facetOptionMap[key]));
    selections = { ...selections, [key]: facetSearch.trim()
      ? new Set([...selections[key]].filter((value) => !visible.has(value))) : new Set<string>() };
    page = 1;
  }

  function selectFacetAll(key: CategoryKey) {
    selections = { ...selections, [key]: new Set([...selections[key], ...visibleFacetOptions(key, facetSearch, facetOptionMap[key])]) };
    page = 1;
  }

  function summarize(items: SalesAnalysisItem[]): FilteredTotals {
    const totals = emptyTotals();
    const sku = new Set<string>();
    for (const item of items) {
      totals.saleQuantity += item.saleQuantity;
      totals.saleAmount += item.saleAmount;
      totals.returnQuantity += item.returnQuantity;
      totals.returnAmount += item.returnAmount;
      totals.netQuantity += item.netQuantity;
      totals.netSalesAmount += item.netSalesAmount;
      sku.add(item.articleCode || item.articleName);
    }
    totals.skuCount = sku.size;
    return totals;
  }

  function totalsForPeriod(
    period: SalesAnalysisPeriodResult | undefined,
    current: FacetSelections,
    searchTerm: string,
    groupCodes: Set<string>,
    scoped: boolean,
  ): FilteredTotals {
    if (!period) return emptyTotals();
    const matching = (period?.items ?? []).filter((item) =>
      matchesFilters(item, current, searchTerm) && matchesGroup(item, groupCodes, scoped));
    if (scoped || filtersActive(current, searchTerm)) return summarize(matching);
    const sku = matching.length > 0
      ? new Set(matching.map((item) => item.articleCode || item.articleName))
      : new Set<string>();
    const totals: FilteredTotals = {
      ...period.totals,
      skuCount: matching.length > 0 ? sku.size : (period.itemCount ?? 0),
    };
    if (totals.transactionCount && totals.transactionCount > 0 && totals.trendNetSalesAmount !== undefined) {
      totals.basketValue = totals.trendNetSalesAmount / totals.transactionCount;
    }
    return totals;
  }

  function buildPerformanceRows(
    current: FilteredTotals,
    previous: FilteredTotals | undefined,
    yearAgo: FilteredTotals | undefined,
  ): PerformanceRow[] {
    return [
      { label: t('analysis.grossSales'), current: current.saleAmount, previous: previous?.saleAmount, yearAgo: yearAgo?.saleAmount, format: 'money' },
      { label: t('analysis.returns'), current: current.returnAmount, previous: previous?.returnAmount, yearAgo: yearAgo?.returnAmount, format: 'money' },
      { label: t('analysis.netSales'), current: current.netSalesAmount, previous: previous?.netSalesAmount, yearAgo: yearAgo?.netSalesAmount, format: 'money' },
      { label: t('analysis.netQuantity'), current: current.netQuantity, previous: previous?.netQuantity, yearAgo: yearAgo?.netQuantity, format: 'number' },
      { label: t('analysis.transactions'), current: current.transactionCount, previous: previous?.transactionCount, yearAgo: yearAgo?.transactionCount, format: 'number' },
      { label: t('analysis.basket'), current: current.basketValue, previous: previous?.basketValue, yearAgo: yearAgo?.basketValue, format: 'money' },
    ];
  }

  function buildCategoryComparison(
    periods: SalesAnalysisPeriodResult[], key: CategoryKey, current: FacetSelections, searchTerm: string,
    groupCodes: Set<string>, scoped: boolean,
  ): CategoryComparisonRow[] {
    const grouped = new Map<string, CategoryComparisonRow>();
    const useSummary = !scoped && !filtersActive(current, searchTerm)
      && periods.every((period) => !['current', 'previous', 'previous2', 'yearAgo'].includes(period.key) || (period.items?.length ?? 0) === 0);
    for (const period of periods) {
      if (!['current', 'previous', 'previous2', 'yearAgo'].includes(period.key)) continue;
      if (useSummary) {
        for (const group of period.categoryGroups?.[key] ?? []) {
          const id = group.id || group.code || group.name;
          const row: CategoryComparisonRow = grouped.get(id) ?? { id, name: group.name, code: group.code ?? '', current: 0, previous: 0, previous2: 0, yearAgo: 0 };
          row[period.key as 'current' | 'previous' | 'previous2' | 'yearAgo'] += group.amount;
          grouped.set(id, row);
        }
        continue;
      }
      for (const item of period.items ?? []) {
        if (!includeInSalesReport(item) || !matchesFilters(item, current, searchTerm) || !matchesGroup(item, groupCodes, scoped)) continue;
        const name = categoryValue(item, key);
        const code = categoryCode(item, key);
        const id = code || name;
        const row: CategoryComparisonRow = grouped.get(id) ?? { id, name, code, current: 0, previous: 0, previous2: 0, yearAgo: 0 };
        row[period.key as 'current' | 'previous' | 'previous2' | 'yearAgo'] += item.netSalesAmount;
        grouped.set(id, row);
      }
    }
    return [...grouped.values()].sort((left, right) => right.current - left.current || left.name.localeCompare(right.name, settings.locale));
  }

  function buildTopItems(items: SalesAnalysisItem[], sortBy: 'amount' | 'quantity'): TopItem[] {
    const grouped = new Map<string, TopItem>();
    for (const item of items) {
      if (!includeInSalesReport(item)) continue;
      const id = item.articleCode || item.articleName;
      const current: TopItem = grouped.get(id) ?? {
        id, code: item.articleCode, name: item.articleName || t('common.notAvailable'), brand: item.brandName ?? '', amount: 0, quantity: 0,
      };
      current.amount += item.netSalesAmount;
      current.quantity += item.netQuantity;
      grouped.set(id, current);
    }
    return [...grouped.values()].sort((left, right) => {
      const difference = sortBy === 'amount' ? right.amount - left.amount : right.quantity - left.quantity;
      return difference || left.id.localeCompare(right.id);
    });
  }

  function buildCategoryRankings(
    period: SalesAnalysisPeriodResult | undefined,
    key: CategoryKey,
    sortBy: 'amount' | 'quantity',
    current: FacetSelections,
    searchTerm: string,
    groupCodes: Set<string>,
    scoped: boolean,
    limit: number,
  ): CategoryRankingGroup[] {
    if (!period) return [];
    const grouped = new Map<string, { code: string; name: string; items: SalesAnalysisItem[]; amount: number; quantity: number }>();
    for (const item of period.items ?? []) {
      if (!includeInSalesReport(item) || !matchesFilters(item, current, searchTerm) || !matchesGroup(item, groupCodes, scoped)) continue;
      const code = categoryCode(item, key);
      const name = categoryValue(item, key);
      const id = code || name;
      const group = grouped.get(id) ?? { code, name, items: [], amount: 0, quantity: 0 };
      group.items.push(item);
      group.amount += item.netSalesAmount;
      group.quantity += item.netQuantity;
      grouped.set(id, group);
    }
    return [...grouped.entries()]
      .map(([id, group]) => ({
        id, code: group.code, name: group.name, amount: group.amount, quantity: group.quantity,
        items: buildTopItems(group.items, sortBy).slice(0, limit),
      }))
      .sort((left, right) => (sortBy === 'amount' ? right.amount - left.amount : right.quantity - left.quantity) || left.id.localeCompare(right.id, settings.locale))
      .slice(0, 6);
  }

  function hasWeekAlignedComparison(periods: SalesAnalysisPeriodResult[]): boolean {
    const current = periodByKey(periods, 'current');
    const previous = periodByKey(periods, 'previous');
    if (!current || !previous) return false;
    const stride = daysBetween(previous.from, current.from);
    return stride > 0
      && stride % 7 === 0
      && daysBetween(current.from, current.to) === daysBetween(previous.from, previous.to)
      && addDays(previous.to, stride) === current.to;
  }

  function weeklyMetricCells(row: SalesAnalysisWeek['totals']): Array<{ text: string; className: string }> {
    const salesChange = delta(row.salesTw, row.salesLw);
    const weekdayChange = delta(row.weekdaySalesTw, row.weekdaySalesLw);
    const weekendChange = delta(row.weekendSalesTw, row.weekendSalesLw);
    const customerChange = delta(row.customersTw, row.customersLw);
    return [
      { text: formatMoney(row.salesTw), className: 'numeric emphasis' },
      { text: formatMoney(row.salesLw), className: 'numeric' },
      { text: formatMoney(row.salesTw - row.salesLw), className: `numeric ${deltaClass(salesChange)}` },
      { text: formatPercent(salesChange), className: `numeric ${deltaClass(salesChange)}` },
      { text: formatPercent(weekdayChange), className: `numeric ${deltaClass(weekdayChange)}` },
      { text: formatPercent(weekendChange), className: `numeric ${deltaClass(weekendChange)}` },
      { text: formatPercent(customerChange), className: `numeric ${deltaClass(customerChange)}` },
    ];
  }

  function buildStoreRows(
    periods: SalesAnalysisPeriodResult[],
    current: FacetSelections,
    searchTerm: string,
    groupCodes: Set<string>,
    groupScoped: boolean,
  ): StoreComparisonRow[] {
    const grouped = new Map<string, StoreComparisonRow>();
    const scoped = groupScoped || filtersActive(current, searchTerm);
    for (const period of periods) {
      if (!['current', 'previous', 'yearAgo'].includes(period.key)) continue;
      if (!scoped) {
        for (const store of period.stores ?? []) {
          const row: StoreComparisonRow = grouped.get(store.businessId) ?? { id: store.businessId, label: store.label };
          row[period.key as 'current' | 'previous' | 'yearAgo'] = store.totals;
          grouped.set(store.businessId, row);
        }
        continue;
      }
      const labels = new Map((period.stores ?? []).map((store) => [store.businessId, store.label]));
      for (const item of period.items ?? []) {
        if (item.storeId && !labels.has(item.storeId)) labels.set(item.storeId, item.storeLabel || item.storeId);
      }
      for (const [storeId, label] of labels) {
        const items = (period.items ?? []).filter((item) => item.storeId === storeId
          && matchesFilters(item, current, searchTerm) && matchesGroup(item, groupCodes, groupScoped));
        const row: StoreComparisonRow = grouped.get(storeId) ?? { id: storeId, label };
        row[period.key as 'current' | 'previous' | 'yearAgo'] = summarize(items);
        grouped.set(storeId, row);
      }
    }
    return [...grouped.values()].sort((left, right) => (right.current?.netSalesAmount ?? 0) - (left.current?.netSalesAmount ?? 0) || left.id.localeCompare(right.id));
  }

  function filterStores(list: SalesAnalysisStore[], query: string): SalesAnalysisStore[] {
    const term = query.trim().toLocaleLowerCase();
    if (!term) return list;
    return list.filter((store) =>
      store.businessId.toLocaleLowerCase().includes(term) || store.label.toLocaleLowerCase().includes(term),
    );
  }

  function toggleFacetMenu(key: CategoryKey) {
    openFacet = openFacet === key ? '' : key;
    facetSearch = '';
  }

  async function openExportFolder() {
    if (!exportDirectory || openingFolder) return;
    openingFolder = true;
    error = '';
    try {
      await backend.openSavedFolder(exportDirectory);
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      openingFolder = false;
    }
  }

  function dismissExportNotice() {
    exportNotice = '';
    exportDirectory = '';
  }

  function changeSearch(event: Event) {
    search = (event.currentTarget as HTMLInputElement).value;
    page = 1;
  }

  function delta(current: number | undefined, base: number | undefined): number | undefined {
    if (current === undefined || base === undefined || base === 0) return undefined;
    return (current - base) / Math.abs(base);
  }

  function formatValue(value: number | undefined, format: ValueFormat): string {
    if (value === undefined) return '—';
    return format === 'money' ? formatMoney(value) : formatNumber(value);
  }

  function formatMoney(value: number): string {
    return new Intl.NumberFormat(settings.locale, { style: 'currency', currency: 'HKD', minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value);
  }

  function formatPeriodMoney(value: number, periodKey: string): string {
    return periodByKey(reportPeriods, periodKey) ? formatMoney(value) : '—';
  }

  function formatNumber(value: number): string {
    return new Intl.NumberFormat(settings.locale, { maximumFractionDigits: 0 }).format(Math.round(value));
  }

  function formatPercent(value: number | undefined): string {
    if (value === undefined || !Number.isFinite(value)) return '—';
    return new Intl.NumberFormat(settings.locale, { style: 'percent', signDisplay: 'always', maximumFractionDigits: 1 }).format(value);
  }

  function deltaClass(value: number | undefined): string {
    if (value === undefined || value === 0) return 'neutral';
    return value > 0 ? 'positive' : 'negative';
  }

  function localISODate(): string {
    const now = new Date();
    return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
  }

  function localISOMonth(): string {
    return localISODate().slice(0, 7);
  }

  function parseISODate(value: string): Date {
    const [year, monthValue, day] = value.split('-').map(Number);
    return new Date(Date.UTC(year, monthValue - 1, day));
  }

  function formatISODate(value: Date): string {
    return `${value.getUTCFullYear()}-${String(value.getUTCMonth() + 1).padStart(2, '0')}-${String(value.getUTCDate()).padStart(2, '0')}`;
  }

  function addDays(value: string, days: number): string {
    const date = parseISODate(value);
    date.setUTCDate(date.getUTCDate() + days);
    return formatISODate(date);
  }

  function daysBetween(start: string, end: string): number {
    return Math.round((parseISODate(end).getTime() - parseISODate(start).getTime()) / 86_400_000);
  }

  function shiftYear(value: string, years: number): string {
    const date = parseISODate(value);
    const monthValue = date.getUTCMonth();
    date.setUTCFullYear(date.getUTCFullYear() + years);
    if (date.getUTCMonth() !== monthValue) date.setUTCDate(0);
    return formatISODate(date);
  }

  function shiftMonth(value: string, months: number): string {
    const [year, monthValue] = value.split('-').map(Number);
    const date = new Date(Date.UTC(year, monthValue - 1 + months, 1));
    return `${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, '0')}`;
  }

  function endOfMonth(value: string): string {
    const [year, monthValue] = value.split('-').map(Number);
    return formatISODate(new Date(Date.UTC(year, monthValue, 0)));
  }

  function monthToDayEnd(value: string, cutoffDay: number): string {
    const lastDay = Number(endOfMonth(value).slice(-2));
    return `${value}-${String(Math.min(cutoffDay, lastDay)).padStart(2, '0')}`;
  }
</script>

{#snippet productName(code: string, name: string, periodKey = 'current')}
  {#if code}<button class="product-name-link" type="button" aria-label={t('data.productOpen', { name })} title={t('data.productOpen', { name })} onclick={() => openProduct(code, name, periodKey)}>{name}</button>{:else}<strong>{name}</strong>{/if}
{/snippet}

{#snippet rankingControl(disabled = false)}
  <div class="rank-seg" role="group" aria-label={t('analysis.rankingLimit')}>
    <span>{t('analysis.rankingLimit')}</span>
    <div class="rank-seg-track">
      {#each RANKING_LIMIT_CHOICES as limit}
        <button type="button" class:active={settings.rankingLimit === limit} aria-pressed={settings.rankingLimit === limit} disabled={disabled} onclick={() => setRankingLimit(limit)}>{limit}</button>
      {/each}
    </div>
    <label class="rank-custom" class:active={!isPresetRankingLimit(settings.rankingLimit)} title={t('analysis.rankingHint')}>
      <span>{t('analysis.rankingLimitCustom')}</span>
      <input
        type="number"
        min={RANKING_LIMIT_MIN}
        max={RANKING_LIMIT_MAX}
        inputmode="numeric"
        disabled={disabled}
        class:active={!isPresetRankingLimit(settings.rankingLimit)}
        aria-label={t('analysis.rankingLimitCustom')}
        title={t('analysis.rankingLimitCustom')}
        value={rankingInput}
        onfocus={() => { rankingFocused = true; }}
        oninput={(event) => { rankingInput = (event.currentTarget as HTMLInputElement).value; }}
        onblur={commitRankingInput}
        onkeydown={(event) => {
          if (event.key === 'Escape') rankingInput = String(settings.rankingLimit);
          if (event.key === 'Enter' || event.key === 'Escape') { event.preventDefault(); (event.currentTarget as HTMLInputElement).blur(); }
        }}
      />
    </label>
  </div>
{/snippet}

<section class="page analysis-page" class:has-results={Boolean(result && currentPeriod)} aria-labelledby="analysis-title">
  <div class="page-heading split-heading">
    <div class="heading-copy">
      <h1 id="analysis-title">{t('analysis.title')}</h1>
      {#if result && currentPeriod}
        <div class="heading-meta">
          <p class="report-context">{t('analysis.querySummary', {
            account: reportAccount || t('analysis.savedReport'),
            period: `${currentPeriod.from} — ${currentPeriod.to}`,
            count: result.selectedStores,
          })}</p>
          <div class="heading-flags">
            <span class="report-status" class:ready={result.complete && !loadingItems && !itemFailureRows.length}>
              <span class="status-dot" aria-hidden="true"></span>{itemFailureRows.length ? t('analysis.itemsNotReady') : loadingItems || result.pending ? t('common.loading') : result.complete ? t('analysis.reportReady') : t('analysis.partialResult', { count: result.issues?.length ?? 0 })}
            </span>
            <details class="period-disclosure">
              <summary>{t('analysis.periods')}<span class="material-symbols-rounded" aria-hidden="true">expand_more</span></summary>
              <dl class="period-summary" aria-label={t('analysis.periods')}>
                {#each reportPeriods as period}<div class:current={period.key === 'current'}><dt>{period.label}</dt><dd>{period.from} — {period.to}</dd></div>{/each}
              </dl>
            </details>
          </div>
        </div>
      {:else}
        <p class="report-context">{t('analysis.queryHint')}</p>
      {/if}
    </div>
    <div class="analysis-heading-actions">
      <button type="button" class="preset-trigger" onclick={openPresets} disabled={loadingProfiles || loadingStores || running || exportingPDF} aria-haspopup="dialog"><span class="material-symbols-rounded" aria-hidden="true">bookmarks</span>{t('presets.title')}</button>
      {#if result}
        <md-filled-button type="button" onclick={openExportDialog} disabled={(exportingPDF || !currentReady) ? true : undefined}>
          <span class="material-symbols-rounded" slot="icon" aria-hidden="true">picture_as_pdf</span>{exportingPDF ? t('analysis.exportingPDFProgress', { current: pdfExportCurrent, total: pdfExportTotal }) : t('analysis.exportPDF')}
        </md-filled-button>
        <md-outlined-button type="button" onclick={() => changeQuery()} disabled={(exportingPDF || running) ? true : undefined} aria-expanded={queryOpen} aria-controls="analysis-query-form">
          <span class="material-symbols-rounded" slot="icon" aria-hidden="true">{queryOpen ? 'close' : 'tune'}</span>{queryOpen ? t('analysis.closeQuery') : t('analysis.adjustQuery')}
        </md-outlined-button>
      {/if}
    </div>
  </div>

  {#if shortcutGroups.pinned.length || shortcutGroups.recent.length}
    <nav class="preset-shortcuts" aria-label={t('presets.shortcuts')}>
      {#each [{ label: 'presets.pinned', entries: shortcutGroups.pinned }, { label: 'presets.recent', entries: shortcutGroups.recent }] as group}
        {#if group.entries.length}<div class="shortcut-group"><span>{t(group.label)}</span>{#each group.entries as preset (preset.id)}<button type="button" aria-label={t('presets.quickApply', { name: preset.name })} title={`${preset.query.profileName} · ${preset.query.storeIds.join(', ')}`} disabled={shortcutBusy || loadingProfiles || loadingStores || running || exportingPDF || exportingData} onclick={() => void quickApplyPreset(preset.id)}>{preset.name}</button>{/each}</div>{/if}
      {/each}
    </nav>
  {/if}
  {#if shortcutNotice}<div class="notice error-notice" role="alert">{shortcutNotice}</div>{/if}

  {#if error}
    <div class="notice error-notice" role="alert"><span class="material-symbols-rounded" aria-hidden="true">error</span><span>{error}</span></div>
  {/if}
  {#if exportNotice}
    <div class="notice success-notice export-notice" role="status">
      <span class="material-symbols-rounded" aria-hidden="true">check_circle</span>
      <div class="export-notice-copy">
        <span>{exportNotice}</span>
        {#if exportDirectory}<code title={exportDirectory}>{exportDirectory}</code>{/if}
      </div>
      {#if exportDirectory && !isWebRuntime()}
        <md-outlined-button type="button" onclick={() => void openExportFolder()} disabled={openingFolder}>
          {openingFolder ? t('analysis.openingFolder') : t('analysis.openFolder')}
        </md-outlined-button>
      {/if}
      <md-icon-button type="button" aria-label={t('common.close')} onclick={dismissExportNotice}>
        <span class="material-symbols-rounded" aria-hidden="true">close</span>
      </md-icon-button>
    </div>
  {/if}
  {#if loadingItems && !result}
    <div class="notice" role="status"><span class="material-symbols-rounded" aria-hidden="true">progress_activity</span><span>{t('analysis.loadingItems')}</span></div>
  {/if}

  {#if loadingProfiles}
    <div class="analysis-loading surface-card" role="status"><md-circular-progress indeterminate></md-circular-progress><strong>{t('common.loading')}</strong></div>
  {:else if profiles.length === 0 && !result}
    <div class="empty-state surface-card">
      <span class="material-symbols-rounded" aria-hidden="true">manage_accounts</span>
      <h2>{t('analysis.noAccounts')}</h2>
      {#if isWebRuntime()}
        <p>{t('web.previewHint')}</p>
        <md-filled-button type="button" onclick={() => void loadWebPreview()} disabled={running}>
          {running ? t('web.loadingPreview') : t('web.loadPreview')}
        </md-filled-button>
      {/if}
      <md-filled-button type="button" onclick={onGoToAccounts}>{t('excel.manageAccounts')}</md-filled-button>
    </div>
  {:else if !result || queryOpen}
    <form id="analysis-query-form" class="analysis-query surface-card" class:compact={Boolean(result)} onsubmit={(event) => { event.preventDefault(); void runAnalysis(); }}>
      <div class="query-section-heading">
        <span class="material-symbols-rounded" aria-hidden="true">tune</span>
        <div><h2>{t('analysis.adjustQuery')}</h2>{#if result}<p>{t('analysis.editQueryHint')}</p>{/if}</div>
      </div>
      <div class="analysis-query-grid">
        <div class="field-group">
          <label for="analysis-profile">{t('analysis.account')}</label>
          <select id="analysis-profile" bind:value={profileId} onchange={() => void loadStores({ keepResult: true })} disabled={loadingStores || running}>
            {#each profiles as profile}<option value={profile.id}>{profile.displayName}</option>{/each}
          </select>
        </div>
        <div class="field-group">
          <label for="analysis-mode">{t('analysis.periodMode')}</label>
          <select id="analysis-mode" bind:value={periodMode} disabled={running}>
            <option value="month">{t('analysis.monthMode')}</option>
            <option value="range">{t('analysis.rangeMode')}</option>
          </select>
        </div>
        {#if periodMode === 'month'}
          <div class="field-group">
            <label for="analysis-month">{t('analysis.month')}</label>
            <input id="analysis-month" type="month" max={localISOMonth()} bind:value={month} disabled={running} />
          </div>
        {:else}
          <div class="field-group"><label for="analysis-from">{t('excel.from')}</label><input id="analysis-from" type="date" bind:value={from} aria-invalid={rangeInvalid} disabled={running} /></div>
          <div class="field-group"><label for="analysis-to">{t('excel.to')}</label><input id="analysis-to" type="date" bind:value={to} aria-invalid={rangeInvalid} disabled={running} /></div>
          {#if periodMode === 'range'}
            <div class="field-group week-compare-field">
              <label class="week-compare-toggle" for="analysis-week-compare">
                <input id="analysis-week-compare" type="checkbox" bind:checked={weekCompare} disabled={running} />
                <span>{t('analysis.weekMode')}</span>
              </label>
              <small>{t('analysis.weekModeHint')}</small>
            </div>
          {/if}
        {/if}
      </div>

      <div class="store-selection" aria-labelledby="analysis-stores-label">
        <div class="selection-heading">
          <strong id="analysis-stores-label">{t('analysis.stores')} <span class="selection-count">{t('analysis.selectedStoreCount', { selected: selectedStoreIds.size, total: stores.length })}</span></strong>
          <div><button type="button" onclick={selectAllStores} disabled={running || loadingStores || visibleStores.length === 0}>{t(storeQuery.trim() ? 'analysis.selectMatches' : 'analysis.selectAll')}</button><button type="button" onclick={clearStores} disabled={running || loadingStores || !visibleStores.some((store) => selectedStoreIds.has(store.businessId))}>{t(storeQuery.trim() ? 'analysis.clearMatches' : 'analysis.clear')}</button></div>
        </div>
        {#if loadingStores}
          <div class="inline-loading"><md-circular-progress indeterminate></md-circular-progress>{t('analysis.loadingStores')}</div>
        {:else if stores.length === 0}
          <div class="store-empty">{t('analysis.noStores')}</div>
        {:else}
          {#if stores.length > 8}
            <div class="analysis-search store-search">
              <span class="material-symbols-rounded" aria-hidden="true">search</span>
              <input aria-label={t('analysis.searchStores')} placeholder={t('analysis.searchStores')} bind:value={storeQuery} />
              {#if storeQuery}<button type="button" aria-label={t('analysis.clear')} onclick={() => { storeQuery = ''; }}><span class="material-symbols-rounded" aria-hidden="true">close</span></button>{/if}
            </div>
          {/if}
          {#if storeQuery.trim()}<p class="selection-scope">{t('analysis.storeMatchHint', { count: visibleStores.length })}</p>{/if}
          <div class="store-grid pane-scroll">
            {#each visibleStores as store (store.businessId)}
              <label class:checked={selectedStoreIds.has(store.businessId)}><input type="checkbox" checked={selectedStoreIds.has(store.businessId)} disabled={running} onchange={() => toggleStore(store.businessId)} /><span class="store-id">{store.businessId}</span><span class="store-name">{store.label}</span></label>
            {:else}
              <div class="store-empty">{t('analysis.noResults')}</div>
            {/each}
          </div>
        {/if}
      </div>

      {#if isWebRuntime()}<div class="inline-blocker"><span class="material-symbols-rounded" aria-hidden="true">info</span>{t('web.previewHint')}</div>{/if}
      {#if rangeInvalid}<div class="inline-blocker"><span class="material-symbols-rounded" aria-hidden="true">event_busy</span>{t('excel.rangeInvalid')}</div>{/if}

      <div class="analysis-query-actions">
        <span>{t('analysis.selectedStores', { count: selectedStoreIds.size })}</span>
        <md-filled-button type="button" onclick={() => void runAnalysis()} disabled={(loadingStores || running || selectedStoreIds.size === 0 || rangeInvalid || (periodMode === 'month' ? !month : !from || !to)) ? true : undefined}><span class="material-symbols-rounded" slot="icon" aria-hidden="true">query_stats</span>{result && queryDirty ? t('analysis.rerun') : t('analysis.run')}</md-filled-button>
      </div>
    </form>
  {/if}

  {#if queryDirty || stagedPreset}
    <div class="query-draft-notice" role="status">
      <span class="material-symbols-rounded" aria-hidden="true">edit_note</span>
      <span>{stagedPreset ? t('presets.staged', { name: stagedPreset.name }) : t('analysis.queryDirty')}</span>
      {#if presetWarning}<strong class="preset-warning">{presetWarning}</strong>{/if}
      <div class="query-draft-actions">
        <button type="button" onclick={restoreQuery} disabled={running || exportingPDF}>{t(!result && stagedPreset ? 'presets.discard' : 'analysis.restoreQuery')}</button>
        {#if !queryOpen}<button type="button" onclick={changeQuery}>{t('analysis.adjustQuery')}</button>{/if}
      </div>
    </div>
  {/if}

  {#if running}
    <section class="analysis-progress surface-card" aria-live="polite">
      <div class="analysis-progress-heading">
        <div><h2>{t('analysis.running')}</h2>{#if progress?.periodLabel || progress?.storeId}<strong>{[progress?.periodLabel, progress?.storeLabel || progress?.storeId].filter(Boolean).join(' · ')}</strong>{/if}</div>
        <span>{progressPercent}%</span>
      </div>
      <md-linear-progress value={progress?.total ? progress.current / progress.total : 0}></md-linear-progress>
      <div class="analysis-progress-footer">
        <strong>{t('excel.progressCount', { current: progress?.current ?? 0, total: progress?.total ?? selectedStoreIds.size * buildPeriodRequests().length })}</strong>
        <md-outlined-button type="button" onclick={() => void cancelAnalysis()} disabled={!operationId || cancelling}>{cancelling ? t('common.loading') : t('common.cancel')}</md-outlined-button>
      </div>
    </section>
  {/if}

  {#if result && currentPeriod}
    <section class="analysis-results">
      {#if result.pending}
        <section class="analysis-supplement" aria-live="polite">
          <div class="analysis-supplement-copy">
            <strong>{t('analysis.supplementing')}</strong>
            <span>{progress?.periodLabel || progress?.storeLabel ? [progress?.periodLabel, progress?.storeLabel || progress?.storeId].filter(Boolean).join(' · ') : t('analysis.supplementingHint')}</span>
          </div>
          <div class="analysis-supplement-meter">
            <md-linear-progress value={progress?.total ? progress.current / progress.total : 0}></md-linear-progress>
            <strong>{t('excel.progressCount', { current: progress?.current ?? 0, total: progress?.total ?? 0 })}</strong>
          </div>
          <md-outlined-button type="button" onclick={() => void cancelAnalysis()} disabled={!(operationId || result.operationId) || cancelling}>{cancelling ? t('common.loading') : t('common.cancel')}</md-outlined-button>
        </section>
      {:else if !result.complete}<div class="notice warning-notice" role="status"><span class="material-symbols-rounded" aria-hidden="true">warning</span><span>{t('analysis.partialResult', { count: result.issues?.length ?? 0 })}</span></div>{/if}

      {#if itemFailureRows.length > 0}
        <section class="item-recovery" role="alert">
          <div><strong>{t('analysis.itemsFailed', { count: itemFailureRows.length })}</strong><p>{t('analysis.itemsRecoveryHint')}</p>
            <ul>{#each itemFailureRows as failure}<li><b>{failure.label}</b> · {failure.message}</li>{/each}</ul>
          </div>
          <button type="button" onclick={retryItemHydration} disabled={loadingItems || running || exportingPDF}>{t('analysis.retryItems')}</button>
        </section>
      {/if}

      <div class="report-navigation" bind:this={reportNavigation}>
        <div class="report-tabs" role="tablist" aria-label={t('analysis.reportViews')}>
          {#each reportTabs as tab}
            <button id={`report-tab-${tab.key}`} type="button" onkeydown={navigateReport} class:active={activeView === tab.key} role="tab" aria-selected={activeView === tab.key} aria-controls={`report-panel-${activeView}`} tabindex={activeView === tab.key ? 0 : -1} onclick={() => void selectReport(tab.key as ReportView)}><span class="material-symbols-rounded" aria-hidden="true">{tab.icon}</span>{tab.label}{#if result.pending && ((tab.key === 'weekly' && !result.weeks?.length) || (tab.key === 'focus' && !periodByKey(reportPeriods, 'yearAgoNext')) || (tab.key === 'stores' && reportPeriods.length < 2))}<span class="tab-pending">{t('common.loading')}</span>{/if}</button>
          {/each}
        </div>
        <div class="navigation-tools">
          {@render rankingControl()}
          <button class="return-to-filters" type="button" aria-label={t('analysis.returnToFilters')} title={t('analysis.returnToFilters')} onclick={returnToFilters}><span class="material-symbols-rounded" aria-hidden="true">manage_search</span>{#if activeFilterCount}<b>{activeFilterCount}</b>{/if}</button>
        </div>
      </div>

      <section class="report-filter" bind:this={reportFilter} aria-label={t('analysis.categoryFilters')}>
        <div class="filter-bar">
          <div class="analysis-search"><span class="material-symbols-rounded" aria-hidden="true">search</span><input bind:this={productSearch} aria-label={t('analysis.search')} placeholder={t('analysis.search')} value={search} oninput={changeSearch} />{#if search}<button type="button" aria-label={t('analysis.clear')} onclick={() => { search = ''; page = 1; }}><span class="material-symbols-rounded" aria-hidden="true">close</span></button>{/if}</div>
          <button class="filter-toggle" class:active={filtersOpen || activeFilterCount > 0} type="button" aria-expanded={filtersOpen} aria-controls="analysis-filter-panel" onclick={() => { filtersOpen = !filtersOpen; openFacet = ''; }}><span class="material-symbols-rounded" aria-hidden="true">filter_list</span>{t('analysis.filterToggle')}{#if activeFilterCount}<b>{activeFilterCount}</b>{/if}<span class="material-symbols-rounded" aria-hidden="true">{filtersOpen ? 'expand_less' : 'expand_more'}</span></button>
          <div class="filter-tools">
            <span class="scope-count" role="status">{currentDetailsMissing ? t('analysis.itemsNotReady') : t(productScopeActive ? 'analysis.filteredRows' : 'analysis.allRows', { count: filteredItems.length })}</span>
            {#if loadingItems}<span class="chrome-busy" role="status"><span class="material-symbols-rounded" aria-hidden="true">progress_activity</span><span>{t('analysis.loadingItems')}</span>{#if hydratingPeriod}<span>· {hydratingPeriod}</span>{/if}</span>{/if}
            <AnalysisDataActions {t} tables={dataTables[activeView] ?? []} context={dataContext} filename={`RTA-${activeView}-${result.from}-${result.to}.xlsx`} compact disabled={running || exportingPDF || scopedViewWaiting || (activeView !== 'weekly' && viewMissingPeriods.length > 0)} onBusy={(value) => { exportingData = value; }} />
          </div>
        </div>
        <div id="analysis-filter-panel" class="filter-panel" hidden={!filtersOpen}>
          <p class="filter-hint">{t('analysis.filterHint')}</p>
          <div class="filter-fields">
        <div class="facet-row" aria-label={t('analysis.categoryFilters')}>
          {#each facets as facet}
            <details class="facet-menu" open={openFacet === facet.key}>
              <summary class:active={selections[facet.key].size > 0} onclick={(event) => { event.preventDefault(); toggleFacetMenu(facet.key); }}><span>{t(facet.label)}</span><strong>{selections[facet.key].size > 0 ? selections[facet.key].size : t('analysis.all')}</strong><span class="material-symbols-rounded" aria-hidden="true">expand_more</span></summary>
              <div class="facet-popover">
                <div class="facet-actions"><button type="button" onclick={() => selectFacetAll(facet.key)} disabled={visibleFacetOptions(facet.key, facetSearch, facetOptionMap[facet.key]).length === 0}>{t(facetSearch.trim() ? 'analysis.selectMatches' : 'analysis.selectAll')}</button><button type="button" onclick={() => clearFacet(facet.key)}>{t(facetSearch.trim() ? 'analysis.clearMatches' : 'analysis.clear')}</button></div>
                <input class="facet-search" aria-label={t('analysis.facetSearch', { name: t(facet.label) })} placeholder={t('analysis.facetSearch', { name: t(facet.label) })} bind:value={facetSearch} />
                <div class="facet-options pane-scroll">
                  {#each visibleFacetOptions(facet.key, facetSearch, facetOptionMap[facet.key]) as option}
                    <label><input aria-label={option} type="checkbox" checked={selections[facet.key].has(option)} onchange={() => toggleFacet(facet.key, option)} /><span>{option}</span></label>
                  {:else}
                    <div class="facet-empty">{loadingItems ? t('analysis.loadingItems') : t('analysis.noResults')}</div>
                  {/each}
                </div>
              </div>
            </details>
          {/each}
        </div>
        {#if manCodeGroups.length > 0}
          <label class="promoter-group-selector">
            <span>{t('analysis.promoterGroup')}</span>
            <select aria-label={t('analysis.promoterGroup')} value={selectedGroupId} onchange={changePromoterGroup}>
              <option value="">{t('analysis.allProducts')}</option>
              {#each manCodeGroups as group (group.id)}
                <option value={group.id}>{group.name} ({group.codes.length})</option>
              {/each}
            </select>
          </label>
        {/if}
      </div>

        </div>
        {#if activeFilterCount > 0}
          <div class="active-filters" aria-label={t('analysis.activeFilters')}>
            {#each facets as facet}
              {#each [...selections[facet.key]] as value}
                <button class="filter-chip" type="button" aria-label={t('analysis.removeFilter', { name: value })} title={`${t(facet.label)}: ${value}`} onclick={() => toggleFacet(facet.key, value)}><span>{t(facet.label)}: {value}</span><span class="material-symbols-rounded" aria-hidden="true">close</span></button>
              {/each}
            {/each}
            {#if search.trim()}<button class="filter-chip" type="button" aria-label={t('analysis.removeFilter', { name: search.trim() })} onclick={() => { search = ''; page = 1; }}><span>“{search.trim()}”</span><span class="material-symbols-rounded" aria-hidden="true">close</span></button>{/if}
            {#if selectedGroup}<button class="filter-chip" type="button" aria-label={t('analysis.removeFilter', { name: selectedGroup.name })} onclick={() => { selectedGroupId = ''; page = 1; }}><span>{selectedGroup.name}</span><span class="material-symbols-rounded" aria-hidden="true">close</span></button>{/if}
            <button class="clear-filters" type="button" onclick={clearScreenFilters}>{t('analysis.clearFilters')}</button>
          </div>
        {/if}
      </section>

      <div class="analysis-workspace" bind:this={reportWorkspace} id={`report-panel-${activeView}`} role="tabpanel" aria-labelledby={`report-tab-${activeView}`} tabindex="0">
      {#if scopedViewWaiting}
        <div class="filter-empty" role="status"><span class="material-symbols-rounded" aria-hidden="true">hourglass_top</span><div><strong>{t('analysis.itemsNotReady')}</strong><p>{t(viewMissingPeriods.some((period) => hydrationFailures[period.key]) ? 'analysis.itemsUnavailable' : 'analysis.itemsPreparing')}</p></div></div>
      {:else}
      {#if productScopeActive && !currentDetailsMissing && filteredItems.length === 0}
        <div class="filter-empty" role="status"><span class="material-symbols-rounded" aria-hidden="true">search_off</span><div><strong>{t('analysis.noResults')}</strong><p>{t('analysis.noMatchHint')}</p></div></div>
      {/if}
      {#if activeView === 'overview'}
        <dl class="analysis-kpis">
          <div class="kpi-primary"><dt>{t('analysis.netSales')}</dt><dd>{formatMoney(currentTotals.netSalesAmount)}</dd><span class={deltaClass(delta(currentTotals.netSalesAmount, yearAgoTotals.netSalesAmount))}>{formatPercent(delta(currentTotals.netSalesAmount, yearAgoTotals.netSalesAmount))} {t('analysis.vsYearAgo')}</span></div>
          <div><dt>{t('analysis.grossSales')}</dt><dd>{formatMoney(currentTotals.saleAmount)}</dd><span class={deltaClass(delta(currentTotals.saleAmount, previousTotals.saleAmount))}>{formatPercent(delta(currentTotals.saleAmount, previousTotals.saleAmount))} {t('analysis.vsPrevious')}</span></div>
          <div><dt>{t('analysis.returns')}</dt><dd>{formatMoney(currentTotals.returnAmount)}</dd><span>{formatNumber(currentTotals.returnQuantity)} {t('analysis.units')}</span></div>
          <div><dt>{t('analysis.netQuantity')}</dt><dd>{formatNumber(currentTotals.netQuantity)}</dd><span class={deltaClass(delta(currentTotals.netQuantity, yearAgoTotals.netQuantity))}>{formatPercent(delta(currentTotals.netQuantity, yearAgoTotals.netQuantity))} {t('analysis.vsYearAgo')}</span></div>
        </dl>
        <dl class="analysis-secondary-kpis">
          <div><dt>{t('analysis.transactions')}</dt><dd>{formatValue(currentTotals.transactionCount, 'number')}</dd><span>{groupScopeActive || filtersActive(selections, search) ? t('analysis.wholeStoreOnly') : t('analysis.orders')}</span></div>
          <div><dt>{t('analysis.basket')}</dt><dd>{formatValue(currentTotals.basketValue, 'money')}</dd><span>{groupScopeActive || filtersActive(selections, search) ? t('analysis.wholeStoreOnly') : t('analysis.perOrder')}</span></div>
          <div><dt>{t('analysis.skus')}</dt><dd>{formatNumber(currentTotals.skuCount)}</dd><span>{t('analysis.products')}</span></div>
        </dl>

        {#if insightData}<AnalysisInsights {t} locale={settings.locale} data={insightData} onProduct={openProduct} />{/if}

        <section class="performance-card surface-card" aria-labelledby="performance-title">
          <details class="performance-details">
          <summary class="performance-summary"><h2 id="performance-title">{t('analysis.performance')}</h2><span>{t('analysis.currentPeriod')} / {t('analysis.previousPeriod')} / {t('analysis.yearAgoPeriod')}</span><span class="material-symbols-rounded" aria-hidden="true">expand_more</span></summary>
          <div class="table-scroll"><table>
            <thead><tr><th>{t('analysis.metric')}</th><th class="numeric">{t('analysis.currentPeriod')}</th><th class="numeric">{t('analysis.previousPeriod')}</th><th class="numeric">{t('analysis.yearAgoPeriod')}</th><th class="numeric">{t('analysis.vsPrevious')}</th><th class="numeric">{t('analysis.vsYearAgo')}</th></tr></thead>
            <tbody>{#each performanceRows as row}<tr><th>{row.label}</th><td class="numeric emphasis">{formatValue(row.current, row.format)}</td><td class="numeric">{formatValue(row.previous, row.format)}</td><td class="numeric">{formatValue(row.yearAgo, row.format)}</td><td class={`numeric ${deltaClass(delta(row.current, row.previous))}`}>{formatPercent(delta(row.current, row.previous))}</td><td class={`numeric ${deltaClass(delta(row.current, row.yearAgo))}`}>{formatPercent(delta(row.current, row.yearAgo))}</td></tr>{/each}</tbody>
          </table></div>
          </details>
        </section>

        <div class="top-grid">
          <section class="top-card surface-card" aria-labelledby="top-sales-title">
            <div class="section-heading"><h2 id="top-sales-title">{t('analysis.topSales', { count: settings.rankingLimit })}</h2></div>
            <ol>{#each topSales as item, index}<li class:podium={index < 3}><span class="rank">{index + 1}</span><div class="top-product">{@render productName(item.code, item.name)}<span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="top-metrics"><b>{formatMoney(item.amount)}</b><span>{formatNumber(item.quantity)} {t('analysis.units')}</span></div></li>{:else}<li class="empty-row">{itemEmptyLabel(currentPeriod, hydrationFailures)}</li>{/each}</ol>
          </section>
          <section class="top-card surface-card" aria-labelledby="top-quantity-title">
            <div class="section-heading"><h2 id="top-quantity-title">{t('analysis.topQuantity', { count: settings.rankingLimit })}</h2></div>
            <ol>{#each topQuantity as item, index}<li class:podium={index < 3}><span class="rank">{index + 1}</span><div class="top-product">{@render productName(item.code, item.name)}<span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="top-metrics"><b>{formatNumber(item.quantity)} {t('analysis.units')}</b><span>{formatMoney(item.amount)}</span></div></li>{:else}<li class="empty-row">{itemEmptyLabel(currentPeriod, hydrationFailures)}</li>{/each}</ol>
          </section>
        </div>
      {:else if activeView === 'weekly'}
        <section class="comparison-card surface-card" aria-labelledby="weekly-title">
          <div class="ranking-heading">
            <div>
              <h2 id="weekly-title">{t('analysis.weeklyTitle')}</h2>
              {#if weeklyWeek}<span>{weeklyWeek.from} — {weeklyWeek.to}</span>{/if}
            </div>
            {#if weeklyPeriods.length > 1}
              <div class="ranking-period-tabs" role="tablist" aria-label={t('analysis.weekly')}>
                {#each weeklyPeriods as week}
                  <button type="button" class:active={`${week.from}:${week.to}` === weeklyKey} role="tab" aria-selected={`${week.from}:${week.to}` === weeklyKey} onclick={() => { weeklyKey = `${week.from}:${week.to}`; }}>{week.from.slice(5)}–{week.to.slice(5)}</button>
                {/each}
              </div>
            {/if}
          </div>
          {#if !weeklyWeek}
            <div class="ranking-empty">{result.pending ? t('common.loading') : t('analysis.weeklyMissing')}</div>
          {:else}
            <p class="focus-note">{t(weeklyUsesAlignedComparison ? 'analysis.weeklyAlignedHint' : 'analysis.weeklyHint')}</p>
            <div class="store-table"><AnalysisDataTable table={dataTables.weekly![0]!} {t} locale={settings.locale} sort={tableSorts.weekly} onSort={sortData} /></div>
          {/if}
        </section>
      {:else if activeView === 'focus'}
        <section class="focus-section surface-card" aria-labelledby="focus-title">
          <div class="focus-heading">
            <div>
              <h2 id="focus-title">{t('analysis.focusTitle')}</h2>
              {#if focusPeriod}<span>{focusPeriod.label} · {focusPeriod.from} — {focusPeriod.to}</span>{/if}
            </div>
          </div>
          {#if !focusPeriod}
            <div class="ranking-empty">{result.pending ? t('common.loading') : t('analysis.focusMissing')}</div>
          {:else}
            <p class="focus-note">{t('analysis.focusHint')}</p>
            {#if periodNeedsItemHydration(focusPeriod)}
              <div class="ranking-empty">{t(hydrationFailures[focusPeriod.key] ? 'analysis.itemsUnavailable' : 'common.loading')}</div>
            {:else}<div class="focus-grid">
              {#each focusGroups as group (group.id)}
                <article class="focus-group">
                  <header><strong>{focusGroupLabel(group)}</strong>{#if group.prefix}<span>{group.prefix}</span>{/if}</header>
                  <div class="focus-columns">
                    <div>
                      <h3>{t('analysis.focusSales')}</h3>
                      <ol>
                        {#each group.sales as item, index}
                          <li>
                            <span class="rank">{index + 1}</span>
                            <div>{@render productName(item.code, item.name || item.code, focusPeriod.key)}<span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div>
                            <div class="top-metrics"><b>{formatMoney(item.amount)}</b><span>{formatNumber(item.quantity)} {t('analysis.units')}</span>{#if item.currentAmount || item.currentQuantity}<em>{t('analysis.focusCurrent')} {formatMoney(item.currentAmount)}</em>{/if}</div>
                          </li>
                        {:else}<li class="empty-row">{t('analysis.noResults')}</li>{/each}
                      </ol>
                    </div>
                    <div>
                      <h3>{t('analysis.focusQuantity')}</h3>
                      <ol>
                        {#each group.quantity as item, index}
                          <li>
                            <span class="rank">{index + 1}</span>
                            <div>{@render productName(item.code, item.name || item.code, focusPeriod.key)}<span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div>
                            <div class="top-metrics"><b>{formatNumber(item.quantity)} {t('analysis.units')}</b><span>{formatMoney(item.amount)}</span>{#if item.currentAmount || item.currentQuantity}<em>{t('analysis.focusCurrent')} {formatNumber(item.currentQuantity)} {t('analysis.units')}</em>{/if}</div>
                          </li>
                        {:else}<li class="empty-row">{t('analysis.noResults')}</li>{/each}
                      </ol>
                    </div>
                  </div>
                </article>
              {:else}<div class="ranking-empty">{t('analysis.noResults')}</div>{/each}
            </div>
            {/if}
          {/if}
        </section>
      {:else if activeView === 'categories'}
        <section class="comparison-card surface-card" aria-labelledby="category-title">
          <div class="comparison-heading"><h2 id="category-title">{t('analysis.rolling')}</h2><div class="group-tabs" role="radiogroup" aria-label={t('analysis.groupBy')}>{#each facets as facet}<button type="button" class:active={groupLevel === facet.key} role="radio" aria-checked={groupLevel === facet.key} onclick={() => { groupLevel = facet.key; }}>{t(facet.label)}</button>{/each}</div></div>
          <div class="category-table"><AnalysisDataTable table={dataTables.categories![0]!} {t} locale={settings.locale} sort={tableSorts.categories} onSort={sortData} /></div>
        </section>

        <section class="ranking-section surface-card" aria-labelledby="sales-ranking-title">
          <div class="ranking-heading">
            <div><h2 id="sales-ranking-title">{t('analysis.categorySalesRanking')}</h2><span>{t(facets.find((facet) => facet.key === groupLevel)?.label ?? 'analysis.category2')}</span></div>
            <div class="ranking-period-tabs" role="tablist" aria-label={t('analysis.salesRankingPeriods')}>
              {#each salesRankingPeriods as period}
                <button type="button" class:active={salesRankingPeriod?.key === period.key} role="tab" aria-selected={salesRankingPeriod?.key === period.key} onclick={() => { salesRankingKey = period.key; }}>{period.label}</button>
              {/each}
            </div>
          </div>
          <div class="ranking-grid">
            {#each salesRankingGroups as group (group.id)}
              <article class="ranking-group">
                <header><div><strong>{group.name}</strong>{#if group.code}<span>{group.code}</span>{/if}</div><b>{formatMoney(group.amount)}</b></header>
                <ol>{#each group.items as item, index}<li><span class="rank">{index + 1}</span><div class="ranking-product">{@render productName(item.code, item.name, salesRankingPeriod?.key)}<span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="ranking-values"><b>{formatMoney(item.amount)}</b><span>{formatNumber(item.quantity)} {t('analysis.units')}</span></div></li>{/each}</ol>
              </article>
            {:else}<div class="ranking-empty">{itemEmptyLabel(salesRankingPeriod, hydrationFailures)}</div>{/each}
          </div>
        </section>

        <section class="ranking-section surface-card" aria-labelledby="quantity-ranking-title">
          <div class="ranking-heading">
            <div><h2 id="quantity-ranking-title">{t('analysis.monthlyQuantityRanking')}</h2><span>{t(facets.find((facet) => facet.key === groupLevel)?.label ?? 'analysis.category2')}</span></div>
            <div class="ranking-period-tabs" role="tablist" aria-label={t('analysis.quantityRankingPeriods')}>
              {#each quantityRankingPeriods as period}
                <button type="button" class:active={quantityRankingPeriod?.key === period.key} role="tab" aria-selected={quantityRankingPeriod?.key === period.key} onclick={() => { quantityRankingKey = period.key; }}>{period.label}</button>
              {/each}
            </div>
          </div>
          <div class="ranking-grid">
            {#each quantityRankingGroups as group (group.id)}
              <article class="ranking-group">
                <header><div><strong>{group.name}</strong>{#if group.code}<span>{group.code}</span>{/if}</div><b>{formatNumber(group.quantity)} {t('analysis.units')}</b></header>
                <ol>{#each group.items as item, index}<li><span class="rank">{index + 1}</span><div class="ranking-product">{@render productName(item.code, item.name, quantityRankingPeriod?.key)}<span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="ranking-values"><b>{formatNumber(item.quantity)} {t('analysis.units')}</b><span>{formatMoney(item.amount)}</span></div></li>{/each}</ol>
              </article>
            {:else}<div class="ranking-empty">{itemEmptyLabel(quantityRankingPeriod, hydrationFailures)}</div>{/each}
          </div>
        </section>
      {:else if activeView === 'products'}
        <section class="analysis-table-card surface-card" aria-labelledby="items-title">
          <div class="analysis-table-heading"><h2 id="items-title">{t('analysis.items')}</h2><strong>{t('common.items', { count: filteredItems.length })}</strong></div>
          <AnalysisDataTable table={dataTables.products![0]!} {t} locale={settings.locale} sort={tableSorts.products} onSort={sortData} onProduct={openProduct} paginated />
        </section>
      {:else}
        <section class="comparison-card surface-card" aria-labelledby="stores-title">
          <div class="section-heading"><h2 id="stores-title">{t('analysis.storeComparison')}</h2></div>
          <div class="store-table"><AnalysisDataTable table={dataTables.stores![0]!} {t} locale={settings.locale} sort={tableSorts.stores} onSort={sortData} /></div>
        </section>
      {/if}
      {/if}
      </div>
    </section>
  {/if}
</section>

{#if selectedProduct && result}
  <ProductDetailsDialog {t} locale={settings.locale} code={selectedProduct.code} name={selectedProduct.name} initialPeriod={selectedProduct.periodKey}
    periods={reportPeriods} context={dataContext.slice(0, reportPeriods.length + 2)} failures={hydrationFailures} loading={loadingItems} pending={Boolean(result.pending)}
    onRetry={retryItemHydration} onClose={() => { selectedProduct = undefined; }} onBusy={(value) => { exportingData = value; }} />
{/if}

{#if presetsOpen}
  <AnalysisPresetsDialog {t} locale={settings.locale} draft={presetSaveDraft} groups={manCodeGroups} onClose={closePresets} onApply={stagePreset} />
{/if}

{#if exportDialog && result}
  <dialog use:modal={{ busy: exportingPDF, onClose: closeExportDialog }} class="app-dialog export-dialog" aria-modal="true" aria-labelledby="export-dialog-title">
    <div class="dialog-header">
      <h2 id="export-dialog-title">{t('analysis.exportDialogTitle')}</h2>
      <md-icon-button aria-label={t('common.close')} onclick={closeExportDialog} disabled={exportingPDF}><span class="material-symbols-rounded">close</span></md-icon-button>
    </div>
    <form class="export-dialog-body" onsubmit={(event) => { event.preventDefault(); void exportPDF(); }}>
      <div class="export-dialog-scroll pane-scroll" tabindex="-1" data-autofocus>
        <div class="export-dialog-grid">
          <section class="export-section export-section-categories" aria-labelledby="export-category-title">
            <div class="export-section-heading">
              <div>
                <h3 id="export-category-title">{t('analysis.exportCategorySection')}</h3>
                <p>{t('analysis.exportContentHint')}</p>
              </div>
            </div>
            {#if exportCanUseScreenFilters && exportUseScreenFilters}
              <div class="export-mode" role="radiogroup" aria-label={t('analysis.exportCategorySection')}>
                <button type="button" class:active={true} role="radio" aria-checked={true} disabled={exportingPDF} onclick={() => setExportUseScreenFilters(true)}>{t('analysis.exportUseScreenFilters')}</button>
                <button type="button" class:active={false} role="radio" aria-checked={false} disabled={exportingPDF} onclick={() => setExportUseScreenFilters(false)}>{t('analysis.exportIgnoreScreenFilters')}</button>
              </div>
              <div class="export-screen-filters">
                {#each exportScreenFilterRows() as row}
                  <div class="export-screen-filter">
                    <strong>{row.label} · {t('analysis.exportScreenFilterCount', { count: row.values.length })}</strong>
                    <ul>{#each row.values as value}<li>{value}</li>{/each}</ul>
                  </div>
                {/each}
              </div>
            {:else}
              <div class="export-all-copy">
                <strong>{t('analysis.exportAllNow')}</strong>
                <p>{t('analysis.exportAllNowHint')}</p>
              </div>
              {#if exportCanUseScreenFilters}
                <div class="export-mode" role="radiogroup" aria-label={t('analysis.exportCategorySection')}>
                  <button type="button" class:active={false} role="radio" aria-checked={false} disabled={exportingPDF} onclick={() => setExportUseScreenFilters(true)}>{t('analysis.exportUseScreenFilters')}</button>
                  <button type="button" class:active={true} role="radio" aria-checked={true} disabled={exportingPDF} onclick={() => setExportUseScreenFilters(false)}>{t('analysis.exportIgnoreScreenFilters')}</button>
                </div>
              {/if}
            {/if}
            <div class="export-flags">
              <label class="export-check"><input type="checkbox" checked={exportFilter.excludeZeroGifts} disabled={exportingPDF} onchange={() => { exportFilter = { ...exportFilter, excludeZeroGifts: !exportFilter.excludeZeroGifts }; }} /><span>{t('analysis.exportSkipGifts')}</span></label>
              <label class="export-check"><input type="checkbox" checked={exportFilter.excludeStamps} disabled={exportingPDF} onchange={() => { exportFilter = { ...exportFilter, excludeStamps: !exportFilter.excludeStamps }; }} /><span>{t('analysis.exportSkipStamps')}</span></label>
            </div>
            {#if !exportUseScreenFilters}
              <details class="export-advanced">
                <summary>{t('analysis.exportAdvancedCategories')}</summary>
                <div class="export-mode" role="radiogroup" aria-label={t('analysis.exportAdvancedCategories')}>
                  <button type="button" class:active={exportFilter.mode === 'blacklist'} role="radio" aria-checked={exportFilter.mode === 'blacklist'} disabled={exportingPDF} onclick={() => setExportMode('blacklist')}>{t('analysis.exportModeAll')}</button>
                  <button type="button" class:active={exportFilter.mode === 'whitelist'} role="radio" aria-checked={exportFilter.mode === 'whitelist'} disabled={exportingPDF} onclick={() => setExportMode('whitelist')}>{t('analysis.exportModeOnly')}</button>
                </div>
                <div class="export-choice-panel">
                  <div class="export-choice-heading">
                    <strong id="export-content-title">{exportFilter.mode === 'whitelist' ? t('analysis.exportKeepCategories') : t('analysis.exportSkipCategories')}</strong>
                    <div>
                      <button type="button" disabled={exportingPDF} onclick={() => { exportFilter = { ...exportFilter, categories: exportCategoryOptions().map((option) => option.id) }; }}>{t('analysis.selectAll')}</button>
                      <button type="button" disabled={exportingPDF} onclick={() => { exportFilter = { ...exportFilter, categories: [] }; }}>{t('analysis.clear')}</button>
                    </div>
                  </div>
                  <div class="export-choice-list export-choice-list-categories pane-scroll">
                    {#each exportCategoryOptions() as option (option.id)}
                      <label><input type="checkbox" checked={exportFilter.categories.includes(option.id)} disabled={exportingPDF} onchange={() => toggleExportCategory(option.id)} /><span>{option.code ? `${option.code}  ${option.name}` : option.name}</span></label>
                    {:else}
                      <div class="export-choice-empty">{t('analysis.noResults')}</div>
                    {/each}
                  </div>
                </div>
              </details>
            {/if}
          </section>
          <section class="export-section" aria-labelledby="export-outputs-title">
            <div class="export-section-heading">
              <h3 id="export-outputs-title">{t('analysis.exportOutputs')}</h3>
            </div>
            <label class="export-check export-check-card">
              <input type="checkbox" checked={exportPDFEnabled} disabled={exportingPDF} onchange={() => { exportPDFEnabled = !exportPDFEnabled; }} />
              <span><b>{t('analysis.exportOutputPDF')}</b><small>{t('analysis.exportOutputPDFHint')}</small></span>
            </label>
            <label class="export-check export-check-card">
              <input type="checkbox" checked={exportAIEnabled} disabled={exportingPDF} onchange={() => { exportAIEnabled = !exportAIEnabled; }} />
              <span><b>{t('analysis.exportOutputAI')}</b><small>{t('analysis.exportOutputAIHint')}</small></span>
            </label>
            <div class="export-ranking-limit">{@render rankingControl(exportingPDF)}</div>
          </section>
          {#if listSuccessfulReportStores(result).length > 1}
            <section class="export-section" aria-labelledby="export-files-title">
              <div class="export-section-heading">
                <h3 id="export-files-title">{t('analysis.exportFiles')}</h3>
                <span>{t('analysis.exportFileCount', { count: exportFilesCount })}</span>
              </div>
              <label class="export-check export-check-card">
                <input type="checkbox" checked={exportIncludeCombined} disabled={exportingPDF} onchange={() => { exportIncludeCombined = !exportIncludeCombined; }} />
                <span><b>{t('analysis.exportFilesCombined')}</b><small>{t('analysis.exportFilesCombinedHint')}</small></span>
              </label>
              <div class="export-choice-panel">
                <div class="export-choice-heading">
                  <strong>{t('analysis.exportStoreReports')}</strong>
                  <div>
                    <button type="button" aria-label={t('analysis.exportSelectAllStores')} disabled={exportingPDF} onclick={() => { if (result) exportStoreIds = new Set(listSuccessfulReportStores(result).map((store) => store.businessId)); }}>{t('analysis.selectAll')}</button>
                    <button type="button" aria-label={t('analysis.exportClearStores')} disabled={exportingPDF} onclick={() => { exportStoreIds = new Set(); }}>{t('analysis.clear')}</button>
                  </div>
                </div>
                <div class="export-choice-list pane-scroll">
                  {#each listSuccessfulReportStores(result) as store (store.businessId)}
                    <label><input type="checkbox" checked={exportStoreIds.has(store.businessId)} disabled={exportingPDF} onchange={() => toggleExportStore(store.businessId)} /><span><b>{store.businessId}</b> {store.label}</span></label>
                  {/each}
                </div>
              </div>
            </section>
          {/if}
          {#if manCodeGroups.length > 0}
            <section class="export-section" aria-labelledby="export-groups-title">
              <div class="export-section-heading">
                <div>
                  <h3 id="export-groups-title">{t('analysis.exportPromoterGroups')}</h3>
                  <p>{t('analysis.exportPromoterGroupsHint')}</p>
                </div>
              </div>
              <div class="export-mode" role="radiogroup" aria-label={t('analysis.exportPromoterGroups')}>
                <button type="button" class:active={!exportGroupDetail} role="radio" aria-checked={!exportGroupDetail} disabled={exportingPDF} onclick={() => { exportGroupDetail = false; }}>{t('analysis.exportGroupSummaryOnly')}</button>
                <button type="button" class:active={exportGroupDetail} role="radio" aria-checked={exportGroupDetail} disabled={exportingPDF || !exportPDFEnabled} onclick={() => { exportGroupDetail = true; }}>{t('analysis.exportGroupDetailFiles')}</button>
              </div>
              <div class="export-choice-panel">
                <div class="export-choice-heading">
                  <strong>{t('analysis.exportReportScope')}</strong>
                  <div>
                    <button type="button" disabled={exportingPDF} onclick={() => { exportGroupIds = new Set(manCodeGroups.map((group) => group.id)); }}>{t('analysis.selectAll')}</button>
                    <button type="button" disabled={exportingPDF} onclick={() => { exportGroupIds = new Set(); }}>{t('analysis.clear')}</button>
                  </div>
                </div>
                <div class="export-choice-list pane-scroll">
                  {#each manCodeGroups as group (group.id)}
                    <label><input type="checkbox" checked={exportGroupIds.has(group.id)} disabled={exportingPDF} onchange={() => toggleExportGroup(group.id)} /><span><b>{group.name}</b> · {t('analysis.itemCodeCount', { count: group.codes.length })}</span></label>
                  {/each}
                </div>
              </div>
            </section>
          {/if}
        </div>
      </div>
      {#if result.pending}
        <p class="export-dialog-warning">{t('analysis.exportIncompleteHint')}</p>
      {/if}
      {#if exportTargetTotal === 0}
        <p class="export-dialog-warning">{t('analysis.exportNeedTarget')}</p>
      {:else if !exportPDFEnabled && !exportAIEnabled}
        <p class="export-dialog-warning">{t('analysis.exportNeedOutput')}</p>
      {:else if !exportUseScreenFilters && exportFilter.mode === 'whitelist' && exportFilter.categories.length === 0}
        <p class="export-dialog-warning">{t('analysis.exportNeedCategory')}</p>
      {/if}
      <div class="dialog-actions export-dialog-actions">
        <span class="export-file-count">{t('analysis.exportFileCount', { count: exportFilesCount })}</span>
        <md-text-button type="button" onclick={closeExportDialog} disabled={exportingPDF}>{t('common.cancel')}</md-text-button>
        <md-filled-button type="submit" onclick={() => void exportPDF()} disabled={exportingPDF || exportFilesCount === 0 || (!exportUseScreenFilters && exportFilter.mode === 'whitelist' && exportFilter.categories.length === 0)}>{exportingPDF ? t('analysis.exportingPDFProgress', { current: pdfExportCurrent, total: pdfExportTotal }) : (isWebRuntime() ? t('web.exportConfirm') : t('analysis.exportConfirm'))}</md-filled-button>
      </div>
    </form>
  </dialog>
{/if}

<style>
  .analysis-page { max-width: 1480px; }
  .analysis-page.has-results { width: 100%; max-width: 1480px; }
  .analysis-page .page-heading { margin-bottom: 12px; align-items: flex-start; gap: 16px; flex-wrap: wrap; }
  .analysis-page .page-heading h1 { font-size: clamp(24px, 2vw, 28px); letter-spacing: -.02em; }
  .heading-copy { display: grid; min-width: 0; gap: 6px; }
  .heading-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 16px; min-width: 0; }
  .heading-flags { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 14px; }
  .report-context { margin: 0; color: var(--md-sys-color-on-surface-variant); font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; }
  .heading-meta .report-context { flex: 1 1 16rem; min-width: min(100%, 12rem); overflow-wrap: break-word; }
  .heading-meta .report-status, .heading-meta .period-disclosure summary { white-space: nowrap; }
  .analysis-page :is(button, summary, input, select):focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .analysis-page :is(button, input, select):disabled { cursor: not-allowed; opacity: .55; }
  .export-notice { display: flex; align-items: center; gap: 12px; }
  .export-notice-copy { display: grid; min-width: 0; flex: 1; gap: 2px; }
  .export-notice-copy code { overflow: hidden; color: var(--md-sys-color-on-surface-variant); font-family: "Cascadia Code", ui-monospace, monospace; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
  .analysis-results { display: grid; gap: 12px; min-width: 0; }
  .analysis-results > *, .analysis-workspace > * { min-width: 0; }
  .report-status { display: inline-flex; align-items: center; gap: 7px; min-height: 28px; font-size: 12px; color: var(--md-sys-color-on-surface-variant); }
  .report-status.ready { color: var(--md-sys-color-primary); }
  .status-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
  .period-disclosure { min-width: 0; }
  .heading-meta .period-disclosure[open] { flex: 1 1 100%; }
  .period-disclosure summary { display: flex; width: fit-content; align-items: center; gap: 4px; min-height: 28px; cursor: pointer; list-style: none; }
  .period-disclosure summary::-webkit-details-marker { display: none; }
  .period-disclosure summary .material-symbols-rounded { font-size: 18px; }
  .period-disclosure[open] summary .material-symbols-rounded { transform: rotate(180deg); }
  .period-summary { display: grid; gap: 8px; margin: 8px 0 0; padding: 12px 16px; border-left: 2px solid var(--app-active-border); font-variant-numeric: tabular-nums; }
  .period-summary > div { display: flex; flex-wrap: wrap; gap: 4px 16px; }
  .period-summary dt { min-width: 5em; font-weight: 650; }
  .period-summary dd { margin: 0; }
  .period-summary .current { color: var(--md-sys-color-primary); }
  .item-recovery { display: flex; align-items: flex-start; flex-wrap: wrap; gap: 12px; padding: 14px 16px; border: 1px solid var(--md-sys-color-error); border-radius: 12px; background: var(--app-card); }
  .item-recovery > div { flex: 1 1 240px; min-width: 0; }
  .item-recovery strong { color: var(--md-sys-color-error); font-size: 14px; }
  .item-recovery p, .item-recovery ul { margin: 6px 0 0; color: var(--md-sys-color-on-surface-variant); font-size: 12px; overflow-wrap: anywhere; }
  .item-recovery ul { padding-left: 18px; }
  .item-recovery button { padding: 8px 14px; min-height: 40px; border: 1px solid var(--app-active-border); border-radius: 10px; color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); font-weight: 650; cursor: pointer; }
  .navigation-tools { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .return-to-filters { display: inline-flex; align-items: center; justify-content: center; gap: 4px; min-width: 40px; min-height: 40px; padding: 4px 8px; border: 1px solid var(--app-border); border-radius: 9px; color: var(--md-sys-color-primary); background: var(--app-card); cursor: pointer; }
  .return-to-filters .material-symbols-rounded { font-size: 22px; }
  .return-to-filters b { font-size: 12px; }
  .report-navigation { position: sticky; top: 0; z-index: 24; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border: 1px solid var(--app-border); border-radius: 14px; background: var(--app-card); box-shadow: 0 4px 12px color-mix(in srgb, var(--md-sys-color-shadow) 5%, transparent); }
  .report-filter { min-width: 0; }
  .filter-bar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
  .filter-bar .analysis-search { flex: 1 1 220px; max-width: 400px; }
  .filter-toggle { display: inline-flex; align-items: center; gap: 7px; min-height: 44px; padding: 0 12px; border: 1px solid var(--app-border); border-radius: 12px; background: var(--app-card); color: var(--md-sys-color-on-surface-variant); font-weight: 650; cursor: pointer; }
  .filter-toggle.active { border-color: var(--app-active-border); color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); }
  .filter-toggle .material-symbols-rounded { font-size: 20px; }
  .filter-toggle b { min-width: 18px; font-size: 12px; }
  .filter-tools { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 8px 12px; margin-left: auto; min-width: 0; }
  .scope-count { font-size: 12px; color: var(--md-sys-color-on-surface-variant); font-variant-numeric: tabular-nums; }
  .filter-panel { margin-top: 12px; padding: 16px; border: 1px solid var(--app-border); border-radius: 14px; background: var(--app-card); }
  .filter-panel[hidden] { display: none; }
  .filter-hint { margin: 0 0 12px; font-size: 12px; color: var(--md-sys-color-on-surface-variant); }
  .filter-fields { display: flex; flex-wrap: wrap; align-items: flex-start; gap: 12px; }
  .filter-fields .facet-row { flex: 1 1 600px; }
  .filter-fields .promoter-group-selector { flex: 1 1 260px; max-width: 360px; }
  .active-filters { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 12px; }
  .filter-chip { display: inline-flex; align-items: center; gap: 6px; max-width: min(100%, 320px); min-height: 32px; padding: 4px 10px; border: 1px solid var(--app-active-border); border-radius: 8px; background: var(--md-sys-color-secondary-container); color: var(--md-sys-color-on-secondary-container); cursor: pointer; font-size: 12px; }
  .filter-chip > span:first-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .filter-chip .material-symbols-rounded { font-size: 16px; }
  .clear-filters, .query-draft-notice button { min-height: 32px; padding: 4px 8px; border: 0; background: transparent; color: var(--md-sys-color-primary); font-size: 12px; font-weight: 650; cursor: pointer; }
  .filter-empty { display: flex; align-items: center; gap: 14px; padding: 20px; border: 1px dashed var(--app-border); border-radius: 14px; color: var(--md-sys-color-on-surface-variant); }
  .filter-empty p { margin: 4px 0 0; font-size: 13px; }
  .chrome-busy { display: inline-flex; align-items: center; gap: 6px; color: var(--md-sys-color-primary); font-size: 12px; }
  .chrome-busy .material-symbols-rounded { font-size: 16px; }
  .analysis-workspace { display: grid; align-content: start; gap: 16px; overflow: visible; min-width: 0; scroll-margin-top: 90px; }
  .analysis-workspace:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 4px; }
  .query-draft-notice { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; padding: 10px 14px; margin: 12px 0; border-left: 3px solid var(--md-sys-color-primary); background: var(--md-sys-color-surface-container-low); color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
  .query-draft-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-left: auto; }
  .selection-scope { margin: 8px 0 0; font-size: 12px; color: var(--md-sys-color-on-surface-variant); }
  .store-search { margin-top: 12px; }
  .preset-shortcuts { display: flex; flex-wrap: wrap; gap: 6px 16px; padding: 0; }
  .shortcut-group { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; min-width: 0; }
  .shortcut-group > span { font-size: 11px; color: var(--md-sys-color-on-surface-variant); }
  .shortcut-group button { max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-height: 36px; border: 1px solid var(--md-sys-color-outline-variant); background: var(--md-sys-color-surface); color: var(--md-sys-color-primary); border-radius: 10px; padding: 7px 10px; font: inherit; font-size: 12px; font-weight: 650; cursor: pointer; }
  .shortcut-group button:disabled { opacity: .5; cursor: default; }
  .shortcut-group button:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .product-name-link { padding: 0; border: 0; background: transparent; color: var(--md-sys-color-primary); font: inherit; font-weight: 650; text-align: left; line-height: 1.45; cursor: pointer; overflow-wrap: anywhere; }
  .product-name-link:hover { text-decoration: underline; }
  .product-name-link:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .analysis-heading-actions { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 8px; flex-shrink: 0; }
  .preset-trigger { display: inline-flex; align-items: center; gap: 6px; min-height: 40px; padding: 8px 12px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 20px; background: var(--md-sys-color-surface); color: var(--md-sys-color-primary); font: inherit; font-size: 13px; cursor: pointer; }
  .preset-trigger .material-symbols-rounded { font-size: 18px; }
  .preset-trigger:disabled { opacity: .5; cursor: default; }
  .preset-trigger:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  .preset-warning { flex-basis: 100%; font-size: 12px; overflow-wrap: anywhere; color: var(--md-sys-color-error); }
  .analysis-loading { display: flex; min-height: 220px; align-items: center; justify-content: center; gap: 14px; }
  .analysis-loading md-circular-progress, .inline-loading md-circular-progress { width: 24px; height: 24px; }
  .analysis-query { padding: 24px; margin-bottom: 16px; }
  .analysis-query.compact { padding: 20px; }
  .query-section-heading { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
  .query-section-heading > .material-symbols-rounded { display: grid; width: 40px; height: 40px; place-items: center; border-radius: 12px; background: var(--md-sys-color-secondary-container); color: var(--md-sys-color-primary); }
  .query-section-heading h2 { margin: 0; font-size: 16px; }
  .query-section-heading p { margin: 4px 0 0; color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
  .selection-count { margin-left: 8px; color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-weight: 500; }
  .analysis-query.compact .store-grid { max-height: min(200px, 28vh); }
  .analysis-query-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(210px, 1fr)); gap: 16px; }
  .week-compare-toggle { display: flex; min-height: 48px; align-items: center; gap: 10px; cursor: pointer; }
  .week-compare-field input[type="checkbox"] {
    width: 18px;
    min-width: 18px;
    height: 18px;
    min-height: 18px;
    padding: 0;
    border: 0;
    border-radius: 4px;
    box-shadow: none;
    accent-color: var(--md-sys-color-primary);
    background: transparent;
  }
  .week-compare-field input[type="checkbox"]:hover,
  .week-compare-field input[type="checkbox"]:focus {
    padding: 0;
    border: 0;
    box-shadow: 0 0 0 3px var(--app-field-ring);
  }
  .store-selection { margin-top: 22px; }
  .selection-heading, .analysis-query-actions, .analysis-progress-heading, .analysis-progress-footer, .comparison-heading, .analysis-table-heading, .section-heading { display: flex; align-items: center; justify-content: space-between; gap: 14px; }
  .selection-heading > div, .facet-actions { display: flex; gap: 8px; }
  .selection-heading button, .facet-actions button, .analysis-search button { cursor: pointer; border: 0; color: var(--md-sys-color-primary); background: transparent; font-weight: 680; }
  .selection-heading button:disabled { cursor: default; opacity: .4; }
  .store-grid { display: grid; max-height: min(420px, 46vh); grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 9px; margin-top: 12px; overflow: auto; padding: 2px; }
  .store-grid label { display: grid; grid-template-columns: auto auto minmax(0, 1fr); align-items: center; gap: 10px; min-height: 50px; padding: 10px 12px; cursor: pointer; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 13px; background: var(--md-sys-color-surface-container-lowest); }
  .store-grid label.checked { border-color: var(--app-active-border); background: var(--md-sys-color-secondary-container); }
  .store-grid input, .facet-options input { width: 18px; height: 18px; accent-color: var(--md-sys-color-primary); }
  .store-id { font-weight: 760; font-variant-numeric: tabular-nums; }
  .store-name { overflow: hidden; color: var(--md-sys-color-on-surface-variant); text-overflow: ellipsis; white-space: nowrap; }
  .inline-loading, .store-empty { display: flex; min-height: 92px; align-items: center; justify-content: center; gap: 10px; color: var(--md-sys-color-on-surface-variant); }
  .analysis-query-actions { margin-top: 22px; padding-top: 18px; border-top: 1px solid var(--md-sys-color-outline-variant); }
  .analysis-query-actions > span { color: var(--md-sys-color-on-surface-variant); font-variant-numeric: tabular-nums; }
  .rank-seg { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; flex-shrink: 0; }
  .rank-seg > span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-weight: 650; }
  .rank-seg-track { display: flex; align-items: stretch; padding: 3px; border-radius: 9px; background: var(--md-sys-color-surface-container-low); }
  .rank-seg-track button { min-width: 34px; min-height: 30px; padding: 0 8px; border: 0; border-radius: 6px; color: var(--md-sys-color-on-surface-variant); background: transparent; font-size: 12px; font-weight: 650; cursor: pointer; }
  .rank-seg-track button.active { color: var(--md-sys-color-primary); background: var(--app-card); box-shadow: 0 1px 3px color-mix(in srgb, var(--md-sys-color-shadow) 12%, transparent); }
  .rank-custom { display: inline-flex; align-items: center; gap: 6px; padding: 0 8px; min-height: 36px; border: 1px solid var(--app-border); border-radius: 9px; color: var(--md-sys-color-on-surface-variant); background: var(--app-card); font-size: 12px; }
  .rank-custom.active, .rank-custom:focus-within { border-color: var(--md-sys-color-primary); color: var(--md-sys-color-primary); }
  .rank-custom input { width: 3.3rem; min-width: 0; min-height: 32px; padding: 0; border: 0; outline: 0; color: var(--md-sys-color-on-surface); background: transparent; text-align: center; font-weight: 700; font-variant-numeric: tabular-nums; }
  .export-ranking-limit { margin-top: 4px; }
  .analysis-progress { margin-top: 18px; padding: 24px; }
  .analysis-progress-heading { align-items: flex-start; margin-bottom: 16px; }
  .analysis-progress-heading h2 { margin: 0 0 5px; }
  .analysis-progress-heading > div > strong { color: var(--md-sys-color-on-surface-variant); }
  .analysis-progress-heading > span { color: var(--md-sys-color-primary); font-size: 28px; font-weight: 740; font-variant-numeric: tabular-nums; }
  .analysis-progress md-linear-progress { width: 100%; --md-linear-progress-track-height: 9px; --md-linear-progress-active-indicator-height: 9px; }
  .analysis-progress-footer { margin-top: 14px; }
  .analysis-progress-footer > strong { color: var(--md-sys-color-on-surface-variant); font-variant-numeric: tabular-nums; }
  .promoter-group-selector { display: grid; min-width: 0; min-height: 44px; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 10px; padding-left: 12px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 12px; background: var(--md-sys-color-surface-container-lowest); }
  .promoter-group-selector:focus-within { border-color: var(--md-sys-color-primary); box-shadow: 0 0 0 3px var(--app-field-ring); }
  .promoter-group-selector > span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; font-weight: 680; white-space: nowrap; }
  .promoter-group-selector select { width: 100%; min-width: 0; min-height: 42px; overflow: hidden; padding: 0 30px 0 0; border: 0; outline: 0; color: var(--md-sys-color-on-surface); background-color: transparent; font-weight: 680; text-overflow: ellipsis; white-space: nowrap; }
  .promoter-group-selector option { color: var(--md-sys-color-on-surface); background-color: var(--md-sys-color-surface-container-lowest); }
  .facet-row { display: flex; flex-wrap: wrap; gap: 8px; overflow: visible; }
  .facet-menu { position: relative; }
  .facet-menu > summary { display: flex; min-height: 44px; align-items: center; gap: 8px; padding: 8px 12px; cursor: pointer; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 12px; color: var(--md-sys-color-on-surface-variant); background: var(--md-sys-color-surface-container-lowest); list-style: none; }
  .facet-menu > summary::-webkit-details-marker { display: none; }
  .facet-menu > summary.active { border-color: var(--app-active-border); color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .facet-menu > summary strong { color: var(--md-sys-color-primary); font-size: 12px; }
  .facet-menu > summary .material-symbols-rounded { font-size: 19px; transition: transform 120ms ease; }
  .facet-menu[open] > summary .material-symbols-rounded { transform: rotate(180deg); }
  .facet-popover { position: absolute; z-index: 40; top: calc(100% + 7px); left: 0; width: min(330px, calc(100vw - 24px)); max-width: calc(100vw - 24px); padding: 10px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 14px; background: var(--md-sys-color-surface-container-lowest); box-shadow: var(--app-shadow-high); }
  .facet-menu:last-child .facet-popover,
  .facet-menu:nth-last-child(-n+2) .facet-popover { left: auto; right: 0; }
  .facet-actions { justify-content: flex-end; padding: 2px 4px 8px; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
  .facet-search { width: 100%; min-width: 0; min-height: 36px; margin-top: 8px; padding: 6px 10px; border: 1px solid var(--app-border); border-radius: 8px; color: var(--md-sys-color-on-surface); background: var(--app-card); }
  .facet-options { display: grid; max-height: min(260px, 45vh); gap: 3px; overflow: auto; padding-top: 7px; }
  .facet-options label { display: flex; align-items: center; gap: 10px; padding: 8px; cursor: pointer; border-radius: 9px; }
  .facet-options label:hover { background: var(--md-sys-color-surface-container-low); }
  .facet-options span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .facet-empty { padding: 16px 8px; color: var(--md-sys-color-on-surface-variant); font-size: 13px; text-align: center; }
  .analysis-search { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; min-height: 44px; padding: 0 11px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 12px; background: var(--md-sys-color-surface-container-lowest); }
  .analysis-search:focus-within { border-color: var(--md-sys-color-primary); box-shadow: 0 0 0 3px var(--app-field-ring); }
  .analysis-search > .material-symbols-rounded { color: var(--md-sys-color-outline); font-size: 20px; }
  .analysis-search input { width: 100%; min-width: 0; border: 0; outline: 0; color: var(--md-sys-color-on-surface); background: transparent; }
  .analysis-search button { display: grid; place-items: center; padding: 0; }
  .analysis-search button .material-symbols-rounded { font-size: 18px; }
  .report-tabs { display: flex; min-width: 0; gap: 4px; padding: 3px; overflow-x: auto; scrollbar-width: thin; }
  .report-tabs button { display: flex; min-height: 40px; align-items: center; gap: 7px; padding: 8px 12px; cursor: pointer; border: 0; border-radius: 8px; color: var(--md-sys-color-on-surface-variant); background: transparent; font-size: 13px; font-weight: 650; white-space: nowrap; }
  .report-tabs button:hover { background: var(--md-sys-color-surface-container-low); }
  .report-tabs button.active { color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); }
  .report-tabs .material-symbols-rounded { font-size: 20px; }
  .analysis-kpis { display: grid; grid-template-columns: 1.2fr repeat(3, minmax(0, 1fr)); gap: 12px; margin: 0; }
  .analysis-kpis > div { min-width: 0; padding: 18px; border: 1px solid var(--app-border); border-radius: 14px; background: var(--app-card); }
  .analysis-kpis > .kpi-primary { border-color: var(--app-active-border); background: linear-gradient(120deg, var(--md-sys-color-secondary-container), var(--app-card)); }
  .analysis-kpis dt { color: var(--md-sys-color-on-surface-variant); font-size: 13px; font-weight: 650; }
  .analysis-kpis dd { margin: 10px 0 8px; color: var(--app-summary-value); font-size: clamp(21px, 1.8vw, 29px); font-weight: 750; font-variant-numeric: tabular-nums; letter-spacing: -.03em; line-height: 1.2; overflow-wrap: anywhere; }
  .analysis-kpis span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
  .analysis-secondary-kpis { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 0; padding: 4px 2px 8px; }
  .analysis-secondary-kpis > div { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 10px; padding: 0 16px; min-width: 0; }
  .analysis-secondary-kpis > div + div { border-left: 1px solid var(--app-border); }
  .analysis-secondary-kpis dt, .analysis-secondary-kpis span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
  .analysis-secondary-kpis dd { margin: 0; font-size: 18px; color: var(--app-summary-value); font-weight: 700; font-variant-numeric: tabular-nums; }
  .positive { color: var(--app-proposed) !important; }
  .negative { color: var(--md-sys-color-error) !important; }
  .neutral { color: var(--md-sys-color-on-surface-variant) !important; }
  .performance-card, .comparison-card { overflow: hidden; padding: 0; }
  .section-heading, .comparison-heading, .analysis-table-heading { padding: 14px 16px 10px; }
  .section-heading h2, .comparison-heading h2, .analysis-table-heading h2 { margin: 0; font-size: 16px; font-weight: 700; }
  .analysis-workspace .surface-card { border-radius: 14px; box-shadow: none; }
  .performance-summary { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; padding: 16px; cursor: pointer; list-style: none; }
  .performance-summary::-webkit-details-marker { display: none; }
  .performance-summary h2 { margin: 0; font-size: 16px; }
  .performance-summary > span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
  .performance-summary > .material-symbols-rounded { margin-left: auto; font-size: 20px; }
  .performance-details[open] .performance-summary { border-bottom: 1px solid var(--app-border); }
  .performance-details[open] .performance-summary > .material-symbols-rounded { transform: rotate(180deg); }
  .performance-card .table-scroll { max-height: none; }
  .category-table { max-height: none; }
  .analysis-workspace .table-scroll { scrollbar-width: thin; }
  .analysis-workspace th, .analysis-workspace td { padding: 12px 14px; }
  .analysis-workspace tbody tr:hover { background: var(--md-sys-color-surface-container-low); }
  .performance-card th:first-child { text-align: left; }
  .emphasis { color: var(--app-summary-value); font-weight: 740; }
  .top-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  .top-card { overflow: hidden; padding: 0; }
  .top-card ol { display: grid; margin: 0; padding: 0 16px 10px; list-style: none; }
  .top-card li { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 56px; padding: 6px 0; border-top: 1px solid var(--app-table-border); }
  .top-card li:hover { background: var(--md-sys-color-surface-container-low); }
  .top-card .top-product strong { white-space: normal; overflow-wrap: anywhere; font-size: 13px; line-height: 1.5; }
  .top-card .podium .rank { color: var(--md-sys-color-on-primary); background: var(--md-sys-color-primary); }
  .top-card li:first-child { border-top: 0; }
  .top-card li > div { display: grid; min-width: 0; gap: 2px; }
  .top-card li strong, .top-card li span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .top-card li span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .top-card li b { font-variant-numeric: tabular-nums; }
  .top-card li > .top-metrics { min-width: max-content; justify-items: end; text-align: right; }
  .top-card .top-metrics span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-variant-numeric: tabular-nums; }
  .top-card .rank { display: grid; width: 24px; height: 24px; place-items: center; border-radius: 8px; color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); font-weight: 750; }
  .top-card .empty-row { display: grid; min-height: 120px; place-items: center; color: var(--md-sys-color-on-surface-variant); }
  .focus-section { overflow: hidden; padding: 0; }
  .focus-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; padding: 18px 20px 0; }
  .focus-heading h2 { margin: 0; }
  .focus-heading span { color: var(--md-sys-color-on-surface-variant); font-size: 13px; font-variant-numeric: tabular-nums; }
  .focus-note { margin: 8px 20px 0; color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
  .focus-grid { display: grid; gap: 14px; padding: 14px 16px 16px; }
  .focus-group { overflow: hidden; border: 1px solid var(--app-border); border-radius: 16px; background: var(--md-sys-color-surface-container-lowest); }
  .focus-group > header { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; padding: 12px 14px; background: var(--md-sys-color-surface-container-low); }
  .focus-group > header span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-variant-numeric: tabular-nums; }
  .focus-columns { display: grid; grid-template-columns: 1fr 1fr; gap: 0; }
  .focus-columns > div { min-width: 0; padding: 0 12px 10px; }
  .focus-columns > div + div { border-left: 1px solid var(--app-table-border); }
  .focus-columns h3 { margin: 12px 2px 4px; color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-weight: 700; }
  .focus-columns ol { display: grid; margin: 0; padding: 0; list-style: none; }
  .focus-columns li { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 8px; min-height: 48px; border-top: 1px solid var(--app-table-border); }
  .focus-columns li:first-child { border-top: 0; }
  .focus-columns li > div { display: grid; min-width: 0; gap: 2px; }
  .focus-columns li strong, .focus-columns li span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .focus-columns li span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .focus-columns .top-metrics { min-width: max-content; justify-items: end; text-align: right; }
  .focus-columns .top-metrics em { color: var(--md-sys-color-primary); font-size: 11px; font-style: normal; font-variant-numeric: tabular-nums; }
  .focus-columns .rank { display: grid; width: 24px; height: 24px; place-items: center; border-radius: 8px; color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); font-weight: 750; }
  .comparison-heading { align-items: flex-start; }
  .group-tabs { display: flex; flex-wrap: wrap; overflow: hidden; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 11px; }
  .group-tabs button { min-height: 38px; padding: 7px 10px; cursor: pointer; border: 0; border-right: 1px solid var(--md-sys-color-outline-variant); color: var(--md-sys-color-on-surface-variant); background: transparent; font-size: 12px; font-weight: 650; }
  .group-tabs button:last-child { border-right: 0; }
  .group-tabs button.active { color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .analysis-table-card { padding: 0; overflow: hidden; }
  .analysis-table-heading > strong { color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
  .ranking-section { overflow: visible; max-height: none; padding: 0; }
  .ranking-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 14px; border-bottom: 1px solid var(--app-table-border); }
  .ranking-heading > div:first-child { display: grid; gap: 3px; }
  .ranking-heading h2 { margin: 0; }
  .ranking-heading > div:first-child > span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
  .ranking-period-tabs { display: flex; flex-wrap: wrap; gap: 5px; }
  .ranking-period-tabs button { min-height: 36px; padding: 7px 11px; cursor: pointer; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 10px; color: var(--md-sys-color-on-surface-variant); background: transparent; font-weight: 650; }
  .ranking-period-tabs button.active { border-color: var(--app-active-border); color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .ranking-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(390px, 1fr)); gap: 12px; padding: 14px; }
  .ranking-group { min-width: 0; overflow: hidden; border: 1px solid var(--app-border); border-radius: 14px; background: var(--md-sys-color-surface-container-lowest); }
  .ranking-group > header { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 58px; padding: 11px 13px; border-bottom: 1px solid var(--app-table-border); background: var(--md-sys-color-surface-container-low); }
  .ranking-group > header > div { display: grid; min-width: 0; gap: 2px; }
  .ranking-group > header strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ranking-group > header span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .ranking-group > header b { color: var(--app-summary-value); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .ranking-group ol { display: grid; margin: 0; padding: 0 12px 8px; list-style: none; }
  .ranking-group li { display: grid; grid-template-columns: 24px minmax(0, 1fr) auto; align-items: center; gap: 8px; min-height: 52px; padding: 6px 0; border-top: 1px solid var(--app-table-border); }
  .ranking-group li:hover { background: var(--md-sys-color-surface-container-low); }
  .ranking-group .ranking-product strong { white-space: normal; overflow-wrap: anywhere; line-height: 1.5; }
  .ranking-group li:first-child { border-top: 0; }
  .ranking-group .rank { display: grid; width: 22px; height: 22px; place-items: center; border-radius: 7px; color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); font-size: 11px; font-weight: 750; }
  .ranking-product, .ranking-values { display: grid; min-width: 0; gap: 2px; }
  .ranking-product strong, .ranking-product span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ranking-product span, .ranking-values span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .ranking-values { min-width: max-content; justify-items: end; text-align: right; font-variant-numeric: tabular-nums; }
  .ranking-empty { min-height: 120px; display: grid; place-items: center; color: var(--md-sys-color-on-surface-variant); }
  .analysis-supplement { display: grid; grid-template-columns: minmax(0, 1fr) minmax(160px, 260px) auto; align-items: center; gap: 12px 16px; padding: 12px 14px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 16px; background: var(--md-sys-color-surface-container-low); }
  .analysis-supplement-copy { display: grid; gap: 2px; min-width: 0; }
  .analysis-supplement-copy strong { font-size: 14px; }
  .analysis-supplement-copy span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .analysis-supplement-meter { display: grid; grid-template-columns: minmax(120px, 1fr) auto; align-items: center; gap: 10px; min-width: 220px; }
  .analysis-supplement-meter md-linear-progress { width: 100%; --md-linear-progress-track-height: 8px; --md-linear-progress-active-indicator-height: 8px; }
  .analysis-supplement-meter strong { color: var(--md-sys-color-primary); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .tab-pending { margin-left: 6px; padding: 1px 6px; border-radius: 999px; color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); font-size: 10px; font-weight: 700; }
  :global(.app-dialog.export-dialog) {
    --export-gutter: var(--app-dialog-gutter, clamp(0.65rem, 2.2vw, 1.35rem));
    --export-gutter-block: var(--app-dialog-gutter-block, clamp(0.65rem, 3vh, 1.35rem));
    --export-pad: clamp(0.7rem, 1.1vw + 0.4rem, 1.15rem);
    --export-gap: clamp(0.4rem, 0.65vw + 0.22rem, 0.75rem);
    --export-section-pad: clamp(0.55rem, 0.8vw + 0.28rem, 0.85rem);
    --export-radius: clamp(0.65rem, 0.65vw + 0.4rem, 1rem);
    --export-title: clamp(1rem, 0.4vw + 0.88rem, 1.2rem);
    --export-heading: clamp(0.8125rem, 0.18vw + 0.76rem, 0.9375rem);
    --export-text: clamp(0.75rem, 0.16vw + 0.7rem, 0.875rem);
    --export-meta: clamp(0.6875rem, 0.1vw + 0.65rem, 0.75rem);
    --export-control: clamp(2rem, 1.4vw + 1.55rem, 2.5rem);
    --export-list: clamp(4.5rem, 13vh, 9rem);
    --export-list: clamp(4.5rem, 13dvh, 9rem);
    --export-list-lg: clamp(5.25rem, 17vh, 11.25rem);
    --export-list-lg: clamp(5.25rem, 17dvh, 11.25rem);
    --export-check: clamp(1rem, 0.15vw + 0.92rem, 1.125rem);
    inset: 0;
    display: flex;
    width: min(calc(100% - 2 * var(--export-gutter)), clamp(30rem, 54vw, 45rem));
    min-width: min(30rem, calc(100% - 2 * var(--export-gutter)));
    max-width: calc(100% - 2 * var(--export-gutter));
    height: fit-content;
    max-height: min(calc(100% - 2 * var(--export-gutter-block)), 82vh, 40rem);
    max-height: min(calc(100% - 2 * var(--export-gutter-block)), 82dvh, 40rem);
    margin: auto;
    padding: var(--export-pad);
    overflow: hidden;
    flex-direction: column;
    border-radius: clamp(1rem, 1.1vw + 0.65rem, 1.6rem);
    outline: none;
  }
  :global(.export-dialog:focus),
  :global(.export-dialog:focus-visible) { outline: none; }
  :global(.app-dialog.export-dialog h2) { font-size: var(--export-title); line-height: 1.2; }
  :global(.export-dialog .dialog-header) {
    flex: 0 0 auto;
    align-items: center;
    gap: var(--export-gap);
    margin-bottom: var(--export-gap);
  }
  .export-dialog-body {
    display: flex;
    min-height: 0;
    flex: 1 1 auto;
    flex-direction: column;
    gap: var(--export-gap);
    overflow: hidden;
  }
  .export-dialog-scroll { min-height: 0; flex: 1 1 auto; overflow: auto; overscroll-behavior: contain; outline: none; }
  .export-dialog-scroll:focus,
  .export-dialog-scroll:focus-visible { outline: none; }
  .export-dialog-grid { display: grid; min-width: 0; gap: var(--export-gap); }
  .export-section {
    display: grid;
    min-width: 0;
    align-content: start;
    gap: var(--export-gap);
    padding: var(--export-section-pad);
    border: 1px solid var(--app-border);
    border-radius: var(--export-radius);
    background: var(--md-sys-color-surface-container-low);
    color: var(--md-sys-color-on-surface);
  }
  .export-section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--export-gap); }
  .export-section-heading h3 { margin: 0; color: var(--app-heading); font-size: var(--export-heading); font-weight: 680; }
  .export-section-heading p { margin: 0.2em 0 0; color: var(--md-sys-color-on-surface-variant); font-size: var(--export-meta); line-height: 1.45; }
  .export-section-heading span { color: var(--md-sys-color-on-surface-variant); font-size: var(--export-meta); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .export-mode { display: grid; grid-template-columns: 1fr 1fr; gap: var(--export-gap); }
  .export-mode button {
    min-height: var(--export-control);
    padding: 0.4em 0.65em;
    cursor: pointer;
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: calc(var(--export-radius) - 2px);
    color: var(--md-sys-color-on-surface-variant);
    background: var(--md-sys-color-surface-container-lowest);
    font-size: var(--export-text);
    font-weight: 650;
    text-align: center;
  }
  .export-mode button:hover:not(:disabled) { border-color: var(--app-field-hover); }
  .export-mode button:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 2px; }
  .export-mode button.active { border-color: var(--app-active-border); color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .export-flags { display: grid; gap: var(--export-gap); }
  .export-check { display: flex; align-items: flex-start; gap: 0.65em; min-width: 0; color: var(--md-sys-color-on-surface); font-size: var(--export-text); line-height: 1.4; }
  .export-check-card { padding: 0.65em 0.75em; border: 1px solid var(--md-sys-color-outline-variant); border-radius: calc(var(--export-radius) - 2px); background: var(--md-sys-color-surface-container-lowest); }
  .export-all-copy { display: grid; gap: 0.25em; padding: 0.7em 0.8em; border: 1px solid var(--md-sys-color-outline-variant); border-radius: calc(var(--export-radius) - 2px); background: var(--md-sys-color-surface-container-lowest); }
  .export-all-copy strong { font-size: var(--export-text); }
  .export-all-copy p { margin: 0; color: var(--md-sys-color-on-surface-variant); font-size: var(--export-meta); line-height: 1.45; }
  .export-advanced { border: 1px solid var(--md-sys-color-outline-variant); border-radius: calc(var(--export-radius) - 2px); background: var(--md-sys-color-surface-container-lowest); }
  .export-advanced > summary { cursor: pointer; padding: 0.7em 0.8em; color: var(--md-sys-color-on-surface-variant); font-size: var(--export-text); font-weight: 650; list-style: none; }
  .export-advanced > summary::-webkit-details-marker { display: none; }
  .export-advanced[open] > summary { border-bottom: 1px solid var(--md-sys-color-outline-variant); }
  .export-advanced > :not(summary) { margin: 0.65em 0.75em 0.75em; }
  .export-advanced .export-choice-panel { margin-top: 0.65em; }
  .export-screen-filters { display: grid; gap: 0.55em; padding: 0.65em 0.75em; border: 1px solid var(--md-sys-color-outline-variant); border-radius: calc(var(--export-radius) - 2px); background: var(--md-sys-color-surface-container-lowest); }
  .export-screen-filter { display: grid; gap: 0.25em; min-width: 0; }
  .export-screen-filter strong { color: var(--md-sys-color-on-surface); font-size: var(--export-text); }
  .export-screen-filter ul { display: grid; gap: 0.15em; margin: 0; padding: 0; list-style: none; color: var(--md-sys-color-on-surface-variant); font-size: var(--export-meta); }
  .export-screen-filter li { overflow: hidden; text-overflow: ellipsis; }
  .export-check b { display: block; font-weight: 700; }
  .export-check small { display: block; color: var(--md-sys-color-on-surface-variant); font-size: var(--export-meta); }
  .export-check input { flex: 0 0 auto; width: var(--export-check); height: var(--export-check); margin-top: 1px; accent-color: var(--md-sys-color-primary); }
  .export-check input:focus { outline: none; }
  .export-check input:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 2px; }
  .export-choice-panel {
    display: grid;
    min-width: 0;
    min-height: 0;
    grid-template-rows: auto minmax(0, 1fr);
    overflow: hidden;
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: calc(var(--export-radius) - 1px);
    background: var(--md-sys-color-surface-container-lowest);
  }
  .export-choice-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--export-gap); padding: 0.55em 0.7em; border-bottom: 1px solid var(--md-sys-color-outline-variant); background: var(--md-sys-color-surface-container); }
  .export-choice-heading strong { min-width: 0; color: var(--md-sys-color-on-surface); font-size: var(--export-text); font-weight: 680; }
  .export-choice-heading > div { display: flex; flex: 0 0 auto; gap: 0.5em; }
  .export-choice-heading button { cursor: pointer; border: 0; color: var(--md-sys-color-primary); background: transparent; font-size: var(--export-text); font-weight: 680; }
  .export-choice-heading button:disabled { cursor: default; opacity: .4; }
  .export-choice-heading button:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 2px; border-radius: 6px; }
  .export-choice-list { display: grid; min-height: 0; max-height: var(--export-list); align-content: start; gap: 2px; overflow: auto; overscroll-behavior: auto; padding: 0.35em; outline: none; }
  .export-choice-list-categories { max-height: var(--export-list-lg); }
  .export-choice-list:focus,
  .export-choice-list:focus-visible { outline: none; }
  .export-choice-list label { display: flex; min-width: 0; align-items: center; gap: 0.65em; padding: 0.45em 0.5em; border-radius: 0.5em; color: var(--md-sys-color-on-surface); font-size: var(--export-text); }
  .export-choice-list label:hover { background: var(--md-sys-color-surface-container-low); }
  .export-choice-list label:focus-within { background: var(--md-sys-color-surface-container); }
  .export-choice-list input { flex: 0 0 auto; width: var(--export-check); height: var(--export-check); accent-color: var(--md-sys-color-primary); }
  .export-choice-list input:focus { outline: none; }
  .export-choice-list input:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 2px; }
  .export-choice-list span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .export-choice-empty { padding: 1em 0.5em; color: var(--md-sys-color-on-surface-variant); font-size: var(--export-text); text-align: center; }
  .export-dialog-warning { flex: 0 0 auto; margin: 0; color: var(--md-sys-color-error); font-size: var(--export-meta); }
  .export-dialog-actions {
    display: flex;
    flex: 0 0 auto;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--export-gap);
    margin-top: 0;
    min-width: 0;
  }
  .export-file-count {
    flex: 1 1 auto;
    min-width: 4.5rem;
    margin-right: auto;
    color: var(--md-sys-color-on-surface-variant);
    font-size: var(--export-text);
    font-variant-numeric: tabular-nums;
  }

  @media (max-width: 900px) {
    .export-mode { grid-template-columns: 1fr; }
    .export-choice-heading { align-items: flex-start; flex-wrap: wrap; }
    .export-section-heading { flex-wrap: wrap; }
  }

  @media (max-height: 768px) {
    :global(.app-dialog.export-dialog) {
      --export-pad: clamp(0.6rem, 1.5vh + 0.28rem, 0.9rem);
      --export-gap: clamp(0.35rem, 1.1vh + 0.15rem, 0.6rem);
      --export-section-pad: clamp(0.5rem, 1.2vh + 0.22rem, 0.7rem);
      --export-list: clamp(3.75rem, 12vh, 7.25rem);
      --export-list: clamp(3.75rem, 12dvh, 7.25rem);
      --export-list-lg: clamp(4.25rem, 15vh, 8.75rem);
      --export-list-lg: clamp(4.25rem, 15dvh, 8.75rem);
      --export-control: clamp(2rem, 5.2vh, 2.35rem);
    }
  }

  @media (max-width: 720px) {
    .analysis-supplement { grid-template-columns: 1fr; }
    .analysis-supplement-meter { min-width: 0; }
  }

  @media (max-width: 1200px) {
    .report-tabs button { padding-inline: 10px; }
    .report-tabs .material-symbols-rounded { display: none; }
    .analysis-kpis > div { padding: 14px; }
    .analysis-kpis dd { font-size: clamp(18px, 1.9vw, 24px); }
  }

  @media (max-width: 980px) {
    .report-navigation { flex-wrap: wrap; }
    .report-tabs { flex: 1 1 100%; }
    .analysis-kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .filter-fields .promoter-group-selector { max-width: none; }
    .filter-fields .facet-row { position: relative; }
    .facet-menu { position: static; }
    .facet-popover, .facet-menu:nth-last-child(-n+2) .facet-popover { left: 0; right: auto; max-width: 100%; }
    .top-grid { grid-template-columns: 1fr; }
    .focus-columns { grid-template-columns: 1fr; }
    .focus-columns > div + div { border-top: 1px solid var(--app-table-border); border-left: 0; }
    .comparison-heading { flex-direction: column; }
    .ranking-heading { align-items: flex-start; flex-direction: column; }
  }

  @media (max-width: 620px) {
    .analysis-heading-actions { width: 100%; justify-content: flex-start; }
    .analysis-query, .analysis-query.compact { padding: 16px; }
    .analysis-query-grid { grid-template-columns: minmax(0, 1fr); }
    .report-navigation { gap: 4px; padding: 6px; }
    .report-navigation .rank-seg { padding: 4px; }
    .report-tabs button { min-height: 44px; }
    .scope-count { flex: 1 1 auto; }
    .filter-tools { flex: 1 1 100%; margin-left: 0; justify-content: space-between; }
    .filter-bar .analysis-search { max-width: none; flex-basis: 160px; }
    .filter-panel { padding: 12px; }
    .analysis-kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
    .analysis-kpis > div { padding: 12px; }
    .analysis-kpis > .kpi-primary, .analysis-kpis > div:nth-child(2) { grid-column: 1 / -1; }
    .analysis-kpis dd { font-size: 22px; }
    .analysis-secondary-kpis { grid-template-columns: 1fr; gap: 8px; }
    .analysis-secondary-kpis > div { padding: 4px 12px; }
    .analysis-secondary-kpis > div + div { border-left: 0; }
    .selection-heading { flex-wrap: wrap; }
    .rank-seg { gap: 6px; }
    .top-card li { grid-template-columns: 24px minmax(0, 1fr); }
    .top-card li > .top-metrics { grid-column: 2; grid-template-columns: auto auto; gap: 10px; justify-content: space-between; min-width: 0; }
    .ranking-group li { grid-template-columns: 24px minmax(0, 1fr); }
    .ranking-values { grid-column: 2; grid-template-columns: auto auto; justify-content: space-between; min-width: 0; }
    .store-grid { grid-template-columns: 1fr; }
    .analysis-query-actions, .analysis-progress-footer { align-items: stretch; flex-direction: column; }
    .group-tabs { width: 100%; }
    .group-tabs button { flex: 1; }
    .report-tabs { width: 100%; }
    .report-tabs button { flex: 1; justify-content: center; padding-inline: 10px; }
    .report-tabs button .material-symbols-rounded { display: none; }
    .ranking-grid { grid-template-columns: 1fr; padding: 10px; }
    .ranking-period-tabs { width: 100%; }
    .ranking-period-tabs button { flex: 1; }
  }
</style>
