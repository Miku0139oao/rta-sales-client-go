import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import type { AnalysisWorkbookRequest } from '../analysisTable';
import {
  EXPORT_FIXTURE_STORE_ID,
  SMALL_EXPORT_GROUP_CODES,
  SMALL_EXPORT_GROUP_ID,
  SMALL_EXPORT_GROUP_NAME,
  SMALL_EXPORT_STORE_LABEL,
  smallExportResult,
} from '../sales-report-export-fixture';
import { FOCUS_SCREEN_LIMIT, SMALL_EXPORT_EXPECTED } from '../sales-report-export-expected';

vi.mock('../sales-report-pdf', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../sales-report-pdf')>();
  return {
    ...actual,
    generateSalesAnalysisPDF: vi.fn(async () => new TextEncoder().encode('%PDF-1.7\nmock report')),
    prepareSalesAnalysisFont: vi.fn(async () => 'Zm9udA=='),
    prepareSalesAnalysisFontFromText: vi.fn(async () => 'Zm9udA=='),
  };
});
vi.mock('../runtime', () => ({ isWebRuntime: vi.fn(() => false) }));
vi.mock('../webStorage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../webStorage')>();
  return { ...actual, loadWebAnalysisSnapshot: vi.fn(() => null) };
});

import AnalysisPage from './AnalysisPage.svelte';

const expected = SMALL_EXPORT_EXPECTED;
const report = smallExportResult();

function listCodes(section: HTMLElement) {
  return [...section.querySelectorAll('li span')]
    .map((node) => (node.textContent ?? '').trim())
    .map((text) => text.match(/^(0\d{6})\b/)?.[1])
    .filter((code): code is string => Boolean(code));
}

async function startPage(options: { groups?: boolean } = {}) {
  const exportFile = vi.fn(async (_request: unknown) => 'report.xlsx');
  configureBackend({
    methods: {
      ListProfiles: vi.fn(async () => [{
        id: 'profile-1', displayName: 'Production', enabled: true, priority: 1, hasCredentials: true,
      }]),
      ListSalesAnalysisStores: vi.fn(async () => [{ businessId: EXPORT_FIXTURE_STORE_ID, label: SMALL_EXPORT_STORE_LABEL }]),
      ListManCodeGroups: vi.fn(async () => options.groups ? [{
        id: SMALL_EXPORT_GROUP_ID, name: SMALL_EXPORT_GROUP_NAME, codes: [...SMALL_EXPORT_GROUP_CODES],
      }] : []),
      RunSalesAnalysis: vi.fn(async () => report),
      ClearSalesAnalysis: vi.fn(async () => undefined),
      ExportSalesAnalysisWorkbook: exportFile,
    },
  });
  render(AnalysisPage, {
    props: { t: translator('zh-TW'), settings: { ...defaultSettings, rankingLimit: 16 } },
  });
  await waitFor(() => expect(screen.getByText(SMALL_EXPORT_STORE_LABEL)).toBeInTheDocument());
  await fireEvent.click(screen.getByText('開始分析'));
  await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 16' })).toBeInTheDocument());
  return { exportFile };
}

