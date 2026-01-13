package worker

import (
	"github.com/newde36524/ew/utils/log"

	"net"
)

type ProxyServer struct {
	listenAddr   string
	clientConfig *ProxyClientConfig
	IPLoader     *IPLoader
	Ech          *Ech
}

type ProxyClientConfig struct {
	ServerAddr string
	ServerIP   string
	Token      string
}

func NewProxyServer(listenAddr string, clientConfig *ProxyClientConfig, ipLoader *IPLoader, ech *Ech) *ProxyServer {
	return &ProxyServer{
		listenAddr:   listenAddr,
		clientConfig: clientConfig,
		IPLoader:     ipLoader,
		Ech:          ech,
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
	listener, err := net.Listen("tcp", p.listenAddr)
	if err != nil {
		log.Fatalf("[代理] 监听失败: %v", err)
	}
	defer listener.Close() //nolint:errcheck

	log.Printf("[代理] 服务器启动: %s (支持 SOCKS5 和 HTTP)", p.listenAddr)

	// //关闭控制台日志输出提升性能
	// log.Default().SetOutput(io.Discard)

	log.Printf("[代理] 后端服务器: %s", p.clientConfig.ServerAddr)
	if len(p.clientConfig.ServerIP) != 0 {
		log.Printf("[代理] 使用固定 IP: %s", p.clientConfig.ServerIP)
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

	proxyClient := NewProxyClient(conn, conn.RemoteAddr().String(), p.clientConfig, p.IPLoader, p.Ech)

	// 使用 switch 判断协议类型
	firstByte := proxyClient.ReadFirstByte()
	switch firstByte {
	case 0x05:
		// SOCKS5 协议
		proxyClient.handleSOCKS5()
	case 'C', 'G', 'P', 'H', 'D', 'O', 'T':
		// HTTP 协议 (CONNECT, GET, POST, HEAD, DELETE, OPTIONS, TRACE, PUT, PATCH)
		proxyClient.handleHTTP(firstByte)
	default:
		log.Printf("[代理] %s 未知协议: 0x%02x", proxyClient.ClientAddr(), firstByte)
	}
}
