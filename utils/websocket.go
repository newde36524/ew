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
	// SOCKS5 不需要在连接时读取首帧数据
	// 实际的请求数据会在连接建立后通过双向转发传输

	// 发送连接请求
	connectMsg := fmt.Sprintf("CONNECT:%s|%s", target, firstFrame)
	// 如果是 SOCKS5 模式，添加 base64: 前缀（即使首帧为空也添加前缀）
	if mode == ModeSOCKS5 {
		connectMsg = fmt.Sprintf("CONNECT:%s|base64:%s", target, firstFrame)
	}
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
