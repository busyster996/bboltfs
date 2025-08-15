package bboltfs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

// File represents a file in the filesystem.
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt

	Name() string
	Readdir(count int) ([]os.FileInfo, error)
	Readdirnames(n int) ([]string, error)
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(size int64) error
	WriteString(s string) (ret int, err error)
}

// Fs is the filesystem interface.
//
// Any simulated or real filesystem should implement this interface.
type Fs interface {
	// Create creates a file in the filesystem, returning the file and an
	// error, if any happens.
	Create(name string) (File, error)

	// Mkdir creates a directory in the filesystem, return an error if any
	// happens.
	Mkdir(name string, perm os.FileMode) error

	// MkdirAll creates a directory path and all parents that does not exist
	// yet.
	MkdirAll(path string, perm os.FileMode) error

	// Open opens a file, returning it or an error, if any happens.
	Open(name string) (File, error)

	// OpenFile opens a file using the given flags and the given mode.
	OpenFile(name string, flag int, perm os.FileMode) (File, error)

	// Remove removes a file identified by name, returning an error, if any
	// happens.
	Remove(name string) error

	// RemoveAll removes a directory path and any children it contains. It
	// does not fail if the path does not exist (return nil).
	RemoveAll(path string) error

	// Rename renames a file.
	Rename(oldname, newname string) error

	// Stat returns a FileInfo describing the named file, or an error, if any
	// happens.
	Stat(name string) (os.FileInfo, error)

	// The name of this FileSystem
	Name() string

	// Chmod changes the mode of the named file to mode.
	Chmod(name string, mode os.FileMode) error

	// Chown changes the uid and gid of the named file.
	Chown(name string, uid, gid int) error

	// Chtimes changes the access and modification times of the named file
	Chtimes(name string, atime time.Time, mtime time.Time) error

	// Close the FileSystem
	Close() error
}

var (
	ErrFileNotFound      = os.ErrNotExist
	ErrFileExists        = os.ErrExist
	ErrDestinationExists = os.ErrExist
)

const (
	bucketFiles = "files" // 存储文件
	bucketDirs  = "dirs"  // 存储目录
)

// BBolt 文件系统实现
type BBolt struct {
	db   *bbolt.DB
	name string
}

func New(path string) (Fs, error) {
	bolt, err := bbolt.Open(path, os.ModePerm, &bbolt.Options{})
	if err != nil {
		return nil, err
	}

	err = bolt.Update(func(tx *bbolt.Tx) error {
		if _, e := tx.CreateBucketIfNotExists([]byte(bucketFiles)); e != nil {
			return e
		}
		if _, e := tx.CreateBucketIfNotExists([]byte(bucketDirs)); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &BBolt{db: bolt, name: path}, nil
}

func (fs *BBolt) saveFile(name string, data []byte, meta fileMeta) error {
	return fs.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketFiles))
		key := []byte(name)
		val := append(fs.encodeMeta(meta), data...)
		return b.Put(key, val)
	})
}

func (fs *BBolt) loadFile(name string) ([]byte, fileMeta, error) {
	var data []byte
	var meta fileMeta
	err := fs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketFiles))
		val := b.Get([]byte(name))
		if val == nil {
			return ErrFileNotFound
		}
		meta = fs.decodeMeta(val)
		data = val[fs.metaLen():]
		return nil
	})
	return data, meta, err
}

func (fs *BBolt) saveDir(name string, meta fileMeta) error {
	return fs.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketDirs))
		return b.Put([]byte(name), fs.encodeMeta(meta))
	})
}

func (fs *BBolt) Create(name string) (File, error) {
	now := time.Now().UnixNano()
	meta := fileMeta{Mode: 0666, Size: 0, ModTime: now, IsDir: false}
	buf := &bytes.Buffer{}
	if err := fs.saveFile(name, buf.Bytes(), meta); err != nil {
		return nil, err
	}
	return &bboltFile{fs: fs, name: name, meta: meta, buffer: buf}, nil
}

func (fs *BBolt) Mkdir(name string, perm os.FileMode) error {
	now := time.Now().UnixNano()
	meta := fileMeta{Mode: perm | os.ModeDir, Size: 0, ModTime: now, IsDir: true}
	return fs.saveDir(name, meta)
}

func (fs *BBolt) MkdirAll(p string, perm os.FileMode) error {
	dirs := strings.Split(filepath.Clean(p), string(os.PathSeparator))
	dir := ""
	for _, d := range dirs {
		if dir == "" {
			dir = d
		} else {
			dir = path.Join(dir, d)
		}
		if err := fs.Mkdir(dir, perm); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return nil
}

func (fs *BBolt) Open(name string) (File, error) {
	data, meta, err := fs.loadFile(name)
	if err == nil {
		return &bboltFile{fs: fs, name: name, meta: meta, buffer: bytes.NewBuffer(data)}, nil
	}
	// 如果不是文件，尝试打开目录
	var dmeta fileMeta
	dirErr := fs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketDirs))
		val := b.Get([]byte(name))
		if val == nil {
			return ErrFileNotFound
		}
		dmeta = fs.decodeMeta(val)
		return nil
	})
	if dirErr == nil {
		return &bboltDirFile{fs: fs, name: name, meta: dmeta}, nil
	}
	return nil, ErrFileNotFound
}

