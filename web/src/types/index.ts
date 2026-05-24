export interface StatResult {
  inodeNum: number;
  mode: string;
  modeOctal: string;
  modeString: string;
  uid: number;
  gid: number;
  size: number;
  links: number;
  blocks: number;
  atime: string;
  mtime: string;
  ctime: string;
  fileType: string;
}

export interface DirEntry {
  name: string;
  inodeNum: number;
  fileType: string;
  modeString: string;
}

export interface BlockMapEntry {
  addr: number;
  type: 'boot' | 'superblock' | 'inode_bitmap' | 'block_bitmap' | 'inode_table' | 'journal' | 'data_used' | 'data_free' | 'cache_dirty';
  inodeNum?: number;
  path?: string;
}

export interface InodeInfo {
  inodeNum: number;
  raw: Record<string, number>;
  interpreted: Record<string, string>;
  blockPointers: {
    direct: number[];
    indirect1: { block: number; entries: number[] } | null;
    indirect2: null;
    indirect3: null;
  };
  dataBlocks: number[];
}

export interface JournalEntry {
  id: number;
  status: 'committed' | 'in-progress' | 'checkpointed';
  blocks: number[];
  timestamp: string;
  operations: string[];
}

export interface MetricsSnapshot {
  reads: number;
  writes: number;
  cacheHits: number;
  cacheMisses: number;
  journalCommits: number;
  blocksAllocated: number;
  blocksFreed: number;
  inodesAllocated: number;
  inodesFreed: number;
  hitRate: number;
}

export interface SSEEvent {
  version?: number;
  type: string;
  source?: string;
  blockAddr?: number;
  inodeNum?: number;
  path?: string;
  txnId?: number;
  blockCount?: number;
  dirty?: boolean;
  ts: string;
  message?: string;
  blocks?: number[];
}

export interface DiskLayout {
  totalBlocks: number;
  blockSize: number;
  superblock: {
    freeBlocks: number;
    freeInodes: number;
    state: string;
  };
  blockMap: BlockMapEntry[];
}

export interface CacheState {
  capacity: number;
  used: number;
  dirtyBlocks: number[];
  hitRate: number;
  entries: Array<{
    addr: number;
    dirty: boolean;
    pinned: number;
    lastUsed: string;
  }>;
}
