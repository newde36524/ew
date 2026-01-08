//go:build windows
// +build windows

// nolint: errcheck
package utils

import (
	"fmt"

	"github.com/newde36524/ew/utils/log"

	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modwininet             = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOptionW = modwininet.NewProc("InternetSetOptionW")
	modadvapi32            = syscall.NewLazyDLL("advapi32.dll")
	procRegSetValueEx      = modadvapi32.NewProc("RegSetValueExW")
	procRegOpenKeyEx       = modadvapi32.NewProc("RegOpenKeyExW")
	procRegCloseKey        = modadvapi32.NewProc("RegCloseKey")
	procRegQueryValueEx    = modadvapi32.NewProc("RegQueryValueExW")
)

const (
	INTERNET_OPTION_SETTINGS_CHANGED = 39
	INTERNET_OPTION_REFRESH          = 37
	KEY_SET_VALUE                    = 0x0002
	KEY_QUERY_VALUE                  = 0x0001
	REG_SZ                           = 1
	REG_DWORD                        = 4
)

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

// GetProxyBypassList 获取代理绕过列表
func GetProxyBypassList() string {
	// 基础绕过列表（本地和内网）
	// 注意：分流功能已在 Go 程序中实现，系统代理设置为全局代理
	// Go 程序会根据分流模式自动决定哪些流量走代理，哪些直连
	baseBypass := "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"
	return baseBypass
}

// SetSystemProxy 设置 Windows 系统代理
func SetSystemProxy(enabled bool, listenAddr, routingMode string) error {
	// 打开注册表
	key, err := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	if err != nil {
		return fmt.Errorf("注册表路径转换失败: %v", err)
	}

	var hKey syscall.Handle
	ret, _, _ := procRegOpenKeyEx.Call(
		uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(key)),
		0,
		uintptr(KEY_SET_VALUE),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return fmt.Errorf("打开注册表失败: 错误码 %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	if enabled {
		// 解析监听地址
		proxyServer := strings.ReplaceAll(listenAddr, "0.0.0.0", "127.0.0.1")
		if !strings.Contains(listenAddr, ":") {
			proxyServer = "127.0.0.1:" + listenAddr
		}

		// 设置代理服务器
		proxyServerPtr, _ := syscall.UTF16PtrFromString(proxyServer)
		valueName, _ := syscall.UTF16PtrFromString("ProxyServer")
		ret, _, _ = procRegSetValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(REG_SZ),
			uintptr(unsafe.Pointer(proxyServerPtr)),
			uintptr(len(proxyServer)*2),
		)
		if ret != 0 {
			return fmt.Errorf("设置 ProxyServer 失败: 错误码 %d", ret)
		}

		// 启用代理
		var enableValue uint32 = 1
		valueName, _ = syscall.UTF16PtrFromString("ProxyEnable")
		ret, _, _ = procRegSetValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(REG_DWORD),
			uintptr(unsafe.Pointer(&enableValue)),
			uintptr(4),
		)
		if ret != 0 {
			return fmt.Errorf("设置 ProxyEnable 失败: 错误码 %d", ret)
		}

		// 设置绕过列表
		bypassList := GetProxyBypassList()
		bypassPtr, _ := syscall.UTF16PtrFromString(bypassList)
		valueName, _ = syscall.UTF16PtrFromString("ProxyOverride")
		ret, _, _ = procRegSetValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(REG_SZ),
			uintptr(unsafe.Pointer(bypassPtr)),
			uintptr(len(bypassList)*2),
		)
		if ret != 0 {
			return fmt.Errorf("设置 ProxyOverride 失败: 错误码 %d", ret)
		}

		log.Printf("[系统] Windows 代理已设置: %s, 分流模式: %s\n", proxyServer, routingMode)
	} else {
		// 关闭代理
		var enableValue uint32 = 0
		valueName, _ := syscall.UTF16PtrFromString("ProxyEnable")
		ret, _, _ = procRegSetValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(REG_DWORD),
			uintptr(unsafe.Pointer(&enableValue)),
			uintptr(4),
		)
		if ret != 0 {
			return fmt.Errorf("关闭 ProxyEnable 失败: 错误码 %d", ret)
		}
		log.Println("[系统] Windows 代理已关闭")
	}

	// 通知系统代理设置已更改
	procInternetSetOptionW.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOptionW.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)

	return nil
}

