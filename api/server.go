package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	fstype "github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/shell"
)

// Server holds all dependencies for the API server.
type Server struct {
	fs     *fstype.Filesystem
	sh     *shell.Shell
	sseBus *SSEBus
	router *chi.Mux
}

// NewServer creates a new API server.
func NewServer(fs *fstype.Filesystem) *Server {
	sh := shell.NewShell(fs)
	sseBus := NewSSEBus(fs.Events)

	s := &Server{
		fs:     fs,
		sh:     sh,
		sseBus: sseBus,
	}

	s.router = s.buildRouter()
	return s
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) buildRouter() *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(RecoveryMiddleware)
	r.Use(LoggingMiddleware)
	r.Use(CORSMiddleware)
	r.Use(CompressUnlessUpgrading(5))

	// Health check (no prefix)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/openapi.json", s.handleOpenAPI)

	// SSE events — frontend proxies /sse → localhost:8080/sse
	r.Get("/sse", s.sseBus.ServeHTTP)
	r.Get("/sse/events", s.sseBus.ServeHTTP)

	// WebSocket shell — frontend connects directly to ws://localhost:8080/ws/shell
	r.Get("/ws/shell", s.handleShellWebSocket)

	// All REST routes under /api (vite proxies /api → localhost:8080/api)
	r.Route("/api", func(r chi.Router) {
		// Metrics
		r.Get("/metrics", s.handleMetrics)
		r.Get("/openapi.json", s.handleOpenAPI)

		// Filesystem operations
		r.Route("/fs", func(r chi.Router) {
			// Stat
			r.Get("/stat", s.handleStat)
			r.Get("/lstat", s.handleLstat)

			// Directory operations
			r.Get("/ls", s.handleReadDir)
			r.Post("/mkdir", s.handleMkdir)
			r.Delete("/rmdir", s.handleRmdir)

			// File operations
			r.Get("/read", s.handleRead)
			r.Post("/write", s.handleWrite)
			r.Post("/create", s.handleCreate)
			r.Delete("/unlink", s.handleUnlink)
			r.Post("/truncate", s.handleTruncate)

			// Link operations
			r.Post("/link", s.handleLink)
			r.Post("/symlink", s.handleSymlink)
			r.Get("/readlink", s.handleReadLink)
			r.Post("/rename", s.handleRename)

			// Attribute operations
			r.Post("/chmod", s.handleChmod)
			r.Post("/chown", s.handleChown)
			r.Post("/utimes", s.handleUtimes)

			// Extended attributes
			r.Get("/xattr", s.handleGetXAttr)
			r.Post("/xattr", s.handleSetXAttr)
			r.Get("/xattr/list", s.handleListXAttr)
			r.Delete("/xattr", s.handleRemoveXAttr)

			// Admin ops
			r.Post("/format", s.handleFormat)
			r.Post("/fsck", s.handleFsck)
			r.Post("/crash", s.handleCrash)
		})

		// Inspection endpoints
		r.Route("/inspect", func(r chi.Router) {
			r.Get("/disk", s.handleDiskLayout)
			r.Get("/superblock", s.handleSuperblock)
			r.Get("/inode/{num}", s.handleInspectInode)
			r.Get("/block/{addr}", s.handleBlockDetail)
			r.Get("/bitmap/blocks", s.handleBlockBitmap)
			r.Get("/bitmap/inodes", s.handleInodeBitmap)
			r.Get("/cache", s.handleCacheStats)
			r.Get("/journal", s.handleJournalInspect)
			r.Get("/tree", s.handleTree)
			r.Get("/fragmentation", s.handleFragmentation)
			r.Get("/df", s.handleDf)
		})

		// Shell (REST fallback)
		r.Post("/shell", s.handleShellCommand)
	})

	// Backward-compat: old routes without /api prefix (for tests and direct curl)
	r.Route("/fs", func(r chi.Router) {
		r.Get("/stat", s.handleStat)
		r.Get("/lstat", s.handleLstat)
		r.Get("/ls", s.handleReadDir)
		r.Post("/mkdir", s.handleMkdir)
		r.Delete("/rmdir", s.handleRmdir)
		r.Get("/read", s.handleRead)
		r.Post("/write", s.handleWrite)
		r.Post("/create", s.handleCreate)
		r.Delete("/unlink", s.handleUnlink)
		r.Post("/truncate", s.handleTruncate)
		r.Post("/link", s.handleLink)
		r.Post("/symlink", s.handleSymlink)
		r.Get("/readlink", s.handleReadLink)
		r.Post("/rename", s.handleRename)
		r.Post("/chmod", s.handleChmod)
		r.Post("/chown", s.handleChown)
		r.Post("/utimes", s.handleUtimes)
		r.Get("/xattr", s.handleGetXAttr)
		r.Post("/xattr", s.handleSetXAttr)
		r.Get("/xattr/list", s.handleListXAttr)
		r.Delete("/xattr", s.handleRemoveXAttr)
		r.Post("/format", s.handleFormat)
		r.Post("/fsck", s.handleFsck)
	})
	r.Route("/inspect", func(r chi.Router) {
		r.Get("/disk", s.handleDiskLayout)
		r.Get("/superblock", s.handleSuperblock)
		r.Get("/inode/{num}", s.handleInspectInode)
		r.Get("/block/{addr}", s.handleBlockDetail)
		r.Get("/bitmap/blocks", s.handleBlockBitmap)
		r.Get("/bitmap/inodes", s.handleInodeBitmap)
		r.Get("/cache", s.handleCacheStats)
		r.Get("/journal", s.handleJournalInspect)
		r.Get("/tree", s.handleTree)
		r.Get("/fragmentation", s.handleFragmentation)
		r.Get("/df", s.handleDf)
		r.Get("/metrics", s.handleMetrics)
	})
	r.Post("/shell", s.handleShellCommand)
	r.Get("/shell/ws", s.handleShellWebSocket)
	r.Get("/metrics", s.handleMetrics)
	r.Get("/openapi.json", s.handleOpenAPI)

	return r
}
