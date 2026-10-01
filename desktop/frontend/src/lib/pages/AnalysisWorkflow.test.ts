import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/svelte";
import { beforeEach, afterEach, it, expect, vi } from "vitest";
import { unzipSync } from "fflate";
import { configureBackend } from "../backend";
import { saveAnalysisPresets, type AnalysisPreset } from "../analysisPresets";
import { translator } from "../i18n";
import { defaultSettings } from "../settings";
import { dataFixture } from "../../test/analysisDataFixture";
import AnalysisPage from "./AnalysisPage.svelte";
beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  configureBackend(undefined);
});
async function setup(complete = true, pending = false) {
  const listeners = new Map<string,(payload:unknown)=>void>();
  const report = {
    ...dataFixture(),
    complete: complete && !pending,
    pending,
    issues: complete
      ? []
      : [
          {
            periodKey: "current",
            storeId: "107",
            storeLabel: "107 Store",
            message: "temporary failure",
          },
        ],
  };
  const preset: AnalysisPreset = {
    id: "recipe",
    name: "Daily",
    query: {
      profileId: "profile",
      profileName: "Test",
      periodMode: "month",
      monthMode: "fixed",
      month: "2026-08",
      from: "2026-08-01",
      to: "2026-08-31",
      weekCompare: false,
      storeIds: report.stores.map((store) => store.businessId),
    },
    filters: {
      search: "",
      groupId: "",
      groupLevel: "category1",
      categories: {
        category1: [],
        category2: [],
        category3: [],
        category4: [],
        category5: [],
      },
    },
    workflow: { pdf: false, excel: true, ai: false, perStore: false },
  };
  saveAnalysisPresets([preset]);
  const build = vi.fn(async () => btoa("xlsx bytes")),
    write = vi.fn(async (_request: unknown) => "reports.zip"),
    choose = vi.fn(async () => "D:\\reports"),
    run = vi.fn(async () => report);
  configureBackend({
    events:{on(name,listener){listeners.set(name,listener);return ()=>listeners.delete(name);}},
    methods: {
      ListProfiles: vi.fn(async () => [
        {
          id: "profile",
          displayName: "Test",
          enabled: true,
          priority: 1,
          hasCredentials: true,
        },
      ]),
      ListSalesAnalysisStores: vi.fn(async () => report.stores),
      ListManCodeGroups: vi.fn(async () => []),
      RunSalesAnalysis: run,
      ClearSalesAnalysis: vi.fn(async () => undefined),
      CancelSalesAnalysis: vi.fn(async () => undefined),
      BuildSalesAnalysisWorkbook: build,
      WriteReportBundle: write,
      ChooseSalesAnalysisPDFDirectory: choose,
      BeginNativeExportLease: vi.fn(async () => "lease"),
      EndNativeExportLease: vi.fn(async () => undefined),
    },
  });
  render(AnalysisPage, {
    props: { t: translator("zh-TW"), settings: defaultSettings },
  });
  await screen.findByText(`${report.stores[0]!.businessId} Store`);
  await fireEvent.click(screen.getByRole("button", { name: "常用條件" }));
  await screen.findByRole("dialog");
  await fireEvent.click(screen.getByRole("button", { name: "一鍵查詢並匯出" }));
  return { build, write, choose, run, finish:()=>listeners.get('rta:sales-analysis-update')?.({...report,pending:false,complete:true}) };
}
it("queries and exports the saved recipe with one directory selection and a valid ZIP", async () => {
  const { write, choose, run } = await setup();
  await waitFor(() => expect(write).toHaveBeenCalledTimes(1));
  expect(choose).toHaveBeenCalledTimes(1);
  expect(run).toHaveBeenCalledTimes(1);
  const request = write.mock.calls[0]![0] as unknown as { dataBase64: string };
  const files = unzipSync(
    Uint8Array.from(atob(request.dataBase64), (char) => char.charCodeAt(0)),
  );
  expect(Object.keys(files)).toEqual(["RTA-2026-08-01-2026-08-31.xlsx"]);
  expect(new TextDecoder().decode(Object.values(files)[0])).toBe("xlsx bytes");
});
it("stops before export when query results are incomplete", async () => {
  const { build, choose, write } = await setup(false);
  await screen.findByText(/查詢不完整，請先重試/);
  expect(build).not.toHaveBeenCalled();
  expect(choose).not.toHaveBeenCalled();
  expect(write).not.toHaveBeenCalled();
});
it('waits for background comparison work before choosing a folder or building reports',async()=>{
  const {build,write,choose,finish}=await setup(true,true);
  await screen.findByText('比較資料補齊中');
  expect(build).not.toHaveBeenCalled();expect(choose).not.toHaveBeenCalled();
  finish();await waitFor(()=>expect(write).toHaveBeenCalledTimes(1));
});
