package bboltfs

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

type bboltDirFile struct {
	fs   *BBolt
	name string
	meta fileMeta
}

func (d *bboltDirFile) Name() string                                 { return d.name }
func (d *bboltDirFile) Read(p []byte) (int, error)                   { return 0, io.EOF }
func (d *bboltDirFile) ReadAt(p []byte, off int64) (int, error)      { return 0, io.EOF }
func (d *bboltDirFile) Seek(offset int64, whence int) (int64, error) { return 0, io.EOF }
func (d *bboltDirFile) Write(p []byte) (int, error)                  { return 0, os.ErrInvalid }
func (d *bboltDirFile) WriteAt(p []byte, off int64) (int, error)     { return 0, os.ErrInvalid }
func (d *bboltDirFile) WriteString(s string) (int, error)            { return 0, os.ErrInvalid }
func (d *bboltDirFile) Close() error                                 { return nil }
func (d *bboltDirFile) Stat() (os.FileInfo, error) {
	return &fileInfo{
		name:    filepath.Base(d.name),
		size:    0,
		mode:    d.meta.Mode,
		modTime: time.Unix(0, d.meta.ModTime),
		isDir:   true,
	}, nil
}
func (d *bboltDirFile) Sync() error               { return nil }
func (d *bboltDirFile) Truncate(size int64) error { return os.ErrInvalid }
func (d *bboltDirFile) Readdir(count int) ([]os.FileInfo, error) {
	return d.fs.readDir(d.name, count)
}
func (d *bboltDirFile) Readdirnames(n int) ([]string, error) {
	infos, err := d.fs.readDir(d.name, n)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, fi := range infos {
		names = append(names, fi.Name())
	}
	return names, nil
}
