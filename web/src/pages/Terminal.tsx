import { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import { useSSE } from '../hooks/useSSE';
import { useFsStore } from '../store/fsStore';
import type { SSEEvent } from '../types';
import { ScrollArea } from '@/components/ui/scroll-area';

const EVENT_COLORS: Record<string, string> = {
  block_alloc: '#3fb950',
  block_free: '#f85149',
  journal_commit: '#388bfd',
  cache_evict: '#ffc107',
  inode_alloc: '#a371f7',
  inode_free: '#f78166',
};

function formatTime(ts: string): string {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
  } catch {
    return ts;
  }
}

function describeEvent(event: SSEEvent): string {
  switch (event.type) {
    case 'block_alloc': return `Block ${event.blockAddr ?? '?'} allocated${event.path ? ` for ${event.path}` : ''}`;
    case 'block_free': return `Block ${event.blockAddr ?? '?'} freed`;
    case 'journal_commit': return `Journal txn #${event.txnId ?? '?'} committed`;
    case 'cache_evict': return `Block ${event.blockAddr ?? '?'} evicted from cache`;
    case 'inode_alloc': return `Inode ${event.inodeNum ?? '?'} allocated${event.path ? ` (${event.path})` : ''}`;
    case 'inode_free': return `Inode ${event.inodeNum ?? '?'} freed`;
    default: return event.type;
  }
}

