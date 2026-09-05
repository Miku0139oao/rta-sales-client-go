import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import type { AnalysisTable } from '../analysisTable';
import AnalysisDataActions from './AnalysisDataActions.svelte';
const table = (): AnalysisTable => ({id:'data',name:'Products',columns:[{label:'Code',format:'text'},{label:'Amount',format:'money'}],rows:Array.from({length:61},(_,index)=>({cells:[`00${index}`,index]}))});
afterEach(()=>{cleanup();configureBackend(undefined);vi.unstubAllGlobals();});
function setup(exportFile=vi.fn(async(_request:unknown)=>'D:\\Reports\\file.xlsx')){
 configureBackend({methods:{ExportSalesAnalysisWorkbook:exportFile}});
 const view=render(AnalysisDataActions,{props:{t:translator('zh-TW'),tables:[table()],context:['Production','2026-08'],filename:'file.xlsx'}});
 return {...view,exportFile};
}
describe('screen data tools',()=>{
 it('exports all pages as typed values and reports the actual written path',async()=>{
  const {exportFile}=setup();await fireEvent.click(screen.getByRole('button',{name:'匯出此頁 Excel'}));
  await screen.findByText('已匯出：D:\\Reports\\file.xlsx');
  const request=exportFile.mock.calls[0]![0] as {sheets:Array<{rows:unknown[][]}>;context:string[]};
  expect(request.sheets[0]!.rows).toHaveLength(61);expect(request.sheets[0]!.rows[0]).toEqual(['000',0]);expect(request.context).toEqual(['Production','2026-08']);
 });
 it('does not claim success when the directory picker is cancelled',async()=>{
  setup(vi.fn(async()=>''));await fireEvent.click(screen.getByRole('button',{name:'匯出此頁 Excel'}));
  await waitFor(()=>expect(screen.getByRole('button',{name:'匯出此頁 Excel'})).not.toBeDisabled());expect(screen.queryByRole('status')).not.toBeInTheDocument();
 });
 it('surfaces export errors and permits another attempt',async()=>{
  const fail=vi.fn(async()=>{throw new Error('disk');});setup(fail);await fireEvent.click(screen.getByRole('button',{name:'匯出此頁 Excel'}));await screen.findByRole('alert');expect(screen.getByRole('button',{name:'匯出此頁 Excel'})).not.toBeDisabled();
 });
 it('copies every row, not only the visible page',async()=>{
  const writeText=vi.fn(async(_text:string)=>undefined);vi.stubGlobal('navigator',{clipboard:{writeText}});setup();await fireEvent.click(screen.getByRole('button',{name:'複製表格'}));await screen.findByText('已複製 61 筆資料，可貼入 Excel。');expect(writeText.mock.calls[0]![0].split('\r\n')).toHaveLength(62);
 });
 it('offers selected text rather than a false success when clipboard access is denied',async()=>{
  vi.stubGlobal('navigator',{clipboard:{writeText:vi.fn(async()=>{throw new Error('denied');})}});setup();await fireEvent.click(screen.getByRole('button',{name:'複製表格'}));await screen.findByRole('dialog',{name:'手動複製表格'});expect((screen.getByRole('textbox',{name:'表格文字'}) as HTMLTextAreaElement).value).toContain('Code\tAmount');expect(screen.queryByText(/已複製/)).not.toBeInTheDocument();
 });
});
