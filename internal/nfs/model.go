package nfs

import (
	"time"
)

// NFSToken NFS 挂载令牌，用于将 NFS 挂载映射到 NDisk 用户
type NFSToken struct {
	ID          uint   `gorm:"primaryKey"`
	UserID      uint   `gorm:"index"`
	Token       string `gorm:"size:64;uniqueIndex"`
	Description string `gorm:"size:255"`
	CreatedAt   time.Time
	ExpiresAt   *time.Time // nil 表示永不过期
}

// CreateNFSTokenRequest 创建 NFS Token 请求
type CreateNFSTokenRequest struct {
	Description string `json:"description"`
}
