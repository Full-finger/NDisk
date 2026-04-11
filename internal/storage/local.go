package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type rangeReader struct {
	file   *os.File
	reader io.Reader
}

func (r *rangeReader) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

func (r *rangeReader) Close() error {
	return r.file.Close()
}

type LocalStorage struct {
	BasePath string
}

func NewLocal(basePath string) *LocalStorage {
	os.MkdirAll(basePath, 0755)
	return &LocalStorage{BasePath: basePath}
}

func (l *LocalStorage) safePath(key string) (string, error) {
	p := filepath.Join(l.BasePath, key)
	if !strings.HasPrefix(p, l.BasePath+string(filepath.Separator)) && p != l.BasePath {
		return "", fmt.Errorf("path traversal detected")
	}
	return p, nil
}

func (l *LocalStorage) Save(key string, reader io.Reader) error {
	path, err := l.safePath(key)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(path), 0755)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

func (l *LocalStorage) Open(key string) (io.ReadCloser, error) {
	path, err := l.safePath(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (l *LocalStorage) OpenRange(key string, offset, length int64) (io.ReadCloser, error) {
	path, err := l.safePath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return &rangeReader{file: f, reader: io.LimitReader(f, length)}, nil
}

func (l *LocalStorage) Delete(key string) error {
	path, err := l.safePath(key)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (l *LocalStorage) Exists(key string) bool {
	path, err := l.safePath(key)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func (l *LocalStorage) Rename(srcKey, dstKey string) error {
	src, err := l.safePath(srcKey)
	if err != nil {
		return err
	}
	dst, err := l.safePath(dstKey)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(dst), 0755)
	return os.Rename(src, dst)
}

// Append 向已有文件追加数据，如果文件不存在则创建
func (l *LocalStorage) Append(key string, reader io.Reader) error {
	path, err := l.safePath(key)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(path), 0755)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

// DeleteDir 删除整个目录及其内容
func (l *LocalStorage) DeleteDir(key string) error {
	path, err := l.safePath(key)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}
