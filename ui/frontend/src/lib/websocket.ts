export interface WsEvent {
  type: 'insert' | 'update' | 'delete';
  database: string;
  table: string;
  uuid: string;
  row?: Record<string, unknown>;
  old_row?: Record<string, unknown>;
  ts: number;
}

export interface SubscribeMessage {
  action: 'subscribe' | 'unsubscribe' | 'ping';
  database?: string;
  tables?: string[];
}

export type WsState = 'connecting' | 'connected' | 'disconnected';

type EventCallback = (event: WsEvent) => void;
type StateCallback = (state: WsState) => void;

const MIN_RECONNECT_DELAY = 1000;
const MAX_RECONNECT_DELAY = 30000;
const PING_INTERVAL = 30000;

// One active subscription, refcounted so overlapping subscribers to the same
// database+tables share a single server-side subscribe/unsubscribe pair.
interface SubscriptionEntry {
  msg: SubscribeMessage;
  count: number;
}

export class NorthwatchWebSocket {
  private ws: WebSocket | null = null;
  private reconnectDelay = MIN_RECONNECT_DELAY;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private eventListeners: EventCallback[] = [];
  private stateListeners: StateCallback[] = [];
  private subscriptions = new Map<string, SubscriptionEntry>();
  private _state: WsState = 'disconnected';
  private url: string;
  private closed = false;

  constructor(url?: string) {
    this.url = url ?? this.buildUrl('');
  }

  // buildUrl derives the WebSocket endpoint for a cluster prefix. An empty
  // prefix targets the top-level `/api/v1/ws`; a cluster prefix like
  // `/api/v1/clusters/<name>` yields `<prefix>/ws`.
  private buildUrl(prefix: string): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const path = prefix === '' ? '/api/v1/ws' : `${prefix}/ws`;
    return `${protocol}//${window.location.host}${path}`;
  }

  get state(): WsState {
    return this._state;
  }

  // setUrl points the socket at a different cluster prefix. It is a no-op when
  // the resolved url is unchanged; otherwise it stores the new url and, if the
  // socket is currently active (connected/connecting or reconnecting), tears it
  // down and reconnects with a fresh backoff. An explicitly disconnected socket
  // is left closed — the next connect() picks up the new url.
  setUrl(prefix: string): void {
    const url = this.buildUrl(prefix);
    if (url === this.url) return;
    this.url = url;
    const active = this.ws !== null || this.reconnectTimer !== null;
    if (this.closed || !active) return;
    this.teardownSocket();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.reconnectDelay = MIN_RECONNECT_DELAY;
    this.connect();
  }

  connect(): void {
    if (this.ws) return;
    this.closed = false;
    this.setState('connecting');

    try {
      this.ws = new WebSocket(this.url);
    } catch {
      this.scheduleReconnect();
      return;
    }

    this.ws.onopen = () => {
      this.reconnectDelay = MIN_RECONNECT_DELAY;
      this.setState('connected');
      // Re-send each active subscription once.
      for (const entry of this.subscriptions.values()) {
        this.send(entry.msg);
      }
      this.startPing();
    };

    this.ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.action === 'pong') return;
        if (data.type) {
          for (const cb of this.eventListeners) {
            cb(data as WsEvent);
          }
        }
      } catch {
        // ignore malformed messages
      }
    };

    this.ws.onclose = () => {
      this.ws = null;
      this.stopPing();
      if (!this.closed) {
        this.setState('disconnected');
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  disconnect(): void {
    this.closed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopPing();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.setState('disconnected');
  }

  subscribe(database: string, tables: string[]): void {
    const key = subKey(database, tables);
    const existing = this.subscriptions.get(key);
    if (existing) {
      existing.count++;
      return;
    }
    const msg: SubscribeMessage = { action: 'subscribe', database, tables };
    this.subscriptions.set(key, { msg, count: 1 });
    this.send(msg);
  }

  unsubscribe(database: string, tables: string[]): void {
    const key = subKey(database, tables);
    const existing = this.subscriptions.get(key);
    if (!existing) return;
    existing.count--;
    if (existing.count > 0) return;
    this.subscriptions.delete(key);
    this.send({ action: 'unsubscribe', database, tables });
  }

  onEvent(cb: EventCallback): () => void {
    this.eventListeners.push(cb);
    return () => {
      this.eventListeners = this.eventListeners.filter((l) => l !== cb);
    };
  }

  onStateChange(cb: StateCallback): () => void {
    this.stateListeners.push(cb);
    return () => {
      this.stateListeners = this.stateListeners.filter((l) => l !== cb);
    };
  }

  private send(msg: SubscribeMessage): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  // teardownSocket detaches handlers (so the old socket's onclose does not
  // trigger a reconnect) and closes the socket, stopping client pings.
  private teardownSocket(): void {
    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.close();
      this.ws = null;
    }
    this.stopPing();
  }

  private startPing(): void {
    this.stopPing();
    this.pingTimer = setInterval(() => {
      this.send({ action: 'ping' });
    }, PING_INTERVAL);
  }

  private stopPing(): void {
    if (this.pingTimer) {
      clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
  }

  private setState(state: WsState): void {
    this._state = state;
    for (const cb of this.stateListeners) {
      cb(state);
    }
  }

  private scheduleReconnect(): void {
    if (this.closed) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.reconnectDelay);
    this.reconnectDelay = Math.min(
      this.reconnectDelay * 2,
      MAX_RECONNECT_DELAY,
    );
  }
}

// subKey builds a stable refcount key from a database and its tables,
// order-independent so ['a','b'] and ['b','a'] collapse to one subscription.
function subKey(database: string, tables: string[]): string {
  return database + '|' + [...tables].sort().join(',');
}
