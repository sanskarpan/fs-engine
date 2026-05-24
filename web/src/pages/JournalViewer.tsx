import { useState, useEffect } from 'react';
import { inspectApi, fsApi } from '../api/client';
import { useSSE } from '../hooks/useSSE';
import { useFsStore } from '../store/fsStore';
import type { JournalEntry, SSEEvent } from '../types';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Button } from '@/components/ui/button';

const STATUS_COLORS: Record<string, { bg: string; text: string; border: string }> = {
  committed: { bg: '#0d2620', text: '#3fb950', border: '#2ea043' },
  'in-progress': { bg: '#1f2d0d', text: '#e3b341', border: '#9e6a03' },
  checkpointed: { bg: '#1c1c1c', text: '#484f58', border: '#30363d' },
};

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

export default function JournalViewer() {
  const [journal, setJournal] = useState<JournalEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [crashing, setCrashing] = useState(false);
  const [crashMsg, setCrashMsg] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [filter, setFilter] = useState<string>('all');
  const { recentEvents, handleSSEEvent } = useFsStore();

  useSSE('/sse', (event: SSEEvent) => {
    handleSSEEvent(event);
    if (event.type === 'journal_commit') {
      loadJournal();
    }
  });

  const loadJournal = async () => {
    setLoading(true);
    try {
      const data = await inspectApi.journal() as { entries?: JournalEntry[] };
      setJournal(data.entries ?? []);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadJournal();
  }, []);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(loadJournal, 3000);
    return () => clearInterval(interval);
  }, [autoRefresh]);

  const simulateCrash = async () => {
    setCrashing(true);
    setCrashMsg(null);
    try {
      const res = await fsApi.crash() as { message?: string };
      setCrashMsg(res.message ?? 'Crash simulated; journal replayed.');
      await loadJournal();
    } catch (e) {
      setCrashMsg('Crash simulation failed.');
    } finally {
      setCrashing(false);
    }
  };

  const filteredJournal = filter === 'all' ? journal : journal.filter(j => j.status === filter);

  return (
    <div className="flex flex-col h-full gap-4 p-4">
      {/* Header */}
      <div className="flex items-center gap-3 flex-wrap">
        <h2 className="text-lg font-semibold text-[#c9d1d9]">Journal Viewer</h2>
        <div className="flex gap-1">
          {['all', 'committed', 'in-progress', 'checkpointed'].map(f => (
            <button
              key={f}
              className={`text-xs px-2 py-1 rounded border transition-colors ${filter === f ? 'bg-[#388bfd] text-white border-[#388bfd]' : 'bg-[#21262d] text-[#8b949e] border-[#30363d] hover:border-[#484f58]'}`}
              onClick={() => setFilter(f)}
            >
              {f}
            </button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-2">
          <label className="flex items-center gap-2 text-sm text-[#8b949e] cursor-pointer">
            <div
              className={`w-8 h-4 rounded-full transition-colors relative cursor-pointer ${autoRefresh ? 'bg-[#238636]' : 'bg-[#30363d]'}`}
              onClick={() => setAutoRefresh(v => !v)}
            >
              <div className={`absolute top-0.5 w-3 h-3 bg-white rounded-full transition-transform ${autoRefresh ? 'translate-x-4' : 'translate-x-0.5'}`} />
            </div>
            Auto-refresh
          </label>
          <Button size="sm" onClick={loadJournal} disabled={loading}
            className="bg-[#21262d] hover:bg-[#30363d] text-[#c9d1d9] border border-[#30363d]">
            {loading ? '...' : 'Refresh'}
          </Button>
          <Button size="sm" onClick={simulateCrash} disabled={crashing}
            className="bg-[#6e1c1c] hover:bg-[#8b2020] text-[#f85149] border border-[#6e1c1c]">
            {crashing ? 'Crashing...' : '⚡ Simulate Crash'}
          </Button>
        </div>
      </div>
      {crashMsg && (
        <div className="text-xs px-3 py-2 rounded bg-[#1f2d0d] border border-[#2ea043] text-[#3fb950]">
          {crashMsg}
        </div>
      )}

      <div className="flex gap-4 flex-1 min-h-0">
        {/* Journal transactions */}
        <div className="flex-1 min-w-0 bg-[#161b22] border border-[#30363d] rounded-lg flex flex-col overflow-hidden">
          <div className="px-3 py-2 border-b border-[#30363d] flex items-center justify-between">
            <span className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold">Transactions</span>
            <Badge className="bg-[#21262d] text-[#8b949e] border-[#30363d] text-xs">{filteredJournal.length}</Badge>
          </div>
          <ScrollArea className="flex-1">
            {filteredJournal.length === 0 ? (
              <div className="text-center text-[#484f58] py-12 text-sm">No journal entries</div>
            ) : (
              <div className="divide-y divide-[#21262d]">
                {[...filteredJournal].reverse().map((entry) => {
                  const colors = STATUS_COLORS[entry.status] ?? STATUS_COLORS.committed;
                  return (
                    <div key={entry.id} className="p-3 hover:bg-[#1c2128] transition-colors">
                      <div className="flex items-center gap-2 mb-2">
                        <span className="font-mono text-sm font-semibold text-[#c9d1d9]">TXN #{entry.id}</span>
                        <span
                          className="text-xs px-2 py-0.5 rounded border font-semibold"
                          style={{ backgroundColor: colors.bg, color: colors.text, borderColor: colors.border }}
                        >
                          {entry.status}
                        </span>
                        <span className="ml-auto text-xs text-[#484f58] font-mono">{formatTime(entry.timestamp)}</span>
                      </div>

                      {entry.operations?.length > 0 && (
                        <div className="space-y-0.5 mb-2">
                          {entry.operations.map((op, i) => (
                            <div key={i} className="text-xs text-[#8b949e] font-mono pl-2 border-l border-[#30363d]">
                              {op}
                            </div>
                          ))}
                        </div>
                      )}

                      {entry.blocks?.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-1">
                          <span className="text-xs text-[#484f58]">blocks:</span>
                          {entry.blocks.map((b, i) => (
                            <Badge key={i} className="text-xs bg-[#21262d] text-[#79c0ff] border-[#30363d] font-mono">
                              {b}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </ScrollArea>
        </div>

        {/* Live SSE events */}
        <div className="w-80 bg-[#161b22] border border-[#30363d] rounded-lg flex flex-col overflow-hidden">
          <div className="px-3 py-2 border-b border-[#30363d] flex items-center gap-2">
            <div className="w-2 h-2 rounded-full bg-[#3fb950] animate-pulse" />
            <span className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold">Live Events</span>
            <Badge className="ml-auto bg-[#21262d] text-[#8b949e] border-[#30363d] text-xs">{recentEvents.length}</Badge>
          </div>
          <ScrollArea className="flex-1">
            {recentEvents.length === 0 ? (
              <div className="text-center text-[#484f58] py-8 text-xs">Waiting for events...</div>
            ) : (
              <div className="divide-y divide-[#21262d]">
                {recentEvents.map((event, i) => (
                  <div key={i} className="px-3 py-2 hover:bg-[#1c2128] transition-colors">
                    <div className="flex items-center gap-2 mb-0.5">
                      <span
                        className="text-xs font-semibold font-mono"
                        style={{ color: EVENT_COLORS[event.type] ?? '#8b949e' }}
                      >
                        {event.type}
                      </span>
                      <span className="ml-auto text-xs text-[#484f58] font-mono">{formatTime(event.ts)}</span>
                    </div>
                    <div className="text-xs text-[#8b949e] font-mono space-y-0.5">
                      {event.blockAddr !== undefined && <div>block: <span className="text-[#79c0ff]">{event.blockAddr}</span></div>}
                      {event.inodeNum !== undefined && <div>inode: <span className="text-[#d2a8ff]">{event.inodeNum}</span></div>}
                      {event.path && <div>path: <span className="text-[#3fb950]">{event.path}</span></div>}
                      {event.txnId !== undefined && <div>txn: <span className="text-[#388bfd]">#{event.txnId}</span></div>}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </ScrollArea>
        </div>
      </div>

      {/* Stats bar */}
      <div className="flex gap-6 text-sm bg-[#161b22] border border-[#30363d] rounded-lg px-4 py-2">
        {Object.entries(
          recentEvents.reduce<Record<string, number>>((acc, e) => ({ ...acc, [e.type]: (acc[e.type] ?? 0) + 1 }), {})
        ).map(([type, count]) => (
          <div key={type} className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full" style={{ backgroundColor: EVENT_COLORS[type] ?? '#484f58' }} />
            <span className="text-[#8b949e] text-xs">{type}:</span>
            <span className="text-xs font-mono" style={{ color: EVENT_COLORS[type] ?? '#c9d1d9' }}>{count}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
