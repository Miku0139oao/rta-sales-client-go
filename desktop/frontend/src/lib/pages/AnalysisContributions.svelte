<script lang="ts">
  import type { Translator } from '../i18n';
  import { CONTRIBUTION_PREVIEW_LIMIT, type ContributionGroup, type ContributionRename, type SalesContributions } from '../salesContributions';
  import { formatTableCell } from '../analysisTable';
  export let t: Translator;
  export let locale: string;
  export let data: SalesContributions | undefined = undefined;
  $: money = (value: number | null) => formatTableCell(value, 'money', locale);
  function labeled(group: { label: string; code?: string }): string {
    const name = group.label || t('analysis.uncategorized');
    return group.code ? t('contributions.namedGroup', { name, code: group.code }) : name;
  }
  function changeText(group: ContributionGroup): string {
    return t('contributions.change', { name: labeled(group), amount: money(group.delta) });
  }
  function transferPreview(transfers: ContributionRename[]): ContributionRename[] {
    return transfers.slice(0, CONTRIBUTION_PREVIEW_LIMIT);
  }
</script>
{#if data?.ready && data.store && data.category}
  <div class="contributions">
    <p>{t('contributions.intro')}</p>
    <p>{t('contributions.method')}</p>
    <p>{t('contributions.noPercent')}</p>
    {#each [data.store, data.category] as view (view.dimension)}
      <section class="contrib-view" aria-labelledby={`contrib-${view.dimension}-title`}>
        <h3 id={`contrib-${view.dimension}-title`}>{t(view.dimension === 'store' ? 'contributions.stores' : 'contributions.categories')}</h3>
        {#if view.dimension === 'category'}
          <p>{t('contributions.categoryLevel', { level: t('analysis.category1') })}</p>
          {#if view.transfers.length}<p>{t('contributions.transfers')}</p>{/if}
        {/if}
        {#if view.gains.length || view.losses.length}
          <ul>
            {#each view.gains as group (`gain:${group.key}`)}
              <li aria-label={changeText(group)}>{changeText(group)}</li>
            {/each}
            {#each view.losses as group (`loss:${group.key}`)}
              <li aria-label={changeText(group)}>{changeText(group)}</li>
            {/each}
          </ul>
        {:else if !view.remainder}
          <p>{t('contributions.empty')}</p>
        {/if}
        {#if view.remainder}
          <p>{t('contributions.remainder', { count: view.remainder.count, amount: money(view.remainder.delta) })}</p>
        {/if}
        <p>{t('contributions.total')} {money(view.totalDelta)}</p>
        {#if view.dimension === 'category'}
          {#each transferPreview(view.transfers) as row (row.key)}
            <p>{labeled({ label: row.currentName, code: row.code })} · {t('contributions.transferNote', { previous: row.previousName, current: row.currentName })}</p>
          {/each}
          {#if view.transfers.length > CONTRIBUTION_PREVIEW_LIMIT}
            <p>{t('contributions.transferOmitted', { count: view.transfers.length - CONTRIBUTION_PREVIEW_LIMIT })}</p>
          {/if}
        {/if}
      </section>
    {/each}
  </div>
{/if}
<style>
  .contributions { margin-top: 10px; }
  h3 { margin: 10px 0 4px; font-size: 12px; font-weight: 650; color: var(--md-sys-color-on-surface); }
  ul { margin: 4px 0 0; padding-left: 1.2em; }
  li { overflow-wrap: anywhere; }
</style>
