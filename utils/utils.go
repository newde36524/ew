// nolint: errcheck
package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	http.DefaultClient.Timeout = 30 * time.Second
	if transport, ok := http.DefaultClient.Transport.(*http.Transport); ok {
		transport.Proxy = nil
	}
}

// IpToUint32 将IP地址转换为uint32
func IpToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// CompareIPv6 比较两个IPv6地址，返回 -1, 0, 或 1
func CompareIPv6(a, b [16]byte) int {
	for i := 0; i < 16; i++ {
		if a[i] < b[i] {
			return -1
		} else if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func IsNormalCloseError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "normal closure")
}

func ParseServerAddr(addr string) (host, port, path string, err error) {
	path = "/"
	slashIdx := strings.Index(addr, "/")
	if slashIdx != -1 {
		path = addr[slashIdx:]
		addr = addr[:slashIdx]
	}

	host, port, err = net.SplitHostPort(addr)
	if err != nil {
		return "", "", "", fmt.Errorf("无效的服务器地址格式: %v", err)
	}

	return host, port, path, nil
}

func getHostByAddr(addr string) (host string, err error) {
	u, err := url.Parse(addr)
	return u.Host, err
}

// ======================== 响应辅助函数 ========================

func SendErrorResponse(conn net.Conn, mode int) {
	switch mode {
	case ModeSOCKS5:
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	case ModeHTTPConnect, ModeHTTPProxy:
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
	}
}

func SendSuccessResponse(conn net.Conn, mode int) error {
	switch mode {
	case ModeSOCKS5:
		// SOCKS5 成功响应
		_, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	case ModeHTTPConnect:
		// HTTP CONNECT 需要发送 200 响应
		_, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		return err
	case ModeHTTPProxy:
		// HTTP GET/POST 等不需要发送响应，直接转发目标服务器的响应
		return nil
	}
	return nil
}

// HandleDirectConnection 处理直连（绕过代理）
func HandleDirectConnection(conn net.Conn, target, clientAddr string, mode int, firstFrame string) error {
	// 解析目标地址
	_, _, err := net.SplitHostPort(target)
	if err != nil {
		// 如果没有端口，根据模式添加默认端口
		var port string
		if mode == ModeHTTPConnect || mode == ModeHTTPProxy {
			port = "443"
		} else {
			port = "80"
		}
		target = net.JoinHostPort(target, port)
	}

	// 直接连接到目标
	targetConn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		SendErrorResponse(conn, mode)
		return fmt.Errorf("直连失败: %w", err)
	}
	defer targetConn.Close()

	// 发送成功响应
	if err := SendSuccessResponse(conn, mode); err != nil {
		return err
	}

	// 如果有预设的第一帧数据，先发送
	if len(firstFrame) != 0 {
		if _, err := targetConn.Write([]byte(firstFrame)); err != nil {
			return err
		}
	}

	// 双向转发
	done := make(chan bool, 2)

	// Client -> Target
	go func() {
		io.Copy(targetConn, conn)
		done <- true
	}()

	// Target -> Client
	go func() {
		io.Copy(conn, targetConn)
		done <- true
	}()

	<-done
	log.Printf("[分流] %s 直连已断开: %s", clientAddr, target)
	return nil
}

func GetDataByUrl(url string, header map[string]string) (*http.Response, error) {
	client := http.Client{}
	cf, err := BuildTLSConfig()
	if err != nil {
		return nil, err
	}
	client.Transport = &http.Transport{
		Proxy:           nil,
		TLSClientConfig: cf,
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)

	for k, v := range header {
		req.Header.Set(k, v)
	}
	// resp, err := http.DefaultClient.Do(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	return resp, nil
}

func BuildTLSConfigWithECH(addr string) (*tls.Config, error) {
	host, _, _, err := ParseServerAddr(addr)
	if err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("加载系统根证书失败: %w", err)
	}
	config := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ServerName: host,
		RootCAs:    roots,
	}
	return config, nil
}

func BuildTLSConfig() (*tls.Config, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("加载系统根证书失败: %w", err)
	}
	config := &tls.Config{
		RootCAs: roots,
	}
	return config, nil
}
