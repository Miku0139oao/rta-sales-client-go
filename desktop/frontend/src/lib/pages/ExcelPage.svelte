<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { backend } from '../backend';
  import { defaultWorkbookEndDate, initialPreviewFilter } from '../excelWorkflow';
  import { errorMessage, type Translator } from '../i18n';
  import { modal } from '../modal';
  import type {
    AnalysisProgress,
    AnalysisResult,
    AppSettings,
    ApplyResult,
    PreviewFilter,
    PreviewRow,
    WorkbookScan,
  } from '../types';

  export let t: Translator;
  export let settings: AppSettings;
  export let onGoToAccounts: () => void;
  export let onBusyChange: (busy: boolean) => void = () => undefined;

  const stages: AnalysisProgress['stage'][] = ['scan', 'login', 'stores', 'query', 'preview'];
  const filters: PreviewFilter[] = ['all', 'change', 'unchanged', 'issue'];

  let inputPath = '';
  let scan: WorkbookScan | undefined;
  let sheetName = '';
  let fromDate = '';
  let toDate = '';
  let opening = false;
  let scanning = false;
  let analyzing = false;
  let cancelling = false;
  let retrying = false;
  let saving = false;
  let progress: AnalysisProgress | undefined;
  let analysis: AnalysisResult | undefined;
  let filter: PreviewFilter = 'all';
  let overwrite = false;
  let allowPartial = false;
  let keepIssueOriginal = false;
  let partialDialog = false;
  let operationError = '';
  let applyResult: ApplyResult | undefined;
  let openingSaved = false;
  let revealingSaved = false;
  let generation = 0;
  let controlsSection: HTMLElement | undefined;
  let progressSection: HTMLElement | undefined;
  let previewSection: HTMLElement | undefined;
  let successSection: HTMLElement | undefined;
  let errorNotice: HTMLElement | undefined;

  $: previewRows = analysis?.preview ?? analysis?.rows ?? [];
  $: filteredRows = filter === 'all'
    ? previewRows
    : filter === 'issue'
      ? previewRows.filter((row) => row.status === 'issue' || row.status === 'failed')
      : previewRows.filter((row) => row.status === filter);
  $: issueCount = analysis ? (analysis.issueCount ?? 0) + (analysis.failedCount ?? 0) : 0;
  $: changeCount = analysis?.changeCount ?? previewRows.filter((row) => row.status === 'change').length;
  $: unchangedCount = analysis?.unchangedCount ?? previewRows.filter((row) => row.status === 'unchanged').length;
  $: failedCount = analysis?.failedCount ?? previewRows.filter((row) => row.status === 'failed').length;
  $: retryableCount = analysis?.retryableCount ?? analysis?.issues?.filter((issue) => issue.retryable).length ?? 0;
  $: analysisComplete = analysis?.complete === true;
  $: hasWritableChanges = (analysis?.changedCellCount ?? 0) > 0;
  $: aggregateProblemCount = analysis?.aggregateProblemCount ?? (analysis ? Math.max(0, analysis.problemCount - issueCount) : 0);
  $: workflowBusy = opening || scanning || analyzing || retrying || saving;
  $: workflowStep = applyResult || saving ? 3 : analyzing || retrying || analysis ? 2 : 1;
  $: onBusyChange(workflowBusy);
  $: activeStageIndex = progress ? stages.indexOf(progress.stage) : -1;
  $: progressValue = progress && progress.total > 0 ? Math.min(1, progress.current / progress.total) : 0;
  $: progressPercent = Math.round(progressValue * 100);
  $: progressRemaining = Math.max(0, (progress?.total ?? 0) - (progress?.current ?? 0));
  $: invalidRange = Boolean(fromDate && toDate && fromDate > toDate);
  $: canAnalyze = Boolean(scan && sheetName && fromDate && toDate && !invalidRange && scan.accounts > 0 && !workflowBusy && !analysis);
  $: partialOverrideAllowed = Boolean(
    analysisComplete && hasWritableChanges && aggregateProblemCount === 0 &&
    issueCount > 0 && allowPartial && keepIssueOriginal
  );
  $: canSave = Boolean(
    analysis && !workflowBusy && analysisComplete && hasWritableChanges && aggregateProblemCount === 0 &&
    (analysis.canApply || partialOverrideAllowed) &&
    (issueCount === 0 || partialOverrideAllowed)
  );

  onMount(() => {
    const cleanups = [
      backend.onProgress((next) => {
        progress = next;
      }),
      backend.onFileDrop((paths) => {
        void acceptDroppedWorkbook(paths);
      }),
    ];
    return () => cleanups.forEach((cleanup) => cleanup());
  });

  async function reveal(getElement: () => HTMLElement | undefined) {
    await tick();
    const element = getElement();
    element?.focus({ preventScroll: true });
    element?.scrollIntoView?.({ behavior: 'auto', block: 'start' });
  }

  async function showOperationError(message: string) {
    operationError = message;
    await reveal(() => errorNotice);
  }

  async function openWorkbook() {
    if (workflowBusy) return;
    const requestGeneration = ++generation;
    opening = true;
    operationError = '';
    try {
      const selected = await backend.openWorkbook();
      if (requestGeneration === generation) opening = false;
      await acceptWorkbook(selected, requestGeneration);
    } catch (error) {
      if (requestGeneration === generation) await showOperationError(errorMessage(settings.locale, error));
    } finally {
      if (requestGeneration === generation) opening = false;
    }
  }

  async function acceptDroppedWorkbook(paths: string[]) {
    if (opening || scanning || analyzing || retrying || saving || paths.length === 0) return;
    const selected = paths.find((path) => isXlsxPath(path)) ?? paths[0];
    const requestGeneration = ++generation;
    await acceptWorkbook(selected, requestGeneration);
  }

  async function acceptWorkbook(selected: string, requestGeneration: number) {
    if (requestGeneration !== generation || !selected) return;
    if (!isXlsxPath(selected)) {
      await showOperationError(t('excel.xlsxOnly'));
      return;
    }
    inputPath = selected;
    sheetName = '';
    await scanWorkbook('', requestGeneration);
  }

  function isXlsxPath(path: string): boolean {
    return path.trim().toLowerCase().endsWith('.xlsx');
  }

  async function scanWorkbook(requestedSheet = sheetName, existingGeneration?: number) {
    if (!inputPath || (existingGeneration === undefined && workflowBusy)) return;
    const requestGeneration = existingGeneration ?? ++generation;
    const requestedInput = inputPath;
    scanning = true;
    operationError = '';
    analysis = undefined;
    applyResult = undefined;
    resetWriteOptions();
    try {
      const next = await backend.scanWorkbook({
        inputPath: requestedInput,
        sheetName: requestedSheet || undefined,
        mappingPath: settings.useLocalMapping ? settings.mappingPath : undefined,
      });
      if (requestGeneration !== generation || inputPath !== requestedInput) return;
      scan = next;
      sheetName = requestedSheet || next.sheetName || next.sheets[0]?.name || '';
      fromDate = next.dateMin || next.dates[0] || '';
      toDate = defaultWorkbookEndDate(fromDate, next.dateMax || next.dates.at(-1) || fromDate);
    } catch (error) {
      if (requestGeneration === generation) {
        scan = undefined;
        await showOperationError(errorMessage(settings.locale, error));
      }
    } finally {
      if (requestGeneration === generation) scanning = false;
    }
  }

  async function changeSheet(event: Event) {
    if (workflowBusy) return;
    const nextSheet = (event.currentTarget as HTMLSelectElement).value;
    sheetName = nextSheet;
    await scanWorkbook(nextSheet);
  }

  async function analyzeWorkbook() {
    allowPartial = false;
    keepIssueOriginal = false;
    partialDialog = false;
    await runAnalysis();
  }

  async function runAnalysis() {
    if (!canAnalyze || !scan) return;
    const requestGeneration = ++generation;
    const requestedInput = inputPath;
    analyzing = true;
    analysis = undefined;
    applyResult = undefined;
    operationError = '';
    progress = { operationId: '', stage: 'scan', current: 0, total: stages.length };
    filter = 'all';
    await reveal(() => progressSection);
    try {
      const result = await backend.analyze({
        inputPath: requestedInput,
        sheetName,
        from: fromDate,
        to: toDate,
        date: fromDate === toDate ? fromDate : undefined,
        maxJobs: settings.maxJobs,
        accountConcurrency: settings.accountConcurrency,
        overwrite,
        useLocalMapping: settings.useLocalMapping,
        mappingPath: settings.useLocalMapping ? settings.mappingPath : '',
      });
      if (requestGeneration !== generation || inputPath !== requestedInput) return;
      const normalized = normalizeAnalysis(result);
      analysis = normalized;
      filter = initialPreviewFilter(normalized);
      progress = { operationId: result.operationId, stage: 'preview', current: stages.length, total: stages.length };
    } catch (error) {
      if (requestGeneration === generation && (error as { code?: string })?.code !== 'cancelled') {
        await showOperationError(errorMessage(settings.locale, error));
      }
    } finally {
      if (requestGeneration === generation) {
        analyzing = false;
        cancelling = false;
        if (analysis && !operationError) await reveal(() => previewSection);
      }
    }
  }

  function normalizeAnalysis(result: AnalysisResult): AnalysisResult {
    const rows = result.preview ?? result.rows ?? [];
    return {
      ...result,
      complete: result.complete === true,
      changedCellCount: result.changedCellCount ?? 0,
      problemCount: result.problemCount ?? 0,
      aggregateProblemCount: result.aggregateProblemCount ?? Math.max(
        0,
        (result.problemCount ?? 0) - rows.filter((row) => row.status === 'issue' || row.status === 'failed').length,
      ),
      preview: rows,
      rows,
      totalCount: result.totalCount ?? rows.length,
      changeCount: result.changeCount ?? rows.filter((row) => row.status === 'change').length,
      unchangedCount: result.unchangedCount ?? rows.filter((row) => row.status === 'unchanged').length,
      issueCount: result.issueCount ?? rows.filter((row) => row.status === 'issue').length,
      failedCount: result.failedCount ?? rows.filter((row) => row.status === 'failed').length,
      overlapCount: result.overlapCount ?? 0,
      issues: result.issues ?? [],
      canApply: result.canApply ?? !rows.some((row) => row.status === 'issue' || row.status === 'failed'),
    };
  }

  async function cancelAnalysis() {
    const operationId = progress?.operationId || analysis?.operationId;
    if ((!analyzing && !retrying) || cancelling || !operationId) return;
    cancelling = true;
    try {
      await backend.cancelAnalysis(operationId);
    } catch (error) {
      await showOperationError(errorMessage(settings.locale, error));
      cancelling = false;
    }
  }

  async function retryFailed() {
    if (!analysis || workflowBusy || retryableCount === 0) return;
    const requestGeneration = ++generation;
    const operationId = analysis.operationId;
    const requestedInput = inputPath;
    retrying = true;
    operationError = '';
    progress = { operationId, stage: 'query', current: 0, total: Math.max(1, retryableCount) };
    await reveal(() => progressSection);
    try {
      const result = await backend.retryFailed(operationId);
      if (requestGeneration !== generation || inputPath !== requestedInput || analysis?.operationId !== operationId) return;
      analysis = normalizeAnalysis(result);
      progress = { operationId, stage: 'preview', current: stages.length, total: stages.length };
    } catch (error) {
      if (requestGeneration === generation) await showOperationError(errorMessage(settings.locale, error));
    } finally {
      if (requestGeneration === generation) {
        retrying = false;
        cancelling = false;
        if (analysis && !operationError) await reveal(() => previewSection);
      }
    }
  }

  async function editSelection() {
    if (workflowBusy) return;
    generation += 1;
    analysis = undefined;
    applyResult = undefined;
    progress = undefined;
    filter = 'all';
    allowPartial = false;
    keepIssueOriginal = false;
    partialDialog = false;
    operationError = '';
    await reveal(() => controlsSection);
  }

  function resetWriteOptions() {
    overwrite = false;
    allowPartial = false;
    keepIssueOriginal = false;
    partialDialog = false;
  }

  function toggleOverwritePolicy() {
    if (analysis || workflowBusy) return;
    overwrite = !overwrite;
  }

  function togglePartial() {
    if (workflowBusy || !analysisComplete || !hasWritableChanges || aggregateProblemCount > 0) return;
    allowPartial = !allowPartial;
    keepIssueOriginal = allowPartial;
  }

  async function requestSave() {
    if (!canSave) return;
    if (allowPartial) {
      partialDialog = true;
      return;
    }
    await saveCopy();
  }

  async function confirmPartial() {
    partialDialog = false;
    await saveCopy();
  }

  async function saveCopy() {
    if (!analysis || saving || workflowBusy) return;
    const requestGeneration = ++generation;
    const requestedAnalysis = analysis;
    const requestedInput = inputPath;
    const requestedFrom = fromDate;
    const requestedTo = toDate;
    saving = true;
    operationError = '';
    try {
      const outputPath = await backend.saveWorkbook({
        inputPath: requestedInput,
        date: requestedFrom === requestedTo ? requestedFrom : undefined,
        from: requestedFrom,
        to: requestedTo,
      });
      if (requestGeneration !== generation || inputPath !== requestedInput || analysis?.operationId !== requestedAnalysis.operationId) return;
      if (!outputPath) return;
      if (normalizePath(outputPath) === normalizePath(requestedInput)) {
        await showOperationError(t('error.output_same_as_input'));
        return;
      }
      const result = await backend.apply({
        operationId: requestedAnalysis.operationId,
        inputPath: requestedInput,
        outputPath,
        overwrite,
        allowPartial,
        keepIssueOriginal,
      });
      if (requestGeneration === generation && inputPath === requestedInput && analysis?.operationId === requestedAnalysis.operationId) {
        applyResult = result;
        await reveal(() => successSection);
      }
    } catch (error) {
      if (requestGeneration === generation) await showOperationError(errorMessage(settings.locale, error));
    } finally {
      if (requestGeneration === generation) saving = false;
    }
  }

  function normalizePath(path: string): string {
    return path.replaceAll('/', '\\').toLocaleLowerCase();
  }

  function filterCount(target: PreviewFilter): number {
    if (target === 'all') return previewRows.length;
    if (target === 'change') return changeCount;
    if (target === 'unchanged') return unchangedCount;
    return issueCount;
  }

  function statusLabel(status: PreviewRow['status']): string {
    return t(`excel.status.${status}`);
  }

  function issueLabel(message?: string): string {
    if (!message) return '';
    const key = `issue.${message}`;
    const translated = t(key);
    return translated === key ? t('issue.generic') : translated;
  }

  function startAnother() {
    if (workflowBusy) return;
    generation += 1;
    inputPath = '';
    scan = undefined;
    analysis = undefined;
    applyResult = undefined;
    progress = undefined;
    operationError = '';
    openingSaved = false;
    revealingSaved = false;
    resetWriteOptions();
  }

  async function openSavedFile() {
    if (!applyResult?.outputPath || openingSaved || revealingSaved) return;
    openingSaved = true;
    operationError = '';
    try {
      await backend.openSavedWorkbook(applyResult.outputPath);
    } catch (error) {
      await showOperationError(errorMessage(settings.locale, error));
    } finally {
      openingSaved = false;
    }
  }

  async function revealSavedFile() {
    if (!applyResult?.outputPath || openingSaved || revealingSaved) return;
    revealingSaved = true;
    operationError = '';
    try {
      await backend.revealSavedWorkbook(applyResult.outputPath);
    } catch (error) {
      await showOperationError(errorMessage(settings.locale, error));
    } finally {
      revealingSaved = false;
    }
  }
