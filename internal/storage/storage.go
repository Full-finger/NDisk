package storage

import "io"

// Storage 存储抽象接口
type Storage interface {
	Save(key string, reader io.Reader) error
	Open(key string) (io.ReadCloser, error)
	Delete(key string) error
	Exists(key string) bool
}