// getCurrentProxyState 获取当前代理状态
func getCurrentProxyState() (*ProxyState, error) {
	key, err := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	if err != nil {
		return nil, fmt.Errorf("注册表路径转换失败: %v", err)
	}

	var hKey syscall.Handle
	ret, _, _ := procRegOpenKeyEx.Call(
		uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(key)),
		0,
		uintptr(KEY_QUERY_VALUE),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return nil, fmt.Errorf("打开注册表失败: 错误码 %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	state := &ProxyState{}

	// 读取 ProxyEnable
	valueName, _ := syscall.UTF16PtrFromString("ProxyEnable")
	var enableValue uint32
	var dataSize uint32 = 4
	var regType uint32
	ret, _, _ = procRegQueryValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(unsafe.Pointer(&regType)),
		uintptr(unsafe.Pointer(&enableValue)),
		uintptr(unsafe.Pointer(&dataSize)),
	)
	if ret == 0 {
		state.Enabled = enableValue != 0
	}

	// 读取 ProxyServer
	valueName, _ = syscall.UTF16PtrFromString("ProxyServer")
	dataSize = 0
	ret, _, _ = procRegQueryValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(unsafe.Pointer(&regType)),
		0,
		uintptr(unsafe.Pointer(&dataSize)),
	)
	if ret == 0 && dataSize > 0 {
		proxyServer := make([]uint16, dataSize/2)
		ret, _, _ = procRegQueryValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(unsafe.Pointer(&regType)),
			uintptr(unsafe.Pointer(&proxyServer[0])),
			uintptr(unsafe.Pointer(&dataSize)),
		)
		if ret == 0 {
			state.ProxyServer = syscall.UTF16ToString(proxyServer)
		}
	}

	// 读取 ProxyOverride
	valueName, _ = syscall.UTF16PtrFromString("ProxyOverride")
	dataSize = 0
	ret, _, _ = procRegQueryValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(unsafe.Pointer(&regType)),
		0,
		uintptr(unsafe.Pointer(&dataSize)),
	)
	if ret == 0 && dataSize > 0 {
		bypassList := make([]uint16, dataSize/2)
		ret, _, _ = procRegQueryValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(unsafe.Pointer(&regType)),
			uintptr(unsafe.Pointer(&bypassList[0])),
			uintptr(unsafe.Pointer(&dataSize)),
		)
		if ret == 0 {
			state.BypassList = syscall.UTF16ToString(bypassList)
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

	key, err := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	if err != nil {
		return fmt.Errorf("注册表路径转换失败: %v", err)
	}

	var hKey syscall.Handle
	ret, _, _ := procRegOpenKeyEx.Call(
		uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(key)),
		0,
		uintptr(KEY_SET_VALUE),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return fmt.Errorf("打开注册表失败: 错误码 %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	// 恢复 ProxyEnable
	var enableValue uint32
	if originalState.Enabled {
		enableValue = 1
	} else {
		enableValue = 0
	}

	valueName, _ := syscall.UTF16PtrFromString("ProxyEnable")
	ret, _, _ = procRegSetValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(REG_DWORD),
		uintptr(unsafe.Pointer(&enableValue)),
		uintptr(4),
	)
	if ret != 0 {
		return fmt.Errorf("恢复 ProxyEnable 失败: 错误码 %d", ret)
	}

	// 恢复 ProxyServer
	if len(originalState.ProxyServer) != 0 {
		proxyServerPtr, _ := syscall.UTF16PtrFromString(originalState.ProxyServer)
		valueName, _ = syscall.UTF16PtrFromString("ProxyServer")
		ret, _, _ = procRegSetValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(REG_SZ),
			uintptr(unsafe.Pointer(proxyServerPtr)),
			uintptr(len(originalState.ProxyServer)*2),
		)
		if ret != 0 {
			return fmt.Errorf("恢复 ProxyServer 失败: 错误码 %d", ret)
		}
	}

	// 恢复 ProxyOverride
	if len(originalState.BypassList) != 0 {
		bypassPtr, _ := syscall.UTF16PtrFromString(originalState.BypassList)
		valueName, _ = syscall.UTF16PtrFromString("ProxyOverride")
		ret, _, _ = procRegSetValueEx.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			uintptr(REG_SZ),
			uintptr(unsafe.Pointer(bypassPtr)),
			uintptr(len(originalState.BypassList)*2),
		)
		if ret != 0 {
			return fmt.Errorf("恢复 ProxyOverride 失败: 错误码 %d", ret)
		}
	}

	// 通知系统代理设置已更改
	procInternetSetOptionW.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOptionW.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)

	log.Printf("[系统] 已恢复代理状态: enabled=%v, server=%s\n", originalState.Enabled, originalState.ProxyServer)
	return nil
}
