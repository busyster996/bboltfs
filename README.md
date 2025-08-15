# bboltfs

bboltfs is a Go package that provides a virtual filesystem (FS) interface backed by the [bbolt](https://github.com/etcd-io/bbolt) embedded key/value database. It allows you to store and manage files and directories inside a single bbolt database file, enabling persistent storage and easy distribution of file data.

## Features

- Implements a filesystem-like interface on top of bbolt
- Stores files and directories inside a single bbolt database file
- Suitable for embedding resources, configuration files, or static assets in Go applications
- Lightweight and dependency-free (other than bbolt)

## Installation

```bash
go get github.com/busyster996/bboltfs
```

## Usage

```go
import (
    "log"
    "os"

    "github.com/busyster996/bboltfs"
)

func main() {
    // Open or create the bbolt database file
    dbFile := "mydata.db"
    fs, err := bboltfs.Open(dbFile, 0600, nil)
    if err != nil {
        log.Fatalf("Failed to open bboltfs: %v", err)
    }
    defer fs.Close()

    // Create a new file
    f, err := fs.Create("/hello.txt")
    if err != nil {
        log.Fatalf("Failed to create file: %v", err)
    }
    f.Write([]byte("Hello, bboltfs!"))
    f.Close()

    // Read the file
    rf, err := fs.Open("/hello.txt")
    if err != nil {
        log.Fatalf("Failed to open file: %v", err)
    }
    content := make([]byte, 100)
    n, _ := rf.Read(content)
    log.Printf("File content: %s", content[:n])
    rf.Close()
}
```

## API

bboltfs aims to provide an interface similar to Go's `io/fs` package, supporting common filesystem operations:

- `Open(name string) (File, error)`
- `Create(name string) (File, error)`
- `Remove(name string) error`
- `Mkdir(name string, perm os.FileMode) error`
- `ReadDir(name string) ([]DirEntry, error)`

Refer to the GoDoc for detailed API documentation.

## When to Use

- Embedding static assets or configuration files in Go binaries
- Building persistent, single-file applications with easy resource management
- Distributing applications or tools with internal filesystem requirements

## License

This project is licensed under the MIT License.

## Acknowledgements

- [bbolt](https://github.com/etcd-io/bbolt) for the underlying key/value store.

---

Feel free to open issues or pull requests for bugs, features, or questions!