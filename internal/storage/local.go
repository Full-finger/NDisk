package storage

import (
	"io"
	"os"
	"path/filepath"
)

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

func (l *LocalStorage) Delete(key string) error {
	path := filepath.Join(l.BasePath, key)
	return os.Remove(path)
}

func (l *LocalStorage) Exists(key string) bool {
	path := filepath.Join(l.BasePath, key)
	_, err := os.Stat(path)
	return err == nil
}