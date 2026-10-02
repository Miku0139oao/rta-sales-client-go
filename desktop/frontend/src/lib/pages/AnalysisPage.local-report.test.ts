import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { browserReportStatus } from '../browserReportCache';
import { dataFixture } from '../../test/analysisDataFixture';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
vi.mock('../runtime', () => ({ isWebRuntime: () => true }));
import AnalysisPage from './AnalysisPage.svelte';
afterEach(() => {
  cleanup();
  configureBackend(undefined);
  browserReportStatus.set({});
  localStorage.clear();
});

it('reads a saved web report without contacting RTA, keeps its owner and requires confirmation to clear only the local report', async () => {
  const result = dataFixture();
  const stores = vi.fn(async () => {
    throw new Error('should not need live stores');
  });
  const run = vi.fn();
  const clear = vi.fn(async () => undefined);
  configureBackend({
    methods: {
      ListProfiles: async () => [
        { id: 'other', displayName: 'Other', enabled: true, hasCredentials: true, priority: 1 },
        { id: 'original', displayName: 'Original', enabled: true, hasCredentials: true, priority: 2 },
      ],
      ListManCodeGroups: async () => [],
      LoadSalesAnalysisSnapshot: async () => ({ result, profileId: 'original', savedAt: '2026-10-02T01:00:00Z' }),
      ListSalesAnalysisStores: stores,
      RunSalesAnalysis: run,
      ClearLocalReportCache: clear,
    },
  });
  browserReportStatus.set({
    operationId: result.operationId,
    savedAt: '2026-10-02T01:00:00Z',
    storage: 'localStorage',
  });
  render(AnalysisPage, { props: { t: translator('zh-TW'), settings: defaultSettings } });
  await screen.findByText('正在查看本機報表');
  expect(stores).not.toHaveBeenCalled();
  expect(run).not.toHaveBeenCalled();
  expect(screen.getByText(/Original · 2026-08-01/)).toBeInTheDocument();
  await fireEvent.click(screen.getByRole('button', { name: '清除本機報表' }));
  expect(clear).not.toHaveBeenCalled();
  await fireEvent.click(screen.getByText('取消'));
  expect(clear).not.toHaveBeenCalled();
  await fireEvent.click(screen.getByRole('button', { name: '清除本機報表' }));
  await fireEvent.click(screen.getByText('確定清除報表'));
  await waitFor(() => expect(clear).toHaveBeenCalledTimes(1));
  expect(screen.queryByText('正在查看本機報表')).not.toBeInTheDocument();
  expect(stores).not.toHaveBeenCalled();
});
