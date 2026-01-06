package worker

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/newde36524/ew/utils"
)

// ipRange 表示一个IPv4 IP范围
type ipRange struct {
	start uint32
	end   uint32
}

// ipRangeV6 表示一个IPv6 IP范围
type ipRangeV6 struct {
	start [16]byte
	end   [16]byte
}

type IPLoader struct {
	// 中国IP列表（IPv4）
	chinaIPRangesMu sync.RWMutex
	chinaIPRanges   []ipRange

	// 中国IP列表（IPv6）
	chinaIPV6RangesMu sync.RWMutex
	chinaIPV6Ranges   []ipRangeV6
	routingMode       string
	ipv4DataSync      utils.DataSync
	ipv6DataSync      utils.DataSync
}

func NewIPLoader(routingMode string) *IPLoader {
	return &IPLoader{
		routingMode: routingMode,
		ipv4DataSync: utils.NewFileSync("IPV4", "chn_ip.txt", func() ([]byte, error) {
			url := "https://gh-proxy.com/https://raw.githubusercontent.com/mayaxcn/china-ip-list/refs/heads/master/chn_ip.txt"
			log.Printf("[下载] 正在下载 IP 列表: %s", url)
			content, err := utils.GetDataByUrl(url)
			if err != nil {
				return nil, fmt.Errorf("自动下载 IPv4 列表失败: %w", err)
			}
			return content, nil
		}),
		ipv6DataSync: utils.NewFileSync("IPV6", "chn_ip_v6.txt", func() ([]byte, error) {
			url := "https://gh-proxy.com/https://raw.githubusercontent.com/mayaxcn/china-ip-list/refs/heads/master/chn_ip_v6.txt"
			log.Printf("[下载] 正在下载 IP 列表: %s", url)
			content, err := utils.GetDataByUrl(url)
			if err != nil {
				log.Printf("[警告] 自动下载 IPv6 列表失败: %v，将跳过 IPv6 支持", err)
				return nil, nil // IPv6 列表下载失败不算致命错误
			}
			return content, nil
		}),
	}
}

func (i *IPLoader) LoadWithRoutingMode() {
	// 加载中国IP列表（如果需要）
	switch i.routingMode {
	case BypassCN:
		log.Printf("[启动] 分流模式: 跳过中国大陆，正在加载中国IP列表...")

		ipv4Count, ipv6Count := 0, 0
		if err := i.LoadChinaIPList(); err != nil {
			log.Printf("[警告] 加载中国IPv4列表失败: %v", err)
		} else {
			i.chinaIPRangesMu.RLock()
			ipv4Count = len(i.chinaIPRanges)
			i.chinaIPRangesMu.RUnlock()
		}

		if err := i.LoadChinaIPV6List(); err != nil {
			log.Printf("[警告] 加载中国IPv6列表失败: %v", err)
		} else {
			i.chinaIPV6RangesMu.RLock()
			ipv6Count = len(i.chinaIPV6Ranges)
			i.chinaIPV6RangesMu.RUnlock()
		}

		if ipv4Count > 0 || ipv6Count > 0 {
			log.Printf("[启动] 已加载 %d 个中国IPv4段, %d 个中国IPv6段", ipv4Count, ipv6Count)
		} else {
			log.Printf("[警告] 未加载到任何中国IP列表，将使用默认规则")
		}
	case Global:
		log.Printf("[启动] 分流模式: 全局代理")
	case None:
		log.Printf("[启动] 分流模式: 不改变代理（直连模式）")
	default:
		log.Printf("[警告] 未知的分流模式: %s，使用默认模式 global", i.routingMode)
		i.routingMode = Global
	}
}

