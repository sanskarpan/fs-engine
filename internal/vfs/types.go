package vfs

import (
	"io"
	"time"
)

// FileInfo represents metadata about a filesystem entry.
type FileInfo struct {
	Name    string
	Size    int64
	Mode    uint32
	ModTime time.Time
	IsDir_  bool
	Inode   uint32
	UID     uint32
	GID     uint32
	NLink   uint32
}

// IsDir returns true if the entry is a directory.
func (fi FileInfo) IsDir() bool {
	return fi.IsDir_
}

// FileSystem is the interface that filesystem implementations must satisfy.
type FileSystem interface {
	Open(path string) (File, error)
	Create(path string) (File, error)
	Mkdir(path string, mode uint32) error
	Remove(path string) error
	Stat(path string) (FileInfo, error)
	ReadDir(path string) ([]FileInfo, error)
}

// File is an open file handle.
type File interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
	Stat() (FileInfo, error)
	Sync() error
	Truncate(size int64) error
}
