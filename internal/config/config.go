package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/seamless-ssh/sssh/internal/domain"
	"gopkg.in/yaml.v3"
)

type Filesystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Stat(name string) (fs.FileInfo, error)
	MkdirAll(path string, perm fs.FileMode) error
}

type Manager struct {
	fs Filesystem
}

func NewManager(fsys Filesystem) *Manager {
	return &Manager{fs: fsys}
}

func (m *Manager) ReadConfig(path string) (domain.Config, error) {
	data, err := m.fs.ReadFile(path)
	if err != nil {
		return domain.Config{}, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return domain.Config{}, errors.New("empty configuration file")
	}

	var cfg domain.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return domain.Config{}, err
	}

	// Validate config
	if len(cfg.Hosts) == 0 {
		return domain.Config{}, errors.New("no hosts configured")
	}
	for i, host := range cfg.Hosts {
		if strings.TrimSpace(host.Alias) == "" {
			return domain.Config{}, fmt.Errorf("host at index %d is missing 'alias'", i)
		}
		if strings.TrimSpace(host.Host) == "" {
			return domain.Config{}, fmt.Errorf("host at index %d is missing 'host'", i)
		}
		if host.Port <= 0 {
			return domain.Config{}, fmt.Errorf("host at index %d has invalid 'port'", i)
		}
		if strings.TrimSpace(host.User) == "" {
			return domain.Config{}, fmt.Errorf("host at index %d is missing 'user'", i)
		}
	}

	return cfg, nil
}

func (m *Manager) ReadLinks(path string) (domain.Links, error) {
	data, err := m.fs.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(domain.Links), nil
		}
		return nil, err
	}

	var links domain.Links
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, err
	}

	if links == nil {
		return make(domain.Links), nil
	}

	return links, nil
}

func (m *Manager) WriteLink(path string, link domain.Link) error {
	links, err := m.ReadLinks(path)
	if err != nil {
		return err
	}

	links[link.LocalPath] = link

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := m.fs.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return m.fs.WriteFile(path, data, 0600)
}
