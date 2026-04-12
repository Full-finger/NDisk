package nfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Full-finger/NDisk/internal/file"
	"github.com/Full-finger/NDisk/internal/storage"
	billy "github.com/go-git/go-billy/v5"
)

// NDiskFS 实现 billy.Filesystem 接口，将 NDisk 虚拟文件系统映射为 POSIX 文件系统
type NDiskFS struct {
	userID      uint
	fileService *file.Service
	store       storage.Storage
	hmacSecret  string

	// 写入缓存：path -> *openFileForWrite
	mu        sync.Mutex
	openFiles map[string]*openFileForWrite
}

type openFileForWrite struct {
	buf        bytes.Buffer
	parentID   *uint
	filename   string
	existingID *uint // 如果是覆盖已存在的文件
}

// ndiskFileInfo 实现 os.FileInfo
type ndiskFileInfo struct {
	f *file.File
}

func (fi *ndiskFileInfo) Name() string { return fi.f.Name }
func (fi *ndiskFileInfo) Size() int64  { return fi.f.Size }
func (fi *ndiskFileInfo) Mode() os.FileMode {
	if fi.f.IsDir {
		return os.ModeDir | 0755
	}
	return 0644
}
func (fi *ndiskFileInfo) ModTime() time.Time { return fi.f.UpdatedAt }
func (fi *ndiskFileInfo) IsDir() bool        { return fi.f.IsDir }
func (fi *ndiskFileInfo) Sys() interface{}   { return fi.f }

// ndiskFile 实现 billy.File
type ndiskFile struct {
	fs       *NDiskFS
	name     string
	path     string // 完整路径，用于 CommitWrite 的 key
	reader   io.ReadCloser
	writer   *bytes.Buffer
	fileInfo *file.File
	isWrite  bool
	offset   int64
}

func (f *ndiskFile) Name() string { return f.name }

func (f *ndiskFile) Read(p []byte) (n int, err error) {
	if f.reader != nil {
		return f.reader.Read(p)
	}
	return 0, io.EOF
}

func (f *ndiskFile) ReadAt(p []byte, off int64) (n int, err error) {
	if f.reader != nil {
		// 需要重新打开从 offset 开始读
		f.reader.Close()
		r, err := f.fs.store.OpenRange(f.fileInfo.StorageKey, off, int64(len(p)))
		if err != nil {
			return 0, err
		}
		n, err = io.ReadFull(r, p)
		r.Close()
		return n, err
	}
	return 0, io.EOF
}

func (f *ndiskFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		if f.fileInfo != nil {
			f.offset = f.fileInfo.Size + offset
		}
	}
	if f.reader != nil && f.fileInfo != nil {
		f.reader.Close()
		r, err := f.fs.store.OpenRange(f.fileInfo.StorageKey, f.offset, f.fileInfo.Size-f.offset)
		if err != nil {
			return 0, err
		}
		f.reader = r
	}
	return f.offset, nil
}

func (f *ndiskFile) Write(p []byte) (n int, err error) {
	if f.writer != nil {
		return f.writer.Write(p)
	}
	return 0, fmt.Errorf("file not open for writing")
}

func (f *ndiskFile) Close() error {
	if f.reader != nil {
		return f.reader.Close()
	}
	if f.isWrite && f.path != "" {
		return f.fs.CommitWrite(f.path)
	}
	return nil
}

func (f *ndiskFile) Lock() error               { return nil }
func (f *ndiskFile) Unlock() error             { return nil }
func (f *ndiskFile) Truncate(size int64) error { return nil }

// NewNDiskFS 创建一个新的 NDisk 文件系统实例
func NewNDiskFS(userID uint, fileService *file.Service, store storage.Storage, hmacSecret string) *NDiskFS {
	return &NDiskFS{
		userID:      userID,
		fileService: fileService,
		store:       store,
		hmacSecret:  hmacSecret,
		openFiles:   make(map[string]*openFileForWrite),
	}
}

