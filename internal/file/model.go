package file

import (
	"net/http"
	"time"

	"gorm.io/gorm"
)

type File struct {
	ID          uint   `gorm:"primaryKey"`
	UserID      uint   `gorm:"index"`         // 所属用户
	ParentID    *uint  `gorm:"index"`         // 父目录ID，nil表示根目录
	Name        string `gorm:"size:255"`      // 原始文件名
	StorageKey  string `gorm:"size:512"`      // 存储层的key（路径或对象ID）
	ContentHash string `gorm:"size:64;index"` // 文件内容SHA-256哈希，用于去重
	Size        int64  // 文件大小
	IsDir       bool   // 是否为目录
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// LastModified 返回HTTP格式的修改时间
func (f *File) LastModified() string {
	return f.UpdatedAt.UTC().Format(http.TimeFormat)
}

// 上传请求
type UploadRequest struct {
	ParentID *uint `form:"parent_id"` // 上传到哪个目录
}

// 创建目录请求
type CreateFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID *uint  `json:"parent_id"` // nil表示根目录
}

// 重命名请求
type RenameRequest struct {
	Name string `json:"name" binding:"required"`
}

// 移动请求
type MoveRequest struct {
	TargetID *uint `json:"target_id"` // 目标文件夹ID，nil表示根目录
}

// DownloadLink 一次性下载短链接
type DownloadLink struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	FileID    uint   `gorm:"index"`
	IsFolder  bool   // 是否为文件夹下载
	Token     string `gorm:"size:64;uniqueIndex"`
	ExpiresAt time.Time
	CreatedAt time.Time
}
