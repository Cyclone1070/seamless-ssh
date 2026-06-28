package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/sssh/internal/domain"
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