// resolvePath 根据路径解析到 file.File
// 路径格式: "/" 或 "" = 根目录, "/dir/file" = 目录下的文件
func (fs *NDiskFS) resolvePath(path string) (*file.File, error) {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		// 根目录 - 返回一个虚拟根
		return &file.File{
			ID:     0,
			UserID: fs.userID,
			Name:   "/",
			IsDir:  true,
		}, nil
	}

	parts := strings.Split(path, "/")
	currentParentID := (*uint)(nil)

	for i, part := range parts {
		if part == "" {
			continue
		}
		isLast := i == len(parts)-1

		// 在当前目录下查找名为 part 的项
		var items []file.File
		query := fs.fileService.DB().Where("user_id = ? AND name = ? AND deleted_at IS NULL", fs.userID, part)
		if currentParentID == nil {
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *currentParentID)
		}

		if err := query.First(&items).Error; err != nil {
			return nil, os.ErrNotExist
		}
		if len(items) == 0 {
			return nil, os.ErrNotExist
		}

		item := items[0]
		if isLast {
			return &item, nil
		}
		if !item.IsDir {
			return nil, os.ErrNotExist
		}
		id := item.ID
		currentParentID = &id
	}

	return nil, os.ErrNotExist
}

// resolveParentID 解析路径的父目录 ID
func (fs *NDiskFS) resolveParentID(path string) (*uint, error) {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		return nil, nil // 根目录
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return nil, nil
	}

	// 只解析到最后一个目录
	currentParentID := (*uint)(nil)
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		var item file.File
		query := fs.fileService.DB().Where("user_id = ? AND name = ? AND is_dir = ? AND deleted_at IS NULL", fs.userID, part, true)
		if currentParentID == nil {
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *currentParentID)
		}
		if err := query.First(&item).Error; err != nil {
			return nil, os.ErrNotExist
		}
		id := item.ID
		currentParentID = &id
	}

	return currentParentID, nil
}

