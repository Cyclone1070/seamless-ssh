package config_test

import (
	"errors"
	"io/fs"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/seamless-ssh/sssh/internal/config"
	"github.com/seamless-ssh/sssh/internal/domain"
)

// mockFileInfo implements fs.FileInfo for mockStat
type mockFileInfo struct {
	name string
	mode fs.FileMode
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() fs.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.mode.IsDir() }
func (m mockFileInfo) Sys() any           { return nil }

type mockFilesystem struct {
	readFiles  map[string][]byte
	readError  error
	written    map[string][]byte
	writeError error
	statMode   map[string]fs.FileMode
	statError  error
	mkdirError error
}

func (m *mockFilesystem) ReadFile(name string) ([]byte, error) {
	if m.readError != nil {
		return nil, m.readError
	}
	data, ok := m.readFiles[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *mockFilesystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if m.writeError != nil {
		return m.writeError
	}
	m.written[name] = data
	return nil
}

func (m *mockFilesystem) Stat(name string) (fs.FileInfo, error) {
	if m.statError != nil {
		return nil, m.statError
	}
	mode, ok := m.statMode[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return mockFileInfo{name: name, mode: mode}, nil
}

func (m *mockFilesystem) MkdirAll(path string, perm fs.FileMode) error {
	return m.mkdirError
}

func TestReadConfig_Success(t *testing.T) {
	content := []byte(`
hosts:
  - alias: dev-box
    host: 192.168.1.50
    port: 22
    user: ubuntu
    ssh_key_path: /Users/mac/.ssh/id_rsa
`)
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/config.yaml": content},
	}
	mgr := config.NewManager(fsys)
	cfg, err := mgr.ReadConfig("/path/to/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := domain.Config{
		Hosts: []domain.HostConfig{
			{
				Alias:      "dev-box",
				Host:       "192.168.1.50",
				Port:       22,
				User:       "ubuntu",
				SSHKeyPath: "/Users/mac/.ssh/id_rsa",
			},
		},
	}
	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("expected %+v, got %+v", expected, cfg)
	}
}

func TestReadConfig_FileNotFound(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{},
	}
	mgr := config.NewManager(fsys)
	_, err := mgr.ReadConfig("/path/to/nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestReadConfig_PermissionDenied(t *testing.T) {
	fsys := &mockFilesystem{
		readError: os.ErrPermission,
	}
	mgr := config.NewManager(fsys)
	_, err := mgr.ReadConfig("/path/to/config.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

func TestReadConfig_InvalidYAML(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/invalid.yaml": []byte(`invalid: yaml: config : [}`)},
	}
	mgr := config.NewManager(fsys)
	_, err := mgr.ReadConfig("/path/to/invalid.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadConfig_EmptyFile(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/empty.yaml": []byte("")},
	}
	mgr := config.NewManager(fsys)
	_, err := mgr.ReadConfig("/path/to/empty.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadConfig_MissingRequiredFields(t *testing.T) {
	content := []byte(`
hosts:
  - alias: dev-box
    host: ""
    port: 22
`)
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/config.yaml": content},
	}
	mgr := config.NewManager(fsys)
	_, err := mgr.ReadConfig("/path/to/config.yaml")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestReadLinks_Success(t *testing.T) {
	content := []byte(`{
		"/Users/mac/proj": {
			"local_path": "/Users/mac/proj",
			"remote_host": "dev-box",
			"remote_path": "/remote/proj",
			"patterns": ["go test *", "make"]
		}
	}`)
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/links.json": content},
	}
	mgr := config.NewManager(fsys)
	links, err := mgr.ReadLinks("/path/to/links.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := domain.Links{
		"/Users/mac/proj": domain.Link{
			LocalPath:  "/Users/mac/proj",
			RemoteHost: "dev-box",
			RemotePath: "/remote/proj",
			Patterns:   []string{"go test *", "make"},
		},
	}
	if !reflect.DeepEqual(links, expected) {
		t.Errorf("expected %+v, got %+v", expected, links)
	}
}

func TestReadLinks_FileNotFound(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{},
	}
	mgr := config.NewManager(fsys)
	links, err := mgr.ReadLinks("/path/to/nonexistent.json")
	if err != nil {
		t.Fatalf("expected nil error on nonexistent links file, got: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected empty links map, got %+v", links)
	}
}

func TestReadLinks_InvalidJSON(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/invalid.json": []byte(`{invalid json`)},
	}
	mgr := config.NewManager(fsys)
	_, err := mgr.ReadLinks("/path/to/invalid.json")
	if err == nil {
		t.Fatal("expected parsing error, got nil")
	}
}

func TestWriteLink_Success(t *testing.T) {
	content := []byte(`{}`)
	fsys := &mockFilesystem{
		readFiles: map[string][]byte{"/path/to/links.json": content},
		written:   map[string][]byte{},
	}
	mgr := config.NewManager(fsys)
	newLink := domain.Link{
		LocalPath:  "/Users/mac/proj",
		RemoteHost: "dev-box",
		RemotePath: "/remote/proj",
		Patterns:   []string{"go test *"},
	}

	err := mgr.WriteLink("/path/to/links.json", newLink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	writtenBytes := fsys.written["/path/to/links.json"]
	if len(writtenBytes) == 0 {
		t.Fatal("expected content to be written, but got empty bytes")
	}
}

func TestWriteLink_PermissionDenied(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles:  map[string][]byte{"/path/to/links.json": []byte(`{}`)},
		writeError: os.ErrPermission,
	}
	mgr := config.NewManager(fsys)
	newLink := domain.Link{LocalPath: "/Users/mac/proj"}

	err := mgr.WriteLink("/path/to/links.json", newLink)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

func TestWriteLink_DirCreationFailed(t *testing.T) {
	fsys := &mockFilesystem{
		readFiles:  map[string][]byte{"/path/to/links.json": []byte(`{}`)},
		mkdirError: errors.New("cannot create dir"),
	}
	mgr := config.NewManager(fsys)
	newLink := domain.Link{LocalPath: "/Users/mac/proj"}

	err := mgr.WriteLink("/path/to/links.json", newLink)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
