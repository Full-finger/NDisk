package file

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type httpRange struct {
	start, length int64
}

type Handler struct {
	service   *Service
	jwtSecret string
}

func NewHandler(service *Service, jwtSecret string) *Handler {
	return &Handler{service: service, jwtSecret: jwtSecret}
}

// TestChunk 处理 GET 请求，检查分块是否已上传
func (h *Handler) TestChunk(c *gin.Context) {
	userID := c.GetUint("user_id")

	identifier := c.Query("resumableIdentifier")
	chunkNumberStr := c.Query("resumableChunkNumber")

	if identifier == "" || chunkNumberStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing resumable parameters"})
		return
	}

	chunkNumber, err := strconv.Atoi(chunkNumberStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk number"})
		return
	}

	if h.service.ChunkExists(userID, identifier, chunkNumber) {
		c.JSON(http.StatusOK, gin.H{"message": "chunk already uploaded"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "chunk not found"})
	}
}

// Upload 上传文件（支持 resumable.js 分块上传）
func (h *Handler) Upload(c *gin.Context) {
	userID := c.GetUint("user_id")

	chunkNumber := c.PostForm("resumableChunkNumber")
	identifier := c.PostForm("resumableIdentifier")
	totalSizeStr := c.PostForm("resumableTotalSize")
	totalChunksStr := c.PostForm("resumableTotalChunks")
	filename := c.PostForm("resumableFilename")

	if chunkNumber != "" && identifier != "" {
		h.uploadChunk(c, userID, chunkNumber, identifier, totalSizeStr, totalChunksStr, filename)
		return
	}

	h.uploadWholeFile(c, userID)
}

func (h *Handler) uploadChunk(c *gin.Context, userID uint, chunkNumberStr, identifier, totalSizeStr, totalChunksStr, filename string) {
	chunkNumber, err := strconv.Atoi(chunkNumberStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk number"})
		return
	}

	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid total chunks"})
		return
	}

	totalSize, err := strconv.ParseInt(totalSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid total size"})
		return
	}

	formFile, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}

	file, err := formFile.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer file.Close()

	if err := h.service.SaveChunk(userID, identifier, chunkNumber, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save chunk"})
		return
	}

	if h.service.AllChunksUploaded(userID, identifier, totalChunks) {
		var parentID *uint
		if pid := c.PostForm("parent_id"); pid != "" {
			if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
				uid := uint(id)
				parentID = &uid
			}
		}

		result, err := h.service.UploadFromChunks(userID, parentID, filename, totalSize, identifier, totalChunks)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":        result.ID,
			"name":      result.Name,
			"size":      result.Size,
			"parent_id": result.ParentID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "chunk uploaded"})
}

func (h *Handler) uploadWholeFile(c *gin.Context, userID uint) {
	var parentID *uint
	if pid := c.PostForm("parent_id"); pid != "" {
		if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
			uid := uint(id)
			parentID = &uid
		}
	}

	formFile, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}

	file, err := formFile.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer file.Close()

	result, err := h.service.Upload(userID, parentID, formFile.Filename, formFile.Size, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":        result.ID,
		"name":      result.Name,
		"size":      result.Size,
		"parent_id": result.ParentID,
	})
}

// CreateFolder 创建文件夹
func (h *Handler) CreateFolder(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	folder, err := h.service.CreateFolder(userID, req.Name, req.ParentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": folder.ID, "name": folder.Name})
}

// List 获取文件列表
func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint("user_id")

	var parentID *uint
	if pid := c.Query("parent_id"); pid != "" {
		if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
			uid := uint(id)
			parentID = &uid
		}
	}

	files, err := h.service.List(userID, parentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	folders, _ := h.service.ListFolders(userID, parentID)

	c.JSON(http.StatusOK, gin.H{"folders": folders, "files": files})
}