// basename 获取路径的最后一部分
func basename(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// ===== billy.Basic 接口实现 =====

func (fs *NDiskFS) Create(filename string) (billy.File, error) {
	filename = filepath.ToSlash(filename)
	filename = strings.TrimPrefix(filename, "/")

	name := basename(filename)
	if name == "" {
		return nil, os.ErrInvalid
	}

	parentID, err := fs.resolveParentID(filename)
	if err != nil {
		return nil, err
	}

	// 检查是否已存在
	var existing file.File
	query := fs.fileService.DB().Where("user_id = ? AND name = ? AND is_dir = ? AND deleted_at IS NULL", fs.userID, name, false)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	existingID := (*uint)(nil)
	if err := query.First(&existing).Error; err == nil {
		id := existing.ID
		existingID = &id
	}

	of := &openFileForWrite{
		buf:        bytes.Buffer{},
		parentID:   parentID,
		filename:   name,
		existingID: existingID,
	}
	fs.mu.Lock()
	fs.openFiles[filename] = of
	fs.mu.Unlock()

	return &ndiskFile{
		fs:      fs,
		name:    name,
		path:    filename,
		writer:  &of.buf,
		isWrite: true,
	}, nil
}

func (fs *NDiskFS) Open(filename string) (billy.File, error) {
	f, err := fs.resolvePath(filename)
	if err != nil {
		return nil, err
	}
	if f.IsDir {
		return &ndiskFile{
			fs:       fs,
			name:     f.Name,
			fileInfo: f,
		}, nil
	}

	reader, err := fs.store.Open(f.StorageKey)
	if err != nil {
		return nil, err
	}

	return &ndiskFile{
		fs:       fs,
		name:     f.Name,
		reader:   reader,
		fileInfo: f,
	}, nil
}

func (fs *NDiskFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	filename = filepath.ToSlash(filename)
	filename = strings.TrimPrefix(filename, "/")

	// 尝试打开已存在的文件
	f, err := fs.resolvePath(filename)
	if err == nil && !f.IsDir {
		if flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0 {
			// 以写模式打开 - 创建写入缓冲区
			name := basename(filename)
			parentID, _ := fs.resolveParentID(filename)
			id := f.ID
			of := &openFileForWrite{
				buf:        bytes.Buffer{},
				parentID:   parentID,
				filename:   name,
				existingID: &id,
			}
			fs.mu.Lock()
			fs.openFiles[filename] = of
			fs.mu.Unlock()
			return &ndiskFile{
				fs:       fs,
				name:     name,
				path:     filename,
				writer:   &of.buf,
				isWrite:  true,
				fileInfo: f,
			}, nil
		}
		// 读模式
		reader, err := fs.store.Open(f.StorageKey)
		if err != nil {
			return nil, err
		}
		return &ndiskFile{
			fs:       fs,
			name:     f.Name,
			reader:   reader,
			fileInfo: f,
		}, nil
	}

	// 文件不存在，且是创建模式
	if flag&os.O_CREATE != 0 {
		return fs.Create(filename)
	}

	return nil, os.ErrNotExist
}

func (fs *NDiskFS) Stat(filename string) (os.FileInfo, error) {
	f, err := fs.resolvePath(filename)
	if err != nil {
		return nil, err
	}
	return &ndiskFileInfo{f: f}, nil
}

func (fs *NDiskFS) Rename(oldpath, newpath string) error {
	oldpath = filepath.ToSlash(oldpath)
	newpath = filepath.ToSlash(newpath)
	oldpath = strings.TrimPrefix(oldpath, "/")
	newpath = strings.TrimPrefix(newpath, "/")

	oldName := basename(oldpath)
	newName := basename(newpath)
	if oldName == "" || newName == "" {
		return os.ErrInvalid
	}

	// 查找源文件
	srcFile, err := fs.resolvePath(oldpath)
	if err != nil {
		return err
	}

	// 如果只是重命名（同目录）
	oldParentDir := filepath.Dir(oldpath)
	newParentDir := filepath.Dir(newpath)
	if filepath.ToSlash(oldParentDir) == filepath.ToSlash(newParentDir) ||
		(oldParentDir == "." && newParentDir == ".") {
		// 同目录重命名
		_, err := fs.fileService.Rename(fs.userID, srcFile.ID, newName)
		return err
	}

	// 跨目录移动 + 重命名
	newParentID, err := fs.resolveParentID(newpath)
	if err != nil {
		return err
	}

	// 先移动
	if _, err := fs.fileService.Move(fs.userID, srcFile.ID, newParentID); err != nil {
		return err
	}
	// 如果需要重命名
	if oldName != newName {
		_, err := fs.fileService.Rename(fs.userID, srcFile.ID, newName)
		return err
	}
	return nil
}

func (fs *NDiskFS) Remove(filename string) error {
	f, err := fs.resolvePath(filename)
	if err != nil {
		return err
	}
	return fs.fileService.Delete(fs.userID, f.ID)
}

func (fs *NDiskFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}

// ===== billy.TempFile 接口实现 =====

func (fs *NDiskFS) TempFile(dir, prefix string) (billy.File, error) {
	// 生成临时文件名
	tmpName := prefix + fmt.Sprintf("_%d", time.Now().UnixNano())
	fullPath := filepath.Join(dir, tmpName)
	return fs.Create(fullPath)
}

// ===== billy.Dir 接口实现 =====

func (fs *NDiskFS) ReadDir(path string) ([]os.FileInfo, error) {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	var parentID *uint
	if path != "" {
		f, err := fs.resolvePath(path)
		if err != nil {
			return nil, err
		}
		if !f.IsDir {
			return nil, fmt.Errorf("not a directory")
		}
		id := f.ID
		parentID = &id
	}

	// 获取文件和文件夹列表
	files, err := fs.fileService.List(fs.userID, parentID)
	if err != nil {
		return nil, err
	}
	folders, err := fs.fileService.ListFolders(fs.userID, parentID)
	if err != nil {
		return nil, err
	}

	var infos []os.FileInfo
	for i := range folders {
		infos = append(infos, &ndiskFileInfo{f: &folders[i]})
	}
	for i := range files {
		infos = append(infos, &ndiskFileInfo{f: &files[i]})
	}

	// 按名称排序
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name() < infos[j].Name()
	})

	return infos, nil
}

