import { AppError } from './types';

interface RPCResponse<T> {
  result?: T;
  error?: { code?: string; message?: string };
}

export async function uploadWebFile(file: File): Promise<{ path: string; fileName: string }> {
  const body = new FormData();
  body.set('file', file, file.name);
  const response = await fetch('/api/upload', { method: 'POST', credentials: 'same-origin', body });
  return readJSON(response);
}

export function downloadWebPath(path: string): void {
  const url = `/api/download?path=${encodeURIComponent(path)}`;
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = path.split(/[\\/]/).pop() || 'download';
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
}

export async function syncWebSession(payload: {
  profiles: Array<{ id: string; displayName: string; enabled: boolean; priority: number }>;
  secrets: Record<string, { account: string; password: string }>;
  groups: Array<{ id: string; name: string; codes: string[] }>;
}): Promise<void> {
  const response = await fetch('/api/session', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  await readJSON(response);
}

export async function webRPC<T>(method: string, ...args: unknown[]): Promise<T> {
  const response = await fetch('/api/rpc', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ method, args }),
  });
  const payload = await readJSON<RPCResponse<T>>(response);
  if (payload.error) {
    throw new AppError(payload.error.code || 'backend_error', payload.error.message || method);
  }
  return payload.result as T;
}

export function listenWebEvents(onEvent: (name: string, payload: unknown) => void): () => void {
  if (typeof EventSource === 'undefined') return () => undefined;
  const source = new EventSource('/api/events');
  const names = ['rta:progress', 'rta:sales-analysis-progress', 'rta:sales-analysis-update'];
  const handlers = names.map((name) => {
    const handler = (event: MessageEvent) => {
      try {
        onEvent(name, JSON.parse(event.data));
      } catch {
        onEvent(name, event.data);
      }
    };
    source.addEventListener(name, handler as EventListener);
    return { name, handler };
  });
  return () => {
    for (const { name, handler } of handlers) {
      source.removeEventListener(name, handler as EventListener);
    }
    source.close();
  };
}

async function readJSON<T>(response: Response): Promise<T> {
  let payload: T & RPCResponse<unknown>;
  try {
    payload = await response.json() as T & RPCResponse<unknown>;
  } catch {
    throw new AppError(
      response.ok ? 'backend_error' : 'backend_unavailable',
      response.ok ? 'The web API returned an unreadable response' : `Web API HTTP ${response.status}`,
    );
  }
  if (!response.ok) {
    throw new AppError(payload.error?.code || 'backend_unavailable', payload.error?.message || `Web API HTTP ${response.status}`);
  }
  return payload;
}
