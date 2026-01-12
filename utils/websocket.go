package utils

import (
	"sync"

	"github.com/gorilla/websocket"
)

type WebSocketWrap struct {
	mu     sync.Mutex
	wsConn *websocket.Conn
}

func NewWebSocketWrap(wsConn *websocket.Conn) *WebSocketWrap {
	return &WebSocketWrap{
		wsConn: wsConn,
	}
}

func (w *WebSocketWrap) WriteMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wsConn.WriteMessage(messageType, data)
}

func (w *WebSocketWrap) ReadMessage() (messageType int, p []byte, err error) {
	return w.wsConn.ReadMessage()
}

func (w *WebSocketWrap) Close() error {
	return w.wsConn.Close()
}
