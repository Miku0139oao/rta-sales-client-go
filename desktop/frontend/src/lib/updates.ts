import { invokeNativeUpdate } from './backend';

export interface UpdateStatus {
  currentVersion: string;
  phase: 'idle' | 'checking' | 'current' | 'available' | 'error' | 'preparing' | 'verifying-current' | 'downloading' | 'starting-helper' | 'ready' | 'cancelling' | 'committing' | 'committed' | 'blocked';
  candidateId: string;
  availableVersion: string;
  releaseNotes: string;
  changelogVersion: string;
  changelogBody: string;
  installSupported: boolean;
  unsupportedReason?: string;
  error: string;
  errorCode?: string;
  errorPath?: string;
}
export function updateFailureText(status: UpdateStatus | undefined, raw: string, english: boolean): string {
  const code = status?.errorCode;
  const path = status?.errorPath;
  const owner = code === 'unsafe_owner' || raw.includes('unsafe update path owner');
  const permissions = code === 'unsafe_permissions' || raw.includes('unsafe update path permissions') || raw.includes('restrictive DACL') || raw.includes('launch-boundary ACE');
  if (!owner && !permissions) return raw;
  const reason = english
    ? (owner ? 'The update path has an untrusted owner.' : 'The update path permissions allow unsafe changes.')
    : (owner ? '更新路徑的擁有者不符合安全要求。' : '更新路徑的權限允許不安全的變更。');
  const location = path ? ` ${english ? 'Affected path' : '問題路徑'}：${path}` : '';
  const recovery = english
    ? ' Close the app and download the official release for a manual update. For automatic updates, use a private folder under your Windows user profile and check again; parent folders must also pass validation.'
    : ' 請關閉程式，下載正式版本手動更新。若要使用自動更新，請將程式放在 Windows 個人使用者目錄下的私人資料夾，再重新檢查；上層資料夾也必須通過驗證。';
  return reason + location + recovery;
}
export interface InstallUpdateRequest { candidateId: string; confirmed: boolean }
export const updateIsExclusive = (phase?: UpdateStatus['phase']) => Boolean(phase && !['idle', 'checking', 'current', 'available', 'error'].includes(phase));
export const updates = {
  status: () => invokeNativeUpdate<UpdateStatus>('GetUpdateStatus'),
  check: () => invokeNativeUpdate<UpdateStatus>('CheckForUpdate'),
  startup: () => invokeNativeUpdate<UpdateStatus>('CheckForUpdateStartup'),
  install: (request: InstallUpdateRequest) => invokeNativeUpdate<void>('InstallUpdate', [request]),
  cancel: () => invokeNativeUpdate<void>('CancelUpdate'),
};
