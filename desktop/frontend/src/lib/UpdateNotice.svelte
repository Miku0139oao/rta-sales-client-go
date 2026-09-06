<script lang="ts">
  import { onMount } from 'svelte';
  import { isWebRuntime } from './runtime';
  import { modal } from './modal';
  import { updates, updateIsExclusive, type UpdateStatus } from './updates';
  import type { AppSettings } from './types';

  export let settings: AppSettings;
  export let details = false;
  export let busy = false;
  export let onChange: (settings: AppSettings) => void;
  export let onBusyChange: (busy: boolean) => void = () => undefined;
  let status: UpdateStatus | undefined;
  let checking = false;
  let installing = false;
  let error = '';
  let alive = true;
  let polling = false;
  let confirmCandidate = '';
  $: en = settings.locale === 'en';
  $: exclusive = installing || updateIsExclusive(status?.phase);
  $: onBusyChange(exclusive || Boolean(confirmCandidate));
  $: cancellationClosed = status?.phase === 'committing' || status?.phase === 'committed';
  $: phaseText = stageText(status?.phase, en);

  onMount(() => {
    alive = true;
    if (!isWebRuntime()) void initialise();
    const timer = setInterval(() => { if (exclusive && !polling) void refresh(); }, 250);
    return () => { alive = false; clearInterval(timer); };
  });
  async function refresh() {
    if (polling) return;
    polling = true;
    try { const next = await updates.status(); if (alive) status = next; }
    catch { /* Keep the last known reservation; never assume failure means idle. */ }
    finally { polling = false; }
  }
  async function initialise() {
    try {
      const next = await updates.status();
      if (!alive) return;
      status = next;
      if (!updateIsExclusive(next.phase) && settings.autoCheckUpdates !== false) await check(true);
    } catch { /* Startup/offline failures must not interrupt the user. */ }
  }
  async function check(background = false) {
    if (checking || exclusive || confirmCandidate) return;
    checking = true; error = '';
    try { const next = await updates.check(); if (alive) status = next; }
    catch (cause) { if (alive && !background) error = cause instanceof Error ? cause.message : String(cause); }
    finally { if (alive) checking = false; }
  }
  function requestInstall() {
    if (!status?.installSupported || !status.candidateId || exclusive || checking || busy) return;
    confirmCandidate = status.candidateId; // snapshot the exact checked candidate
  }
  async function installConfirmed() {
    if (!confirmCandidate || exclusive || busy) return;
    const candidateId = confirmCandidate;
    confirmCandidate = ''; installing = true; error = '';
    try { await updates.install({ candidateId, confirmed: true }); }
    catch (cause) { if (alive) error = cause instanceof Error ? cause.message : String(cause); }
    finally { await refresh(); if (alive) installing = false; }
  }
  async function cancelUpdate() {
    if (cancellationClosed) return;
    error = '';
    try { await updates.cancel(); }
    catch (cause) { error = cause instanceof Error ? cause.message : String(cause); }
    await refresh();
  }
  function stageText(phase: UpdateStatus['phase'] | undefined, english: boolean) {
    const stages: Partial<Record<UpdateStatus['phase'], [string, string]>> = {
      preparing: ['Preparing update', '準備更新'],
      'verifying-current': ['Verifying current executable and staging', '驗證目前執行檔與暫存'],
      downloading: ['Downloading and verifying the signed update', '下載並驗證簽章更新'],
      'starting-helper': ['Verifying helper readiness', '驗證更新輔助程式就緒狀態'],
      ready: ['Helper ready', '更新輔助程式已就緒'],
      cancelling: ['Cancelling; waiting for safe cleanup', '取消中；等待安全清理'],
      committing: ['Committing restart; cancellation closed', '確認重啟；無法再取消'],
      committed: ['Closing for update and restart', '關閉程式以更新並重啟'],
      blocked: ['Keep this app open; retry cancellation', '請保持程式開啟並重試取消'],
    };
    return phase && stages[phase] ? stages[phase]![english ? 0 : 1] : '';
  }
</script>

{#if !isWebRuntime() && (details || status?.availableVersion || exclusive)}
  <section class="surface-card" aria-label={en ? 'Portable updates' : '免安裝版更新'}>
    <h2>{en ? 'Portable updates' : '免安裝版更新'}</h2>
    {#if details}
      <p>{en ? 'Current version' : '目前版本'}: {status?.currentVersion ?? 'dev'}</p>
      <label>
        <input type="checkbox" checked={settings.autoCheckUpdates !== false}
          onchange={(event) => onChange({ ...settings, autoCheckUpdates: event.currentTarget.checked })} />
        {en ? 'Check for updates at startup (no automatic download)' : '啟動時檢查更新（不會自動下載）'}
      </label>
      <button type="button" disabled={checking || exclusive || Boolean(confirmCandidate)} onclick={() => void check()}>
        {checking ? (en ? 'Checking…' : '檢查中…') : (en ? 'Check for updates' : '檢查更新')}
      </button>
      {#if status?.phase === 'current'}<p role="status">{en ? 'No newer stable release.' : '沒有較新的正式版本。'}</p>{/if}
      {#if status && !status.installSupported}
        <p>{en ? 'Automatic installation is unavailable for this build.' : '此版本無法自動安裝更新。'} {status.unsupportedReason}</p>
      {/if}
    {/if}
    {#if status?.availableVersion}
      <p role="status">{en ? 'New version available' : '有新版本'}: {status.availableVersion}</p>
      {#if details}<pre style="white-space: pre-wrap; overflow-wrap: anywhere">{status.releaseNotes}</pre>{/if}
      {#if status.installSupported}
        <button type="button" disabled={exclusive || checking || busy} onclick={requestInstall}>{en ? 'Download and restart…' : '下載並重啟…'}</button>
      {:else}
        <p>{en ? 'Close the app and update manually from Releases.' : '請關閉程式後，從 Releases 手動更新。'}</p>
        <a href="https://github.com/Miku0139oao/rta-sales-client-go/releases" target="_blank" rel="noreferrer">GitHub Releases</a>
      {/if}
    {/if}
    {#if exclusive}
      <p role="status">{phaseText}</p>
      {#if !cancellationClosed}<button type="button" onclick={() => void cancelUpdate()}>{en ? 'Cancel update' : '取消更新'}</button>{/if}
    {/if}
    {#if error || (exclusive && status?.error)}<p role="status">{error || status?.error}</p>{/if}
  </section>
{/if}

{#if confirmCandidate}
  <dialog class="app-dialog" use:modal={{ onClose: () => { confirmCandidate = ''; } }} aria-labelledby="update-confirm-title">
    <h2 id="update-confirm-title">{en ? 'Confirm update and restart' : '確認更新並重啟'}</h2>
    <p>{en ? 'The app will close and restart. Unsaved reports and previews will be lost. Finish and export your work first. Accounts and settings are preserved.' : '程式將關閉並重啟。未儲存的報表與預覽將遺失，請先完成並匯出工作。帳號與設定會保留。'}</p>
    <button type="button" onclick={() => { confirmCandidate = ''; }}>{en ? 'Not now' : '暫時不要'}</button>
    <button type="button" disabled={busy} onclick={() => void installConfirmed()}>{en ? 'Confirm download and restart' : '確認下載並重啟'}</button>
  </dialog>
{/if}
