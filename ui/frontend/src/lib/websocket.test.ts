import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { NorthwatchWebSocket, type WsEvent } from './websocket';

// Mock WebSocket
class MockWebSocket {
  static instances: MockWebSocket[] = [];
  url: string;
  readyState = 0; // CONNECTING
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sentMessages: string[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  send(data: string): void {
    this.sentMessages.push(data);
  }

  close(): void {
    this.readyState = 3; // CLOSED
    this.onclose?.();
  }

  // Test helpers
  simulateOpen(): void {
    this.readyState = 1; // OPEN
    this.onopen?.();
  }

  simulateMessage(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  simulateClose(): void {
    this.readyState = 3;
    this.onclose?.();
  }
}

// Assign OPEN constant
(MockWebSocket as unknown as { OPEN: number }).OPEN = 1;

describe('NorthwatchWebSocket', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.useFakeTimers();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (globalThis as any).WebSocket = MockWebSocket;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('connects and changes state', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    const states: string[] = [];
    ws.onStateChange((s) => states.push(s));

    ws.connect();
    expect(states).toContain('connecting');

    MockWebSocket.instances[0].simulateOpen();
    expect(states).toContain('connected');
    expect(ws.state).toBe('connected');

    ws.disconnect();
    expect(ws.state).toBe('disconnected');
  });

  it('dispatches events to listeners', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    const events: WsEvent[] = [];
    ws.onEvent((e) => events.push(e));

    ws.connect();
    MockWebSocket.instances[0].simulateOpen();

    MockWebSocket.instances[0].simulateMessage({
      type: 'insert',
      database: 'nb',
      table: 'Logical_Switch',
      uuid: 'test-uuid',
      row: { name: 'ls1' },
      ts: 123,
    });

    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('insert');
    expect(events[0].uuid).toBe('test-uuid');

    ws.disconnect();
  });

