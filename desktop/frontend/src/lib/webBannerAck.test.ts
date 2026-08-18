import { afterEach, describe, expect, it } from 'vitest';
import { readWebBannerAck, WEB_BANNER_ACK_KEY, writeWebBannerAck } from './webBannerAck';

afterEach(() => {
  localStorage.clear();
});

describe('web banner acknowledgement', () => {
  it('starts unread and persists after the user acknowledges', () => {
    expect(readWebBannerAck()).toBe(false);
    writeWebBannerAck();
    expect(localStorage.getItem(WEB_BANNER_ACK_KEY)).toBe('1');
    expect(readWebBannerAck()).toBe(true);
  });
});
