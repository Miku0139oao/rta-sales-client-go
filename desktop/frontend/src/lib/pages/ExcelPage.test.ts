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
    await fireEvent.click(button(container, '分析並建立預覽'));
    await waitFor(() => expect(screen.getByText('帳號授權範圍重疊')).toBeInTheDocument());

    expect(analyze).toHaveBeenCalledTimes(1);
    expect(analyze.mock.calls[0]?.[0]).toMatchObject({ overwrite: true, maxJobs: 2000 });
    expect(screen.getAllByText('允許覆寫不同值').length).toBeGreaterThan(0);
  });

  it('blocks issue rows until partial mode and keep-original are both selected, then asks again', async () => {
    const apply = vi.fn(async (_request: unknown) => ({
      outputPath: 'D:\\filled.xlsx', changedCells: 2, skippedRows: 1,
    }));
    configureBackend({
      methods: {
        OpenWorkbook: vi.fn(async () => 'D:\\sales.xlsx'),
        ScanWorkbook: vi.fn(async () => scan),
        SaveWorkbook: vi.fn(async () => 'D:\\filled.xlsx'),
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
    await fireEvent.click(button(container, '分析並建立預覽'));
    await waitFor(() => expect(screen.getByText(/目前有 1 個問題/)).toBeInTheDocument());
    expect(container.textContent).not.toContain('重試失敗項目');
    expect(button(container, '另存新檔')).toHaveAttribute('disabled');

    await fireEvent.click(container.querySelectorAll('md-switch')[1]);
    await fireEvent.click(screen.getByRole('checkbox', { name: '問題列保持原值' }));
    expect(button(container, '另存新檔')).toHaveAttribute('disabled', 'false');
    await fireEvent.click(button(container, '另存新檔'));

    expect(screen.getByRole('heading', { name: '確認部分寫入' })).toBeInTheDocument();
    expect(screen.getByText('問題列將保持原值，其餘可用資料會寫入新檔。')).toBeInTheDocument();
    await fireEvent.click(container.querySelector('.app-dialog md-filled-button')!);
    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
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
    await fireEvent.click(button(container, '分析並建立預覽'));
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
    await fireEvent.click(button(container, '分析並建立預覽'));
    await waitFor(() => expect(screen.getByText('分析仍有未完成的工作。請先重試可重試項目，完成後才能另存。')).toBeInTheDocument());

    expect(container.querySelectorAll('md-switch')[1]).toHaveAttribute('disabled', 'true');
    expect(button(container, '另存新檔')).toHaveAttribute('disabled', 'true');
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
    await fireEvent.click(button(container, '分析並建立預覽'));
    await waitFor(() => expect(screen.getByText('分析回報 1 個阻擋問題，必須先處理才能另存。')).toBeInTheDocument());

    expect(button(container, '另存新檔')).toHaveAttribute('disabled', 'true');
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
    await fireEvent.click(button(container, '分析並建立預覽'));
    await waitFor(() => expect(container.textContent).toContain('重試失敗項目'));
    await fireEvent.click(button(container, '重試失敗項目'));

    await waitFor(() => expect(retry).toHaveBeenCalledWith({ operationId: 'operation-1' }));
    expect(button(container, '更換檔案')).toHaveAttribute('disabled', 'true');
    expect(button(container, '重新掃描')).toHaveAttribute('disabled', 'true');
    const cancelButton = button(container, '取消分析');
    expect(cancelButton).toHaveAttribute('disabled', 'false');
    await fireEvent.click(cancelButton);
    expect(cancel).toHaveBeenCalledWith({ operationId: 'operation-1' });

    resolveRetry(result());
    await waitFor(() => expect(screen.getByRole('heading', { name: '寫入前預覽', level: 2 })).toBeInTheDocument());
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
    await fireEvent.click(button(container, '分析並建立預覽'));
    await waitFor(() => expect(screen.getByRole('heading', { name: '寫入前預覽', level: 2 })).toBeInTheDocument());
    expect(container.textContent).not.toContain('重試失敗項目');
  });
});
