import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { configureBackend } from '../backend';
import { translator } from '../i18n';
import type { AnalysisProgress, Profile, ProfileTestResult } from '../types';
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
    configureBackend({ methods: {
      ListProfiles: vi.fn(async () => [profile]),
      TestProfile: testProfile,
      CreateOrUpdateProfile: save,
    } });
    const { container } = render(AccountsPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('.profile-actions md-outlined-button')!);
    await waitFor(() => expect(testProfile).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByText('連線測試成功')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('md-icon-button[aria-label="編輯 Primary"]')!);
    await fireEvent.input(screen.getByLabelText('帳號'), { target: { value: 'replacement' } });
    await fireEvent.input(screen.getByLabelText('新密碼（留空表示不變）'), { target: { value: 'new-password' } });
    await fireEvent.submit(container.querySelector('.app-dialog form')!);

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
    await waitFor(() => expect(screen.getByText('尚未測試')).toBeInTheDocument());
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
});
