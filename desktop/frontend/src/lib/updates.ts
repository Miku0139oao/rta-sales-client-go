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
}
export interface InstallUpdateRequest { candidateId: string; confirmed: boolean }
export const updateIsExclusive = (phase?: UpdateStatus['phase']) => Boolean(phase && !['idle', 'checking', 'current', 'available', 'error'].includes(phase));
export const updates = {
  status: () => invokeNativeUpdate<UpdateStatus>('GetUpdateStatus'),
  check: () => invokeNativeUpdate<UpdateStatus>('CheckForUpdate'),
  install: (request: InstallUpdateRequest) => invokeNativeUpdate<void>('InstallUpdate', [request]),
  cancel: () => invokeNativeUpdate<void>('CancelUpdate'),
};
