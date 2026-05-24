package api

import (
	"net/http"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

const apiContractVersion = "v1"

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":   "fs-engine API",
			"version": apiContractVersion,
		},
		"servers": []map[string]string{
			{"url": "/api"},
		},
		"paths": map[string]interface{}{
			"/metrics": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Metrics snapshot",
				},
			},
			"/fs/stat": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Stat a filesystem path",
					"parameters": []map[string]string{
						{"name": "path", "in": "query", "required": "true"},
					},
				},
			},
			"/fs/ls": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List a directory",
				},
			},
			"/fs/read": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Read a file",
				},
			},
			"/fs/write": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Write file content",
				},
			},
			"/fs/rename": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Rename or move a path",
				},
			},
			"/fs/crash": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Simulate crash and replay journal",
				},
			},
			"/inspect/disk": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Disk layout for visualizer",
				},
			},
			"/inspect/inode/{num}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Inspect inode details",
				},
			},
			"/inspect/journal": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Recent journal transactions",
				},
			},
		},
		"components": map[string]interface{}{
			"x-fs-engine": map[string]interface{}{
				"sseVersion": fstype.EventSchemaVersion,
				"sseEventShape": map[string]string{
					"version": "integer",
					"type":    "string",
					"source":  "string",
					"path":    "string",
					"ts":      "RFC3339 timestamp",
				},
			},
		},
	})
}
