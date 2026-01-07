package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/newde36524/ew/utils"
	"github.com/newde36524/ew/worker"
)

// ======================== 全局参数 ========================

var (
	listenAddr  string
	serverAddr  string
	serverIP    string
	token       string
	dnsServer   string
	echDomain   string
	routingMode string // 分流模式: "global", "bypass_cn", "none"
)

// func init() {
// 	flag.StringVar(&listenAddr, "l", "0.0.0.0:30000", "代理监听地址 (支持 SOCKS5 和 HTTP)")
// 	flag.StringVar(&serverAddr, "f", "", "服务端地址 (格式: x.x.workers.dev:443)")
// 	flag.StringVar(&serverIP, "ip", "", "指定服务端 IP（绕过 DNS 解析）")
// 	flag.StringVar(&token, "token", "", "身份验证令牌")
// 	flag.StringVar(&dnsServer, "dns", "dns.alidns.com/dns-query", "ECH 查询 DoH 服务器")
// 	flag.StringVar(&echDomain, "ech", "cloudflare-ech.com", "ECH 查询域名")
// 	flag.StringVar(&routingMode, "routing", "global", "分流模式: global(全局代理), bypass_cn(跳过中国大陆), none(不改变代理)")
// }

func init() {
	flag.StringVar(&serverAddr, "f", "frosty-bar-5bc6.zhu718114245.workers.dev:443", "服务端地址 (格式: x.x.workers.dev:443)")
	flag.StringVar(&token, "token", "jmrx", "身份验证令牌")
	flag.StringVar(&listenAddr, "l", "0.0.0.0:30000", "代理监听地址 (支持 SOCKS5 和 HTTP)")
	flag.StringVar(&serverIP, "ip", "saas.sin.fan", "指定服务端 IP(绕过 DNS 解析)")
	flag.StringVar(&dnsServer, "dns", "dns.alidns.com/dns-query", "ECH 查询 DoH 服务器")
	flag.StringVar(&echDomain, "ech", "cloudflare-ech.com", "ECH 查询域名")
	flag.StringVar(&routingMode, "routing", "bypass_cn", "分流模式: global(全局代理), bypass_cn(跳过中国大陆), none(不改变代理)")
	flag.Parse()
}

func main() {
	if len(serverAddr) == 0 {
		log.Fatal("必须指定服务端地址 -f\n\n示例:\n  ./ew -l 0.0.0.0:30000 -f your-worker.workers.dev:443 -token your-token")
		return
	}

	//修改系统代理
	setSystemProxy(true, listenAddr, routingMode)

	//设置安全退出，处理善后工作
	safeExit()

	//关闭控制台日志输出提升性能
	log.Default().SetOutput(io.Discard)

	//启动代理
	run()
}

// setSystemProxy 设置系统代理（根据操作系统自动选择）
func setSystemProxy(enabled bool, listenAddr, routingMode string) {
	// 保存当前代理状态
	if err := utils.SaveProxyState(); err != nil {
		log.Printf("[系统] 保存代理状态失败: %v\n", err)
	}

	// 如果是"不改变代理"模式，不设置系统代理
	if routingMode == "none" && enabled {
		log.Println(`[系统] 分流模式为"不改变代理"，跳过系统代理设置`)
		return
	}

	// 调用 utils 包中的平台特定实现
	if err := utils.SetSystemProxy(enabled, listenAddr, routingMode); err != nil {
		log.Fatal(err)
	}
}

// safeExit 设置退出时恢复代理的信号处理
func safeExit() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\n[系统] 收到退出信号，正在恢复代理设置...")
		if err := utils.RestoreProxyState(); err != nil {
			log.Printf("[系统] 恢复代理状态失败: %v\n", err)
		}
		os.Exit(0)
	}()
}

// run 启动代理
func run() {
	config := &worker.ProxyServerConfig{
		ListenAddr: listenAddr,
		ServerAddr: serverAddr,
		ServerIP:   serverIP,
		Token:      token,
	}
	ipLoader := worker.NewIPLoader(routingMode)
	ech := worker.NewEch(dnsServer, echDomain)
	proxyServer := worker.NewProxyServer(config, ipLoader, ech)
	if err := proxyServer.Run(); err != nil {
		log.Fatal(err)
	}
}
