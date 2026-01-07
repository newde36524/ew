package worker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/newde36524/ew/utils"

	"github.com/gorilla/websocket"
)

type ProxyServer struct {
	Config   *ProxyServerConfig
	IPLoader *IPLoader
	Ech      *Ech
}

type ProxyServerConfig struct {
	ListenAddr string
	ServerAddr string
	ServerIP   string
	Token      string
}

func NewProxyServer(config *ProxyServerConfig, ipLoader *IPLoader, ech *Ech) *ProxyServer {
	return &ProxyServer{
		Config:   config,
		IPLoader: ipLoader,
		Ech:      ech,
	}
}

func (p *ProxyServer) Run() error {
	log.Printf("[启动] 正在获取 ECH 配置...")
	if err := p.Ech.PrepareECH(); err != nil {
		log.Fatalf("[启动] 获取 ECH 配置失败: %v", err)
	}
	p.IPLoader.LoadWithRoutingMode()

	return p.runProxyServer()
}

func (p *ProxyServer) runProxyServer() error {
	listener, err := net.Listen("tcp", p.Config.ListenAddr)
	if err != nil {
		log.Fatalf("[代理] 监听失败: %v", err)
	}
	defer listener.Close() //nolint:errcheck

	log.Printf("[代理] 服务器启动: %s (支持 SOCKS5 和 HTTP)", p.Config.ListenAddr)

	//关闭控制台日志输出提升性能
	log.Default().SetOutput(io.Discard)

	log.Printf("[代理] 后端服务器: %s", p.Config.ServerAddr)
	if len(p.Config.ServerIP) != 0 {
		log.Printf("[代理] 使用固定 IP: %s", p.Config.ServerIP)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[代理] 接受连接失败: %v", err)
			continue
		}

		go p.handleConnection(conn)
	}
}

func (p *ProxyServer) handleConnection(conn net.Conn) {
	defer conn.Close() //nolint:errcheck

	clientAddr := conn.RemoteAddr().String()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	// 读取第一个字节判断协议
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	firstByte := buf[0]

	// 使用 switch 判断协议类型
	switch firstByte {
	case 0x05:
		// SOCKS5 协议
		p.handleSOCKS5(conn, clientAddr, firstByte)
	case 'C', 'G', 'P', 'H', 'D', 'O', 'T':
		// HTTP 协议 (CONNECT, GET, POST, HEAD, DELETE, OPTIONS, TRACE, PUT, PATCH)
		p.handleHTTP(conn, clientAddr, firstByte)
	default:
		log.Printf("[代理] %s 未知协议: 0x%02x", clientAddr, firstByte)
	}
}

func (p *ProxyServer) handleHTTP(conn net.Conn, clientAddr string, firstByte byte) {
	// 将第一个字节放回缓冲区
	reader := bufio.NewReader(io.MultiReader(
		strings.NewReader(string(firstByte)),
		conn,
	))

	// 读取 HTTP 请求行
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	parts := strings.Fields(requestLine)
	if len(parts) < 3 {
		return
	}

	method := parts[0]
	requestURL := parts[1]
	httpVersion := parts[2]

	// 读取所有 headers
	headers := make(map[string]string)
	var headerLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) == 0 {
			break
		}
		headerLines = append(headerLines, line)
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			headers[strings.ToLower(key)] = value
		}
	}

	switch method {
	case "CONNECT":
		// HTTPS 隧道代理 - 需要发送 200 响应
		log.Printf("[HTTP-CONNECT] %s -> %s", clientAddr, requestURL)
		if err := p.handleTunnel(conn, requestURL, clientAddr, utils.ModeHTTPConnect, ""); err != nil {
			if !utils.IsNormalCloseError(err) {
				log.Printf("[HTTP-CONNECT] %s 代理失败: %v", clientAddr, err)
			}
		}

	case "GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE":
		// HTTP 代理 - 直接转发，不发送 200 响应
		log.Printf("[HTTP-%s] %s -> %s", method, clientAddr, requestURL)

		var target string
		var path string

		if strings.HasPrefix(requestURL, "http://") {
			// 解析完整 URL
			urlWithoutScheme := strings.TrimPrefix(requestURL, "http://")
			idx := strings.Index(urlWithoutScheme, "/")
			if idx > 0 {
				target = urlWithoutScheme[:idx]
				path = urlWithoutScheme[idx:]
			} else {
				target = urlWithoutScheme
				path = "/"
			}
		} else {
			// 相对路径，从 Host header 获取
			target = headers["host"]
			path = requestURL
		}

		if len(target) == 0 {
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n")) //nolint:errcheck
			return
		}

		// 添加默认端口
		if !strings.Contains(target, ":") {
			target += ":80"
		}

		// 重构 HTTP 请求（去掉完整 URL，使用相对路径）
		var requestBuilder strings.Builder
		requestBuilder.WriteString(fmt.Sprintf("%s %s %s\r\n", method, path, httpVersion))

		// 写入 headers（过滤掉 Proxy-Connection）
		for _, line := range headerLines {
			key := strings.Split(line, ":")[0]
			keyLower := strings.ToLower(strings.TrimSpace(key))
			if keyLower != "proxy-connection" && keyLower != "proxy-authorization" {
				requestBuilder.WriteString(line)
				requestBuilder.WriteString("\r\n")
			}
		}
		requestBuilder.WriteString("\r\n")

		// 如果有请求体，需要读取并附加
		if contentLength := headers["content-length"]; len(contentLength) != 0 {
			var length int
			fmt.Sscanf(contentLength, "%d", &length) //nolint:errcheck
			if length > 0 && length < 10*1024*1024 { // 限制 10MB
				body := make([]byte, length)
				if _, err := io.ReadFull(reader, body); err == nil {
					requestBuilder.Write(body)
				}
			}
		}

		firstFrame := requestBuilder.String()

		// 使用 modeHTTPProxy 模式（不发送 200 响应）
		if err := p.handleTunnel(conn, target, clientAddr, utils.ModeHTTPProxy, firstFrame); err != nil {
			if !utils.IsNormalCloseError(err) {
				log.Printf("[HTTP-%s] %s 代理失败: %v", method, clientAddr, err)
			}
		}

	default:
		log.Printf("[HTTP] %s 不支持的方法: %s", clientAddr, method)
		conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n")) //nolint:errcheck
	}
}

