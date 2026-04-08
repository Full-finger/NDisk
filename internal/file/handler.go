package file

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Full-finger/NDisk/internal/auth"
	"github.com/gin-gonic/gin"
)

// httpRange 表示一个字节范围
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

	if h.service.ChunkExists(identifier, chunkNumber) {
		c.JSON(http.StatusOK, gin.H{"message": "chunk already uploaded"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "chunk not found"})
	}
}

// 上传文件（支持 resumable.js 分块上传）
func (h *Handler) Upload(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 检查是否为 resumable.js 分块上传
	chunkNumber := c.PostForm("resumableChunkNumber")
	identifier := c.PostForm("resumableIdentifier")
	totalSizeStr := c.PostForm("resumableTotalSize")
	totalChunksStr := c.PostForm("resumableTotalChunks")
	filename := c.PostForm("resumableFilename")

	if chunkNumber != "" && identifier != "" {
		// 分块上传模式
		h.uploadChunk(c, userID, chunkNumber, identifier, totalSizeStr, totalChunksStr, filename)
		return
	}

	// 普通整文件上传模式（向后兼容）
	h.uploadWholeFile(c, userID)
}

// uploadChunk 处理分块上传
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

	// 获取上传的 chunk 文件
	formFile, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}

	// 打开文件流
	file, err := formFile.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer file.Close()

	// 保存分块
	if err := h.service.SaveChunk(identifier, chunkNumber, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save chunk"})
		return
	}

	// 检查是否所有分块都已上传
	if h.service.AllChunksUploaded(identifier, totalChunks) {
		// 获取 parent_id
		var parentID *uint
		if pid := c.PostForm("parent_id"); pid != "" {
			if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
				uid := uint(id)
				parentID = &uid
			}
		}

		// 合并所有分块
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

	// 分块已保存，但未全部上传完成
	c.JSON(http.StatusOK, gin.H{"message": "chunk uploaded"})
}

// uploadWholeFile 处理普通整文件上传（向后兼容）
func (h *Handler) uploadWholeFile(c *gin.Context, userID uint) {
	// 获取 parent_id（可选）
	var parentID *uint
	if pid := c.PostForm("parent_id"); pid != "" {
		if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
			uid := uint(id)
			parentID = &uid
		}
	}

	// 获取上传的文件
	formFile, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}

	// 打开文件流
	file, err := formFile.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer file.Close()

	// 上传
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

// 创建文件夹
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

	c.JSON(http.StatusOK, gin.H{
		"id":   folder.ID,
		"name": folder.Name,
	})
}

// 获取文件列表
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

	// 同时获取目录列表
	folders, _ := h.service.ListFolders(userID, parentID)

	c.JSON(http.StatusOK, gin.H{
		"folders": folders,
		"files":   files,
	})
}

// 下载文件（支持Range请求，从Cookie或Authorization header认证）
func (h *Handler) Download(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 如果没有通过中间件获取到userID，尝试从Cookie解析
	if userID == 0 {
		if tokenStr, err := c.Cookie("token"); err == nil && tokenStr != "" {
			if claims, err := auth.ParseToken(tokenStr, h.jwtSecret); err == nil {
				userID = claims.UserID
			}
		}
	}

	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	// 获取文件信息
	file, err := h.service.GetFile(userID, uint(fileID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	// 设置通用头
	c.Header("Accept-Ranges", "bytes")
	c.Header("ETag", file.ETag())
	c.Header("Last-Modified", file.LastModified())
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(file.Name))

	// 解析Range头
	rangeHeader := c.GetHeader("Range")
	if rangeHeader == "" {
		h.serveFullFile(c, file)
		return
	}

	// 检查If-Range
	if !h.checkIfRange(c, file) {
		h.serveFullFile(c, file)
		return
	}

	// 解析范围
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

// serveSingleRange 返回单范围内容
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

// serveMultipleRanges 返回多范围内容
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

// checkIfRange 检查If-Range条件（支持ETag和Last-Modified）
func (h *Handler) checkIfRange(c *gin.Context, file *File) bool {
	ifRange := c.GetHeader("If-Range")
	if ifRange == "" {
		return true
	}
	// ETag匹配（支持带引号和不带引号）
	if strings.Trim(ifRange, `"`) == strings.Trim(file.ETag(), `"`) {
		return true
	}
	// Last-Modified匹配
	if ifRange == file.LastModified() {
		return true
	}
	return false
}

// parseRange 解析Range头
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
			// -500: 最后500字节
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
			// 500-: 从500到结尾
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return nil, err
			}
			end = fileSize - 1
		} else {
			// 0-499: 前500字节
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

// DownloadHead 处理HEAD请求，只返回文件信息不返回内容
func (h *Handler) DownloadHead(c *gin.Context) {
	userID := c.GetUint("user_id")
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	file, err := h.service.GetFile(userID, uint(fileID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("ETag", file.ETag())
	c.Header("Last-Modified", file.LastModified())
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(file.Name))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
	c.Status(http.StatusOK)
}

// 删除文件
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

// 重命名文件或文件夹
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

	c.JSON(http.StatusOK, gin.H{
		"id":   file.ID,
		"name": file.Name,
	})
}
