package shell

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/ops"
)

// commandFunc is a shell command handler.
type commandFunc func(s *Shell, args []string) (string, error)

// commands maps command names to handlers.
var commands = map[string]commandFunc{
	"ls":       cmdLs,
	"ll":       cmdLl,
	"cd":       cmdCd,
	"pwd":      cmdPwd,
	"mkdir":    cmdMkdir,
	"rmdir":    cmdRmdir,
	"touch":    cmdTouch,
	"cat":      cmdCat,
	"write":    cmdWrite,
	"rm":       cmdRm,
	"mv":       cmdMv,
	"cp":       cmdCp,
	"ln":       cmdLn,
	"stat":     cmdStat,
	"chmod":    cmdChmod,
	"chown":    cmdChown,
	"df":       cmdDf,
	"help":     cmdHelp,
	"echo":     cmdEcho,
	"find":     cmdFind,
	"readlink": cmdReadlink,
	"hexdump":  cmdHexdump,
	"du":       cmdDu,
	"xattr":    cmdXattr,
	"journal":  cmdJournal,
	"fsck":     cmdFsck,
}

func resolvePath(s *Shell, path string) string {
	if path == "" {
		return ""
	}
	if path[0] == '/' {
		return path
	}
	// Build absolute path from CWD
	cwd, err := ops.GetCwd(s.fs, s.cred, s.cred.CWD)
	if err != nil {
		return "/" + path
	}
	if cwd == "/" {
		return "/" + path
	}
	return cwd + "/" + path
}

func cmdLs(s *Shell, args []string) (string, error) {
	path := "/"
	if len(args) > 0 {
		path = resolvePath(s, args[0])
	} else {
		var err error
		path, err = ops.GetCwd(s.fs, s.cred, s.cred.CWD)
		if err != nil {
			path = "/"
		}
	}

	entries, err := ops.ReadDir(s.fs, s.cred, path)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		sb.WriteString(e.Name)
		if e.FileType == fstype.FT_DIR {
			sb.WriteByte('/')
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func cmdLl(s *Shell, args []string) (string, error) {
	path := "/"
	if len(args) > 0 {
		path = resolvePath(s, args[0])
	} else {
		var err error
		path, err = ops.GetCwd(s.fs, s.cred, s.cred.CWD)
		if err != nil {
			path = "/"
		}
	}

	entries, err := ops.ReadDir(s.fs, s.cred, path)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, e := range entries {
		var entryPath string
		if path == "/" {
			entryPath = "/" + e.Name
		} else {
			entryPath = path + "/" + e.Name
		}
		si, serr := ops.Lstat(s.fs, s.cred, entryPath)
		if serr != nil {
			sb.WriteString(fmt.Sprintf("? %-20s\n", e.Name))
			continue
		}
		di, derr := s.fs.ReadInode(e.Inode)
		if derr != nil {
			continue
		}
		modeStr := di.ModeString()
		sb.WriteString(fmt.Sprintf("%s %4d %4d %4d %8d %s %s\n",
			modeStr, si.Links, si.UID, si.GID, si.Size,
			si.MTime.Format("Jan 02 15:04"), e.Name))
	}
	return sb.String(), nil
}

func cmdCd(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		s.cred.CWD = 1
		return "", nil
	}

	path := resolvePath(s, args[0])
	inum, err := ops.Resolve(s.fs, s.cred, path)
	if err != nil {
		return "", err
	}

	di, err := s.fs.ReadInode(inum)
	if err != nil {
		return "", err
	}
	if !di.IsDir() {
		return "", fstype.ErrNotDir
	}
	if err := ops.CheckDirAccess(di, s.cred); err != nil {
		return "", err
	}

	s.cred.CWD = inum
	return "", nil
}

func cmdPwd(s *Shell, args []string) (string, error) {
	cwd, err := ops.GetCwd(s.fs, s.cred, s.cred.CWD)
	if err != nil {
		return "", err
	}
	return cwd + "\n", nil
}

func cmdMkdir(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: mkdir <path>")
	}
	for _, arg := range args {
		path := resolvePath(s, arg)
		if err := ops.Mkdir(s.fs, s.cred, path, 0755); err != nil {
			return "", fmt.Errorf("%s: %w", arg, err)
		}
	}
	return "", nil
}

func cmdRmdir(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: rmdir <path>")
	}
	for _, arg := range args {
		path := resolvePath(s, arg)
		if err := ops.Rmdir(s.fs, s.cred, path); err != nil {
			return "", fmt.Errorf("%s: %w", arg, err)
		}
	}
	return "", nil
}

