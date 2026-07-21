"use client";

import { useEffect, useMemo, useRef } from "react";

type EventHandler = (data: unknown) => void;
type StatusHandler = (status: ConnectionStatus) => void;

export type ConnectionStatus = "connecting" | "connected" | "disconnected" | "reconnecting";

export interface WebSocketConfig {
  url?: string | (() => Promise<string>);
  factory?: () => Promise<WebSocket>;
  onMessage?: EventHandler;
  onStatusChange?: StatusHandler;
  onError?: (error: Event) => void;
  maxRetries?: number;
  baseDelay?: number;
  maxDelay?: number;
}

const INITIAL_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 30000;
const MAX_RETRIES = 10;

export class WebSocketManager {
  private ws: WebSocket | null = null;
  private config: WebSocketConfig;
  private retryCount = 0;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private aborted = false;
  private _status: ConnectionStatus = "disconnected";
  private buffer: string[] = [];
  private initialConnection = true;

  constructor(config: WebSocketConfig) {
    this.config = {
      maxRetries: MAX_RETRIES,
      baseDelay: INITIAL_BACKOFF_MS,
      maxDelay: MAX_BACKOFF_MS,
      ...config,
    };
  }

  get status() {
    return this._status;
  }

  set onMessage(handler: EventHandler | undefined) {
    this.config.onMessage = handler;
  }

  set onStatusChange(handler: StatusHandler | undefined) {
    this.config.onStatusChange = handler;
  }

  private setStatus(status: ConnectionStatus) {
    this._status = status;
    this.config.onStatusChange?.(status);
  }

  private getDelay(): number {
    const delay = Math.min(
      this.config.baseDelay! * Math.pow(2, this.retryCount),
      this.config.maxDelay!,
    );
    const jitter = Math.random() * Math.min(delay, 1000);
    return delay + jitter;
  }

  async connect() {
    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return;
    }

    this.aborted = false;
    this.setStatus(this.initialConnection ? "connecting" : "reconnecting");

    try {
      if (this.config.factory) {
        this.ws = await this.config.factory();
      } else if (this.config.url) {
        const url = typeof this.config.url === "function" ? await this.config.url() : this.config.url;
        this.ws = new WebSocket(url);
      } else {
        this.setStatus("disconnected");
        return;
      }
    } catch {
      this.initialConnection = false;
      this.scheduleReconnect();
      return;
    }

    this.ws.onopen = () => {
      this.retryCount = 0;
      this.initialConnection = false;
      this.setStatus("connected");
      this.flushBuffer();
    };

    this.ws.onmessage = (event) => {
      if (this.aborted) return;
      try {
        const data = JSON.parse(event.data);
        this.config.onMessage?.(data);
      } catch {
        this.config.onMessage?.(event.data);
      }
    };

    this.ws.onerror = () => {
      if (this.aborted) return;
      this.config.onError?.(new Event("WebSocket error"));
    };

    this.ws.onclose = () => {
      if (this.aborted) return;
      this.setStatus("disconnected");
      this.initialConnection = false;
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect() {
    if (this.aborted) return;
    if (this.retryCount >= this.config.maxRetries!) {
      this.setStatus("disconnected");
      return;
    }

    this.setStatus("reconnecting");
    const delay = this.getDelay();
    this.retryCount++;

    this.retryTimer = setTimeout(() => {
      if (!this.aborted) {
        void this.connect();
      }
    }, delay);
  }

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(data);
    } else if (typeof data === "string") {
      this.buffer.push(data);
    }
  }

  private flushBuffer() {
    if (this.buffer.length === 0) return;
    const pending = this.buffer.splice(0);
    for (const cmd of pending) {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(cmd);
      }
    }
  }

  disconnect() {
    this.aborted = true;
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    this.ws?.close();
    this.ws = null;
    this.buffer = [];
    this.setStatus("disconnected");
  }

  reconnect() {
    this.aborted = true;
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    this.ws?.close();
    this.ws = null;
    this.retryCount = 0;
    this.initialConnection = true;
    this.aborted = false;
    void this.connect();
  }
}

export function useWebSocket(config: WebSocketConfig) {
  const managerRef = useRef<WebSocketManager | null>(null);

  useEffect(() => {
    const manager = new WebSocketManager(config);
    managerRef.current = manager;
    void manager.connect();
    return () => {
      manager.disconnect();
      managerRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return useMemo(
    () => ({
      send: (data: string | ArrayBufferLike | Blob | ArrayBufferView) => managerRef.current?.send(data),
      disconnect: () => managerRef.current?.disconnect(),
      reconnect: () => managerRef.current?.reconnect(),
      get status() {
        return managerRef.current?.status ?? "disconnected";
      },
    }),
    [],
  );
}
