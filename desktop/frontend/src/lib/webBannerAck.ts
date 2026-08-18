export const WEB_BANNER_ACK_KEY = 'rta-web-privacy-ack-v2';

export function readWebBannerAck(): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    return localStorage.getItem(WEB_BANNER_ACK_KEY) === '1';
  } catch {
    return false;
  }
}

export function writeWebBannerAck(): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(WEB_BANNER_ACK_KEY, '1');
  } catch {
    // Ignore quota or private-mode failures; the banner can be dismissed for this visit only.
  }
}
