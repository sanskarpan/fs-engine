package api

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	fstype "github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/ops"
)

func (s *Server) handleSuperblock(w http.ResponseWriter, r *http.Request) {
	sb := s.fs.Superblock()
	if sb == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("superblock not available"))
		return
	}

	volName := strings.TrimRight(string(sb.VolumeName[:]), "\x00")
	writeJSON(w, http.StatusOK, SuperblockResponse{
		Magic:      sb.Magic,
		InodeCount: sb.InodeCount,
		BlockCount: sb.BlockCount,
		FreeInodes: sb.FreeInodes,
		FreeBlocks: sb.FreeBlocks,
		BlockSize:  sb.BlockSize,
		InodeSize:  sb.InodeSize,
		State:      sb.State,
		MountCount: sb.MountCount,
		Version:    sb.Version,
		VolumeName: volName,
	})
}

func (s *Server) handleInspectInode(w http.ResponseWriter, r *http.Request) {
	numStr := chi.URLParam(r, "num")
	if numStr == "" {
		parts := strings.Split(r.URL.Path, "/")
		numStr = parts[len(parts)-1]
	}
	num, err := strconv.ParseUint(numStr, 10, 32)
	if err != nil || num == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid inode number"))
		return
	}

	di, err := fstype.ReadInode(s.fs.Dev(), uint32(num))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	blocks, _ := fstype.BlockList(s.fs.Dev(), di)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"inode":       num,
		"mode":        fmt.Sprintf("0%04o", di.Mode),
		"mode_str":    di.ModeString(),
		"uid":         di.UID,
		"gid":         di.GID,
		"size":        di.Size(),
		"links":       di.Links,
		"blocks512":   di.Blocks512,
		"atime":       di.ATime,
		"mtime":       di.MTime,
		"ctime":       di.CTime,
		"is_dir":      di.IsDir(),
		"is_regular":  di.IsRegular(),
		"is_symlink":  di.IsSymlink(),
		"direct":      di.Direct,
		"indirect1":   di.Indirect1,
		"indirect2":   di.Indirect2,
		"indirect3":   di.Indirect3,
		"data_blocks": blocks,
	})
}

func (s *Server) handleBlockBitmap(w http.ResponseWriter, r *http.Request) {
	bm, err := fstype.ReadBlockBitmap(s.fs.Dev())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"used":  bm.Count(),
		"total": fstype.TotalBlocks,
		"free":  fstype.TotalBlocks - bm.Count(),
	})
}

func (s *Server) handleInodeBitmap(w http.ResponseWriter, r *http.Request) {
	bm, err := fstype.ReadInodeBitmap(s.fs.Dev())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"used":  bm.Count(),
		"total": fstype.InodeCount,
		"free":  fstype.InodeCount - bm.Count(),
	})
}

func (s *Server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := s.fs.BufCache().Stats()
	var hitRate float64
	total := stats.Hits + stats.Misses
	if total > 0 {
		hitRate = float64(stats.Hits) / float64(total)
	}
	writeJSON(w, http.StatusOK, CacheStatsResponse{
		Hits:      stats.Hits,
		Misses:    stats.Misses,
		Evictions: stats.Evictions,
		Size:      stats.Size,
		Capacity:  stats.Capacity,
		HitRate:   hitRate,
	})
}

