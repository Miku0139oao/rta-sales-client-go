import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
  it('starts in Traditional Chinese with the five primary destinations', () => {
    render(App);
    expect(screen.getByRole('heading', { name: '從活頁簿開始' })).toBeInTheDocument();
    expect(screen.getAllByText('Excel 填入').length).toBeGreaterThan(0);
    expect(screen.getAllByText('帳號').length).toBeGreaterThan(0);
    expect(screen.getAllByText('商品代碼').length).toBeGreaterThan(0);
    expect(screen.getAllByText('設定').length).toBeGreaterThan(0);
  });

  it('opens the ItemCode page from the nav and lists the mock groups', async () => {
    render(App);
    await fireEvent.click(screen.getAllByRole('button', { name: /商品代碼/ })[0]);
    await waitFor(() => expect(screen.getByRole('heading', { name: '商品代碼' })).toBeInTheDocument());
    expect(screen.getByText('保健')).toBeInTheDocument();
    expect(screen.getByText('護膚')).toBeInTheDocument();
  });

  it('opens and scans an xlsx workbook with the file range selected', async () => {
    const { container } = render(App);
    const openButton = container.querySelector('.file-drop-card md-filled-button');
    expect(openButton).not.toBeNull();
    await fireEvent.click(openButton!);

    await waitFor(() => expect(screen.getByText('掃描摘要')).toBeInTheDocument());
    expect((container.querySelector('#from-date') as HTMLInputElement).value).toBe('2026-08-01');
    expect((container.querySelector('#to-date') as HTMLInputElement).value).toBe('2026-08-14');
    expect(screen.queryByText('來源檔案永遠不會被覆蓋。')).not.toBeInTheDocument();
  });

  it('never fills a stored password back into the edit form', async () => {
    const { container } = render(App);
    const accountsNav = screen.getAllByRole('button', { name: /帳號/ })[0];
    await fireEvent.click(accountsNav);
    await waitFor(() => expect(screen.getByText('主要帳號')).toBeInTheDocument());

    const editButton = container.querySelector('[aria-label^="編輯"]');
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

  it('toggles the effective theme from the top bar and persists the choice', async () => {
    render(App);
    expect(document.documentElement.dataset.theme).toBe('light');
    await fireEvent.click(screen.getByRole('button', { name: '切換為深色主題' }));

    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(JSON.parse(localStorage.getItem('rta-sales-desktop-settings-v2') ?? '{}')).toMatchObject({ theme: 'dark' });
    expect(screen.getByRole('button', { name: '切換為亮色主題' })).toBeInTheDocument();
  });

  it('follows live system theme changes until the quick toggle pins a preference', async () => {
    let change: ((event: MediaQueryListEvent) => void) | undefined;
    const descriptor = Object.getOwnPropertyDescriptor(window, 'matchMedia');
    const media = {
      matches: false,
      addEventListener: (_name: string, listener: (event: MediaQueryListEvent) => void) => { change = listener; },
      removeEventListener: vi.fn(),
    } as unknown as MediaQueryList;
    Object.defineProperty(window, 'matchMedia', { configurable: true, value: () => media });
    try {
      render(App);
      change?.({ matches: true } as MediaQueryListEvent);
      await waitFor(() => expect(document.documentElement.dataset.theme).toBe('dark'));

      await fireEvent.click(screen.getByRole('button', { name: '切換為亮色主題' }));
      expect(JSON.parse(localStorage.getItem('rta-sales-desktop-settings-v2') ?? '{}')).toMatchObject({ theme: 'light' });
      change?.({ matches: true } as MediaQueryListEvent);
      expect(document.documentElement.dataset.theme).toBe('light');
    } finally {
      if (descriptor) Object.defineProperty(window, 'matchMedia', descriptor);
      else Reflect.deleteProperty(window, 'matchMedia');
    }
  });

  it('applies a settings-page theme immediately without saving unrelated draft values', async () => {
    render(App);
    await fireEvent.click(screen.getAllByRole('button', { name: /設定/ })[0]);
    const maxJobs = screen.getByRole('spinbutton', { name: '每次最多查詢工作' }) as HTMLInputElement;
    await fireEvent.input(maxJobs, { target: { value: '17' } });
    await fireEvent.click(screen.getByRole('radio', { name: '深色' }));

    const saved = JSON.parse(localStorage.getItem('rta-sales-desktop-settings-v2') ?? '{}');
    expect(saved).toMatchObject({ theme: 'dark', maxJobs: 2000 });
    expect(maxJobs.value).toBe('17');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('moves focus and scroll position immediately after page navigation', async () => {
    render(App);
    const main = document.querySelector('#main-content') as HTMLElement;
    main.scrollTop = 240;
    document.documentElement.scrollTop = 240;
    await fireEvent.click(screen.getAllByRole('button', { name: /帳號/ })[0]);
    await waitFor(() => expect(screen.getByRole('heading', { name: '帳號設定檔' })).toBeInTheDocument());

    expect(main.scrollTop).toBe(0);
    expect(document.documentElement.scrollTop).toBe(0);
    expect(document.activeElement).toBe(main);
  });
});
