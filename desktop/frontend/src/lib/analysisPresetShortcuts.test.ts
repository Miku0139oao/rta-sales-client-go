import { beforeEach,afterEach,describe,expect,it,vi } from 'vitest';
import { ANALYSIS_PRESETS_KEY,loadAnalysisPresets,putAnalysisPreset,saveAnalysisPresets,setAnalysisPresetPinned,markAnalysisPresetUsed,analysisPresetShortcuts,type AnalysisPresetDraft } from './analysisPresets';
const draft:AnalysisPresetDraft={query:{profileId:'p',profileName:'P',periodMode:'month',monthMode:'current',month:'2026-08',from:'2026-08-01',to:'2026-08-31',weekCompare:false,storeIds:['107']},filters:{search:'',groupId:'',groupLevel:'category4',categories:{category1:[],category2:[],category3:[],category4:[],category5:[]}}};
beforeEach(()=>localStorage.clear());afterEach(()=>vi.restoreAllMocks());
describe('pinned and recent presets',()=>{
 it('reads existing v1 records without requiring new metadata',()=>{const list=putAnalysisPreset([],draft,'Old');expect(loadAnalysisPresets()).toEqual(list);expect(analysisPresetShortcuts(list)).toEqual({pinned:[],recent:[]});});
 it('persists pins and last-use times across rename and query updates',()=>{
  let list=putAnalysisPreset([],draft,'Saved');const id=list[0]!.id;
  list=setAnalysisPresetPinned(list,id,true);list=markAnalysisPresetUsed(id,1000);list=putAnalysisPreset(list,{...draft,filters:{...draft.filters,search:'Mask'}},'Renamed',id);
  expect(loadAnalysisPresets()[0]).toMatchObject({name:'Renamed',pinned:true,lastUsedAt:1000,filters:{search:'Mask'}});
 });
 it('caps pins at three without changing saved data on failure',()=>{
  let list=loadAnalysisPresets();
  for(let index=0;index<4;index++)list=putAnalysisPreset(list,draft,`Name ${index}`);
  for(const preset of list.slice(0,3))list=setAnalysisPresetPinned(list,preset.id,true);
  const before=localStorage.getItem(ANALYSIS_PRESETS_KEY);expect(()=>setAnalysisPresetPinned(list,list[3]!.id,true)).toThrow('pinLimit');expect(localStorage.getItem(ANALYSIS_PRESETS_KEY)).toBe(before);
 });
 it('shows only three recent entries, newest first, without duplicating pins',()=>{
  let list=loadAnalysisPresets();for(let index=0;index<5;index++){list=putAnalysisPreset(list,draft,`Name ${index}`);list=markAnalysisPresetUsed(list[index]!.id,index+1);}
  list=setAnalysisPresetPinned(list,list[4]!.id,true);
  expect(analysisPresetShortcuts(list).recent.map(preset=>preset.name)).toEqual(['Name 3','Name 2','Name 1']);expect(analysisPresetShortcuts(list).pinned.map(preset=>preset.name)).toEqual(['Name 4']);
 });
 it('never resurrects an entry deleted while its query was running',()=>{const list=putAnalysisPreset([],draft,'Deleted');saveAnalysisPresets([]);expect(markAnalysisPresetUsed(list[0]!.id)).toEqual([]);});
 it('rejects malformed metadata without erasing local storage',()=>{
  const list=putAnalysisPreset([],draft,'Bad');localStorage.setItem(ANALYSIS_PRESETS_KEY,JSON.stringify({version:1,presets:[{...list[0],lastUsedAt:'yesterday'}]}));const before=localStorage.getItem(ANALYSIS_PRESETS_KEY);expect(()=>loadAnalysisPresets()).toThrow('invalid');expect(localStorage.getItem(ANALYSIS_PRESETS_KEY)).toBe(before);
 });
 it('does not mutate the last-use record if the write fails',()=>{
  const list=putAnalysisPreset([],draft,'Saved');vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('quota');});expect(()=>markAnalysisPresetUsed(list[0]!.id)).toThrow('quota');expect(loadAnalysisPresets()[0]!.lastUsedAt).toBeUndefined();
 });
});
