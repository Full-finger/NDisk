package nfs

import (
	"fmt"
	"log"
	"net"

	"github.com/Full-finger/NDisk/internal/file"
	"github.com/Full-finger/NDisk/internal/storage"
	nfslib "github.com/willscott/go-nfs"
	"github.com/willscott/go-nfs/helpers"
)

// Server 管理 NFS 服务端生命周期
type Server struct {
	listener   net.Listener
	nfsHandler *NDiskNFSHandler
	port       string
}

// NewServer 创建 NFS 服务实例
func NewServer(fileService *file.Service, store storage.Storage, hmacSecret string, port string) *Server {
	handler := NewNDiskNFSHandler(fileService, store, hmacSecret)
	return &Server{
		nfsHandler: handler,
		port:       port,
	}
}

// Start 启动 NFS 服务
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("NFS listen failed: %w", err)
	}
	s.listener = listener

	// 使用 CachingHandler 包装以提高性能
	cachingHandler := helpers.NewCachingHandler(s.nfsHandler, 1024)

	go func() {
		log.Printf("NFS server starting on :%s", s.port)
		if err := nfslib.Serve(listener, cachingHandler); err != nil {
			log.Printf("NFS server stopped: %v", err)
		}
	}()

	return nil
}

// Stop 停止 NFS 服务
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
		log.Println("NFS server stopped")
	}
}

// Addr 返回 NFS 服务监听地址
func (s *Server) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}
