//go:build darwin
// +build darwin

package utils

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
)

// GetProxyBypassList 获取代理绕过列表
func GetProxyBypassList(routingMode string) []string {
	// 基础绕过列表（本地和内网）
	// 注意：分流功能已在 Go 程序中实现，系统代理设置为全局代理
	// Go 程序会根据分流模式自动决定哪些流量走代理，哪些直连
	baseBypass := []string{
		"localhost", "127.*", "10.*", "172.16.*", "172.17.*", "172.18.*",
		"172.19.*", "172.20.*", "172.21.*", "172.22.*", "172.23.*", "172.24.*",
		"172.25.*", "172.26.*", "172.27.*", "172.28.*", "172.29.*", "172.30.*",
		"172.31.*", "192.168.*", "*.local", "169.254.*",
	}
	return baseBypass
}

// ProxyState 保存代理状态
type ProxyState struct {
	Enabled     bool
	ProxyServer string
	BypassList  []string
}

var (
	originalState *ProxyState
	stateMutex    sync.Mutex
	proxyModified bool
)

// SetSystemProxy 设置 macOS 系统代理
func SetSystemProxy(enabled bool, listenAddr, routingMode string) error {
	// 解析监听地址
	host := "127.0.0.1"
	port := listenAddr
	if strings.Contains(listenAddr, ":") {
		parts := strings.Split(listenAddr, ":")
		if len(parts) == 2 {
			host = parts[0]
			port = parts[1]
		}
	}

	// 获取所有网络服务
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取网络服务列表失败: %v", err)
	}

	// 解析网络服务列表（跳过第一行说明）
	lines := strings.Split(string(output), "\n")
	var services []string
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, strings.TrimSpace(line))
	}

	// 获取绕过列表
	bypassList := GetProxyBypassList(routingMode)

	// 对每个网络服务设置代理
	for _, service := range services {
		if enabled {
			// 设置 SOCKS 代理
			cmd := exec.Command("networksetup", "-setsocksfirewallproxy", service, host, port)
			if err := cmd.Run(); err != nil {
				log.Printf("[系统] 设置 %s 的 SOCKS 代理失败: %v\n", service, err)
				continue
			}

			// 设置绕过列表
			args := []string{"-setsocksfirewallproxybypassdomains", service}
			args = append(args, bypassList...)
			cmd = exec.Command("networksetup", args...)
			if err := cmd.Run(); err != nil {
				log.Printf("[系统] 设置 %s 的绕过列表失败: %v\n", service, err)
				continue
			}

			// 启用 SOCKS 代理
			cmd = exec.Command("networksetup", "-setsocksfirewallproxystate", service, "on")
			if err := cmd.Run(); err != nil {
				log.Printf("[系统] 启用 %s 的 SOCKS 代理失败: %v\n", service, err)
				continue
			}
		} else {
			// 关闭 SOCKS 代理
			cmd := exec.Command("networksetup", "-setsocksfirewallproxystate", service, "off")
			if err := cmd.Run(); err != nil {
				log.Printf("[系统] 关闭 %s 的 SOCKS 代理失败: %v\n", service, err)
				continue
			}
		}
	}

	if enabled {
		log.Printf("[系统] macOS 代理已设置: %s:%s, 分流模式: %s\n", host, port, routingMode)
	} else {
		log.Println("[系统] macOS 代理已关闭")
	}

	// 标记代理已修改
	stateMutex.Lock()
	proxyModified = enabled
	stateMutex.Unlock()

	return nil
}

// getCurrentProxyState 获取当前代理状态
func getCurrentProxyState(service string) (*ProxyState, error) {
	state := &ProxyState{}

	// 获取 SOCKS 代理状态
	cmd := exec.Command("networksetup", "-getsocksfirewallproxy", service)
	output, err := cmd.CombinedOutput()
	if err == nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "Enabled: Yes") {
			state.Enabled = true
			// 解析服务器和端口
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Server:") {
					state.ProxyServer = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
				}
			}
		}
	}

	// 获取绕过列表
	cmd = exec.Command("networksetup", "-getsocksfirewallproxybypassdomains", service)
	output, err = cmd.CombinedOutput()
	if err == nil {
		bypassList := strings.Fields(string(output))
		state.BypassList = bypassList
	}

	return state, nil
}

// SaveProxyState 保存当前代理状态
func SaveProxyState() error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	// 获取所有网络服务
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取网络服务列表失败: %v", err)
	}

	// 解析网络服务列表（跳过第一行说明）
	lines := strings.Split(string(output), "\n")
	var services []string
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, strings.TrimSpace(line))
	}

	if len(services) == 0 {
		return fmt.Errorf("未找到网络服务")
	}

	// 使用第一个网络服务的状态
	state, err := getCurrentProxyState(services[0])
	if err != nil {
		return err
	}

	originalState = state
	log.Printf("[系统] 已保存当前代理状态: enabled=%v, server=%s\n", state.Enabled, state.ProxyServer)
	return nil
}

// RestoreProxyState 恢复代理状态
func RestoreProxyState() error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	if originalState == nil {
		log.Println("[系统] 无需恢复代理状态（未修改过）")
		return nil
	}

	// 获取所有网络服务
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取网络服务列表失败: %v", err)
	}

	// 解析网络服务列表（跳过第一行说明）
	lines := strings.Split(string(output), "\n")
	var services []string
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, strings.TrimSpace(line))
	}

	// 对每个网络服务恢复代理
	for _, service := range services {
		if originalState.Enabled && originalState.ProxyServer != "" {
			// 解析服务器地址
			parts := strings.Split(originalState.ProxyServer, ":")
			if len(parts) == 2 {
				host := parts[0]
				port := parts[1]

				// 设置 SOCKS 代理
				cmd := exec.Command("networksetup", "-setsocksfirewallproxy", service, host, port)
				if err := cmd.Run(); err != nil {
					log.Printf("[系统] 恢复 %s 的 SOCKS 代理失败: %v\n", service, err)
					continue
				}

				// 设置绕过列表
				if len(originalState.BypassList) > 0 {
					args := []string{"-setsocksfirewallproxybypassdomains", service}
					args = append(args, originalState.BypassList...)
					cmd = exec.Command("networksetup", args...)
					if err := cmd.Run(); err != nil {
						log.Printf("[系统] 恢复 %s 的绕过列表失败: %v\n", service, err)
					}
				}

				// 启用 SOCKS 代理
				cmd = exec.Command("networksetup", "-setsocksfirewallproxystate", service, "on")
				if err := cmd.Run(); err != nil {
					log.Printf("[系统] 启用 %s 的 SOCKS 代理失败: %v\n", service, err)
				}
			}
		} else {
			// 关闭 SOCKS 代理
			cmd := exec.Command("networksetup", "-setsocksfirewallproxystate", service, "off")
			if err := cmd.Run(); err != nil {
				log.Printf("[系统] 关闭 %s 的 SOCKS 代理失败: %v\n", service, err)
			}
		}
	}

	log.Printf("[系统] 已恢复代理状态: enabled=%v, server=%s\n", originalState.Enabled, originalState.ProxyServer)
	return nil
}
