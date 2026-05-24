import { create } from 'zustand';
import type { BlockMapEntry, SSEEvent, MetricsSnapshot } from '../types';

interface FsStore {
  diskLayout: BlockMapEntry[];
  selectedBlock: number | null;
  selectedInode: number | null;
  dirtyBlocks: Set<number>;
  flashBlocks: Set<number>;
  metrics: MetricsSnapshot | null;
  recentEvents: SSEEvent[];

  setDiskLayout: (layout: BlockMapEntry[]) => void;
  setSelectedBlock: (addr: number | null) => void;
  setSelectedInode: (num: number | null) => void;
  handleSSEEvent: (event: SSEEvent) => void;
  setMetrics: (m: MetricsSnapshot) => void;
}

export const useFsStore = create<FsStore>((set) => ({
  diskLayout: [],
  selectedBlock: null,
  selectedInode: null,
  dirtyBlocks: new Set(),
  flashBlocks: new Set(),
  metrics: null,
  recentEvents: [],

  setDiskLayout: (layout) => set({ diskLayout: layout }),
  setSelectedBlock: (addr) => set({ selectedBlock: addr }),
  setSelectedInode: (num) => set({ selectedInode: num }),

  handleSSEEvent: (event) =>
    set((state) => {
      const next: Partial<FsStore> = {
        recentEvents: [event, ...state.recentEvents.slice(0, 99)],
      };

      if (event.type === 'block_alloc' && event.blockAddr !== undefined) {
        const flash = new Set(state.flashBlocks);
        flash.add(event.blockAddr);
        next.flashBlocks = flash;
        const dirty = new Set(state.dirtyBlocks);
        dirty.add(event.blockAddr);
        next.dirtyBlocks = dirty;
        setTimeout(() => {
          useFsStore.setState((s) => {
            const f = new Set(s.flashBlocks);
            f.delete(event.blockAddr!);
            return { flashBlocks: f };
          });
        }, 800);
      }

      if (event.type === 'cache_evict' && event.blockAddr !== undefined) {
        const dirty = new Set(state.dirtyBlocks);
        dirty.delete(event.blockAddr);
        next.dirtyBlocks = dirty;
      }

      if (event.type === 'cache_flush' && event.blockAddr !== undefined) {
        const dirty = new Set(state.dirtyBlocks);
        dirty.delete(event.blockAddr);
        next.dirtyBlocks = dirty;
      }

      return next;
    }),

  setMetrics: (m) => set({ metrics: m }),
}));
