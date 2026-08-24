import { configureBackend } from './backend';
import { collectManCodes } from './manCodes';
import { decodeManCodeCatalog, encodeManCodeCatalog } from './manCodeCatalog';
import { packSalesAnalysisItems } from './salesAnalysisItems';
import {
  AppError,
  type ManCodeGroup,
  type Profile,
  type ProfileUpsertRequest,
  type SalesAnalysisReportMemoRequest,
  type SalesAnalysisRequest,
  type SalesAnalysisResult,
} from './types';
import { downloadWebPath, listenWebEvents, syncWebSession, uploadWebFile, webRPC } from './webApi';
import { downloadBase64, downloadText, pickBinaryFile, pickTextFile } from './webDownloads';
import { buildWebReportMemo, collectReportGlyphs } from './webReportMemo';
import { loadWebSnapshot, saveWebSnapshot, type StoredProfileSecret, type WebSnapshot } from './webStorage';

const WEB_DOWNLOADS = 'downloads';

let snapshot = loadWebSnapshot();
const listeners = new Map<string, Set<(payload: unknown) => void>>();

function emit(name: string, payload: unknown): void {
  listeners.get(name)?.forEach((listener) => listener(payload));
}

function persist(next: WebSnapshot): WebSnapshot {
  snapshot = saveWebSnapshot(next);
  return snapshot;
}

function cloneProfile(profile: Profile): Profile {
  return { ...profile };
}

function cloneGroup(group: ManCodeGroup): ManCodeGroup {
  return { ...group, codes: [...group.codes] };
}

function accountHint(account: string): string {
  if (account.length <= 4) return `${account.slice(0, 1)}••••`;
  return `${account.slice(0, 2)}••••${account.slice(-2)}`;
}

function newId(prefix: string): string {
  const random = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return prefix === 'profile' ? random : `${prefix}-${random}`;
}

function isProfileUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
}

function normalizeSnapshotIDs(value: WebSnapshot): WebSnapshot {
  const secrets = { ...value.secrets };
  const profiles = value.profiles.map((profile) => {
    if (isProfileUUID(profile.id)) return profile;
    const nextId = newId('profile');
    if (secrets[profile.id]) {
      secrets[nextId] = secrets[profile.id];
      delete secrets[profile.id];
    }
    return { ...profile, id: nextId };
  });
  return { ...value, profiles, secrets };
}

async function syncLiveSession(): Promise<void> {
  await syncWebSession({
    profiles: snapshot.profiles.map((profile) => ({
      id: profile.id,
      displayName: profile.displayName,
      enabled: profile.enabled,
      priority: profile.priority,
    })),
    secrets: snapshot.secrets,
    groups: snapshot.manCodeGroups,
  });
}

async function liveRPC<T>(method: string, arg?: unknown): Promise<T> {
  await syncLiveSession();
  return arg === undefined ? webRPC<T>(method) : webRPC<T>(method, arg);
}

function currentAnalysis(operationId?: string): SalesAnalysisResult {
  if (!snapshot.analysis || (operationId && snapshot.analysis.operationId !== operationId)) {
    throw new AppError('analysis_required', 'Sales analysis result is no longer available');
  }
  return snapshot.analysis;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {};
}

function rememberArticleNames(result: SalesAnalysisResult): void {
  const names = { ...snapshot.articleNames };
  for (const period of result.periods ?? []) {
    for (const item of period.items ?? []) {
      const code = item.articleCode?.trim();
      const name = item.articleName?.trim();
      if (code && name) names[code] = name;
    }
  }
  persist({ ...snapshot, articleNames: names, analysis: result });
}

