package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/ops"
)

// Credential used for all API requests (simplified: root).
var apiCred = ops.RootCredential

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, ErrorResponse{Error: err.Error(), Code: status})
}

func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func writeFSError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		writeError(w, http.StatusInternalServerError, errors.New("unknown filesystem error"))
	case errors.Is(err, fstype.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, fstype.ErrPermission):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, fstype.ErrExists), errors.Is(err, fstype.ErrNotEmpty):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, fstype.ErrNoSpace):
		writeError(w, http.StatusInsufficientStorage, err)
	case errors.Is(err, fstype.ErrInvalidArg),
		errors.Is(err, fstype.ErrIsDir),
		errors.Is(err, fstype.ErrNotDir),
		errors.Is(err, fstype.ErrNameTooLong),
		errors.Is(err, fstype.ErrLoop):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

// --- Stat ---

func (s *Server) handleStat(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}

	si, err := ops.Stat(s.fs, apiCred, path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	di, _ := s.fs.ReadInode(si.Inode)
	resp := StatResponse{
		Inode:  si.Inode,
		Mode:   si.Mode,
		UID:    si.UID,
		GID:    si.GID,
		Size:   si.Size,
		Blocks: si.Blocks,
		ATime:  si.ATime,
		MTime:  si.MTime,
		CTime:  si.CTime,
		Links:  si.Links,
		IsDir:  si.IsDir,
		IsLink: si.IsLink,
		IsFile: si.IsFile,
	}
	if di != nil {
		resp.ModeStr = di.ModeString()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLstat(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}

	si, err := ops.Lstat(s.fs, apiCred, path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	di, _ := s.fs.ReadInode(si.Inode)
	resp := StatResponse{
		Inode:  si.Inode,
		Mode:   si.Mode,
		UID:    si.UID,
		GID:    si.GID,
		Size:   si.Size,
		Blocks: si.Blocks,
		ATime:  si.ATime,
		MTime:  si.MTime,
		CTime:  si.CTime,
		Links:  si.Links,
		IsDir:  si.IsDir,
		IsLink: si.IsLink,
		IsFile: si.IsFile,
	}
	if di != nil {
		resp.ModeStr = di.ModeString()
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Directory ---

func (s *Server) handleReadDir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	entries, err := ops.ReadDir(s.fs, apiCred, path)
	if err != nil {
		writeFSError(w, err)
		return
	}

	resp := ReadDirResponse{
		Path:    path,
		Entries: make([]DirEntry, 0, len(entries)),
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, DirEntry{
			Name:     e.Name,
			Inode:    e.Inode,
			FileType: e.FileType,
			TypeName: fileTypeName(e.FileType),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var req MkdirRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Mode == 0 {
		req.Mode = 0755
	}
	if err := ops.Mkdir(s.fs, apiCred, req.Path, req.Mode); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleRmdir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}
	if err := ops.Rmdir(s.fs, apiCred, path); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// --- File ---

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}

	inum, err := ops.Resolve(s.fs, apiCred, path)
	if err != nil {
		writeFSError(w, err)
		return
	}

	di, err := s.fs.ReadInode(inum)
	if err != nil {
		writeFSError(w, err)
		return
	}

	size := di.Size()
	buf := make([]byte, size)
	n, err := ops.Read(s.fs, apiCred, inum, 0, buf)
	if err != nil {
		writeFSError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ReadResponse{
		Path:    path,
		Offset:  0,
		Length:  n,
		Content: base64.StdEncoding.EncodeToString(buf[:n]),
	})
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	var req WriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	inum, err := ops.Resolve(s.fs, apiCred, req.Path)
	if err != nil {
		// Try to create
		inum, err = ops.Create(s.fs, apiCred, req.Path, 0644)
		if err != nil {
			writeFSError(w, err)
			return
		}
	}

	// Decode content (try base64 first, fall back to raw)
	var data []byte
	data, err = base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		data = []byte(req.Content)
	}

	n, err := ops.Write(s.fs, apiCred, inum, req.Offset, data)
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, WriteResponse{Path: req.Path, Written: n})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Mode == 0 {
		req.Mode = 0644
	}
	inum, err := ops.Create(s.fs, apiCred, req.Path, req.Mode)
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "inode": inum})
}

func (s *Server) handleUnlink(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}
	if err := ops.Unlink(s.fs, apiCred, path); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleTruncate(w http.ResponseWriter, r *http.Request) {
	var req TruncateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	inum, err := ops.Resolve(s.fs, apiCred, req.Path)
	if err != nil {
		writeFSError(w, err)
		return
	}
	if err := ops.Truncate(s.fs, apiCred, inum, req.Size); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// --- Links ---

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	var req LinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := ops.Link(s.fs, apiCred, req.OldPath, req.NewPath); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleSymlink(w http.ResponseWriter, r *http.Request) {
	var req SymlinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := ops.Symlink(s.fs, apiCred, req.Target, req.Path); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleReadLink(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}
	target, err := ops.ReadLink(s.fs, apiCred, path)
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"target": target})
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	var req RenameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := ops.Rename(s.fs, apiCred, req.OldPath, req.NewPath); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// --- Attributes ---

func (s *Server) handleChmod(w http.ResponseWriter, r *http.Request) {
	var req ChmodRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := ops.Chmod(s.fs, apiCred, req.Path, req.Mode); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleChown(w http.ResponseWriter, r *http.Request) {
	var req ChownRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := ops.Chown(s.fs, apiCred, req.Path, req.UID, req.GID); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleUtimes(w http.ResponseWriter, r *http.Request) {
	var req UtimesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.ATime.IsZero() {
		req.ATime = time.Now()
	}
	if req.MTime.IsZero() {
		req.MTime = time.Now()
	}
	if err := ops.Utimes(s.fs, apiCred, req.Path, req.ATime, req.MTime); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// --- XAttrs ---

func (s *Server) handleGetXAttr(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	name := r.URL.Query().Get("name")
	if path == "" || name == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path or name"))
		return
	}
	val, err := ops.GetXAttr(s.fs, apiCred, path, name)
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, XAttrGetResponse{Path: path, Name: name, Value: string(val)})
}

func (s *Server) handleSetXAttr(w http.ResponseWriter, r *http.Request) {
	var req XAttrRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := ops.SetXAttr(s.fs, apiCred, req.Path, req.Name, []byte(req.Value), 0); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

func (s *Server) handleListXAttr(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path"))
		return
	}
	names, err := ops.ListXAttr(s.fs, apiCred, path)
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, XAttrListResponse{Path: path, Names: names})
}

func (s *Server) handleRemoveXAttr(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	name := r.URL.Query().Get("name")
	if path == "" || name == "" {
		writeError(w, http.StatusBadRequest, errMissingParam("path or name"))
		return
	}
	if err := ops.RemoveXAttr(s.fs, apiCred, path, name); err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// errMissingParam creates an error for a missing query parameter.
func errMissingParam(name string) error {
	return &paramError{name: name}
}

type paramError struct{ name string }

func (e *paramError) Error() string { return "missing required parameter: " + e.name }