func (s *Server) handleDf(w http.ResponseWriter, r *http.Request) {
	total, used, free := s.fs.Df()
	writeJSON(w, http.StatusOK, DfResponse{
		TotalBytes: total,
		UsedBytes:  used,
		FreeBytes:  free,
		TotalMB:    total / (1024 * 1024),
		UsedMB:     used / (1024 * 1024),
		FreeMB:     free / (1024 * 1024),
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m := s.fs.GetMetrics()
	snap := m.Snapshot()
	if r.URL.Query().Get("format") == "prometheus" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		lines := make([]string, 0, len(snap))
		for key, value := range snap {
			lines = append(lines, fmt.Sprintf("fs_engine_%s %d", strings.ReplaceAll(key, "-", "_"), value))
		}
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
		return
	}

	// Compute hit rate.
	hits := snap["cache_hits"]
	misses := snap["cache_miss"]
	var hitRate float64
	if total := hits + misses; total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	// Convert windowed buckets to DTO.
	rawBuckets := m.Windowed.Window()
	buckets := make([]WindowBucketDTO, len(rawBuckets))
	for i, b := range rawBuckets {
		buckets[i] = WindowBucketDTO{
			TS:              b.TS,
			Reads:           b.Reads,
			Writes:          b.Writes,
			CacheHits:       b.CacheHits,
			CacheMiss:       b.CacheMiss,
			JournalCommits:  b.JournalCommits,
			BlocksAllocated: b.BlocksAllocated,
			BlocksFreed:     b.BlocksFreed,
			InodesAllocated: b.InodesAllocated,
			InodesFreed:     b.InodesFreed,
		}
	}

	writeJSON(w, http.StatusOK, MetricsResponse{
		Counters: snap,
		Window:   buckets,
		HitRate:  hitRate,
	})
}

// handleDiskLayout returns the full disk block map for the visualizer.
func (s *Server) handleDiskLayout(w http.ResponseWriter, r *http.Request) {
	blockBM, err := fstype.ReadBlockBitmap(s.fs.Dev())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Dirty blocks from buffer cache.
	cacheStats := s.fs.BufCache().Stats()
	dirtySet := make(map[uint32]bool, len(cacheStats.DirtyAddrs))
	for _, addr := range cacheStats.DirtyAddrs {
		dirtySet[addr] = true
	}

	blockMap := make([]map[string]interface{}, fstype.TotalBlocks)
	for i := uint32(0); i < fstype.TotalBlocks; i++ {
		entry := map[string]interface{}{"addr": i}

		switch {
		case i == fstype.BootBlock:
			entry["type"] = "boot"
		case i == fstype.SuperblockAddr:
			entry["type"] = "superblock"
		case i == fstype.InodeBitmapAddr:
			entry["type"] = "inode_bitmap"
		case i == fstype.BlockBitmapAddr:
			entry["type"] = "block_bitmap"
		case i >= fstype.InodeTableStart && i < fstype.InodeTableStart+fstype.InodeTableBlocks:
			entry["type"] = "inode_table"
			first := (i-fstype.InodeTableStart)*32 + 1
			entry["inode_range"] = [2]uint32{first, first + 31}
		case i >= fstype.JournalStart && i < fstype.JournalStart+fstype.JournalBlocks:
			entry["type"] = "journal"
		default:
			if dirtySet[i] {
				entry["type"] = "cache_dirty"
			} else if blockBM.IsSet(i) {
				entry["type"] = "data_used"
			} else {
				entry["type"] = "data_free"
			}
		}

		blockMap[i] = entry
	}

	sb := s.fs.Superblock()
	stateStr := "clean"
	if sb.State == fstype.StateError {
		stateStr = "error"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"totalBlocks": fstype.TotalBlocks,
		"blockSize":   fstype.BlockSize,
		"superblock": map[string]interface{}{
			"freeBlocks": sb.FreeBlocks,
			"freeInodes": sb.FreeInodes,
			"state":      stateStr,
		},
		"blockMap": blockMap,
	})
}

// handleBlockDetail returns hex dump + interpretation of a raw block.
func (s *Server) handleBlockDetail(w http.ResponseWriter, r *http.Request) {
	addrStr := chi.URLParam(r, "addr")
	if addrStr == "" {
		parts := strings.Split(r.URL.Path, "/")
		addrStr = parts[len(parts)-1]
	}
	addr, err := strconv.ParseUint(addrStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid block address"))
		return
	}

	data, err := s.fs.Dev().ReadBlock(uint32(addr))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	// Show first 256 bytes as hex.
	preview := data
	if len(preview) > 256 {
		preview = preview[:256]
	}
	hexStr := hex.EncodeToString(preview)

	blockType := blockTypeStr(uint32(addr))
	resp := map[string]interface{}{
		"addr": addr,
		"type": blockType,
		"hex":  hexStr,
		"size": len(data),
	}

	// Interpret based on type.
	switch blockType {
	case "directory_data":
		entries, _ := fstype.ParseDirBlock(data)
		resp["dir_entries"] = entries
	case "inode_table":
		inodes := make([]map[string]interface{}, 0, 32)
		for i := 0; i < 32; i++ {
			off := i * fstype.InodeSize
			if off+fstype.InodeSize > len(data) {
				break
			}
			di, err2 := fstype.DecodeInodeBytes(data[off : off+fstype.InodeSize])
			if err2 == nil && di.Links > 0 {
				slotNum := (uint32(addr)-fstype.InodeTableStart)*32 + uint32(i) + 1
				inodes = append(inodes, map[string]interface{}{
					"slot":     slotNum,
					"mode_str": di.ModeString(),
					"size":     di.Size(),
					"links":    di.Links,
				})
			}
		}
		resp["inodes"] = inodes
	}

	writeJSON(w, http.StatusOK, resp)
}

func blockTypeStr(addr uint32) string {
	switch {
	case addr == fstype.BootBlock:
		return "boot"
	case addr == fstype.SuperblockAddr:
		return "superblock"
	case addr == fstype.InodeBitmapAddr:
		return "inode_bitmap"
	case addr == fstype.BlockBitmapAddr:
		return "block_bitmap"
	case addr >= fstype.InodeTableStart && addr < fstype.InodeTableStart+fstype.InodeTableBlocks:
		return "inode_table"
	case addr >= fstype.JournalStart && addr < fstype.JournalStart+fstype.JournalBlocks:
		return "journal"
	default:
		return "data"
	}
}

