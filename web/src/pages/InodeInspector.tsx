import { useState } from 'react';
import { inspectApi, fsApi } from '../api/client';
import type { InodeInfo } from '../types';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';

export default function InodeInspector() {
  const [searchType, setSearchType] = useState<'inode' | 'path'>('inode');
  const [inodeNum, setInodeNum] = useState('');
  const [path, setPath] = useState('');
  const [inode, setInode] = useState<InodeInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rawJson, setRawJson] = useState(false);

  const search = async () => {
    setLoading(true);
    setError(null);
    setInode(null);
    try {
      if (searchType === 'inode') {
        const num = parseInt(inodeNum, 10);
        if (isNaN(num)) throw new Error('Invalid inode number');
        const data = await inspectApi.inode(num) as unknown as InodeInfo;
        setInode(data);
      } else {
        const stat = await fsApi.stat(path) as { stat?: { inodeNum?: number } };
        const num = stat?.stat?.inodeNum;
        if (!num) throw new Error('Could not get inode from stat');
        const data = await inspectApi.inode(num) as unknown as InodeInfo;
        setInode(data);
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const fieldColor = (key: string): string => {
    if (['mode', 'modeString', 'permissions'].includes(key)) return 'text-[#e3b341]';
    if (['uid', 'gid'].includes(key)) return 'text-[#79c0ff]';
    if (['size', 'blocks', 'blocksCount'].includes(key)) return 'text-[#3fb950]';
    if (key.includes('time') || key.includes('Time')) return 'text-[#d2a8ff]';
    return 'text-[#c9d1d9]';
  };

  return (
    <div className="flex flex-col h-full gap-4 p-4">
      {/* Search bar */}
      <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4">
        <div className="flex gap-2 mb-3">
          <button
            className={`px-3 py-1 rounded text-sm border transition-colors ${searchType === 'inode' ? 'bg-[#388bfd] text-white border-[#388bfd]' : 'bg-[#21262d] text-[#8b949e] border-[#30363d] hover:border-[#484f58]'}`}
            onClick={() => setSearchType('inode')}
          >
            By Inode #
          </button>
          <button
            className={`px-3 py-1 rounded text-sm border transition-colors ${searchType === 'path' ? 'bg-[#388bfd] text-white border-[#388bfd]' : 'bg-[#21262d] text-[#8b949e] border-[#30363d] hover:border-[#484f58]'}`}
            onClick={() => setSearchType('path')}
          >
            By Path
          </button>
        </div>

        <div className="flex gap-2">
          {searchType === 'inode' ? (
            <input
              type="number"
              className="flex-1 bg-[#0d1117] border border-[#30363d] rounded px-3 py-2 text-sm text-[#c9d1d9] placeholder-[#484f58] focus:outline-none focus:border-[#388bfd] font-mono"
              placeholder="Inode number (e.g. 2)"
              value={inodeNum}
              onChange={e => setInodeNum(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && search()}
            />
          ) : (
            <input
              type="text"
              className="flex-1 bg-[#0d1117] border border-[#30363d] rounded px-3 py-2 text-sm text-[#c9d1d9] placeholder-[#484f58] focus:outline-none focus:border-[#388bfd] font-mono"
              placeholder="File path (e.g. /etc/passwd)"
              value={path}
              onChange={e => setPath(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && search()}
            />
          )}
          <Button onClick={search} disabled={loading}
            className="bg-[#388bfd] hover:bg-[#58a6ff] text-white border-transparent">
            {loading ? 'Searching...' : 'Inspect'}
          </Button>
        </div>

        {error && (
          <div className="mt-2 text-sm text-[#f85149] bg-[#1f2937] border border-[#f85149] rounded px-3 py-2">
            {error}
          </div>
        )}
      </div>

      {/* Inode detail */}
      {inode && (
        <div className="flex-1 min-h-0 flex flex-col gap-4">
          {/* Header */}
          <div className="flex items-center gap-3">
            <h3 className="text-base font-semibold text-[#c9d1d9]">Inode #{inode.inodeNum}</h3>
            <Badge className="bg-[#388bfd] text-white border-[#388bfd]">
              {inode.interpreted?.fileType ?? 'unknown'}
            </Badge>
            <Badge className="bg-[#21262d] text-[#e3b341] border-[#30363d] font-mono">
              {inode.interpreted?.modeString ?? inode.interpreted?.mode ?? ''}
            </Badge>
            <div className="ml-auto flex gap-2">
              <button
                className={`text-xs px-2 py-1 rounded border transition-colors ${!rawJson ? 'bg-[#388bfd] text-white border-[#388bfd]' : 'bg-[#21262d] text-[#8b949e] border-[#30363d]'}`}
                onClick={() => setRawJson(false)}
              >
                Formatted
              </button>
              <button
                className={`text-xs px-2 py-1 rounded border transition-colors ${rawJson ? 'bg-[#388bfd] text-white border-[#388bfd]' : 'bg-[#21262d] text-[#8b949e] border-[#30363d]'}`}
                onClick={() => setRawJson(true)}
              >
                Raw JSON
              </button>
            </div>
          </div>

          {rawJson ? (
            <div className="bg-[#161b22] border border-[#30363d] rounded-lg flex-1 overflow-hidden">
              <ScrollArea className="h-full">
                <pre className="p-4 text-xs text-[#c9d1d9] font-mono">{JSON.stringify(inode, null, 2)}</pre>
              </ScrollArea>
            </div>
          ) : (
            <div className="flex gap-4 flex-1 min-h-0">
              {/* Interpreted fields */}
              <div className="flex-1 min-w-0 bg-[#161b22] border border-[#30363d] rounded-lg flex flex-col overflow-hidden">
                <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
                  Interpreted
                </div>
                <ScrollArea className="flex-1">
                  <div className="p-4 space-y-2">
                    {Object.entries(inode.interpreted ?? {}).map(([k, v]) => (
                      <div key={k} className="flex gap-2 text-sm border-b border-[#21262d] pb-1">
                        <span className="text-[#8b949e] w-32 shrink-0 font-mono">{k}</span>
                        <span className={`font-mono ${fieldColor(k)}`}>{v}</span>
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              </div>

              {/* Raw fields */}
              <div className="w-60 bg-[#161b22] border border-[#30363d] rounded-lg flex flex-col overflow-hidden">
                <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
                  Raw
                </div>
                <ScrollArea className="flex-1">
                  <div className="p-4 space-y-1">
                    {Object.entries(inode.raw ?? {}).map(([k, v]) => (
                      <div key={k} className="flex gap-2 text-xs">
                        <span className="text-[#8b949e] w-28 shrink-0 font-mono">{k}</span>
                        <span className="text-[#79c0ff] font-mono">0x{v.toString(16).padStart(8, '0')}</span>
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              </div>

              {/* Block pointers */}
              <div className="w-64 bg-[#161b22] border border-[#30363d] rounded-lg flex flex-col overflow-hidden">
                <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
                  Block Pointers
                </div>
                <ScrollArea className="flex-1">
                  <div className="p-4 space-y-4">
                    <div>
                      <div className="text-xs text-[#8b949e] mb-2 font-semibold">Direct ({inode.blockPointers?.direct?.length ?? 0})</div>
                      <div className="grid grid-cols-4 gap-1">
                        {inode.blockPointers?.direct?.map((b, i) => (
                          <div
                            key={i}
                            className={`text-xs font-mono rounded px-1 py-0.5 text-center border ${b ? 'bg-[#1f2937] text-[#3fb950] border-[#2ea043]' : 'bg-[#161b22] text-[#484f58] border-[#21262d]'}`}
                          >
                            {b || '-'}
                          </div>
                        ))}
                      </div>
                    </div>

                    {inode.blockPointers?.indirect1 && (
                      <div>
                        <div className="text-xs text-[#8b949e] mb-2 font-semibold">
                          Indirect1 → <span className="text-[#e3b341]">{inode.blockPointers.indirect1.block}</span>
                        </div>
                        <div className="grid grid-cols-4 gap-1">
                          {inode.blockPointers.indirect1.entries.map((b, i) => (
                            <div key={i} className="text-xs font-mono rounded px-1 py-0.5 text-center border bg-[#1f2937] text-[#e3b341] border-[#e3b341]">
                              {b}
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    <div>
                      <div className="text-xs text-[#8b949e] mb-2 font-semibold">All Data Blocks</div>
                      <div className="flex flex-wrap gap-1">
                        {inode.dataBlocks?.map((b, i) => (
                          <Badge key={i} className="text-xs bg-[#21262d] text-[#d2a8ff] border-[#30363d] font-mono">
                            {b}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  </div>
                </ScrollArea>
              </div>
            </div>
          )}
        </div>
      )}

      {!inode && !loading && (
        <div className="flex-1 flex items-center justify-center">
          <div className="text-center space-y-2">
            <div className="text-[#484f58] text-4xl">🔍</div>
            <div className="text-[#484f58] text-sm">Search by inode number or file path to inspect inode details</div>
            <div className="text-xs text-[#30363d]">Try inode #2 (root directory)</div>
          </div>
        </div>
      )}
    </div>
  );
}
