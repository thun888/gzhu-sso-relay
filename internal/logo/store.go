package logo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Store 把应用图标按 client_id 存成文件，内容类型记在数据库里。
type Store struct {
	dir string
}

func New(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) path(clientID string) string {
	return filepath.Join(s.dir, clientID)
}

func (s *Store) Save(clientID string, data []byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("logo: 创建目录 %s: %w", s.dir, err)
	}
	if err := os.WriteFile(s.path(clientID), data, 0o644); err != nil {
		return fmt.Errorf("logo: 写入 %s: %w", clientID, err)
	}
	return nil
}

func (s *Store) Load(clientID string) ([]byte, error) {
	data, err := os.ReadFile(s.path(clientID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("logo: 读取 %s: %w", clientID, err)
	}
	return data, nil
}

func (s *Store) Remove(clientID string) error {
	err := os.Remove(s.path(clientID))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("logo: 删除 %s: %w", clientID, err)
	}
	return nil
}

// Exists 用于启动时判断旧数据是否需要迁移。
func (s *Store) Exists(clientID string) bool {
	_, err := os.Stat(s.path(clientID))
	return err == nil
}
