import { describe,expect,it } from 'vitest';
import { dataFixture } from '../test/analysisDataFixture';
import { translator } from './i18n';
import { exportSortFromView, productDetailHero, productDetailTables, productDetailViewTables } from './productDetails';
const t=translator('zh-TW');
describe('product details',()=>{
 it('aggregates one code across stores and calculates current versus previous',()=>{
  const tables=productDetailTables('00107',dataFixture().periods!,'current',t);
  expect(tables[0]!.rows[0]!.cells.slice(3,7)).toEqual([60,6,0,1]);
  expect(tables[1]!.rows.map(row=>row.cells[2])).toEqual([40,20,0]);
 });
 it('switches store totals by period without mutating report data',()=>{
  const source=dataFixture();const tables=productDetailTables('00107',source.periods!,'previous',t);
  expect(tables[1]!.rows.map(row=>row.cells[2])).toEqual([20,10,0]);expect(source.periods![0]!.items![0]!.netSalesAmount).toBe(40);
 });
 it('shows missing details as unknown, never a zero or a misleading growth rate',()=>{
  const source=dataFixture();source.periods![1]!.items=[];
  const tables=productDetailTables('00107',source.periods!,'previous',t);
  expect(tables[0]!.rows[1]!.cells[3]).toBeNull();expect(tables[0]!.rows[0]!.cells[6]).toBeNull();expect(tables[1]!.rows.every(row=>row.cells[2]===null)).toBe(true);
 });
 it('distinguishes a failed store from a queried store with no sales',()=>{
  const source=dataFixture();source.periods![0]!.complete=false;source.periods![0]!.issues=[{storeId:'109',storeLabel:'109 Store',message:'failed'}];
  const tables=productDetailTables('00107',source.periods!,'current',t);
  expect(tables[1]!.rows[2]!.cells[2]).toBeNull();expect(tables[1]!.rows[2]!.cells[6]).toBe('門店資料不可用');expect(tables[0]!.rows[0]!.cells[6]).toBeNull();
 });
 it('does not invent zero totals for an absent product in an incomplete period',()=>{
  const source=dataFixture();source.periods![0]!.complete=false;
  expect(productDetailTables('absent',source.periods!,'current',t)[0]!.rows[0]!.cells[3]).toBeNull();
 });
 it('puts money and quantity before dates on screen without changing export columns',()=>{
  const tables=productDetailTables('00107',dataFixture().periods!,'current',t);
  const view=productDetailViewTables(tables);
  expect(tables[0]!.columns.map(column=>column.label)).toEqual(['比較期間','開始日期','結束日期','淨銷售額','淨銷售數量','退款金額','較上期','資料狀態']);
  expect(view[0]!.columns.map(column=>column.label)).toEqual(['比較期間','淨銷售額','淨銷售數量','較上期','開始日期','結束日期','退款金額','資料狀態']);
  expect(view[0]!.rows[0]!.cells.slice(0,4)).toEqual(['本期',60,6,1]);
  expect(view[1]!.columns.map(column=>column.label)).toEqual(['門店','淨銷售額','淨銷售數量','門店名稱','銷售金額','退款金額','資料狀態']);
  expect(view[1]!.rows.map(row=>row.cells[1])).toEqual([40,20,0]);
  expect(exportSortFromView('product-periods',{column:1,direction:'descending'})).toEqual({column:3,direction:'descending'});
  expect(exportSortFromView('product-stores',{column:1,direction:'ascending'})).toEqual({column:2,direction:'ascending'});
 });
 it('handles a large multi-store detail without repeatedly scanning all rows per store',()=>{
  const current=dataFixture().periods![0]!;
  current.stores=Array.from({length:200},(_,index)=>({businessId:String(index+100),label:`Store ${index+100}`,totals:current.totals}));
  current.successfulStores=200;
  current.items=Array.from({length:100000},(_,index)=>({...current.items![0]!,storeId:String(100+Math.floor(index/500)),articleCode:index%500===0?'00107':'other',netSalesAmount:1,netQuantity:1}));
  current.itemCount=current.items.length;
  const hero=productDetailHero('00107',[current],'current',t);
  expect(hero).toMatchObject({amount:200,quantity:200,storesWithSales:200,storesTotal:200});
  expect(productDetailTables('00107',[current],'current',t)[1]!.rows).toHaveLength(200);
 });
 it('keeps the store-sales count unknown for a hydrated but incomplete selected period',()=>{
  const source=dataFixture();source.periods![1]!.complete=false;
  source.periods![1]!.issues=[{storeId:'109',storeLabel:'109 Store',message:'failed'}];
  const hero=productDetailHero('00107',source.periods!,'previous',t);
  expect(hero).toMatchObject({amount:60,storesWithSales:null,storesTotal:3,selectedLabel:'上期'});
  source.periods![1]!.issues=[];
  expect(productDetailHero('00107',source.periods!,'previous',t).storesWithSales).toBeNull();
 });
 it('does not claim an exact ratio when a denominator store has no coverage in the selected period',()=>{
  const source=dataFixture();source.periods![0]!.stores=source.periods![0]!.stores.filter(store=>store.businessId!=='109');
  expect(productDetailHero('00107',source.periods!,'current',t).storesWithSales).toBeNull();
 });
 it('summarises identity metrics without inventing zeros for missing periods',()=>{
  const ready=productDetailHero('00107',dataFixture().periods!,'current',t);
  expect(ready).toMatchObject({amount:60,quantity:6,vsPrevious:1,storesWithSales:2,storesTotal:3});
  const source=dataFixture();source.periods![0]!.complete=false;source.periods![0]!.items=[];
  const missing=productDetailHero('00107',source.periods!,'current',t);
  expect(missing.amount).toBeNull();expect(missing.quantity).toBeNull();expect(missing.vsPrevious).toBeNull();expect(missing.storesWithSales).toBeNull();
 });
});
