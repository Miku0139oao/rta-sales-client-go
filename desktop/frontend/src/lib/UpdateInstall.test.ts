import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import UpdateNotice from './UpdateNotice.svelte';
import { configureBackend, invokeNativeUpdate } from './backend';
import { defaultSettings } from './settings';
import type { UpdateStatus } from './updates';
vi.mock('./runtime', () => ({ isWebRuntime: () => false }));
const available: UpdateStatus = { currentVersion: '0.4.5', availableVersion: '0.5.0', phase: 'available', candidateId: 'checked-1', releaseNotes: '<b>text only</b>', changelogVersion: '0.5.0', changelogBody: '<b>text only</b>', installSupported: true, error: '' };
const settings = { ...defaultSettings, locale: 'en' as const, autoCheckUpdates: false };
afterEach(() => { cleanup(); configureBackend(undefined); });

it('requires the explicit dialog confirmation and supports declining without download', async () => {
  const install = vi.fn(); const check = vi.fn(); const onBusy = vi.fn();
  configureBackend({ methods: { GetUpdateStatus: async () => available, InstallUpdate: install, CheckForUpdate: check } });
  render(UpdateNotice, { settings, details: true, onChange: vi.fn(), onBusyChange: onBusy });
  await fireEvent.click(await screen.findByRole('button', { name: 'Download and restart…' }));
  expect(screen.getByRole('dialog')).toHaveTextContent('Unsaved reports and previews will be lost');
  expect(screen.getByRole('dialog')).toHaveTextContent('Accounts and settings are preserved');
  expect(onBusy).toHaveBeenLastCalledWith(true);
  expect(install).not.toHaveBeenCalled();
  await fireEvent.click(screen.getByRole('button', { name: 'Not now' }));
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(install).not.toHaveBeenCalled();
  expect(check).not.toHaveBeenCalled();
});

it('sends only the snapshotted candidate and explicit consent and displays install errors', async () => {
  const install = vi.fn().mockRejectedValue(new Error('publisher verification failed'));
  configureBackend({ methods: { GetUpdateStatus: async () => available, InstallUpdate: install } });
  render(UpdateNotice, { settings, details: true, onChange: vi.fn() });
  await fireEvent.click(await screen.findByRole('button', { name: 'Download and restart…' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Confirm download and restart' }));
  await waitFor(() => expect(install).toHaveBeenCalledWith({ candidateId: 'checked-1', confirmed: true }));
  expect(await screen.findByText('publisher verification failed')).toBeInTheDocument();
  expect(screen.queryByRole('dialog')).toBeNull();
});

it('keeps stage and cancellation state through detail navigation without extra downloads', async () => {
  let status = available;
  let rejectInstall!: (error: Error) => void;
  const install = vi.fn(() => { status = { ...available, phase: 'downloading' }; return new Promise<void>((_resolve, reject) => { rejectInstall = reject; }); });
  const cancel = vi.fn(async () => { status = { ...available, phase: 'idle', candidateId: '', availableVersion: '' }; rejectInstall(new Error('cancelled')); });
  configureBackend({ methods: { GetUpdateStatus: async () => status, InstallUpdate: install, CancelUpdate: cancel } });
  const view = render(UpdateNotice, { settings, details: true, onChange: vi.fn() });
  await fireEvent.click(await screen.findByRole('button', { name: 'Download and restart…' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Confirm download and restart' }));
  await view.rerender({ details: false });
  expect(await screen.findByText('Downloading and verifying the signed update')).toBeInTheDocument();
  expect(install).toHaveBeenCalledTimes(1);
  await fireEvent.click(screen.getByRole('button', { name: 'Cancel update' }));
  await waitFor(() => expect(cancel).toHaveBeenCalledTimes(1));
});

it('blocks initiating installation while analysis/export work is busy', async () => {
  const install = vi.fn();
  configureBackend({ methods: { GetUpdateStatus: async () => available, InstallUpdate: install } });
  render(UpdateNotice, { settings, details: true, busy: true, onChange: vi.fn() });
  expect(await screen.findByRole('button', { name: 'Download and restart…' })).toBeDisabled();
  expect(install).not.toHaveBeenCalled();
});

it('does not offer cancellation after the backend commit boundary', async () => {
  configureBackend({ methods: { GetUpdateStatus: async () => ({ ...available, phase: 'committed' }) } });
  render(UpdateNotice, { settings, details: true, onChange: vi.fn() });
  expect(await screen.findByText('Closing for update and restart')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Cancel update' })).toBeNull();
});

it('native update transport never grants consent by itself', async () => {
  const install = vi.fn(); configureBackend({ methods: { InstallUpdate: install } });
  await invokeNativeUpdate('InstallUpdate', [{ candidateId: 'checked-1', confirmed: false }]);
  expect(install).toHaveBeenCalledWith({ candidateId: 'checked-1', confirmed: false });
});
