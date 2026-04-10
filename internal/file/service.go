package file

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Full-finger/NDisk/internal/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	storage storage.Storage
	zipDir  string // ZIP 缓存目录
}

func NewService(db *gorm.DB, storageObj storage.Storage) *Service {
	return &Service{
		db:      db,
		storage: storageObj,
	}
}

// SetZipDir 设置 ZIP 缓存目录
func (s *Service) SetZipDir(dir string) {
	s.zipDir = dir
	os.MkdirAll(dir, 0755)
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

	// 使相关 ZIP 缓存失效
	s.invalidateCacheForItem(userID, &file)

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

	// 使相关 ZIP 缓存失效
	s.invalidateCacheForItem(userID, &file)

	return &file, nil
}

// chunkKey 返回 chunk 临时文件的存储 key
func chunkKey(identifier string, chunkNumber int) string {
	return fmt.Sprintf(".chunks/%s/%d", identifier, chunkNumber)
}

// chunkCompleteMarker 返回上传完成的标记文件 key
func chunkCompleteMarker(identifier string) string {
	return fmt.Sprintf(".chunks/%s/.complete", identifier)
}

// SaveChunk 保存单个分块到临时存储
func (s *Service) SaveChunk(identifier string, chunkNumber int, reader io.Reader) error {
	key := chunkKey(identifier, chunkNumber)
	return s.storage.Save(key, reader)
}

// ChunkExists 检查分块是否已上传
func (s *Service) ChunkExists(identifier string, chunkNumber int) bool {
	// 先检查完成标记，如果文件已合并完成，所有 chunk 都视为已存在
	if s.storage.Exists(chunkCompleteMarker(identifier)) {
		return true
	}
	return s.storage.Exists(chunkKey(identifier, chunkNumber))
}

// AllChunksUploaded 检查所有分块是否都已上传
func (s *Service) AllChunksUploaded(identifier string, totalChunks int) bool {
	for i := 1; i <= totalChunks; i++ {
		if !s.storage.Exists(chunkKey(identifier, i)) {
			return false
		}
	}
	return true
}

// UploadFromChunks 合并所有分块并创建文件记录
func (s *Service) UploadFromChunks(userID uint, parentID *uint, filename string, totalSize int64, identifier string, totalChunks int) (*File, error) {
	// 按顺序打开所有分块
	var readers []io.Reader
	var closers []io.ReadCloser

	for i := 1; i <= totalChunks; i++ {
		key := chunkKey(identifier, i)
		reader, err := s.storage.Open(key)
		if err != nil {
			// 清理已打开的 reader
			for _, c := range closers {
				c.Close()
			}
			return nil, fmt.Errorf("分块 %d 未找到: %v", i, err)
		}
		readers = append(readers, reader)
		closers = append(closers, reader)
	}

	// 合并所有分块为一个 reader
	combined := io.MultiReader(readers...)

	// 调用已有的 Upload 方法完成最终存储和数据库记录
	result, err := s.Upload(userID, parentID, filename, totalSize, combined)

	// 关闭所有分块 reader
	for _, c := range closers {
		c.Close()
	}

	if err != nil {
		return nil, err
	}

	// 创建完成标记（在清理分块之前，防止并发问题）
	s.storage.Save(chunkCompleteMarker(identifier), strings.NewReader(""))

	// 清理临时分块文件
	for i := 1; i <= totalChunks; i++ {
		s.storage.Delete(chunkKey(identifier, i))
	}

	// 使父目录的 ZIP 缓存失效
	if parentID != nil {
		s.InvalidateFolderCache(*parentID)
	}

	return result, nil
}

// ListAllFolders 获取用户所有文件夹（用于移动目标选择等场景）
func (s *Service) ListAllFolders(userID uint) ([]File, error) {
	var folders []File
	err := s.db.Where("user_id = ? AND is_dir = ?", userID, true).Find(&folders).Error
	return folders, err
}

