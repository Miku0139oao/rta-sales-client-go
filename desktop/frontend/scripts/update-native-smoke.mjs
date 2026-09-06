import assert from 'node:assert/strict';
import { readFile, writeFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE);
const m = JSON.parse(await readFile(process.env.RTA_PORTABLE_SMOKE_MANIFEST,'utf8'));
const delay = ms => new Promise(r=>setTimeout(r,ms));
async function until(fn,timeout=90000) { const end=Date.now()+timeout; let last; while(Date.now()<end) { try { const value=await fn(); if(value)return value; }catch(e){last=e;} await delay(250); } throw last??Error('Timed out'); }
async function connect() { return until(()=>chromium.connectOverCDP(`http://127.0.0.1:${m.port}`,{timeout:2000})); }
const hash = async p=>createHash('sha256').update(await readFile(p)).digest('hex');
const settingsKey='rta-sales-desktop-settings-v2';
let browser, page;
try {
 browser=await connect(); page=browser.contexts()[0].pages()[0];
 await page.locator('main').waitFor();
 await page.evaluate(({settingsKey,nonce})=>{localStorage.setItem(settingsKey,JSON.stringify({locale:'en',theme:'dark',rankingLimit:32,autoCheckUpdates:false}));localStorage.setItem('portable-smoke-nonce',nonce);},{settingsKey,nonce:m.nonce});
 await page.reload();
 await page.getByRole('button',{name:'Settings',exact:true}).click();
 await page.getByText('Current version: 0.4.6',{exact:true}).waitFor();
 const settings=await page.evaluate(key=>localStorage.getItem(key),settingsKey);
 const data={};for(const name of await readdir(join(m.root,'data'))) {if(name.endsWith('.json')||name==='nonce')data[name]=await readFile(join(m.root,'data',name),'utf8');}
 await page.getByRole('button',{name:'Check for updates',exact:true}).click();
 await page.getByText('New version available: 0.4.7',{exact:true}).last().waitFor();
 await page.getByText(`Signed isolated smoke ${m.nonce}`,{exact:true}).waitFor();
 await page.getByRole('button',{name:'Download and restart…',exact:true}).last().click();
 const dialog=page.getByRole('dialog',{name:'Confirm update and restart'});await dialog.waitFor();
 await page.screenshot({path:join(m.root,'confirmation.png')});
 await dialog.getByRole('button',{name:'Confirm download and restart',exact:true}).click();
 const starts=await until(async()=>{const lines=(await readFile(join(m.root,'starts.log'),'utf8')).trim().split(/\r?\n/);return lines.length===2?lines:false;});
 assert.match(starts[0],new RegExp(`^\\d+ 0\\.4\\.6 ${m.nonce}$`));assert.match(starts[1],new RegExp(`^\\d+ 0\\.4\\.7 ${m.nonce}$`));
 assert.notEqual(starts[0].split(' ')[0],starts[1].split(' ')[0]);
 await browser.close().catch(()=>{});browser=await connect();page=browser.contexts()[0].pages()[0];
 await page.locator('main').waitFor();
 assert.equal(await page.evaluate(key=>localStorage.getItem(key),settingsKey),settings);
 assert.equal(await page.evaluate(()=>localStorage.getItem('portable-smoke-nonce')),m.nonce);
 await page.getByRole('button',{name:'Settings',exact:true}).click();
 await page.getByText('Current version: 0.4.7',{exact:true}).waitFor();
 assert.equal(await hash(join(m.root,'fixture.exe')),m.newHash);
 const stage=await until(async()=>{for(const name of await readdir(m.root)){try{const r=JSON.parse(await readFile(join(m.root,name,'result.json'),'utf8'));return {name,result:r};}catch{}}});
 assert.equal(stage.result.phase,'complete');
 assert.equal(await hash(join(m.root,stage.name,'previous.exe')),m.oldHash);
 for(const [name,bytes] of Object.entries(data))assert.equal(await readFile(join(m.root,'data',name),'utf8'),bytes);
 await page.screenshot({path:join(m.root,'restarted.png')});
 await writeFile(join(m.root,'e2e-result.json'),JSON.stringify({passed:true,starts,stage,oldHash:m.oldHash,newHash:m.newHash,settingsPreserved:true,repositoryFiles:Object.keys(data)},null,2));
 console.log(`PASS signed real Wails upgrade: ${m.root}`);
} catch(error) {
 await writeFile(join(m.root,'failure.txt'),`${error.stack}\n${page?await page.locator('body').innerText().catch(()=>'<closed>'):''}`);
 throw error;
} finally {await browser?.close().catch(()=>{});}
