package utils

import (
	"net/http"
	"time"
)

func init() {
	http.DefaultClient.Timeout = 30 * time.Second
	http.DefaultClient.Transport = &http.Transport{
		Proxy: nil, // 显式设置为 nil 表示不使用任何代理
	}
}
