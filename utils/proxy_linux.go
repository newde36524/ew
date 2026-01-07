//go:build linux
// +build linux

// nolint: errcheck
package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// GetProxyBypassList 获取代理绕过列表
func GetProxyBypassList() string {
	// 基础绕过列表（本地和内网）
	// 注意：分流功能已在 Go 程序中实现，系统代理设置为全局代理
	// Go 程序会根据分流模式自动决定哪些流量走代理，哪些直连
	baseBypass := "localhost,127.*,10.*,172.16.*,172.17.*,172.18.*,172.19.*,172.20.*,172.21.*,172.22.*,172.23.*,172.24.*,172.25.*,172.26.*,172.27.*,172.28.*,172.29.*,172.30.*,172.31.*,192.168.*"
	return baseBypass
}

// ProxyState 保存代理状态
type ProxyState struct {
	Enabled     bool
	ProxyServer string
	BypassList  string
}

var (
	originalState *ProxyState
	stateMutex    sync.Mutex
)

// SetSystemProxy 设置 Linux 系统代理
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

	// 检测桌面环境
	desktopEnv := detectDesktopEnvironment()

	switch desktopEnv {
	case "gnome", "gnome-wayland", "gnome-xorg":
		return setGnomeProxy(enabled, host, port, routingMode)
	case "kde":
		return setKDEProxy(enabled, host, port, routingMode)
	case "xfce":
		return setXFCEProxy(enabled, host, port, routingMode)
	default:
		// 尝试通用的环境变量方式
		return setEnvProxy(enabled, host, port, routingMode)
	}
}

// detectDesktopEnvironment 检测桌面环境
func detectDesktopEnvironment() string {
	// 检查 XDG_CURRENT_DESKTOP 环境变量
	if xdgDesktop := os.Getenv("XDG_CURRENT_DESKTOP"); len(xdgDesktop) != 0 {
		return strings.ToLower(strings.TrimSpace(xdgDesktop))
	}

	// 检查 GNOME
	if isProcessRunning("gnome-shell") || isProcessRunning("gnome-session") {
		return "gnome"
	}

	// 检查 KDE
	if isProcessRunning("ksmserver") || isProcessRunning("plasmashell") {
		return "kde"
	}

	// 检查 XFCE
	if isProcessRunning("xfce4-session") || isProcessRunning("xfconfd") {
		return "xfce"
	}

	return "unknown"
}

// isProcessRunning 检查进程是否运行
func isProcessRunning(name string) bool {
	cmd := exec.Command("pgrep", "-x", name)
	err := cmd.Run()
	return err == nil
}

// setGnomeProxy 设置 GNOME 代理
func setGnomeProxy(enabled bool, host, port, routingMode string) error {
	if enabled {
		// 启用 SOCKS 代理
		cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 GNOME 代理模式失败: %v\n", err)
			return err
		}

		// 设置 SOCKS 代理地址
		cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", host)
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 GNOME SOCKS 主机失败: %v\n", err)
			return err
		}

		cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", port)
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 GNOME SOCKS 端口失败: %v\n", err)
			return err
		}

		// 设置绕过列表
		bypassList := GetProxyBypassList()
		cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", bypassList)
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 GNOME 绕过列表失败: %v\n", err)
			return err
		}

		log.Printf("[系统] GNOME 代理已设置: %s:%s, 分流模式: %s\n", host, port, routingMode)
	} else {
		// 关闭代理
		cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none")
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 关闭 GNOME 代理失败: %v\n", err)
			return err
		}
		log.Println("[系统] GNOME 代理已关闭")
	}

	return nil
}