func (p *ProxyServer) handleTunnel(conn net.Conn, target, clientAddr string, mode int, firstFrame string) error {
	// 解析目标地址
	targetHost, _, err := net.SplitHostPort(target)
	if err != nil {
		targetHost = target
	}

	// 检查是否应该绕过代理（直连）
	if p.IPLoader.ShouldBypassProxy(targetHost) {
		log.Printf("[分流] %s -> %s (直连，绕过代理)", clientAddr, target)
		return utils.HandleDirectConnection(conn, target, clientAddr, mode, firstFrame)
	}

	// 走代理
	log.Printf("[分流] %s -> %s (通过代理)", clientAddr, target)
	wsConn, err := p.dialWebSocketWithECH(2)
	if err != nil {
		utils.SendErrorResponse(conn, mode)
		return err
	}
	defer wsConn.Close() //nolint:errcheck

	var mu sync.Mutex

	// 保活
	stopPing := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				wsConn.WriteMessage(websocket.PingMessage, nil) //nolint:errcheck
				mu.Unlock()
			case <-stopPing:
				return
			}
		}
	}()
	defer close(stopPing)

	conn.SetDeadline(time.Time{}) //nolint:errcheck

	// 如果没有预设的 firstFrame，尝试读取第一帧数据（仅 SOCKS5）
	if len(firstFrame) == 0 && mode == utils.ModeSOCKS5 {
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		buffer := make([]byte, 32768)
		n, _ := conn.Read(buffer)
		_ = conn.SetReadDeadline(time.Time{})
		if n > 0 {
			firstFrame = string(buffer[:n])
		}
	}

	// 发送连接请求
	connectMsg := fmt.Sprintf("CONNECT:%s|%s", target, firstFrame)
	mu.Lock()
	err = wsConn.WriteMessage(websocket.TextMessage, []byte(connectMsg))
	mu.Unlock()
	if err != nil {
		utils.SendErrorResponse(conn, mode)
		return err
	}

	// 等待响应
	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		utils.SendErrorResponse(conn, mode)
		return err
	}

	response := string(msg)
	if strings.HasPrefix(response, "ERROR:") {
		utils.SendErrorResponse(conn, mode)
		return errors.New(response)
	}
	if response != "CONNECTED" {
		utils.SendErrorResponse(conn, mode)
		return fmt.Errorf("意外响应: %s", response)
	}

	// 发送成功响应（根据模式不同而不同）
	if err := utils.SendSuccessResponse(conn, mode); err != nil {
		return err
	}

	log.Printf("[代理] %s 已连接: %s", clientAddr, target)

	// 双向转发
	done := make(chan bool, 2)

	// Client -> Server
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				mu.Lock()
				wsConn.WriteMessage(websocket.TextMessage, []byte("CLOSE")) //nolint:errcheck
				mu.Unlock()
				done <- true
				return
			}

			mu.Lock()
			err = wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
			mu.Unlock()
			if err != nil {
				done <- true
				return
			}
		}
	}()

	// Server -> Client
	go func() {
		for {
			mt, msg, err := wsConn.ReadMessage()
			if err != nil {
				done <- true
				return
			}

			if mt == websocket.TextMessage {
				if len(msg) == 5 && string(msg) == "CLOSE" {
					done <- true
					return
				}
			}

			if _, err := conn.Write(msg); err != nil {
				done <- true
				return
			}
		}
	}()

	<-done
	log.Printf("[代理] %s 已断开: %s", clientAddr, target)
	return nil
}