func cmdTouch(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: touch <path>")
	}
	for _, arg := range args {
		path := resolvePath(s, arg)
		// Try to update times; if not found, create
		si, err := ops.Lstat(s.fs, s.cred, path)
		if err == nil {
			now := time.Now()
			_ = ops.Utimes(s.fs, s.cred, path, now, now)
			_ = si
		} else {
			if _, err := ops.Create(s.fs, s.cred, path, 0644); err != nil {
				return "", fmt.Errorf("%s: %w", arg, err)
			}
		}
	}
	return "", nil
}

func cmdCat(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: cat <path>")
	}
	var sb strings.Builder
	for _, arg := range args {
		path := resolvePath(s, arg)
		inum, err := ops.Resolve(s.fs, s.cred, path)
		if err != nil {
			return "", fmt.Errorf("%s: %w", arg, err)
		}
		di, err := s.fs.ReadInode(inum)
		if err != nil {
			return "", err
		}
		buf := make([]byte, di.Size())
		n, err := ops.Read(s.fs, s.cred, inum, 0, buf)
		if err != nil {
			return "", fmt.Errorf("%s: %w", arg, err)
		}
		sb.Write(buf[:n])
	}
	return sb.String(), nil
}

func cmdWrite(s *Shell, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: write <path> <content>")
	}
	path := resolvePath(s, args[0])
	content := strings.Join(args[1:], " ")

	inum, err := ops.Resolve(s.fs, s.cred, path)
	if err != nil {
		// Create if not exists
		inum, err = ops.Create(s.fs, s.cred, path, 0644)
		if err != nil {
			return "", err
		}
	}

	if err := ops.Truncate(s.fs, s.cred, inum, 0); err != nil {
		return "", err
	}
	data := []byte(content)
	n, err := ops.Write(s.fs, s.cred, inum, 0, data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes\n", n), nil
}

func cmdRm(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: rm [-r] <path>")
	}
	recursive := false
	paths := args
	if args[0] == "-r" || args[0] == "-rf" {
		recursive = true
		paths = args[1:]
	}

	for _, arg := range paths {
		path := resolvePath(s, arg)
		si, err := ops.Lstat(s.fs, s.cred, path)
		if err != nil {
			return "", fmt.Errorf("%s: %w", arg, err)
		}
		if si.IsDir {
			if !recursive {
				return "", fmt.Errorf("%s: is a directory", arg)
			}
			if err := rmRecursive(s, path); err != nil {
				return "", fmt.Errorf("%s: %w", arg, err)
			}
		} else {
			if err := ops.Unlink(s.fs, s.cred, path); err != nil {
				return "", fmt.Errorf("%s: %w", arg, err)
			}
		}
	}
	return "", nil
}

func rmRecursive(s *Shell, path string) error {
	entries, err := ops.ReadDir(s.fs, s.cred, path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		childPath := path + "/" + e.Name
		if e.FileType == fstype.FT_DIR {
			if err := rmRecursive(s, childPath); err != nil {
				return err
			}
		} else {
			if err := ops.Unlink(s.fs, s.cred, childPath); err != nil {
				return err
			}
		}
	}
	return ops.Rmdir(s.fs, s.cred, path)
}

func cmdMv(s *Shell, args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("usage: mv <src> <dst>")
	}
	src := resolvePath(s, args[0])
	dst := resolvePath(s, args[1])
	return "", ops.Rename(s.fs, s.cred, src, dst)
}

func cmdCp(s *Shell, args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("usage: cp <src> <dst>")
	}
	src := resolvePath(s, args[0])
	dst := resolvePath(s, args[1])

	srcInum, err := ops.Resolve(s.fs, s.cred, src)
	if err != nil {
		return "", err
	}
	srcDI, err := s.fs.ReadInode(srcInum)
	if err != nil {
		return "", err
	}
	if srcDI.IsDir() {
		return "", fmt.Errorf("cp: omitting directory %s", src)
	}

	dstInum, err := ops.Create(s.fs, s.cred, dst, 0644)
	if err != nil {
		return "", err
	}
	if err := ops.Truncate(s.fs, s.cred, dstInum, 0); err != nil {
		return "", err
	}

	size := srcDI.Size()
	buf := make([]byte, size)
	n, err := ops.Read(s.fs, s.cred, srcInum, 0, buf)
	if err != nil {
		return "", err
	}
	_, err = ops.Write(s.fs, s.cred, dstInum, 0, buf[:n])
	return "", err
}

