package nfs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Full-finger/NDisk/internal/file"
	"github.com/Full-finger/NDisk/internal/storage"
	billy "github.com/go-git/go-billy/v5"
	nfs "github.com/willscott/go-nfs"
)

// NDiskNFSHandler 实现 nfs.Handler 接口
type NDiskNFSHandler struct {
	fileService *file.Service
	store       storage.Storage
	hmacSecret  string

	// 已验证的文件系统缓存: token -> *NDiskFS
	fsCache map[string]*NDiskFS
}

// NewNDiskNFSHandler 创建 NFS Handler
func NewNDiskNFSHandler(fileService *file.Service, store storage.Storage, hmacSecret string) *NDiskNFSHandler {
	return &NDiskNFSHandler{
		fileService: fileService,
		store:       store,
		hmacSecret:  hmacSecret,
		fsCache:     make(map[string]*NDiskFS),
	}
}

// Mount 处理 NFS 挂载请求，提取路径中的 Token 进行认证
func (h *NDiskNFSHandler) Mount(ctx context.Context, conn net.Conn, req nfs.MountRequest) (nfs.MountStatus, billy.Filesystem, []nfs.AuthFlavor) {
	// 从挂载路径中提取 Token
	mountPath := string(req.Dirpath)
	mountPath = strings.TrimPrefix(mountPath, "/")
	mountPath = strings.TrimSuffix(mountPath, "/")

	// 支持格式: /token_xxxxx 或直接 token_xxxxx
	token := strings.TrimPrefix(mountPath, "token_")
	if token == "" || token == mountPath {
		log.Printf("NFS Mount: invalid mount path: %s", mountPath)
		return nfs.MountStatusErrAcces, nil, nil
	}

	// 查询数据库验证 Token
	var nfsToken NFSToken
	if err := h.fileService.DB().Where("token = ?", token).First(&nfsToken).Error; err != nil {
		log.Printf("NFS Mount: token not found: %s", token)
		return nfs.MountStatusErrAcces, nil, nil
	}

	// 检查是否过期
	if nfsToken.ExpiresAt != nil && nfsToken.ExpiresAt.Before(time.Now()) {
		log.Printf("NFS Mount: token expired: %s", token)
		return nfs.MountStatusErrAcces, nil, nil
	}

	// 获取或创建文件系统
	fs, ok := h.fsCache[token]
	if !ok {
		fs = NewNDiskFS(nfsToken.UserID, h.fileService, h.store, h.hmacSecret)
		h.fsCache[token] = fs
	}

	log.Printf("NFS Mount: user %d mounted via token %s... from %s", nfsToken.UserID, token[:8], conn.RemoteAddr())
	return nfs.MountStatusOk, fs, []nfs.AuthFlavor{nfs.AuthFlavorNull}
}

// Change 提供文件属性修改接口
func (h *NDiskNFSHandler) Change(fs billy.Filesystem) billy.Change {
	return nil // 暂不支持属性修改
}

// FSStat 提供文件系统统计信息
func (h *NDiskNFSHandler) FSStat(ctx context.Context, fs billy.Filesystem, stat *nfs.FSStat) error {
	// 返回默认值
	stat.TotalSize = 1 << 40 // 1TB
	stat.FreeSize = 1 << 39  // 512GB
	stat.AvailableSize = 1 << 39
	stat.TotalFiles = 1 << 20
	stat.FreeFiles = 1 << 19
	stat.AvailableFiles = 1 << 19
	return nil
}

// ToHandle 将文件系统路径转换为文件句柄
func (h *NDiskNFSHandler) ToHandle(fs billy.Filesystem, path []string) []byte {
	ndiskFS, ok := fs.(*NDiskFS)
	if !ok {
		return nil
	}

	// 路径为根目录
	if len(path) == 0 || (len(path) == 1 && path[0] == "") {
		return h.encodeHandle(ndiskFS.userID, 0)
	}

	// 解析路径获取文件 ID
	fullPath := strings.Join(path, "/")
	f, err := ndiskFS.resolvePath(fullPath)
	if err != nil {
		// 无法解析，返回根目录句柄
		return h.encodeHandle(ndiskFS.userID, 0)
	}

	return h.encodeHandle(ndiskFS.userID, f.ID)
}

// FromHandle 将文件句柄转换回文件系统路径
func (h *NDiskNFSHandler) FromHandle(fh []byte) (billy.Filesystem, []string, error) {
	userID, fileID, err := h.decodeHandle(fh)
	if err != nil {
		return nil, nil, err
	}

	// 查找对应的文件系统
	var ndiskFS *NDiskFS
	for _, fs := range h.fsCache {
		if fs.userID == userID {
			ndiskFS = fs
			break
		}
	}
	if ndiskFS == nil {
		// 创建临时文件系统
		ndiskFS = NewNDiskFS(userID, h.fileService, h.store, h.hmacSecret)
	}

	// 根目录
	if fileID == 0 {
		return ndiskFS, []string{}, nil
	}

	// 构建路径
	path, err := h.buildPath(ndiskFS, fileID)
	if err != nil {
		return nil, nil, err
	}

	return ndiskFS, path, nil
}

// InvalidateHandle 使文件句柄缓存失效
func (h *NDiskNFSHandler) InvalidateHandle(fs billy.Filesystem, fh []byte) error {
	return nil
}

// HandleLimit 返回句柄缓存限制
func (h *NDiskNFSHandler) HandleLimit() int {
	return 1024
}

// encodeHandle 编码文件句柄: UserID(4) + FileID(4) + HMAC(32) + Padding(24) = 64 bytes
func (h *NDiskNFSHandler) encodeHandle(userID uint, fileID uint) []byte {
	buf := make([]byte, nfs.FHSize)
	binary.BigEndian.PutUint32(buf[0:4], uint32(userID))
	binary.BigEndian.PutUint32(buf[4:8], uint32(fileID))

	// 计算 HMAC
	mac := hmac.New(sha256.New, []byte(h.hmacSecret))
	mac.Write(buf[0:8])
	copy(buf[8:40], mac.Sum(nil))

	return buf
}

// decodeHandle 解码文件句柄
func (h *NDiskNFSHandler) decodeHandle(fh []byte) (uint, uint, error) {
	if len(fh) < 40 {
		return 0, 0, fmt.Errorf("handle too short")
	}

	// 验证 HMAC
	mac := hmac.New(sha256.New, []byte(h.hmacSecret))
	mac.Write(fh[0:8])
	expectedMAC := mac.Sum(nil)

	if !hmac.Equal(fh[8:40], expectedMAC) {
		return 0, 0, fmt.Errorf("invalid handle signature")
	}

	userID := binary.BigEndian.Uint32(fh[0:4])
	fileID := binary.BigEndian.Uint32(fh[4:8])

	return uint(userID), uint(fileID), nil
}

// buildPath 根据文件 ID 构建从根目录到文件的路径
func (h *NDiskNFSHandler) buildPath(fs *NDiskFS, fileID uint) ([]string, error) {
	if fileID == 0 {
		return []string{}, nil
	}

	var parts []string
	currentID := fileID
	visited := make(map[uint]bool)

	for currentID != 0 {
		if visited[currentID] {
			break
		}
		visited[currentID] = true

		var f file.File
		if err := h.fileService.DB().Where("id = ? AND user_id = ?", currentID, fs.userID).First(&f).Error; err != nil {
			break
		}

		parts = append([]string{f.Name}, parts...)

		if f.ParentID == nil {
			break
		}
		currentID = *f.ParentID
	}

	return parts, nil
}