// setKDEProxy 设置 KDE 代理
func setKDEProxy(enabled bool, host, port, routingMode string) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config")
	kioslaverc := filepath.Join(configDir, "kioslaverc")

	if enabled {
		// 创建或更新 kioslaverc 配置文件
		content := fmt.Sprintf(`[Proxy Settings]
ProxyType=1
SOCKSProxy=%s %s
SOCKSVersion=5
NoProxyForList=%s
ReversedException=false
`, host, port, GetProxyBypassList())

		if err := os.WriteFile(kioslaverc, []byte(content), 0644); err != nil {
			log.Printf("[系统] 设置 KDE 代理失败: %v\n", err)
			return err
		}

		// 通知 KDE 刷新配置
		cmd := exec.Command("qdbus", "org.kde.kded5", "/modules/proxy", "org.kde.KProxyProxyManager.proxyChanged")
		_ = cmd.Run() // 忽略错误，某些 KDE 版本可能不支持

		log.Printf("[系统] KDE 代理已设置: %s:%s, 分流模式: %s\n", host, port, routingMode)
	} else {
		// 关闭代理
		content := `[Proxy Settings]
ProxyType=0
`
		if err := os.WriteFile(kioslaverc, []byte(content), 0644); err != nil {
			log.Printf("[系统] 关闭 KDE 代理失败: %v\n", err)
			return err
		}

		cmd := exec.Command("qdbus", "org.kde.kded5", "/modules/proxy", "org.kde.KProxyProxyManager.proxyChanged")
		_ = cmd.Run()

		log.Println("[系统] KDE 代理已关闭")
	}

	return nil
}

// setXFCEProxy 设置 XFCE 代理
func setXFCEProxy(enabled bool, host, port, routingMode string) error {
	if enabled {
		// 启用 SOCKS 代理
		cmd := exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/mode", "-s", "manual")
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 XFCE 代理模式失败: %v\n", err)
			return err
		}

		// 设置 SOCKS 代理地址
		cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/socks/host", "-s", host)
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 XFCE SOCKS 主机失败: %v\n", err)
			return err
		}

		cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/socks/port", "-s", port)
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 XFCE SOCKS 端口失败: %v\n", err)
			return err
		}

		// 设置绕过列表
		bypassList := GetProxyBypassList()
		cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/no-proxy", "-s", bypassList)
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 设置 XFCE 绕过列表失败: %v\n", err)
			return err
		}

		log.Printf("[系统] XFCE 代理已设置: %s:%s, 分流模式: %s\n", host, port, routingMode)
	} else {
		// 关闭代理
		cmd := exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/mode", "-s", "none")
		if err := cmd.Run(); err != nil {
			log.Printf("[系统] 关闭 XFCE 代理失败: %v\n", err)
			return err
		}
		log.Println("[系统] XFCE 代理已关闭")
	}

	return nil
}

// setEnvProxy 设置环境变量代理（通用方式）
func setEnvProxy(enabled bool, host, port, routingMode string) error {
	if enabled {
		log.Printf("[系统] Linux 环境变量代理提示: %s:%s, 分流模式: %s\n", host, port, routingMode)
		log.Println("[系统] 请手动设置环境变量:")
		log.Printf("  export http_proxy=socks5://%s:%s\n", host, port)
		log.Printf("  export https_proxy=socks5://%s:%s\n", host, port)
		log.Printf("  export all_proxy=socks5://%s:%s\n", host, port)
		log.Printf("  export no_proxy=%s\n", GetProxyBypassList())
	} else {
		log.Println("[系统] 请手动清除环境变量:")
		log.Println("  unset http_proxy https_proxy all_proxy no_proxy")
	}
	return nil
}

