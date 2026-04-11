package share

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/Full-finger/NDisk/internal/file"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	shareService *Service
	fileService  *file.Service
}

func NewHandler(shareService *Service, fileService *file.Service) *Handler {
	return &Handler{
		shareService: shareService,
		fileService:  fileService,
	}
}

// CreateShare 创建分享（需要 JWT 认证）
func (h *Handler) CreateShare(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req CreateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证文件/文件夹归属
	if req.IsFolder {
		if _, err := h.fileService.GetFolder(userID, uint(req.ItemID)); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件夹不存在"})
			return
		}
	} else {
		if _, err := h.fileService.GetFile(userID, uint(req.ItemID)); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
	}

	share, err := h.shareService.CreateShare(userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"share_token": share.ShareToken})
}

// SharePage 渲染分享访问页面（公开）
func (h *Handler) SharePage(c *gin.Context) {
	token := c.Param("token")

	share, err := h.shareService.GetShareByToken(token)
	if err != nil {
		c.HTML(http.StatusNotFound, "share_error", gin.H{
			"title": "分享不存在",
			"error": "分享不存在或已过期",
		})
		return
	}

	// 获取文件/文件夹信息
	var itemName string
	var itemSize int64
	if share.IsFolder {
		folder, err := h.fileService.GetFolder(share.UserID, share.ItemID)
		if err != nil {
			c.HTML(http.StatusNotFound, "share_error", gin.H{
				"title": "文件不存在",
				"error": "原始文件已被删除",
			})
			return
		}
		itemName = folder.Name
	} else {
		f, err := h.fileService.GetFile(share.UserID, share.ItemID)
		if err != nil {
			c.HTML(http.StatusNotFound, "share_error", gin.H{
				"title": "文件不存在",
				"error": "原始文件已被删除",
			})
			return
		}
		itemName = f.Name
		itemSize = f.Size
	}

	needsPassword := h.shareService.HasPassword(share)

	c.HTML(http.StatusOK, "share", gin.H{
		"title":          share.ProjectName,
		"project_name":   share.ProjectName,
		"item_name":      itemName,
		"item_size":      itemSize,
		"is_folder":      share.IsFolder,
		"needs_password": needsPassword,
		"share_token":    share.ShareToken,
	})
}

// VerifyPassword 验证分享密码（公开）
func (h *Handler) VerifyPassword(c *gin.Context) {
	token := c.Param("token")

	share, err := h.shareService.GetShareByToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享不存在或已过期"})
		return
	}

	var req VerifyPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !h.shareService.VerifyPassword(share, req.Password) {
		c.JSON(http.StatusForbidden, gin.H{"error": "密码错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "验证成功"})
}

// Download 通过分享下载文件/文件夹（公开）
func (h *Handler) Download(c *gin.Context) {
	token := c.Param("token")

	share, err := h.shareService.GetShareByToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享不存在或已过期"})
		return
	}

	// 如果有密码，检查是否通过密码验证
	if h.shareService.HasPassword(share) {
		password := c.Query("password")
		if !h.shareService.VerifyPassword(share, password) {
			c.JSON(http.StatusForbidden, gin.H{"error": "密码错误"})
			return
		}
	}

	if share.IsFolder {
		h.downloadFolder(c, share)
	} else {
		h.downloadFile(c, share)
	}
}

func (h *Handler) downloadFile(c *gin.Context, share *Share) {
	f, err := h.fileService.GetFile(share.UserID, share.ItemID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	_, reader, err := h.fileService.Download(share.UserID, share.ItemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取文件"})
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(f.Name))
	c.Header("Content-Length", strconv.FormatInt(f.Size, 10))
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}

func (h *Handler) downloadFolder(c *gin.Context, share *Share) {
	zipPath, folder, _, err := h.fileService.GenerateFolderZip(share.UserID, share.ItemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	zipFile, err := os.Open(zipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取文件"})
		return
	}
	defer zipFile.Close()

	zipInfo, err := zipFile.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取文件信息"})
		return
	}

	zipName := folder.Name + ".zip"
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(zipName))
	c.Header("Content-Length", strconv.FormatInt(zipInfo.Size(), 10))
	c.Status(http.StatusOK)
	io.Copy(c.Writer, zipFile)
}
