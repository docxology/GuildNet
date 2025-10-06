import type { LogLine } from './types';

type WSState = 'connecting' | 'open' | 'closed' | 'reconnecting' | 'error';

type Listener = (...args: any[]) => void;

class Emitter {
  private map = new Map<string, Set<Listener>>();
  on(ev: string, fn: Listener) {
    if (!this.map.has(ev)) this.map.set(ev, new Set());
    this.map.get(ev)!.add(fn);
    return () => this.off(ev, fn);
  }
  off(ev: string, fn: Listener) {
    this.map.get(ev)?.delete(fn);
  }
  emit(ev: string, ...args: any[]) {
    this.map.get(ev)?.forEach((fn) => fn(...args));
  }
}

export class WSManager extends Emitter {
  private url: string;
  private ws?: WebSocket;
  private state: WSState = 'closed';
  private retries = 0;
  private maxRetries = 10;
  private heartbeatInterval = 15000;
  private heartbeatTimer?: number;
  private connectTimer?: number;
  private lastPong = Date.now();

  constructor(url: string, opts?: { maxRetries?: number; heartbeatInterval?: number }) {
    super();
    this.url = url;
    if (opts?.maxRetries != null) this.maxRetries = opts.maxRetries;
    if (opts?.heartbeatInterval != null) this.heartbeatInterval = opts.heartbeatInterval;
  }

  getState() { return this.state; }

  open() {
    this.cleanup();
    this.state = this.retries > 0 ? 'reconnecting' : 'connecting';
    this.emit('state', this.state, this.retries);
    try {
      this.ws = new WebSocket(this.url);
    } catch (e) {
      this.fail(e instanceof Error ? e.message : String(e));
      return;
    }
    this.ws.onopen = () => {
      this.state = 'open';
      this.retries = 0;
      this.emit('state', this.state, this.retries);
      this.startHeartbeat();
    };
    this.ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        if (ev.data === 'pong') { this.lastPong = Date.now(); return; }
        try {
          const obj = JSON.parse(ev.data);
          this.emit('message', obj);
        } catch {
          // ignore malformed line
        }
      }
    };
    this.ws.onerror = () => {
      this.fail('ws error');
    };
    this.ws.onclose = () => {
      this.fail('ws closed');
    };
  }

  private fail(reason: string) {
    this.cleanup();
    if (this.state === 'closed') return;
    this.state = 'error';
    this.emit('state', this.state, this.retries, reason);
    if (this.retries >= this.maxRetries) {
      this.state = 'closed';
      this.emit('state', this.state, this.retries, 'max_retries');
      return;
    }
    const backoff = Math.min(30000, (2 ** this.retries) * 500) * (0.8 + Math.random() * 0.4);
    this.retries++;
    this.connectTimer = window.setTimeout(() => this.open(), backoff);
    this.emit('backoff', backoff, this.retries);
  }

  private startHeartbeat() {
    this.stopHeartbeat();
    this.lastPong = Date.now();
    this.heartbeatTimer = window.setInterval(() => {
      if (!this.ws || this.state !== 'open') return;
      try { this.ws.send('ping'); } catch {}
      if (Date.now() - this.lastPong > this.heartbeatInterval * 2) {
        try { this.ws.close(); } catch {}
      }
    }, this.heartbeatInterval) as unknown as number;
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) window.clearInterval(this.heartbeatTimer);
    this.heartbeatTimer = undefined;
  }

  close() {
    this.cleanup();
    this.state = 'closed';
    try { this.ws?.close(); } catch {}
    this.emit('state', this.state, this.retries);
  }

  private cleanup() {
    if (this.connectTimer) window.clearTimeout(this.connectTimer);
    this.connectTimer = undefined;
    this.stopHeartbeat();
  }
}

export function openLogsStream(params: { target: string; level: 'info' | 'debug' | 'error'; tail?: number }) {
  const qs = new URLSearchParams({ target: params.target, level: params.level, tail: String(params.tail ?? 200) });
  const url = `${location.origin.replace('http', 'ws')}/ws/logs?${qs.toString()}`;
  const ws = new WSManager(url);
  return ws;
}

export function ringBuffer<T>(capacity: number) {
  let items: T[] = [];
  return {
    push(item: T) {
      items.push(item);
      if (items.length > capacity) items.shift();
    },
    pushMany(arr: T[]) {
      for (const it of arr) this.push(it);
    },
    clear() { items = []; },
    get() { return items; },
    size() { return items.length; }
  };
}