export function installWebBackend(): void {
  snapshot = normalizeSnapshotIDs(loadWebSnapshot());
  persist(snapshot);
  listenWebEvents((name, payload) => {
    if (name === 'rta:sales-analysis-update' && payload && typeof payload === 'object') {
      rememberArticleNames(payload as SalesAnalysisResult);
    }
    emit(name, payload);
  });
  configureBackend({
    methods: {
      OpenWorkbook: async () => {
        const file = await pickBinaryFile('.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet');
        if (!file) return '';
        await syncLiveSession();
        return (await uploadWebFile(file)).path;
      },
      OpenMappingFile: async () => {
        const file = await pickBinaryFile('.json,.csv,application/json,text/csv');
        if (!file) return '';
        await syncLiveSession();
        return (await uploadWebFile(file)).path;
      },
      SaveWorkbook: async (value: unknown) => liveRPC<string>('SaveWorkbook', value),
      ScanWorkbook: async (value: unknown) => liveRPC('ScanWorkbook', value),
      Analyze: async (value: unknown) => liveRPC('Analyze', value),
      RetryFailed: async (value: unknown) => liveRPC('RetryFailed', value),
      Apply: async (value: unknown) => liveRPC('Apply', value),
      Cancel: async (value: unknown) => liveRPC('Cancel', value),
      OpenSavedWorkbook: async (value: unknown) => {
        const path = String(asRecord(value).path ?? '');
        if (path) downloadWebPath(path);
      },
      RevealSavedWorkbook: async (value: unknown) => {
        const path = String(asRecord(value).path ?? '');
        if (path) downloadWebPath(path);
      },
      OpenSavedFolder: async (value: unknown) => {
        const path = String(asRecord(value).path ?? '');
        if (path) downloadWebPath(path);
      },
      TestProfile: async (value: unknown) => {
        const profileId = String(asRecord(value).profileId ?? '');
        const result = await liveRPC<{ success?: boolean; ok?: boolean; storeCount?: number; message?: string }>('TestProfile', { profileId });
        const success = result.success === true || result.ok === true;
        persist({
          ...snapshot,
          profiles: snapshot.profiles.map((profile) => profile.id === profileId
            ? {
              ...profile,
              lastTestStatus: success ? 'success' : 'failed',
              lastTestMessage: result.message,
              enabled: success ? true : profile.enabled,
            }
            : profile),
        });
        return result;
      },

      ListProfiles: async () => snapshot.profiles.map(cloneProfile),

      CreateOrUpdateProfile: async (value: unknown) => {
        const request = value as ProfileUpsertRequest;
        const current = request.id ? snapshot.profiles.find((profile) => profile.id === request.id) : undefined;
        if (request.id && !current) throw new AppError('profile_not_found', 'Profile not found');
        const secrets = { ...snapshot.secrets };
        const incomingAccount = request.account.trim();
        const incomingPassword = request.password;
        const previousSecret = current ? secrets[current.id] : undefined;
        const nextSecret: StoredProfileSecret | undefined = incomingAccount || incomingPassword
          ? {
            account: incomingAccount || previousSecret?.account || '',
            password: incomingPassword || previousSecret?.password || '',
          }
          : previousSecret;
        if (nextSecret && (!nextSecret.account || !nextSecret.password) && !current?.hasCredentials) {
          throw new AppError('credentials_required', 'Enter an account and password');
        }
        const saved: Profile = {
          id: current?.id ?? newId('profile'),
          displayName: request.displayName.trim(),
          enabled: request.enabled,
          priority: current?.priority ?? snapshot.profiles.length + 1,
          hasCredentials: Boolean(nextSecret?.account && nextSecret?.password) || current?.hasCredentials === true,
          accountHint: nextSecret?.account ? accountHint(nextSecret.account) : current?.accountHint,
          lastTestStatus: incomingAccount || incomingPassword ? 'untested' : current?.lastTestStatus ?? 'untested',
        };
        const profiles = current
          ? snapshot.profiles.map((profile) => profile.id === saved.id ? saved : profile)
          : [...snapshot.profiles, saved];
        if (nextSecret) secrets[saved.id] = nextSecret;
        persist({ ...snapshot, profiles, secrets });
        return cloneProfile(saved);
      },

      DeleteProfile: async (value: unknown) => {
        const profileId = String(asRecord(value).profileId ?? '');
        const secrets = { ...snapshot.secrets };
        delete secrets[profileId];
        persist({
          ...snapshot,
          profiles: snapshot.profiles.filter((profile) => profile.id !== profileId)
            .map((profile, index) => ({ ...profile, priority: index + 1 })),
          secrets,
        });
      },

      Reorder: async (value: unknown) => {
        const profileIds = (asRecord(value).profileIds as string[] | undefined) ?? [];
        const byId = new Map(snapshot.profiles.map((profile) => [profile.id, profile]));
        const profiles = profileIds.flatMap((id, index) => {
          const profile = byId.get(id);
          return profile ? [{ ...profile, priority: index + 1 }] : [];
        });
        persist({ ...snapshot, profiles });
        return profiles.map(cloneProfile);
      },

      Enable: async (value: unknown) => {
        const request = asRecord(value);
        const profileId = String(request.profileId ?? '');
        const enabled = request.enabled === true;
        const found = snapshot.profiles.find((profile) => profile.id === profileId);
        if (!found) throw new AppError('profile_not_found', 'Profile not found');
        const saved = { ...found, enabled };
        persist({
          ...snapshot,
          profiles: snapshot.profiles.map((profile) => profile.id === saved.id ? saved : profile),
        });
        return cloneProfile(saved);
      },

      ListManCodeGroups: async () => snapshot.manCodeGroups.map(cloneGroup),

      SaveManCodeGroup: async (value: unknown) => {
        const request = value as { id?: string; name: string; codes?: string[]; raw?: string };
        const name = request.name.trim();
        if (!name) throw new AppError('backend_error', 'group name is required');
        const current = request.id ? snapshot.manCodeGroups.find((group) => group.id === request.id) : undefined;
        if (request.id && !current) throw new AppError('backend_error', 'mancode group does not exist');
        if (snapshot.manCodeGroups.some((group) => group.name === name && group.id !== request.id)) {
          throw new AppError('backend_error', 'group name already exists');
        }
        const saved: ManCodeGroup = {
          id: current?.id ?? newId('group'),
          name,
          codes: current && request.codes === undefined && !request.raw
            ? [...current.codes]
            : collectManCodes(request.raw, request.codes),
        };
        persist({
          ...snapshot,
          manCodeGroups: current
            ? snapshot.manCodeGroups.map((group) => group.id === saved.id ? saved : group)
            : [...snapshot.manCodeGroups, saved],
        });
        return cloneGroup(saved);
      },

      DeleteManCodeGroup: async (value: unknown) => {
        const id = String(value ?? '');
        persist({ ...snapshot, manCodeGroups: snapshot.manCodeGroups.filter((group) => group.id !== id) });
      },

      ReplaceManCodeGroupCodes: async (value: unknown) => {
        const request = value as { id: string; raw?: string; codes?: string[] };
        const current = snapshot.manCodeGroups.find((group) => group.id === request.id);
        if (!current) throw new AppError('backend_error', 'mancode group does not exist');
        const saved: ManCodeGroup = { ...current, codes: collectManCodes(request.raw, request.codes) };
        persist({
          ...snapshot,
          manCodeGroups: snapshot.manCodeGroups.map((group) => group.id === saved.id ? saved : group),
        });
        return cloneGroup(saved);
      },

      ExportManCodeCatalog: async () => {
        const groups = snapshot.manCodeGroups.map(cloneGroup);
        const filename = 'item-codes.json';
        downloadText(filename, encodeManCodeCatalog(groups), 'application/json');
        return { cancelled: false, path: filename, groups };
      },

      ImportManCodeCatalog: async () => {
        const picked = await pickTextFile('application/json,.json');
        if (!picked) return { cancelled: true };
        const groups = decodeManCodeCatalog(picked.text);
        persist({ ...snapshot, manCodeGroups: groups });
        return { cancelled: false, path: picked.name, groups: groups.map(cloneGroup) };
      },

      GetLatestArticleNames: async () => ({ ...snapshot.articleNames }),

      ListSalesAnalysisStores: async (value: unknown) =>
        liveRPC('ListSalesAnalysisStores', value),

      RunSalesAnalysis: async (value: unknown) => {
        const result = await liveRPC<SalesAnalysisResult>('RunSalesAnalysis', value);
        rememberArticleNames(result);
        return result;
      },

      GetSalesAnalysisItems: async (value: unknown) => {
        try {
          return await liveRPC('GetSalesAnalysisItems', value);
        } catch {
          const request = asRecord(value);
          const operationId = String(request.operationId ?? '');
          const periodKey = String(request.periodKey ?? '');
          const result = currentAnalysis(operationId);
          const period = result.periods?.find((candidate) => candidate.key === periodKey);
          return packSalesAnalysisItems(periodKey, period?.items ?? [], period?.stores ?? result.stores ?? []);
        }
      },

      GetSalesAnalysisReportGlyphs: async (value: unknown) => {
        try {
          return await liveRPC<string>('GetSalesAnalysisReportGlyphs', value);
        } catch {
          return collectReportGlyphs(currentAnalysis(String(asRecord(value).operationId ?? '')), snapshot.manCodeGroups);
        }
      },

      GetSalesAnalysisReportMemo: async (value: unknown) => {
        try {
          return await liveRPC('GetSalesAnalysisReportMemo', value);
        } catch {
          const request = value as SalesAnalysisReportMemoRequest;
          return buildWebReportMemo(currentAnalysis(request.operationId), request, snapshot.manCodeGroups);
        }
      },

      ClearSalesAnalysis: async (value: unknown) => {
        const operationId = String(asRecord(value).operationId ?? '');
        try { await liveRPC('ClearSalesAnalysis', value); } catch { /* keep local copy if the session expired */ }
        if (snapshot.analysis?.operationId === operationId) persist({ ...snapshot, analysis: null });
      },

      CancelSalesAnalysis: async (value: unknown) => {
        try { await liveRPC('CancelSalesAnalysis', value ?? {}); } catch { /* local cancel still applies */ }
      },

      ChooseSalesAnalysisPDFDirectory: async () => WEB_DOWNLOADS,

      WriteSalesAnalysisPDF: async (value: unknown) => {
        const request = asRecord(value);
        return downloadBase64(String(request.filename ?? 'report.pdf'), String(request.dataBase64 ?? ''), 'application/pdf');
      },

      WriteSalesAnalysisTextExport: async (value: unknown) => {
        const request = asRecord(value);
        return downloadBase64(String(request.filename ?? 'export.md'), String(request.dataBase64 ?? ''), 'text/markdown;charset=utf-8');
      },
    },
    events: {
      on(name, listener) {
        let set = listeners.get(name);
        if (!set) {
          set = new Set();
          listeners.set(name, set);
        }
        set.add(listener);
        return () => set!.delete(listener);
      },
    },
  });
}
