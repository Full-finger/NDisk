package file

import (
	"time"

	"gorm.io/gorm"
)

type File struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"index"`    // 所属用户
	ParentID   *uint  `gorm:"index"`    // 父目录ID，nil表示根目录
	Name       string `gorm:"size:255"` // 原始文件名
	StorageKey string `gorm:"size:512"` // 存储层的key（路径或对象ID）
	Size       int64  // 文件大小
	IsDir      bool   // 是否为目录
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
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
