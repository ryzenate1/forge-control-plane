//go:build !windows

package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

// handleHostTerminalWS is not implemented on non-Windows builds yet.
// It returns a JSON error so the panel UI can display a clear message
// instead of hanging or reporting false success.
func (s *Server) handleHostTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	msg, _ := json.Marshal(map[string]string{
		"type":  "error",
		"error": "host terminal is not implemented on this platform yet",
	})
	_ = conn.WriteMessage(websocket.TextMessage, msg)
}
