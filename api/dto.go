package api

import "time"

// --- Request types ---

// ReadRequest is the request body for a file read operation.
type ReadRequest struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
}

// WriteRequest is the request body for a file write operation.
type WriteRequest struct {
	Path    string `json:"path"`
	Offset  int64  `json:"offset"`
	Content string `json:"content"` // base64-encoded or plaintext
}

// MkdirRequest creates a directory.
type MkdirRequest struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
}

// CreateRequest creates a file.
type CreateRequest struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
}

// LinkRequest creates a hard link.
type LinkRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

// SymlinkRequest creates a symbolic link.
type SymlinkRequest struct {
	Target string `json:"target"`
	Path   string `json:"path"`
}

// RenameRequest renames/moves a file.
type RenameRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

// ChmodRequest changes file permissions.
type ChmodRequest struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
}

// ChownRequest changes file ownership.
type ChownRequest struct {
	Path string `json:"path"`
	UID  uint32 `json:"uid"`
	GID  uint32 `json:"gid"`
}

// UtimesRequest updates file timestamps.
type UtimesRequest struct {
	Path  string    `json:"path"`
	ATime time.Time `json:"atime"`
	MTime time.Time `json:"mtime"`
}

// TruncateRequest truncates a file.
type TruncateRequest struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// XAttrRequest sets an extended attribute.
type XAttrRequest struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ShellRequest is a shell command request.
type ShellRequest struct {
	Command string `json:"command"`
}

// --- Response types ---

// StatResponse is the response for stat operations.
type StatResponse struct {
	Inode   uint32    `json:"inode"`
	Mode    uint16    `json:"mode"`
	ModeStr string    `json:"mode_str"`
	UID     uint16    `json:"uid"`
	GID     uint16    `json:"gid"`
	Size    int64     `json:"size"`
	Blocks  uint32    `json:"blocks"`
	ATime   time.Time `json:"atime"`
	MTime   time.Time `json:"mtime"`
	CTime   time.Time `json:"ctime"`
	Links   uint16    `json:"links"`
	IsDir   bool      `json:"is_dir"`
	IsLink  bool      `json:"is_link"`
	IsFile  bool      `json:"is_file"`
}

// DirEntry is a directory entry in a listing.
type DirEntry struct {
	Name     string `json:"name"`
	Inode    uint32 `json:"inode"`
	FileType uint8  `json:"file_type"`
	TypeName string `json:"type_name"`
}

// ReadDirResponse is the response for a directory listing.
type ReadDirResponse struct {
	Path    string     `json:"path"`
	Entries []DirEntry `json:"entries"`
}

// ReadResponse is the response for a file read.
type ReadResponse struct {
	Path    string `json:"path"`
	Offset  int64  `json:"offset"`
	Length  int    `json:"length"`
	Content string `json:"content"` // base64 or text
}

// WriteResponse is the response for a file write.
type WriteResponse struct {
	Path    string `json:"path"`
	Written int    `json:"written"`
}

// DfResponse is the response for a disk usage query.
type DfResponse struct {
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	TotalMB    uint64 `json:"total_mb"`
	UsedMB     uint64 `json:"used_mb"`
	FreeMB     uint64 `json:"free_mb"`
}

// MetricsResponse holds all metrics.
type MetricsResponse struct {
	Counters map[string]int64     `json:"counters"`
	Window   []WindowBucketDTO    `json:"window,omitempty"`
	HitRate  float64              `json:"hitRate"`
}

// WindowBucketDTO is one second of per-counter deltas for the stats page.
type WindowBucketDTO struct {
	TS              int64 `json:"ts"`
	Reads           int64 `json:"reads"`
	Writes          int64 `json:"writes"`
	CacheHits       int64 `json:"cache_hits"`
	CacheMiss       int64 `json:"cache_miss"`
	JournalCommits  int64 `json:"journal_commits"`
	BlocksAllocated int64 `json:"blocks_allocated"`
	BlocksFreed     int64 `json:"blocks_freed"`
	InodesAllocated int64 `json:"inodes_allocated"`
	InodesFreed     int64 `json:"inodes_freed"`
}

// SuperblockResponse holds superblock information.
type SuperblockResponse struct {
	Magic       uint32 `json:"magic"`
	InodeCount  uint32 `json:"inode_count"`
	BlockCount  uint32 `json:"block_count"`
	FreeInodes  uint32 `json:"free_inodes"`
	FreeBlocks  uint32 `json:"free_blocks"`
	BlockSize   uint32 `json:"block_size"`
	InodeSize   uint16 `json:"inode_size"`
	State       uint16 `json:"state"`
	MountCount  uint16 `json:"mount_count"`
	Version     uint32 `json:"version"`
	VolumeName  string `json:"volume_name"`
}

// CacheStatsResponse holds buffer cache statistics.
type CacheStatsResponse struct {
	Hits      uint64 `json:"hits"`
	Misses    uint64 `json:"misses"`
	Evictions uint64 `json:"evictions"`
	Size      int    `json:"size"`
	Capacity  int    `json:"capacity"`
	HitRate   float64 `json:"hit_rate"`
}

// ErrorResponse is returned on error.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// OKResponse is returned on success with no data.
type OKResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// ShellResponse is returned from a shell command.
type ShellResponse struct {
	Output string `json:"output"`
}

// XAttrListResponse lists extended attributes.
type XAttrListResponse struct {
	Path  string   `json:"path"`
	Names []string `json:"names"`
}

// XAttrGetResponse returns an extended attribute value.
type XAttrGetResponse struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// fileTypeName returns a human-readable file type name.
func fileTypeName(ft uint8) string {
	switch ft {
	case 1:
		return "file"
	case 2:
		return "dir"
	case 3:
		return "chrdev"
	case 4:
		return "blkdev"
	case 5:
		return "fifo"
	case 6:
		return "socket"
	case 7:
		return "symlink"
	default:
		return "unknown"
	}
}
