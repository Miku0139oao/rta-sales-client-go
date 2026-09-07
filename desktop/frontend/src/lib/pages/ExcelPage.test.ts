import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import type { AnalysisProgress, AnalysisResult, WorkbookScan } from '../types';
import ExcelPage from './ExcelPage.svelte';

const scan: WorkbookScan = {
  inputPath: 'D:\\sales.xlsx', fileName: 'sales.xlsx', sheetName: 'August',
  sheets: [{ name: 'August', dateMin: '2026-08-01', dateMax: '2026-08-02', rows: 2 }],
  dateMin: '2026-08-01', dateMax: '2026-08-02', dates: ['2026-08-01', '2026-08-02'],
  rows: 2, stores: 1, jobs: 2, accounts: 1,
};

function result(overrides: Partial<AnalysisResult> = {}): AnalysisResult {
  return {
    operationId: 'operation-1',
    complete: true,
    changedCellCount: 2,
    problemCount: 0,
    aggregateProblemCount: 0,
    preview: [{
      id: 'row-1', date: '2026-08-01', row: 4, storeLabel: 'Store 1', profileLabel: 'Primary',
      currentL: '1', currentAB: '2', proposedL: '3', proposedAB: '4', status: 'change',
    }],
    totalCount: 1, changeCount: 1, unchangedCount: 0, issueCount: 0, failedCount: 0,
    overlapCount: 0, issues: [], canApply: true,
    ...overrides,
  };
}