// handleJournalInspect returns recent committed journal transactions.
func (s *Server) handleJournalInspect(w http.ResponseWriter, r *http.Request) {
	txns := s.fs.Journal().RecentTransactions(32)
	entries := make([]map[string]interface{}, 0, len(txns))
	for _, tx := range txns {
		entries = append(entries, map[string]interface{}{
			"id":         tx.ID,
			"status":     tx.Status,
			"blocks":     tx.Blocks,
			"timestamp":  tx.Timestamp,
			"operations": tx.Ops,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sequence": s.fs.Journal().CurrentSequence(),
		"entries":  entries,
	})
}

// handleTree returns a recursive directory tree.
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	depthStr := r.URL.Query().Get("depth")
	depth := 3
	if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
		depth = d
	}

	cred := ops.RootCredential
	node, err := buildTree(s.fs, cred, path, depth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

type treeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	FileType string      `json:"fileType"`
	Size     int64       `json:"size,omitempty"`
	Mode     string      `json:"mode"`
	Children []*treeNode `json:"children,omitempty"`
}

func buildTree(fs *fstype.Filesystem, cred ops.Credential, path string, depth int) (*treeNode, error) {
	si, err := ops.Stat(fs, cred, path)
	if err != nil {
		return nil, err
	}
	di, _ := fs.ReadInode(si.Inode)
	node := &treeNode{
		Name:     lastComponent(path),
		Path:     path,
		FileType: fileTypeString(si),
		Size:     si.Size,
	}
	if di != nil {
		node.Mode = di.ModeString()
	}

	if si.IsDir && depth > 0 {
		entries, err := ops.ReadDir(fs, cred, path)
		if err != nil {
			return node, nil
		}
		for _, e := range entries {
			if e.Name == "." || e.Name == ".." {
				continue
			}
			childPath := path
			if childPath != "/" {
				childPath += "/"
			}
			childPath += e.Name
			child, err := buildTree(fs, cred, childPath, depth-1)
			if err == nil {
				node.Children = append(node.Children, child)
			}
		}
	}
	return node, nil
}

func fileTypeString(si ops.StatInfo) string {
	switch {
	case si.IsDir:
		return "dir"
	case si.IsLink:
		return "symlink"
	case si.IsFile:
		return "file"
	default:
		return "unknown"
	}
}

func lastComponent(path string) string {
	if path == "/" {
		return "/"
	}
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return parts[len(parts)-1]
}

// handleFragmentation returns fragmentation analysis.
func (s *Server) handleFragmentation(w http.ResponseWriter, r *http.Request) {
	bm, err := fstype.ReadBlockBitmap(s.fs.Dev())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	type run struct {
		Start  uint32 `json:"start"`
		Length uint32 `json:"length"`
	}

	var freeRuns []run
	var largest uint32
	inRun := false
	var runStart, runLen uint32

	for i := uint32(fstype.DataStart); i < fstype.TotalBlocks; i++ {
		if !bm.IsSet(i) {
			if !inRun {
				inRun = true
				runStart = i
				runLen = 0
			}
			runLen++
		} else {
			if inRun {
				freeRuns = append(freeRuns, run{Start: runStart, Length: runLen})
				if runLen > largest {
					largest = runLen
				}
				inRun = false
			}
		}
	}
	if inRun {
		freeRuns = append(freeRuns, run{Start: runStart, Length: runLen})
		if runLen > largest {
			largest = runLen
		}
	}

	bmCount := uint32(bm.Count())
	var usedBlocks uint32
	if bmCount > fstype.DataStart {
		usedBlocks = bmCount - fstype.DataStart
	}
	fragPct := 0.0
	if len(freeRuns) > 1 {
		fragPct = float64(len(freeRuns)-1) / float64(len(freeRuns)) * 100
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"freeBlockRuns":    freeRuns,
		"largestFreeRun":   largest,
		"fragmentationPct": fragPct,
		"totalFreeRuns":    len(freeRuns),
		"usedDataBlocks":   usedBlocks,
	})
}

// handleFormat re-formats the filesystem.
func (s *Server) handleFormat(w http.ResponseWriter, r *http.Request) {
	if err := s.fs.Reformat(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true, Message: "filesystem reformatted"})
}

// handleFsck runs a filesystem consistency check.
func (s *Server) handleFsck(w http.ResponseWriter, r *http.Request) {
	result := s.fs.FsckFull()
	writeJSON(w, http.StatusOK, result)
}

// handleCrash simulates a crash by marking the filesystem dirty and forcing remount.
func (s *Server) handleCrash(w http.ResponseWriter, r *http.Request) {
	if err := s.fs.SimulateCrash(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true, Message: "crash simulated; filesystem recovered via journal replay"})
}
