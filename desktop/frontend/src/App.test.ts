import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import App from './App.svelte';
import { configureBackend } from './lib/backend';
import type { ProfileTestResult } from './lib/types';

beforeEach(() => {
  localStorage.clear();
  configureBackend(undefined);
});

afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('desktop application shell', () => {
  it('starts in Traditional Chinese with the three primary destinations', () => {
    render(App);
    expect(screen.getByRole('heading', { name: '從活頁簿開始' })).toBeInTheDocument();
    expect(screen.getAllByText('Excel 填入').length).toBeGreaterThan(0);
    expect(screen.getAllByText('帳號').length).toBeGreaterThan(0);
    expect(screen.getAllByText('設定').length).toBeGreaterThan(0);
  });

  it('opens and scans an xlsx workbook with the file range selected', async () => {
    const { container } = render(App);
    const openButton = container.querySelector('.file-drop-card md-filled-button');
    expect(openButton).not.toBeNull();
    await fireEvent.click(openButton!);

    await waitFor(() => expect(screen.getByText('掃描摘要')).toBeInTheDocument());
    expect((container.querySelector('#from-date') as HTMLInputElement).value).toBe('2026-08-01');
    expect((container.querySelector('#to-date') as HTMLInputElement).value).toBe('2026-08-14');
    expect(screen.getByText('來源檔案永遠不會被覆蓋。')).toBeInTheDocument();
  });

  it('never fills a stored password back into the edit form', async () => {
    const { container } = render(App);
    const accountsNav = screen.getAllByRole('button', { name: /帳號/ })[0];
    await fireEvent.click(accountsNav);
    await waitFor(() => expect(screen.getByText('主要帳號')).toBeInTheDocument());

    const editButton = container.querySelector('md-icon-button[aria-label^="編輯"]');
    expect(editButton).not.toBeNull();
    await fireEvent.click(editButton!);

    const password = container.querySelector('#profile-password') as HTMLInputElement;
    expect(password.value).toBe('');
  });

  it('locks cross-page navigation while an account test is running', async () => {
    let resolveTest!: (result: ProfileTestResult) => void;
    configureBackend({ methods: {
      ListProfiles: async () => [{
        id: 'profile-1', displayName: 'Primary', enabled: false, priority: 1, hasCredentials: true,
      }],
      TestProfile: (_request: unknown) => new Promise<ProfileTestResult>((resolve) => { resolveTest = resolve; }),
    } });
    const { container } = render(App);
    await fireEvent.click(screen.getAllByRole('button', { name: /帳號/ })[0]);
    await waitFor(() => expect(screen.getByText('Primary')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('.profile-actions md-outlined-button')!);

    const excelNavigation = screen.getAllByRole('button', { name: /Excel 填入/ });
    await waitFor(() => excelNavigation.forEach((button) => expect(button).toBeDisabled()));
    resolveTest({ success: true });
    await waitFor(() => excelNavigation.forEach((button) => expect(button).not.toBeDisabled()));
  });

  it('switches every visible setting label to English after saving', async () => {
    const { container } = render(App);
    await fireEvent.click(screen.getAllByRole('button', { name: /設定/ })[0]);
    const language = screen.getByRole('combobox', { name: '介面語言' }) as HTMLSelectElement;
    await fireEvent.change(language, { target: { value: 'en' } });
    await fireEvent.submit(container.querySelector('.settings-grid')!);

    expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Workload' })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('en');
  });
});