// CreateDownloadLink 生成下载短链接（需要 JWT 认证）
func (h *Handler) CreateDownloadLink(c *gin.Context) {
	userID := c.GetUint("user_id")

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	// 验证文件归属
	if _, err := h.service.GetFile(userID, uint(fileID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	link, err := h.service.CreateDownloadLink(userID, uint(fileID), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create download link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": "/api/dl/" + link.Token})
}

// CreateFolderDownloadLink 生成文件夹下载短链接（需要 JWT 认证）
func (h *Handler) CreateFolderDownloadLink(c *gin.Context) {
	userID := c.GetUint("user_id")

	folderID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}

	// 验证文件夹归属
	var folder File
	if err := h.service.db.Where("id = ? AND user_id = ? AND is_dir = ?", folderID, userID, true).First(&folder).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return
	}

	link, err := h.service.CreateDownloadLink(userID, uint(folderID), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create download link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": "/api/dl/" + link.Token})
}

// DownloadByToken 通过短链接下载（无需认证）
func (h *Handler) DownloadByToken(c *gin.Context) {
	token := c.Param("token")
	link := h.service.ValidateDownloadLink(token)
	if link == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "download link expired or invalid"})
		return
	}

	if link.IsFolder {
		h.downloadFolderByLink(c, link)
	} else {
		h.downloadFileByLink(c, link)
	}
}

// downloadFileByLink 通过短链接下载文件
func (h *Handler) downloadFileByLink(c *gin.Context, link *DownloadLink) {
	file, err := h.service.GetFile(link.UserID, link.FileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("ETag", file.ETag())
	c.Header("Last-Modified", file.LastModified())
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(file.Name))

	rangeHeader := c.GetHeader("Range")
	if rangeHeader == "" {
		h.serveFullFile(c, file)
		return
	}

	if !h.checkIfRange(c, file) {
		h.serveFullFile(c, file)
		return
	}

	ranges, err := parseRange(rangeHeader, file.Size)
	if err != nil {
		c.Header("Content-Range", fmt.Sprintf("bytes */%d", file.Size))
		c.JSON(http.StatusRequestedRangeNotSatisfiable, gin.H{"error": "invalid range"})
		return
	}

	if len(ranges) == 1 {
		h.serveSingleRange(c, file, ranges[0])
	} else {
		h.serveMultipleRanges(c, file, ranges)
	}
}

// downloadFolderByLink 通过短链接下载文件夹
func (h *Handler) downloadFolderByLink(c *gin.Context, link *DownloadLink) {
	zipPath, folder, fromCache, err := h.service.GenerateFolderZip(link.UserID, link.FileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取压缩文件"})
		return
	}

	zipSize := zipInfo.Size()
	zipName := folder.Name + ".zip"

	c.Header("Accept-Ranges", "bytes")
	c.Header("ETag", fmt.Sprintf(`"%x-%d"`, link.FileID, zipSize))
	c.Header("Last-Modified", zipInfo.ModTime().UTC().Format(http.TimeFormat))
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(zipName))

	rangeHeader := c.GetHeader("Range")
	if rangeHeader == "" {
		h.serveZipFull(c, zipPath, zipSize)
		return
	}

	ranges, err := parseRange(rangeHeader, zipSize)
	if err != nil {
		c.Header("Content-Range", fmt.Sprintf("bytes */%d", zipSize))
		c.JSON(http.StatusRequestedRangeNotSatisfiable, gin.H{"error": "invalid range"})
		return
	}

	if len(ranges) == 1 {
		h.serveZipRange(c, zipPath, zipSize, ranges[0])
	} else {
		h.serveZipMultipleRanges(c, zipPath, zipSize, ranges)
	}

	if !fromCache {
		log.Printf("Generated ZIP for folder %d, size: %d bytes", link.FileID, zipSize)
	}
}

// serveFullFile 返回完整文件
func (h *Handler) serveFullFile(c *gin.Context, file *File) {
	_, reader, err := h.service.Download(file.UserID, file.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read file"})
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}

func (h *Handler) serveSingleRange(c *gin.Context, file *File, r httpRange) {
	_, reader, err := h.service.DownloadRange(file.UserID, file.ID, r.start, r.length)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read file"})
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(r.length, 10))
	c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, file.Size))
	c.Status(http.StatusPartialContent)
	io.Copy(c.Writer, reader)
}

func (h *Handler) serveMultipleRanges(c *gin.Context, file *File, ranges []httpRange) {
	boundary := fmt.Sprintf("BOUNDARY-%d", file.ID)
	c.Header("Content-Type", "multipart/byteranges; boundary="+boundary)
	c.Status(http.StatusPartialContent)

	for _, r := range ranges {
		c.Writer.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		c.Writer.WriteString("Content-Type: application/octet-stream\r\n")
		c.Writer.WriteString(fmt.Sprintf("Content-Range: bytes %d-%d/%d\r\n\r\n", r.start, r.start+r.length-1, file.Size))

		_, reader, err := h.service.DownloadRange(file.UserID, file.ID, r.start, r.length)
		if err != nil {
			return
		}
		io.Copy(c.Writer, reader)
		reader.Close()
	}
	c.Writer.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))
}

