package file

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// 上传文件
func (h *Handler) Upload(c *gin.Context) {
	userID := c.GetUint("user_id")

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

// 下载文件
func (h *Handler) Download(c *gin.Context) {
	userID := c.GetUint("user_id")
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	file, reader, err := h.service.Download(userID, uint(fileID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	defer reader.Close()

	// 设置下载头
	c.Header("Content-Disposition", "attachment; filename="+file.Name)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(file.Size, 10))

	// 流式传输
	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		// 记录日志，但已经发送头部，无法返回错误
		return
	}
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
