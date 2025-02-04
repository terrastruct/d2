// memfs implements an in-memory fs.FS implementation
// This is useful in for running d2 in javascript environments where native file calls are not available
package memfs

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"time"
)

type MemoryFile struct {
	name    string
	content []byte
	modTime time.Time
	isDir   bool
}

type MemoryFS struct {
	files map[string]*MemoryFile
}

func New(m map[string]string) (*MemoryFS, error) {
	memFS := &MemoryFS{files: make(map[string]*MemoryFile)}

	for p, s := range m {
		p = filepath.Clean(p)
		dirPath := path.Dir(p)
		memFS.addFile(dirPath, nil, true)
		memFS.addFile(p, []byte(s), false)
	}
	return memFS, nil
}

func (mfs *MemoryFS) addFile(p string, content []byte, isDir bool) {
	mfs.files[p] = &MemoryFile{
		name:    filepath.Base(p),
		content: content,
		modTime: time.Now(),
		isDir:   isDir,
	}
}

type MemoryFileHandle struct {
	*MemoryFile
	offset int
}

func (mfs *MemoryFS) Open(name string) (fs.File, error) {
	file, ok := mfs.files[filepath.Clean(name)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &MemoryFileHandle{MemoryFile: file}, nil
}

func (fh *MemoryFileHandle) Stat() (fs.FileInfo, error) { return fh.MemoryFile, nil }

func (fh *MemoryFileHandle) Read(b []byte) (int, error) {
	if fh.isDir {
		return 0, errors.New("cannot read a directory")
	}
	if fh.offset >= len(fh.content) {
		return 0, io.EOF
	}
	n := copy(b, fh.content[fh.offset:])
	fh.offset += n
	return n, nil
}

func (fh *MemoryFileHandle) Close() error { return nil }

func (mf *MemoryFile) Stat() (fs.FileInfo, error) { return mf, nil }
func (mf *MemoryFile) Read(b []byte) (int, error) {
	if mf.isDir {
		return 0, errors.New("cannot read a directory")
	}
	copy(b, mf.content)
	return len(mf.content), nil
}
func (mf *MemoryFile) Close() error { return nil }

func (mf *MemoryFile) Name() string       { return mf.name }
func (mf *MemoryFile) Size() int64        { return int64(len(mf.content)) }
func (mf *MemoryFile) Mode() fs.FileMode  { return 0644 }
func (mf *MemoryFile) ModTime() time.Time { return mf.modTime }
func (mf *MemoryFile) IsDir() bool        { return mf.isDir }
func (mf *MemoryFile) Sys() interface{}   { return nil }
