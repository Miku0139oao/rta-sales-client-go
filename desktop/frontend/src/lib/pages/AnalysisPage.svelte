<script lang="ts">
  import { onMount } from 'svelte';
  import { backend } from '../backend';
  import { errorMessage } from '../i18n';
  import {
    bytesToBase64,
    generateSalesAnalysisPDF,
    listSuccessfulReportStores,
    salesAnalysisPDFFilename,
  } from '../sales-report-pdf';
  import type { Translator } from '../i18n';
  import type {
    AppSettings,
    Profile,
    SalesAnalysisItem,
    SalesAnalysisPeriodRequest,
    SalesAnalysisPeriodResult,
    SalesAnalysisProgress,
    SalesAnalysisResult,
    SalesAnalysisStore,
    SalesAnalysisTotals,
  } from '../types';

  export let t: Translator;
  export let settings: AppSettings;
  export let onBusyChange: (busy: boolean) => void = () => undefined;
  export let onGoToAccounts: () => void = () => undefined;

  type CategoryKey = 'category1' | 'category2' | 'category3' | 'category4' | 'category5';
  type FacetSelections = Record<CategoryKey, Set<string>>;
  type ReportView = 'overview' | 'categories' | 'products' | 'stores';
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
  let cancelling = false;
  let exportingPDF = false;
  let pdfExportCurrent = 0;
  let pdfExportTotal = 0;
  let error = '';
  let exportNotice = '';
  let result: SalesAnalysisResult | undefined;
  let progress: SalesAnalysisProgress | undefined;
  let operationId = '';
  let periodMode: PeriodMode = 'month';
  let month = localISOMonth();
  let from = `${month}-01`;
  let to = localISODate();
  let activeView: ReportView = 'overview';
  let search = '';
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
  let page = 1;
  let pageCount = 1;
  let pageRows: SalesAnalysisItem[] = [];

  $: busy = loadingProfiles || loadingStores || running || exportingPDF;
  $: onBusyChange(busy);
  $: rangeInvalid = periodMode === 'range' && Boolean(from && to && from > to);
  $: reportPeriods = normalizePeriods(result);
  $: currentPeriod = periodByKey(reportPeriods, 'current') ?? reportPeriods[0];
  $: filteredItems = currentPeriod ? currentPeriod.items.filter((item) => matchesFilters(item, selections, search)) : [];
  $: currentTotals = totalsForPeriod(currentPeriod, selections, search);
  $: previousTotals = totalsForPeriod(periodByKey(reportPeriods, 'previous'), selections, search);
  $: previous2Totals = totalsForPeriod(periodByKey(reportPeriods, 'previous2'), selections, search);
  $: yearAgoTotals = totalsForPeriod(periodByKey(reportPeriods, 'yearAgo'), selections, search);
  $: performanceRows = buildPerformanceRows(currentTotals, previousTotals, yearAgoTotals);
  $: categoryRows = buildCategoryComparison(reportPeriods, groupLevel, selections, search);
  $: topSales = buildTopItems(filteredItems, 'amount').slice(0, 15);
  $: topQuantity = buildTopItems(filteredItems, 'quantity').slice(0, 15);
  $: salesRankingPeriods = reportPeriods.filter((period) => ['current', 'yearAgo', 'yearAgoNext'].includes(period.key));
  $: quantityRankingPeriods = reportPeriods.filter((period) => ['current', 'previous', 'previous2'].includes(period.key));
  $: salesRankingPeriod = periodByKey(salesRankingPeriods, salesRankingKey) ?? salesRankingPeriods[0];
  $: quantityRankingPeriod = periodByKey(quantityRankingPeriods, quantityRankingKey) ?? quantityRankingPeriods[0];
  $: salesRankingGroups = buildCategoryRankings(salesRankingPeriod, groupLevel, 'amount', selections, search);
  $: quantityRankingGroups = buildCategoryRankings(quantityRankingPeriod, groupLevel, 'quantity', selections, search);
  $: storeRows = buildStoreRows(reportPeriods);
  $: pageCount = Math.max(1, Math.ceil(filteredItems.length / pageSize));
  $: if (page > pageCount) page = pageCount;
  $: pageRows = filteredItems.slice((page - 1) * pageSize, page * pageSize);
  $: progressPercent = progress?.total ? Math.round((progress.current / progress.total) * 100) : 0;

  onMount(() => {
    const unsubscribe = backend.onSalesAnalysisProgress((next) => {
      progress = next;
      operationId = next.operationId;
    });
    void initialize();
    return unsubscribe;
  });

  async function initialize() {
    loadingProfiles = true;
    error = '';
    try {
      profiles = (await backend.listProfiles()).filter((profile) => profile.enabled && profile.hasCredentials);
      profileId = profiles[0]?.id ?? '';
      if (profileId) await loadStores();
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      loadingProfiles = false;
    }
  }

  async function loadStores() {
    if (!profileId) return;
    loadingStores = true;
    error = '';
    result = undefined;
    stores = [];
    selectedStoreIds = new Set<string>();
    try {
      stores = await backend.listSalesAnalysisStores(profileId);
      selectedStoreIds = new Set(stores.map((store) => store.businessId));
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      loadingStores = false;
    }
  }

  function toggleStore(storeId: string) {
    const next = new Set(selectedStoreIds);
    if (next.has(storeId)) next.delete(storeId);
    else next.add(storeId);
    selectedStoreIds = next;
    result = undefined;
  }

  function selectAllStores() {
    selectedStoreIds = new Set(stores.map((store) => store.businessId));
    result = undefined;
  }

  function clearStores() {
    selectedStoreIds = new Set<string>();
    result = undefined;
  }

  async function runAnalysis() {
    if (!profileId || selectedStoreIds.size === 0 || rangeInvalid) return;
    const periods = buildPeriodRequests();
    if (periods.length === 0) return;
    running = true;
    cancelling = false;
    error = '';
    result = undefined;
    progress = undefined;
    operationId = '';
    resetFilters();
    try {
      result = await backend.runSalesAnalysis({
        profileId,
        storeIds: [...selectedStoreIds],
        periods,
        concurrency: settings.accountConcurrency,
      });
      activeView = 'overview';
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      running = false;
      cancelling = false;
    }
  }

  async function cancelAnalysis() {
    if (!operationId || cancelling) return;
    cancelling = true;
    try {
      await backend.cancelSalesAnalysis(operationId);
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
      cancelling = false;
    }
  }

  async function exportPDF() {
    if (!result || exportingPDF) return;
    exportingPDF = true;
    pdfExportCurrent = 0;
    pdfExportTotal = 0;
    error = '';
    exportNotice = '';
    try {
      const directory = await backend.chooseSalesAnalysisPDFDirectory();
      if (!directory) return;
      const reportStores = listSuccessfulReportStores(result);
      if (reportStores.length === 0) throw new Error('No successful store is available for PDF export');
      pdfExportTotal = reportStores.length;
      const written: string[] = [];
      for (const [index, store] of reportStores.entries()) {
        pdfExportCurrent = index + 1;
        await yieldToUI();
        const data = await generateSalesAnalysisPDF(result, store.businessId, groupLevel, settings.locale);
        written.push(await backend.writeSalesAnalysisPDF({
          directory,
          filename: salesAnalysisPDFFilename(store.businessId, result.from, result.to),
          dataBase64: bytesToBase64(data),
        }));
      }
      exportNotice = t('analysis.exportedPDF', { count: written.length, directory });
    } catch (caught) {
      error = errorMessage(settings.locale, caught);
    } finally {
      exportingPDF = false;
    }
  }

  function yieldToUI(): Promise<void> {
    return new Promise((resolve) => window.setTimeout(resolve, 0));
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
      stores: value.stores, items: value.items, issues: value.issues,
    }];
  }

  function periodByKey(periods: SalesAnalysisPeriodResult[], key: string): SalesAnalysisPeriodResult | undefined {
    return periods.find((period) => period.key === key);
  }

  function categoryValue(item: SalesAnalysisItem, key: CategoryKey): string {
    return item[key].trim() || t('analysis.uncategorized');
  }

  function categoryCode(item: SalesAnalysisItem, key: CategoryKey): string {
    if (key === 'category1') return item.category1Code?.trim() ?? '';
    if (key === 'category2') return item.category2Code?.trim() ?? '';
    if (key === 'category3') return item.category3Code?.trim() ?? '';
    if (key === 'category4') return item.category4Code?.trim() ?? '';
    return item.category5Code?.trim() ?? '';
  }

  function matchesSelections(item: SalesAnalysisItem, current: FacetSelections, skipped?: CategoryKey): boolean {
    return facets.every(({ key }) => key === skipped || current[key].size === 0 || current[key].has(categoryValue(item, key)));
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

  function filtersActive(current: FacetSelections, searchTerm: string): boolean {
    return Boolean(searchTerm.trim()) || facets.some(({ key }) => current[key].size > 0);
  }

  function facetOptions(key: CategoryKey): string[] {
    if (!currentPeriod) return [];
    return [...new Set(currentPeriod.items.filter((item) => matchesSelections(item, selections, key)).map((item) => categoryValue(item, key)))]
      .sort((left, right) => left.localeCompare(right, settings.locale));
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
    selections = { ...selections, [key]: new Set(facetOptions(key)) };
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

  function totalsForPeriod(period: SalesAnalysisPeriodResult | undefined, current: FacetSelections, searchTerm: string): FilteredTotals {
    if (!period) return emptyTotals();
    const matching = period.items.filter((item) => matchesFilters(item, current, searchTerm));
    if (filtersActive(current, searchTerm)) return summarize(matching);
    const sku = new Set(matching.map((item) => item.articleCode || item.articleName));
    const totals: FilteredTotals = { ...period.totals, skuCount: sku.size };
    if (totals.transactionCount && totals.transactionCount > 0 && totals.trendNetSalesAmount !== undefined) {
      totals.basketValue = totals.trendNetSalesAmount / totals.transactionCount;
    }
    return totals;
  }

  function buildPerformanceRows(current: FilteredTotals, previous: FilteredTotals, yearAgo: FilteredTotals): PerformanceRow[] {
    return [
      { label: t('analysis.grossSales'), current: current.saleAmount, previous: previous.saleAmount, yearAgo: yearAgo.saleAmount, format: 'money' },
      { label: t('analysis.returns'), current: current.returnAmount, previous: previous.returnAmount, yearAgo: yearAgo.returnAmount, format: 'money' },
      { label: t('analysis.netSales'), current: current.netSalesAmount, previous: previous.netSalesAmount, yearAgo: yearAgo.netSalesAmount, format: 'money' },
      { label: t('analysis.netQuantity'), current: current.netQuantity, previous: previous.netQuantity, yearAgo: yearAgo.netQuantity, format: 'number' },
      { label: t('analysis.transactions'), current: current.transactionCount, previous: previous.transactionCount, yearAgo: yearAgo.transactionCount, format: 'number' },
      { label: t('analysis.basket'), current: current.basketValue, previous: previous.basketValue, yearAgo: yearAgo.basketValue, format: 'money' },
    ];
  }

  function buildCategoryComparison(
    periods: SalesAnalysisPeriodResult[], key: CategoryKey, current: FacetSelections, searchTerm: string,
  ): CategoryComparisonRow[] {
    const grouped = new Map<string, CategoryComparisonRow>();
    for (const period of periods) {
      if (!['current', 'previous', 'previous2', 'yearAgo'].includes(period.key)) continue;
      for (const item of period.items) {
        if (!matchesFilters(item, current, searchTerm)) continue;
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
  ): CategoryRankingGroup[] {
    if (!period) return [];
    const grouped = new Map<string, { code: string; name: string; items: SalesAnalysisItem[]; amount: number; quantity: number }>();
    for (const item of period.items) {
      if (!matchesFilters(item, current, searchTerm)) continue;
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

  function buildStoreRows(periods: SalesAnalysisPeriodResult[]): StoreComparisonRow[] {
    const grouped = new Map<string, StoreComparisonRow>();
    for (const period of periods) {
      if (!['current', 'previous', 'yearAgo'].includes(period.key)) continue;
      for (const store of period.stores) {
        const row: StoreComparisonRow = grouped.get(store.businessId) ?? { id: store.businessId, label: store.label };
        row[period.key as 'current' | 'previous' | 'yearAgo'] = store.totals;
        grouped.set(store.businessId, row);
      }
    }
    return [...grouped.values()].sort((left, right) => (right.current?.netSalesAmount ?? 0) - (left.current?.netSalesAmount ?? 0) || left.id.localeCompare(right.id));
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

  function formatNumber(value: number): string {
    return new Intl.NumberFormat(settings.locale, { maximumFractionDigits: 2 }).format(value);
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

<section class="page analysis-page" aria-labelledby="analysis-title">
  <div class="page-heading split-heading">
    <h1 id="analysis-title">{t('analysis.title')}</h1>
    {#if result}
      <div class="analysis-heading-actions">
        <md-filled-button type="button" onclick={() => void exportPDF()} disabled={exportingPDF}>
          <span class="material-symbols-rounded" slot="icon" aria-hidden="true">picture_as_pdf</span>{exportingPDF ? t('analysis.exportingPDFProgress', { current: pdfExportCurrent, total: pdfExportTotal }) : t('analysis.exportPDF')}
        </md-filled-button>
        <md-outlined-button type="button" onclick={() => { result = undefined; exportNotice = ''; resetFilters(); }} disabled={exportingPDF}>
          <span class="material-symbols-rounded" slot="icon" aria-hidden="true">tune</span>{t('analysis.changeQuery')}
        </md-outlined-button>
      </div>
    {/if}
  </div>

  {#if error}
    <div class="notice error-notice" role="alert"><span class="material-symbols-rounded" aria-hidden="true">error</span><span>{error}</span></div>
  {/if}
  {#if exportNotice}
    <div class="notice success-notice" role="status"><span class="material-symbols-rounded" aria-hidden="true">check_circle</span><span>{exportNotice}</span></div>
  {/if}

  {#if loadingProfiles}
    <div class="analysis-loading surface-card" role="status"><md-circular-progress indeterminate></md-circular-progress><strong>{t('common.loading')}</strong></div>
  {:else if profiles.length === 0}
    <div class="empty-state surface-card">
      <span class="material-symbols-rounded" aria-hidden="true">manage_accounts</span>
      <h2>{t('analysis.noAccounts')}</h2>
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
          <div class="store-grid">
            {#each stores as store (store.businessId)}
              <label class:checked={selectedStoreIds.has(store.businessId)}><input type="checkbox" checked={selectedStoreIds.has(store.businessId)} onchange={() => toggleStore(store.businessId)} /><span class="store-id">{store.businessId}</span><span class="store-name">{store.label}</span></label>
            {/each}
          </div>
        {/if}
      </div>

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
      {#if !result.complete}<div class="notice warning-notice" role="status"><span class="material-symbols-rounded" aria-hidden="true">warning</span><span>{t('analysis.partialResult', { count: result.issues?.length ?? 0 })}</span></div>{/if}

      <div class="period-strip surface-card" aria-label={t('analysis.periods')}>
        {#each reportPeriods as period}<div><strong>{period.label}</strong><span>{period.from} — {period.to}</span></div>{/each}
      </div>

      <div class="analysis-filters surface-card">
        <div class="facet-row" aria-label={t('analysis.categoryFilters')}>
          {#each facets as facet}
            <details class="facet-menu">
              <summary class:active={selections[facet.key].size > 0}><span>{t(facet.label)}</span><strong>{selections[facet.key].size > 0 ? selections[facet.key].size : t('analysis.all')}</strong><span class="material-symbols-rounded" aria-hidden="true">expand_more</span></summary>
              <div class="facet-popover">
                <div class="facet-actions"><button type="button" onclick={() => selectFacetAll(facet.key)}>{t('analysis.selectAll')}</button><button type="button" onclick={() => clearFacet(facet.key)}>{t('analysis.clear')}</button></div>
                <div class="facet-options">{#each facetOptions(facet.key) as option}<label><input aria-label={option} type="checkbox" checked={selections[facet.key].has(option)} onchange={() => toggleFacet(facet.key, option)} /><span>{option}</span></label>{/each}</div>
              </div>
            </details>
          {/each}
        </div>
        <div class="analysis-search"><span class="material-symbols-rounded" aria-hidden="true">search</span><input aria-label={t('analysis.search')} placeholder={t('analysis.search')} value={search} oninput={changeSearch} />{#if search}<button type="button" aria-label={t('analysis.clear')} onclick={() => { search = ''; page = 1; }}><span class="material-symbols-rounded" aria-hidden="true">close</span></button>{/if}</div>
      </div>

      <div class="report-tabs" role="tablist" aria-label={t('analysis.reportViews')}>
        {#each [
          { key: 'overview', label: t('analysis.overview'), icon: 'space_dashboard' },
          { key: 'categories', label: t('analysis.categories'), icon: 'account_tree' },
          { key: 'products', label: t('analysis.products'), icon: 'inventory_2' },
          { key: 'stores', label: t('analysis.stores'), icon: 'storefront' },
        ] as tab}
          <button type="button" class:active={activeView === tab.key} role="tab" aria-selected={activeView === tab.key} onclick={() => { activeView = tab.key as ReportView; }}><span class="material-symbols-rounded" aria-hidden="true">{tab.icon}</span>{tab.label}</button>
        {/each}
      </div>

      {#if activeView === 'overview'}
        <dl class="analysis-kpis">
          <div><dt>{t('analysis.netSales')}</dt><dd>{formatMoney(currentTotals.netSalesAmount)}</dd><span class={deltaClass(delta(currentTotals.netSalesAmount, yearAgoTotals.netSalesAmount))}>{formatPercent(delta(currentTotals.netSalesAmount, yearAgoTotals.netSalesAmount))} {t('analysis.vsYearAgo')}</span></div>
          <div><dt>{t('analysis.grossSales')}</dt><dd>{formatMoney(currentTotals.saleAmount)}</dd><span class={deltaClass(delta(currentTotals.saleAmount, previousTotals.saleAmount))}>{formatPercent(delta(currentTotals.saleAmount, previousTotals.saleAmount))} {t('analysis.vsPrevious')}</span></div>
          <div><dt>{t('analysis.returns')}</dt><dd>{formatMoney(currentTotals.returnAmount)}</dd><span>{formatNumber(currentTotals.returnQuantity)} {t('analysis.units')}</span></div>
          <div><dt>{t('analysis.netQuantity')}</dt><dd>{formatNumber(currentTotals.netQuantity)}</dd><span class={deltaClass(delta(currentTotals.netQuantity, yearAgoTotals.netQuantity))}>{formatPercent(delta(currentTotals.netQuantity, yearAgoTotals.netQuantity))} {t('analysis.vsYearAgo')}</span></div>
          <div><dt>{t('analysis.transactions')}</dt><dd>{formatValue(currentTotals.transactionCount, 'number')}</dd><span>{filtersActive(selections, search) ? t('analysis.wholeStoreOnly') : t('analysis.orders')}</span></div>
          <div><dt>{t('analysis.basket')}</dt><dd>{formatValue(currentTotals.basketValue, 'money')}</dd><span>{filtersActive(selections, search) ? t('analysis.wholeStoreOnly') : t('analysis.perOrder')}</span></div>
          <div><dt>{t('analysis.skus')}</dt><dd>{formatNumber(currentTotals.skuCount)}</dd><span>{t('analysis.products')}</span></div>
        </dl>

        <section class="performance-card surface-card" aria-labelledby="performance-title">
          <div class="section-heading"><h2 id="performance-title">{t('analysis.performance')}</h2></div>
          <div class="table-scroll"><table>
            <thead><tr><th>{t('analysis.metric')}</th><th class="numeric">{t('analysis.currentPeriod')}</th><th class="numeric">{t('analysis.previousPeriod')}</th><th class="numeric">{t('analysis.yearAgoPeriod')}</th><th class="numeric">{t('analysis.variance')}</th><th class="numeric">{t('analysis.variancePercent')}</th></tr></thead>
            <tbody>{#each performanceRows as row}<tr><th>{row.label}</th><td class="numeric emphasis">{formatValue(row.current, row.format)}</td><td class="numeric">{formatValue(row.previous, row.format)}</td><td class="numeric">{formatValue(row.yearAgo, row.format)}</td><td class="numeric">{row.current === undefined || row.yearAgo === undefined ? '—' : formatValue(row.current - row.yearAgo, row.format)}</td><td class={`numeric ${deltaClass(delta(row.current, row.yearAgo))}`}>{formatPercent(delta(row.current, row.yearAgo))}</td></tr>{/each}</tbody>
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
      {:else if activeView === 'categories'}
        <section class="comparison-card surface-card" aria-labelledby="category-title">
          <div class="comparison-heading"><h2 id="category-title">{t('analysis.rolling')}</h2><div class="group-tabs" role="radiogroup" aria-label={t('analysis.groupBy')}>{#each facets as facet}<button type="button" class:active={groupLevel === facet.key} role="radio" aria-checked={groupLevel === facet.key} onclick={() => { groupLevel = facet.key; }}>{t(facet.label)}</button>{/each}</div></div>
          <div class="table-scroll category-table"><table>
            <thead><tr><th>{t('analysis.category')}</th><th class="numeric">{t('analysis.currentPeriod')}</th><th class="numeric">{t('analysis.previousPeriod')}</th><th class="numeric">{t('analysis.previous2Period')}</th><th class="numeric">{t('analysis.yearAgoPeriod')}</th><th class="numeric">{t('analysis.vsPrevious')}</th><th class="numeric">{t('analysis.vsYearAgo')}</th></tr></thead>
            <tbody>{#each categoryRows as row}<tr><th><strong>{row.name}</strong>{#if row.code}<span>{row.code}</span>{/if}</th><td class="numeric emphasis">{formatMoney(row.current)}</td><td class="numeric">{formatMoney(row.previous)}</td><td class="numeric">{formatMoney(row.previous2)}</td><td class="numeric">{formatMoney(row.yearAgo)}</td><td class={`numeric ${deltaClass(delta(row.current, row.previous))}`}>{formatPercent(delta(row.current, row.previous))}</td><td class={`numeric ${deltaClass(delta(row.current, row.yearAgo))}`}>{formatPercent(delta(row.current, row.yearAgo))}</td></tr>{:else}<tr><td colspan="7" class="empty-table">{t('analysis.noResults')}</td></tr>{/each}</tbody>
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
    </section>
  {/if}
</section>

<style>
  .analysis-page { max-width: 1480px; }
  .analysis-heading-actions { display: flex; align-items: center; gap: 9px; }
  .analysis-loading { display: flex; min-height: 220px; align-items: center; justify-content: center; gap: 14px; }
  .analysis-loading md-circular-progress, .inline-loading md-circular-progress { width: 24px; height: 24px; }
  .analysis-query { padding: 24px; }
  .analysis-query-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(210px, 1fr)); gap: 16px; }
  .store-selection { margin-top: 22px; }
  .selection-heading, .analysis-query-actions, .analysis-progress-heading, .analysis-progress-footer, .comparison-heading, .analysis-table-heading, .section-heading { display: flex; align-items: center; justify-content: space-between; gap: 14px; }
  .selection-heading > div, .facet-actions { display: flex; gap: 8px; }
  .selection-heading button, .facet-actions button, .analysis-search button { cursor: pointer; border: 0; color: var(--md-sys-color-primary); background: transparent; font-weight: 680; }
  .selection-heading button:disabled { cursor: default; opacity: .4; }
  .store-grid { display: grid; max-height: 280px; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 9px; margin-top: 12px; overflow: auto; padding: 2px; }
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
  .analysis-results { display: grid; gap: 16px; }
  .period-strip { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 1px; overflow: hidden; padding: 0; }
  .period-strip > div { display: grid; gap: 4px; padding: 15px 17px; border-right: 1px solid var(--md-sys-color-outline-variant); }
  .period-strip > div:last-child { border-right: 0; }
  .period-strip span { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-variant-numeric: tabular-nums; }
  .analysis-filters { display: grid; grid-template-columns: minmax(0, 1fr) minmax(240px, 320px); align-items: start; gap: 14px; padding: 14px; }
  .facet-row { display: flex; flex-wrap: wrap; gap: 8px; }
  .facet-menu { position: relative; }
  .facet-menu > summary { display: flex; min-height: 44px; align-items: center; gap: 8px; padding: 8px 12px; cursor: pointer; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 12px; color: var(--md-sys-color-on-surface-variant); background: var(--md-sys-color-surface-container-lowest); list-style: none; }
  .facet-menu > summary::-webkit-details-marker { display: none; }
  .facet-menu > summary.active { border-color: var(--app-active-border); color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .facet-menu > summary strong { color: var(--md-sys-color-primary); font-size: 12px; }
  .facet-menu > summary .material-symbols-rounded { font-size: 19px; transition: transform 120ms ease; }
  .facet-menu[open] > summary .material-symbols-rounded { transform: rotate(180deg); }
  .facet-popover { position: absolute; z-index: 12; top: calc(100% + 7px); left: 0; width: min(330px, calc(100vw - 40px)); padding: 10px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 14px; background: var(--md-sys-color-surface-container-lowest); box-shadow: var(--app-shadow-high); }
  .facet-actions { justify-content: flex-end; padding: 2px 4px 8px; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
  .facet-options { display: grid; max-height: 260px; gap: 3px; overflow: auto; padding-top: 7px; }
  .facet-options label { display: flex; align-items: center; gap: 10px; padding: 8px; cursor: pointer; border-radius: 9px; }
  .facet-options label:hover { background: var(--md-sys-color-surface-container-low); }
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
  .analysis-kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(290px, 1fr)); gap: 11px; margin: 0; }
  .analysis-kpis > div { min-width: 0; min-height: 116px; padding: 18px 19px; border: 1px solid var(--app-border); border-radius: 17px; background: var(--app-card); box-shadow: var(--app-shadow); }
  .analysis-kpis dt { color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-weight: 700; }
  .analysis-kpis dd { margin: 8px 0 5px; color: var(--app-summary-value); font-size: clamp(20px, 1.8vw, 27px); font-weight: 730; font-variant-numeric: tabular-nums; letter-spacing: -.02em; line-height: 1.15; white-space: nowrap; }
  .analysis-kpis span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; font-weight: 650; }
  .positive { color: var(--app-proposed) !important; }
  .negative { color: var(--md-sys-color-error) !important; }
  .neutral { color: var(--md-sys-color-on-surface-variant) !important; }
  .performance-card, .comparison-card { overflow: hidden; padding: 0; }
  .section-heading, .comparison-heading, .analysis-table-heading { padding: 19px 21px 14px; }
  .section-heading h2, .comparison-heading h2, .analysis-table-heading h2 { margin: 0; }
  .performance-card .table-scroll, .comparison-card .table-scroll { max-height: min(620px, 62vh); }
  .analysis-results .table-scroll { scrollbar-width: none; -ms-overflow-style: none; }
  .analysis-results .table-scroll::-webkit-scrollbar { display: none; width: 0; height: 0; }
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
  .comparison-heading { align-items: flex-start; }
  .group-tabs { display: flex; flex-wrap: wrap; overflow: hidden; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 11px; }
  .group-tabs button { min-height: 38px; padding: 7px 10px; cursor: pointer; border: 0; border-right: 1px solid var(--md-sys-color-outline-variant); color: var(--md-sys-color-on-surface-variant); background: transparent; font-size: 12px; font-weight: 650; }
  .group-tabs button:last-child { border-right: 0; }
  .group-tabs button.active { color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
  .category-table th:first-child, .store-table th:first-child { min-width: 220px; }
  .category-table th span, .store-table th span, .category-cell span, .article-cell span { display: block; margin-top: 3px; color: var(--md-sys-color-on-surface-variant); font-size: 11px; font-weight: 500; }
  .analysis-table-card { padding: 0; overflow: hidden; }
  .analysis-table-heading > strong { color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
  .analysis-table-card .table-scroll { max-height: min(620px, 62vh); }
  .category-cell strong, .article-cell strong { display: block; }
  .net-value { color: var(--app-proposed); font-weight: 700; }
  .empty-table { height: 150px; text-align: center !important; color: var(--md-sys-color-on-surface-variant); }
  .pagination { display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding: 14px 18px; border-top: 1px solid var(--app-table-border); }
  .pagination strong { min-width: 70px; text-align: center; font-variant-numeric: tabular-nums; }
  .ranking-section { overflow: hidden; padding: 0; }
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
  .ranking-group ol { display: grid; margin: 0; padding: 0 12px 9px; list-style: none; }
  .ranking-group li { display: grid; grid-template-columns: 24px minmax(0, 1fr) auto; align-items: center; gap: 9px; min-height: 48px; border-top: 1px solid var(--app-table-border); }
  .ranking-group li:first-child { border-top: 0; }
  .ranking-group .rank { display: grid; width: 22px; height: 22px; place-items: center; border-radius: 7px; color: var(--md-sys-color-primary); background: var(--md-sys-color-secondary-container); font-size: 11px; font-weight: 750; }
  .ranking-product, .ranking-values { display: grid; min-width: 0; gap: 2px; }
  .ranking-product strong, .ranking-product span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ranking-product span, .ranking-values span { color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .ranking-values { min-width: max-content; justify-items: end; text-align: right; font-variant-numeric: tabular-nums; }
  .ranking-empty { min-height: 120px; display: grid; place-items: center; color: var(--md-sys-color-on-surface-variant); }

  @media (max-width: 980px) {
    .analysis-filters { grid-template-columns: 1fr; }
    .period-strip { grid-template-columns: 1fr 1fr; }
    .period-strip > div:nth-child(2) { border-right: 0; }
    .top-grid { grid-template-columns: 1fr; }
    .comparison-heading { flex-direction: column; }
    .ranking-heading { align-items: flex-start; flex-direction: column; }
  }

  @media (max-width: 620px) {
    .analysis-heading-actions { width: 100%; align-items: stretch; flex-direction: column; }
    .analysis-query { padding: 18px; }
    .analysis-kpis { grid-template-columns: 1fr; }
    .store-grid, .period-strip { grid-template-columns: 1fr; }
    .period-strip > div { border-right: 0; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
    .period-strip > div:last-child { border-bottom: 0; }
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
