import { useEffect, useRef } from 'react';
import type { SSEEvent } from '../types';

export function useSSE(url: string, onEvent: (event: SSEEvent) => void) {
  const esRef = useRef<EventSource | null>(null);
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  useEffect(() => {
    const connect = () => {
      const es = new EventSource(url);
      esRef.current = es;

      es.onmessage = (e) => {
        try {
          const raw = JSON.parse(e.data as string) as Record<string, unknown>;
          const data = (raw.data && typeof raw.data === 'object') ? raw.data as Record<string, unknown> : {};
          const event: SSEEvent = {
            version: typeof raw.version === 'number' ? raw.version : undefined,
            type: String(raw.type ?? raw.event ?? 'unknown'),
            source: typeof raw.source === 'string' ? raw.source : undefined,
            ts: String(raw.ts ?? raw.time ?? new Date().toISOString()),
            path: typeof raw.path === 'string' ? raw.path : (typeof data.path === 'string' ? data.path : undefined),
            blockAddr: typeof data.blockAddr === 'number' ? data.blockAddr : undefined,
            inodeNum: typeof data.inodeNum === 'number' ? data.inodeNum : undefined,
            txnId: typeof data.txnId === 'number' ? data.txnId : undefined,
            blockCount: typeof data.blockCount === 'number' ? data.blockCount : undefined,
            dirty: typeof data.dirty === 'boolean' ? data.dirty : undefined,
            blocks: Array.isArray(data.blocks) ? data.blocks.filter((v): v is number => typeof v === 'number') : undefined,
            message: typeof data.message === 'string' ? data.message : undefined,
          };
          onEventRef.current(event);
        } catch {
          // ignore parse errors
        }
      };

      es.onerror = () => {
        es.close();
        setTimeout(() => {
          connect();
        }, 3000);
      };
    };

    connect();

    return () => {
      esRef.current?.close();
    };
  }, [url]);
}