// Move 移动文件或文件夹到目标目录
func (s *Service) Move(userID uint, itemID uint, targetID *uint) (*File, error) {
	// 查找源项目
	var item File
	if err := s.db.Where("id = ? AND user_id = ?", itemID, userID).First(&item).Error; err != nil {
		return nil, fmt.Errorf("项目不存在")
	}

	// 验证目标文件夹
	if targetID != nil {
		var target File
		if err := s.db.Where("id = ? AND user_id = ? AND is_dir = ?", *targetID, userID, true).First(&target).Error; err != nil {
			return nil, fmt.Errorf("目标文件夹不存在")
		}

		// 不能移动到自身
		if *targetID == itemID {
			return nil, fmt.Errorf("不能移动到自身")
		}

		// 循环引用检查：如果移动的是文件夹，目标不能是源的后代
		if item.IsDir {
			if err := s.checkNotDescendant(itemID, *targetID, userID); err != nil {
				return nil, err
			}
		}
	}

	// 检查目标目录下是否已存在同名项
	query := s.db.Where("user_id = ? AND name = ? AND id != ?", userID, item.Name, itemID)
	if targetID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *targetID)
	}
	var existing File
	if err := query.First(&existing).Error; err == nil {
		return nil, fmt.Errorf("目标目录下已存在同名项目")
	}

	// 保存旧的 parentID 用于缓存失效
	oldParentID := item.ParentID

	// 执行移动：只更新 parent_id
	item.ParentID = targetID
	if err := s.db.Save(&item).Error; err != nil {
		return nil, err
	}

	// 使源目录和目标目录的 ZIP 缓存失效
	if oldParentID != nil {
		s.InvalidateFolderCache(*oldParentID)
	} else {
		s.invalidateRootCache(userID)
	}
	if targetID != nil {
		s.InvalidateFolderCache(*targetID)
	} else {
		s.invalidateRootCache(userID)
	}

	return &item, nil
}

