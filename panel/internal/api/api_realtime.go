package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func (s *Server) realtimeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return s.checkWSOrigin(r)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
		return nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Use a mutex to serialize all writes to the WebSocket connection.
	// gorilla/websocket supports only one concurrent writer.
	var wsMu sync.Mutex
	writeRealtime := func(kind string, data any) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteJSON(map[string]any{"type": kind, "time": time.Now().UTC(), "data": data})
	}
	writePing := func() error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
	}

	_ = writeRealtime("stats", s.dashboardStatsPayload())
	_ = writeRealtime("sessions", s.liveSessionsPayload())
	notifCh := s.addWSSubscriber()
	defer s.removeWSSubscriber(notifCh)
	ticker := time.NewTicker(3 * time.Second)
	pingTicker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	defer pingTicker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := writeRealtime("stats", s.dashboardStatsPayload()); err != nil {
				return
			}
			if err := writeRealtime("sessions", s.liveSessionsPayload()); err != nil {
				return
			}
			if err := writeRealtime("bandwidth", s.bandwidthPayload()); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := writePing(); err != nil {
				return
			}
		case notif := <-notifCh:
			// If the message already has a "type" field (e.g. node_metrics, node_status_change),
			// send it directly as a top-level message instead of wrapping in "notification".
			if msgType, ok := notif["type"].(string); ok && msgType != "" {
				wsMu.Lock()
				err := conn.WriteJSON(notif)
				wsMu.Unlock()
				if err != nil {
					return
				}
			} else {
				if err := writeRealtime("notification", notif); err != nil {
					return
				}
			}
		}
	}
}
