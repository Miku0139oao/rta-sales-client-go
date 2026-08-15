import { afterEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, resolveTheme, watchSystemTheme } from './theme';

afterEach(() => {
  document.documentElement.removeAttribute('data-theme');
  document.documentElement.style.colorScheme = '';
  document.querySelector('meta[name="theme-color"]')?.remove();
});

describe('theme behavior', () => {
  it('resolves explicit and system preferences', () => {
    expect(resolveTheme('system', false)).toBe('light');
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('light', true)).toBe('light');
    expect(resolveTheme('dark', false)).toBe('dark');
  });

  it('applies the document color scheme and theme color', () => {
    const meta = document.createElement('meta');
    meta.name = 'theme-color';
    document.head.append(meta);

    applyTheme('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.style.colorScheme).toBe('dark');
    expect(meta.content).toBe('#0f1217');
  });

  it('subscribes to live system theme changes and cleans up', () => {
    let change: ((event: MediaQueryListEvent) => void) | undefined;
    const remove = vi.fn();
    const media = {
      matches: false,
      addEventListener: vi.fn((_name: string, listener: (event: MediaQueryListEvent) => void) => { change = listener; }),
      removeEventListener: remove,
    } as unknown as MediaQueryList;
    const listener = vi.fn();
    const cleanup = watchSystemTheme(listener, { matchMedia: () => media } as Pick<Window, 'matchMedia'>);

    change?.({ matches: true } as MediaQueryListEvent);
    expect(listener).toHaveBeenCalledWith(true);
    cleanup();
    expect(remove).toHaveBeenCalledWith('change', change);
  });
});
