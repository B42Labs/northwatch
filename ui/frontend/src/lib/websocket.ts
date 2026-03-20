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

export class NorthwatchWebSocket {
  private ws: WebSocket | null = null;
  private reconnectDelay = MIN_RECONNECT_DELAY;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private eventListeners: EventCallback[] = [];
  private stateListeners: StateCallback[] = [];
  private pendingSubscriptions: SubscribeMessage[] = [];
  private _state: WsState = 'disconnected';
  private url: string;
  private closed = false;

  constructor(url?: string) {
    this.url = url ?? this.buildUrl();
  }

  private buildUrl(): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/api/v1/ws`;
  }

  get state(): WsState {
    return this._state;
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
      // Re-send pending subscriptions
      for (const msg of this.pendingSubscriptions) {
        this.send(msg);
      }
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
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.setState('disconnected');
  }

  subscribe(database: string, tables: string[]): void {
    const msg: SubscribeMessage = { action: 'subscribe', database, tables };
    this.pendingSubscriptions.push(msg);
    this.send(msg);
  }

  unsubscribe(database: string, tables: string[]): void {
    const msg: SubscribeMessage = { action: 'unsubscribe', database, tables };
    // Remove from pending
    this.pendingSubscriptions = this.pendingSubscriptions.filter(
      (m) =>
        !(
          m.action === 'subscribe' &&
          m.database === database &&
          arraysEqual(m.tables ?? [], tables)
        ),
    );
    this.send(msg);
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

function arraysEqual(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}
