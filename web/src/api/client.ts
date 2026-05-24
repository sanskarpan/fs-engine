import ky from 'ky';

import type { CacheState, DirEntry, InodeInfo, JournalEntry, StatResult } from '../types';

const api = ky.create({ prefix: '/api', timeout: 30000 });

function mapStat(stat: Record<string, unknown>): StatResult {
  return {
    inodeNum: Number(stat.inode ?? 0),
    mode: String(stat.mode ?? ''),
    modeOctal: String(stat.mode ?? ''),
    modeString: String(stat.mode_str ?? ''),
    uid: Number(stat.uid ?? 0),
    gid: Number(stat.gid ?? 0),
    size: Number(stat.size ?? 0),
    links: Number(stat.links ?? 0),
    blocks: Number(stat.blocks ?? 0),
    atime: String(stat.atime ?? ''),
    mtime: String(stat.mtime ?? ''),
    ctime: String(stat.ctime ?? ''),
    fileType: Boolean(stat.is_dir) ? 'dir' : Boolean(stat.is_link) ? 'symlink' : 'file',
  };
}

function buildQuery(path: string): string {
  return `?path=${encodeURIComponent(path)}`;
}

export const fsApi = {
  async stat(path: string): Promise<{ stat: StatResult }> {
    const stat = await api.get(`fs/stat${buildQuery(path)}`).json<Record<string, unknown>>();
    return { stat: mapStat(stat) };
  },

  async readdir(path: string): Promise<{ entries: DirEntry[] }> {
    const resp = await api.get(`fs/ls${buildQuery(path)}`).json<{ entries?: Array<Record<string, unknown>> }>();
    return {
      entries: (resp.entries ?? []).map((entry) => ({
        name: String(entry.name ?? ''),
        inodeNum: Number(entry.inode ?? 0),
        fileType: String(entry.type_name ?? 'unknown'),
        modeString: '',
      })),
    };
  },

  async read(path: string): Promise<{ data: string; length: number }> {
    const resp = await api.get(`fs/read${buildQuery(path)}`).json<{ content?: string; length?: number }>();
    const content = resp.content ? atob(resp.content) : '';
    return { data: content, length: Number(resp.length ?? content.length) };
  },

  write: async (path: string, data: string, offset = 0, _flags: string[] = ['O_CREAT', 'O_WRONLY']) =>
    api.post('fs/write', { json: { path, content: btoa(data), offset } }).json<Record<string, unknown>>(),

  mkdir: (path: string, mode = 0o755) =>
    api.post('fs/mkdir', { json: { path, mode } }).json<Record<string, unknown>>(),

  unlink: (path: string) =>
    api.delete(`fs/unlink${buildQuery(path)}`).json<Record<string, unknown>>(),

  rmdir: (path: string) =>
    api.delete(`fs/rmdir${buildQuery(path)}`).json<Record<string, unknown>>(),

  rename: (oldPath: string, newPath: string) =>
    api.post('fs/rename', { json: { old_path: oldPath, new_path: newPath } }).json<Record<string, unknown>>(),

  chmod: (path: string, mode: number) =>
    api.post('fs/chmod', { json: { path, mode } }).json<Record<string, unknown>>(),

  symlink: (target: string, linkPath: string) =>
    api.post('fs/symlink', { json: { target, path: linkPath } }).json<Record<string, unknown>>(),

  readlink: (path: string) =>
    api.get(`fs/readlink${buildQuery(path)}`).json<Record<string, unknown>>(),

  format: () => api.post('fs/format').json<Record<string, unknown>>(),
  fsck: () => api.post('fs/fsck').json<Record<string, unknown>>(),
  crash: () => api.post('fs/crash').json<Record<string, unknown>>(),
};

export const inspectApi = {
  disk: () => api.get('inspect/disk').json<Record<string, unknown>>(),

  async inode(num: number): Promise<InodeInfo> {
    const resp = await api.get(`inspect/inode/${num}`).json<Record<string, unknown>>();
    const direct = Array.isArray(resp.direct) ? resp.direct.map((v) => Number(v ?? 0)) : [];
    return {
      inodeNum: Number(resp.inode ?? num),
      raw: {
        mode: parseInt(String(resp.mode ?? '0'), 8) || 0,
        uid: Number(resp.uid ?? 0),
        gid: Number(resp.gid ?? 0),
        links: Number(resp.links ?? 0),
        blocks512: Number(resp.blocks512 ?? 0),
      },
      interpreted: {
        fileType: Boolean(resp.is_dir) ? 'dir' : Boolean(resp.is_symlink) ? 'symlink' : 'file',
        mode: String(resp.mode ?? ''),
        modeString: String(resp.mode_str ?? ''),
        uid: String(resp.uid ?? ''),
        gid: String(resp.gid ?? ''),
        size: String(resp.size ?? ''),
        links: String(resp.links ?? ''),
        atime: String(resp.atime ?? ''),
        mtime: String(resp.mtime ?? ''),
        ctime: String(resp.ctime ?? ''),
      },
      blockPointers: {
        direct,
        indirect1: resp.indirect1 ? { block: Number(resp.indirect1), entries: [] } : null,
        indirect2: null,
        indirect3: null,
      },
      dataBlocks: Array.isArray(resp.data_blocks) ? resp.data_blocks.map((v) => Number(v ?? 0)) : [],
    };
  },

  block: (addr: number) => api.get(`inspect/block/${addr}`).json<Record<string, unknown>>(),

  async journal(): Promise<{ entries: JournalEntry[] }> {
    const resp = await api.get('inspect/journal').json<{ entries?: JournalEntry[] }>();
    return { entries: resp.entries ?? [] };
  },

  async cache(): Promise<CacheState> {
    const resp = await api.get('inspect/cache').json<Record<string, unknown>>();
    return {
      capacity: Number(resp.capacity ?? 0),
      used: Number(resp.size ?? 0),
      dirtyBlocks: [],
      hitRate: Number(resp.hit_rate ?? 0),
      entries: [],
    };
  },

  tree: (path = '/', depth = 3) =>
    api.get(`inspect/tree?path=${encodeURIComponent(path)}&depth=${depth}`).json<Record<string, unknown>>(),
};

export const metricsApi = {
  get: () => api.get('metrics').json<Record<string, unknown>>(),
};
