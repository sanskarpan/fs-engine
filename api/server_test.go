package api_test

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourname/fs-engine/api"
	"github.com/yourname/fs-engine/internal/disk"
	"github.com/yourname/fs-engine/internal/fs"
)

func reconcileSuperblockAPI(t *testing.T, filesystem *fs.Filesystem) {
	t.Helper()
	dev := filesystem.Dev()

	blockBM, err := fs.ReadBlockBitmap(dev)
	require.NoError(t, err)
	inodeBM, err := fs.ReadInodeBitmap(dev)
	require.NoError(t, err)

	sb := filesystem.Superblock()
	sb.FreeBlocks = uint32(fs.TotalBlocks) - uint32(blockBM.Count())
	sb.FreeInodes = uint32(fs.InodeCount) - uint32(inodeBM.Count())
	require.NoError(t, fs.WriteSuperblock(dev, sb))
}

// newTestServer creates a fresh filesystem and wraps it in an API server
// backed by an httptest.Server.
func newTestServer(t *testing.T) (*api.Server, *httptest.Server) {
	t.Helper()
	dev, err := disk.NewBlockDevice(t.TempDir()+"/disk.img", 64*1024*1024)
	require.NoError(t, err)
	filesystem, err := fs.NewFormat(dev)
	require.NoError(t, err)

	server := api.NewServer(filesystem)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		ts.Close()
		filesystem.Unmount()
		dev.Close()
	})
	return server, ts
}

// TestAPI_ErrorMapping_NotFound verifies missing paths return 404 rather than 500.
func TestAPI_ErrorMapping_NotFound(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doDelete(t, ts, "/fs/unlink?path=/tmp/missing.txt")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestAPI_ErrorMapping_Conflict verifies mkdir on an existing path returns 409.
func TestAPI_ErrorMapping_Conflict(t *testing.T) {
	_, ts := newTestServer(t)

	resp1 := doPost(t, ts, "/fs/mkdir", map[string]interface{}{"path": "/tmp/conflict", "mode": 493})
	require.Equal(t, http.StatusOK, resp1.StatusCode)
	resp1.Body.Close()

	resp2 := doPost(t, ts, "/fs/mkdir", map[string]interface{}{"path": "/tmp/conflict", "mode": 493})
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)
	resp2.Body.Close()
}

// TestAPI_ErrorMapping_BadRequest verifies invalid file reads return 400.
func TestAPI_ErrorMapping_BadRequest(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/fs/read?path=/tmp")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestAPI_OpenAPIContract(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/api/openapi.json")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)
	assert.Equal(t, "3.1.0", body["openapi"])

	paths, ok := body["paths"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, paths, "/fs/stat")
	assert.Contains(t, paths, "/fs/write")
	assert.Contains(t, paths, "/inspect/journal")
}

func TestAPI_SSESchema(t *testing.T) {
	_, ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/sse")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	require.True(t, scanner.Scan())
	line := strings.TrimPrefix(scanner.Text(), "data: ")
	var evt map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(line), &evt))
	assert.Equal(t, "connected", evt["type"])
	assert.Equal(t, float64(1), evt["version"])
	assert.Equal(t, "sse", evt["source"])
}

// doGet performs a GET request to the test server and returns the response.
func doGet(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	require.NoError(t, err)
	return resp
}

// doPost performs a POST request with a JSON body and returns the response.
func doPost(t *testing.T, ts *httptest.Server, path string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	return resp
}

