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
      if (!updateIsExclusive(next.phase) && settings.autoCheckUpdates !== false) await check();
    } catch { /* Startup/offline failures must not interrupt the user. */ }
  }
  async function check() {
    if (checking || exclusive || confirmCandidate) return;
    checking = true; error = '';
    if (status) status = { ...status, phase: 'checking', candidateId: '', availableVersion: '', releaseNotes: '', changelogVersion: '', changelogBody: '' };
    try { const next = await updates.check(); if (alive) status = next; }
    catch (cause) { if (alive) error = cause instanceof Error ? cause.message : String(cause); }
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
  <section class="surface-card settings-card update-card" class:compact={!details} class:settings-column={details} aria-label={en ? 'Portable updates' : '免安裝版更新'}>
    <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">system_update_alt</span></div>
    <div class="settings-content update-content">
    <header class="update-heading">
      <div>
        {#if details}<span class="update-eyebrow">{en ? 'APPLICATION' : '應用程式'}</span>{/if}
    <h2>{en ? 'Portable updates' : '免安裝版更新'}</h2>
        {#if details}<p class="update-description">{en ? 'Stay up to date. You decide when to download and restart.' : '保持最新版本，由您決定何時下載與重啟。'}</p>{/if}
      </div>
      <span class="version-badge">{en ? 'Current version' : '目前版本'}: {status?.currentVersion ?? 'dev'}</span>
    </header>
    {#if details}
      <label class="setting-row update-preference">
        <span><strong>{en ? 'Check for updates at startup (no automatic download)' : '啟動時檢查更新（不會自動下載）'}</strong>
          <small id="update-startup-hint">{en ? 'Only release metadata is checked. Downloads always require your confirmation.' : '只檢查版本資訊；下載前一律先取得您的確認。'}</small>
        </span>
        <input class="update-switch" type="checkbox" aria-label={en ? 'Check for updates at startup (no automatic download)' : '啟動時檢查更新（不會自動下載）'} aria-describedby="update-startup-hint" checked={settings.autoCheckUpdates !== false}
          onchange={(event) => onChange({ ...settings, autoCheckUpdates: event.currentTarget.checked })} />
      </label>
      {#if checking}<p class="update-status" role="status"><span class="material-symbols-rounded" aria-hidden="true">sync</span>{en ? 'Checking…' : '檢查中…'}</p>
      {:else if !exclusive && status?.phase === 'current'}<p class="update-status current" role="status"><span class="material-symbols-rounded" aria-hidden="true">check_circle</span>{en ? 'No newer stable release.' : '沒有較新的正式版本。'}</p>{/if}
      {#if status && !status.installSupported}
        <p class="update-support">{en ? 'Automatic installation is unavailable for this build.' : '此版本無法自動安裝更新。'} {status.unsupportedReason}</p>
      {/if}
    {/if}
    {#if details && !checking && status?.changelogVersion}
      <details class="release-notes">
        <summary>{en ? 'Changelog' : '更新日誌'} — {en ? 'Latest GitHub Release' : 'GitHub 最新正式版本'} v{status.changelogVersion}</summary>
        <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users need to focus and scroll long changelogs.) -->
        <pre role="region" tabindex="0" aria-label={`${en ? 'Changelog' : '更新日誌'} v${status.changelogVersion}`}>{status.changelogBody || (en ? 'This release has no changelog.' : '此正式版本未提供更新日誌。')}</pre>
      </details>
    {/if}
    {#if status?.availableVersion}
      {#if !exclusive && !checking}<p class="update-status available" role="status"><span class="material-symbols-rounded" aria-hidden="true">new_releases</span>{en ? 'New version available' : '有新版本'}: {status.availableVersion}</p>{/if}
      {#if !status.installSupported}
        <p>{en ? 'Close the app and update manually from Releases.' : '請關閉程式後，從 Releases 手動更新。'}</p>
        <a class="update-link" href="https://github.com/Miku0139oao/rta-sales-client-go/releases" target="_blank" rel="noreferrer">GitHub Releases <span class="material-symbols-rounded" aria-hidden="true">open_in_new</span></a>
      {/if}
    {/if}
    {#if exclusive}
      <p class="update-status" role="status"><span class="material-symbols-rounded" aria-hidden="true">sync</span>{phaseText}</p>
      {#if !cancellationClosed}<button class="update-button secondary" type="button" onclick={() => void cancelUpdate()}>{en ? 'Cancel update' : '取消更新'}</button>{/if}
    {/if}
    {#if (details && error) || (exclusive && (error || status?.error))}<p class="dialog-error" role="status">{error || status?.error}</p>{/if}
    <div class="form-actions update-actions">
      {#if details}<button class="update-button secondary" type="button" disabled={checking || exclusive || Boolean(confirmCandidate)} onclick={() => void check()}>
        <span class="material-symbols-rounded" aria-hidden="true">refresh</span>{checking ? (en ? 'Checking…' : '檢查中…') : (en ? 'Check for updates' : '檢查更新')}
      </button>{/if}
      {#if status?.availableVersion && status.installSupported}<button class="update-button primary" type="button" disabled={exclusive || checking || busy} onclick={requestInstall}><span class="material-symbols-rounded" aria-hidden="true">download</span>{en ? 'Download and restart…' : '下載並重啟…'}</button>{/if}
    </div>
    </div>
  </section>
{/if}

{#if confirmCandidate}
  <dialog class="app-dialog update-dialog" use:modal={{ onClose: () => { confirmCandidate = ''; } }} aria-labelledby="update-confirm-title" aria-describedby="update-confirm-warning update-confirm-preserved">
    <div class="update-dialog-body">
      <div class="dialog-header">
        <div class="section-icon" aria-hidden="true"><span class="material-symbols-rounded">system_update_alt</span></div>
        <div><h2 id="update-confirm-title">{en ? 'Confirm update and restart' : '確認更新並重啟'}</h2><p class="update-description">{en ? 'Download the verified update, then restart the app.' : '下載並驗證更新後，程式將重新啟動。'}</p></div>
      </div>
      <div class="version-transition" aria-label={en ? 'Version transition' : '版本變更'}><span>{status?.currentVersion ?? 'dev'}</span><span class="material-symbols-rounded" aria-hidden="true">arrow_forward</span><strong>{status?.availableVersion}</strong></div>
      {#if status?.releaseNotes}
        <details class="release-notes">
          <summary>{en ? 'Changelog' : '更新日誌'} v{status.availableVersion}</summary>
          <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users need to scroll candidate notes.) -->
          <pre role="region" tabindex="0" aria-label={`${en ? 'Changelog' : '更新日誌'} v${status.availableVersion}`}>{status.releaseNotes}</pre>
        </details>
      {/if}
      <div class="notice warning-notice" id="update-confirm-warning"><span class="material-symbols-rounded" aria-hidden="true">warning</span><p>{en ? 'The app will close and restart. Unsaved reports and previews will be lost. Finish and export your work first.' : '程式將關閉並重啟。未儲存的報表與預覽將遺失，請先完成並匯出工作。'}</p></div>
      <p class="update-preserved" id="update-confirm-preserved"><span class="material-symbols-rounded" aria-hidden="true">verified_user</span>{en ? 'Accounts and settings are preserved.' : '已儲存的帳號與設定會保留。'}</p>
    </div>
    <div class="dialog-actions update-dialog-footer">
      <button class="update-button secondary" type="button" onclick={() => { confirmCandidate = ''; }}>{en ? 'Not now' : '暫時不要'}</button>
      <button class="update-button primary" type="button" disabled={busy} onclick={() => void installConfirmed()}>{en ? 'Confirm download and restart' : '確認下載並重啟'}</button>
    </div>
  </dialog>
{/if}

<style>
  .update-card { margin-bottom: 22px; }
  .compact { max-width: 960px; }
  .update-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .update-content h2 { margin: 0; }
  .update-eyebrow { display: block; margin-bottom: 5px; font-size: 11px; font-weight: 700; letter-spacing: .1em; color: var(--md-sys-color-on-surface-variant); }
  .update-description { margin: 7px 0 0; color: var(--md-sys-color-on-surface-variant); font-size: 13px; line-height: 1.6; }
  .version-badge { flex-shrink: 0; padding: 6px 10px; border-radius: 8px; background: var(--md-sys-color-surface-container); color: var(--md-sys-color-on-surface-variant); font-size: 12px; font-variant-numeric: tabular-nums; }
  .update-preference { margin-top: 20px; padding: 16px 0; border-block: 1px solid var(--app-border-soft); cursor: pointer; }
  .update-preference strong { font-size: 13px; }
  .update-preference small { display: block; color: var(--md-sys-color-on-surface-variant); font-size: 12px; line-height: 1.6; }
  .update-switch { appearance: none; flex: 0 0 44px; width: 44px; height: 26px; padding: 3px; margin: 0; border: 2px solid var(--md-sys-color-outline); border-radius: 999px; background: var(--md-sys-color-surface-container-highest); cursor: pointer; }
  .update-switch::before { content: ''; display: block; width: 16px; height: 16px; border-radius: 50%; background: var(--md-sys-color-outline); }
  .update-switch:checked { background: var(--md-sys-color-primary); border-color: var(--md-sys-color-primary); }
  .update-switch:checked::before { transform: translateX(18px); background: var(--md-sys-color-on-primary); }
  .update-switch:hover { box-shadow: 0 0 0 4px var(--app-focus-ring); }
  .update-status, .update-preserved { display: flex; align-items: center; gap: 8px; font-size: 13px; line-height: 1.6; }
  .available, .current, .update-preserved { color: var(--md-sys-color-primary); }
  .update-status .material-symbols-rounded, .update-preserved .material-symbols-rounded { font-size: 20px; flex-shrink: 0; }
  .update-support { font-size: 13px; color: var(--md-sys-color-on-surface-variant); line-height: 1.6; overflow-wrap: anywhere; }
  .release-notes { border: 1px solid var(--app-border-soft); border-radius: 12px; background: var(--md-sys-color-surface-container-low); }
  summary { padding: 12px 14px; cursor: pointer; font-size: 13px; font-weight: 650; border-radius: 12px; }
  summary:hover { background: var(--md-sys-color-surface-container); }
  summary:focus-visible, .update-link:focus-visible { outline: 3px solid var(--app-focus-ring); outline-offset: 2px; }
  pre { max-height: 220px; overflow: auto; overscroll-behavior: contain; margin: 0; padding: 0 14px 14px; white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; font-size: 13px; line-height: 1.7; color: var(--md-sys-color-on-surface-variant); }
  .update-actions { flex-wrap: wrap; margin-top: 16px; }
  .update-button { display: inline-flex; min-height: 40px; align-items: center; justify-content: center; gap: 8px; padding: 9px 20px; border: 1px solid transparent; border-radius: 999px; font: inherit; font-size: 14px; font-weight: 600; cursor: pointer; }
  .update-button .material-symbols-rounded { font-size: 18px; }
  .primary { background: var(--md-sys-color-primary); color: var(--md-sys-color-on-primary); }
  .secondary { background: transparent; border-color: var(--md-sys-color-outline); color: var(--md-sys-color-primary); }
  .update-button:hover:not(:disabled) { box-shadow: inset 0 0 0 999px rgb(127 127 127 / .12); }
  .update-button:disabled { opacity: .4; cursor: not-allowed; }
  .update-link { display: inline-flex; align-items: center; gap: 6px; color: var(--md-sys-color-primary); font-size: 13px; }
  .update-link .material-symbols-rounded { font-size: 16px; }
  .compact { padding: 16px 20px; gap: 14px; }
  .compact h2 { font-size: 16px; }
  .compact .update-content { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 18px; }
  .compact .update-heading { flex: 1 1 240px; align-items: center; }
  .compact .update-status, .compact .update-actions { margin: 0; }
  .compact .version-badge { display: none; }
  .update-dialog { padding: 0; }
  .update-dialog-body { padding: 26px; }
  .update-dialog .dialog-header { justify-content: flex-start; gap: 16px; margin-bottom: 20px; }
  .version-transition { display: flex; align-items: center; justify-content: center; gap: 20px; padding: 16px; margin-bottom: 20px; border-radius: 14px; background: var(--md-sys-color-surface-container-low); font-size: 18px; font-variant-numeric: tabular-nums; }
  .version-transition strong { color: var(--md-sys-color-primary); }
  .update-dialog .notice { margin: 0; align-items: flex-start; }
  .update-dialog .notice p { margin: 0; font-size: 13px; line-height: 1.7; }
  .update-preserved { margin: 16px 0 0; }
  .update-dialog-footer { position: sticky; bottom: 0; flex-wrap: wrap; padding: 16px 26px; border-top: 1px solid var(--app-border-soft); background: var(--md-sys-color-surface-container-lowest); }
  @media (max-width: 600px) {
    .update-card { padding: 18px; gap: 12px; }
    .update-card > .section-icon { display: none; }
    .update-heading { flex-wrap: wrap; gap: 10px; }
    .update-actions { justify-content: flex-start; }
    .update-dialog-body { padding: 20px; }
    .update-dialog-footer { padding: 14px 20px; }
    .update-dialog-footer .update-button { flex: 1 1 auto; }
  }
  @media (prefers-reduced-motion: reduce) { .update-button { transition: none; } .update-button:active { transform: none; } }
</style>
