package bboltfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustTmpFile(t *testing.T) string {
	t.Helper()
	tmp := filepath.Join(os.TempDir(), "bboltfs_test_"+time.Now().Format("20060102150405"))
	t.Cleanup(func() {
		os.Remove(tmp)
	})
	return tmp
}

func TestBBoltFs_Create_Write_Read(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	f, err := fs.Create("test.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := "hello, bboltfs!"
	n, err := f.WriteString(want)
	if err != nil || n != len(want) {
		t.Fatalf("WriteString = %v, %v", n, err)
	}
	f.Close()

	// Open and check contents
	f2, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f2.Close()
	buf := make([]byte, 100)
	n, err = f2.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	got := string(buf[:n])
	if got != want {
		t.Errorf("Read = %q, want %q", got, want)
	}
}

func TestBBoltFs_Mkdir_MkdirAll_Stat(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	if err := fs.Mkdir("dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := fs.MkdirAll("a/b/c", 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := fs.Stat("dir")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Stat.IsDir() = false, want true")
	}
	info, err = fs.Stat("a/b/c")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Stat.IsDir() = false, want true")
	}
}

func TestBBoltFs_Remove_RemoveAll(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	f, err := fs.Create("foo.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.Close()
	if err := fs.Remove("foo.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := fs.Open("foo.txt"); err == nil {
		t.Fatalf("Open after Remove should error")
	}

	_ = fs.MkdirAll("d1/d2", 0755)
	_, _ = fs.Create("d1/d2/f1.txt")
	_, _ = fs.Create("d1/d2/f2.txt")
	if err := fs.RemoveAll("d1"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := fs.Open("d1/d2/f1.txt"); err == nil {
		t.Errorf("file should be deleted")
	}
}

func TestBBoltFs_Rename(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	f, err := fs.Create("old.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.WriteString("data")
	f.Close()
	if err := fs.Rename("old.txt", "new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := fs.Open("old.txt"); err == nil {
		t.Errorf("old.txt should not exist")
	}
	f2, err := fs.Open("new.txt")
	if err != nil {
		t.Fatalf("Open new.txt: %v", err)
	}
	buf := make([]byte, 10)
	n, _ := f2.Read(buf)
	if string(buf[:n]) != "data" {
		t.Errorf("content mismatch after rename")
	}
}

func TestBBoltFs_Chmod_Chtimes(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	f, err := fs.Create("chmod.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.Close()
	if err := fs.Chmod("chmod.txt", 0400); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	info, _ := fs.Stat("chmod.txt")
	if info.Mode().Perm() != 0400 {
		t.Errorf("Chmod failed, got mode %v", info.Mode())
	}
	now := time.Now().Add(-time.Hour)
	if err := fs.Chtimes("chmod.txt", now, now); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	info, _ = fs.Stat("chmod.txt")
	if info.ModTime().Unix() != now.Unix() {
		t.Errorf("Chtimes failed, got %v, want %v", info.ModTime(), now)
	}
}

func TestBBoltFs_Readdir_Readdirnames(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	_ = fs.MkdirAll("dir/sub", 0755)
	fs.Create("dir/a.txt")
	fs.Create("dir/b.txt")
	fs.Create("dir/c.txt")

	f, err := fs.Open("dir")
	if err != nil {
		t.Fatalf("Open dir: %v", err)
	}
	defer f.Close()
	infos, err := f.Readdir(0)
	if err != nil {
		t.Fatalf("Readdir: %v", err)
	}
	var files []string
	for _, fi := range infos {
		files = append(files, fi.Name())
	}
	if len(files) == 0 {
		t.Fatalf("Readdir should return files, got none")
	}

	names, err := f.Readdirnames(0)
	if err != nil {
		t.Fatalf("Readdirnames: %v", err)
	}
	if len(names) == 0 {
		t.Fatalf("Readdirnames got none")
	}
}

func TestBBoltFs_Truncate(t *testing.T) {
	dbfile := mustTmpFile(t)
	fs, err := New(dbfile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	f, err := fs.Create("trunc.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.WriteString("1234567890")
	if err := f.Truncate(5); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	f.Close()
	f2, _ := fs.Open("trunc.txt")
	buf := make([]byte, 10)
	n, _ := f2.Read(buf)
	if string(buf[:n]) != "12345" {
		t.Errorf("Truncate failed, got %q", string(buf[:n]))
	}
}
