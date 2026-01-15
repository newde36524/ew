package utils

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketWrap struct {
	wsConn   *websocket.Conn
	stopPing chan struct{}
}

func NewWebSocketWrap(wsConn *websocket.Conn) *WebSocketWrap {
	return &WebSocketWrap{
		wsConn:   wsConn,
		stopPing: make(chan struct{}),
	}
}

func (w *WebSocketWrap) WriteMessage(messageType int, data []byte) error {
	return w.wsConn.WriteMessage(messageType, data)
}

func (w *WebSocketWrap) ReadMessage() (messageType int, p []byte, err error) {
	return w.wsConn.ReadMessage()
}

func (w *WebSocketWrap) Close() error {
	close(w.stopPing)
	return w.wsConn.Close()
}

func (w *WebSocketWrap) KeepAlive() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.WriteMessage(websocket.PingMessage, nil) //nolint:errcheck
		case <-w.stopPing:
			return
		}
	}
}

func (w *WebSocketWrap) Connenct(conn io.ReadWriter, target, firstFrame string, mode int) error {
	// 如果没有预设的 firstFrame，尝试读取第一帧数据（仅 SOCKS5）
	if mode == ModeSOCKS5 && len(firstFrame) == 0 {
		// _ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		buffer := make([]byte, 32*1024)
		n, _ := conn.Read(buffer)
		// _ = conn.SetReadDeadline(time.Time{})
		if n > 0 {
			firstFrame = string(buffer[:n])
		}
	}

	// 发送连接请求
	connectMsg := fmt.Sprintf("CONNECT:%s|%s", target, firstFrame)
	if err := w.WriteMessage(websocket.TextMessage, []byte(connectMsg)); err != nil {
		SendErrorResponse(conn, mode)
		return err
	}

	// 等待响应
	_, msg, err := w.ReadMessage()
	if err != nil {
		SendErrorResponse(conn, mode)
		return err
	}

	response := string(msg)
	if strings.HasPrefix(response, "ERROR:") {
		SendErrorResponse(conn, mode)
		return errors.New(response)
	}
	if response != "CONNECTED" {
		SendErrorResponse(conn, mode)
		return fmt.Errorf("意外响应: %s", response)
	}

	// 发送成功响应（根据模式不同而不同）
	return SendSuccessResponse(conn, mode)
}
