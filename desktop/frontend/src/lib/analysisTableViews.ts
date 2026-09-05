import type { Translator } from './i18n';
import type { SalesAnalysisItem, SalesAnalysisPeriodResult, SalesAnalysisTotals, SalesAnalysisWeek } from './types';
import type { FocusGroup } from './analysisFocus';
import { weeklySegmentRows, storeSegment } from './storeSegment';
import { periodNeedsItemHydration } from './salesAnalysisItems';
import { sortAnalysisTable, type AnalysisTable, type TableRow, type CellValue, type CellFormat, type TableSort } from './analysisTable';

type Ranked = { code: string; name: string; amount: number; quantity: number };
interface Data {
 items: SalesAnalysisItem[];
 performance: Array<{ label: string; current?: number; previous?: number; yearAgo?: number; format: 'money' | 'number' }>;
 categories: Array<{ name: string; code: string; current: number; previous: number; previous2: number; yearAgo: number }>;
 stores: Array<{ id: string; label: string; current?: SalesAnalysisTotals; previous?: SalesAnalysisTotals; yearAgo?: SalesAnalysisTotals }>;
 periods: SalesAnalysisPeriodResult[];
 week?: SalesAnalysisWeek;
 weekAligned: boolean;
 topSales: Ranked[]; topQuantity: Ranked[];
 salesGroups: Array<{ name: string; items: Ranked[] }>; quantityGroups: Array<{ name: string; items: Ranked[] }>;
 focus: FocusGroup[];
 insights?: AnalysisTable;
}
const num = (value: number | undefined): CellValue => value !== undefined && Number.isFinite(value) ? value : null;
export const relativeChange = (value: number | undefined, base: number | undefined): number | undefined => value === undefined || base === undefined || base === 0 ? undefined : (value - base) / Math.abs(base);
export function buildAnalysisTables(data: Data, t: Translator, locale: string, sorts: Record<string, TableSort>): Record<string, AnalysisTable[]> {
 const table = (id: string, name: string, columns: Array<[string, CellFormat]>, rows: TableRow[]): AnalysisTable => sortAnalysisTable({ id, name, columns: columns.map(([label, format]) => ({ label, format })), rows }, sorts[id], locale);
 const col = (key: string, format: CellFormat = 'money'): [string, CellFormat] => [t(key), format];
 const rankCols = [col('data.rank', 'number'), col('data.code', 'text'), col('analysis.article', 'text'), col('analysis.netSales'), col('analysis.netQuantity', 'number')];
 const rankRows = (items: Ranked[]): TableRow[] => items.map((item, index) => ({ cells: [index+1, item.code, item.name, num(item.amount), num(item.quantity)] }));
 const groupRankRows = (groups: Data['salesGroups']): TableRow[] => groups.flatMap((group) => rankRows(group.items).map((row) => ({ cells: [group.name, ...row.cells] })));
 const categoryNumber = (value: number, key: string): number | undefined => {
  const period = data.periods.find((period) => period.key === key);
  return period && !periodNeedsItemHydration(period) ? value : undefined;
 };
 const performance = table('performance', t('analysis.performance'), [col('analysis.metric','text'),col('analysis.currentPeriod'),col('analysis.previousPeriod'),col('analysis.yearAgoPeriod'),col('analysis.vsPrevious','percent'),col('analysis.vsYearAgo','percent')], data.performance.map((row) => ({ cells: [row.label,num(row.current),num(row.previous),num(row.yearAgo),num(relativeChange(row.current,row.previous)),num(relativeChange(row.current,row.yearAgo))] })));
 const products = table('products', t('analysis.items'), [col('analysis.store','text'),col('analysis.category','text'),col('data.code','text'),col('analysis.article','text'),col('analysis.transactions','number'),col('analysis.grossSales'),col('analysis.returns'),col('analysis.netQuantity','number'),col('analysis.netSales')], data.items.map((item) => ({
  cells: [item.storeId, item.category4 || item.category5 || t('analysis.uncategorized'), item.articleCode, item.articleName, num(item.transactionCount), num(item.saleAmount), num(item.returnAmount), num(item.netQuantity), num(item.netSalesAmount)],
  secondary: { 0: item.storeLabel, 1: item.category4Code || item.category5Code || '', 3: item.brandName || '' }, product: { column: 3, code: item.articleCode, name: item.articleName },
 })));
 const categories = table('categories', t('analysis.rolling'), [col('analysis.category','text'),col('analysis.currentPeriod'),col('analysis.previousPeriod'),col('analysis.previous2Period'),col('analysis.yearAgoPeriod'),col('analysis.vsPrevious','percent'),col('analysis.vsYearAgo','percent')], data.categories.map((row) => {
  const current = categoryNumber(row.current,'current'), previous = categoryNumber(row.previous,'previous'), yearAgo = categoryNumber(row.yearAgo,'yearAgo');
  return { cells: [row.name,num(current),num(previous),num(categoryNumber(row.previous2,'previous2')),num(yearAgo),num(relativeChange(current,previous)),num(relativeChange(current,yearAgo))], secondary: {0:row.code} };
 }));
 const stores = table('stores', t('analysis.storeComparison'), [col('analysis.store','text'),col('analysis.currentPeriod'),col('analysis.previousPeriod'),col('analysis.yearAgoPeriod'),col('analysis.vsPrevious','percent'),col('analysis.vsYearAgo','percent'),col('analysis.transactions','number'),col('analysis.basket')], data.stores.map((row) => ({ cells: [row.id,num(row.current?.netSalesAmount),num(row.previous?.netSalesAmount),num(row.yearAgo?.netSalesAmount),num(relativeChange(row.current?.netSalesAmount,row.previous?.netSalesAmount)),num(relativeChange(row.current?.netSalesAmount,row.yearAgo?.netSalesAmount)),num(row.current?.transactionCount),num(row.current?.transactionCount && row.current.trendNetSalesAmount !== undefined ? row.current.trendNetSalesAmount/row.current.transactionCount : undefined)],secondary:{0:row.label} })));
 const weeklyRows = data.week ? weeklySegmentRows(data.week.stores ?? [], {store:(store)=>store.businessId||store.label||'',localTotal:t('analysis.localTotal'),touristTotal:t('analysis.touristTotal'),allStores:t('analysis.allStores')}).map((row):TableRow => ({ fixed:row.kind!=='store',group:storeSegment(row.values),secondary:row.kind==='store'?{0:row.values.label||''}:undefined,cells:[row.label,num(row.values.salesTw),num(row.values.salesLw),num(row.values.salesTw-row.values.salesLw),num(relativeChange(row.values.salesTw,row.values.salesLw)),num(relativeChange(row.values.weekdaySalesTw,row.values.weekdaySalesLw)),num(relativeChange(row.values.weekendSalesTw,row.values.weekendSalesLw)),num(relativeChange(row.values.customersTw,row.values.customersLw))] })) : [];
 const weekly = table('weekly',t('analysis.weeklyTitle'),[col('analysis.store','text'),col(data.weekAligned?'analysis.currentPeriod':'analysis.thisWeek'),col(data.weekAligned?'analysis.previousPeriod':'analysis.lastWeek'),col('analysis.variance'),col('analysis.variancePercent','percent'),col('analysis.weekday','percent'),col('analysis.weekend','percent'),col('analysis.customers','percent')],weeklyRows);
 const focusName = (group: FocusGroup) => group.name || ({ health: t('analysis.focusHealth'), skin: t('analysis.focusSkin'), pc: t('analysis.focusPC') }[group.id] ?? group.id);
 const focusTable = (kind:'sales'|'quantity') => table(`focus-${kind}`,t(kind==='sales'?'analysis.focusSales':'analysis.focusQuantity'),[col('analysis.promoterGroup','text'),...rankCols,col('analysis.currentPeriod'),col('analysis.netQuantity','number')],data.focus.flatMap((group)=>group[kind].map((item,index)=>({cells:[focusName(group),index+1,item.code,item.name,num(item.amount),num(item.quantity),num(item.currentAmount),num(item.currentQuantity)]}))));
 return {
  overview:[performance,table('top-sales',t('analysis.topSales',{count:data.topSales.length}),rankCols,rankRows(data.topSales)),table('top-quantity',t('analysis.topQuantity',{count:data.topQuantity.length}),rankCols,rankRows(data.topQuantity)),...(data.insights?[data.insights]:[])],
  products:[products], stores:[stores], weekly:data.week?[weekly]:[],
  categories:[categories,table('category-sales',t('analysis.categorySalesRanking'),[col('analysis.category','text'),...rankCols],groupRankRows(data.salesGroups)),table('category-quantity',t('analysis.monthlyQuantityRanking'),[col('analysis.category','text'),...rankCols],groupRankRows(data.quantityGroups))],
  focus:data.focus.length?[focusTable('sales'),focusTable('quantity')]:[],
 };
}