// doDelete performs a DELETE request and returns the response.
func doDelete(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// decodeJSON decodes the response body into v.
func decodeResp(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

// TestAPI_Health verifies GET /health returns 200 with {"status":"ok"}.
func TestAPI_Health(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/health")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	decodeResp(t, resp, &body)
	assert.Equal(t, "ok", body["status"])
}

// TestAPI_Stat verifies GET /fs/stat?path=/ returns inode info for root.
func TestAPI_Stat(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/fs/stat?path=/")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	assert.Equal(t, float64(1), body["inode"], "root inode should be 1")
	assert.Equal(t, true, body["is_dir"], "root should be a directory")
}

// TestAPI_Stat_NotFound verifies GET /fs/stat?path=/nonexistent returns 404.
func TestAPI_Stat_NotFound(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/fs/stat?path=/nonexistent")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestAPI_ReadDir verifies GET /fs/ls?path=/ returns entries including seeded dirs.
func TestAPI_ReadDir(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/fs/ls?path=/")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Path    string `json:"path"`
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	decodeResp(t, resp, &body)

	names := make(map[string]bool)
	for _, e := range body.Entries {
		names[e.Name] = true
	}

	// The seeded filesystem always contains etc, home, tmp, bin.
	assert.True(t, names["etc"], "expected 'etc' in root listing")
	assert.True(t, names["home"], "expected 'home' in root listing")
	assert.True(t, names["tmp"], "expected 'tmp' in root listing")
}

// TestAPI_Mkdir verifies POST /fs/mkdir creates a directory.
func TestAPI_Mkdir(t *testing.T) {
	_, ts := newTestServer(t)

	body := map[string]interface{}{
		"path": "/api_test_dir",
		"mode": 493, // 0755 in decimal
	}
	resp := doPost(t, ts, "/fs/mkdir", body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify stat returns a directory.
	statResp := doGet(t, ts, "/fs/stat?path=/api_test_dir")
	assert.Equal(t, http.StatusOK, statResp.StatusCode)

	var statBody map[string]interface{}
	decodeResp(t, statResp, &statBody)
	assert.Equal(t, true, statBody["is_dir"])
}

// TestAPI_WriteRead creates a file via POST /fs/write, then reads it back via
// GET /fs/read and verifies the content matches.
func TestAPI_WriteRead(t *testing.T) {
	_, ts := newTestServer(t)

	const testContent = "hello API world"
	encoded := base64.StdEncoding.EncodeToString([]byte(testContent))

	writeBody := map[string]interface{}{
		"path":    "/tmp/api_test.txt",
		"offset":  0,
		"content": encoded,
	}
	writeResp := doPost(t, ts, "/fs/write", writeBody)
	assert.Equal(t, http.StatusOK, writeResp.StatusCode)
	writeResp.Body.Close()

	// Read it back.
	readResp := doGet(t, ts, "/fs/read?path=/tmp/api_test.txt")
	assert.Equal(t, http.StatusOK, readResp.StatusCode)

	var readBody struct {
		Content string `json:"content"`
		Length  int    `json:"length"`
	}
	decodeResp(t, readResp, &readBody)

	decoded, err := base64.StdEncoding.DecodeString(readBody.Content)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(decoded))
	assert.Equal(t, len(testContent), readBody.Length)
}

// TestAPI_Unlink creates a file, deletes it via DELETE /fs/unlink, and verifies
// that stat returns 404.
func TestAPI_Unlink(t *testing.T) {
	_, ts := newTestServer(t)

	// Create the file first.
	writeBody := map[string]interface{}{
		"path":    "/tmp/to_delete.txt",
		"offset":  0,
		"content": base64.StdEncoding.EncodeToString([]byte("delete me")),
	}
	writeResp := doPost(t, ts, "/fs/write", writeBody)
	require.Equal(t, http.StatusOK, writeResp.StatusCode)
	writeResp.Body.Close()

	// Delete it.
	delResp := doDelete(t, ts, "/fs/unlink?path=/tmp/to_delete.txt")
	assert.Equal(t, http.StatusOK, delResp.StatusCode)
	delResp.Body.Close()

	// Stat should now return 404.
	statResp := doGet(t, ts, "/fs/stat?path=/tmp/to_delete.txt")
	assert.Equal(t, http.StatusNotFound, statResp.StatusCode)
	statResp.Body.Close()
}

// TestAPI_Rename creates a file at path A, renames it to B, verifies stat(B)
// succeeds and stat(A) fails.
func TestAPI_Rename(t *testing.T) {
	_, ts := newTestServer(t)

	// Create file at A.
	writeBody := map[string]interface{}{
		"path":    "/tmp/rename_src.txt",
		"offset":  0,
		"content": base64.StdEncoding.EncodeToString([]byte("rename me")),
	}
	writeResp := doPost(t, ts, "/fs/write", writeBody)
	require.Equal(t, http.StatusOK, writeResp.StatusCode)
	writeResp.Body.Close()

	// Rename A → B.
	renameBody := map[string]interface{}{
		"old_path": "/tmp/rename_src.txt",
		"new_path": "/tmp/rename_dst.txt",
	}
	renameResp := doPost(t, ts, "/fs/rename", renameBody)
	assert.Equal(t, http.StatusOK, renameResp.StatusCode)
	renameResp.Body.Close()

	// Stat(B) should succeed.
	statB := doGet(t, ts, "/fs/stat?path=/tmp/rename_dst.txt")
	assert.Equal(t, http.StatusOK, statB.StatusCode)
	statB.Body.Close()

	// Stat(A) should fail.
	statA := doGet(t, ts, "/fs/stat?path=/tmp/rename_src.txt")
	assert.Equal(t, http.StatusNotFound, statA.StatusCode)
	statA.Body.Close()
}

// TestAPI_Symlink creates a symlink via POST /fs/symlink and reads it back via
// GET /fs/readlink.
func TestAPI_Symlink(t *testing.T) {
	_, ts := newTestServer(t)

	// Create a target file.
	writeBody := map[string]interface{}{
		"path":    "/tmp/symlink_target.txt",
		"offset":  0,
		"content": base64.StdEncoding.EncodeToString([]byte("target content")),
	}
	writeResp := doPost(t, ts, "/fs/write", writeBody)
	require.Equal(t, http.StatusOK, writeResp.StatusCode)
	writeResp.Body.Close()

	// Create symlink.
	symlinkBody := map[string]interface{}{
		"target": "/tmp/symlink_target.txt",
		"path":   "/tmp/symlink_link.txt",
	}
	symlinkResp := doPost(t, ts, "/fs/symlink", symlinkBody)
	assert.Equal(t, http.StatusOK, symlinkResp.StatusCode)
	symlinkResp.Body.Close()

	// Read the link target.
	readlinkResp := doGet(t, ts, "/fs/readlink?path=/tmp/symlink_link.txt")
	assert.Equal(t, http.StatusOK, readlinkResp.StatusCode)

	var body map[string]string
	decodeResp(t, readlinkResp, &body)
	assert.Equal(t, "/tmp/symlink_target.txt", body["target"])
}

// TestAPI_Chmod changes the mode of a file via POST /fs/chmod and verifies the
// updated mode via stat.
func TestAPI_Chmod(t *testing.T) {
	_, ts := newTestServer(t)

	// Create a file.
	writeBody := map[string]interface{}{
		"path":    "/tmp/chmod_test.txt",
		"offset":  0,
		"content": base64.StdEncoding.EncodeToString([]byte("chmod me")),
	}
	writeResp := doPost(t, ts, "/fs/write", writeBody)
	require.Equal(t, http.StatusOK, writeResp.StatusCode)
	writeResp.Body.Close()

	// Change mode to 0400.
	chmodBody := map[string]interface{}{
		"path": "/tmp/chmod_test.txt",
		"mode": 0o400,
	}
	chmodResp := doPost(t, ts, "/fs/chmod", chmodBody)
	assert.Equal(t, http.StatusOK, chmodResp.StatusCode)
	chmodResp.Body.Close()

	// Verify the updated mode contains the read-only bit.
	statResp := doGet(t, ts, "/fs/stat?path=/tmp/chmod_test.txt")
	assert.Equal(t, http.StatusOK, statResp.StatusCode)

	var statBody map[string]interface{}
	decodeResp(t, statResp, &statBody)
	// mode is uint16; the lower 12 bits should include 0o400 (256).
	mode := uint16(statBody["mode"].(float64))
	assert.Equal(t, uint16(0o400), mode&0o777, "expected mode 0400")
}

// TestAPI_InspectDisk verifies GET /inspect/disk returns a blockMap with
// TotalBlocks (16384) elements.
func TestAPI_InspectDisk(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/inspect/disk")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	blockMap, ok := body["blockMap"].([]interface{})
	require.True(t, ok, "expected blockMap array in response")
	assert.Equal(t, 16384, len(blockMap), "expected 16384 block entries")
}

// TestAPI_InspectInode verifies GET /inspect/inode/1 returns root inode fields.
func TestAPI_InspectInode(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/inspect/inode/1")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	assert.Equal(t, float64(1), body["inode"], "inode field should be 1")
	assert.Equal(t, true, body["is_dir"], "root inode should be a directory")
}

// TestAPI_InspectCache verifies GET /inspect/cache returns cache stats.
func TestAPI_InspectCache(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/inspect/cache")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	// The response should have the expected fields.
	_, hasHits := body["hits"]
	_, hasMisses := body["misses"]
	_, hasCapacity := body["capacity"]
	assert.True(t, hasHits, "expected 'hits' field in cache stats")
	assert.True(t, hasMisses, "expected 'misses' field in cache stats")
	assert.True(t, hasCapacity, "expected 'capacity' field in cache stats")
}

// TestAPI_Metrics verifies GET /metrics returns a counter map.
func TestAPI_Metrics(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/metrics")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Counters map[string]int64 `json:"counters"`
	}
	decodeResp(t, resp, &body)

	require.NotNil(t, body.Counters, "expected counters map in metrics response")
}

// TestAPI_Fsck verifies POST /fs/fsck returns a clean result on a freshly
// formatted filesystem. The superblock is reconciled with on-disk bitmaps in
// newTestServer before the server is created, so FsckFull sees consistent
// free block/inode counts.
func TestAPI_Fsck(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doPost(t, ts, "/fs/fsck", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Clean  bool     `json:"clean"`
		Issues []string `json:"issues"`
		Fixed  []string `json:"fixed"`
	}
	decodeResp(t, resp, &body)

	assert.True(t, body.Clean, "expected clean filesystem")
	assert.Empty(t, body.Issues, "expected no issues")
}

// TestAPI_ShellCommand verifies POST /shell with command "ls /" returns output
// containing "etc".
func TestAPI_ShellCommand(t *testing.T) {
	_, ts := newTestServer(t)

	body := map[string]string{"command": "ls /"}
	resp := doPost(t, ts, "/shell", body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var shellBody struct {
		Output string `json:"output"`
	}
	decodeResp(t, resp, &shellBody)

	assert.True(t, strings.Contains(shellBody.Output, "etc"),
		"expected 'etc' in shell ls output, got: %q", shellBody.Output)
}

// TestAPI_InspectTree verifies GET /inspect/tree?path=/&depth=1 returns a tree
// with children.
func TestAPI_InspectTree(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/inspect/tree?path=/&depth=1")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	assert.Equal(t, "/", body["path"], "expected root path in tree response")
	children, ok := body["children"].([]interface{})
	require.True(t, ok, "expected children array in tree response")
	assert.NotEmpty(t, children, "expected at least one child in root tree")
}

// TestAPI_InspectJournal verifies GET /inspect/journal returns an entries array.
func TestAPI_InspectJournal(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/inspect/journal")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	_, hasEntries := body["entries"]
	assert.True(t, hasEntries, "expected 'entries' field in journal response")
}

// TestAPI_Fragmentation verifies GET /inspect/fragmentation returns freeBlockRuns.
func TestAPI_Fragmentation(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doGet(t, ts, "/inspect/fragmentation")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	decodeResp(t, resp, &body)

	_, hasFreeBlockRuns := body["freeBlockRuns"]
	assert.True(t, hasFreeBlockRuns, "expected 'freeBlockRuns' field in fragmentation response")
}
