import { Call, Events } from '@wailsio/runtime';
import { configureBackend } from './backend';

const serviceMethod = (name: string) => `github.com/Miku0139oao/rta-sales-client-go/desktop.App.${name}`;

type AnyMethod = (...args: unknown[]) => unknown;

const methods = new Proxy({} as Record<string, AnyMethod>, {
  get(_target, name) {
    if (typeof name !== 'string') return undefined;
    return (...args: unknown[]) => Call.ByName(serviceMethod(name), ...args);
  },
});

function eventData(payload: unknown): unknown {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as { data: unknown }).data;
  }
  return payload;
}

configureBackend({
  methods,
  events: {
    on(name, listener) {
      return Events.On(name, (event) => listener(eventData(event)));
    },
  },
  fileDrops: {
    on(listener) {
      return Events.On('rta:file-drop', (event) => {
        const data = eventData(event);
        listener(Array.isArray(data) ? (data as string[]) : []);
      });
    },
  },
});
