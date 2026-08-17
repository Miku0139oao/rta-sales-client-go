export function isWebRuntime(): boolean {
  return import.meta.env.VITE_WEB === 'true';
}
