package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return allowedOrigin(r.Header.Get("Origin"), r)
	},
}

// handleShellCommand handles a single shell command via HTTP POST.
func (s *Server) handleShellCommand(w http.ResponseWriter, r *http.Request) {
	var req ShellRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	output := s.sh.Execute(req.Command)
	writeJSON(w, http.StatusOK, ShellResponse{Output: output})
}

// handleShellWebSocket handles an interactive shell session over WebSocket.
func (s *Server) handleShellWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Send welcome message
	welcome := ShellResponse{Output: "fs-engine shell. Type 'help' for commands.\n$ "}
	if data, err := json.Marshal(welcome); err == nil {
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		var req ShellRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			// Try treating the raw message as a command
			req.Command = string(msg)
		}

		output := s.sh.Execute(req.Command)
		resp := ShellResponse{Output: output + "$ "}
		data, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			break
		}
	}
}
