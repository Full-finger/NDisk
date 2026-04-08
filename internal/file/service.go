package file

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
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
	// 安全处理文件名：只取基本文件名，去除路径部分
	safeFilename := filepath.Base(filename)

	// 进一步清理：移除可能导致问题的字符
	safeFilename = strings.ReplaceAll(safeFilename, "..", "")
	safeFilename = strings.TrimSpace(safeFilename)

	// 验证文件名
	if err := validateName(safeFilename); err != nil {
		return nil, err
	}

	// 防止空文件名
	if safeFilename == "" || safeFilename == "." || safeFilename == "/" {
		return nil, fmt.Errorf("无效的文件名")
	}

	// 生成存储key：用户ID/时间戳_uuid/文件名
	storageKey := fmt.Sprintf("%d/%d_%s/%s", userID, time.Now().Unix(), uuid.New().String()[:8], safeFilename)

	// 存储文件
	if err := s.storage.Save(storageKey, reader); err != nil {
		return nil, err
	}

	// 保存元数据
	file := &File{
		UserID:     userID,
		ParentID:   parentID,
		Name:       safeFilename,
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

// DownloadRange 范围下载
func (s *Service) DownloadRange(userID uint, fileID uint, offset, length int64) (*File, io.ReadCloser, error) {
	var file File
	if err := s.db.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		return nil, nil, err
	}
	if file.IsDir {
		return nil, nil, fmt.Errorf("cannot download directory")
	}

	reader, err := s.storage.OpenRange(file.StorageKey, offset, length)
	if err != nil {
		return nil, nil, err
	}

	return &file, reader, nil
}

// GetFile 获取文件信息（不打开文件）
func (s *Service) GetFile(userID uint, fileID uint) (*File, error) {
	var file File
	if err := s.db.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

// ETag 生成文件ETag
func (f *File) ETag() string {
	return fmt.Sprintf(`"%x-%d"`, f.ID, f.Size)
}

// BreadcrumbItem 面包屑导航项
type BreadcrumbItem struct {
	ID   uint
	Name string
}

// GetBreadcrumb 获取当前文件夹的面包屑路径（从根目录到当前目录）
func (s *Service) GetBreadcrumb(userID uint, folderID *uint) ([]BreadcrumbItem, error) {
	if folderID == nil {
		return []BreadcrumbItem{}, nil
	}

	var path []BreadcrumbItem
	currentID := *folderID
	visited := make(map[uint]bool) // 防止循环引用

	for currentID != 0 {
		if visited[currentID] {
			break // 检测到循环，退出
		}
		visited[currentID] = true

		var folder File
		if err := s.db.Where("id = ? AND user_id = ? AND is_dir = ?", currentID, userID, true).First(&folder).Error; err != nil {
			break // 找不到文件夹，退出
		}

		// 将当前文件夹添加到路径开头
		path = append([]BreadcrumbItem{{ID: folder.ID, Name: folder.Name}}, path...)

		if folder.ParentID == nil {
			break
		}
		currentID = *folder.ParentID
	}

	return path, nil
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

// 重命名文件或文件夹
func (s *Service) Rename(userID uint, fileID uint, newName string) (*File, error) {
	var file File
	if err := s.db.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		return nil, err
	}

	// 验证新名称
	if err := validateName(newName); err != nil {
		return nil, err
	}

	// 检查同一目录下是否存在同名文件/文件夹
	query := s.db.Where("user_id = ? AND name = ? AND id != ?", userID, newName, fileID)
	if file.ParentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *file.ParentID)
	}

	var existing File
	if err := query.First(&existing).Error; err == nil {
		return nil, fmt.Errorf("同名文件或文件夹已存在")
	}

	// 更新名称
	file.Name = newName
	if err := s.db.Save(&file).Error; err != nil {
		return nil, err
	}

	return &file, nil
}

// validateName 验证文件名是否符合规则
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("文件名不能为空")
	}

	// 检查是否只包含空格或点
	onlySpaceOrDot := true
	for _, r := range name {
		if r != ' ' && r != '.' {
			onlySpaceOrDot = false
			break
		}
	}
	if onlySpaceOrDot {
		return fmt.Errorf("文件名不能只包含空格或点")
	}

	// 检查禁止字符
	forbiddenChars := []rune{'\\', '/', ':', '*', '?', '"', '<', '>', '|'}
	for _, r := range name {
		for _, f := range forbiddenChars {
			if r == f {
				return fmt.Errorf("文件名不能包含以下字符: \\ / : * ? \" < > |")
			}
		}
	}

	// 检查名称长度
	if len(name) > 255 {
		return fmt.Errorf("文件名过长，最大255个字符")
	}

	return nil
}
