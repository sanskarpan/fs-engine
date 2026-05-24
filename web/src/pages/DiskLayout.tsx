import { useEffect, useState, useRef } from 'react';
import { motion } from 'framer-motion';
import { inspectApi } from '../api/client';
import { useFsStore } from '../store/fsStore';
import type { BlockMapEntry, InodeInfo, CacheState } from '../types';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Button } from '@/components/ui/button';

const BLOCK_COLORS: Record<string, string> = {
  boot: '#1a1a2e',
  superblock: '#16213e',
  inode_bitmap: '#0f3460',
  block_bitmap: '#0f3460',
  inode_table: '#533483',
  journal: '#e94560',
  data_used: '#28a745',
  data_free: '#1a1a1a',
  cache_dirty: '#ffc107',
};

const BLOCK_LABELS: Record<string, string> = {
  boot: 'Boot',
  superblock: 'Superblock',
  inode_bitmap: 'Inode Bitmap',
  block_bitmap: 'Block Bitmap',
  inode_table: 'Inode Table',
  journal: 'Journal',
  data_used: 'Data (Used)',
  data_free: 'Data (Free)',
  cache_dirty: 'Cache (Dirty)',
};

const COLS = 128;

interface DiskInfo {
  totalBlocks?: number;
  blockSize?: number;
  superblock?: { freeBlocks?: number; freeInodes?: number; state?: string };
  blockMap?: BlockMapEntry[];
}

