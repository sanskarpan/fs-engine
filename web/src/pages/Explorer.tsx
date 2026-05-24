import { useState, useCallback, useRef, useEffect } from 'react';
import { fsApi } from '../api/client';
import type { DirEntry, StatResult } from '../types';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';

const FILE_ICONS: Record<string, string> = {
  dir: '📁',
  file: '📄',
  symlink: '🔗',
  unknown: '❓',
};

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}K`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}M`;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

interface TreeNode {
  name: string;
  path: string;
  inodeNum: number;
  fileType: string;
  modeString: string;
  children?: TreeNode[];
  expanded?: boolean;
}

interface ContextMenu {
  x: number;
  y: number;
  node: TreeNode;
}

function updateNodeTree(nodes: TreeNode[], path: string, updater: (node: TreeNode) => TreeNode): TreeNode[] {
  return nodes.map((node) => {
    if (node.path === path) {
      return updater(node);
    }
    if (node.children) {
      return { ...node, children: updateNodeTree(node.children, path, updater) };
    }
    return node;
  });
}

export default function Explorer() {
  const [tree, setTree] = useState<TreeNode[]>([]);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [selectedStat, setSelectedStat] = useState<StatResult | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newPath, setNewPath] = useState('');
  const [newContent, setNewContent] = useState('');
  const [actionFeedback, setActionFeedback] = useState<string | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenu | null>(null);
  const [renameTarget, setRenameTarget] = useState<string | null>(null);
  const [renameTo, setRenameTo] = useState('');
  const contextMenuRef = useRef<HTMLDivElement>(null);

  const loadDir = useCallback(async (path: string): Promise<TreeNode[]> => {
    const res = await fsApi.readdir(path) as { entries?: DirEntry[] };
    const entries = res.entries ?? [];
    return entries
      .filter((e) => e.name !== '.' && e.name !== '..')
      .map((e) => ({
        name: e.name,
        path: path === '/' ? `/${e.name}` : `${path}/${e.name}`,
        inodeNum: e.inodeNum,
        fileType: e.fileType,
        modeString: e.modeString,
      }));
  }, []);

  const loadRoot = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const nodes = await loadDir('/');
      setTree(nodes);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [loadDir]);

  const toggleExpand = useCallback(async (node: TreeNode, nodes: TreeNode[], setNodes: (n: TreeNode[]) => void) => {
    if (node.fileType !== 'dir') return;
    if (node.expanded) {
      setNodes(updateNodeTree(nodes, node.path, (current) => ({ ...current, expanded: false, children: [] })));
      return;
    }
    try {
      const children = await loadDir(node.path);
      setNodes(updateNodeTree(nodes, node.path, (current) => ({ ...current, expanded: true, children })));
    } catch (e) {
      setError(String(e));
    }
  }, [loadDir]);

  const selectNode = useCallback(async (node: TreeNode) => {
    setSelectedPath(node.path);
    setFileContent(null);
    setError(null);
    try {
      const stat = await fsApi.stat(node.path) as { stat?: StatResult };
      setSelectedStat(stat.stat ?? null);
      if (node.fileType === 'file') {
        const res = await fsApi.read(node.path) as { data?: string };
        setFileContent(res.data ?? '');
      }
    } catch (e) {
      setError(String(e));
    }
  }, []);

  const showFeedback = (msg: string) => {
    setActionFeedback(msg);
    setTimeout(() => setActionFeedback(null), 3000);
  };

  // Close context menu on outside click.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (contextMenuRef.current && !contextMenuRef.current.contains(e.target as Node)) {
        setContextMenu(null);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const openContextMenu = (e: React.MouseEvent, node: TreeNode) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, node });
  };

  const ctxStat = async () => {
    if (!contextMenu) return;
    setContextMenu(null);
    await selectNode(contextMenu.node);
  };

  const ctxViewInode = async () => {
    if (!contextMenu) return;
    setContextMenu(null);
    await selectNode(contextMenu.node);
    // Navigate to inode inspector via URL (best-effort)
    window.open(`/inode?path=${encodeURIComponent(contextMenu.node.path)}`, '_blank');
  };

  const ctxDelete = async () => {
    if (!contextMenu) return;
    const node = contextMenu.node;
    setContextMenu(null);
    if (!confirm(`Delete ${node.path}?`)) return;
    setLoading(true);
    try {
      if (node.fileType === 'dir') {
        await fsApi.rmdir(node.path);
      } else {
        await fsApi.unlink(node.path);
      }
      showFeedback(`Deleted ${node.path}`);
      if (selectedPath === node.path) {
        setSelectedPath(null);
        setSelectedStat(null);
        setFileContent(null);
      }
      await loadRoot();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const ctxRename = () => {
    if (!contextMenu) return;
    setRenameTarget(contextMenu.node.path);
    setRenameTo(contextMenu.node.name);
    setContextMenu(null);
  };

  const handleRename = async () => {
    if (!renameTarget || !renameTo) return;
    const parts = renameTarget.split('/');
    parts[parts.length - 1] = renameTo;
    const newFullPath = parts.join('/');
    setLoading(true);
    try {
      await fsApi.rename(renameTarget, newFullPath);
      showFeedback(`Renamed to ${newFullPath}`);
      setRenameTarget(null);
      setRenameTo('');
      await loadRoot();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const ctxNewFile = () => {
    if (!contextMenu) return;
    const base = contextMenu.node.fileType === 'dir' ? contextMenu.node.path : contextMenu.node.path.split('/').slice(0, -1).join('/') || '/';
    setNewPath(`${base}/newfile`);
    setContextMenu(null);
  };

  const ctxNewDir = () => {
    if (!contextMenu) return;
    const base = contextMenu.node.fileType === 'dir' ? contextMenu.node.path : contextMenu.node.path.split('/').slice(0, -1).join('/') || '/';
    setNewPath(`${base}/newdir`);
    setContextMenu(null);
  };

  const handleCreate = async () => {
    if (!newPath) return;
    setLoading(true);
    try {
      await fsApi.write(newPath, newContent, 0, ['O_CREAT', 'O_WRONLY']);
      showFeedback(`Created ${newPath}`);
      setNewPath('');
      setNewContent('');
      await loadRoot();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleMkdir = async () => {
    if (!newPath) return;
    setLoading(true);
    try {
      await fsApi.mkdir(newPath);
      showFeedback(`Created directory ${newPath}`);
      setNewPath('');
      await loadRoot();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleUnlink = async () => {
    if (!selectedPath) return;
    if (!confirm(`Delete ${selectedPath}?`)) return;
    setLoading(true);
    try {
      await fsApi.unlink(selectedPath);
      showFeedback(`Deleted ${selectedPath}`);
      setSelectedPath(null);
      setSelectedStat(null);
      setFileContent(null);
      await loadRoot();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleFormat = async () => {
    if (!confirm('Format the filesystem? ALL DATA WILL BE LOST!')) return;
    setLoading(true);
    try {
      await fsApi.format();
      showFeedback('Filesystem formatted');
      setTree([]);
      setSelectedPath(null);
      setSelectedStat(null);
      setFileContent(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleFsck = async () => {
    setLoading(true);
    try {
      const res = await fsApi.fsck() as { result?: string; errors?: string[] };
      showFeedback(`fsck: ${res.result ?? JSON.stringify(res)}`);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const renderTree = (nodes: TreeNode[], depth = 0): React.ReactNode => {
    return nodes.map((node) => (
      <div key={node.path}>
        <div
          className={`flex items-center gap-1 px-2 py-0.5 rounded cursor-pointer hover:bg-[#161b22] transition-colors text-sm ${selectedPath === node.path ? 'bg-[#1f2937] text-[#58a6ff]' : 'text-[#c9d1d9]'}`}
          style={{ paddingLeft: `${8 + depth * 16}px` }}
          onClick={() => {
            if (node.fileType === 'dir') toggleExpand(node, tree, setTree);
            selectNode(node);
          }}
          onContextMenu={(e) => openContextMenu(e, node)}
        >
          <span className="text-xs opacity-60">{node.expanded ? '▼' : node.fileType === 'dir' ? '▶' : ' '}</span>
          <span>{FILE_ICONS[node.fileType] ?? FILE_ICONS.unknown}</span>
          <span className="truncate">{node.name}</span>
          <span className="ml-auto text-xs opacity-50 font-mono">{node.modeString}</span>
        </div>
        {node.expanded && node.children && renderTree(node.children, depth + 1)}
      </div>
    ));
  };

  const breadcrumbs = selectedPath
    ? ['/', ...selectedPath.split('/').filter(Boolean)]
    : ['/'];

  return (
    <div className="flex flex-col h-full gap-4 p-4">
      {/* Breadcrumbs */}
      <nav className="flex items-center gap-1 text-sm text-[#8b949e] flex-wrap">
        {breadcrumbs.map((crumb, i) => {
          const fullPath = i === 0 ? '/' : '/' + breadcrumbs.slice(1, i + 1).join('/');
          const isLast = i === breadcrumbs.length - 1;
          return (
            <span key={i} className="flex items-center gap-1">
              {i > 0 && <span className="text-[#30363d]">/</span>}
              <button
                className={`hover:text-[#58a6ff] transition-colors ${isLast ? 'text-[#c9d1d9] font-medium' : 'hover:underline'}`}
                onClick={() => !isLast && setSelectedPath(fullPath === '/' ? null : fullPath)}
                disabled={isLast}
              >
                {crumb}
              </button>
            </span>
          );
        })}
      </nav>
      {/* Toolbar */}
      <div className="flex items-center gap-2 flex-wrap">
        <Button size="sm" onClick={loadRoot} disabled={loading}
          className="bg-[#21262d] hover:bg-[#30363d] text-[#c9d1d9] border border-[#30363d]">
          Load /
        </Button>
        <input
          className="flex-1 min-w-[180px] bg-[#161b22] border border-[#30363d] rounded px-3 py-1.5 text-sm text-[#c9d1d9] placeholder-[#484f58] focus:outline-none focus:border-[#388bfd]"
          placeholder="/path/to/file"
          value={newPath}
          onChange={e => setNewPath(e.target.value)}
        />
        <Button size="sm" onClick={handleCreate} disabled={loading || !newPath}
          className="bg-[#238636] hover:bg-[#2ea043] text-white border border-[#2ea043]">
          New File
        </Button>
        <Button size="sm" onClick={handleMkdir} disabled={loading || !newPath}
          className="bg-[#21262d] hover:bg-[#30363d] text-[#c9d1d9] border border-[#30363d]">
          Mkdir
        </Button>
        <Button size="sm" onClick={handleUnlink} disabled={loading || !selectedPath}
          className="bg-[#21262d] hover:bg-[#da3633] text-[#f85149] border border-[#f85149] hover:text-white">
          Delete
        </Button>
        <div className="ml-auto flex gap-2">
          <Button size="sm" onClick={handleFsck} disabled={loading}
            className="bg-[#21262d] hover:bg-[#30363d] text-[#e3b341] border border-[#30363d]">
            fsck
          </Button>
          <Button size="sm" onClick={handleFormat} disabled={loading}
            className="bg-[#21262d] hover:bg-[#da3633] text-[#f85149] border border-[#f85149] hover:text-white">
            Format
          </Button>
        </div>
      </div>

      {actionFeedback && (
        <div className="bg-[#1f2937] border border-[#2ea043] rounded px-3 py-2 text-sm text-[#3fb950]">
          {actionFeedback}
        </div>
      )}
      {error && (
        <div className="bg-[#1f2937] border border-[#f85149] rounded px-3 py-2 text-sm text-[#f85149]">
          {error}
        </div>
      )}

      <div className="flex gap-4 flex-1 min-h-0">
        {/* File tree */}
        <div className="w-72 flex-shrink-0 bg-[#161b22] border border-[#30363d] rounded-lg overflow-hidden flex flex-col">
          <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
            File Tree
          </div>
          <ScrollArea className="flex-1">
            <div className="py-1">
              {tree.length === 0 ? (
                <div className="text-center text-[#484f58] text-sm py-8">
                  Click "Load /" to browse filesystem
                </div>
              ) : (
                renderTree(tree)
              )}
            </div>
          </ScrollArea>
        </div>

        {/* Detail panel */}
        <div className="flex-1 min-w-0 flex flex-col gap-4">
          {selectedStat ? (
            <>
              {/* Stat info */}
              <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4">
                <div className="flex items-center gap-2 mb-3">
                  <span className="text-[#8b949e] text-sm">Path:</span>
                  <span className="text-[#58a6ff] font-mono text-sm">{selectedPath}</span>
                  <Badge className="ml-auto bg-[#21262d] text-[#8b949e] border-[#30363d]">
                    {selectedStat.fileType}
                  </Badge>
                </div>
                <div className="grid grid-cols-2 md:grid-cols-3 gap-x-6 gap-y-2 text-sm">
                  {[
                    ['Inode', selectedStat.inodeNum],
                    ['Mode', selectedStat.modeString],
                    ['Size', formatSize(selectedStat.size)],
                    ['Links', selectedStat.links],
                    ['Blocks', selectedStat.blocks],
                    ['UID/GID', `${selectedStat.uid}/${selectedStat.gid}`],
                    ['atime', formatDate(selectedStat.atime)],
                    ['mtime', formatDate(selectedStat.mtime)],
                    ['ctime', formatDate(selectedStat.ctime)],
                  ].map(([label, val]) => (
                    <div key={String(label)} className="flex gap-2">
                      <span className="text-[#8b949e] w-16 shrink-0">{label}:</span>
                      <span className="text-[#c9d1d9] font-mono truncate">{String(val)}</span>
                    </div>
                  ))}
                </div>
              </div>

              {/* File content / write */}
              <div className="bg-[#161b22] border border-[#30363d] rounded-lg flex-1 overflow-hidden flex flex-col">
                <div className="px-3 py-2 border-b border-[#30363d] text-xs text-[#8b949e] uppercase tracking-wider font-semibold">
                  Content
                </div>
                <ScrollArea className="flex-1">
                  {fileContent !== null ? (
                    <pre className="p-4 text-sm text-[#c9d1d9] whitespace-pre-wrap break-all">{fileContent || '(empty)'}</pre>
                  ) : (
                    <div className="p-4 text-sm text-[#484f58]">
                      {selectedStat.fileType === 'dir' ? 'Directory — select a file to view contents' : 'No content available'}
                    </div>
                  )}
                </ScrollArea>
                {selectedStat.fileType === 'file' && (
                  <div className="border-t border-[#30363d] p-3 flex flex-col gap-2">
                    <textarea
                      className="w-full bg-[#0d1117] border border-[#30363d] rounded p-2 text-sm text-[#c9d1d9] font-mono resize-none focus:outline-none focus:border-[#388bfd]"
                      rows={4}
                      placeholder="New content to write..."
                      value={newContent}
                      onChange={e => setNewContent(e.target.value)}
                    />
                    <Button size="sm" onClick={async () => {
                      if (!selectedPath) return;
                      try {
                        await fsApi.write(selectedPath, newContent, 0, ['O_WRONLY']);
                        showFeedback('File written');
                        const res = await fsApi.read(selectedPath) as { data?: string };
                        setFileContent(res.data ?? '');
                      } catch (e) {
                        setError(String(e));
                      }
                    }}
                      className="self-end bg-[#238636] hover:bg-[#2ea043] text-white border border-[#2ea043]">
                      Write
                    </Button>
                  </div>
                )}
              </div>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-center text-[#484f58] text-sm">
              Select a file or directory to inspect
            </div>
          )}
        </div>
      </div>

      {/* Context menu */}
      {contextMenu && (
        <div
          ref={contextMenuRef}
          className="fixed z-50 bg-[#161b22] border border-[#30363d] rounded-lg shadow-xl py-1 min-w-[160px]"
          style={{ top: contextMenu.y, left: contextMenu.x }}
        >
          {[
            { label: 'Stat', action: ctxStat },
            { label: 'View Inode', action: ctxViewInode },
            null,
            { label: 'New File', action: ctxNewFile },
            { label: 'New Directory', action: ctxNewDir },
            { label: 'Rename', action: ctxRename },
            null,
            { label: 'Delete', action: ctxDelete, danger: true },
          ].map((item, i) =>
            item === null ? (
              <div key={i} className="border-t border-[#30363d] my-1" />
            ) : (
              <button
                key={item.label}
                className={`w-full text-left px-3 py-1.5 text-sm hover:bg-[#21262d] transition-colors ${item.danger ? 'text-[#f85149]' : 'text-[#c9d1d9]'}`}
                onClick={item.action}
              >
                {item.label}
              </button>
            )
          )}
        </div>
      )}

      {/* Rename dialog */}
      {renameTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4 w-80 shadow-xl">
            <div className="text-sm font-semibold text-[#c9d1d9] mb-3">Rename {renameTarget}</div>
            <input
              className="w-full bg-[#0d1117] border border-[#30363d] rounded px-3 py-1.5 text-sm text-[#c9d1d9] focus:outline-none focus:border-[#388bfd] mb-3"
              value={renameTo}
              onChange={e => setRenameTo(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') handleRename(); if (e.key === 'Escape') setRenameTarget(null); }}
              autoFocus
            />
            <div className="flex gap-2 justify-end">
              <Button size="sm" onClick={() => setRenameTarget(null)}
                className="bg-[#21262d] hover:bg-[#30363d] text-[#8b949e] border border-[#30363d]">
                Cancel
              </Button>
              <Button size="sm" onClick={handleRename} disabled={loading || !renameTo}
                className="bg-[#238636] hover:bg-[#2ea043] text-white border border-[#2ea043]">
                Rename
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
