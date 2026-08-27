<script lang="ts">
  import { onMount } from 'svelte';
  import { backend } from '../backend';
  import { errorMessage, type Translator } from '../i18n';
  import { isWebRuntime } from '../runtime';
  import { modal } from '../modal';
  import type { AnalysisProgress, Locale, Profile, ProfileTestResult, ProfileUpsertRequest } from '../types';

  export let t: Translator;
  export let locale: Locale;
  export let onBusyChange: (busy: boolean) => void = () => undefined;

  let profiles: Profile[] = [];
  let loading = true;
  let error = '';
  let dialogError = '';
  let editorOpen = false;
  let editing: Profile | undefined;
  let deleting: Profile | undefined;
  let saving = false;
  let deletingBusy = false;
  let enablePendingId = '';
  let reorderBusy = false;
  let testingId = '';
  let testOperationId = '';
  let cancellingTest = false;
  let testResults = new Map<string, ProfileTestResult>();
  let form: ProfileUpsertRequest = emptyForm();
  let formErrors: { displayName?: string; account?: string; password?: string } = {};

  function emptyForm(): ProfileUpsertRequest {
    return { displayName: '', account: '', password: '', enabled: false };
  }

  function closeEditor() {
    editorOpen = false;
    editing = undefined;
    form = emptyForm();
    formErrors = {};
    dialogError = '';
  }

  $: accountBusy = Boolean(testingId || saving || deletingBusy || enablePendingId || reorderBusy);
  $: activationLocked = !editing || !editing.hasCredentials || Boolean(form.account.trim() || form.password);
  $: onBusyChange(accountBusy);

  onMount(() => {
    void loadProfiles();
    return backend.onProgress((next: AnalysisProgress) => {
      if (testingId && next.operationId) testOperationId = next.operationId;
    });
  });

  async function loadProfiles() {
    loading = true;
    error = '';
    try {
      profiles = (await backend.listProfiles()).sort((a, b) => a.priority - b.priority);
    } catch (caught) {
      error = errorMessage(locale, caught);
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    if (accountBusy) return;
    error = '';
    dialogError = '';
    editing = undefined;
    form = emptyForm();
    formErrors = {};
    editorOpen = true;
  }

  function openEdit(profile: Profile) {
    if (accountBusy) return;
    error = '';
    dialogError = '';
    editing = profile;
    form = {
      id: profile.id,
      displayName: profile.displayName,
      account: '',
      password: '',
      enabled: profile.enabled,
    };
    formErrors = {};
    editorOpen = true;
  }

  function validate(): boolean {
    formErrors = {};
    if (!form.displayName.trim()) formErrors.displayName = t('common.required');
    if (!editing && !form.account.trim()) formErrors.account = t('common.required');
    if (!editing && !form.password) formErrors.password = t('common.required');
    if (editing && !editing.hasCredentials && (form.account.trim() || form.password)) {
      if (!form.account.trim()) formErrors.account = t('common.required');
      if (!form.password) formErrors.password = t('common.required');
    }
    return Object.keys(formErrors).length === 0;
  }

  async function saveProfile() {
    if (saving || !validate()) return;
    saving = true;
    error = '';
    dialogError = '';
    const credentialsChanged = !editing || Boolean(form.account.trim() || form.password);
    try {
      const saved = await backend.saveProfile({
        ...form,
        displayName: form.displayName.trim(),
        account: form.account.trim(),
        enabled: credentialsChanged ? false : form.enabled,
      });
      profiles = editing
        ? profiles.map((profile) => profile.id === saved.id ? saved : profile).sort((a, b) => a.priority - b.priority)
        : [...profiles, saved].sort((a, b) => a.priority - b.priority);
      if (credentialsChanged) {
        const nextResults = new Map(testResults);
        nextResults.delete(saved.id);
        testResults = nextResults;
      }
      closeEditor();
    } catch (caught) {
      dialogError = errorMessage(locale, caught);
    } finally {
      saving = false;
    }
  }

  async function test(profile: Profile) {
    if (accountBusy || !profile.hasCredentials) return;
    testingId = profile.id;
    testOperationId = '';
    cancellingTest = false;
    error = '';
    try {
      const result = await backend.testProfile(profile.id);
      testResults = new Map(testResults).set(profile.id, result);
      if (result.success && !profile.enabled) {
        try {
          const updated = await backend.setProfileEnabled(profile.id, true);
          profiles = profiles.map((candidate) => candidate.id === updated.id ? updated : candidate);
        } catch (caught) {
          error = errorMessage(locale, caught);
        }
      }
    } catch (caught) {
      if (!cancellingTest) {
        testResults = new Map(testResults).set(profile.id, { success: false });
        error = errorMessage(locale, caught);
      }
    } finally {
      testingId = '';
      testOperationId = '';
      cancellingTest = false;
    }
  }

  async function cancelTest() {
    if (!testingId || !testOperationId || cancellingTest) return;
    cancellingTest = true;
    try {
      await backend.cancelAnalysis(testOperationId);
    } catch (caught) {
      cancellingTest = false;
      error = errorMessage(locale, caught);
    }
  }

  async function toggleEnabled(profile: Profile) {
    if (accountBusy) return;
    const previous = profiles;
    const enabled = !profile.enabled;
    profiles = profiles.map((candidate) => candidate.id === profile.id ? { ...candidate, enabled } : candidate);
    enablePendingId = profile.id;
    error = '';
    try {
      const updated = await backend.setProfileEnabled(profile.id, enabled);
      profiles = profiles.map((candidate) => candidate.id === updated.id ? updated : candidate);
    } catch (caught) {
      profiles = previous;
      error = errorMessage(locale, caught);
    } finally {
      enablePendingId = '';
    }
  }

  async function removeProfile() {
    if (!deleting || deletingBusy || testingId || saving || enablePendingId || reorderBusy) return;
    const profileId = deleting.id;
    deletingBusy = true;
    error = '';
    dialogError = '';
    try {
      await backend.deleteProfile(profileId);
      profiles = profiles.filter((profile) => profile.id !== profileId).map((profile, index) => ({ ...profile, priority: index + 1 }));
      deleting = undefined;
    } catch (caught) {
      dialogError = errorMessage(locale, caught);
    } finally {
      deletingBusy = false;
    }
  }

  function openDelete(profile: Profile) {
    if (accountBusy) return;
    error = '';
    dialogError = '';
    deleting = profile;
  }

  function closeDelete() {
    if (deletingBusy) return;
    deleting = undefined;
    dialogError = '';
  }

  async function move(profileId: string, direction: -1 | 1) {
    if (accountBusy) return;
    const index = profiles.findIndex((profile) => profile.id === profileId);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= profiles.length) return;
    const reordered = [...profiles];
    [reordered[index], reordered[target]] = [reordered[target], reordered[index]];
    await persistOrder(reordered, profiles);
  }

  async function persistOrder(reordered: Profile[], previous: Profile[]) {
    if (reorderBusy) return;
    reorderBusy = true;
    profiles = reordered.map((profile, index) => ({ ...profile, priority: index + 1 }));
    error = '';
    try {
      profiles = (await backend.reorderProfiles(profiles.map((profile) => profile.id))).sort((a, b) => a.priority - b.priority);
    } catch (caught) {
      profiles = previous;
      error = errorMessage(locale, caught);
    } finally {
      reorderBusy = false;
    }
  }

  function statusFor(profile: Profile, results: Map<string, ProfileTestResult>): { className: string; icon: string; label: string } {
    const result = results.get(profile.id);
    if (result) return result.success
      ? { className: 'success', icon: 'check_circle', label: t('accounts.testSuccess') }
      : { className: 'danger', icon: 'error', label: t('accounts.testFailed') };
    if (profile.lastTestStatus === 'success') return { className: 'success', icon: 'check_circle', label: t('accounts.testSuccess') };
    if (profile.lastTestStatus === 'failed') return { className: 'danger', icon: 'error', label: t('accounts.testFailed') };
    return { className: 'neutral', icon: 'pending', label: t('accounts.testUntested') };
  }
