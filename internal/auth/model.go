package auth

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID            uint   `gorm:"primaryKey"`
	Username      string `gorm:"uniqueIndex;size:64"`
	Nickname      string `gorm:"size:64"`
	Password      string `gorm:"size:255"` // bcrypt hash
	WallpaperURL  string `gorm:"size:512"`
	WallpaperBlur int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

// 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

// 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshToken 刷新令牌（存储在数据库中）
type RefreshToken struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	Token     string `gorm:"size:64;uniqueIndex"` // SHA256 哈希
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
}

// 登录响应
type LoginResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"-"` // 仅内部使用，通过 cookie 设置
	User         UserInfo `json:"user"`
}

type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// UpdateProfileRequest 更新个人资料请求
type UpdateProfileRequest struct {
	Nickname string `json:"nickname" binding:"required,min=1,max=64"`
}

// UpdateWallpaperRequest 更新壁纸设置请求
type UpdateWallpaperRequest struct {
	WallpaperURL  string `json:"wallpaper_url"`
	WallpaperBlur int    `json:"wallpaper_blur"`
}
