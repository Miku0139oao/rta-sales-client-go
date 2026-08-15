import type { ResolvedTheme, ThemePreference } from './types';

const DARK_QUERY = '(prefers-color-scheme: dark)';
const THEME_COLORS: Record<ResolvedTheme, string> = {
  light: '#f6f8f4',
  dark: '#111410',
};

export function resolveTheme(preference: ThemePreference, systemDark: boolean): ResolvedTheme {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
}

export function systemPrefersDark(target: Pick<Window, 'matchMedia'> | undefined = typeof window === 'undefined' ? undefined : window): boolean {
  return target?.matchMedia?.(DARK_QUERY).matches === true;
}

export function applyTheme(theme: ResolvedTheme, target: Document | undefined = typeof document === 'undefined' ? undefined : document): void {
  if (!target) return;
  target.documentElement.dataset.theme = theme;
  target.documentElement.style.colorScheme = theme;
  target.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.setAttribute('content', THEME_COLORS[theme]);
}

export function watchSystemTheme(
  listener: (dark: boolean) => void,
  target: Pick<Window, 'matchMedia'> | undefined = typeof window === 'undefined' ? undefined : window,
): () => void {
  const media = target?.matchMedia?.(DARK_QUERY);
  if (!media) return () => undefined;

  const handleChange = (event: MediaQueryListEvent) => listener(event.matches);
  media.addEventListener?.('change', handleChange);
  return () => media.removeEventListener?.('change', handleChange);
}
