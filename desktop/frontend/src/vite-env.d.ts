/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_WEB?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare namespace svelteHTML {
  interface IntrinsicElements {
    'md-filled-button': Record<string, unknown>;
    'md-outlined-button': Record<string, unknown>;
    'md-text-button': Record<string, unknown>;
    'md-icon-button': Record<string, unknown>;
    'md-filled-tonal-button': Record<string, unknown>;
    'md-switch': Record<string, unknown>;
    'md-checkbox': Record<string, unknown>;
    'md-linear-progress': Record<string, unknown>;
    'md-circular-progress': Record<string, unknown>;
    'md-dialog': Record<string, unknown>;
  }
}
