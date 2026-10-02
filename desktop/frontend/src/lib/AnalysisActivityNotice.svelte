<script lang="ts">
  import { analysisActivity } from './analysisActivity';
  export let locale: string;
  export let onOpen: () => void;
  export let onCancel: () => void;
  $: en = locale === 'en';
  $: phases = en
    ? {
        authorizing: 'Signing in and loading stores',
        current: 'Querying current sales',
        comparison: 'Loading comparison periods',
        trend: 'Loading transaction trends',
        export: 'Generating reports',
        workflow: 'Running report workflow',
        error: 'Action required',
      }
    : {
        authorizing: '登入並載入門店',
        current: '查詢本期銷售',
        comparison: '補齊比較期間',
        trend: '補齊交易趨勢',
        export: '產生報表',
        workflow: '執行報表流程',
        error: '需要處理',
      };
</script>

{#if $analysisActivity}
  <aside class="activity" aria-label={en ? 'Current task' : '目前任務'}>
    <div class="copy">
      <strong
        >{phases[$analysisActivity.phase as keyof typeof phases] ?? (en ? 'Query in progress' : '查詢進行中')}</strong
      ><span>{$analysisActivity.scope}</span>{#if $analysisActivity.error}<span role="alert"
          >{$analysisActivity.error}</span
        >{/if}
    </div>
    {#if $analysisActivity.total > 0}<span>{$analysisActivity.current} / {$analysisActivity.total}</span><progress
        max={$analysisActivity.total}
        value={$analysisActivity.current}
        aria-label={en ? 'Completed tasks' : '已完成工作'}
      ></progress>{:else if !$analysisActivity.error}<progress aria-label={en ? 'Preparing task' : '準備工作'}
      ></progress>{/if}
    <button type="button" onclick={onOpen}>{en ? 'View task' : '查看任務'}</button>
    {#if !$analysisActivity.error && $analysisActivity.phase !== 'export'}<button type="button" onclick={onCancel}
        >{en ? 'Cancel' : '取消'}</button
      >{/if}
    {#if $analysisActivity.error}<button type="button" onclick={() => analysisActivity.set(undefined)}
        >{en ? 'Dismiss' : '關閉'}</button
      >{/if}
  </aside>
{/if}

<style>
  .activity {
    position: sticky;
    top: 0;
    z-index: 8;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 12px 18px;
    background: var(--md-sys-color-surface-container-high);
    border-bottom: 1px solid var(--md-sys-color-outline-variant);
  }
  .copy {
    display: grid;
    gap: 3px;
    flex: 1;
    min-width: 160px;
  }
  .copy span {
    font-size: 12px;
    color: var(--md-sys-color-on-surface-variant);
    overflow-wrap: anywhere;
  }
  button {
    min-height: 44px;
    border: 1px solid var(--md-sys-color-outline-variant);
    border-radius: 20px;
    padding: 8px 14px;
    color: var(--md-sys-color-primary);
    background: transparent;
    cursor: pointer;
  }
  progress {
    width: 100px;
    accent-color: var(--md-sys-color-primary);
  }
  @media (max-width: 760px) {
    .activity { position: static; gap: 8px; padding: 12px; border-radius: 12px; margin-bottom: 12px; }
    .copy { flex-basis: 100%; }
    progress { flex: 1; min-width: 48px; }
    button { padding-inline: 12px; }
  }
  @media (max-height: 540px) { .activity { position: static; } }
</style>
