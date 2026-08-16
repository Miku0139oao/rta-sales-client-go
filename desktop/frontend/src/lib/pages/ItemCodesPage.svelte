<script lang="ts">
  import { onMount } from 'svelte';
  import { backend } from '../backend';
  import { errorMessage, type Translator } from '../i18n';
  import { splitManCodes } from '../manCodes';
  import { modal } from '../modal';
  import type { Locale, ManCodeGroup } from '../types';

  export let t: Translator;
  export let locale: Locale;
  export let onBusyChange: (busy: boolean) => void = () => undefined;

  let groups: ManCodeGroup[] = [];
  let articleNames: Record<string, string> = {};
  let loading = true;
  let error = '';
  let dialogError = '';
  let duplicateNotice = '';
  let search = '';
  let expandedIds = new Set<string>();
  let pasteDrafts: Record<string, string> = {};
  let editorOpen = false;
  let renaming: ManCodeGroup | undefined;
  let deleting: ManCodeGroup | undefined;
  let groupName = '';
  let nameError = '';
  let saving = false;
  let deletingBusy = false;
  let codesBusyId = '';

  $: pageBusy = Boolean(saving || deletingBusy || codesBusyId);
  $: onBusyChange(pageBusy);
  $: query = search.trim().toLowerCase();
  $: visibleGroups = query
    ? groups.filter((group) => group.name.toLowerCase().includes(query) || group.codes.some((code) => code.toLowerCase().includes(query)))
    : groups;
  $: expandGroupsMatchingQuery(query, groups);

  let appliedSearchExpand = '';

  function expandGroupsMatchingQuery(nextQuery: string, nextGroups: ManCodeGroup[]) {
    const key = `${nextQuery}\0${nextGroups.map((group) => group.id).join(',')}`;
    if (key === appliedSearchExpand) return;
    appliedSearchExpand = key;
    if (!nextQuery) return;
    const next = new Set(expandedIds);
    for (const group of nextGroups) {
      if (group.codes.some((code) => code.toLowerCase().includes(nextQuery))) next.add(group.id);
    }
    expandedIds = next;
  }

  onMount(() => {
    void load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      groups = await backend.listManCodeGroups();
    } catch (caught) {
      error = errorMessage(locale, caught);
    } finally {
      loading = false;
    }
    try {
      articleNames = await backend.getLatestArticleNames();
    } catch {
      articleNames = {};
    }
  }

  function toggleExpand(group: ManCodeGroup) {
    const next = new Set(expandedIds);
    if (next.has(group.id)) next.delete(group.id);
    else next.add(group.id);
    expandedIds = next;
  }

  function openCreate() {
    if (pageBusy) return;
    error = '';
    dialogError = '';
    nameError = '';
    renaming = undefined;
    groupName = '';
    editorOpen = true;
  }

  function openRename(group: ManCodeGroup) {
    if (pageBusy) return;
    error = '';
    dialogError = '';
    nameError = '';
    renaming = group;
    groupName = group.name;
    editorOpen = true;
  }

  function closeEditor() {
    editorOpen = false;
    renaming = undefined;
    groupName = '';
    nameError = '';
    dialogError = '';
  }

  function nameTaken(name: string, id?: string): boolean {
    return groups.some((group) => group.name === name && group.id !== id);
  }

  async function saveGroup() {
    if (saving) return;
    const name = groupName.trim();
    nameError = '';
    dialogError = '';
    if (!name) {
      nameError = t('common.required');
      return;
    }
    if (nameTaken(name, renaming?.id)) {
      nameError = t('itemcodes.nameTaken');
      return;
    }
    saving = true;
    error = '';
    try {
      const saved = await backend.saveManCodeGroup(renaming ? { id: renaming.id, name } : { name });
      groups = renaming
        ? groups.map((group) => group.id === saved.id ? saved : group)
        : [...groups, saved];
      if (!renaming) {
        const next = new Set(expandedIds);
        next.add(saved.id);
        expandedIds = next;
      }
      closeEditor();
    } catch (caught) {
      dialogError = errorMessage(locale, caught);
    } finally {
      saving = false;
    }
  }

  function openDelete(group: ManCodeGroup) {
    if (pageBusy) return;
    error = '';
    dialogError = '';
    deleting = group;
  }

  function closeDelete() {
    if (deletingBusy) return;
    deleting = undefined;
    dialogError = '';
  }

  async function removeGroup() {
    if (!deleting || deletingBusy || saving || codesBusyId) return;
    const groupId = deleting.id;
    deletingBusy = true;
    error = '';
    dialogError = '';
    try {
      await backend.deleteManCodeGroup(groupId);
      groups = groups.filter((group) => group.id !== groupId);
      const next = new Set(expandedIds);
      next.delete(groupId);
      expandedIds = next;
      deleting = undefined;
    } catch (caught) {
      dialogError = errorMessage(locale, caught);
    } finally {
      deletingBusy = false;
    }
  }

  async function addCodes(group: ManCodeGroup) {
    if (pageBusy) return;
    const incoming = splitManCodes(pasteDrafts[group.id] ?? '');
    if (incoming.length === 0) return;
    const existing = new Set(group.codes);
    const added: string[] = [];
    const seen = new Set<string>();
    let skipped = 0;
    for (const code of incoming) {
      if (existing.has(code) || seen.has(code)) {
        skipped += 1;
        continue;
      }
      seen.add(code);
      added.push(code);
    }
    duplicateNotice = skipped ? t('itemcodes.duplicateNotice', { count: skipped }) : '';
    if (added.length === 0) {
      pasteDrafts = { ...pasteDrafts, [group.id]: '' };
      return;
    }
    codesBusyId = group.id;
    error = '';
    try {
      const saved = await backend.replaceManCodeGroupCodes({ id: group.id, codes: [...group.codes, ...added] });
      groups = groups.map((candidate) => candidate.id === saved.id ? saved : candidate);
      pasteDrafts = { ...pasteDrafts, [group.id]: '' };
    } catch (caught) {
      error = errorMessage(locale, caught);
    } finally {
      codesBusyId = '';
    }
  }

  async function removeCode(group: ManCodeGroup, code: string) {
    if (pageBusy) return;
    codesBusyId = group.id;
    error = '';
    try {
      const saved = await backend.replaceManCodeGroupCodes({
        id: group.id,
        codes: group.codes.filter((candidate) => candidate !== code),
      });
      groups = groups.map((candidate) => candidate.id === saved.id ? saved : candidate);
    } catch (caught) {
      error = errorMessage(locale, caught);
    } finally {
      codesBusyId = '';
    }
  }
