// internal/realtime/websocket.go
package realtime

import "github.com/gofiber/websocket/v2"

// WebSocketConn wraps websocket.Conn (biar hub.go tidak perlu import websocket)
type WebSocketConn struct {
	Conn *websocket.Conn
}

func NewWebSocketConn(c *websocket.Conn) *WebSocketConn {
	return &WebSocketConn{Conn: c}
}
