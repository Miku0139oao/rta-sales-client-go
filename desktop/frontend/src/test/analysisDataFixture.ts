import type { SalesAnalysisItem, SalesAnalysisResult, SalesAnalysisTotals } from '../lib/types';
export function dataFixture(): SalesAnalysisResult {
 const item=(storeId:string,articleCode:string,articleName:string,amount:number):SalesAnalysisItem=>({storeId,storeLabel:`${storeId} Store`,articleCode,articleName,brandName:'Brand',category1:'Health',category2:'Beauty',category2Code:'A02',category3:'Skin',category4:'Face',category5:articleName,transactionCount:1,saleQuantity:amount/10,saleAmount:amount,returnQuantity:0,returnTransactionCount:0,returnAmount:0,netQuantity:amount/10,netSalesAmount:amount});
 const items=[item('107','00107','Mask',40),item('108','00107','Mask',20),item('107','00002','Wipes',80)];
 const sum=(rows:SalesAnalysisItem[]):SalesAnalysisTotals=>rows.reduce((total,row)=>({saleQuantity:total.saleQuantity+row.saleQuantity,saleAmount:total.saleAmount+row.saleAmount,returnQuantity:0,returnAmount:0,netQuantity:total.netQuantity+row.netQuantity,netSalesAmount:total.netSalesAmount+row.netSalesAmount}),{saleQuantity:0,saleAmount:0,returnQuantity:0,returnAmount:0,netQuantity:0,netSalesAmount:0});
 const periods=['current','previous'].map((key,index)=>{
  const rows=items.map(row=>({...row,netSalesAmount:row.netSalesAmount/(index+1),saleAmount:row.saleAmount/(index+1),netQuantity:row.netQuantity/(index+1),saleQuantity:row.saleQuantity/(index+1)}));
  return {key,label:index?'上期':'本期',from:index?'2026-07-01':'2026-08-01',to:index?'2026-07-31':'2026-08-31',complete:true,successfulStores:3,totals:sum(rows),stores:['107','108','109'].map(id=>({businessId:id,label:`${id} Store`,totals:sum(rows.filter(row=>row.storeId===id))})),items:rows,itemCount:rows.length};
 });
 return {operationId:'data-test',from:'2026-08-01',to:'2026-08-31',complete:true,pending:false,selectedStores:3,successfulStores:3,totals:periods[0]!.totals,stores:periods[0]!.stores,periods,weeks:[],queryDurationMs:10};
}
