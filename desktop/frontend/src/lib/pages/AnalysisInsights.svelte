<script lang="ts">
  import type { Translator } from '../i18n';
  import type { SalesInsights } from '../salesInsights';
  import { formatTableCell } from '../analysisTable';
  export let t: Translator;
  export let locale: string;
  export let data: SalesInsights;
  export let onProduct: (code: string, name: string) => void;
  $: money = (value: number | null) => formatTableCell(value, 'money', locale);
</script>
<section class="insights" aria-labelledby="insights-title">
  <div class="insights-heading"><h2 id="insights-title">{t('insights.title')}</h2><span>{t('insights.local')}</span></div>
  {#if data.reason !== 'ready'}<p class="coverage-note" role="status">{t(`insights.reason.${data.reason}`)}</p>{/if}
  {#if data.entries.length}
    <div class="insight-grid">
      {#each data.entries as entry (`${entry.kind}:${entry.code}`)}
        <article class:decline={entry.kind === 'decline'}>
          <h3>{t(`insights.${entry.kind}`)}</h3>
          <strong class="insight-value">{entry.kind === 'growth' ? '+' : ''}{money(entry.kind === 'decline' || entry.kind === 'growth' ? entry.difference : entry.kind === 'returns' ? entry.refunds : entry.current)}</strong>
          <button type="button" class="insight-link" aria-label={t('insights.inspect', { name: entry.name })} onclick={() => onProduct(entry.code, entry.name)}>{entry.name}<span>{entry.code}</span></button>
          {#if entry.kind === 'decline' || entry.kind === 'growth'}
            <p>{t('analysis.previousPeriod')} {money(entry.previous)} → {t('analysis.currentPeriod')} {money(entry.current)}<br />{entry.percent === null ? t('insights.noRate') : `${formatTableCell(entry.percent, 'percent', locale)} ${t('analysis.vsPrevious')}`}</p>
          {:else}<p>{t(entry.kind === 'returns' ? 'insights.refundBasis' : 'insights.netBasis')}</p>{/if}
        </article>
      {/each}
    </div>
  {:else if data.reason === 'ready'}<p class="coverage-note">{t('insights.empty')}</p>{/if}
  <details>
    <summary>{t('insights.basis')}</summary>
    {#if data.current}<p>{data.current.label} · {data.current.from} — {data.current.to}</p>{/if}
    {#if data.previous}<p>{data.previous.label} · {data.previous.from} — {data.previous.to}</p>{/if}
    <p>{t('insights.method')}</p><p>{t('insights.scope')}</p>
  </details>
</section>
<style>
  .insights { min-width: 0; padding: 16px; border: 1px solid var(--app-border); border-radius: 14px; background: var(--app-card); }
  .insights-heading { display: flex; align-items: baseline; flex-wrap: wrap; gap: 6px 16px; margin-bottom: 12px; }
  h2 { margin: 0; font-size: 16px; font-weight: 700; }
  .insights-heading > span { margin-left: auto; color: var(--md-sys-color-on-surface-variant); font-size: 11px; }
  .insight-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr)); gap: 12px; }
  article { min-width: 0; border-radius: 10px; padding: 12px; background: var(--md-sys-color-surface-container-low); }
  h3 { margin: 0 0 7px; font-size: 12px; color: var(--md-sys-color-on-surface-variant); font-weight: 600; }
  .insight-value { display: block; margin-bottom: 8px; color: var(--md-sys-color-primary); font-size: 20px; line-height: 1.3; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  .decline .insight-value { color: var(--md-sys-color-error); }
  .insight-link { padding: 0; border: 0; background: transparent; color: var(--md-sys-color-primary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; overflow-wrap: anywhere; font-weight: 650; }
  .insight-link > span { display: block; margin-top: 3px; color: var(--md-sys-color-on-surface-variant); font-size: 11px; font-weight: 400; }
  .insight-link:hover { text-decoration: underline; }
  .insight-link:focus-visible, summary:focus-visible { outline: 2px solid var(--md-sys-color-primary); outline-offset: 3px; }
  p { margin: 8px 0 0; font-size: 11px; line-height: 1.7; color: var(--md-sys-color-on-surface-variant); overflow-wrap: anywhere; }
  .coverage-note { margin: 0 0 12px; }
  details { margin-top: 12px; font-size: 11px; color: var(--md-sys-color-on-surface-variant); }
  summary { cursor: pointer; width: fit-content; }
  @media(max-width:520px) { .insights { padding: 12px; }.insights-heading > span { margin-left: 0; } }
</style>