beforeEach(() => {
  configureBackend(undefined);
});
afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('AnalysisPage export parity snapshots', () => {
  it('captures screen performance, rankings, Focus 10, filters/groups and Excel comparison cells', async () => {
    const { exportFile } = await startPage();
    const money = expected.formats.screenMoney;
    const number = expected.formats.screenNumber;

    expect(screen.getByRole('heading', { name: '銷售表現' }).closest('section')).toHaveTextContent(money(expected.current.netSalesAmount));
    const performance = screen.getByRole('heading', { name: '銷售表現' }).closest('section');
    expect(performance).toBeTruthy();
    expect(performance).toHaveTextContent(money(expected.previous.netSalesAmount));
    expect(performance).toHaveTextContent(money(expected.yearAgo.netSalesAmount));
    expect(performance).toHaveTextContent(expected.formats.screenPercent(expected.current.netSalesAmount, expected.previous.netSalesAmount));
    expect(performance).toHaveTextContent(expected.formats.screenPercent(expected.current.netSalesAmount, expected.yearAgo.netSalesAmount));
    const netRow = [...(performance as HTMLElement).querySelectorAll('tr')].find((row) => row.textContent?.includes('淨銷售額'));
    expect(netRow).toHaveTextContent(money(expected.previous.netSalesAmount));
    expect(netRow).toHaveTextContent(money(expected.yearAgo.netSalesAmount));
    expect(netRow?.textContent).not.toMatch(/淨銷售額[^\d]*HK\$0\.00/);

    const topSales = screen.getByRole('heading', { name: '銷售額 Top 16' }).closest('section')!;
    const topQuantity = screen.getByRole('heading', { name: '銷量 Top 16' }).closest('section')!;
    expect(listCodes(topSales).slice(0, 16)).toEqual(expected.topSales.map((row) => row.code));
    expect(topSales).toHaveTextContent(money(expected.topSales[0]!.amount));
    expect(topSales).toHaveTextContent(expected.topSales.at(-1)!.code);
    expect(listCodes(topQuantity)[0]).toBe(expected.topQuantity[0]!.code);
    expect(topQuantity).toHaveTextContent(`${number(expected.topQuantity[0]!.quantity)} 件`);

    await fireEvent.click(screen.getByRole('tab', { name: '關注' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: '接下來關注' })).toBeInTheDocument());
    const health = screen.getByText('保健').closest('article')!;
    const healthCodes = listCodes(health.querySelectorAll('ol')[0] as HTMLElement);
    expect(healthCodes).toEqual(expected.focusScreenHealth.map((row) => row.code));
    expect(healthCodes.at(-1)).toBe('0100010');

    await fireEvent.click(screen.getByRole('tab', { name: '概覽' }));
    await fireEvent.click(screen.getByRole('button', { name: '匯出此頁 Excel' }));
    await waitFor(() => expect(exportFile).toHaveBeenCalledTimes(1));
    const snapshot = exportFile.mock.calls[0]![0] as AnalysisWorkbookRequest;
    const performanceSheet = snapshot.sheets.find((sheet) => sheet.name.includes('銷售表現'));
    const salesSheet = snapshot.sheets.find((sheet) => sheet.name.includes('銷售額 Top'));
    expect(performanceSheet?.rows.find((row) => row[0] === '淨銷售額')?.slice(1, 4)).toEqual([
      expected.current.netSalesAmount, expected.previous.netSalesAmount, expected.yearAgo.netSalesAmount,
    ]);
    expect(salesSheet?.rows.map((row) => row[1])).toEqual(expected.topSales.map((row) => row.code));
    expect(salesSheet?.rows.at(-1)?.[1]).toBe('0100012');
    expect(salesSheet?.rows[0]?.[3]).toBe(2000);

    await fireEvent.click(screen.getByRole('button', { name: '篩選' }));
    await fireEvent.click(screen.getAllByText('商品部門')[0]!);
    const healthFilter = await screen.findByText(/A01\s+保健護理/);
    await fireEvent.click(healthFilter.closest('label') ?? healthFilter);
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 16' }).closest('section')).toHaveTextContent('H01'));
    const filteredSales = screen.getByRole('heading', { name: '銷售額 Top 16' }).closest('section')!;
    expect(listCodes(filteredSales).slice(0, 12)).toEqual(expected.filterHealth.topSales.map((row) => row.code));
    expect(filteredSales).not.toHaveTextContent('S01');
    expect(screen.getByRole('heading', { name: '銷售表現' }).closest('section')).toHaveTextContent(money(expected.filterHealth.netSalesAmount));
  });

  it('scopes screen rankings and period comparisons to a promoter group', async () => {
    await startPage({ groups: true });
    const money = expected.formats.screenMoney;
    const group = await screen.findByLabelText('商品範圍');
    await fireEvent.change(group, { target: { value: SMALL_EXPORT_GROUP_ID } });
    await waitFor(() => expect(screen.getByRole('heading', { name: '銷售額 Top 16' }).closest('section')).toHaveTextContent('S01'));
    const groupedSales = screen.getByRole('heading', { name: '銷售額 Top 16' }).closest('section')!;
    expect(listCodes(groupedSales).slice(0, 2)).toEqual(expected.groupPair.topSales.map((row) => row.code));
    expect(groupedSales).not.toHaveTextContent('H01');
    expect(screen.getByRole('heading', { name: '銷售表現' }).closest('section')).toHaveTextContent(money(expected.groupPair.netSalesAmount));
    expect(screen.getByRole('heading', { name: '銷售表現' }).closest('section')).toHaveTextContent(money(expected.groupPair.previousNetSalesAmount));
  });
});
