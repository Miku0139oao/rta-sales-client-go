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

  it('confirms overwrite before the single analysis and keeps the preview policy read-only', async () => {
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
    await fireEvent.click(container.querySelectorAll('md-switch')[0]);
    expect(screen.getByText('分析可能覆寫 L／AB 欄位中已存在的不同值。此選擇在本次分析後無法變更。')).toBeInTheDocument();
    await fireEvent.click(container.querySelector('.app-dialog md-filled-button')!);
    await fireEvent.click(button(container, '開始分析'));
    await waitFor(() => expect(screen.getByText('帳號授權範圍重疊')).toBeInTheDocument());

    expect(analyze).toHaveBeenCalledTimes(1);
    expect(analyze.mock.calls[0]?.[0]).toMatchObject({ overwrite: true, maxJobs: 2000 });
    expect(screen.getAllByText('允許覆寫不同值').length).toBeGreaterThan(0);
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

    await fireEvent.click(container.querySelector('md-switch')!);
    expect(screen.queryByRole('checkbox', { name: '問題列保持原值' })).not.toBeInTheDocument();
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

    progressListener?.({ operationId: 'operation-live', stage: 'query', current: 1, total: 2 } satisfies AnalysisProgress);
    await waitFor(() => expect(cancelButton).toHaveAttribute('disabled', 'false'));
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

    expect(container.querySelector('md-switch')).toHaveAttribute('disabled', 'true');
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
});