func cmdLn(s *Shell, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: ln [-s] <target> <link>")
	}
	if args[0] == "-s" {
		if len(args) != 3 {
			return "", fmt.Errorf("usage: ln -s <target> <link>")
		}
		target := args[1]
		linkPath := resolvePath(s, args[2])
		return "", ops.Symlink(s.fs, s.cred, target, linkPath)
	}
	// Hard link
	target := resolvePath(s, args[0])
	linkPath := resolvePath(s, args[1])
	return "", ops.Link(s.fs, s.cred, target, linkPath)
}

func cmdStat(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: stat <path>")
	}
	path := resolvePath(s, args[0])
	si, err := ops.Lstat(s.fs, s.cred, path)
	if err != nil {
		return "", err
	}
	di, err := s.fs.ReadInode(si.Inode)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("  File: %s\n  Inode: %d  Links: %d\n  Mode: %s (%04o)\n  UID: %d  GID: %d\n  Size: %d  Blocks: %d\n  ATime: %s\n  MTime: %s\n  CTime: %s\n",
		args[0], si.Inode, si.Links,
		di.ModeString(), si.Mode&0xFFF,
		si.UID, si.GID,
		si.Size, si.Blocks*512,
		si.ATime.Format(time.RFC3339),
		si.MTime.Format(time.RFC3339),
		si.CTime.Format(time.RFC3339),
	), nil
}

func cmdChmod(s *Shell, args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("usage: chmod <mode> <path>")
	}
	mode, err := strconv.ParseUint(args[0], 8, 32)
	if err != nil {
		return "", fmt.Errorf("invalid mode: %s", args[0])
	}
	path := resolvePath(s, args[1])
	return "", ops.Chmod(s.fs, s.cred, path, uint32(mode))
}

func cmdChown(s *Shell, args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("usage: chown <uid>[:<gid>] <path>")
	}
	path := resolvePath(s, args[1])
	parts := strings.SplitN(args[0], ":", 2)
	uid, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return "", fmt.Errorf("invalid uid: %s", parts[0])
	}
	gid := uint64(^uint32(0))
	if len(parts) == 2 {
		gid, err = strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return "", fmt.Errorf("invalid gid: %s", parts[1])
		}
	}
	return "", ops.Chown(s.fs, s.cred, path, uint32(uid), uint32(gid))
}

func cmdDf(s *Shell, args []string) (string, error) {
	total, used, free := s.fs.Df()
	return fmt.Sprintf("Filesystem      Total     Used     Free\n%-16s %-8d %-8d %-8d\n",
		"fs-engine", total/1024, used/1024, free/1024), nil
}

func cmdHelp(s *Shell, args []string) (string, error) {
	cmds := []string{
		"ls [path]       - list directory",
		"ll [path]       - long listing",
		"cd [path]       - change directory",
		"pwd             - print working directory",
		"mkdir <path>    - create directory",
		"rmdir <path>    - remove empty directory",
		"touch <path>    - create or update file",
		"cat <path>      - print file contents",
		"write <path> <content> - write content to file",
		"rm [-r] <path>  - remove file or directory",
		"mv <src> <dst>  - move/rename",
		"cp <src> <dst>  - copy file",
		"ln [-s] <target> <link> - create link",
		"stat <path>     - show file status",
		"chmod <mode> <path> - change permissions",
		"chown <uid>[:<gid>] <path> - change owner",
		"df              - show disk usage",
		"find <path> [name] - find files",
		"echo <text>     - print text",
		"help            - show this help",
	}
	return strings.Join(cmds, "\n") + "\n", nil
}

func cmdEcho(s *Shell, args []string) (string, error) {
	return strings.Join(args, " ") + "\n", nil
}

func cmdFind(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: find <path> [name]")
	}
	root := resolvePath(s, args[0])
	var nameFilter string
	if len(args) > 1 {
		nameFilter = args[1]
	}

	var sb strings.Builder
	var findRecursive func(path string) error
	findRecursive = func(path string) error {
		entries, err := ops.ReadDir(s.fs, s.cred, path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name == "." || e.Name == ".." {
				continue
			}
			var fullPath string
			if path == "/" {
				fullPath = "/" + e.Name
			} else {
				fullPath = path + "/" + e.Name
			}
			if nameFilter == "" || e.Name == nameFilter {
				sb.WriteString(fullPath + "\n")
			}
			if e.FileType == fstype.FT_DIR {
				_ = findRecursive(fullPath)
			}
		}
		return nil
	}

	if nameFilter == "" {
		sb.WriteString(root + "\n")
	}
	_ = findRecursive(root)
	return sb.String(), nil
}