func (fs *BBolt) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if flag&(os.O_CREATE|os.O_RDWR|os.O_WRONLY) != 0 {
		return fs.Create(name)
	}
	return fs.Open(name)
}

func (fs *BBolt) Remove(name string) error {
	return fs.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketFiles))
		return b.Delete([]byte(name))
	})
}

func (fs *BBolt) RemoveAll(p string) error {
	// 递归删除子文件
	err := fs.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketFiles))
		c := b.Cursor()
		prefix := []byte(p)
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 删除目录元数据
	return fs.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketDirs))
		return b.Delete([]byte(p))
	})
}

func (fs *BBolt) Rename(oldname, newname string) error {
	data, meta, err := fs.loadFile(oldname)
	if err != nil {
		return err
	}
	if err = fs.saveFile(newname, data, meta); err != nil {
		return err
	}
	return fs.Remove(oldname)
}

func (fs *BBolt) Stat(name string) (os.FileInfo, error) {
	_, meta, err := fs.loadFile(name)
	if err != nil {
		// 尝试作为目录
		var dmeta fileMeta
		errDir := fs.db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte(bucketDirs))
			val := b.Get([]byte(name))
			if val == nil {
				return ErrFileNotFound
			}
			dmeta = fs.decodeMeta(val)
			return nil
		})
		if errDir != nil {
			return nil, err
		}
		return &fileInfo{
			name:    filepath.Base(name),
			size:    0,
			mode:    dmeta.Mode,
			modTime: time.Unix(0, dmeta.ModTime),
			isDir:   true,
		}, nil
	}
	return &fileInfo{
		name:    filepath.Base(name),
		size:    meta.Size,
		mode:    meta.Mode,
		modTime: time.Unix(0, meta.ModTime),
		isDir:   meta.IsDir,
	}, nil
}

func (fs *BBolt) Name() string { return fs.name }

func (fs *BBolt) Chmod(name string, mode os.FileMode) error {
	data, meta, err := fs.loadFile(name)
	if err != nil {
		return err
	}
	meta.Mode = mode
	return fs.saveFile(name, data, meta)
}

func (fs *BBolt) Chown(name string, uid, gid int) error {
	// 不支持，忽略
	return nil
}

func (fs *BBolt) Chtimes(name string, atime, mtime time.Time) error {
	data, meta, err := fs.loadFile(name)
	if err != nil {
		return err
	}
	meta.ModTime = mtime.UnixNano()
	return fs.saveFile(name, data, meta)
}

func (fs *BBolt) Close() error {
	return fs.db.Close()
}

func (fs *BBolt) readDir(dir string, count int) ([]os.FileInfo, error) {
	var fis []os.FileInfo
	prefix := dir
	if !strings.HasSuffix(prefix, "/") && prefix != "" {
		prefix += "/"
	}
	err := fs.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketFiles))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if !strings.HasPrefix(string(k), prefix) {
				continue
			}
			rest := strings.TrimPrefix(string(k), prefix)
			if rest == "" || strings.Contains(rest, "/") {
				continue // 只返回当前目录下的
			}
			meta := fs.decodeMeta(v)
			fis = append(fis, &fileInfo{
				name:    rest,
				size:    meta.Size,
				mode:    meta.Mode,
				modTime: time.Unix(0, meta.ModTime),
				isDir:   meta.IsDir,
			})
			if count > 0 && len(fis) >= count {
				break
			}
		}
		return nil
	})
	return fis, err
}

func (fs *BBolt) encodeMeta(meta fileMeta) []byte {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, meta.Mode)
	_ = binary.Write(buf, binary.LittleEndian, meta.Size)
	_ = binary.Write(buf, binary.LittleEndian, meta.ModTime)
	_ = binary.Write(buf, binary.LittleEndian, meta.IsDir)
	return buf.Bytes()
}
func (fs *BBolt) decodeMeta(b []byte) fileMeta {
	buf := bytes.NewReader(b)
	var meta fileMeta
	_ = binary.Read(buf, binary.LittleEndian, &meta.Mode)
	_ = binary.Read(buf, binary.LittleEndian, &meta.Size)
	_ = binary.Read(buf, binary.LittleEndian, &meta.ModTime)
	_ = binary.Read(buf, binary.LittleEndian, &meta.IsDir)
	return meta
}
func (fs *BBolt) metaLen() int { return 4 + 8 + 8 + 1 }