func (fs *NDiskFS) MkdirAll(filename string, perm os.FileMode) error {
	filename = filepath.ToSlash(filename)
	filename = strings.TrimPrefix(filename, "/")
	filename = strings.TrimSuffix(filename, "/")

	if filename == "" {
		return nil
	}

	parts := strings.Split(filename, "/")
	currentParentID := (*uint)(nil)

	for _, part := range parts {
		if part == "" {
			continue
		}
		// 检查是否已存在
		var existing file.File
		query := fs.fileService.DB().Where("user_id = ? AND name = ? AND deleted_at IS NULL", fs.userID, part)
		if currentParentID == nil {
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *currentParentID)
		}

		if err := query.First(&existing).Error; err != nil {
			// 不存在，创建
			folder, err := fs.fileService.CreateFolder(fs.userID, part, currentParentID)
			if err != nil {
				// 可能并发创建导致冲突，忽略
				if strings.Contains(err.Error(), "已存在") {
					// 重新查找
					query.Find(&existing)
					id := existing.ID
					currentParentID = &id
					continue
				}
				return err
			}
			id := folder.ID
			currentParentID = &id
		} else {
			id := existing.ID
			currentParentID = &id
		}
	}
	return nil
}

// ===== billy.Symlink 接口实现 =====

func (fs *NDiskFS) Lstat(filename string) (os.FileInfo, error) {
	return fs.Stat(filename)
}

func (fs *NDiskFS) Symlink(target, link string) error {
	return fmt.Errorf("symlinks not supported")
}

func (fs *NDiskFS) Readlink(link string) (string, error) {
	return "", fmt.Errorf("readlink not supported")
}

// ===== billy.Chroot 接口实现 =====

func (fs *NDiskFS) Chroot(path string) (billy.Filesystem, error) {
	return fs, nil // 已经是用户根目录
}

func (fs *NDiskFS) Root() string {
	return "/"
}

// ===== billy.Capable 接口实现 =====

func (fs *NDiskFS) Capabilities() billy.Capability {
	return billy.ReadCapability | billy.WriteCapability | billy.SeekCapability
}

// ===== 写入提交方法 =====

// CommitWrite 提交一个打开的写入文件
func (fs *NDiskFS) CommitWrite(path string) error {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "/")

	fs.mu.Lock()
	of, ok := fs.openFiles[path]
	if !ok {
		fs.mu.Unlock()
		return fmt.Errorf("no open file for path: %s", path)
	}
	delete(fs.openFiles, path)
	fs.mu.Unlock()

	data := of.buf.Bytes()
	size := int64(len(data))

	// 如果覆盖已有文件，先删除旧的
	if of.existingID != nil {
		fs.fileService.Delete(fs.userID, *of.existingID)
	}

	// 上传新文件
	_, err := fs.fileService.Upload(fs.userID, of.parentID, of.filename, size, bytes.NewReader(data))
	return err
}

// CommitAllWrites 提交所有未提交的写入
func (fs *NDiskFS) CommitAllWrites() error {
	fs.mu.Lock()
	paths := make([]string, 0, len(fs.openFiles))
	for p := range fs.openFiles {
		paths = append(paths, p)
	}
	fs.mu.Unlock()

	for _, p := range paths {
		if err := fs.CommitWrite(p); err != nil {
			return err
		}
	}
	return nil
}

// computeContentHash 计算内容哈希
func computeContentHash(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
