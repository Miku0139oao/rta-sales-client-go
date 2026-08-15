import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import type { AnalysisProgress, Profile, ProfileTestResult, ProfileUpsertRequest } from '../types';
import AccountsPage from './AccountsPage.svelte';

const profile: Profile = {
  id: 'profile-1', displayName: 'Primary', enabled: false, priority: 1, hasCredentials: true,
};

function button(container: HTMLElement, label: string): Element {
  const found = [...container.querySelectorAll('md-filled-button, md-filled-tonal-button, md-outlined-button, md-text-button')]
    .find((element) => element.textContent?.includes(label));
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('account safety workflow', () => {
  it('creates profiles disabled and ignores duplicate save submissions', async () => {
    let resolveSave!: (saved: Profile) => void;
    const save = vi.fn((_request: unknown) => new Promise<Profile>((resolve) => { resolveSave = resolve; }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => []),
      CreateOrUpdateProfile: save,
    } });
    const { container } = render(AccountsPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('尚未建立帳號')).toBeInTheDocument());
    await fireEvent.click(button(container, '新增帳號'));
    await fireEvent.input(screen.getByLabelText('顯示名稱'), { target: { value: 'New profile' } });
    await fireEvent.input(screen.getByLabelText('帳號'), { target: { value: 'account' } });
    await fireEvent.input(screen.getByLabelText('密碼'), { target: { value: 'password' } });
    await fireEvent.click(container.querySelector('.app-dialog md-switch')!);

    const form = container.querySelector('.app-dialog form')!;
    await fireEvent.submit(form);
    await fireEvent.submit(form);
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
    expect(screen.getByLabelText('顯示名稱')).toBeDisabled();

    resolveSave({ ...profile, displayName: 'New profile' });
    await waitFor(() => expect(screen.getByText('New profile')).toBeInTheDocument());
  });

  it('clears a successful test and disables the profile after credentials change', async () => {
    const save = vi.fn(async (_request: unknown) => ({ ...profile, displayName: 'Primary' }));
    const testProfile = vi.fn(async (_request: unknown): Promise<ProfileTestResult> => ({ success: true }));
    const enable = vi.fn(async () => ({ ...profile, enabled: true }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile]),
      TestProfile: testProfile,
      Enable: enable,
      CreateOrUpdateProfile: save,
    } });
    const { container } = render(AccountsPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('.profile-actions md-outlined-button')!);
    await waitFor(() => expect(testProfile).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByText('連線測試成功')).toBeInTheDocument());
    expect(enable).toHaveBeenCalledWith({ profileId: profile.id, enabled: true });
    expect(screen.getByText('已啟用')).toBeInTheDocument();
    await fireEvent.click(container.querySelector('[aria-label="編輯 Primary"]')!);
    await fireEvent.input(screen.getByLabelText('新帳號（留空表示不變）'), { target: { value: 'replacement' } });
    await fireEvent.input(screen.getByLabelText('新密碼（留空表示不變）'), { target: { value: 'new-password' } });
    await fireEvent.submit(container.querySelector('.app-dialog form')!);

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
    await waitFor(() => expect(screen.getByText('尚未測試')).toBeInTheDocument());
  });

  it('keeps edit and test available after adding a credentialed profile', async () => {
    const testProfile = vi.fn(async (): Promise<ProfileTestResult> => ({ success: true }));
    const enable = vi.fn(async (input: unknown) => ({
      id: (input as { profileId: string }).profileId,
      displayName: 'Renamed profile', enabled: true, priority: 1, hasCredentials: true,
    } satisfies Profile));
    const save = vi.fn(async (input: unknown) => {
      const request = input as ProfileUpsertRequest;
      return {
        id: request.id ?? 'profile-added',
        displayName: request.displayName,
        enabled: request.enabled,
        priority: 1,
        hasCredentials: true,
      } satisfies Profile;
    });
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => []),
      CreateOrUpdateProfile: save,
      TestProfile: testProfile,
      Enable: enable,
    } });
    const { container } = render(AccountsPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('尚未建立帳號')).toBeInTheDocument());
    await fireEvent.click(button(container, '新增帳號'));
    await fireEvent.input(screen.getByLabelText('顯示名稱'), { target: { value: 'Editable profile' } });
    await fireEvent.input(screen.getByLabelText('帳號'), { target: { value: 'account' } });
    await fireEvent.input(screen.getByLabelText('密碼'), { target: { value: 'password' } });
    await fireEvent.click(button(container, '儲存'));

    await waitFor(() => expect(screen.getByText('Editable profile')).toBeInTheDocument());
    const card = screen.getByText('Editable profile').closest('.profile-card')!;
    const testButton = [...card.querySelectorAll('md-outlined-button')].find((element) => element.textContent?.includes('測試'))!;
    const editButton = card.querySelector('[aria-label="編輯 Editable profile"]')!;
    expect(testButton).toHaveAttribute('disabled', 'false');
    expect(editButton).toHaveAttribute('disabled', 'false');

    await fireEvent.click(editButton);
    await fireEvent.input(screen.getByLabelText('顯示名稱'), { target: { value: 'Renamed profile' } });
    await fireEvent.click(button(container, '儲存'));
    await waitFor(() => expect(screen.getByText('Renamed profile')).toBeInTheDocument());
    expect(save).toHaveBeenLastCalledWith(expect.objectContaining({
      id: 'profile-added', displayName: 'Renamed profile', account: '', password: '',
    }));

    const renamedCard = screen.getByText('Renamed profile').closest('.profile-card')!;
    const testAndEnable = [...renamedCard.querySelectorAll('md-outlined-button')].find((element) => element.textContent?.includes('測試並啟用'))!;
    expect(testAndEnable).toBeTruthy();
    await fireEvent.click(testAndEnable);
    await waitFor(() => expect(testProfile).toHaveBeenCalledWith({ profileId: 'profile-added' }));
    await waitFor(() => expect(enable).toHaveBeenCalledWith({ profileId: 'profile-added', enabled: true }));
    expect(screen.getByText('已啟用')).toBeInTheDocument();
  });

  it('disables testing only when the profile has no stored credentials', async () => {
    const missingCredentials = { ...profile, id: 'profile-missing', displayName: 'Missing', hasCredentials: false };
    const testProfile = vi.fn(async (): Promise<ProfileTestResult> => ({ success: true }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile, missingCredentials]),
      TestProfile: testProfile,
    } });
    render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('Missing')).toBeInTheDocument());

    const availableCard = screen.getByText('Primary').closest('.profile-card')!;
    const missingCard = screen.getByText('Missing').closest('.profile-card')!;
    const availableTest = [...availableCard.querySelectorAll('md-outlined-button')].find((element) => element.textContent?.includes('測試'))!;
    const missingTest = [...missingCard.querySelectorAll('md-outlined-button')].find((element) => element.textContent?.includes('測試'))!;
    expect(availableTest).toHaveAttribute('disabled', 'false');
    expect(missingTest).toHaveAttribute('disabled', 'true');
    expect(missingCard.querySelector('md-switch')).toHaveAttribute('disabled', 'true');

    await fireEvent.click(missingTest);
    expect(testProfile).not.toHaveBeenCalled();
  });

  it('allows one saved credential field to be changed without re-entering the other', async () => {
    const enabledProfile = { ...profile, enabled: true };
    const save = vi.fn(async (_request: unknown) => ({ ...enabledProfile, enabled: false }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [enabledProfile]),
      CreateOrUpdateProfile: save,
    } });
    const { container } = render(AccountsPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('[aria-label="編輯 Primary"]')!);
    const activationSwitch = container.querySelector('.app-dialog md-switch')!;
    expect(activationSwitch).toHaveAttribute('selected', 'true');
    expect(activationSwitch).toHaveAttribute('disabled', 'false');
    await fireEvent.input(screen.getByLabelText('新密碼（留空表示不變）'), { target: { value: 'replacement-password' } });
    expect(activationSwitch).toHaveAttribute('selected', 'false');
    expect(activationSwitch).toHaveAttribute('disabled', 'true');
    await fireEvent.click(button(container, '儲存'));

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({
      id: profile.id, account: '', password: 'replacement-password', enabled: false,
    }));
    expect(screen.queryByText('此欄位為必填')).not.toBeInTheDocument();
  });

  it('publishes busy state and cancels a profile test using its progress operation id', async () => {
    let progressListener: ((payload: unknown) => void) | undefined;
    let rejectTest!: (reason: Error) => void;
    const cancel = vi.fn(async () => { rejectTest(new Error('context canceled')); });
    const onBusyChange = vi.fn();
    configureBackend({
      methods: {
        ListProfiles: vi.fn(async () => [profile]),
        TestProfile: vi.fn(() => new Promise<ProfileTestResult>((_resolve, reject) => { rejectTest = reject; })),
        Cancel: cancel,
      },
      events: {
        on(name, listener) {
          if (name === 'rta:progress') progressListener = listener;
          return () => undefined;
        },
      },
    });
    const { container } = render(AccountsPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW', onBusyChange },
    });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(button(container, '測試'));
    await waitFor(() => expect(onBusyChange).toHaveBeenLastCalledWith(true));
    const cancelButton = button(container, '取消測試');
    expect(cancelButton).toHaveAttribute('disabled');

    progressListener?.({ operationId: 'profile-test-live', stage: 'login', current: 0, total: 1 } satisfies AnalysisProgress);
    await waitFor(() => expect(cancelButton).toHaveAttribute('disabled', 'false'));
    await fireEvent.click(cancelButton);

    expect(cancel).toHaveBeenCalledWith({ operationId: 'profile-test-live' });
    await waitFor(() => expect(onBusyChange).toHaveBeenLastCalledWith(false));
    expect(screen.getByText('尚未測試')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('updates enable state immediately and rolls it back when persistence fails', async () => {
    let rejectEnable!: (reason: Error) => void;
    const enable = vi.fn(() => new Promise<Profile>((_resolve, reject) => { rejectEnable = reject; }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile]),
      Enable: enable,
    } });
    render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    const card = screen.getByText('Primary').closest('.profile-card')!;
    const activation = card.querySelector('md-switch')!;

    await fireEvent.click(activation);
    expect(screen.getByText('已啟用')).toBeInTheDocument();
    expect(activation).toHaveAttribute('aria-busy', 'true');
    expect(enable).toHaveBeenCalledWith({ profileId: profile.id, enabled: true });

    rejectEnable(new Error('failed'));
    await waitFor(() => expect(screen.getByText('已停用')).toBeInTheDocument());
    expect(screen.getByRole('alert')).toBeInTheDocument();
  });

  it('prevents duplicate deletion and keeps a busy modal open', async () => {
    let resolveDelete!: () => void;
    const remove = vi.fn(() => new Promise<void>((resolve) => { resolveDelete = resolve; }));
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile]),
      DeleteProfile: remove,
    } });
    const { container } = render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('.danger-action')!);
    const deleteButton = button(container, '刪除');
    await fireEvent.click(deleteButton);
    await fireEvent.click(deleteButton);

    expect(remove).toHaveBeenCalledTimes(1);
    expect(remove).toHaveBeenCalledWith({ profileId: profile.id });
    const dialog = container.querySelector('.app-dialog')!;
    await fireEvent(dialog, new Event('cancel', { cancelable: true }));
    expect(container.querySelector('.app-dialog')).toBeInTheDocument();

    resolveDelete();
    await waitFor(() => expect(container.querySelector('.app-dialog')).not.toBeInTheDocument());
    expect(screen.queryByText('Primary')).not.toBeInTheDocument();
  });

  it('shows a failed deletion inside the still-open modal', async () => {
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile]),
      DeleteProfile: vi.fn(async () => { throw new Error('failed'); }),
    } });
    const { container } = render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('.danger-action')!);
    await fireEvent.click(button(container, '刪除'));

    const dialog = container.querySelector('.app-dialog')!;
    await waitFor(() => expect(dialog.querySelector('[role="alert"]')).toHaveTextContent('桌面服務發生錯誤，請再試一次。'));
    expect(dialog).toBeInTheDocument();
  });

  it('reorders with the up and down buttons and has no drag handle', async () => {
    const profiles: Profile[] = [
      { ...profile, id: 'a', displayName: 'A', priority: 1 },
      { ...profile, id: 'b', displayName: 'B', priority: 2 },
      { ...profile, id: 'c', displayName: 'C', priority: 3 },
    ];
    const reorder = vi.fn(async (input: unknown) => {
      const ids = (input as { profileIds: string[] }).profileIds;
      return ids.map((id, index) => ({ ...profiles.find((candidate) => candidate.id === id)!, priority: index + 1 }));
    });
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => profiles),
      Reorder: reorder,
    } });
    const { container } = render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('C')).toBeInTheDocument());
    expect(container.querySelector('.drag-handle')).toBeNull();
    await fireEvent.click(container.querySelector('[aria-label="下移 A"]')!);
    await waitFor(() => expect(reorder).toHaveBeenCalledWith({ profileIds: ['b', 'a', 'c'] }));
  });

  it('rolls the visible order back when persistence fails', async () => {
    const second = { ...profile, id: 'profile-2', displayName: 'Secondary', priority: 2 };
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile, second]),
      Reorder: vi.fn(async () => { throw new Error('failed'); }),
    } });
    const { container } = render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('Secondary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('[aria-label="下移 Primary"]')!);

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    expect([...container.querySelectorAll('.profile-card h2')].map((heading) => heading.textContent)).toEqual(['Primary', 'Secondary']);
  });

  it('restores opener focus after Escape and closes on a backdrop click', async () => {
    configureBackend({ methods: { ListProfiles: vi.fn(async () => [profile]) } });
    const { container } = render(AccountsPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    const edit = container.querySelector<HTMLElement>('[aria-label="編輯 Primary"]')!;
    edit.tabIndex = 0;
    edit.focus();
    await fireEvent.click(edit);
    await waitFor(() => expect(document.activeElement).toBe(container.querySelector('#profile-name')));

    await fireEvent(container.querySelector('.app-dialog')!, new Event('cancel', { cancelable: true }));
    await waitFor(() => expect(container.querySelector('.app-dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(document.activeElement).toBe(edit));

    await fireEvent.click(edit);
    await waitFor(() => expect(container.querySelector('.app-dialog')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('.app-dialog')!, { clientX: -1, clientY: -1 });
    await waitFor(() => expect(container.querySelector('.app-dialog')).not.toBeInTheDocument());
  });
});
