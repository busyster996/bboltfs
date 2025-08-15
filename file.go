package bboltfs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.isDir }
func (fi *fileInfo) Sys() interface{}   { return nil }

// fileMeta 存储文件或目录的元信息
type fileMeta struct {
	Mode    os.FileMode
	Size    int64
	ModTime int64
	IsDir   bool
}

// --------- bboltFile 实现 ---------
type bboltFile struct {
	fs     *BBolt
	name   string
	meta   fileMeta
	buffer *bytes.Buffer
	offset int64
	mu     sync.Mutex
	closed bool
}

func (f *bboltFile) Name() string { return f.name }

func (f *bboltFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	return f.buffer.Read(p)
}

func (f *bboltFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	buf := f.buffer.Bytes()
	if off >= int64(len(buf)) {
		return 0, io.EOF
	}
	n := copy(p, buf[off:])
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *bboltFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = int64(f.buffer.Len()) + offset
	case io.SeekEnd:
		abs = int64(f.buffer.Len()) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative position")
	}
	f.offset = abs
	return abs, nil
}

func (f *bboltFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	n, err := f.buffer.Write(p)
	if err != nil {
		return n, err
	}
	f.meta.Size = int64(f.buffer.Len())
	f.meta.ModTime = time.Now().UnixNano()
	return n, f.fs.saveFile(f.name, f.buffer.Bytes(), f.meta)
}

func (f *bboltFile) WriteAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	buf := f.buffer.Bytes()
	if off > int64(len(buf)) {
		// 填充0
		padding := make([]byte, off-int64(len(buf)))
		f.buffer.Write(padding)
		buf = f.buffer.Bytes()
	}
	tmp := make([]byte, len(buf))
	copy(tmp, buf)
	copy(tmp[off:], p)
	f.buffer = bytes.NewBuffer(tmp)
	f.meta.Size = int64(f.buffer.Len())
	f.meta.ModTime = time.Now().UnixNano()
	return len(p), f.fs.saveFile(f.name, f.buffer.Bytes(), f.meta)
}

func (f *bboltFile) WriteString(s string) (int, error) {
	return f.Write([]byte(s))
}

func (f *bboltFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *bboltFile) Stat() (os.FileInfo, error) {
	return &fileInfo{
		name:    filepath.Base(f.name),
		size:    f.meta.Size,
		mode:    f.meta.Mode,
		modTime: time.Unix(0, f.meta.ModTime),
		isDir:   f.meta.IsDir,
	}, nil
}

func (f *bboltFile) Sync() error { return nil }

func (f *bboltFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return os.ErrClosed
	}
	buf := f.buffer.Bytes()
	if int(size) < len(buf) {
		f.buffer = bytes.NewBuffer(buf[:size])
	} else if int(size) > len(buf) {
		padding := make([]byte, int(size)-len(buf))
		f.buffer.Write(padding)
	}
	f.meta.Size = size
	f.meta.ModTime = time.Now().UnixNano()
	return f.fs.saveFile(f.name, f.buffer.Bytes(), f.meta)
}

func (f *bboltFile) Readdir(count int) ([]os.FileInfo, error) {
	return f.fs.readDir(f.name, count)
}

func (f *bboltFile) Readdirnames(n int) ([]string, error) {
	infos, err := f.fs.readDir(f.name, n)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, fi := range infos {
		names = append(names, fi.Name())
	}
	return names, nil
}