</script>

<section class="page accounts-page" aria-labelledby="accounts-title">
  <div class="page-heading split-heading">
    <div>
      <h1 id="accounts-title">{t('accounts.title')}</h1>
    </div>
    <md-filled-button onclick={openCreate} disabled={accountBusy || Boolean(error && profiles.length === 0)}>
      <span class="material-symbols-rounded" slot="icon">add</span>{t('accounts.add')}
    </md-filled-button>
  </div>

  {#if isWebRuntime()}
    <div class="notice warning-notice" role="status">
      <span class="material-symbols-rounded" aria-hidden="true">info</span>
      <span>{t('web.accountsHint')}</span>
    </div>
  {/if}

  {#if error}
    <div class="notice error-notice" role="alert">
      <span class="material-symbols-rounded" aria-hidden="true">error</span>
      <div><strong>{t('error.title')}</strong><p>{error}</p></div>
    </div>
  {/if}

  {#if loading}
    <div class="loading-state" aria-live="polite">
      <md-circular-progress indeterminate></md-circular-progress>
      <span>{t('common.loading')}</span>
    </div>
  {:else if error && profiles.length === 0}
    <div class="empty-state surface-card">
      <span class="material-symbols-rounded" aria-hidden="true">error</span>
      <h2>{t('error.title')}</h2>
      <md-filled-tonal-button onclick={() => void loadProfiles()}>{t('common.retry')}</md-filled-tonal-button>
    </div>
  {:else if profiles.length === 0}
    <div class="empty-state surface-card">
      <span class="material-symbols-rounded" aria-hidden="true">person_add</span>
      <h2>{t('accounts.emptyTitle')}</h2>
      <md-filled-tonal-button onclick={openCreate}>{t('accounts.add')}</md-filled-tonal-button>
    </div>
  {:else}
    <ol class="profile-list" aria-label={t('accounts.title')}>
      {#each profiles as profile, index (profile.id)}
        {@const status = statusFor(profile, testResults)}
        <li
          class="surface-card profile-card"
          class:disabled-card={!profile.enabled}
          data-profile-id={profile.id}
        >
          <div class="priority-badge" aria-label={t('accounts.priority', { value: index + 1 })}>{index + 1}</div>
          <div class="profile-main">
            <div class="profile-title-row">
              <h2>{profile.displayName}</h2>
              <span class:profile-enabled={profile.enabled} class="state-pill">{profile.enabled ? t('common.enabled') : t('common.disabled')}</span>
            </div>
            <div class="profile-meta">
              <span><span class="material-symbols-rounded" aria-hidden="true">{profile.hasCredentials ? 'key' : 'key_off'}</span>{profile.hasCredentials ? t('accounts.credentialsStored') : t('accounts.credentialsMissing')}</span>
              {#if profile.accountHint}<span><span class="material-symbols-rounded" aria-hidden="true">person</span>{profile.accountHint}</span>{/if}
              <span class={status.className}><span class="material-symbols-rounded" aria-hidden="true">{status.icon}</span>{status.label}</span>
            </div>
          </div>
          <div class="profile-order-actions">
            <md-icon-button aria-label={t('accounts.moveUp', { name: profile.displayName })} disabled={accountBusy || index === 0} onclick={() => move(profile.id, -1)}>
              <span class="material-symbols-rounded">arrow_upward</span>
            </md-icon-button>
            <md-icon-button aria-label={t('accounts.moveDown', { name: profile.displayName })} disabled={accountBusy || index === profiles.length - 1} onclick={() => move(profile.id, 1)}>
              <span class="material-symbols-rounded">arrow_downward</span>
            </md-icon-button>
          </div>
          <div class="profile-actions">
            <md-switch
              aria-label={`${profile.displayName}: ${profile.enabled ? t('common.enabled') : t('common.disabled')}`}
              selected={profile.enabled}
              disabled={accountBusy || (!profile.hasCredentials && !profile.enabled)}
              aria-busy={enablePendingId === profile.id}
              onclick={() => toggleEnabled(profile)}
            ></md-switch>
            <md-outlined-button
              onclick={() => test(profile)}
              disabled={accountBusy || !profile.hasCredentials}
              title={!profile.hasCredentials ? t('accounts.testMissingCredentials') : ''}
            >
              {testingId === profile.id ? t('common.testing') : profile.enabled ? t('common.test') : t('accounts.testAndEnable')}
            </md-outlined-button>
            {#if testingId === profile.id}
              <md-text-button onclick={cancelTest} disabled={!testOperationId || cancellingTest}>{t('accounts.cancelTest')}</md-text-button>
            {/if}
            <md-outlined-button aria-label={`${t('common.edit')} ${profile.displayName}`} disabled={accountBusy} onclick={() => openEdit(profile)}>
              <span class="material-symbols-rounded" slot="icon">edit</span>{t('common.edit')}
            </md-outlined-button>
            <md-icon-button class="danger-action" aria-label={`${t('common.delete')} ${profile.displayName}`} disabled={accountBusy} onclick={() => openDelete(profile)}><span class="material-symbols-rounded">delete</span></md-icon-button>
          </div>
        </li>
      {/each}
    </ol>
  {/if}
</section>

{#if editorOpen}
  <dialog use:modal={{ busy: saving, onClose: closeEditor }} class="app-dialog" aria-modal="true" aria-labelledby="profile-dialog-title">
    <div class="dialog-header">
      <div>
        <h2 id="profile-dialog-title">{editing ? t('accounts.dialogEdit') : t('accounts.dialogAdd')}</h2>
      </div>
      <md-icon-button aria-label={t('common.close')} onclick={closeEditor} disabled={saving}><span class="material-symbols-rounded">close</span></md-icon-button>
    </div>
    <form onsubmit={(event) => { event.preventDefault(); saveProfile(); }}>
      {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
      <div class="field-group">
        <label for="profile-name">{t('accounts.displayName')}</label>
        <input id="profile-name" bind:value={form.displayName} disabled={saving} autocomplete="off" data-autofocus aria-invalid={Boolean(formErrors.displayName)} aria-describedby={formErrors.displayName ? 'profile-name-error' : undefined} />
        {#if formErrors.displayName}<small class="field-error" id="profile-name-error">{formErrors.displayName}</small>{/if}
      </div>
      <div class="field-group">
        <label for="profile-account">{editing ? t('accounts.accountEdit') : t('accounts.account')}</label>
        <input id="profile-account" bind:value={form.account} disabled={saving} autocomplete="username" aria-invalid={Boolean(formErrors.account)} aria-describedby={formErrors.account ? 'profile-account-error' : undefined} />
        {#if formErrors.account}<small class="field-error" id="profile-account-error">{formErrors.account}</small>{/if}
      </div>
      <div class="field-group">
        <label for="profile-password">{editing ? t('accounts.passwordEdit') : t('accounts.passwordNew')}</label>
        <input id="profile-password" type="password" bind:value={form.password} disabled={saving} autocomplete="new-password" aria-invalid={Boolean(formErrors.password)} aria-describedby={formErrors.password ? 'profile-password-error' : undefined} />
        {#if formErrors.password}<small id="profile-password-error" class="field-error">{formErrors.password}</small>{/if}
      </div>
      <div class="setting-row dialog-setting-row">
        <strong>{t('common.enabled')}</strong>
        <md-switch
          aria-label={t('common.enabled')}
          selected={activationLocked ? false : form.enabled}
          disabled={saving || activationLocked}
          onclick={() => { if (!activationLocked) form = { ...form, enabled: !form.enabled }; }}
        ></md-switch>
      </div>
      <div class="dialog-actions">
        <md-text-button type="button" onclick={closeEditor} disabled={saving}>{t('common.cancel')}</md-text-button>
        <md-filled-button type="submit" onclick={saveProfile} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</md-filled-button>
      </div>
    </form>
  </dialog>
{/if}

{#if deleting}
  <dialog use:modal={{ busy: deletingBusy, onClose: closeDelete }} class="app-dialog compact-dialog" aria-modal="true" aria-labelledby="delete-dialog-title" aria-describedby="delete-dialog-body">
    <form class="confirmation-form" onsubmit={(event) => { event.preventDefault(); removeProfile(); }}>
      <div class="dialog-symbol danger-symbol"><span class="material-symbols-rounded" aria-hidden="true">delete_forever</span></div>
      <h2 id="delete-dialog-title">{t('accounts.deleteTitle')}</h2>
      <p id="delete-dialog-body">{t('accounts.deleteBody', { name: deleting.displayName })}</p>
      {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
      <div class="dialog-actions">
        <md-text-button type="button" onclick={closeDelete} disabled={deletingBusy}>{t('common.cancel')}</md-text-button>
        <md-filled-button type="submit" class="danger-button" onclick={removeProfile} disabled={deletingBusy} data-autofocus>{deletingBusy ? t('common.deleting') : t('common.delete')}</md-filled-button>
      </div>
    </form>
  </dialog>
{/if}
