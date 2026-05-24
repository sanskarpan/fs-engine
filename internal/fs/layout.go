package fs

import "errors"

const (
	BlockSize        = 4096
	DiskSize         = 64 * 1024 * 1024
	TotalBlocks      = 16384
	InodeCount       = 4096
	InodeSize        = 128
	PointersPerBlock = 1024

	BootBlock        = 0
	SuperblockAddr   = 1
	InodeBitmapAddr  = 2
	BlockBitmapAddr  = 3
	InodeTableStart  = 4
	InodeTableBlocks = 128
	JournalStart     = 132
	JournalBlocks    = 2048
	DataStart        = 2180

	SuperblockMagic   = 0xEF53
	SuperblockMagicV2 = 0x53EF
	StateClean        = 1
	StateError        = 2

	// File types
	S_IFMT   = 0xF000
	S_IFSOCK = 0xC000
	S_IFLNK  = 0xA000
	S_IFREG  = 0x8000
	S_IFBLK  = 0x6000
	S_IFDIR  = 0x4000
	S_IFCHR  = 0x2000
	S_IFIFO  = 0x1000

	S_ISUID = 0x0800
	S_ISGID = 0x0400
	S_ISVTX = 0x0200

	S_IRWXU = 0x01C0
	S_IRUSR = 0x0100
	S_IWUSR = 0x0080
	S_IXUSR = 0x0040
	S_IRWXG = 0x0038
	S_IRGRP = 0x0020
	S_IWGRP = 0x0010
	S_IXGRP = 0x0008
	S_IRWXO = 0x0007
	S_IROTH = 0x0004
	S_IWOTH = 0x0002
	S_IXOTH = 0x0001

	// Dir entry file types
	FT_UNKNOWN  = 0
	FT_REG_FILE = 1
	FT_DIR      = 2
	FT_CHRDEV   = 3
	FT_BLKDEV   = 4
	FT_FIFO     = 5
	FT_SOCK     = 6
	FT_SYMLINK  = 7
)

var (
	ErrNotFound     = errors.New("ENOENT: no such file or directory")
	ErrNotDir       = errors.New("ENOTDIR: not a directory")
	ErrIsDir        = errors.New("EISDIR: is a directory")
	ErrPermission   = errors.New("EACCES: permission denied")
	ErrExists       = errors.New("EEXIST: file exists")
	ErrNotEmpty     = errors.New("ENOTEMPTY: directory not empty")
	ErrLoop         = errors.New("ELOOP: too many levels of symbolic links")
	ErrNameTooLong  = errors.New("ENAMETOOLONG: file name too long")
	ErrNoSpace      = errors.New("ENOSPC: no space left on device")
	ErrInvalidArg   = errors.New("EINVAL: invalid argument")
	ErrIO           = errors.New("EIO: input/output error")
	ErrTooBig       = errors.New("EFBIG: file too large")
	ErrReadOnly     = errors.New("EROFS: read-only file system")
	ErrBadFS        = errors.New("EUCLEAN: filesystem is corrupt")
	ErrCrossDevice  = errors.New("EXDEV: invalid cross-device link")
	ErrTooManyOpen  = errors.New("EMFILE: too many open files")
	ErrNotSupported = errors.New("ENOTSUP: operation not supported")
)
