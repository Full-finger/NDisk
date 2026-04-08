package storage

import (
	"io"
	"os"
	"path/filepath"
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

func (l *LocalStorage) Save(key string, reader io.Reader) error {
	path := filepath.Join(l.BasePath, key)
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
	path := filepath.Join(l.BasePath, key)
	return os.Open(path)
}

func (l *LocalStorage) OpenRange(key string, offset, length int64) (io.ReadCloser, error) {
	path := filepath.Join(l.BasePath, key)
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
	path := filepath.Join(l.BasePath, key)
	return os.Remove(path)
}

func (l *LocalStorage) Exists(key string) bool {
	path := filepath.Join(l.BasePath, key)
	_, err := os.Stat(path)
	return err == nil
}
