import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import UpdateNotice from './UpdateNotice.svelte';
import { configureBackend, beginNativeExportLease, endNativeExportLease } from './backend';
import { defaultSettings, loadSettings, saveSettings } from './settings';

const state = vi.hoisted(() => ({ web: false }));
vi.mock('./runtime', () => ({ isWebRuntime: () => state.web }));
const status = { currentVersion: '0.4.5', phase: 'idle', candidateId: '', availableVersion: '', releaseNotes: '', installSupported: false, error: '' };
afterEach(() => { cleanup(); configureBackend(undefined); state.web = false; localStorage.clear(); });
function setup(check = vi.fn().mockResolvedValue({ ...status, phase: 'current' })) {
  const install = vi.fn();
  const get = vi.fn().mockResolvedValue(status);
  configureBackend({ methods: { GetUpdateStatus: get, CheckForUpdate: check, InstallUpdate: install } });
  return { get, check, install };
}
describe('portable update notice', () => {
  it('checks once, persists across detail navigation and never installs automatically', async () => {
    const methods = setup();
    const view = render(UpdateNotice, { settings: defaultSettings, details: false, onChange: vi.fn() });
    await waitFor(() => expect(methods.check).toHaveBeenCalledTimes(1));
    await view.rerender({ details: true });
    expect(await screen.findByText(/0.4.5/)).toBeInTheDocument();
    expect(methods.check).toHaveBeenCalledTimes(1);
    expect(methods.install).not.toHaveBeenCalled();
  });
  it('preserves opt-out under the existing storage key and allows a manual check', async () => {
    const methods = setup();
    saveSettings({ ...defaultSettings, autoCheckUpdates: false });
    expect(JSON.parse(localStorage.getItem('rta-sales-desktop-settings-v2')!).autoCheckUpdates).toBe(false);
    render(UpdateNotice, { settings: { ...loadSettings(), locale: 'en' }, details: true, onChange: vi.fn() });
    await screen.findByText(/0.4.5/);
    expect(methods.check).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: 'Check for updates' }));
    await waitFor(() => expect(methods.check).toHaveBeenCalledTimes(1));
    expect(methods.install).not.toHaveBeenCalled();
  });
  it('escapes release notes and clearly disables automatic installation', async () => {
    const methods = setup(vi.fn().mockResolvedValue({ ...status, phase: 'available', availableVersion: '0.5.0', releaseNotes: '<script>bad()</script>' }));
    const { container } = render(UpdateNotice, { settings: { ...defaultSettings, locale: 'en' }, details: true, onChange: vi.fn() });
    expect(await screen.findByText('<script>bad()</script>')).toBeInTheDocument();
    expect(container.querySelector('script')).toBeNull();
    expect(screen.getByText(/Automatic installation is unavailable for this build/) ).toBeInTheDocument();
    expect(methods.install).not.toHaveBeenCalled();
  });
  it('keeps notes collapsed, labelled and keyboard-scrollable, and hides details in the compact notice', async () => {
    setup(vi.fn().mockResolvedValue({ ...status, phase: 'available', availableVersion: '0.5.0', releaseNotes: 'Long release notes' }));
    const view = render(UpdateNotice, { settings: { ...defaultSettings, locale: 'en' }, details: true, onChange: vi.fn() });
    await screen.findByText('Long release notes');
    const disclosure = view.container.querySelector('details')!;
    expect(disclosure.open).toBe(false);
    disclosure.open = true;
    expect(screen.getByRole('region', { name: 'Release notes' })).toHaveAttribute('tabindex', '0');
    await view.rerender({ details: false });
    expect(view.container.querySelector('details')).toBeNull();
    expect(screen.queryByRole('checkbox')).toBeNull();
    expect(view.container.querySelector('.update-card')).toHaveClass('compact');
  });
  it('labels the startup switch and updates only the preference', async () => {
    setup();
    const onChange = vi.fn();
    const settings = { ...defaultSettings, locale: 'en' as const, autoCheckUpdates: false };
    render(UpdateNotice, { settings, details: true, onChange });
    const toggle = screen.getByRole('checkbox', { name: 'Check for updates at startup (no automatic download)' });
    expect(toggle).toHaveAccessibleDescription('Only release metadata is checked. Downloads always require your confirmation.');
    await fireEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith({ ...settings, autoCheckUpdates: true });
  });
  it('does not disrupt startup with offline errors', async () => {
    const methods = setup(vi.fn().mockRejectedValue(new Error('offline')));
    render(UpdateNotice, { settings: defaultSettings, details: false, onChange: vi.fn() });
    await waitFor(() => expect(methods.check).toHaveBeenCalled());
    expect(screen.queryByText('offline')).toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
  });
  it('excludes web UI and rejects direct web update APIs', async () => {
    state.web = true;
    const methods = setup();
    const { container } = render(UpdateNotice, { settings: defaultSettings, details: true, onChange: vi.fn() });
    expect(container.textContent?.trim()).toBe('');
    const { updates } = await import('./updates');
    for (const invoke of [updates.status, updates.check, updates.cancel, () => updates.install({ candidateId: 'candidate', confirmed: true })]) {
      await expect(invoke()).rejects.toThrow('Windows native app');
    }
    await expect(beginNativeExportLease()).rejects.toThrow('unavailable on web');
    await expect(endNativeExportLease('lease')).rejects.toThrow('unavailable on web');
    expect(methods.get).not.toHaveBeenCalled();
    expect(methods.check).not.toHaveBeenCalled();
    expect(methods.install).not.toHaveBeenCalled();
  });
});
