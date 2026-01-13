package worker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/newde36524/ew/utils"
	"github.com/newde36524/ew/utils/log"
)

type ProxyClient struct {
	Conn         io.ReadWriter
	wsConn       *utils.WebSocketWrap
	clientAddr   string
	clientConfig *ProxyClientConfig
	IPLoader     *IPLoader
	Ech          *Ech
	done         chan struct{}
}

func NewProxyClient(conn io.ReadWriter, clientAddr string, config *ProxyClientConfig, ipLoader *IPLoader, ech *Ech) *ProxyClient {
	return &ProxyClient{
		Conn:         conn,
		clientConfig: config,
		IPLoader:     ipLoader,
		Ech:          ech,
		clientAddr:   clientAddr,
		done:         make(chan struct{}),
	}
}

func (p *ProxyClient) ClientAddr() string {
	return p.clientAddr
}

func (p *ProxyClient) wait() {
	p.done <- struct{}{}
	close(p.done)
	p.wsConn.Close() //nolint:errcheck
}

func (p *ProxyClient) ReadFirstByte() byte {
	// p.Conn.SetDeadline(time.Now().Add(60 * time.Second))
	// defer p.Conn.SetDeadline(time.Time{})
	// 读取第一个字节判断协议
	buf := make([]byte, 1)
	n, err := p.Conn.Read(buf)
	if err != nil || n == 0 {
		return 0
	}
	return buf[0]
}

func (p *ProxyClient) handleHTTP(firstByte byte) {
	// 将第一个字节放回缓冲区
	reader := bufio.NewReader(io.MultiReader(
		bytes.NewReader([]byte{firstByte}),
		p.Conn,
	))
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if len(req.Host) == 0 {
		p.Conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n")) //nolint:errcheck
		return
	}
	delHeader := []string{
		"proxy-connection",
		"proxy-authorization",
		"user-agent",
	}
	for key := range req.Header {
		if slices.Contains(delHeader, strings.ToLower(key)) {
			req.Header.Del(key)
		}
	}

	method := req.Method
	requestURL := strings.TrimPrefix(req.URL.String(), "//")

	switch method {
	case "CONNECT":
		// HTTPS 隧道代理 - 需要发送 200 响应
		log.Printf("[HTTP-CONNECT] %s -> %s", p.clientAddr, requestURL)
		if err := p.handleTunnel(requestURL, utils.ModeHTTPConnect, ""); err != nil {
			if !utils.IsNormalCloseError(err) {
				log.Printf("[HTTP-CONNECT] %s 代理失败: %v", p.clientAddr, err)
			}
		}

	case "GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE":
		// HTTP 代理 - 直接转发，不发送 200 响应
		log.Printf("[HTTP-%s] %s -> %s", method, p.clientAddr, requestURL)

		target := req.Host
		// 添加默认端口
		if !strings.Contains(target, ":") {
			target += ":80"
		}

		buf := bytes.NewBuffer(nil)
		if err := req.Write(buf); err != nil {
			log.Printf("[HTTP-%s] %s 请求构造失败: %v", method, p.clientAddr, err)
			return
		}
		firstFrame := buf.String()

		// 使用 modeHTTPProxy 模式（不发送 200 响应）
		if err := p.handleTunnel(target, utils.ModeHTTPProxy, firstFrame); err != nil {
			if !utils.IsNormalCloseError(err) {
				log.Printf("[HTTP-%s] %s 代理失败: %v", method, p.clientAddr, err)
			}
		}

	default:
		log.Printf("[HTTP] %s 不支持的方法: %s", p.clientAddr, method)
		p.Conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n")) //nolint:errcheck
	}
}

func (p *ProxyClient) handleTunnel(target string, mode int, firstFrame string) error {
	// 解析目标地址
	targetHost, _, err := net.SplitHostPort(target)
	if err != nil {
		targetHost = target
	}

	// 检查是否应该绕过代理（直连）
	if p.IPLoader.ShouldBypassProxy(targetHost) {
		log.Printf("[分流] %s -> %s (直连，绕过代理)", p.clientAddr, target)
		return utils.HandleDirectConnection(p.Conn, target, p.clientAddr, mode, firstFrame)
	}

	// 走代理
	log.Printf("[分流] %s -> %s (通过代理)", p.clientAddr, target)

	if err := p.connenct(target, mode, firstFrame); err != nil {
		return err
	}

	log.Printf("[代理] %s 已连接: %s", p.clientAddr, target)

	// 双向转发
	// Client -> Server
	go p.clientToServer()

	// Server -> Client
	go p.serverToClient()

	p.wait()
	log.Printf("[代理] %s 已断开: %s", p.clientAddr, target)
	return nil
}

