package shell

import (
	"fmt"
	"strings"
	"sync"

	fstype "github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/ops"
)

// Shell is an interactive filesystem shell.
type Shell struct {
	mu   sync.Mutex
	fs   *fstype.Filesystem
	cred ops.Credential
}

// NewShell creates a new Shell session rooted at the filesystem.
func NewShell(fs *fstype.Filesystem) *Shell {
	return &Shell{
		fs: fs,
		cred: ops.Credential{
			UID: 0,
			GID: 0,
			CWD: 1,
		},
	}
}

// Execute parses and executes a shell command string.
// Returns the output as a string.
func (s *Shell) Execute(cmdLine string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmdLine = strings.TrimSpace(cmdLine)
	if cmdLine == "" {
		return ""
	}

	parts := splitArgs(cmdLine)
	if len(parts) == 0 {
		return ""
	}

	cmd := parts[0]
	args := parts[1:]

	handler, ok := commands[cmd]
	if !ok {
		return fmt.Sprintf("sh: %s: command not found\n", cmd)
	}

	result, err := handler(s, args)
	if err != nil {
		return fmt.Sprintf("sh: %s: %s\n", cmd, err.Error())
	}
	return result
}

// Cred returns the current credential.
func (s *Shell) Cred() ops.Credential {
	return s.cred
}

// SetCWD changes the current working directory.
func (s *Shell) SetCWD(inum uint32) {
	s.cred.CWD = inum
}

// FS returns the filesystem.
func (s *Shell) FS() *fstype.Filesystem {
	return s.fs
}

// splitArgs splits a command line into arguments, respecting quoted strings.
func splitArgs(line string) []string {
	var args []string
	var cur strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		c := line[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}
