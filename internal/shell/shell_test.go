package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/fs-engine/internal/disk"
	fstype "github.com/yourname/fs-engine/internal/fs"
)

func newTestShell(t *testing.T) *Shell {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shell_test.img")
	dev, err := disk.NewBlockDevice(path, fstype.DiskSize)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove(path)
	})
	fs, err := fstype.NewFormat(dev)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fs.Unmount()
		_ = dev.Close()
	})
	return NewShell(fs)
}

func TestShell_Echo(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("echo hello world")
	assert.Equal(t, "hello world\n", out)
}

func TestShell_Help(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("help")
	assert.Contains(t, out, "ls")
	assert.Contains(t, out, "mkdir")
}

func TestShell_Pwd(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("pwd")
	assert.Equal(t, "/\n", strings.TrimSpace(out)+"\n")
}

func TestShell_Ls(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("ls /")
	assert.Contains(t, out, "bin")
	assert.Contains(t, out, "etc")
}

func TestShell_Mkdir_Ls(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("mkdir /testdir")
	assert.Equal(t, "", out)

	out = sh.Execute("ls /")
	assert.Contains(t, out, "testdir")
}

func TestShell_Cd_Pwd(t *testing.T) {
	sh := newTestShell(t)
	sh.Execute("mkdir /mydir")
	out := sh.Execute("cd /mydir")
	assert.Equal(t, "", out)

	out = sh.Execute("pwd")
	assert.Contains(t, out, "mydir")
}

func TestShell_Touch_Cat(t *testing.T) {
	sh := newTestShell(t)
	sh.Execute("touch /testfile.txt")
	sh.Execute("write /testfile.txt hello content")
	out := sh.Execute("cat /testfile.txt")
	assert.Equal(t, "hello content", out)
}

func TestShell_Rm(t *testing.T) {
	sh := newTestShell(t)
	sh.Execute("touch /delme.txt")
	out := sh.Execute("rm /delme.txt")
	assert.Equal(t, "", out)

	out = sh.Execute("ls /")
	assert.NotContains(t, out, "delme.txt")
}

func TestShell_Rmdir(t *testing.T) {
	sh := newTestShell(t)
	sh.Execute("mkdir /emptydir")
	out := sh.Execute("rmdir /emptydir")
	assert.Equal(t, "", out)
}

func TestShell_UnknownCommand(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("xyzzy")
	assert.Contains(t, out, "command not found")
}

func TestShell_EmptyCommand(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("")
	assert.Equal(t, "", out)

	out = sh.Execute("   ")
	assert.Equal(t, "", out)
}

func TestShell_Stat(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("stat /bin")
	assert.Contains(t, out, "Inode")
}

func TestShell_Df(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("df")
	assert.Contains(t, out, "Total")
}

func TestShell_Find(t *testing.T) {
	sh := newTestShell(t)
	out := sh.Execute("find /")
	assert.Contains(t, out, "/bin")
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"ls /", []string{"ls", "/"}},
		{"echo hello world", []string{"echo", "hello", "world"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{"write /f 'some content'", []string{"write", "/f", "some content"}},
		{"  ls  /tmp  ", []string{"ls", "/tmp"}},
	}
	for _, tt := range tests {
		got := splitArgs(tt.input)
		assert.Equal(t, tt.expected, got, "input: %q", tt.input)
	}
}