</script>

<section class="page excel-page" aria-labelledby="excel-title">
  <div class="page-heading split-heading">
    <div>
      <h1 id="excel-title">{t('excel.title')}</h1>
    </div>
  </div>

  <ol class="workflow-steps" aria-label={t('excel.workflow')}>
    {#each [1, 2, 3] as step}
      <li class:active={workflowStep === step} class:complete={workflowStep > step}>
        <span>{workflowStep > step ? '✓' : step}</span>
        <strong>{t(`excel.step.${step}`)}</strong>
      </li>
    {/each}
  </ol>

  {#if operationError}
    <div bind:this={errorNotice} class="notice error-notice" role="alert" tabindex="-1">
      <span class="material-symbols-rounded" aria-hidden="true">error</span>
      <div><strong>{t('error.title')}</strong><p>{operationError}</p></div>
    </div>
  {/if}

  {#if !inputPath}
    <section
      class="file-drop-card surface-card workbook-drop-target"
      class:drop-disabled={workflowBusy}
      aria-label={t('excel.open')}
    >
      <div class="file-illustration" aria-hidden="true">
        <span class="material-symbols-rounded sheet-symbol">table_view</span>
        <span class="material-symbols-rounded search-symbol">search</span>
      </div>
      <strong class="drop-instruction"><span class="material-symbols-rounded" aria-hidden="true">move_to_inbox</span>{t('excel.dropHere')}</strong>
      <md-filled-button onclick={openWorkbook} disabled={workflowBusy}>
        <span class="material-symbols-rounded" slot="icon">folder_open</span>
        {opening ? t('excel.opening') : t('excel.open')}
      </md-filled-button>
    </section>
  {:else}
    <section
      class="source-card surface-card workbook-drop-target"
      class:drop-disabled={workflowBusy}
      aria-labelledby="source-title"
    >
      <div class="source-icon"><span class="material-symbols-rounded" aria-hidden="true">description</span></div>
      <div class="source-copy">
        <span id="source-title" class="label">{t('excel.source')}</span>
        <strong>{scan?.fileName ?? inputPath.split(/[\\/]/).pop()}</strong>
        <span class="path-text" title={inputPath}>{inputPath}</span>
        {#if workflowStep > 1}<span class="selection-brief">{sheetName} · {fromDate}{fromDate === toDate ? '' : ` → ${toDate}`}</span>{/if}
      </div>
      <md-outlined-button onclick={openWorkbook} disabled={workflowBusy}>{t('excel.changeFile')}</md-outlined-button>
    </section>

    {#if scanning}
      <div class="loading-state surface-card" aria-live="polite">
        <md-circular-progress indeterminate></md-circular-progress>
        <span>{t('excel.progress.scan')}</span>
      </div>
    {:else if scan && !analysis && !analyzing && !retrying && !applyResult}
      <section bind:this={controlsSection} class="workbook-controls surface-card" aria-labelledby="scan-summary-title" tabindex="-1">
        <div class="control-grid">
          <div class="field-group">
            <label for="sheet-name">{t('excel.sheet')}</label>
            <select id="sheet-name" value={sheetName} onchange={changeSheet} disabled={workflowBusy || Boolean(analysis)}>
              {#each scan.sheets as sheet}
                <option value={sheet.name}>{sheet.name}</option>
              {/each}
            </select>
          </div>
          <div class="field-group">
            <label for="from-date">{t('excel.from')}</label>
            <input id="from-date" type="date" min={scan.dateMin} max={scan.dateMax} bind:value={fromDate} disabled={workflowBusy || Boolean(analysis)} />
          </div>
          <div class="date-arrow material-symbols-rounded" aria-hidden="true">arrow_forward</div>
          <div class="field-group">
            <label for="to-date">{t('excel.to')}</label>
            <input id="to-date" type="date" min={scan.dateMin} max={scan.dateMax} bind:value={toDate} disabled={workflowBusy || Boolean(analysis)} />
          </div>
        </div>
        {#if invalidRange}<p class="range-hint field-error">{t('excel.rangeInvalid')}</p>{/if}

        <div class="divider"></div>
        <h2 id="scan-summary-title">{t('excel.summary')}</h2>
        <dl class="summary-grid">
          <div><dt><span class="material-symbols-rounded" aria-hidden="true">tab</span>{t('excel.summarySheets')}</dt><dd>{scan.sheets.length}</dd></div>
          <div><dt><span class="material-symbols-rounded" aria-hidden="true">date_range</span>{t('excel.summaryDates')}</dt><dd>{scan.dates.length}</dd></div>
          <div><dt><span class="material-symbols-rounded" aria-hidden="true">view_list</span>{t('excel.summaryRows')}</dt><dd>{scan.rows}</dd></div>
          <div><dt><span class="material-symbols-rounded" aria-hidden="true">store</span>{t('excel.summaryStores')}</dt><dd>{scan.stores}</dd></div>
          <div><dt><span class="material-symbols-rounded" aria-hidden="true">manage_accounts</span>{t('excel.summaryAccounts')}</dt><dd>{scan.accounts}</dd></div>
        </dl>

        {#if scan.accounts === 0}
          <div class="notice warning-notice" role="status">
            <span class="material-symbols-rounded" aria-hidden="true">warning</span>
            <div><p>{t('excel.noAccounts')}</p><md-text-button onclick={onGoToAccounts}>{t('excel.manageAccounts')}</md-text-button></div>
          </div>
        {/if}

        <div class="pre-analysis-option">
          <div class="write-option">
            <strong>{t('excel.overwriteLabel')}</strong>
            <md-checkbox
              aria-label={t('excel.overwriteLabel')}
              checked={overwrite}
              disabled={workflowBusy || Boolean(analysis)}
              onclick={toggleOverwritePolicy}
            ></md-checkbox>
          </div>
        </div>

        <div class="card-actions">
          <md-text-button onclick={() => scanWorkbook()} disabled={workflowBusy}>{t('excel.scanAgain')}</md-text-button>
          <md-filled-button onclick={analyzeWorkbook} disabled={!canAnalyze}>
            <span class="material-symbols-rounded" slot="icon">analytics</span>{t('excel.analyze')}
          </md-filled-button>
        </div>
      </section>
    {/if}

    {#if analyzing || retrying}
      <section bind:this={progressSection} class="progress-card surface-card" aria-labelledby="progress-title" aria-live="polite" tabindex="-1">
        <div class="progress-heading">
          <h2 id="progress-title">{retrying ? t('excel.retrying') : t('excel.progressTitle')}</h2>
          <div class="progress-percentage">
            <strong>{progressPercent}%</strong>
            <span>{t('excel.progressCount', { current: progress?.current ?? 0, total: progress?.total ?? stages.length })}</span>
          </div>
        </div>
        <md-linear-progress value={progressValue}></md-linear-progress>
        <div class="progress-stats">
          <div><span>{t('excel.progress.completed')}</span><strong>{progress?.current ?? 0}</strong></div>
          <div><span>{t('excel.progress.remaining')}</span><strong>{progressRemaining}</strong></div>
          <div><span>{t('excel.progress.total')}</span><strong>{progress?.total ?? stages.length}</strong></div>
        </div>
        {#if progress?.stage === 'query'}
          {#if progress.storeId || progress.date || progress.profile}
            <div class="progress-latest" class:progress-issue={progress.status === 'issue'}>
              <span class="progress-latest-icon material-symbols-rounded" aria-hidden="true">{progress.status === 'issue' ? 'error' : 'task_alt'}</span>
              <div class="progress-latest-main">
                <span>{t('excel.progress.latest')}</span>
                <strong>{t('excel.progress.latestJob', { store: progress.storeId || '—', date: progress.date || '—' })}</strong>
              </div>
              <div class="progress-latest-meta">
                {#if progress.profile}<span>{t('excel.progress.account', { profile: progress.profile })}</span>{/if}
                <strong>{progress.status === 'issue' ? t('excel.progress.issue') : t('excel.progress.success')}</strong>
                {#if (progress.attempt ?? 0) > 1}<span>{t('excel.progress.attempt', { count: progress.attempt ?? 0 })}</span>{/if}
              </div>
            </div>
          {:else}
            <div class="progress-waiting"><span class="material-symbols-rounded" aria-hidden="true">hourglass_top</span>{t('excel.progress.waiting')}</div>
          {/if}
        {/if}
        <ol class="stage-list">
          {#each stages as stage, index}
            <li class:complete={index < activeStageIndex} class:active={index === activeStageIndex}>
              <span class="stage-marker material-symbols-rounded" aria-hidden="true">{index < activeStageIndex ? 'check' : index === activeStageIndex ? 'progress_activity' : 'circle'}</span>
              <span>{t(`excel.progress.${stage}`)}</span>
            </li>
          {/each}
        </ol>
        <div class="card-actions"><md-outlined-button onclick={cancelAnalysis} disabled={cancelling || !progress?.operationId}>{t('excel.cancelAnalysis')}</md-outlined-button></div>
      </section>
    {/if}

    {#if analysis && !analyzing && !retrying && !applyResult}
      <section bind:this={previewSection} class="preview-section" aria-labelledby="preview-title" tabindex="-1">
        <div class="preview-heading">
          <h2 id="preview-title">{t('excel.previewTitle')}</h2>
          <div class="preview-actions">
            <md-text-button onclick={editSelection} disabled={workflowBusy}>{t('excel.editSelection')}</md-text-button>
            {#if retryableCount > 0}
              <md-outlined-button onclick={retryFailed} disabled={retrying}>
                <span class="material-symbols-rounded" slot="icon">refresh</span>{retrying ? t('excel.retrying') : t('common.retry')}
              </md-outlined-button>
            {/if}
            <md-filled-button onclick={requestSave} disabled={!canSave}>
              <span class="material-symbols-rounded" slot="icon">save_as</span>{saving ? t('excel.savingAs') : t('excel.saveAs')}
            </md-filled-button>
          </div>
        </div>

        {#if analysis.overlapCount > 0}
          <div class="notice warning-notice" role="status">
            <span class="material-symbols-rounded" aria-hidden="true">difference</span>
            <div><strong>{t('excel.overlapTitle')}</strong><p>{t('excel.overlapBody', { count: analysis.overlapCount })}</p></div>
          </div>
        {/if}

        <section class="write-options surface-card" aria-label={t('excel.saveAs')}>
          <div class="write-option">
            <strong>{t('excel.overwriteLabel')}</strong>
            <div class="policy-selection">
              <span class="state-pill" class:profile-enabled={overwrite}>{overwrite ? t('common.on') : t('common.off')}</span>
            </div>
          </div>
          <div class="divider"></div>
          <div class="write-option">
            <strong>{t('excel.partialLabel')}</strong>
            <md-checkbox
              aria-label={t('excel.partialLabel')}
              checked={allowPartial}
              disabled={workflowBusy || !analysisComplete || !hasWritableChanges || aggregateProblemCount > 0}
              onclick={togglePartial}
            ></md-checkbox>
          </div>
          {#if !analysisComplete}
            <div class="inline-blocker" role="status"><span class="material-symbols-rounded" aria-hidden="true">pending_actions</span>{t('excel.incompleteBlock')}</div>
          {:else if aggregateProblemCount > 0}
            <div class="inline-blocker" role="status"><span class="material-symbols-rounded" aria-hidden="true">block</span>{t('excel.aggregateBlock', { count: aggregateProblemCount })}</div>
          {:else if !hasWritableChanges}
            <div class="inline-blocker" role="status"><span class="material-symbols-rounded" aria-hidden="true">info</span>{t('excel.noChanges')}</div>
          {:else if issueCount > 0 && !(allowPartial && keepIssueOriginal)}
            <div class="inline-blocker" role="status"><span class="material-symbols-rounded" aria-hidden="true">block</span>{t('excel.issueBlock', { count: issueCount })}</div>
          {/if}
        </section>

        <div class="filter-bar" role="group" aria-label={t('excel.filterLabel')}>
          {#each filters as target}
            <button type="button" class="filter-chip" class:active={filter === target} aria-pressed={filter === target} onclick={() => (filter = target)}>
              {t(`excel.filter.${target}`)}<span>{filterCount(target)}</span>
            </button>
          {/each}
        </div>

        <div class="table-card surface-card">
          <div class="table-scroll">
            <table>
              <thead><tr>
                <th scope="col">{t('excel.column.date')}</th><th scope="col">{t('excel.column.row')}</th><th scope="col">{t('excel.column.store')}</th><th scope="col">{t('excel.column.profile')}</th>
                <th scope="col" class="numeric">{t('excel.column.currentL')}</th><th scope="col" class="numeric proposed-column">{t('excel.column.proposedL')}</th>
                <th scope="col" class="numeric">{t('excel.column.currentAB')}</th><th scope="col" class="numeric proposed-column">{t('excel.column.proposedAB')}</th><th scope="col">{t('excel.column.status')}</th>
              </tr></thead>
              <tbody>
                {#each filteredRows as row (row.id)}
                  <tr class:issue-row={row.status === 'issue' || row.status === 'failed'}>
                    <td>{row.date}</td><td>{row.row}</td><td>{row.storeLabel}</td><td>{row.profileLabel}</td>
                    <td class="numeric value-current">{row.currentL || '—'}</td><td class="numeric proposed-column">{row.proposedL || '—'}</td>
                    <td class="numeric value-current">{row.currentAB || '—'}</td><td class="numeric proposed-column">{row.proposedAB || '—'}</td>
                    <td><span class="status-chip status-{row.status}">{statusLabel(row.status)}</span>{#if row.message}<small class="row-message">{issueLabel(row.message)}</small>{/if}</td>
                  </tr>
                {:else}
                  <tr><td colspan="9" class="empty-table">{t('excel.noRows')}</td></tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>

      </section>
    {/if}

    {#if applyResult}
      <section bind:this={successSection} class="success-card surface-card" aria-labelledby="saved-title" aria-live="polite" tabindex="-1">
        <div class="success-symbol"><span class="material-symbols-rounded" aria-hidden="true">task_alt</span></div>
        <div>
          <h2 id="saved-title">{t('excel.savedTitle')}</h2>
          <p>{t('excel.savedBody', { changed: applyResult.changedCells, skipped: applyResult.skippedRows })}</p>
          <span class="label">{t('excel.savedPath')}</span>
          <code title={applyResult.outputPath}>{applyResult.outputPath}</code>
        </div>
        <div class="success-actions">
          <md-filled-button onclick={openSavedFile} disabled={openingSaved || revealingSaved}>
            <span class="material-symbols-rounded" slot="icon">file_open</span>
            {openingSaved ? t('excel.openingSaved') : t('excel.openSaved')}
          </md-filled-button>
          <md-outlined-button onclick={revealSavedFile} disabled={openingSaved || revealingSaved}>
            <span class="material-symbols-rounded" slot="icon">folder_open</span>
            {revealingSaved ? t('excel.revealingSaved') : t('excel.showInFolder')}
          </md-outlined-button>
          <md-text-button onclick={startAnother} disabled={workflowBusy}>{t('excel.startAnother')}</md-text-button>
        </div>
      </section>
    {/if}
  {/if}
</section>

{#if partialDialog}
  <dialog use:modal={{ busy: workflowBusy, onClose: () => (partialDialog = false) }} class="app-dialog compact-dialog" aria-modal="true" aria-labelledby="partial-title" aria-describedby="partial-body">
    <form class="confirmation-form" onsubmit={(event) => { event.preventDefault(); confirmPartial(); }}>
      <div class="dialog-symbol warning-symbol"><span class="material-symbols-rounded" aria-hidden="true">rule</span></div>
      <h2 id="partial-title">{t('excel.partialConfirmTitle')}</h2>
      <p id="partial-body">{t('excel.partialConfirmBody')}</p>
      <div class="dialog-actions"><md-text-button type="button" onclick={() => (partialDialog = false)}>{t('common.cancel')}</md-text-button><md-filled-button type="submit" onclick={confirmPartial} data-autofocus>{t('common.confirm')}</md-filled-button></div>
    </form>
  </dialog>
{/if}
