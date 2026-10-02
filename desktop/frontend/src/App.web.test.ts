import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WEB_BANNER_ACK_KEY } from './lib/webBannerAck';

vi.mock('./lib/runtime', () => ({ isWebRuntime: () => true }));

import App from './App.svelte';
import { configureBackend } from './lib/backend';

beforeEach(() => {
  localStorage.clear();
  configureBackend(undefined);
});

afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('web privacy banner', () => {
  it('shows the notice until the user acknowledges it', async () => {
    render(App);
    expect(screen.getByText('使用者需知')).toBeInTheDocument();
    expect(screen.getByText(/伺服器不持久儲存帳號與密碼/)).toBeInTheDocument();
    expect(screen.getByText(/本機完整報表使用 localStorage 或 IndexedDB/)).toBeInTheDocument();
    await fireEvent.click(screen.getByText('我已知曉'));
    expect(screen.queryByText('我已知曉')).not.toBeInTheDocument();
    expect(localStorage.getItem(WEB_BANNER_ACK_KEY)).toBe('1');

    cleanup();
    render(App);
    expect(screen.queryByText('我已知曉')).not.toBeInTheDocument();
  });
});
