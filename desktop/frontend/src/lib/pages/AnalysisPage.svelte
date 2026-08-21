<script lang="ts">
  import { onMount } from 'svelte';
  import { backend } from '../backend';
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
  import {
    defaultSalesReportFilter,
    includeInSalesReport,
    reportCategoryId,
    type SalesReportFacets,
    type SalesReportFilter,
  } from '../salesReportItems';
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
  export let onGoToAccounts: () => void = () => undefined;

  type CategoryKey = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';
  type FacetSelections = Record<CategoryKey, Set<string>>;
  type ReportView = 'overview' | 'weekly' | 'focus' | 'categories' | 'products' | 'stores';
  type PeriodMode = 'month' | 'range';
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
  let storeLoadGeneration = 0;

  $: if (!loadingProfiles && profileId && loadedSimulateCount !== settings.simulateStoreCount) {
    loadedSimulateCount = settings.simulateStoreCount;
    void loadStores();
  }
  $: busy = loadingProfiles || loadingStores || running || Boolean(result?.pending);
  $: visibleStores = filterStores(stores, storeQuery);
  $: onBusyChange(busy);
  $: rangeInvalid = periodMode === 'range' && Boolean(from && to && from > to);
  $: reportPeriods = normalizePeriods(result);
  $: currentPeriod = periodByKey(reportPeriods, 'current') ?? reportPeriods[0];
  $: neededPeriodKeys = periodKeysForAnalysisView(
    activeView,
    [salesRankingKey, quantityRankingKey],
    groupScopeActive || filtersActive(selections, search),
  );
  $: if (result && !exportingPDF) void ensurePeriodItems(neededPeriodKeys);
  $: selectedGroup = manCodeGroups.find((group) => group.id === selectedGroupId);
  $: selectedGroupCodes = new Set((selectedGroup?.codes ?? []).map((code) => code.trim()).filter(Boolean));
  $: groupScopeActive = Boolean(selectedGroup);
  $: filteredItems = (currentPeriod?.items ?? []).filter((item) => {
    if (!matchesFilters(item, selections, search)) return false;
    return !groupScopeActive || selectedGroupCodes.has(item.articleCode.trim());
  });
  $: productScopeActive = groupScopeActive || filtersActive(selections, search);
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
  $: topSales = productScopeActive || (currentPeriod?.items?.length ?? 0) > 0
    ? buildTopItems(filteredItems, 'amount').slice(0, 15)
    : rankedToTopItems(currentPeriod?.topAmount);
  $: topQuantity = productScopeActive || (currentPeriod?.items?.length ?? 0) > 0
    ? buildTopItems(filteredItems, 'quantity').slice(0, 15)
    : rankedToTopItems(currentPeriod?.topQuantity);
  $: salesRankingPeriods = reportPeriods.filter((period) => ['current', 'yearAgo', 'yearAgoNext'].includes(period.key));
  $: quantityRankingPeriods = reportPeriods.filter((period) => ['current', 'previous', 'previous2'].includes(period.key));
  $: salesRankingPeriod = periodByKey(salesRankingPeriods, salesRankingKey) ?? salesRankingPeriods[0];
  $: quantityRankingPeriod = periodByKey(quantityRankingPeriods, quantityRankingKey) ?? quantityRankingPeriods[0];
  $: salesRankingGroups = buildCategoryRankings(salesRankingPeriod, groupLevel, 'amount', selections, search, selectedGroupCodes, groupScopeActive);
  $: quantityRankingGroups = buildCategoryRankings(quantityRankingPeriod, groupLevel, 'quantity', selections, search, selectedGroupCodes, groupScopeActive);
  $: storeRows = buildStoreRows(reportPeriods, selections, search, selectedGroupCodes, groupScopeActive);
  $: focusPeriod = periodByKey(reportPeriods, 'yearAgoNext');
  $: focusGroups = focusGroupsForScope(
    focusPeriod, currentPeriod, selections, search, selectedGroup, manCodeGroups, selectedGroupCodes, groupScopeActive,
  );
  $: pageCount = Math.max(1, Math.ceil(filteredItems.length / pageSize));
  $: if (page > pageCount) page = pageCount;
  $: pageRows = filteredItems.slice((page - 1) * pageSize, page * pageSize);
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
    void initialize();
    return () => {
      unsubscribe();
      unsubscribeUpdate();
      document.removeEventListener('pointerdown', closeFacetMenus);
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
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      loadingProfiles = false;
    }
  }

  async function loadStores(options: { keepResult?: boolean } = {}) {
    if (!profileId) return;
    const generation = ++storeLoadGeneration;
    loadingStores = true;
    error = '';
    if (!options.keepResult) void discardResult();
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

  async function changeQuery() {
    dismissExportNotice();
    resetFilters();
    await discardResult();
    if (profileId) await loadStores({ keepResult: true });
  }

  function toggleStore(storeId: string) {
    const next = new Set(selectedStoreIds);
    if (next.has(storeId)) next.delete(storeId);
    else next.add(storeId);
    selectedStoreIds = next;
    void discardResult();
  }

  function selectAllStores() {
    selectedStoreIds = new Set(stores.map((store) => store.businessId));
    void discardResult();
  }

  function clearStores() {
    selectedStoreIds = new Set<string>();
    void discardResult();
  }

  async function discardResult() {
    const operationId = result?.operationId;
    result = undefined;
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
    if (periods.length === 0) return;
    if (operationId) {
      try { await backend.cancelSalesAnalysis(operationId); } catch { /* previous run may already be finished */ }
    }
    running = true;
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
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      running = false;
      cancelling = false;
    }
  }

  async function ensurePeriodItems(keys: string[], options: { retain?: boolean } = {}): Promise<SalesAnalysisPeriodResult[]> {
    const summary = result;
    if (!summary?.periods?.length) return summary?.periods ?? [];
    const generation = ++hydrateGeneration;
    const operationId = summary.operationId;
    const keep = new Set(keys.filter((key) => summary.periods!.some((period) => period.key === key)));
    let periods = summary.periods;
    let changed = false;
    const stillCurrent = () => generation === hydrateGeneration && result?.operationId === operationId;
    if (!options.retain) {
      const trimmed = periods.map((period) => {
        if (keep.has(period.key) || period.key === 'current' || !period.items?.length) return period;
        changed = true;
        return { ...period, items: undefined, itemCount: period.itemCount || period.items.length };
      });
      if (changed) periods = trimmed;
    }
    for (const key of keep) {
      const period = periods.find((candidate) => candidate.key === key);
      if (!period || !periodNeedsItemHydration(period)) continue;
      if (!operationId) continue;
      loadingItems = true;
      try {
        const packed = await backend.getSalesAnalysisItems({ operationId, periodKey: key });
        if (!stillCurrent()) return periods;
        const storeList = (period.stores?.length ? period.stores : summary.stores) ?? [];
        const items = unpackSalesAnalysisItems(packed, storeList);
        if (items.length === 0 && (period.itemCount ?? 0) > 0) continue;
        periods = periods.map((candidate) => candidate.key === key ? { ...candidate, items, itemCount: period.itemCount || items.length } : candidate);
        changed = true;
      } catch (caught) {
        if (!stillCurrent()) return periods;
        if (options.retain) throw caught;
        error = errorMessage(settings.locale, caught);
      }
    }
    if (changed && stillCurrent() && result) result = keepHydratedItems(result, { ...result, periods });
    if (stillCurrent()) loadingItems = false;
    return periods;
  }

  async function cancelAnalysis() {
    const id = operationId || result?.operationId;
    if (!id || cancelling) return;
    cancelling = true;
    try {
      await backend.cancelSalesAnalysis(id);
      if (result?.operationId === id) {
        await discardResult();
        progress = undefined;
        operationId = '';
        activeView = 'overview';
      }
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      cancelling = false;
    }
  }

  function openExportDialog() {
    if (!result || result.pending || exportingPDF || loadingItems) return;
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
    if (!result || exportingPDF || loadingItems) return;
    if (exportFilter.mode === 'whitelist' && exportFilter.categories.length === 0) return;
    if (exportFilesCount === 0) return;
    exportingPDF = true;
    pdfExportCurrent = 0;
    pdfExportTotal = 0;
    error = '';
    exportNotice = '';
    try {
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
      exportingPDF = false;
      exportDialog = false;
      if (result) void ensurePeriodItems(periodKeysForView(activeView, [salesRankingKey, quantityRankingKey]));
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
        if ((period.items?.length ?? 0) > 0 || !prev?.items?.length) return period;
        if (period.key !== 'current' && period.itemCount && period.itemCount > 0 && !period.items) {
          return period;
        }
        return { ...period, items: prev.items, itemCount: period.itemCount || prev.items.length };
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
    ].some((value) => value.toLocaleLowerCase().includes(term));
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

  function clearFacet(key: CategoryKey) {
    selections = { ...selections, [key]: new Set<string>() };
    page = 1;
  }

  function selectFacetAll(key: CategoryKey) {
    selections = { ...selections, [key]: new Set(facetOptionMap[key]) };
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
        items: buildTopItems(group.items, sortBy).slice(0, 15),
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

<section class="page analysis-page" class:has-results={Boolean(result && currentPeriod)} aria-labelledby="analysis-title">
  <div class="page-heading split-heading">
    <h1 id="analysis-title">{t('analysis.title')}</h1>
    {#if result}
      <div class="analysis-heading-actions">
        <md-filled-button type="button" onclick={openExportDialog} disabled={exportingPDF || loadingItems || result.pending}>
          <span class="material-symbols-rounded" slot="icon" aria-hidden="true">picture_as_pdf</span>{exportingPDF ? t('analysis.exportingPDFProgress', { current: pdfExportCurrent, total: pdfExportTotal }) : t('analysis.exportPDF')}
        </md-filled-button>
        <md-outlined-button type="button" onclick={() => void changeQuery()} disabled={exportingPDF}>
          <span class="material-symbols-rounded" slot="icon" aria-hidden="true">tune</span>{t('analysis.changeQuery')}
        </md-outlined-button>
      </div>
    {/if}
  </div>

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
  {#if loadingItems}
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
  {:else if !result}
    <form class="analysis-query surface-card" onsubmit={(event) => { event.preventDefault(); void runAnalysis(); }}>
      <div class="analysis-query-grid">
        <div class="field-group">
          <label for="analysis-profile">{t('analysis.account')}</label>
          <select id="analysis-profile" bind:value={profileId} onchange={() => void loadStores()} disabled={loadingStores || running}>
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
          <strong id="analysis-stores-label">{t('analysis.stores')}</strong>
          <div><button type="button" onclick={selectAllStores} disabled={loadingStores || stores.length === 0}>{t('analysis.selectAll')}</button><button type="button" onclick={clearStores} disabled={loadingStores || selectedStoreIds.size === 0}>{t('analysis.clear')}</button></div>
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
          <div class="store-grid pane-scroll">
            {#each visibleStores as store (store.businessId)}
              <label class:checked={selectedStoreIds.has(store.businessId)}><input type="checkbox" checked={selectedStoreIds.has(store.businessId)} onchange={() => toggleStore(store.businessId)} /><span class="store-id">{store.businessId}</span><span class="store-name">{store.label}</span></label>
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
        <md-filled-button type="button" onclick={() => void runAnalysis()} disabled={loadingStores || running || selectedStoreIds.size === 0 || rangeInvalid || (periodMode === 'month' ? !month : !from || !to)}><span class="material-symbols-rounded" slot="icon" aria-hidden="true">query_stats</span>{t('analysis.run')}</md-filled-button>
      </div>
    </form>
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
      <div class="analysis-toolbar">
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

      <div class="period-summary" aria-label={t('analysis.periods')}>
        {#each reportPeriods as period}<span class:current={period.key === 'current'}><strong>{period.label}</strong> {period.from} — {period.to}</span>{/each}
      </div>

      <div class="analysis-filters surface-card">
        <div class="facet-row" aria-label={t('analysis.categoryFilters')}>
          {#each facets as facet}
            <details class="facet-menu" open={openFacet === facet.key}>
              <summary class:active={selections[facet.key].size > 0} onclick={(event) => { event.preventDefault(); toggleFacetMenu(facet.key); }}><span>{t(facet.label)}</span><strong>{selections[facet.key].size > 0 ? selections[facet.key].size : t('analysis.all')}</strong><span class="material-symbols-rounded" aria-hidden="true">expand_more</span></summary>
              <div class="facet-popover">
                <div class="facet-actions"><button type="button" onclick={() => selectFacetAll(facet.key)}>{t('analysis.selectAll')}</button><button type="button" onclick={() => clearFacet(facet.key)}>{t('analysis.clear')}</button></div>
                <div class="facet-options pane-scroll">
                  {#each facetOptionMap[facet.key] as option}
                    <label><input aria-label={option} type="checkbox" checked={selections[facet.key].has(option)} onchange={() => toggleFacet(facet.key, option)} /><span>{option}</span></label>
                  {:else}
                    <div class="facet-empty">{loadingItems ? t('analysis.loadingItems') : t('analysis.noResults')}</div>
                  {/each}
                </div>
              </div>
            </details>
          {/each}
        </div>
        <div class="analysis-filter-tools">
          <div class="analysis-search"><span class="material-symbols-rounded" aria-hidden="true">search</span><input aria-label={t('analysis.search')} placeholder={t('analysis.search')} value={search} oninput={changeSearch} />{#if search}<button type="button" aria-label={t('analysis.clear')} onclick={() => { search = ''; page = 1; }}><span class="material-symbols-rounded" aria-hidden="true">close</span></button>{/if}</div>
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

      <div class="report-tabs" role="tablist" aria-label={t('analysis.reportViews')}>
        {#each [
          { key: 'overview', label: t('analysis.overview'), icon: 'space_dashboard' },
          { key: 'weekly', label: t('analysis.weekly'), icon: 'calendar_view_week' },
          { key: 'focus', label: t('analysis.focus'), icon: 'upcoming' },
          { key: 'categories', label: t('analysis.categories'), icon: 'account_tree' },
          { key: 'products', label: t('analysis.products'), icon: 'inventory_2' },
          { key: 'stores', label: t('analysis.stores'), icon: 'storefront' },
        ].filter((tab) => !groupScopeActive || tab.key !== 'weekly') as tab}
          <button type="button" class:active={activeView === tab.key} role="tab" aria-selected={activeView === tab.key} onclick={() => { activeView = tab.key as ReportView; }}><span class="material-symbols-rounded" aria-hidden="true">{tab.icon}</span>{tab.label}{#if result.pending && ((tab.key === 'weekly' && !result.weeks?.length) || (tab.key === 'focus' && !periodByKey(reportPeriods, 'yearAgoNext')) || (tab.key === 'stores' && reportPeriods.length < 2))}<span class="tab-pending">{t('common.loading')}</span>{/if}</button>
        {/each}
      </div>
      </div>

      <div class="analysis-workspace">
      {#if activeView === 'overview'}
        <dl class="analysis-kpis">
          <div><dt>{t('analysis.netSales')}</dt><dd>{formatMoney(currentTotals.netSalesAmount)}</dd><span class={deltaClass(delta(currentTotals.netSalesAmount, yearAgoTotals.netSalesAmount))}>{formatPercent(delta(currentTotals.netSalesAmount, yearAgoTotals.netSalesAmount))} {t('analysis.vsYearAgo')}</span></div>
          <div><dt>{t('analysis.grossSales')}</dt><dd>{formatMoney(currentTotals.saleAmount)}</dd><span class={deltaClass(delta(currentTotals.saleAmount, previousTotals.saleAmount))}>{formatPercent(delta(currentTotals.saleAmount, previousTotals.saleAmount))} {t('analysis.vsPrevious')}</span></div>
          <div><dt>{t('analysis.returns')}</dt><dd>{formatMoney(currentTotals.returnAmount)}</dd><span>{formatNumber(currentTotals.returnQuantity)} {t('analysis.units')}</span></div>
          <div><dt>{t('analysis.netQuantity')}</dt><dd>{formatNumber(currentTotals.netQuantity)}</dd><span class={deltaClass(delta(currentTotals.netQuantity, yearAgoTotals.netQuantity))}>{formatPercent(delta(currentTotals.netQuantity, yearAgoTotals.netQuantity))} {t('analysis.vsYearAgo')}</span></div>
          <div><dt>{t('analysis.transactions')}</dt><dd>{formatValue(currentTotals.transactionCount, 'number')}</dd><span>{groupScopeActive || filtersActive(selections, search) ? t('analysis.wholeStoreOnly') : t('analysis.orders')}</span></div>
          <div><dt>{t('analysis.basket')}</dt><dd>{formatValue(currentTotals.basketValue, 'money')}</dd><span>{groupScopeActive || filtersActive(selections, search) ? t('analysis.wholeStoreOnly') : t('analysis.perOrder')}</span></div>
          <div><dt>{t('analysis.skus')}</dt><dd>{formatNumber(currentTotals.skuCount)}</dd><span>{t('analysis.products')}</span></div>
        </dl>

        <section class="performance-card surface-card" aria-labelledby="performance-title">
          <div class="section-heading"><h2 id="performance-title">{t('analysis.performance')}</h2></div>
          <div class="table-scroll"><table>
            <thead><tr><th>{t('analysis.metric')}</th><th class="numeric">{t('analysis.currentPeriod')}</th><th class="numeric">{t('analysis.previousPeriod')}</th><th class="numeric">{t('analysis.yearAgoPeriod')}</th><th class="numeric">{t('analysis.vsPrevious')}</th><th class="numeric">{t('analysis.vsYearAgo')}</th></tr></thead>
            <tbody>{#each performanceRows as row}<tr><th>{row.label}</th><td class="numeric emphasis">{formatValue(row.current, row.format)}</td><td class="numeric">{formatValue(row.previous, row.format)}</td><td class="numeric">{formatValue(row.yearAgo, row.format)}</td><td class={`numeric ${deltaClass(delta(row.current, row.previous))}`}>{formatPercent(delta(row.current, row.previous))}</td><td class={`numeric ${deltaClass(delta(row.current, row.yearAgo))}`}>{formatPercent(delta(row.current, row.yearAgo))}</td></tr>{/each}</tbody>
          </table></div>
        </section>

        <div class="top-grid">
          <section class="top-card surface-card" aria-labelledby="top-sales-title">
            <div class="section-heading"><h2 id="top-sales-title">{t('analysis.topSales')}</h2></div>
            <ol>{#each topSales as item, index}<li><span class="rank">{index + 1}</span><div><strong>{item.name}</strong><span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="top-metrics"><b>{formatMoney(item.amount)}</b><span>{formatNumber(item.quantity)} {t('analysis.units')}</span></div></li>{:else}<li class="empty-row">{t('analysis.noResults')}</li>{/each}</ol>
          </section>
          <section class="top-card surface-card" aria-labelledby="top-quantity-title">
            <div class="section-heading"><h2 id="top-quantity-title">{t('analysis.topQuantity')}</h2></div>
            <ol>{#each topQuantity as item, index}<li><span class="rank">{index + 1}</span><div><strong>{item.name}</strong><span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="top-metrics"><b>{formatNumber(item.quantity)} {t('analysis.units')}</b><span>{formatMoney(item.amount)}</span></div></li>{:else}<li class="empty-row">{t('analysis.noResults')}</li>{/each}</ol>
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
            <div class="table-scroll store-table">
              <table>
                <thead>
                  <tr>
                    <th>{t('analysis.store')}</th>
                    <th class="numeric">{t(weeklyUsesAlignedComparison ? 'analysis.currentPeriod' : 'analysis.thisWeek')}</th>
                    <th class="numeric">{t(weeklyUsesAlignedComparison ? 'analysis.previousPeriod' : 'analysis.lastWeek')}</th>
                    <th class="numeric">{t('analysis.variance')}</th>
                    <th class="numeric">{t('analysis.variancePercent')}</th>
                    <th class="numeric">{t('analysis.weekday')}</th>
                    <th class="numeric">{t('analysis.weekend')}</th>
                    <th class="numeric">{t('analysis.customers')}</th>
                  </tr>
                </thead>
                <tbody>
                  {#each weeklySegmentRows(weeklyWeek.stores ?? [], {
                    store: (store) => store.businessId || store.label || t('analysis.store'),
                    localTotal: t('analysis.localTotal'),
                    touristTotal: t('analysis.touristTotal'),
                    allStores: t('analysis.allStores'),
                  }) as row}
                    <tr class:weekly-total={row.kind !== 'store'}>
                      <th>
                        {#if row.kind === 'store'}
                          <strong>{row.values.businessId || row.label}</strong>
                          {#if row.values.label && row.values.label !== row.values.businessId}<span>{row.values.label}</span>{/if}
                        {:else}
                          <strong>{row.label}</strong>
                        {/if}
                      </th>
                      {#each weeklyMetricCells(row.values) as cell}
                        <td class={cell.className}>{cell.text}</td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
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
              <div class="ranking-empty">{t('common.loading')}</div>
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
                            <div><strong>{item.name || item.code}</strong><span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div>
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
                            <div><strong>{item.name || item.code}</strong><span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div>
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
          <div class="category-table"><table>
            <thead><tr><th>{t('analysis.category')}</th><th class="numeric">{t('analysis.currentPeriod')}</th><th class="numeric">{t('analysis.previousPeriod')}</th><th class="numeric">{t('analysis.previous2Period')}</th><th class="numeric">{t('analysis.yearAgoPeriod')}</th><th class="numeric">{t('analysis.vsPrevious')}</th><th class="numeric">{t('analysis.vsYearAgo')}</th></tr></thead>
            <tbody>{#each categoryRows as row}<tr><th><strong>{row.name}</strong>{#if row.code}<span>{row.code}</span>{/if}</th><td class="numeric emphasis">{formatMoney(row.current)}</td><td class="numeric">{formatPeriodMoney(row.previous, 'previous')}</td><td class="numeric">{formatPeriodMoney(row.previous2, 'previous2')}</td><td class="numeric">{formatPeriodMoney(row.yearAgo, 'yearAgo')}</td><td class={`numeric ${deltaClass(delta(row.current, row.previous))}`}>{formatPercent(delta(row.current, row.previous))}</td><td class={`numeric ${deltaClass(delta(row.current, row.yearAgo))}`}>{formatPercent(delta(row.current, row.yearAgo))}</td></tr>{:else}<tr><td colspan="7" class="empty-table">{t('analysis.noResults')}</td></tr>{/each}</tbody>
          </table></div>
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
                <ol>{#each group.items as item, index}<li><span class="rank">{index + 1}</span><div class="ranking-product"><strong>{item.name}</strong><span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="ranking-values"><b>{formatMoney(item.amount)}</b><span>{formatNumber(item.quantity)} {t('analysis.units')}</span></div></li>{/each}</ol>
              </article>
            {:else}<div class="ranking-empty">{t('analysis.noResults')}</div>{/each}
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
                <ol>{#each group.items as item, index}<li><span class="rank">{index + 1}</span><div class="ranking-product"><strong>{item.name}</strong><span>{item.code}{item.brand ? ` · ${item.brand}` : ''}</span></div><div class="ranking-values"><b>{formatNumber(item.quantity)} {t('analysis.units')}</b><span>{formatMoney(item.amount)}</span></div></li>{/each}</ol>
              </article>
            {:else}<div class="ranking-empty">{t('analysis.noResults')}</div>{/each}
          </div>
        </section>
      {:else if activeView === 'products'}
        <section class="analysis-table-card surface-card" aria-labelledby="items-title">
          <div class="analysis-table-heading"><h2 id="items-title">{t('analysis.items')}</h2><strong>{t('common.items', { count: filteredItems.length })}</strong></div>
          <div class="table-scroll"><table>
            <thead><tr><th>{t('analysis.store')}</th><th>{t('analysis.category')}</th><th>{t('analysis.article')}</th><th class="numeric">{t('analysis.transactions')}</th><th class="numeric">{t('analysis.grossSales')}</th><th class="numeric">{t('analysis.returns')}</th><th class="numeric">{t('analysis.netQuantity')}</th><th class="numeric">{t('analysis.netSales')}</th></tr></thead>
            <tbody>{#each pageRows as item (`${item.storeId}:${item.articleCode}:${item.category5}`)}<tr><td><strong>{item.storeId}</strong></td><td class="category-cell"><strong>{categoryValue(item, 'category4')}</strong><span>{item.category4Code || item.category5Code || categoryValue(item, 'category5')}</span></td><td class="article-cell"><strong>{item.articleName || t('common.notAvailable')}</strong><span>{item.articleCode}{item.brandName ? ` · ${item.brandName}` : ''}</span></td><td class="numeric">{formatNumber(item.transactionCount)}</td><td class="numeric">{formatMoney(item.saleAmount)}</td><td class="numeric">{formatMoney(item.returnAmount)}</td><td class="numeric">{formatNumber(item.netQuantity)}</td><td class="numeric net-value">{formatMoney(item.netSalesAmount)}</td></tr>{:else}<tr><td colspan="8" class="empty-table">{t('analysis.noResults')}</td></tr>{/each}</tbody>
          </table></div>
          {#if pageCount > 1}<div class="pagination"><md-outlined-button type="button" disabled={page === 1} onclick={() => { page -= 1; }}>{t('analysis.previous')}</md-outlined-button><strong>{page} / {pageCount}</strong><md-outlined-button type="button" disabled={page === pageCount} onclick={() => { page += 1; }}>{t('analysis.next')}</md-outlined-button></div>{/if}
        </section>
      {:else}
        <section class="comparison-card surface-card" aria-labelledby="stores-title">
          <div class="section-heading"><h2 id="stores-title">{t('analysis.storeComparison')}</h2></div>
          <div class="table-scroll store-table"><table>
            <thead><tr><th>{t('analysis.store')}</th><th class="numeric">{t('analysis.currentPeriod')}</th><th class="numeric">{t('analysis.previousPeriod')}</th><th class="numeric">{t('analysis.yearAgoPeriod')}</th><th class="numeric">{t('analysis.vsPrevious')}</th><th class="numeric">{t('analysis.vsYearAgo')}</th><th class="numeric">{t('analysis.transactions')}</th><th class="numeric">{t('analysis.basket')}</th></tr></thead>
            <tbody>{#each storeRows as row}<tr><th><strong>{row.id}</strong><span>{row.label}</span></th><td class="numeric emphasis">{formatValue(row.current?.netSalesAmount, 'money')}</td><td class="numeric">{formatValue(row.previous?.netSalesAmount, 'money')}</td><td class="numeric">{formatValue(row.yearAgo?.netSalesAmount, 'money')}</td><td class={`numeric ${deltaClass(delta(row.current?.netSalesAmount, row.previous?.netSalesAmount))}`}>{formatPercent(delta(row.current?.netSalesAmount, row.previous?.netSalesAmount))}</td><td class={`numeric ${deltaClass(delta(row.current?.netSalesAmount, row.yearAgo?.netSalesAmount))}`}>{formatPercent(delta(row.current?.netSalesAmount, row.yearAgo?.netSalesAmount))}</td><td class="numeric">{formatValue(row.current?.transactionCount, 'number')}</td><td class="numeric">{row.current?.transactionCount && row.current?.trendNetSalesAmount !== undefined ? formatMoney(row.current.trendNetSalesAmount / row.current.transactionCount) : '—'}</td></tr>{:else}<tr><td colspan="8" class="empty-table">{t('analysis.noResults')}</td></tr>{/each}</tbody>
          </table></div>
        </section>
      {/if}
      </div>
    </section>
  {/if}
</section>

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
  .export-notice { display: flex; align-items: center; gap: 12px; }
  .export-notice-copy { display: grid; min-width: 0; flex: 1; gap: 2px; }
  .export-notice-copy code { overflow: hidden; color: var(--md-sys-color-on-surface-variant); font-family: "Cascadia Code", ui-monospace, monospace; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
  .analysis-results { display: grid; gap: 12px; }
  .analysis-toolbar { display: grid; gap: 10px; overflow: visible; }
  .analysis-workspace { display: grid; align-content: start; gap: 16px; overflow: visible; }
  .period-summary { display: flex; flex-wrap: wrap; gap: 6px 14px; color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-variant-numeric: tabular-nums; }
  .period-summary .current { color: var(--md-sys-color-primary); font-weight: 650; }
  .store-search { margin-top: 12px; }
  .analysis-heading-actions { display: flex; align-items: center; gap: 9px; }
  .analysis-loading { display: flex; min-height: 220px; align-items: center; justify-content: center; gap: 14px; }
  .analysis-loading md-circular-progress, .inline-loading md-circular-progress { width: 24px; height: 24px; }
  .analysis-query { padding: 24px; }
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
  .analysis-progress { margin-top: 18px; padding: 24px; }
  .analysis-progress-heading { align-items: flex-start; margin-bottom: 16px; }
  .analysis-progress-heading h2 { margin: 0 0 5px; }
  .analysis-progress-heading > div > strong { color: var(--md-sys-color-on-surface-variant); }
  .analysis-progress-heading > span { color: var(--md-sys-color-primary); font-size: 28px; font-weight: 740; font-variant-numeric: tabular-nums; }
  .analysis-progress md-linear-progress { width: 100%; --md-linear-progress-track-height: 9px; --md-linear-progress-active-indicator-height: 9px; }
  .analysis-progress-footer { margin-top: 14px; }
  .analysis-progress-footer > strong { color: var(--md-sys-color-on-surface-variant); font-variant-numeric: tabular-nums; }
  .analysis-filters { display: flex; flex-direction: column; gap: 10px; overflow: visible; padding: 12px 14px; }
  .analysis-filter-tools { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 360px); align-items: stretch; gap: 8px; }
  .analysis-filter-tools .analysis-search { min-width: 0; }
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
  .report-tabs { display: flex; width: fit-content; max-width: 100%; gap: 4px; padding: 4px; overflow-x: auto; border: 1px solid var(--app-border); border-radius: 14px; background: var(--app-card); }
  .report-tabs button { display: flex; min-height: 42px; align-items: center; gap: 8px; padding: 8px 16px; cursor: pointer; border: 0; border-radius: 10px; color: var(--md-sys-color-on-surface-variant); background: transparent; font-weight: 680; white-space: nowrap; }
  .report-tabs button.active { color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .report-tabs .material-symbols-rounded { font-size: 20px; }
  .analysis-kpis { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 10px; margin: 0; }
  .analysis-kpis > div { grid-column: span 3; min-width: 0; padding: 14px 16px; border: 1px solid var(--app-border); border-radius: 16px; background: var(--app-card); box-shadow: var(--app-shadow); }
  .analysis-kpis > div:nth-child(n + 5) { grid-column: span 4; }
  .analysis-kpis dt { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-weight: 700; }
  .analysis-kpis dd { margin: 6px 0 4px; color: var(--app-summary-value); font-size: clamp(20px, 1.6vw, 26px); font-weight: 730; font-variant-numeric: tabular-nums; letter-spacing: -.02em; line-height: 1.15; white-space: nowrap; }
  .analysis-kpis span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; font-weight: 650; }
  .positive { color: var(--app-proposed) !important; }
  .negative { color: var(--md-sys-color-error) !important; }
  .neutral { color: var(--md-sys-color-on-surface-variant) !important; }
  .performance-card, .comparison-card { overflow: hidden; padding: 0; }
  .section-heading, .comparison-heading, .analysis-table-heading { padding: 19px 21px 14px; }
  .section-heading h2, .comparison-heading h2, .analysis-table-heading h2 { margin: 0; }
  .performance-card .table-scroll { max-height: none; }
  .comparison-card .table-scroll,
    .category-table { max-height: none; overflow: visible; }
  .performance-card th:first-child, .comparison-card th:first-child { text-align: left; }
  .emphasis { color: var(--app-summary-value); font-weight: 740; }
  .top-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  .top-card { overflow: hidden; padding: 0; }
  .top-card ol { display: grid; margin: 0; padding: 0 18px 14px; list-style: none; }
  .top-card li { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 51px; border-top: 1px solid var(--md-sys-color-outline-variant); }
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
  .category-table th:first-child, .store-table th:first-child { min-width: 220px; }
  .weekly-total { background: var(--md-sys-color-surface-container-low); }
  .weekly-total th, .weekly-total td { font-weight: 740; }
  .category-table th span, .store-table th span, .category-cell span, .article-cell span { display: block; margin-top: 3px; color: var(--md-sys-color-on-surface-variant); font-size: 11px; font-weight: 500; }
  .analysis-table-card { padding: 0; overflow: hidden; }
  .analysis-table-heading > strong { color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
  .analysis-table-card .table-scroll { max-height: min(620px, 62vh); }
  .category-cell strong, .article-cell strong { display: block; }
  .net-value { color: var(--app-proposed); font-weight: 700; }
  .empty-table { height: 150px; text-align: center !important; color: var(--md-sys-color-on-surface-variant); }
  .pagination { display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding: 14px 18px; border-top: 1px solid var(--app-table-border); }
  .pagination strong { min-width: 70px; text-align: center; font-variant-numeric: tabular-nums; }
  .ranking-section { overflow: visible; max-height: none; padding: 0; }
  .ranking-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 18px 20px; border-bottom: 1px solid var(--app-table-border); }
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
  .ranking-group ol { display: grid; margin: 0; padding: 0 12px 9px; overflow: visible; list-style: none; }
  .ranking-group li { display: grid; grid-template-columns: 24px minmax(0, 1fr) auto; align-items: center; gap: 9px; min-height: 48px; border-top: 1px solid var(--app-table-border); }
  .ranking-group li:first-child { border-top: 0; }
  .ranking-group .rank { display: grid; width: 22px; height: 22px; place-items: center; border-radius: 7px; color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); font-size: 11px; font-weight: 750; }
  .ranking-product, .ranking-values { display: grid; min-width: 0; gap: 2px; }
  .ranking-product strong, .ranking-product span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ranking-product span, .ranking-values span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .ranking-values { min-width: max-content; justify-items: end; text-align: right; font-variant-numeric: tabular-nums; }
  .ranking-empty { min-height: 120px; display: grid; place-items: center; color: var(--md-sys-color-on-surface-variant); }
  .analysis-supplement { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 12px 16px; padding: 12px 14px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 16px; background: var(--md-sys-color-surface-container-low); }
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

  @media (max-width: 1100px) {
    .analysis-kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .analysis-kpis > div,
    .analysis-kpis > div:nth-child(n + 5) { grid-column: auto; }
  }

  @media (max-width: 980px) {
    .analysis-filter-tools { grid-template-columns: 1fr; }
    .top-grid { grid-template-columns: 1fr; }
    .focus-columns { grid-template-columns: 1fr; }
    .focus-columns > div + div { border-top: 1px solid var(--app-table-border); border-left: 0; }
    .comparison-heading { flex-direction: column; }
    .ranking-heading { align-items: flex-start; flex-direction: column; }
  }

  @media (max-width: 620px) {
    .analysis-heading-actions { width: 100%; align-items: stretch; flex-direction: column; }
    .analysis-query { padding: 18px; }
    .analysis-kpis { grid-template-columns: 1fr; }
    .analysis-kpis > div,
    .analysis-kpis > div:nth-child(n + 5) { grid-column: auto; }
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