// ======================== SOCKS5 处理 ========================

func (p *ProxyServer) handleSOCKS5(conn net.Conn, clientAddr string, firstByte byte) {
	// 验证版本
	if firstByte != 0x05 {
		log.Printf("[SOCKS5] %s 版本错误: 0x%02x", clientAddr, firstByte)
		return
	}

	// 读取认证方法数量
	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}

	nmethods := buf[0]
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	// 响应无需认证
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 读取请求
	buf = make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}

	if buf[0] != 5 {
		return
	}

	command := buf[1]
	atyp := buf[3]

	var host string
	switch atyp {
	case 0x01: // IPv4
		buf = make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()

	case 0x03: // 域名
		buf = make([]byte, 1)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		domainBuf := make([]byte, buf[0])
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return
		}
		host = string(domainBuf)

	case 0x04: // IPv6
		buf = make([]byte, 16)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()

	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:errcheck
		return
	}

	// 读取端口
	buf = make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])

	switch command {
	case 0x01: // CONNECT
		var target string
		if atyp == 0x04 {
			target = fmt.Sprintf("[%s]:%d", host, port)
		} else {
			target = fmt.Sprintf("%s:%d", host, port)
		}

		log.Printf("[SOCKS5] %s -> %s", clientAddr, target)

		if err := p.handleTunnel(conn, target, clientAddr, utils.ModeSOCKS5, ""); err != nil {
			if !utils.IsNormalCloseError(err) {
				log.Printf("[SOCKS5] %s 代理失败: %v", clientAddr, err)
			}
		}

	case 0x03: // UDP ASSOCIATE
		p.handleUDPAssociate(conn, clientAddr)

	default:
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:errcheck
		return
	}
}

func (p *ProxyServer) handleUDPAssociate(tcpConn net.Conn, clientAddr string) {
	// 创建 UDP 监听器
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		log.Printf("[UDP] %s 解析地址失败: %v", clientAddr, err)
		tcpConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:errcheck
		return
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Printf("[UDP] %s 监听失败: %v", clientAddr, err)
		tcpConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:errcheck
		return
	}

	// 获取实际监听的端口
	localAddr := udpConn.LocalAddr().(*net.UDPAddr)
	port := localAddr.Port

	log.Printf("[UDP] %s UDP ASSOCIATE 监听端口: %d", clientAddr, port)

	// 发送成功响应
	response := []byte{0x05, 0x00, 0x00, 0x01}
	response = append(response, 127, 0, 0, 1) // 127.0.0.1
	response = append(response, byte(port>>8), byte(port&0xff))

	if _, err := tcpConn.Write(response); err != nil {
		udpConn.Close() //nolint:errcheck
		return
	}

	// 启动 UDP 处理
	stopChan := make(chan struct{})
	go p.handleUDPRelay(udpConn, clientAddr, stopChan)

	// 保持 TCP 连接，直到客户端关闭
	buf := make([]byte, 1)
	tcpConn.Read(buf) //nolint:errcheck

	close(stopChan)
	udpConn.Close() //nolint:errcheck
	log.Printf("[UDP] %s UDP ASSOCIATE 连接关闭", clientAddr)
}

func (p *ProxyServer) handleUDPRelay(udpConn *net.UDPConn, clientAddr string, stopChan chan struct{}) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-stopChan:
			return
		default:
		}

		udpConn.SetReadDeadline(time.Now().Add(1 * time.Second)) //nolint:errcheck
		n, addr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		// 解析 SOCKS5 UDP 请求头
		if n < 10 {
			continue
		}

		// SOCKS5 UDP 请求格式:
		// +----+------+------+----------+----------+----------+
		// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
		// +----+------+------+----------+----------+----------+
		// | 2  |  1   |  1   | Variable |    2     | Variable |
		// +----+------+------+----------+----------+----------+

		data := buf[:n]

		if data[2] != 0x00 { // FRAG 必须为 0
			continue
		}

		atyp := data[3]
		var headerLen int
		var dstHost string
		var dstPort int

		switch atyp {
		case 0x01: // IPv4
			if n < 10 {
				continue
			}
			dstHost = net.IP(data[4:8]).String()
			dstPort = int(data[8])<<8 | int(data[9])
			headerLen = 10

		case 0x03: // 域名
			if n < 5 {
				continue
			}
			domainLen := int(data[4])
			if n < 7+domainLen {
				continue
			}
			dstHost = string(data[5 : 5+domainLen])
			dstPort = int(data[5+domainLen])<<8 | int(data[6+domainLen])
			headerLen = 7 + domainLen

		case 0x04: // IPv6
			if n < 22 {
				continue
			}
			dstHost = net.IP(data[4:20]).String()
			dstPort = int(data[20])<<8 | int(data[21])
			headerLen = 22

		default:
			continue
		}

		udpData := data[headerLen:]
		target := fmt.Sprintf("%s:%d", dstHost, dstPort)

		// 检查是否是 DNS 查询（端口 53）
		if dstPort == 53 {
			log.Printf("[UDP-DNS] %s -> %s (DoH 查询)", clientAddr, target)
			go p.handleDNSQuery(udpConn, addr, udpData, data[:headerLen])
		} else {
			log.Printf("[UDP] %s -> %s (暂不支持非 DNS UDP)", clientAddr, target)
			// 这里可以扩展支持其他 UDP 流量
		}
	}
}