func cmdReadlink(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: readlink <path>")
	}
	target, err := ops.ReadLink(s.fs, s.cred, resolvePath(s, args[0]))
	if err != nil {
		return "", err
	}
	return target + "\n", nil
}

func cmdHexdump(s *Shell, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: hexdump <file>")
	}
	path := resolvePath(s, args[0])
	si, err := ops.Stat(s.fs, s.cred, path)
	if err != nil {
		return "", err
	}
	size := si.Size
	if size > 512 {
		size = 512
	}
	buf := make([]byte, size)
	n, err := ops.Read(s.fs, s.cred, si.Inode, 0, buf)
	if err != nil {
		return "", err
	}
	buf = buf[:n]

	var sb strings.Builder
	for i := 0; i < len(buf); i += 16 {
		sb.WriteString(fmt.Sprintf("%08x  ", i))
		end := i + 16
		if end > len(buf) {
			end = len(buf)
		}
		for j := i; j < end; j++ {
			sb.WriteString(fmt.Sprintf("%02x ", buf[j]))
			if j == i+7 {
				sb.WriteByte(' ')
			}
		}
		// Pad if last row is short
		for j := end; j < i+16; j++ {
			sb.WriteString("   ")
			if j == i+7 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(" |")
		for j := i; j < end; j++ {
			c := buf[j]
			if c >= 32 && c < 127 {
				sb.WriteByte(c)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String(), nil
}

func cmdDu(s *Shell, args []string) (string, error) {
	path := "/"
	if len(args) > 0 {
		path = resolvePath(s, args[0])
	}

	var total int64
	var duRecursive func(p string) error
	duRecursive = func(p string) error {
		si, err := ops.Stat(s.fs, s.cred, p)
		if err != nil {
			return err
		}
		total += si.Size
		if si.IsDir {
			entries, err := ops.ReadDir(s.fs, s.cred, p)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Name == "." || e.Name == ".." {
					continue
				}
				childPath := p
				if childPath != "/" {
					childPath += "/"
				}
				childPath += e.Name
				_ = duRecursive(childPath)
			}
		}
		return nil
	}
	_ = duRecursive(path)

	// Report in KB (like du -sk)
	kb := (total + 1023) / 1024
	return fmt.Sprintf("%d\t%s\n", kb, path), nil
}

func cmdXattr(s *Shell, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: xattr get|set|list|rm <path> [name] [value]")
	}
	subcmd := args[0]
	path := resolvePath(s, args[1])

	switch subcmd {
	case "list":
		names, err := ops.ListXAttr(s.fs, s.cred, path)
		if err != nil {
			return "", err
		}
		return strings.Join(names, "\n") + "\n", nil
	case "get":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: xattr get <path> <name>")
		}
		val, err := ops.GetXAttr(s.fs, s.cred, path, args[2])
		if err != nil {
			return "", err
		}
		return string(val) + "\n", nil
	case "set":
		if len(args) < 4 {
			return "", fmt.Errorf("usage: xattr set <path> <name> <value>")
		}
		return "", ops.SetXAttr(s.fs, s.cred, path, args[2], []byte(args[3]), 0)
	case "rm":
		if len(args) < 3 {
			return "", fmt.Errorf("usage: xattr rm <path> <name>")
		}
		return "", ops.RemoveXAttr(s.fs, s.cred, path, args[2])
	default:
		return "", fmt.Errorf("xattr: unknown subcommand %q", subcmd)
	}
}

func cmdJournal(s *Shell, _ []string) (string, error) {
	txns := s.fs.Journal().RecentTransactions(10)
	if len(txns) == 0 {
		return "No journal transactions recorded.\n", nil
	}
	var sb strings.Builder
	for _, tx := range txns {
		sb.WriteString(fmt.Sprintf("txn #%d  status=%-12s  blocks=%v  time=%s\n",
			tx.ID, tx.Status, tx.Blocks,
			tx.Timestamp.Format("15:04:05")))
	}
	return sb.String(), nil
}

func cmdFsck(s *Shell, _ []string) (string, error) {
	result := s.fs.FsckFull()
	var sb strings.Builder
	if result.Clean {
		sb.WriteString("fsck: filesystem is clean\n")
	} else {
		sb.WriteString("fsck: issues found:\n")
		for _, issue := range result.Issues {
			sb.WriteString("  ERROR: " + issue + "\n")
		}
	}
	for _, fix := range result.Fixed {
		sb.WriteString("  FIXED: " + fix + "\n")
	}
	return sb.String(), nil
}
