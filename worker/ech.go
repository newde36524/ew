package worker

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/newde36524/ew/utils/log"

	"reflect"
	"sync"

	"github.com/newde36524/ew/utils"
)

type Ech struct {
	dnsServer string
	echDomain string
	echListMu sync.RWMutex
	echList   []byte
}

func NewEch(dnsServer, echDomain string) *Ech {
	return &Ech{
		dnsServer: dnsServer,
		echDomain: echDomain,
	}
}

func (e *Ech) PrepareECH() error {
	echBase64, err := utils.QueryHTTPSRecord(e.echDomain, e.dnsServer)
	if err != nil {
		return fmt.Errorf("DNS 查询失败: %w", err)
	}
	if len(echBase64) == 0 {
		return errors.New("未找到 ECH 参数")
	}
	raw, err := base64.StdEncoding.DecodeString(echBase64)
	if err != nil {
		return fmt.Errorf("ECH 解码失败: %w", err)
	}
	e.echListMu.Lock()
	e.echList = raw
	e.echListMu.Unlock()
	log.Printf("[ECH] 配置已加载，长度: %d 字节", len(raw))
	return nil
}

func (e *Ech) RefreshECH() error {
	log.Printf("[ECH] 刷新配置...")
	return e.PrepareECH()
}

func (e *Ech) GetECHList() ([]byte, error) {
	e.echListMu.RLock()
	defer e.echListMu.RUnlock()
	if len(e.echList) == 0 {
		return nil, errors.New("ECH 配置未加载")
	}
	return e.echList, nil
}

func (e *Ech) BuildTLSConfigWithECH(serverName string, echList []byte) (*tls.Config, error) {
	if len(echList) == 0 {
		return nil, errors.New("ECH 配置为空，这是必需功能")
	}
	config, err := utils.BuildTLSConfigWithECH(serverName)
	if err != nil {
		return nil, err
	}
	// 使用反射设置 ECH 字段（ECH 是核心功能，必须设置成功）
	if err := e.setECHConfig(config, echList); err != nil {
		return nil, fmt.Errorf("设置 ECH 配置失败（需要 Go 1.23+ 或支持 ECH 的版本）: %w", err)
	}

	return config, nil
}

// setECHConfig 使用反射设置 ECH 配置（ECH 是核心功能，必须成功）
func (e *Ech) setECHConfig(config *tls.Config, echList []byte) error {
	configValue := reflect.ValueOf(config).Elem()

	// 设置 EncryptedClientHelloConfigList（必需）
	field1 := configValue.FieldByName("EncryptedClientHelloConfigList")
	if !field1.IsValid() || !field1.CanSet() {
		return fmt.Errorf("EncryptedClientHelloConfigList 字段不可用，需要 Go 1.23+ 版本")
	}
	field1.Set(reflect.ValueOf(echList))

	// 设置 EncryptedClientHelloRejectionVerify（必需）
	field2 := configValue.FieldByName("EncryptedClientHelloRejectionVerify")
	if !field2.IsValid() || !field2.CanSet() {
		return fmt.Errorf("EncryptedClientHelloRejectionVerify 字段不可用，需要 Go 1.23+ 版本")
	}
	rejectionFunc := func(cs tls.ConnectionState) error {
		return errors.New("服务器拒绝 ECH")
	}
	field2.Set(reflect.ValueOf(rejectionFunc))

	return nil
}

func (e *Ech) GetTlsCfg() (*tls.Config, error) {
	echBytes, err := e.GetECHList()
	if err != nil {
		return nil, fmt.Errorf("获取 ECH 配置失败: %w", err)
	}

	tlsCfg, err := e.BuildTLSConfigWithECH("cloudflare-dns.com", echBytes)
	if err != nil {
		return nil, fmt.Errorf("构建 TLS 配置失败: %w", err)
	}
	return tlsCfg, nil
}
