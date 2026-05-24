package vfs

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const maxOpenFiles = 1024

// OpenFile represents an open file descriptor.
type OpenFile struct {
	Inode  uint32
	Flags  int
	Offset int64
	mu     sync.Mutex
}

// Seek updates the file offset. size is the current file size (needed for SEEK_END).
func (f *OpenFile) SeekWithSize(offset int64, whence int, size int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var newOff int64
	switch whence {
	case 0: // SEEK_SET
		newOff = offset
	case 1: // SEEK_CUR
		newOff = f.Offset + offset
	case 2: // SEEK_END
		newOff = size + offset
	default:
		return 0, fmt.Errorf("vfs: invalid whence %d", whence)
	}
	if newOff < 0 {
		return 0, fmt.Errorf("vfs: negative offset")
	}
	f.Offset = newOff
	return newOff, nil
}

// FileDescriptorTable manages open file descriptors.
type FileDescriptorTable struct {
	mu    sync.Mutex
	files [maxOpenFiles]*OpenFile
	next  atomic.Int64
}

// NewFileDescriptorTable creates a new FileDescriptorTable.
func NewFileDescriptorTable() *FileDescriptorTable {
	return &FileDescriptorTable{}
}

// Open allocates a new file descriptor and returns its number.
func (t *FileDescriptorTable) Open(inum uint32, flags int) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := 0; i < maxOpenFiles; i++ {
		if t.files[i] == nil {
			t.files[i] = &OpenFile{
				Inode: inum,
				Flags: flags,
			}
			return i, nil
		}
	}
	return -1, fmt.Errorf("vfs: too many open files")
}

// Get returns the OpenFile for the given fd.
func (t *FileDescriptorTable) Get(fd int) (*OpenFile, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if fd < 0 || fd >= maxOpenFiles {
		return nil, fmt.Errorf("vfs: bad file descriptor %d", fd)
	}
	f := t.files[fd]
	if f == nil {
		return nil, fmt.Errorf("vfs: file descriptor %d not open", fd)
	}
	return f, nil
}

// Close closes the given file descriptor.
func (t *FileDescriptorTable) Close(fd int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if fd < 0 || fd >= maxOpenFiles {
		return fmt.Errorf("vfs: bad file descriptor %d", fd)
	}
	if t.files[fd] == nil {
		return fmt.Errorf("vfs: file descriptor %d not open", fd)
	}
	t.files[fd] = nil
	return nil
}