</script>

<section class="page itemcodes-page" aria-labelledby="itemcodes-title">
  <div class="page-heading split-heading">
    <div>
      <h1 id="itemcodes-title">{t('itemcodes.title')}</h1>
    </div>
    <md-filled-button onclick={openCreate} disabled={pageBusy}>
      <span class="material-symbols-rounded" slot="icon">add</span>{t('itemcodes.add')}
    </md-filled-button>
  </div>

  <div class="field-group itemcodes-search">
    <label for="itemcodes-search">{t('itemcodes.search')}</label>
    <input id="itemcodes-search" bind:value={search} autocomplete="off" />
  </div>

  {#if error}
    <div class="notice error-notice" role="alert">
      <span class="material-symbols-rounded" aria-hidden="true">error</span>
      <div><strong>{t('error.title')}</strong><p>{error}</p></div>
    </div>
  {/if}

  {#if duplicateNotice}
    <div class="notice warning-notice" role="status">
      <span class="material-symbols-rounded" aria-hidden="true">info</span>
      <span>{duplicateNotice}</span>
    </div>
  {/if}

  {#if loading}
    <div class="loading-state" aria-live="polite">
      <md-circular-progress indeterminate></md-circular-progress>
      <span>{t('common.loading')}</span>
    </div>
  {:else if groups.length === 0}
    <div class="empty-state surface-card">
      <span class="material-symbols-rounded" aria-hidden="true">qr_code_2</span>
      <h2>{t('itemcodes.emptyTitle')}</h2>
      <md-filled-tonal-button onclick={openCreate}>{t('itemcodes.add')}</md-filled-tonal-button>
    </div>
  {:else if visibleGroups.length === 0}
    <div class="empty-state surface-card">
      <span class="material-symbols-rounded" aria-hidden="true">search_off</span>
      <h2>{t('itemcodes.noMatches')}</h2>
    </div>
  {:else}
    <ol class="itemcode-list" aria-label={t('itemcodes.title')}>
      {#each visibleGroups as group (group.id)}
        {@const expanded = expandedIds.has(group.id)}
        <li class="surface-card itemcode-card" data-group-id={group.id}>
          <div class="itemcode-main">
            <div class="profile-title-row">
              <h2>{group.name}</h2>
              <span class="state-pill">{t('itemcodes.codeCount', { count: group.codes.length })}</span>
            </div>
          </div>
          <div class="profile-actions">
            <md-outlined-button
              aria-expanded={expanded}
              aria-label={`${expanded ? t('itemcodes.collapse') : t('itemcodes.expand')} ${group.name}`}
              disabled={pageBusy}
              onclick={() => toggleExpand(group)}
            >
              <span class="material-symbols-rounded" slot="icon">{expanded ? 'expand_less' : 'expand_more'}</span>
              {expanded ? t('itemcodes.collapse') : t('itemcodes.expand')}
            </md-outlined-button>
            <md-outlined-button aria-label={`${t('itemcodes.rename')} ${group.name}`} disabled={pageBusy} onclick={() => openRename(group)}>
              <span class="material-symbols-rounded" slot="icon">edit</span>{t('itemcodes.rename')}
            </md-outlined-button>
            <md-icon-button class="danger-action" aria-label={`${t('common.delete')} ${group.name}`} disabled={pageBusy} onclick={() => openDelete(group)}>
              <span class="material-symbols-rounded">delete</span>
            </md-icon-button>
          </div>
          {#if expanded}
            <div class="itemcode-body">
              {#if group.codes.length === 0}
                <p class="itemcode-empty">{t('itemcodes.noCodes')}</p>
              {:else}
                <ul class="itemcode-codes">
                  {#each group.codes as code (code)}
                    <li>
                      <span class="itemcode-value">
                        <strong>{code}</strong>
                        {#if articleNames[code]}<span class="itemcode-article">{articleNames[code]}</span>{/if}
                      </span>
                      <md-icon-button
                        class="danger-action"
                        aria-label={t('itemcodes.deleteCode', { code })}
                        disabled={pageBusy}
                        onclick={() => removeCode(group, code)}
                      >
                        <span class="material-symbols-rounded">close</span>
                      </md-icon-button>
                    </li>
                  {/each}
                </ul>
              {/if}
              <form class="itemcode-paste" onsubmit={(event) => { event.preventDefault(); void addCodes(group); }}>
                <div class="field-group">
                  <label for={`itemcodes-paste-${group.id}`}>{t('itemcodes.pasteLabel')}</label>
                  <textarea
                    id={`itemcodes-paste-${group.id}`}
                    value={pasteDrafts[group.id] ?? ''}
                    oninput={(event) => { pasteDrafts = { ...pasteDrafts, [group.id]: (event.currentTarget as HTMLTextAreaElement).value }; }}
                    disabled={pageBusy}
                    rows="3"
                  ></textarea>
                  <small class="field-hint">{t('itemcodes.pasteHint')}</small>
                </div>
                <md-filled-tonal-button type="submit" disabled={pageBusy || !splitManCodes(pasteDrafts[group.id] ?? '').length} onclick={() => void addCodes(group)}>
                  {codesBusyId === group.id ? t('itemcodes.addingCodes') : t('itemcodes.addCodes')}
                </md-filled-tonal-button>
              </form>
            </div>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</section>

{#if editorOpen}
  <dialog use:modal={{ busy: saving, onClose: closeEditor }} class="app-dialog" aria-modal="true" aria-labelledby="itemcodes-dialog-title">
    <div class="dialog-header">
      <div>
        <h2 id="itemcodes-dialog-title">{renaming ? t('itemcodes.dialogRename') : t('itemcodes.dialogAdd')}</h2>
      </div>
      <md-icon-button aria-label={t('common.close')} onclick={closeEditor} disabled={saving}><span class="material-symbols-rounded">close</span></md-icon-button>
    </div>
    <form onsubmit={(event) => { event.preventDefault(); void saveGroup(); }}>
      {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
      <div class="field-group">
        <label for="itemcodes-group-name">{t('itemcodes.groupName')}</label>
        <input id="itemcodes-group-name" bind:value={groupName} disabled={saving} autocomplete="off" data-autofocus aria-invalid={Boolean(nameError)} aria-describedby={nameError ? 'itemcodes-group-name-error' : undefined} />
        {#if nameError}<small class="field-error" id="itemcodes-group-name-error">{nameError}</small>{/if}
      </div>
      <div class="dialog-actions">
        <md-text-button type="button" onclick={closeEditor} disabled={saving}>{t('common.cancel')}</md-text-button>
        <md-filled-button type="submit" onclick={saveGroup} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</md-filled-button>
      </div>
    </form>
  </dialog>
{/if}

{#if deleting}
  <dialog use:modal={{ busy: deletingBusy, onClose: closeDelete }} class="app-dialog compact-dialog" aria-modal="true" aria-labelledby="itemcodes-delete-title" aria-describedby="itemcodes-delete-body">
    <form class="confirmation-form" onsubmit={(event) => { event.preventDefault(); void removeGroup(); }}>
      <div class="dialog-symbol danger-symbol"><span class="material-symbols-rounded" aria-hidden="true">delete_forever</span></div>
      <h2 id="itemcodes-delete-title">{t('itemcodes.deleteTitle')}</h2>
      <p id="itemcodes-delete-body">{t('itemcodes.deleteBody', { name: deleting.name })}</p>
      {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
      <div class="dialog-actions">
        <md-text-button type="button" onclick={closeDelete} disabled={deletingBusy}>{t('common.cancel')}</md-text-button>
        <md-filled-button type="submit" class="danger-button" onclick={removeGroup} disabled={deletingBusy} data-autofocus>{deletingBusy ? t('common.deleting') : t('common.delete')}</md-filled-button>
      </div>
    </form>
  </dialog>
{/if}
