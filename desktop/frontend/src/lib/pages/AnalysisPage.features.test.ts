import { cleanup,fireEvent,render,screen,waitFor,within } from '@testing-library/svelte';
import { afterEach,beforeEach,describe,expect,it,vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import { dataFixture } from '../../test/analysisDataFixture';
import type { AnalysisWorkbookRequest } from '../analysisTable';
import AnalysisPage from './AnalysisPage.svelte';
beforeEach(()=>{localStorage.clear();});
afterEach(()=>{cleanup();configureBackend(undefined);});
async function setup(){
 const report=dataFixture(),run=vi.fn(async()=>report),exportFile=vi.fn(async(_request:unknown)=>'report.xlsx');
 configureBackend({methods:{ListProfiles:vi.fn(async()=>[{id:'profile',displayName:'Test',enabled:true,priority:1,hasCredentials:true}]),ListSalesAnalysisStores:vi.fn(async()=>report.stores),ListManCodeGroups:vi.fn(async()=>[]),RunSalesAnalysis:run,ClearSalesAnalysis:vi.fn(async()=>undefined),CancelSalesAnalysis:vi.fn(async()=>undefined),ExportSalesAnalysisWorkbook:exportFile}});
 const view=render(AnalysisPage,{props:{t:translator('zh-TW'),settings:defaultSettings}});
 await screen.findByText('107 Store');await fireEvent.click(screen.getByText('開始分析'));await screen.findByRole('heading',{name:'銷售額 Top 24'});
 return {...view,run,exportFile};
}
describe('complete analysis tools',()=>{
 it('sorts products numerically in both directions and exports exactly the filtered order',async()=>{
  const {run,exportFile}=await setup();await fireEvent.click(screen.getByRole('tab',{name:'商品'}));
  const panel=within(screen.getByRole('tabpanel'));
  await fireEvent.click(panel.getByRole('button',{name:/^淨銷售額/}));
  expect(panel.getAllByRole('row')[1]).toHaveTextContent('Wipes');
  expect(panel.getByRole('columnheader',{name:/淨銷售額/})).toHaveAttribute('aria-sort','descending');
  await fireEvent.click(panel.getByRole('button',{name:/^淨銷售額/}));expect(panel.getAllByRole('row')[1]).toHaveTextContent('108');
  await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'),{target:{value:'Mask'}});
  await fireEvent.click(screen.getByRole('button',{name:'匯出此頁 Excel'}));await waitFor(()=>expect(exportFile).toHaveBeenCalledTimes(1));
  const snapshot=exportFile.mock.calls[0]![0] as AnalysisWorkbookRequest;
  expect(snapshot.sheets[0]!.rows.map(row=>row[0])).toEqual(['108','107']);expect(snapshot.sheets[0]!.rows.map(row=>row[8])).toEqual([20,40]);expect(run).toHaveBeenCalledTimes(1);
 });
 it('opens ranking details, switches periods, exports the detail scope, and restores focus without rerunning',async()=>{
  const {run,exportFile}=await setup();const trigger=screen.getAllByRole('button',{name:'查看商品詳情：Mask'})[0]!;trigger.focus();await fireEvent.click(trigger);
  const dialog=within(await screen.findByRole('dialog',{name:'Mask'}));
  expect(dialog.getByRole('heading',{name:'期間銷售比較'})).toBeInTheDocument();expect(dialog.getAllByRole('table')).toHaveLength(2);
  const snapshotRegion=dialog.getByRole('region',{name:'本期銷售摘要'});
  expect(snapshotRegion).toHaveTextContent('淨銷售額');expect(snapshotRegion).toHaveTextContent('60');expect(snapshotRegion).toHaveTextContent('較上期');
  expect(within(dialog.getAllByRole('table')[0]!).getAllByRole('columnheader')[1]).toHaveTextContent('淨銷售額');
  expect(within(dialog.getAllByRole('table')[1]!).getAllByRole('columnheader')[1]).toHaveTextContent('淨銷售額');
  await fireEvent.change(dialog.getByLabelText('門店明細期間'),{target:{value:'previous'}});
  expect(within(dialog.getAllByRole('table')[1]!).getAllByRole('row')[1]).toHaveTextContent('20.00');
  await fireEvent.click(dialog.getByRole('button',{name:'匯出此頁 Excel'}));await waitFor(()=>expect(exportFile).toHaveBeenCalledTimes(1));
  const snapshot=exportFile.mock.calls[0]![0] as AnalysisWorkbookRequest;
  expect(snapshot.sheets).toHaveLength(2);expect(snapshot.sheets[1]!.rows.map(row=>row[2])).toEqual([20,10,0]);expect(snapshot.context).toContain('商品編碼: 00107');
  await waitFor(()=>expect(dialog.getAllByRole('button',{name:'關閉'})[0]).not.toBeDisabled());await fireEvent(screen.getByRole('dialog'),new Event('cancel',{cancelable:true}));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();expect(trigger).toHaveFocus();expect(run).toHaveBeenCalledTimes(1);
 });
 it('traces a filtered highlight without rerunning and includes its evidence in Excel',async()=>{
  const {run,exportFile}=await setup();
  const insights=within(screen.getByRole('region',{name:'分析重點'}));
  expect(insights.getByRole('heading',{name:'淨銷售成長最多'})).toBeInTheDocument();
  await fireEvent.input(screen.getByLabelText('搜尋商品或編碼'),{target:{value:'00107'}});
  const trigger=insights.getByRole('button',{name:'追查分析重點：Mask'});trigger.focus();await fireEvent.click(trigger);
  await screen.findByRole('dialog',{name:'Mask'});
  await fireEvent(screen.getByRole('dialog'),new Event('cancel',{cancelable:true}));expect(trigger).toHaveFocus();
  expect(screen.getByLabelText('搜尋商品或編碼')).toHaveValue('00107');
  await fireEvent.click(screen.getByRole('button',{name:'匯出此頁 Excel'}));await waitFor(()=>expect(exportFile).toHaveBeenCalledTimes(1));
  const request=exportFile.mock.calls[0]![0] as AnalysisWorkbookRequest;
  const evidence=request.sheets.find(sheet=>sheet.name==='分析重點');
  expect(evidence?.rows[0]?.slice(1,7)).toEqual(['00107','Mask',60,30,30,1]);expect(run).toHaveBeenCalledTimes(1);
 });
 it('can open the same details from the products table',async()=>{
  const {run}=await setup();await fireEvent.click(screen.getByRole('tab',{name:'商品'}));await fireEvent.click(within(screen.getByRole('tabpanel')).getAllByRole('button',{name:'查看商品詳情：Mask'})[0]!);await screen.findByRole('dialog',{name:'Mask'});expect(run).toHaveBeenCalledTimes(1);
 });
});