// getCurrentProxyState 获取当前代理状态
func getCurrentProxyState() (*ProxyState, error) {
	state := &ProxyState{}

	// 检测桌面环境
	desktopEnv := detectDesktopEnvironment()

	switch desktopEnv {
	case "gnome", "gnome-wayland", "gnome-xorg":
		// 读取 GNOME 代理设置
		cmd := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode")
		output, err := cmd.CombinedOutput()
		if err == nil {
			mode := strings.TrimSpace(string(output))
			if strings.Contains(mode, "manual") {
				state.Enabled = true

				// 读取 SOCKS 代理
				cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.socks", "host")
				output, _ = cmd.CombinedOutput()
				host := strings.TrimSpace(strings.Trim(string(output), "'"))

				cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.socks", "port")
				output, _ = cmd.CombinedOutput()
				port := strings.TrimSpace(strings.Trim(string(output), "'"))

				if len(host) != 0 && len(port) != 0 {
					state.ProxyServer = host + ":" + port
				}

				// 读取绕过列表
				cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy", "ignore-hosts")
				output, _ = cmd.CombinedOutput()
				state.BypassList = strings.TrimSpace(strings.Trim(string(output), "'"))
			}
		}
	case "kde":
		// 读取 KDE 代理设置
		configDir := filepath.Join(os.Getenv("HOME"), ".config")
		kioslaverc := filepath.Join(configDir, "kioslaverc")
		if data, err := os.ReadFile(kioslaverc); err == nil {
			content := string(data)
			if strings.Contains(content, "ProxyType=1") {
				state.Enabled = true
				// 解析 SOCKS 代理
				lines := strings.Split(content, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "SOCKSProxy=") {
						state.ProxyServer = strings.TrimSpace(strings.TrimPrefix(line, "SOCKSProxy="))
					}
					if strings.HasPrefix(line, "NoProxyForList=") {
						state.BypassList = strings.TrimSpace(strings.TrimPrefix(line, "NoProxyForList="))
					}
				}
			}
		}
	case "xfce":
		// 读取 XFCE 代理设置
		cmd := exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/mode")
		output, err := cmd.CombinedOutput()
		if err == nil {
			mode := strings.TrimSpace(string(output))
			if mode == "manual" {
				state.Enabled = true

				// 读取 SOCKS 代理
				cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/socks/host")
				output, _ = cmd.CombinedOutput()
				host := strings.TrimSpace(string(output))

				cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/socks/port")
				output, _ = cmd.CombinedOutput()
				port := strings.TrimSpace(string(output))

				if len(host) != 0 && len(port) != 0 {
					state.ProxyServer = host + ":" + port
				}

				// 读取绕过列表
				cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/no-proxy")
				output, _ = cmd.CombinedOutput()
				state.BypassList = strings.TrimSpace(string(output))
			}
		}
	}

	return state, nil
}

// SaveProxyState 保存当前代理状态
func SaveProxyState() error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := getCurrentProxyState()
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
	defer func() {
		originalState = nil
	}()

	// 检测桌面环境
	desktopEnv := detectDesktopEnvironment()

	switch desktopEnv {
	case "gnome", "gnome-wayland", "gnome-xorg":
		if originalState.Enabled && len(originalState.ProxyServer) != 0 {
			parts := strings.Split(originalState.ProxyServer, ":")
			if len(parts) == 2 {
				host := parts[0]
				port := parts[1]

				// 设置代理模式
				cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")
				cmd.Run()

				// 设置 SOCKS 代理
				cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", host)
				cmd.Run()

				cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", port)
				cmd.Run()

				// 设置绕过列表
				if len(originalState.BypassList) != 0 {
					cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", originalState.BypassList)
					cmd.Run()
				}
			}
		} else {
			// 关闭代理
			cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none")
			cmd.Run()
		}
	case "kde":
		configDir := filepath.Join(os.Getenv("HOME"), ".config")
		kioslaverc := filepath.Join(configDir, "kioslaverc")

		if originalState.Enabled && len(originalState.ProxyServer) != 0 {
			content := fmt.Sprintf(`[Proxy Settings]
ProxyType=1
SOCKSProxy=%s
SOCKSVersion=5
NoProxyForList=%s
ReversedException=false
`, originalState.ProxyServer, originalState.BypassList)
			os.WriteFile(kioslaverc, []byte(content), 0644)
		} else {
			content := `[Proxy Settings]
ProxyType=0
`
			os.WriteFile(kioslaverc, []byte(content), 0644)
		}
		// 通知 KDE 刷新配置
		cmd := exec.Command("qdbus", "org.kde.kded5", "/modules/proxy", "org.kde.KProxyProxyManager.proxyChanged")
		_ = cmd.Run()
	case "xfce":
		if originalState.Enabled && len(originalState.ProxyServer) != 0 {
			parts := strings.Split(originalState.ProxyServer, ":")
			if len(parts) == 2 {
				host := parts[0]
				port := parts[1]

				// 设置代理模式
				cmd := exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/mode", "-s", "manual")
				cmd.Run()

				// 设置 SOCKS 代理
				cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/socks/host", "-s", host)
				cmd.Run()

				cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/socks/port", "-s", port)
				cmd.Run()

				// 设置绕过列表
				if len(originalState.BypassList) != 0 {
					cmd = exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/no-proxy", "-s", originalState.BypassList)
					cmd.Run()
				}
			}
		} else {
			// 关闭代理
			cmd := exec.Command("xfconf-query", "-c", "xfce4-proxy", "-p", "/mode", "-s", "none")
			cmd.Run()
		}
	}

	log.Printf("[系统] 已恢复代理状态: enabled=%v, server=%s\n", originalState.Enabled, originalState.ProxyServer)
	return nil
}