export default function DiskLayout() {
  const [diskInfo, setDiskInfo] = useState<DiskInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [hoveredBlock, setHoveredBlock] = useState<BlockMapEntry | null>(null);
  const [blockDetail, setBlockDetail] = useState<Record<string, unknown> | null>(null);
  const [inodeDetail, setInodeDetail] = useState<InodeInfo | null>(null);
  const [cacheState, setCacheState] = useState<CacheState | null>(null);
  const tooltipRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });

  const { diskLayout, setDiskLayout, selectedBlock, setSelectedBlock, flashBlocks, dirtyBlocks } = useFsStore();

  const loadDisk = async () => {
    setLoading(true);
    try {
      const data = await inspectApi.disk() as DiskInfo;
      setDiskInfo(data);
      if (data.blockMap) setDiskLayout(data.blockMap);
    } finally {
      setLoading(false);
    }
  };

  const loadCache = async () => {
    try {
      const data = await inspectApi.cache() as unknown as CacheState;
      setCacheState(data);
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    loadDisk();
    loadCache();
    const interval = setInterval(() => {
      loadDisk();
      loadCache();
    }, 5000);
    return () => clearInterval(interval);
  }, []);

  const selectBlock = async (addr: number) => {
    setSelectedBlock(addr);
    try {
      const data = await inspectApi.block(addr) as Record<string, unknown>;
      setBlockDetail(data);
      setInodeDetail(null);
    } catch {
      setBlockDetail(null);
    }
  };

  const selectInode = async (num: number) => {
    try {
      const data = await inspectApi.inode(num) as unknown as InodeInfo;
      setInodeDetail(data);
    } catch {
      setInodeDetail(null);
    }
  };

  const getBlockColor = (entry: BlockMapEntry): string => {
    if (dirtyBlocks.has(entry.addr)) return BLOCK_COLORS.cache_dirty;
    return BLOCK_COLORS[entry.type] ?? '#1a1a1a';
  };

  const blocks = diskLayout.length > 0 ? diskLayout : [];
  const rows = Math.ceil(blocks.length / COLS);

  return (
    <div className="flex flex-col h-full gap-4 p-4">
      {/* Header */}
      <div className="flex items-center gap-4">
        <h2 className="text-lg font-semibold text-[#c9d1d9]">Disk Layout</h2>
        <Button size="sm" onClick={loadDisk} disabled={loading}
          className="bg-[#21262d] hover:bg-[#30363d] text-[#c9d1d9] border border-[#30363d]">
          {loading ? 'Loading...' : 'Refresh'}
        </Button>
        {diskInfo?.superblock && (
          <div className="flex gap-4 text-sm ml-auto">
            <span className="text-[#8b949e]">Free Blocks: <span className="text-[#3fb950]">{diskInfo.superblock.freeBlocks}</span></span>
            <span className="text-[#8b949e]">Free Inodes: <span className="text-[#3fb950]">{diskInfo.superblock.freeInodes}</span></span>
            <span className="text-[#8b949e]">State: <span className="text-[#e3b341]">{diskInfo.superblock.state}</span></span>
            <span className="text-[#8b949e]">Block Size: <span className="text-[#c9d1d9]">{diskInfo.blockSize}B</span></span>
          </div>
        )}
      </div>

      {/* Legend */}
      <div className="flex flex-wrap gap-2">
        {Object.entries(BLOCK_LABELS).map(([type, label]) => (
          <div key={type} className="flex items-center gap-1.5 text-xs text-[#8b949e]">
            <div className="w-3 h-3 rounded-sm border border-[#30363d]" style={{ backgroundColor: BLOCK_COLORS[type] }} />
            {label}
          </div>
        ))}
      </div>

      <div className="flex gap-4 flex-1 min-h-0">
        {/* Block map */}
        <div className="flex-1 min-w-0">
          <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4 h-full overflow-auto">
            {blocks.length === 0 ? (
              <div className="text-center text-[#484f58] py-16">No disk data loaded. Click Refresh.</div>
            ) : (
              <div
                className="inline-grid gap-0.5"
                style={{ gridTemplateColumns: `repeat(${COLS}, 10px)` }}
              >
                {blocks.map((block) => {
                  const isFlash = flashBlocks.has(block.addr);
                  const isSelected = selectedBlock === block.addr;
                  const bgColor = isFlash ? '#ffffff' : getBlockColor(block);
                  return (
                    <motion.div
                      key={block.addr}
                      title={`Block ${block.addr}: ${block.type}${block.path ? ` (${block.path})` : ''}`}
                      className={`w-2.5 h-2.5 rounded-sm cursor-pointer ${isFlash ? 'ring-1 ring-white ring-offset-0' : ''} ${isSelected ? 'ring-1 ring-[#58a6ff]' : ''}`}
                      animate={{ backgroundColor: bgColor }}
                      transition={{ duration: 0.3 }}
                      onMouseEnter={(e) => {
                        setHoveredBlock(block);
                        tooltipRef.current = { x: e.clientX, y: e.clientY };
                        setTooltipPos({ x: e.clientX, y: e.clientY });
                      }}
                      onMouseLeave={() => setHoveredBlock(null)}
                      onClick={() => selectBlock(block.addr)}
                    />
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* Right panel */}
        <div className="w-80 flex flex-col gap-3">
          {/* Block detail */}
          <div className="bg-[#161b22] border border-[#30363d] rounded-lg flex-1 flex flex-col overflow-hidden">
            <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
              Block Detail {selectedBlock !== null && `#${selectedBlock}`}
            </div>
            <ScrollArea className="flex-1">
              {blockDetail ? (
                <div className="p-3 space-y-1">
                  {Object.entries(blockDetail).map(([k, v]) => (
                    <div key={k} className="flex gap-2 text-xs">
                      <span className="text-[#8b949e] w-24 shrink-0 font-mono">{k}:</span>
                      <span className="text-[#c9d1d9] font-mono break-all">{JSON.stringify(v)}</span>
                    </div>
                  ))}
                  {blockDetail.inodeNum !== undefined && (
                    <Button
                      size="sm"
                      className="mt-2 w-full bg-[#21262d] hover:bg-[#30363d] text-[#c9d1d9] border border-[#30363d]"
                      onClick={() => selectInode(blockDetail.inodeNum as number)}
                    >
                      Inspect Inode #{blockDetail.inodeNum as number}
                    </Button>
                  )}
                </div>
              ) : (
                <div className="p-4 text-sm text-[#484f58]">Click a block to inspect</div>
              )}
            </ScrollArea>
          </div>

          {/* Inode detail */}
          {inodeDetail && (
            <div className="bg-[#161b22] border border-[#30363d] rounded-lg flex-1 flex flex-col overflow-hidden">
              <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
                Inode #{inodeDetail.inodeNum}
              </div>
              <ScrollArea className="flex-1">
                <div className="p-3 space-y-3">
                  <div>
                    <div className="text-xs text-[#8b949e] mb-1 uppercase tracking-wider">Interpreted</div>
                    {Object.entries(inodeDetail.interpreted ?? {}).map(([k, v]) => (
                      <div key={k} className="flex gap-2 text-xs">
                        <span className="text-[#8b949e] w-20 shrink-0">{k}:</span>
                        <span className="text-[#c9d1d9] font-mono">{v}</span>
                      </div>
                    ))}
                  </div>
                  <div>
                    <div className="text-xs text-[#8b949e] mb-1 uppercase tracking-wider">Direct Blocks</div>
                    <div className="flex flex-wrap gap-1">
                      {inodeDetail.blockPointers?.direct?.map((b, i) => (
                        <Badge key={i} className="text-xs bg-[#21262d] text-[#3fb950] border-[#30363d] font-mono"
                          onClick={() => b && selectBlock(b)}>
                          {b || '-'}
                        </Badge>
                      ))}
                    </div>
                  </div>
                  {inodeDetail.blockPointers?.indirect1 && (
                    <div>
                      <div className="text-xs text-[#8b949e] mb-1">Indirect1 → Block {inodeDetail.blockPointers.indirect1.block}</div>
                      <div className="flex flex-wrap gap-1">
                        {inodeDetail.blockPointers.indirect1.entries.map((b, i) => (
                          <Badge key={i} className="text-xs bg-[#21262d] text-[#e3b341] border-[#30363d] font-mono cursor-pointer"
                            onClick={() => b && selectBlock(b)}>
                            {b}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </ScrollArea>
            </div>
          )}

          {/* Cache state */}
          {cacheState && (
            <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-3 space-y-2">
              <div className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold">Cache</div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                <span className="text-[#8b949e]">Capacity:</span><span className="text-[#c9d1d9] font-mono">{cacheState.capacity}</span>
                <span className="text-[#8b949e]">Used:</span><span className="text-[#c9d1d9] font-mono">{cacheState.used}</span>
                <span className="text-[#8b949e]">Hit Rate:</span><span className="text-[#3fb950] font-mono">{(cacheState.hitRate * 100).toFixed(1)}%</span>
                <span className="text-[#8b949e]">Dirty:</span><span className="text-[#ffc107] font-mono">{cacheState.dirtyBlocks?.length ?? 0}</span>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Tooltip */}
      {hoveredBlock && (
        <div
          className="fixed z-50 pointer-events-none bg-[#161b22] border border-[#30363d] rounded px-2 py-1.5 text-xs text-[#c9d1d9] shadow-lg"
          style={{ left: tooltipPos.x + 12, top: tooltipPos.y - 40 }}
        >
          <div className="font-semibold text-[#58a6ff]">Block #{hoveredBlock.addr}</div>
          <div className="text-[#8b949e]">{BLOCK_LABELS[hoveredBlock.type] ?? hoveredBlock.type}</div>
          {hoveredBlock.path && <div className="text-[#3fb950]">{hoveredBlock.path}</div>}
        </div>
      )}

      <div className="text-xs text-[#484f58]">
        {rows} rows × {COLS} cols = {blocks.length} blocks displayed
      </div>
    </div>
  );
}