func (p *ProxyClient) connenct(target string, mode int, firstFrame string) error {
	wsConn, err := p.dialWebSocketWithECH(2)
	if err != nil {
		utils.SendErrorResponse(p.Conn, mode)
		return err
	}
	go wsConn.KeepAlive() // 保活
	p.wsConn = wsConn

	if err := wsConn.Connenct(p.Conn, target, firstFrame, mode); err != nil {
		return err
	}
	return nil
}

func (p *ProxyClient) clientToServer() error {
	defer func() {
		select {
		case <-p.done:
		default:
		}
	}()
	buf := make([]byte, 32*1024)
	for {
		n, err := p.Conn.Read(buf)
		if err != nil {
			p.wsConn.WriteMessage(websocket.TextMessage, []byte("CLOSE")) //nolint:errcheck
			return err
		}

		err = p.wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
		if err != nil {
			return err
		}
	}
}

func (p *ProxyClient) serverToClient() error {
	defer func() {
		select {
		case <-p.done:
		default:
		}
	}()
	for {
		mt, msg, err := p.wsConn.ReadMessage()
		if err != nil {
			return err
		}

		if mt == websocket.TextMessage {
			if len(msg) == 5 && string(msg) == "CLOSE" {
				return err
			}
		}
		if _, err := p.Conn.Write(msg); err != nil {
			return err
		}
	}
}

// ======================== SOCKS5 处理 ========================

func (p *ProxyClient) handleSOCKS5() {
	// 读取认证方法数量
	buf := make([]byte, 1)
	if _, err := io.ReadFull(p.Conn, buf); err != nil {
		return
	}

	nmethods := buf[0]
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(p.Conn, methods); err != nil {
		return
	}

	// 响应无需认证
	if _, err := p.Conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 读取请求
	buf = make([]byte, 4)
	if _, err := io.ReadFull(p.Conn, buf); err != nil {
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
		if _, err := io.ReadFull(p.Conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()

	case 0x03: // 域名
		buf = make([]byte, 1)
		if _, err := io.ReadFull(p.Conn, buf); err != nil {
			return
		}
		domainBuf := make([]byte, buf[0])
		if _, err := io.ReadFull(p.Conn, domainBuf); err != nil {
			return
		}
		host = string(domainBuf)

	case 0x04: // IPv6
		buf = make([]byte, 16)
		if _, err := io.ReadFull(p.Conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()

	default:
		p.Conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:errcheck
		return
	}

	// 读取端口
	buf = make([]byte, 2)
	if _, err := io.ReadFull(p.Conn, buf); err != nil {
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

		log.Printf("[SOCKS5] %s -> %s", p.clientAddr, target)

		if err := p.handleTunnel(target, utils.ModeSOCKS5, ""); err != nil {
			if !utils.IsNormalCloseError(err) {
				log.Printf("[SOCKS5] %s 代理失败: %v", p.clientAddr, err)
			}
		}

	case 0x03: // UDP ASSOCIATE
		p.handleUDPAssociate(p.Conn, p.clientAddr)

	default:
		p.Conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) //nolint:errcheck
		return
	}
}

func (p *ProxyClient) handleUDPAssociate(tcpConn io.ReadWriter, clientAddr string) {
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

func (p *ProxyClient) handleUDPRelay(udpConn *net.UDPConn, clientAddr string, stopChan chan struct{}) {
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

func (p *ProxyClient) handleDNSQuery(udpConn *net.UDPConn, clientAddr *net.UDPAddr, dnsQuery []byte, socks5Header []byte) {
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

func (p *ProxyClient) dialWebSocketWithECH(maxRetries int) (*utils.WebSocketWrap, error) {
	host, port, path, err := utils.ParseServerAddr(p.clientConfig.ServerAddr)
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

		tlsCfg, tlsErr := p.Ech.BuildTLSConfigWithECH(p.clientConfig.ServerAddr, echBytes)
		if tlsErr != nil {
			return nil, tlsErr
		}

		dialer := websocket.Dialer{
			TLSClientConfig: tlsCfg,
			Subprotocols: func() []string {
				if len(p.clientConfig.Token) == 0 {
					return nil
				}
				return []string{p.clientConfig.Token}
			}(),
			HandshakeTimeout: 10 * time.Second,
		}

		if len(p.clientConfig.ServerIP) != 0 {
			dialer.NetDial = func(network, address string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				return net.DialTimeout(network, net.JoinHostPort(p.clientConfig.ServerIP, port), 10*time.Second)
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

		return utils.NewWebSocketWrap(wsConn), nil
	}

	return nil, errors.New("连接失败，已达最大重试次数")
}

// queryDoHForProxy 通过 ECH 转发 DNS 查询到 Cloudflare DoH
func (p *ProxyClient) queryDoHForProxy(dnsQuery []byte) ([]byte, error) {
	_, port, _, err := utils.ParseServerAddr(p.clientConfig.ServerAddr)
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
	if len(p.clientConfig.ServerIP) != 0 {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			dialer := &net.Dialer{
				Timeout: 10 * time.Second,
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(p.clientConfig.ServerIP, port))
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
