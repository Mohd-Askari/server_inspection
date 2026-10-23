package collector

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FSReader abstracts file system reads for collectors, allowing test fixtures.
type FSReader interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]os.DirEntry, error)
	ReadLink(path string) (string, error)
	Stat(path string) (os.FileInfo, error)
	Exists(path string) bool
}

// OSFSReader reads directly from the host operating system.
type OSFSReader struct{}

// NewOSFSReader returns an OSFSReader instance.
func NewOSFSReader() *OSFSReader {
	return &OSFSReader{}
}

func (r *OSFSReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (r *OSFSReader) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

func (r *OSFSReader) ReadLink(path string) (string, error) {
	return os.Readlink(path)
}

func (r *OSFSReader) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (r *OSFSReader) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MockFSReader provides in-memory or fixture-backed files for unit tests & dev.
type MockFSReader struct {
	files    map[string][]byte
	symlinks map[string]string
	dirs     map[string][]os.DirEntry
}

// NewMockFSReader creates an in-memory FS reader.
func NewMockFSReader() *MockFSReader {
	return &MockFSReader{
		files:    make(map[string][]byte),
		symlinks: make(map[string]string),
		dirs:     make(map[string][]os.DirEntry),
	}
}

// SetFile adds or updates a virtual file.
func (m *MockFSReader) SetFile(path string, content string) {
	m.files[filepath.Clean(path)] = []byte(content)
}

// SetBytes adds or updates a virtual file from bytes.
func (m *MockFSReader) SetBytes(path string, data []byte) {
	m.files[filepath.Clean(path)] = data
}

// SetSymlink sets a mock symlink target.
func (m *MockFSReader) SetSymlink(path, target string) {
	m.symlinks[filepath.Clean(path)] = target
}

// AddDirEntry registers a directory entry for ReadDir.
func (m *MockFSReader) AddDirEntry(dir string, entry os.DirEntry) {
	cleanDir := filepath.Clean(dir)
	m.dirs[cleanDir] = append(m.dirs[cleanDir], entry)
}

func (m *MockFSReader) ReadFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	if data, ok := m.files[clean]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFSReader) ReadDir(path string) ([]os.DirEntry, error) {
	clean := filepath.Clean(path)
	if entries, ok := m.dirs[clean]; ok {
		return entries, nil
	}
	// Fallback: search keys prefixed with clean/
	var entries []os.DirEntry
	seen := make(map[string]bool)
	prefix := clean + "/"
	for k := range m.files {
		if strings.HasPrefix(k, prefix) {
			rel := strings.TrimPrefix(k, prefix)
			parts := strings.Split(rel, "/")
			name := parts[0]
			if !seen[name] {
				seen[name] = true
				entries = append(entries, &mockDirEntry{name: name, isDir: len(parts) > 1})
			}
		}
	}
	if len(entries) > 0 {
		return entries, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFSReader) ReadLink(path string) (string, error) {
	clean := filepath.Clean(path)
	if target, ok := m.symlinks[clean]; ok {
		return target, nil
	}
	return "", os.ErrNotExist
}

func (m *MockFSReader) Stat(path string) (os.FileInfo, error) {
	clean := filepath.Clean(path)
	if _, ok := m.files[clean]; ok {
		return &mockFileInfo{name: filepath.Base(clean), size: int64(len(m.files[clean]))}, nil
	}
	if _, ok := m.dirs[clean]; ok {
		return &mockFileInfo{name: filepath.Base(clean), isDir: true}, nil
	}
	prefix := clean + "/"
	for k := range m.files {
		if strings.HasPrefix(k, prefix) {
			return &mockFileInfo{name: filepath.Base(clean), isDir: true}, nil
		}
	}
	return nil, os.ErrNotExist
}

func (m *MockFSReader) Exists(path string) bool {
	clean := filepath.Clean(path)
	if _, ok := m.files[clean]; ok {
		return true
	}
	if _, ok := m.dirs[clean]; ok {
		return true
	}
	prefix := clean + "/"
	for k := range m.files {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

type mockDirEntry struct {
	name  string
	isDir bool
}

func (e *mockDirEntry) Name() string      { return e.name }
func (e *mockDirEntry) IsDir() bool       { return e.isDir }
func (e *mockDirEntry) Type() os.FileMode { return 0 }
func (e *mockDirEntry) Info() (os.FileInfo, error) {
	return &mockFileInfo{name: e.name, isDir: e.isDir}, nil
}

type mockFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (f *mockFileInfo) Name() string       { return f.name }
func (f *mockFileInfo) Size() int64        { return f.size }
func (f *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (f *mockFileInfo) ModTime() time.Time { return time.Now() }
func (f *mockFileInfo) IsDir() bool        { return f.isDir }
func (f *mockFileInfo) Sys() any           { return nil }

// CommandRunner abstracts subprocess execution.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath(file string) (string, error)
}

// OSCommandRunner executes commands safely on the host with timeout and context.
type OSCommandRunner struct {
	DefaultTimeout time.Duration
}

// NewOSCommandRunner creates an OSCommandRunner with standard 5s execution timeout.
func NewOSCommandRunner(defaultTimeout time.Duration) *OSCommandRunner {
	if defaultTimeout <= 0 {
		defaultTimeout = 5 * time.Second
	}
	return &OSCommandRunner{DefaultTimeout: defaultTimeout}
}

// Run executes command without shell interpolation, respecting context deadlines.
func (r *OSCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), r.DefaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("command %s %v failed: %w (output: %s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// LookPath checks if executable exists in host PATH.
func (r *OSCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// MockCommandRunner enables deterministic testing for utilities (systemctl, smartctl, etc).
type MockCommandRunner struct {
	Commands map[string][]byte
	Errors   map[string]error
	Paths    map[string]string
}

// NewMockCommandRunner returns a mock runner.
func NewMockCommandRunner() *MockCommandRunner {
	return &MockCommandRunner{
		Commands: make(map[string][]byte),
		Errors:   make(map[string]error),
		Paths:    make(map[string]string),
	}
}

func (m *MockCommandRunner) SetResult(cmd string, output []byte, err error) {
	m.Commands[cmd] = output
	if err != nil {
		m.Errors[cmd] = err
	}
}

func (m *MockCommandRunner) SetPath(file, path string) {
	m.Paths[file] = path
}

func (m *MockCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	fullCmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	if err, ok := m.Errors[fullCmd]; ok {
		return m.Commands[fullCmd], err
	}
	if out, ok := m.Commands[fullCmd]; ok {
		return out, nil
	}
	// Try prefix match for command name
	if out, ok := m.Commands[name]; ok {
		return out, m.Errors[name]
	}
	return nil, fmt.Errorf("mock command not found: %s", fullCmd)
}

func (m *MockCommandRunner) LookPath(file string) (string, error) {
	if p, ok := m.Paths[file]; ok {
		return p, nil
	}
	return "", fmt.Errorf("executable %q not found in mock PATH", file)
}

// ReadTrimmed reads a file and trims leading/trailing whitespace.
func ReadTrimmed(fs FSReader, path string) (string, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SafeReadString reads a file and returns trimmed string, or empty string on error.
func SafeReadString(fs FSReader, path string) string {
	val, err := ReadTrimmed(fs, path)
	if err != nil {
		return ""
	}
	return val
}

// DiscardCloser wraps an io.Reader.
type DiscardCloser struct {
	io.Reader
}

func (DiscardCloser) Close() error { return nil }
