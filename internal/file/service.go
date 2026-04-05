package file

import (
	"fmt"
	"io"
	"time"

	"github.com/Full-finger/NDisk/internal/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	storage storage.Storage
}

func NewService(db *gorm.DB, storage storage.Storage) *Service {
	return &Service{
		db:      db,
		storage: storage,
	}
}

// 上传文件
func (s *Service) Upload(userID uint, parentID *uint, filename string, size int64, reader io.Reader) (*File, error) {
	// 生成存储key：用户ID/时间戳_uuid/文件名
	storageKey := fmt.Sprintf("%d/%d_%s/%s", userID, time.Now().Unix(), uuid.New().String()[:8], filename)

	// 存储文件
	if err := s.storage.Save(storageKey, reader); err != nil {
		return nil, err
	}

	// 保存元数据
	file := &File{
		UserID:     userID,
		ParentID:   parentID,
		Name:       filename,
		StorageKey: storageKey,
		Size:       size,
		IsDir:      false,
	}
	if err := s.db.Create(file).Error; err != nil {
		// 回滚：删除已存文件
		s.storage.Delete(storageKey)
		return nil, err
	}

	return file, nil
}

// 创建文件夹
func (s *Service) CreateFolder(userID uint, name string, parentID *uint) (*File, error) {
	folder := &File{
		UserID:   userID,
		ParentID: parentID,
		Name:     name,
		IsDir:    true,
		Size:     0,
	}
	if err := s.db.Create(folder).Error; err != nil {
		return nil, err
	}
	return folder, nil
}

// 获取文件列表
func (s *Service) List(userID uint, parentID *uint) ([]File, error) {
	var files []File
	query := s.db.Where("user_id = ? AND is_dir = ?", userID, false)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.Find(&files).Error
	return files, err
}

// 获取文件夹列表（单独查目录）
func (s *Service) ListFolders(userID uint, parentID *uint) ([]File, error) {
	var folders []File
	query := s.db.Where("user_id = ? AND is_dir = ?", userID, true)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.Find(&folders).Error
	return folders, err
}

// 下载文件
func (s *Service) Download(userID uint, fileID uint) (*File, io.ReadCloser, error) {
	var file File
	if err := s.db.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		return nil, nil, err
	}
	if file.IsDir {
		return nil, nil, fmt.Errorf("cannot download directory")
	}

	reader, err := s.storage.Open(file.StorageKey)
	if err != nil {
		return nil, nil, err
	}

	return &file, reader, nil
}

// 删除文件
func (s *Service) Delete(userID uint, fileID uint) error {
	var file File
	if err := s.db.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		return err
	}

	// 删除物理文件（如果不是目录）
	if !file.IsDir {
		s.storage.Delete(file.StorageKey)
	}

	return s.db.Delete(&file).Error
}
