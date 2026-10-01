import { afterEach, describe, expect, it, vi } from 'vitest';
import { webRPC } from './webApi';
import { errorMessage } from './i18n';
import { AppError } from './types';

vi.mock('./runtime', () => ({ isWebRuntime: () => true }));
afterEach(() => vi.unstubAllGlobals());

describe('web API errors', () => {
  it('preserves a server reason in the displayed error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: {
      code: 'backend_error', message: 'store 107 is not authorized',
    } }), { status: 400 })));
    const error = await webRPC('RunSalesAnalysis').catch((error) => error);
    expect(errorMessage('zh-TW', error)).toContain('store 107 is not authorized');
    expect(errorMessage('zh-TW', error)).not.toContain('桌面');
  });
  it('identifies proxy timeout HTML without showing its contents', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>proxy timeout</html>', { status: 524 })));
    const error = await webRPC('RunSalesAnalysis').catch((error) => error);
    expect(error).toBeInstanceOf(AppError);
    expect((error as AppError).code).toBe('web_timeout');
    expect(errorMessage('zh-TW', error)).toContain('逾時');
  });
  it('identifies a disconnected network', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch'); }));
    const error = await webRPC('RunSalesAnalysis').catch((error) => error);
    expect(errorMessage('zh-TW', error)).toContain('網路連線中斷');
  });
});
