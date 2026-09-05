import { describe, expect, it } from 'vitest';
import { analysisTableTSV, sortAnalysisTable, workbookSnapshot, type AnalysisTable } from './analysisTable';
const table = (): AnalysisTable => ({ id:'products',name:'商品',columns:[{label:'編碼',format:'text'},{label:'金額',format:'money'}],rows:[{cells:['0012',20]},{cells:['0002',null]},{cells:['0003',-4]},{cells:['0004',20]}] });
describe('analysis table snapshots',()=>{
 it('sorts numerically and stably with missing data last in both directions',()=>{
  const source=table();
  expect(sortAnalysisTable(source,{column:1,direction:'descending'},'zh-TW').rows.map(row=>row.cells[0])).toEqual(['0012','0004','0003','0002']);
  expect(sortAnalysisTable(source,{column:1,direction:'ascending'},'zh-TW').rows.map(row=>row.cells[0])).toEqual(['0003','0012','0004','0002']);
  expect(source.rows[1]!.cells[0]).toBe('0002');
 });
 it('keeps weekly subtotal boundaries while sorting each segment',()=>{
  const source=table(); source.rows=[{cells:['Local A',1],group:'local'},{cells:['Local B',3],group:'local'},{cells:['Subtotal',4],fixed:true},{cells:['Tourist',8],group:'tourist'},{cells:['All',12],fixed:true}];
  expect(sortAnalysisTable(source,{column:1,direction:'descending'},'en').rows.map(row=>row.cells[0])).toEqual(['Local B','Local A','Subtotal','Tourist','All']);
 });
 it('copies all rows, escapes formula-like text and keeps numbers numeric',()=>{
  const source=table(); source.rows=[{cells:['=HYPERLINK("bad")',-4]},{cells:['\t+SUM(A1)',0]},{cells:['line\nname',null]}];
  const text=analysisTableTSV(source);
  expect(text).toContain("'=HYPERLINK"); expect(text).toContain("' +SUM(A1)\t0"); expect(text).toContain('line name\t'); expect(text).toContain('\t-4');
 });
 it('makes a detached numeric workbook snapshot with exact order and zero-padded codes',()=>{
  const source=table(), snapshot=workbookSnapshot([source],['2026-08'], 'report.xlsx');
  source.rows[0]!.cells[0]='changed';
  expect(snapshot.sheets[0]!.rows[0]).toEqual(['0012',20]); expect(snapshot.context).toEqual(['2026-08']);
 });
 it('sorts large reports without argument-spread stack overflow',()=>{
  const source=table();source.rows=Array.from({length:200000},(_,index)=>({cells:[String(index),index]}));
  const sorted=sortAnalysisTable(source,{column:1,direction:'descending'},'en');
  expect(sorted.rows).toHaveLength(200000);expect(sorted.rows[0]!.cells[1]).toBe(199999);expect(source.rows[0]!.cells[1]).toBe(0);
 });
 it('rejects an oversized export before dispatch',()=>{
  const source=table(); source.rows=Array.from({length:250001},()=>({cells:['1',1]}));
  expect(()=>workbookSnapshot([source],[],'huge.xlsx')).toThrow('table_limit');
 });
});