func (h *Handler) checkIfRange(c *gin.Context, file *File) bool {
	ifRange := c.GetHeader("If-Range")
	if ifRange == "" {
		return true
	}
	if strings.Trim(ifRange, `"`) == strings.Trim(file.ETag(), `"`) {
		return true
	}
	if ifRange == file.LastModified() {
		return true
	}
	return false
}

func parseRange(rangeHeader string, fileSize int64) ([]httpRange, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, fmt.Errorf("invalid range header")
	}

	specs := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), ",")
	var ranges []httpRange

	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}

		parts := strings.Split(spec, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range spec")
		}

		var start, end int64
		var err error

		if parts[0] == "" {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, err
			}
			if end > fileSize {
				end = fileSize
			}
			start = fileSize - end
			end = fileSize - 1
		} else if parts[1] == "" {
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return nil, err
			}
			end = fileSize - 1
		} else {
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return nil, err
			}
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, err
			}
		}

		if start > end || start >= fileSize {
			return nil, fmt.Errorf("range out of bounds")
		}

		if end >= fileSize {
			end = fileSize - 1
		}

		ranges = append(ranges, httpRange{start: start, length: end - start + 1})
	}

	if len(ranges) == 0 {
		return nil, fmt.Errorf("no valid ranges")
	}

	return ranges, nil
}

// Delete 删除文件
func (h *Handler) Delete(c *gin.Context) {
	userID := c.GetUint("user_id")
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	if err := h.service.Delete(userID, uint(fileID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// Rename 重命名
func (h *Handler) Rename(c *gin.Context) {
	userID := c.GetUint("user_id")
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	var req RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	file, err := h.service.Rename(userID, uint(fileID), req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": file.ID, "name": file.Name})
}

// Move 移动
func (h *Handler) Move(c *gin.Context) {
	userID := c.GetUint("user_id")
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	var req MoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	file, err := h.service.Move(userID, uint(fileID), req.TargetID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": file.ID, "name": file.Name, "parent_id": file.ParentID})
}

// ListAllFolders 获取所有文件夹
func (h *Handler) ListAllFolders(c *gin.Context) {
	userID := c.GetUint("user_id")
	folders, err := h.service.ListAllFolders(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"folders": folders})
}

// serveZipFull 返回完整 ZIP 文件
func (h *Handler) serveZipFull(c *gin.Context, zipPath string, zipSize int64) {
	f, err := os.Open(zipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取压缩文件"})
		return
	}
	defer f.Close()

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Length", strconv.FormatInt(zipSize, 10))
	c.Status(http.StatusOK)
	io.Copy(c.Writer, f)
}

func (h *Handler) serveZipRange(c *gin.Context, zipPath string, zipSize int64, r httpRange) {
	f, err := os.Open(zipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取压缩文件"})
		return
	}
	defer f.Close()

	if _, err := f.Seek(r.start, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法定位文件"})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Length", strconv.FormatInt(r.length, 10))
	c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, zipSize))
	c.Status(http.StatusPartialContent)
	io.Copy(c.Writer, io.LimitReader(f, r.length))
}

func (h *Handler) serveZipMultipleRanges(c *gin.Context, zipPath string, zipSize int64, ranges []httpRange) {
	boundary := fmt.Sprintf("BOUNDARY-ZIP-%d", time.Now().UnixNano())
	c.Header("Content-Type", "multipart/byteranges; boundary="+boundary)
	c.Status(http.StatusPartialContent)

	for _, r := range ranges {
		c.Writer.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		c.Writer.WriteString("Content-Type: application/zip\r\n")
		c.Writer.WriteString(fmt.Sprintf("Content-Range: bytes %d-%d/%d\r\n\r\n", r.start, r.start+r.length-1, zipSize))

		f, err := os.Open(zipPath)
		if err != nil {
			return
		}
		f.Seek(r.start, io.SeekStart)
		io.Copy(c.Writer, io.LimitReader(f, r.length))
		f.Close()
	}
	c.Writer.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))
}
