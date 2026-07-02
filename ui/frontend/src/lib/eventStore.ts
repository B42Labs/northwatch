import { writable, derived } from 'svelte/store';
import { NorthwatchWebSocket, type WsEvent, type WsState } from './websocket';
import { clusterApiPrefix, activeClusterInfo } from './clusterStore';

// Singleton WebSocket instance — created lazily on first subscription
let wsInstance: NorthwatchWebSocket | null = null;

export const wsConnectionState = writable<WsState>('disconnected');
export const changedUuids = writable<Map<string, number>>(new Map());

const HIGHLIGHT_DURATION = 5000;

function getWsInstance(): NorthwatchWebSocket {
  if (!wsInstance) {
    const ws = new NorthwatchWebSocket();
    wsInstance = ws;
    ws.onStateChange((state) => {
      wsConnectionState.set(state);
    });
    // Follow the active cluster: point the socket at the cluster's ws endpoint
    // and reconnect on switch. Snapshot clusters serve no /ws route on their
    // sub-mux, so disconnect while one is active.
    derived([clusterApiPrefix, activeClusterInfo], ([$prefix, $info]) => ({
      prefix: $prefix,
      isSnapshot: $info?.mode === 'snapshot',
    })).subscribe(({ prefix, isSnapshot }) => {
      if (isSnapshot) {
        ws.disconnect();
      } else {
        ws.setUrl(prefix);
        ws.connect();
      }
    });
  }
  return wsInstance;
}

// Eagerly open the WebSocket so the connection-status badge reflects real
// backend connectivity from app start, not just after the first subscription.
export function ensureWsConnected(): void {
  getWsInstance();
}

// Track changed UUIDs with auto-expiry
function markChanged(uuid: string): void {
  changedUuids.update((map) => {
    const now = Date.now();
    map.set(uuid, now);
    return new Map(map);
  });

  setTimeout(() => {
    changedUuids.update((map) => {
      map.delete(uuid);
      return new Map(map);
    });
  }, HIGHLIGHT_DURATION);
}

// Subscribe to events for a specific table, returns unsubscribe function
export function subscribeToTable(
  database: string,
  table: string,
  callback: (event: WsEvent) => void,
): () => void {
  const ws = getWsInstance();
  ws.subscribe(database, [table]);

  const removeListener = ws.onEvent((event) => {
    if (
      (database === '*' || event.database === database) &&
      (table === '*' || event.table === table)
    ) {
      markChanged(event.uuid);
      callback(event);
    }
  });

  return () => {
    removeListener();
    ws.unsubscribe(database, [table]);
  };
}

// Subscribe to multiple tables, returns unsubscribe function
export function subscribeToTables(
  database: string,
  tables: string[],
  callback: (event: WsEvent) => void,
): () => void {
  const ws = getWsInstance();
  ws.subscribe(database, tables);

  const removeListener = ws.onEvent((event) => {
    if (
      (database === '*' || event.database === database) &&
      (tables.includes('*') || tables.includes(event.table))
    ) {
      markChanged(event.uuid);
      callback(event);
    }
  });

  return () => {
    removeListener();
    ws.unsubscribe(database, tables);
  };
}