  it('sends subscribe messages', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    MockWebSocket.instances[0].simulateOpen();

    ws.subscribe('nb', ['Logical_Switch']);

    const mock = MockWebSocket.instances[0];
    expect(mock.sentMessages).toHaveLength(1);
    const msg = JSON.parse(mock.sentMessages[0]);
    expect(msg.action).toBe('subscribe');
    expect(msg.database).toBe('nb');
    expect(msg.tables).toEqual(['Logical_Switch']);

    ws.disconnect();
  });

  it('re-sends subscriptions on reconnect', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    MockWebSocket.instances[0].simulateOpen();
    ws.subscribe('nb', ['*']);

    // Simulate disconnect
    MockWebSocket.instances[0].simulateClose();

    // Advance timer past reconnect delay
    vi.advanceTimersByTime(1500);

    // New WebSocket instance should be created
    expect(MockWebSocket.instances).toHaveLength(2);
    MockWebSocket.instances[1].simulateOpen();

    // The subscription should be resent
    const resent = MockWebSocket.instances[1].sentMessages;
    expect(resent).toHaveLength(1);
    const msg = JSON.parse(resent[0]);
    expect(msg.action).toBe('subscribe');

    ws.disconnect();
  });

  it('exponential backoff on reconnect', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    MockWebSocket.instances[0].simulateClose();

    // First reconnect at 1s
    vi.advanceTimersByTime(1000);
    expect(MockWebSocket.instances).toHaveLength(2);

    MockWebSocket.instances[1].simulateClose();

    // Second reconnect at 2s (not yet)
    vi.advanceTimersByTime(1500);
    expect(MockWebSocket.instances).toHaveLength(2);

    // Now at 2s
    vi.advanceTimersByTime(500);
    expect(MockWebSocket.instances).toHaveLength(3);

    ws.disconnect();
  });

  it('removes event listener', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    const events: WsEvent[] = [];
    const unsubscribe = ws.onEvent((e) => events.push(e));

    ws.connect();
    MockWebSocket.instances[0].simulateOpen();

    unsubscribe();

    MockWebSocket.instances[0].simulateMessage({
      type: 'insert',
      database: 'nb',
      table: 'Logical_Switch',
      uuid: 'test',
      ts: 123,
    });

    expect(events).toHaveLength(0);
    ws.disconnect();
  });

  it('ignores pong messages', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    const events: WsEvent[] = [];
    ws.onEvent((e) => events.push(e));

    ws.connect();
    MockWebSocket.instances[0].simulateOpen();

    MockWebSocket.instances[0].simulateMessage({ action: 'pong' });

    expect(events).toHaveLength(0);
    ws.disconnect();
  });

  function count(mock: MockWebSocket, action: string): number {
    return mock.sentMessages.filter((m) => JSON.parse(m).action === action)
      .length;
  }

  it('refcounts subscriptions so a second subscriber survives the first unsubscribe', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    const mock = MockWebSocket.instances[0];
    mock.simulateOpen();

    ws.subscribe('nb', ['Logical_Switch']);
    ws.subscribe('nb', ['Logical_Switch']);

    // subscribe sent once for two subscribers (no duplicate pending entry)
    expect(count(mock, 'subscribe')).toBe(1);

    // first unsubscribe: still one subscriber, no unsubscribe sent
    ws.unsubscribe('nb', ['Logical_Switch']);
    expect(count(mock, 'unsubscribe')).toBe(0);

    // second unsubscribe: last subscriber leaves, unsubscribe sent once
    ws.unsubscribe('nb', ['Logical_Switch']);
    expect(count(mock, 'unsubscribe')).toBe(1);

    ws.disconnect();
  });

  it('dedups subscriptions independent of table order and resends each once', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    MockWebSocket.instances[0].simulateOpen();

    ws.subscribe('nb', ['A', 'B']);
    ws.subscribe('nb', ['B', 'A']); // same set, reversed order

    expect(count(MockWebSocket.instances[0], 'subscribe')).toBe(1);

    // Reconnect and confirm the single entry is resent exactly once.
    MockWebSocket.instances[0].simulateClose();
    vi.advanceTimersByTime(1500);
    MockWebSocket.instances[1].simulateOpen();

    expect(count(MockWebSocket.instances[1], 'subscribe')).toBe(1);

    ws.disconnect();
  });

  it('setUrl reconnects to the new cluster prefix and re-sends active subscriptions', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    MockWebSocket.instances[0].simulateOpen();
    ws.subscribe('nb', ['Logical_Switch']);

    ws.setUrl('/api/v1/clusters/prod');

    // A fresh socket is opened against the cluster's ws endpoint.
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].url).toContain(
      '/api/v1/clusters/prod/ws',
    );

    MockWebSocket.instances[1].simulateOpen();
    expect(count(MockWebSocket.instances[1], 'subscribe')).toBe(1);

    ws.disconnect();
  });

  it('setUrl is a no-op when the resolved url is unchanged', () => {
    const ws = new NorthwatchWebSocket();
    ws.connect();
    MockWebSocket.instances[0].simulateOpen();

    ws.setUrl(''); // default prefix — resolves to the current url

    expect(MockWebSocket.instances).toHaveLength(1);
    ws.disconnect();
  });

  it('sends client pings on an interval while connected', () => {
    const ws = new NorthwatchWebSocket('ws://localhost/api/v1/ws');
    ws.connect();
    const mock = MockWebSocket.instances[0];
    mock.simulateOpen();

    expect(count(mock, 'ping')).toBe(0);

    vi.advanceTimersByTime(30000);
    expect(count(mock, 'ping')).toBe(1);

    vi.advanceTimersByTime(30000);
    expect(count(mock, 'ping')).toBe(2);

    // No further pings after disconnect clears the interval.
    ws.disconnect();
    vi.advanceTimersByTime(60000);
    expect(count(mock, 'ping')).toBe(2);
  });
});