function button(container: HTMLElement, label: string): Element {
  const found = [...container.querySelectorAll('md-filled-button, md-outlined-button, md-text-button')]
    .find((element) => element.textContent?.includes(label));
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

async function openAndScan(container: HTMLElement): Promise<void> {
  await fireEvent.click(button(container, '開啟 Excel 檔案'));
  await waitFor(() => expect(screen.getByText('掃描摘要')).toBeInTheDocument());
}

afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('Excel safety workflow', () => {
  it('marks the first workflow step as current before a file is opened', () => {
    render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    expect(screen.getByText('選擇範圍').closest('li')).toHaveAttribute('aria-current', 'step');
    expect(screen.getByText('檢查結果').closest('li')).not.toHaveAttribute('aria-current');
  });

  it('scans a dropped .xlsx workbook and rejects unsupported drops', async () => {
    let dropListener: ((paths: string[]) => void) | undefined;
    const scanWorkbook = vi.fn(async (input: unknown) => {
      const request = input as { inputPath: string };
      return {
        ...scan,
        inputPath: request.inputPath,
        fileName: request.inputPath.split('\\').pop(),
      };
    });
    configureBackend({
      methods: { ScanWorkbook: scanWorkbook },
      fileDrops: {
        on(listener) {
          dropListener = listener;
          return () => undefined;
        },
      },
    });
    render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await waitFor(() => expect(dropListener).toBeDefined());
    expect(screen.getByText('拖放 .xlsx 到這裡')).toBeInTheDocument();

    dropListener!(['D:\\notes.csv']);
    await waitFor(() => expect(screen.getByText('僅支援 .xlsx 活頁簿')).toBeInTheDocument());
    expect(scanWorkbook).not.toHaveBeenCalled();

    dropListener!(['D:\\notes.csv', 'D:\\daily-sales.XLSX', 'D:\\later.xlsx']);
    dropListener!(['D:\\ignored-while-scanning.xlsx']);
    await waitFor(() => expect(screen.getByText('掃描摘要')).toBeInTheDocument());
    expect(scanWorkbook).toHaveBeenCalledTimes(1);
    expect(scanWorkbook).toHaveBeenCalledWith(expect.objectContaining({ inputPath: 'D:\\daily-sales.XLSX' }));
    expect(screen.getByText('daily-sales.XLSX')).toBeInTheDocument();
  });

  it('enables overwrite-all in one click and keeps the preview policy read-only', async () => {
    const analyze = vi.fn(async (_request: unknown) => result({ overlapCount: 3 }));
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: analyze,
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    const overwriteCheckbox = container.querySelector('md-checkbox')!;
    await fireEvent.click(overwriteCheckbox);
    expect(overwriteCheckbox).toHaveAttribute('checked', 'true');
    expect(container.querySelector('.app-dialog')).not.toBeInTheDocument();
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByText('帳號授權範圍重疊')).toBeInTheDocument());

    expect(analyze).toHaveBeenCalledTimes(1);
    expect(analyze.mock.calls[0]?.[0]).toMatchObject({ overwrite: true, maxJobs: 2000 });
    expect(screen.getAllByText('允許覆寫全部不同值').length).toBeGreaterThan(0);
  });

  it('returns from results to the same range without rescanning the workbook', async () => {
    const scanWorkbook = vi.fn(async () => scan);
    const analyze = vi.fn(async () => result());
    configureBackend({ methods: {
      OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
      ScanWorkbook: scanWorkbook,
      Analyze: analyze,
    } });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.input(screen.getByLabelText('開始日期'), { target: { value: '2026-08-02' } });
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '檢查分析結果' })).toBeInTheDocument());
    expect(screen.queryByRole('heading', { name: '掃描摘要' })).not.toBeInTheDocument();

    await fireEvent.click(button(container, '修改範圍'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '掃描摘要' })).toBeInTheDocument());
    expect((screen.getByLabelText('開始日期') as HTMLInputElement).value).toBe('2026-08-02');
    expect(screen.queryByRole('heading', { name: '檢查分析結果' })).not.toBeInTheDocument();
    expect(scanWorkbook).toHaveBeenCalledTimes(1);
    expect(analyze).toHaveBeenCalledTimes(1);
  });

  it('enables partial write with one explicit choice and asks once before saving', async () => {
    const apply = vi.fn(async (_request: unknown) => ({
      outputPath: 'D:\\filled.xlsx', changedCells: 2, skippedRows: 1,
    }));
    const saveWorkbook = vi.fn(async () => 'D:\\filled.xlsx');
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        SaveWorkbook: saveWorkbook,
        Apply: apply,
        Analyze: vi.fn(async () => result({
          preview: [{
            id: 'row-issue', date: '2026-08-01', row: 8, storeLabel: 'Unknown', profileLabel: '',
            currentL: '', currentAB: '', proposedL: '', proposedAB: '', status: 'issue', message: 'store_not_mapped',
          }],
          changeCount: 0, issueCount: 1, problemCount: 1, canApply: false,
        })),
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByText(/還有 1 個問題/)).toBeInTheDocument());
    expect(container.textContent).not.toContain('重試失敗項目');
    expect(button(container, '另存並寫入')).toHaveAttribute('disabled');

    const partialCheckbox = container.querySelector('md-checkbox')!;
    await fireEvent.click(partialCheckbox);
    expect(partialCheckbox).toHaveAttribute('checked', 'true');
    expect(button(container, '另存並寫入')).toHaveAttribute('disabled', 'false');
    await fireEvent.click(button(container, '另存並寫入'));

    expect(screen.getByRole('heading', { name: '確認部分寫入' })).toBeInTheDocument();
    expect(screen.getByText('問題列會保留原值，只寫入其餘列。')).toBeInTheDocument();
    await fireEvent.click(container.querySelector('.app-dialog md-filled-button')!);
    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
    expect(saveWorkbook).toHaveBeenCalledTimes(1);
    expect(apply).toHaveBeenCalledWith(expect.objectContaining({
      operationId: 'operation-1', inputPath: 'D:\\sales.xlsx', outputPath: 'D:\\filled.xlsx',
      allowPartial: true, keepIssueOriginal: true,
    }));
  });

  it('keeps cancel disabled until the backend provides a real operation id', async () => {
    let progressListener: ((payload: unknown) => void) | undefined;
    const cancel = vi.fn(async () => undefined);
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(() => new Promise<AnalysisResult>(() => undefined)),
        Cancel: cancel,
      },
      events: {
        on(name, listener) {
          if (name === 'rta:progress') progressListener = listener;
          return () => undefined;
        },
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    const cancelButton = button(container, '取消分析');
    expect(cancelButton).toHaveAttribute('disabled');

    progressListener?.({
      operationId: 'operation-live', stage: 'query', current: 14, total: 30,
      storeId: '107', date: '2026-08-07', profile: 'Production', attempt: 2, status: 'success',
    } satisfies AnalysisProgress);
    await waitFor(() => expect(cancelButton).toHaveAttribute('disabled', 'false'));
    expect(screen.getByText('47%')).toBeInTheDocument();
    expect(screen.getByText('門店 107 · 2026-08-07')).toBeInTheDocument();
    expect(screen.getByText('帳號 Production')).toBeInTheDocument();
    expect(screen.getByText('查詢成功')).toBeInTheDocument();
    expect(screen.getByText('嘗試 2 次')).toBeInTheDocument();
    expect(screen.getByText('剩餘').parentElement).toHaveTextContent('16');
    expect(screen.getByText('總計').parentElement).toHaveTextContent('30');
    await fireEvent.click(cancelButton);

    expect(cancel).toHaveBeenCalledWith({ operationId: 'operation-live' });
  });

  it('never allows partial save for an incomplete plan', async () => {
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(async () => result({ complete: false, issueCount: 1, problemCount: 1, canApply: false })),
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByText('分析仍有未完成的工作。請先重試可重試項目，完成後才能另存。')).toBeInTheDocument());

    expect(container.querySelector('md-checkbox')).toHaveAttribute('disabled', 'true');
    expect(button(container, '另存並寫入')).toHaveAttribute('disabled', 'true');
  });

  it('blocks aggregate backend problems even when preview rows have no issues', async () => {
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(async () => result({
          complete: true, issueCount: 0, problemCount: 1, aggregateProblemCount: 1, canApply: false,
        })),
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByText('分析回報 1 個阻擋問題，必須先處理才能另存。')).toBeInTheDocument());

    expect(button(container, '另存並寫入')).toHaveAttribute('disabled', 'true');
  });

  it('locks workbook changes during retry and allows cancelling that operation', async () => {
    let resolveRetry!: (value: AnalysisResult) => void;
    const retry = vi.fn(() => new Promise<AnalysisResult>((resolve) => { resolveRetry = resolve; }));
    const cancel = vi.fn(async () => undefined);
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(async () => result({
          complete: true,
          retryableCount: 1,
          problemCount: 1,
          aggregateProblemCount: 0,
          failedCount: 1,
          canApply: false,
          preview: [{
            id: 'row-failed', date: '2026-08-01', row: 4, storeLabel: 'Store 1', profileLabel: 'Primary',
            currentL: '', currentAB: '', proposedL: '', proposedAB: '', status: 'failed', message: 'upstream_error',
          }],
        })),
        RetryFailed: retry,
        Cancel: cancel,
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(container.textContent).toContain('重試失敗項目'));
    await fireEvent.click(button(container, '重試失敗項目'));

    await waitFor(() => expect(retry).toHaveBeenCalledWith({ operationId: 'operation-1' }));
    expect(button(container, '更換檔案')).toHaveAttribute('disabled', 'true');
    expect(container.textContent).not.toContain('修改範圍');
    const cancelButton = button(container, '取消分析');
    expect(cancelButton).toHaveAttribute('disabled', 'false');
    await fireEvent.click(cancelButton);
    expect(cancel).toHaveBeenCalledWith({ operationId: 'operation-1' });

    resolveRetry(result());
    await waitFor(() => expect(screen.getByRole('heading', { name: '檢查分析結果', level: 2 })).toBeInTheDocument());
  });

  it('labels retryable query failures separately from permission issues', async () => {
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(async () => result({
          problemCount: 2,
          issueCount: 1,
          failedCount: 2,
          retryableCount: 2,
          canApply: false,
          preview: [
            {
              id: 'row-permission', date: '2026-08-01', row: 4, storeLabel: 'Store 1', profileLabel: 'Primary',
              currentL: '', currentAB: '', proposedL: '', proposedAB: '', status: 'issue', message: 'store_not_authorized',
            },
            {
              id: 'row-failed', date: '2026-08-02', row: 5, storeLabel: 'Store 2', profileLabel: 'Primary',
              currentL: '', currentAB: '', proposedL: '', proposedAB: '', status: 'failed', message: 'upstream_error',
            },
            {
              id: 'row-query-failed', date: '2026-08-03', row: 6, storeLabel: 'Store 3', profileLabel: 'Primary',
              currentL: '', currentAB: '', proposedL: '', proposedAB: '', status: 'failed', message: 'query_failed',
            },
          ],
        })),
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '檢查分析結果' })).toBeInTheDocument());

    expect(container.textContent).toContain('查詢失敗（可重試）');
    expect(container.textContent).toContain('RTA 服務暫時無法回應，請稍後重試。');
    expect(container.textContent).toContain('資料查詢失敗，請稍後重試。');
    expect(container.textContent).toContain('權限問題');
    expect(container.textContent).toContain('此帳號沒有這間門市的權限。');
    expect(translator('zh-TW')('excel.failedCount', { count: 2 })).toBe('2 個查詢失敗（可重試）');
  });

  it('opens or reveals the saved workbook from the success card', async () => {
    const openSavedWorkbook = vi.fn(async () => undefined);
    const revealSavedWorkbook = vi.fn(async () => undefined);
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(async () => result()),
        SaveWorkbook: vi.fn(async () => 'D:\\reports\\filled.xlsx'),
        Apply: vi.fn(async () => ({ outputPath: 'D:\\reports\\filled.xlsx', changedCells: 2, skippedRows: 0 })),
        OpenSavedWorkbook: openSavedWorkbook,
        RevealSavedWorkbook: revealSavedWorkbook,
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '檢查分析結果' })).toBeInTheDocument());
    await fireEvent.click(button(container, '另存並寫入'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '已建立新檔案' })).toBeInTheDocument());
    expect(screen.getByText('檔案名稱')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'filled.xlsx' })).toBeInTheDocument();
    expect(screen.getByText('位置')).toBeInTheDocument();
    expect(screen.getByText('D:\\reports')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'filled.xlsx' }));
    await waitFor(() => expect(openSavedWorkbook).toHaveBeenCalledWith({ path: 'D:\\reports\\filled.xlsx' }));
    await fireEvent.click(button(container, '開啟檔案'));
    await waitFor(() => expect(openSavedWorkbook).toHaveBeenCalledTimes(2));
    await fireEvent.click(button(container, '在資料夾中顯示'));
    await waitFor(() => expect(revealSavedWorkbook).toHaveBeenCalledWith({ path: 'D:\\reports\\filled.xlsx' }));
  });

  it('does not infer retry work when the backend explicitly returns zero', async () => {
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        Analyze: vi.fn(async () => result({
          retryableCount: 0,
          issues: [{ row: 4, message: 'upstream_error', retryable: true }],
        })),
      },
    });
    const { container } = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn() },
    });
    await openAndScan(container);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '檢查分析結果', level: 2 })).toBeInTheDocument());
    expect(container.textContent).not.toContain('重試失敗項目');
  });

  it('prompts a rescan after the account catalog changes', async () => {
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
      },
    });
    const view = render(ExcelPage, {
      props: { t: translator('zh-TW'), settings: defaultSettings, onGoToAccounts: vi.fn(), catalogEpoch: 0 },
    });
    await openAndScan(view.container);
    expect(screen.queryByText('若剛新增或啟用帳號，請重新掃描活頁簿。')).not.toBeInTheDocument();
    await view.rerender({
      t: translator('zh-TW'),
      settings: defaultSettings,
      onGoToAccounts: vi.fn(),
      catalogEpoch: 1,
    });
    expect(screen.getByText('若剛新增或啟用帳號，請重新掃描活頁簿。')).toBeInTheDocument();
  });
});