// IsChinaIP 检查IP是否在中国IP列表中（支持IPv4和IPv6）
func (i *IPLoader) IsChinaIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// 检查IPv4
	if ip.To4() != nil {
		ipUint32 := utils.IpToUint32(ip)
		if ipUint32 == 0 {
			return false
		}

		i.chinaIPRangesMu.RLock()
		defer i.chinaIPRangesMu.RUnlock()

		// 二分查找
		left, right := 0, len(i.chinaIPRanges)
		for left < right {
			mid := (left + right) / 2
			r := i.chinaIPRanges[mid]
			if ipUint32 < r.start {
				right = mid
			} else if ipUint32 > r.end {
				left = mid + 1
			} else {
				return true
			}
		}
		return false
	}

	// 检查IPv6
	ipBytes := ip.To16()
	if ipBytes == nil {
		return false
	}

	var ipArray [16]byte
	copy(ipArray[:], ipBytes)

	i.chinaIPV6RangesMu.RLock()
	defer i.chinaIPV6RangesMu.RUnlock()

	// 二分查找IPv6
	left, right := 0, len(i.chinaIPV6Ranges)
	for left < right {
		mid := (left + right) / 2
		r := i.chinaIPV6Ranges[mid]

		// 比较起始IP
		cmpStart := utils.CompareIPv6(ipArray, r.start)
		if cmpStart < 0 {
			right = mid
			continue
		}

		// 比较结束IP
		cmpEnd := utils.CompareIPv6(ipArray, r.end)
		if cmpEnd > 0 {
			left = mid + 1
			continue
		}

		// 在范围内
		return true
	}
	return false
}

// LoadChinaIPList 从程序目录加载中国IP列表
func (i *IPLoader) LoadChinaIPList() error {
	data, err := i.ipv4DataSync.Sync()
	if err != nil {
		return err
	}
	var ranges []ipRange
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		startIP := net.ParseIP(parts[0])
		endIP := net.ParseIP(parts[1])
		if startIP == nil || endIP == nil {
			continue
		}

		start := utils.IpToUint32(startIP)
		end := utils.IpToUint32(endIP)
		if start > 0 && end > 0 && start <= end {
			ranges = append(ranges, ipRange{start: start, end: end})
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取IP列表文件失败: %w", err)
	}

	if len(ranges) == 0 {
		return errors.New("IP列表为空")
	}

	// 按起始IP排序
	for i := 0; i < len(ranges)-1; i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[i].start > ranges[j].start {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}
	i.chinaIPRangesMu.Lock()
	i.chinaIPRanges = ranges
	i.chinaIPRangesMu.Unlock()
	return nil
}

// LoadChinaIPV6List 从程序目录加载中国IPv6 IP列表
func (i *IPLoader) LoadChinaIPV6List() error {
	data, err := i.ipv6DataSync.Sync()
	if err != nil {
		return err
	}
	var ranges []ipRangeV6
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		startIP := net.ParseIP(parts[0])
		endIP := net.ParseIP(parts[1])
		if startIP == nil || endIP == nil {
			continue
		}

		// 转换为16字节数组
		startBytes := startIP.To16()
		endBytes := endIP.To16()
		if startBytes == nil || endBytes == nil {
			continue
		}

		var start, end [16]byte
		copy(start[:], startBytes)
		copy(end[:], endBytes)

		// 检查范围是否有效
		if utils.CompareIPv6(start, end) <= 0 {
			ranges = append(ranges, ipRangeV6{start: start, end: end})
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取IPv6 IP列表文件失败: %w", err)
	}

	if len(ranges) == 0 {
		// IPv6列表为空不算错误，可能文件不存在或为空
		return nil
	}

	// 按起始IP排序
	for i := 0; i < len(ranges)-1; i++ {
		for j := i + 1; j < len(ranges); j++ {
			if utils.CompareIPv6(ranges[i].start, ranges[j].start) > 0 {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}

	i.chinaIPV6RangesMu.Lock()
	i.chinaIPV6Ranges = ranges
	i.chinaIPV6RangesMu.Unlock()
	return nil
}

// ShouldBypassProxy 根据分流模式判断是否应该绕过代理（直连）
func (i *IPLoader) ShouldBypassProxy(targetHost string) bool {
	if i.routingMode == None {
		// "不改变代理"模式：所有流量都直连
		return true
	}
	if i.routingMode == Global {
		// "全局代理"模式：所有流量都走代理
		return false
	}
	if i.routingMode == BypassCN {
		// "跳过中国大陆"模式：检查是否是中国IP
		// 先尝试解析为IP
		if ip := net.ParseIP(targetHost); ip != nil {
			return i.IsChinaIP(targetHost)
		}
		// 如果是域名，先解析IP
		ips, err := net.LookupIP(targetHost)
		if err != nil {
			// 解析失败，默认走代理
			return false
		}
		// 检查所有解析到的IP，如果有一个是中国IP，就直连
		for _, ip := range ips {
			if i.IsChinaIP(ip.String()) {
				return true
			}
		}
		// 都不是中国IP，走代理
		return false
	}
	// 未知模式，默认走代理
	return false
}
