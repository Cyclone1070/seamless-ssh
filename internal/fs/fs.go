package fs

import (
	"io/fs"
	"os"
)

type RealFS struct{}

func NewRealFS() *RealFS {
	return &RealFS{}
}

func (RealFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (RealFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (RealFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (RealFS) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}
