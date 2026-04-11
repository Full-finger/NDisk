package share

import (
	"time"

	"gorm.io/gorm"
)

// Share 分享记录
type Share struct {
	ID           uint   `gorm:"primaryKey"`
	ShareToken   string `gorm:"size:64;uniqueIndex"`
	UserID       uint   `gorm:"index"`
	ItemID       uint   `gorm:"index"`
	IsFolder     bool
	ProjectName  string     `gorm:"size:255"`
	PasswordHash string     `gorm:"size:255"` // bcrypt hash, 空表示无密码
	ExpiresAt    *time.Time // nil 表示永久有效
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// CreateShareRequest 创建分享请求
type CreateShareRequest struct {
	ItemID      int    `json:"item_id" binding:"required"`
	IsFolder    bool   `json:"is_folder"`
	ProjectName string `json:"project_name" binding:"required"`
	Password    string `json:"password"`
	ExpiresIn   string `json:"expires_in"` // 1d, 7d, 30d, never
}

// VerifyPasswordRequest 验证密码请求
type VerifyPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}
