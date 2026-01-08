package utils

import (
	"fmt"

	"github.com/newde36524/ew/utils/log"

	"os"
	"path/filepath"
)

type DataSync interface {
	Sync() (data []byte, err error)
}

type FileSync struct {
	tag      string
	fileName string
	data     []byte
	source   func() ([]byte, error)
}

func NewFileSync(tag string, fileName string, source func() ([]byte, error)) *FileSync {
	return &FileSync{
		tag:      tag,
		fileName: fileName,
		source:   source,
	}
}

func (f *FileSync) Sync() (data []byte, err error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	fileFullName := filepath.Join(exeDir, f.fileName)

	needDownload := false
	if info, err := os.Stat(fileFullName); os.IsNotExist(err) {
		needDownload = true
		log.Printf("[加载] %s 列表文件不存在，将自动下载", f.tag)
	} else if info.Size() == 0 {
		needDownload = true
		log.Printf("[加载] %s 列表文件为空，将自动下载", f.tag)
	}
	if needDownload {
		data, err = f.source()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(fileFullName, data, 0644); err != nil {
			return nil, fmt.Errorf("保存文件失败: %w", err)
		}
		log.Printf("[下载] 已保存到: %s", fileFullName)
		f.data = data
		return data, nil
	}
	return os.ReadFile(fileFullName)
}