// checkNotDescendant 检查 targetID 不是 itemID 的后代（防止循环引用）
func (s *Service) checkNotDescendant(itemID uint, targetID uint, userID uint) error {
	currentID := targetID
	visited := make(map[uint]bool)
	for currentID != 0 {
		if currentID == itemID {
			return fmt.Errorf("不能移动到自身的子目录中")
		}
		if visited[currentID] {
			break
		}
		visited[currentID] = true

		var folder File
		if err := s.db.Where("id = ? AND user_id = ?", currentID, userID).First(&folder).Error; err != nil {
			break
		}
		if folder.ParentID == nil {
			break
		}
		currentID = *folder.ParentID
	}
	return nil
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

// ==================== 文件夹下载（ZIP）相关方法 ====================

// folderTreeItem 表示文件夹树中的一个文件条目
type folderTreeItem struct {
	RelPath string // 相对于根文件夹的路径
	File    File   // 文件元数据
}

// getFolderTree 递归获取文件夹下所有文件（不包含子文件夹本身，只包含文件）
func (s *Service) getFolderTree(userID uint, folderID uint, basePath string) ([]folderTreeItem, error) {
	var items []folderTreeItem

	// 获取该文件夹下的所有文件
	var files []File
	if err := s.db.Where("user_id = ? AND parent_id = ? AND is_dir = ?", userID, folderID, false).Find(&files).Error; err != nil {
		return nil, err
	}
	for _, f := range files {
		items = append(items, folderTreeItem{
			RelPath: filepath.Join(basePath, f.Name),
			File:    f,
		})
	}

	// 递归获取子文件夹
	var subFolders []File
	if err := s.db.Where("user_id = ? AND parent_id = ? AND is_dir = ?", userID, folderID, true).Find(&subFolders).Error; err != nil {
		return nil, err
	}
	for _, sf := range subFolders {
		subItems, err := s.getFolderTree(userID, sf.ID, filepath.Join(basePath, sf.Name))
		if err != nil {
			return nil, err
		}
		items = append(items, subItems...)
	}

	return items, nil
}

// computeContentHash 计算文件夹内容的哈希值，用于缓存失效检测
func (s *Service) computeContentHash(userID uint, folderID uint) (string, error) {
	items, err := s.getFolderTree(userID, folderID, "")
	if err != nil {
		return "", err
	}

	h := sha1.New()
	// 按 RelPath 排序以确保确定性
	sort.Slice(items, func(i, j int) bool {
		return items[i].RelPath < items[j].RelPath
	})

	for _, item := range items {
		fmt.Fprintf(h, "%s:%d:%d:%d", item.RelPath, item.File.ID, item.File.Size, item.File.UpdatedAt.UnixNano())
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// zipCachePath 返回 ZIP 缓存文件路径
func (s *Service) zipCachePath(folderID uint, contentHash string) string {
	return filepath.Join(s.zipDir, fmt.Sprintf("%d_%s.zip", folderID, contentHash[:12]))
}

// findCachedZip 查找文件夹的缓存 ZIP 文件（匹配前缀），返回文件路径或空字符串
func (s *Service) findCachedZip(folderID uint) string {
	if s.zipDir == "" {
		return ""
	}
	pattern := filepath.Join(s.zipDir, fmt.Sprintf("%d_*.zip", folderID))
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return ""
	}
	// 返回最新的匹配
	latest := matches[0]
	for _, m := range matches[1:] {
		info1, _ := os.Stat(m)
		info2, _ := os.Stat(latest)
		if info1 != nil && info2 != nil && info1.ModTime().After(info2.ModTime()) {
			latest = m
		}
	}
	return latest
}

// GenerateFolderZip 生成文件夹的 ZIP 压缩包（带缓存）
// 返回值：ZIP 文件路径、文件夹信息、是否来自缓存
func (s *Service) GenerateFolderZip(userID uint, folderID uint) (string, *File, bool, error) {
	// 获取文件夹信息
	var folder File
	if err := s.db.Where("id = ? AND user_id = ? AND is_dir = ?", folderID, userID, true).First(&folder).Error; err != nil {
		return "", nil, false, fmt.Errorf("文件夹不存在")
	}

	// 计算内容哈希
	contentHash, err := s.computeContentHash(userID, folderID)
	if err != nil {
		return "", nil, false, fmt.Errorf("计算内容哈希失败: %v", err)
	}

	// 检查缓存
	expectedPath := s.zipCachePath(folderID, contentHash)
	if _, err := os.Stat(expectedPath); err == nil {
		// 缓存命中
		return expectedPath, &folder, true, nil
	}

	// 缓存未命中，生成新的 ZIP
	// 获取文件夹树
	items, err := s.getFolderTree(userID, folderID, "")
	if err != nil {
		return "", nil, false, fmt.Errorf("获取文件夹内容失败: %v", err)
	}

	if len(items) == 0 {
		return "", nil, false, fmt.Errorf("文件夹为空")
	}

	// 创建临时 ZIP 文件
	tmpPath := expectedPath + ".tmp"
	zipFile, err := os.Create(tmpPath)
	if err != nil {
		return "", nil, false, fmt.Errorf("创建临时文件失败: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	for _, item := range items {
		// 打开源文件
		reader, err := s.storage.Open(item.File.StorageKey)
		if err != nil {
			zipWriter.Close()
			zipFile.Close()
			os.Remove(tmpPath)
			return "", nil, false, fmt.Errorf("打开文件 %s 失败: %v", item.RelPath, err)
		}

		// 创建 ZIP 条目
		w, err := zipWriter.Create(item.RelPath)
		if err != nil {
			reader.Close()
			zipWriter.Close()
			zipFile.Close()
			os.Remove(tmpPath)
			return "", nil, false, fmt.Errorf("创建 ZIP 条目失败: %v", err)
		}

		_, err = io.Copy(w, reader)
		reader.Close()
		if err != nil {
			zipWriter.Close()
			zipFile.Close()
			os.Remove(tmpPath)
			return "", nil, false, fmt.Errorf("写入文件 %s 到 ZIP 失败: %v", item.RelPath, err)
		}
	}

	// 完成写入
	if err := zipWriter.Close(); err != nil {
		zipFile.Close()
		os.Remove(tmpPath)
		return "", nil, false, fmt.Errorf("关闭 ZIP 写入器失败: %v", err)
	}
	zipFile.Close()

	// 删除旧缓存文件
	s.cleanFolderCache(folderID)

	// 重命名临时文件为正式文件
	if err := os.Rename(tmpPath, expectedPath); err != nil {
		os.Remove(tmpPath)
		return "", nil, false, fmt.Errorf("重命名临时文件失败: %v", err)
	}

	return expectedPath, &folder, false, nil
}

// GetFolderZipInfo 获取 ZIP 缓存文件信息（用于 HEAD 请求）
func (s *Service) GetFolderZipInfo(userID uint, folderID uint) (*File, int64, error) {
	// 获取文件夹信息
	var folder File
	if err := s.db.Where("id = ? AND user_id = ? AND is_dir = ?", folderID, userID, true).First(&folder).Error; err != nil {
		return nil, 0, fmt.Errorf("文件夹不存在")
	}

	// 查找缓存
	cachedPath := s.findCachedZip(folderID)
	if cachedPath == "" {
		return &folder, 0, nil
	}

	info, err := os.Stat(cachedPath)
	if err != nil {
		return &folder, 0, nil
	}

	return &folder, info.Size(), nil
}

// OpenZipCache 打开 ZIP 缓存文件
func (s *Service) OpenZipCache(path string) (*os.File, error) {
	return os.Open(path)
}

// InvalidateFolderCache 使指定文件夹的 ZIP 缓存失效
func (s *Service) InvalidateFolderCache(folderID uint) {
	s.cleanFolderCache(folderID)
}

// invalidateRootCache 使根目录的缓存失效（通过遍历所有 parent_id IS NULL 的文件夹）
func (s *Service) invalidateRootCache(userID uint) {
	// 根目录不生成 ZIP，不需要处理
}

// invalidateCacheForItem 使与某个文件/文件夹相关的所有 ZIP 缓存失效
// 包括其直接父文件夹以及所有祖先文件夹
func (s *Service) invalidateCacheForItem(userID uint, item *File) {
	// 使直接父目录的缓存失效
	if item.ParentID != nil {
		s.InvalidateFolderCache(*item.ParentID)
		// 向上递归使祖先目录的缓存失效
		s.invalidateAncestorCache(userID, *item.ParentID)
	}
	// 如果是文件夹，使自身的缓存也失效
	if item.IsDir {
		s.InvalidateFolderCache(item.ID)
	}
}

// invalidateAncestorCache 递归使祖先文件夹的缓存失效
func (s *Service) invalidateAncestorCache(userID uint, folderID uint) {
	var folder File
	if err := s.db.Where("id = ? AND user_id = ?", folderID, userID).First(&folder).Error; err != nil {
		return
	}
	if folder.ParentID != nil {
		s.InvalidateFolderCache(*folder.ParentID)
		s.invalidateAncestorCache(userID, *folder.ParentID)
	}
}

// cleanFolderCache 清除指定文件夹的所有 ZIP 缓存文件
func (s *Service) cleanFolderCache(folderID uint) {
	if s.zipDir == "" {
		return
	}
	pattern := filepath.Join(s.zipDir, fmt.Sprintf("%d_*.zip", folderID))
	matches, _ := filepath.Glob(pattern)
	for _, m := range matches {
		os.Remove(m)
	}
}

// CleanExpiredCache 清理过期的 ZIP 缓存文件（超过 24 小时）
func (s *Service) CleanExpiredCache() {
	if s.zipDir == "" {
		return
	}

	entries, err := os.ReadDir(s.zipDir)
	if err != nil {
		return
	}

	expiry := 24 * time.Hour
	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > expiry {
			os.Remove(filepath.Join(s.zipDir, entry.Name()))
		}
	}
}

// StartCacheCleaner 启动定时缓存清理 goroutine
func (s *Service) StartCacheCleaner() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.CleanExpiredCache()
			s.CleanExpiredDownloadLinks()
		}
	}()
}

// ==================== 下载短链接 ====================

const DownloadLinkExpiry = 5 * time.Minute

// CreateDownloadLink 为文件创建下载短链接
func (s *Service) CreateDownloadLink(userID uint, fileID uint, isFolder bool) (*DownloadLink, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(b)

	link := &DownloadLink{
		UserID:    userID,
		FileID:    fileID,
		IsFolder:  isFolder,
		Token:     token,
		ExpiresAt: time.Now().Add(DownloadLinkExpiry),
	}
	if err := s.db.Create(link).Error; err != nil {
		return nil, err
	}
	return link, nil
}

// ValidateDownloadLink 验证下载链接，返回 link 或 nil
func (s *Service) ValidateDownloadLink(token string) *DownloadLink {
	var link DownloadLink
	if err := s.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&link).Error; err != nil {
		return nil
	}
	return &link
}

// CleanExpiredDownloadLinks 清理过期的下载链接
func (s *Service) CleanExpiredDownloadLinks() {
	s.db.Where("expires_at < ?", time.Now()).Delete(&DownloadLink{})
}