function terminalBackendURL(): URL {
  if (typeof window === 'undefined') {
    return new URL('ws://localhost:8080/ws/shell');
  }

  const envOrigin = import.meta.env.VITE_BACKEND_ORIGIN as string | undefined;
  if (envOrigin) {
    const base = new URL(envOrigin);
    base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:';
    base.pathname = '/ws/shell';
    base.search = '';
    base.hash = '';
    return base;
  }

  if (import.meta.env.DEV && window.location.port !== '8080') {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return new URL(`${proto}//${window.location.hostname}:8080/ws/shell`);
  }

  return new URL(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws/shell`);
}

export default function Terminal() {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const lineBufferRef = useRef('');
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [recentOps, setRecentOps] = useState<SSEEvent[]>([]);

  const { handleSSEEvent } = useFsStore();

  useSSE('/sse', (event: SSEEvent) => {
    handleSSEEvent(event);
    setRecentOps(prev => [event, ...prev.slice(0, 9)]);
  });

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerm({
      theme: {
        background: '#0d1117',
        foreground: '#c9d1d9',
        cursor: '#58a6ff',
        selectionBackground: '#1f2937',
        black: '#0d1117',
        red: '#f85149',
        green: '#3fb950',
        yellow: '#e3b341',
        blue: '#388bfd',
        magenta: '#a371f7',
        cyan: '#56d364',
        white: '#c9d1d9',
        brightBlack: '#484f58',
        brightRed: '#ff7b72',
        brightGreen: '#56d364',
        brightYellow: '#ffa657',
        brightBlue: '#79c0ff',
        brightMagenta: '#d2a8ff',
        brightCyan: '#56d364',
        brightWhite: '#f0f6fc',
      },
      fontFamily: "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
      fontSize: 14,
      lineHeight: 1.4,
      cursorBlink: true,
      allowTransparency: true,
      scrollback: 5000,
    });

    const fitAddon = new FitAddon();
    const linksAddon = new WebLinksAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(linksAddon);
    term.open(containerRef.current);
    fitAddon.fit();

    termRef.current = term;
    fitRef.current = fitAddon;

    term.writeln('\x1b[1;34m╔══════════════════════════════════════════╗\x1b[0m');
    term.writeln('\x1b[1;34m║\x1b[0m  \x1b[1;32mFilesystem Engine\x1b[0m — Interactive Shell  \x1b[1;34m║\x1b[0m');
    term.writeln('\x1b[1;34m╚══════════════════════════════════════════╝\x1b[0m');
    term.writeln('');
    term.writeln('\x1b[33mConnecting to backend WebSocket...\x1b[0m');

    const wsUrl = terminalBackendURL().toString();
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setError(null);
      term.writeln('\x1b[32m✓ Connected!\x1b[0m');
      term.writeln('');
    };

    ws.onmessage = (e) => {
      try {
        const payload = JSON.parse(e.data as string) as { output?: string };
        term.write(payload.output ?? '');
      } catch {
        term.write(String(e.data));
      }
    };

    ws.onerror = () => {
      setConnected(false);
      setError('WebSocket connection failed');
      term.writeln('');
      term.writeln('\x1b[31m✗ WebSocket connection failed.\x1b[0m');
      term.writeln('\x1b[33mThe backend server may not be running or WebSocket endpoint not available.\x1b[0m');
      term.writeln('\x1b[33mThis shell is line-oriented; commands are submitted on Enter.\x1b[0m');
      term.writeln('');
      term.writeln('\x1b[90m$ \x1b[0m');
    };

    ws.onclose = () => {
      setConnected(false);
      term.writeln('');
      term.writeln('\x1b[33m[Connection closed. Refresh to reconnect.]\x1b[0m');
    };

    term.onData((data) => {
      if (data === '\r') {
        term.write('\r\n');
        const command = lineBufferRef.current;
        lineBufferRef.current = '';
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ command }));
        }
        return;
      }

      if (data === '\u007f') {
        if (lineBufferRef.current.length > 0) {
          lineBufferRef.current = lineBufferRef.current.slice(0, -1);
          term.write('\b \b');
        }
        return;
      }

      if (data >= ' ') {
        lineBufferRef.current += data;
        term.write(data);
      }
    });

    const handleResize = () => {
      fitAddon.fit();
    };

    const ro = new ResizeObserver(handleResize);
    if (containerRef.current) ro.observe(containerRef.current);

    return () => {
      ro.disconnect();
      ws.close();
      term.dispose();
    };
  }, []);

  const handleClear = () => {
    termRef.current?.clear();
  };

  const handleReconnect = () => {
    setError(null);
    window.location.reload();
  };

  return (
    <div className="flex gap-3 h-full p-4">
      {/* Terminal */}
      <div className="flex-1 flex flex-col gap-3 min-w-0">
        {/* Toolbar */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div className={`w-2.5 h-2.5 rounded-full ${connected ? 'bg-[#3fb950]' : 'bg-[#f85149]'}`} />
            <span className="text-sm text-[#8b949e]">
              {connected ? 'Connected' : error ? 'Disconnected' : 'Connecting...'}
            </span>
          </div>
          <div className="text-xs text-[#484f58] font-mono">{typeof window !== 'undefined' ? terminalBackendURL().toString() : 'ws://localhost:8080/ws/shell'}</div>
          <div className="ml-auto flex gap-2">
            <button
              className="text-xs px-3 py-1 rounded border bg-[#21262d] text-[#8b949e] border-[#30363d] hover:border-[#484f58] hover:text-[#c9d1d9] transition-colors"
              onClick={handleClear}
            >
              Clear
            </button>
            {!connected && (
              <button
                className="text-xs px-3 py-1 rounded border bg-[#21262d] text-[#e3b341] border-[#e3b341] hover:bg-[#1f2d0d] transition-colors"
                onClick={handleReconnect}
              >
                Reconnect
              </button>
            )}
          </div>
        </div>

        {/* Terminal container */}
        <div className="flex-1 bg-[#0d1117] border border-[#30363d] rounded-lg overflow-hidden">
          <div ref={containerRef} className="h-full w-full" style={{ padding: '8px' }} />
        </div>

        {/* Keyboard hints */}
        <div className="flex gap-4 text-xs text-[#484f58]">
          <span><kbd className="px-1 py-0.5 bg-[#21262d] border border-[#30363d] rounded text-[#8b949e]">Enter</kbd> run command</span>
          <span><kbd className="px-1 py-0.5 bg-[#21262d] border border-[#30363d] rounded text-[#8b949e]">Backspace</kbd> edit line</span>
          <span className="ml-auto">WebSocket shell bridge</span>
        </div>
      </div>

      {/* Side panel: last 10 live operations */}
      <div className="w-64 flex-shrink-0 bg-[#161b22] border border-[#30363d] rounded-lg flex flex-col overflow-hidden">
        <div className="px-3 py-2 border-b border-[#30363d] flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-[#3fb950] animate-pulse" />
          <span className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold">Live Operations</span>
        </div>
        <ScrollArea className="flex-1">
          {recentOps.length === 0 ? (
            <div className="text-center text-[#484f58] py-8 text-xs">Waiting for events...</div>
          ) : (
            <div className="divide-y divide-[#21262d]">
              {recentOps.map((event, i) => (
                <div key={i} className="px-3 py-2 hover:bg-[#1c2128] transition-colors">
                  <div className="flex items-center gap-1.5 mb-0.5">
                    <div
                      className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                      style={{ backgroundColor: EVENT_COLORS[event.type] ?? '#484f58' }}
                    />
                    <span
                      className="text-xs font-semibold font-mono truncate"
                      style={{ color: EVENT_COLORS[event.type] ?? '#8b949e' }}
                    >
                      {event.type}
                    </span>
                    <span className="ml-auto text-xs text-[#484f58] font-mono flex-shrink-0">
                      {formatTime(event.ts)}
                    </span>
                  </div>
                  <div className="text-xs text-[#8b949e] pl-3 leading-relaxed">
                    {describeEvent(event)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </ScrollArea>
      </div>
    </div>
  );
}