func (p *ProxyServer) handleDNSQuery(udpConn *net.UDPConn, clientAddr *net.UDPAddr, dnsQuery []byte, socks5Header []byte) {
	// 通过 DoH 查询（使用重命名后的函数）
	dnsResponse, err := p.queryDoHForProxy(dnsQuery)
	if err != nil {
		log.Printf("[UDP-DNS] DoH 查询失败: %v", err)
		return
	}

	// 构建 SOCKS5 UDP 响应
	response := make([]byte, 0, len(socks5Header)+len(dnsResponse))
	response = append(response, socks5Header...)
	response = append(response, dnsResponse...)

	// 发送响应
	_, err = udpConn.WriteToUDP(response, clientAddr)
	if err != nil {
		log.Printf("[UDP-DNS] 发送响应失败: %v", err)
		return
	}

	log.Printf("[UDP-DNS] DoH 查询成功，响应 %d 字节", len(dnsResponse))
}

func (p *ProxyServer) dialWebSocketWithECH(maxRetries int) (*websocket.Conn, error) {
	host, port, path, err := utils.ParseServerAddr(p.Config.ServerAddr)
	if err != nil {
		return nil, err
	}

	wsURL := fmt.Sprintf("wss://%s:%s%s", host, port, path)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		echBytes, echErr := p.Ech.GetECHList()
		if echErr != nil {
			if attempt < maxRetries {
				p.Ech.RefreshECH() //nolint:errcheck
				continue
			}
			return nil, echErr
		}

		tlsCfg, tlsErr := p.Ech.BuildTLSConfigWithECH(p.Config.ServerAddr, echBytes)
		if tlsErr != nil {
			return nil, tlsErr
		}

		dialer := websocket.Dialer{
			TLSClientConfig: tlsCfg,
			Subprotocols: func() []string {
				if len(p.Config.Token) == 0 {
					return nil
				}
				return []string{p.Config.Token}
			}(),
			HandshakeTimeout: 10 * time.Second,
		}

		if len(p.Config.ServerIP) != 0 {
			dialer.NetDial = func(network, address string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				return net.DialTimeout(network, net.JoinHostPort(p.Config.ServerIP, port), 10*time.Second)
			}
		}

		wsConn, _, dialErr := dialer.Dial(wsURL, nil)
		if dialErr != nil {
			if strings.Contains(dialErr.Error(), "ECH") && attempt < maxRetries {
				log.Printf("[ECH] 连接失败，尝试刷新配置 (%d/%d)", attempt, maxRetries)
				p.Ech.RefreshECH() //nolint:errcheck
				time.Sleep(time.Second)
				continue
			}
			return nil, dialErr
		}

		return wsConn, nil
	}

	return nil, errors.New("连接失败，已达最大重试次数")
}

// queryDoHForProxy 通过 ECH 转发 DNS 查询到 Cloudflare DoH
func (p *ProxyServer) queryDoHForProxy(dnsQuery []byte) ([]byte, error) {
	_, port, _, err := utils.ParseServerAddr(p.Config.ServerAddr)
	if err != nil {
		return nil, err
	}

	// 构建 DoH URL
	dohURL := fmt.Sprintf("https://cloudflare-dns.com:%s/dns-query", port)

	tlsCfg, err := p.Ech.GetTlsCfg()
	if err != nil {
		return nil, fmt.Errorf("构建 TLS 配置失败: %w", err)
	}
	// 创建 HTTP 客户端
	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
		Proxy:           nil, // 显式设置为 nil 表示不使用任何代理
	}

	// 如果指定了 IP，使用自定义 Dialer
	if len(p.Config.ServerIP) != 0 {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			dialer := &net.Dialer{
				Timeout: 10 * time.Second,
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(p.Config.ServerIP, port))
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// 发送 DoH 请求
	req, err := http.NewRequest("POST", dohURL, bytes.NewReader(dnsQuery))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH 请求失败: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH 响应错误: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
